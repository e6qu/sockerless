// Package spawner runs a GitHub Actions runner image as a Cloud Run
// Job using the `cloud.google.com/go/run/apiv2` SDK. One Cloud Run
// Job execution per queued workflow_job. The runner image's
// entrypoint registers with GitHub using `RUNNER_REG_TOKEN`, runs the
// job, and exits — Cloud Run terminates the execution on subprocess
// exit (no idle wait needed; Cloud Run Jobs are one-shot).
//
// Differs from the AWS-side dispatcher's spawner.go in one important
// way: there is no shared docker daemon. Each spawn talks directly to
// the GCP control plane via the Run v2 REST client. State recovery
// uses Cloud Run Jobs labels (the GCP-equivalent of docker labels)
// the same way the docker spawner uses container labels.
//
// Job-name shape: `gh-<short(jobID)>-<random>`. GCP's Job ID rules
// (lowercase alphanumerics + hyphens, max 49 chars) preclude the
// `<repo>/<runner-name>` shape the docker dispatcher uses; the long
// names are stored in labels for state recovery instead.
package spawner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	run "cloud.google.com/go/run/apiv2"
	runpb "cloud.google.com/go/run/apiv2/runpb"
	"google.golang.org/protobuf/types/known/durationpb"
)

// Labels stamped on every Cloud Run Job so a restarted dispatcher can
// rediscover its state from the Cloud Run control plane (no on-disk
// dispatcher state). Real GCP enforces label-key constraints
// (lowercase letters, numbers, hyphens, underscores, max 63 chars).
const (
	LabelJobID      = "sockerless-dispatcher-job-id"
	LabelRunnerName = "sockerless-dispatcher-runner-name"
	LabelManagedBy  = "sockerless-dispatcher-managed-by"
	LabelManagedVal = "github-runner-dispatcher-gcp"
)

// Request is one spawn directive. Every field required (no fallbacks
// per project rule) — caller validates before invoking Spawn.
type Request struct {
	Project        string // GCP project ID
	Region         string // Cloud Run region (e.g. "us-central1")
	Image          string // runner image URI (Artifact Registry)
	RegToken       string // GitHub ephemeral runner registration token
	Repo           string // owner/repo for runner registration
	RunnerName     string // unique name; logs / Actions UI uses it
	Labels         []string
	JobID          int64  // GitHub workflow_job ID — written to LabelJobID for restart recovery
	ServiceAccount string // GCP service account email (Job execution identity)
	BuildBucket    string // GCS bucket for sockerless backend's `docker build` context uploads
	// BackendKind selects which sockerless backend the runner image
	// bakes — drives which SOCKERLESS_<KIND>_* env vars get set on
	// the Job. "cloudrun" or "gcf".
	BackendKind string
	// RunnerWorkspaceBucket is the GCS bucket that backs the runner-
	// task workspace shared volume (`/tmp/runner-work` and
	// `/opt/runner/externals`). Mounted on the spawned runner Cloud
	// Run Job via Volume{Gcs{Bucket}}; the in-image sockerless backend
	// reads `SOCKERLESS_GCP_SHARED_VOLUMES` to translate sub-task bind
	// mounts into the same bucket. BUG-909 — Phase-110b-equivalent.
	RunnerWorkspaceBucket string
}

// Spawn calls Cloud Run Jobs CreateJob then RunJob. Returns the Job
// resource name (`projects/.../locations/.../jobs/<jobID>`) on
// success — caller persists this for state recovery + cleanup.
func Spawn(ctx context.Context, req Request) (string, error) {
	if req.Project == "" {
		return "", fmt.Errorf("project required")
	}
	if req.Region == "" {
		return "", fmt.Errorf("region required")
	}
	if req.Image == "" {
		return "", fmt.Errorf("image required")
	}
	if req.RegToken == "" {
		return "", fmt.Errorf("registration token required")
	}
	if req.Repo == "" {
		return "", fmt.Errorf("repo required")
	}
	if req.RunnerName == "" {
		return "", fmt.Errorf("runner name required")
	}
	cli, err := run.NewJobsRESTClient(ctx)
	if err != nil {
		return "", fmt.Errorf("new jobs REST client: %w", err)
	}
	defer func() { _ = cli.Close() }()

	parent := fmt.Sprintf("projects/%s/locations/%s", req.Project, req.Region)
	jobID := jobIDFromRunnerName(req.RunnerName, req.JobID)
	fullName := fmt.Sprintf("%s/jobs/%s", parent, jobID)

	if req.BuildBucket == "" {
		return "", fmt.Errorf("BuildBucket required (sockerless backend's docker build context bucket)")
	}
	if req.BackendKind == "" {
		return "", fmt.Errorf("BackendKind required (\"cloudrun\" or \"gcf\")")
	}
	if req.RunnerWorkspaceBucket == "" {
		return "", fmt.Errorf("RunnerWorkspaceBucket required (GCS bucket backing the runner-task /tmp/runner-work + /opt/runner/externals shared volumes — BUG-909)")
	}

	// Sockerless backend env vars — required (fail-loudly contract).
	// BackendKind selects which prefix (SOCKERLESS_GCR_* for cloudrun,
	// SOCKERLESS_GCF_* for gcf) the in-image backend reads.
	var prefix string
	switch req.BackendKind {
	case "cloudrun":
		prefix = "SOCKERLESS_GCR_"
	case "gcf":
		prefix = "SOCKERLESS_GCF_"
	default:
		return "", fmt.Errorf("unknown BackendKind %q (want \"cloudrun\" or \"gcf\")", req.BackendKind)
	}

	// SharedVolumes string the in-image sockerless backend reads to
	// translate sub-task bind mounts. Two named volumes back the
	// github-actions-runner workspace; both ride one GCS bucket via
	// distinct mount paths. Subpath-style binds the runner emits
	// (`/tmp/runner-work/_temp` etc.) drop via isSubPathOfSharedVolume.
	sharedVolumesEnv := fmt.Sprintf(
		"runner-workspace=/tmp/runner-work=%s,runner-externals=/opt/runner/externals=%s",
		req.RunnerWorkspaceBucket, req.RunnerWorkspaceBucket,
	)

	containerCfg := &runpb.Container{
		Image: req.Image,
		Env: []*runpb.EnvVar{
			{Name: "RUNNER_REG_TOKEN", Values: &runpb.EnvVar_Value{Value: req.RegToken}},
			{Name: "RUNNER_REPO", Values: &runpb.EnvVar_Value{Value: req.Repo}},
			{Name: "RUNNER_NAME", Values: &runpb.EnvVar_Value{Value: req.RunnerName}},
			{Name: "RUNNER_LABELS", Values: &runpb.EnvVar_Value{Value: strings.Join(req.Labels, ",")}},
			// BUG-910: bootstrap.sh wraps run.sh with `timeout`; the
			// 60s default kills the runner mid-job. 3600s = upper
			// bound for one CI pipeline; --once / --ephemeral exits
			// naturally when the single job completes.
			{Name: "RUNNER_IDLE_SECONDS", Values: &runpb.EnvVar_Value{Value: "3600"}},
			{Name: prefix + "PROJECT", Values: &runpb.EnvVar_Value{Value: req.Project}},
			{Name: prefix + "REGION", Values: &runpb.EnvVar_Value{Value: req.Region}},
			{Name: "SOCKERLESS_GCP_BUILD_BUCKET", Values: &runpb.EnvVar_Value{Value: req.BuildBucket}},
			{Name: "SOCKERLESS_GCP_SHARED_VOLUMES", Values: &runpb.EnvVar_Value{Value: sharedVolumesEnv}},
		},
		VolumeMounts: []*runpb.VolumeMount{
			{Name: "runner-workspace", MountPath: "/tmp/runner-work"},
			{Name: "runner-externals", MountPath: "/opt/runner/externals"},
		},
	}

	template := &runpb.ExecutionTemplate{
		Template: &runpb.TaskTemplate{
			Containers: []*runpb.Container{containerCfg},
			Volumes: []*runpb.Volume{
				{
					Name:       "runner-workspace",
					VolumeType: &runpb.Volume_Gcs{Gcs: &runpb.GCSVolumeSource{Bucket: req.RunnerWorkspaceBucket}},
				},
				{
					Name:       "runner-externals",
					VolumeType: &runpb.Volume_Gcs{Gcs: &runpb.GCSVolumeSource{Bucket: req.RunnerWorkspaceBucket}},
				},
			},
			// One-shot: failed job → failed execution, no retries.
			// MaxRetries is a oneof; wrap the int in the typed wrapper.
			Retries: &runpb.TaskTemplate_MaxRetries{MaxRetries: 0},
			// BUG-911: Cloud Run Job task_timeout default is 10 min;
			// runner-task running a real CI pipeline needs much more.
			// Match RUNNER_IDLE_SECONDS (3600s) so the bash timeout
			// and Cloud Run task timeout are aligned.
			Timeout: durationpb.New(3600 * time.Second),
		},
	}
	if req.ServiceAccount != "" {
		template.Template.ServiceAccount = req.ServiceAccount
	}

	// Cloud Run Jobs API requires Job.Name to be empty on CreateJob —
	// the name comes from CreateJobRequest.JobId. Setting it on the
	// nested struct returns 400 BadRequest "job.name must be empty".
	createOp, err := cli.CreateJob(ctx, &runpb.CreateJobRequest{
		Parent: parent,
		JobId:  jobID,
		Job: &runpb.Job{
			Labels: map[string]string{
				LabelManagedBy:  LabelManagedVal,
				LabelJobID:      fmt.Sprintf("%d", req.JobID),
				LabelRunnerName: sanitizeLabelValue(req.RunnerName),
			},
			Template: template,
		},
	})
	if err != nil {
		return "", fmt.Errorf("CreateJob %s: %w", fullName, err)
	}
	if _, err := createOp.Wait(ctx); err != nil {
		return "", fmt.Errorf("CreateJob %s wait: %w", fullName, err)
	}

	runOp, err := cli.RunJob(ctx, &runpb.RunJobRequest{Name: fullName})
	if err != nil {
		// Best-effort cleanup — leave the job orphaned rather than retry.
		// State recovery sweep will catch it.
		return fullName, fmt.Errorf("RunJob %s: %w", fullName, err)
	}
	if _, err := runOp.Wait(ctx); err != nil {
		return fullName, fmt.Errorf("RunJob %s wait: %w", fullName, err)
	}
	return fullName, nil
}

// jobIDFromRunnerName produces a GCP-Job-ID-safe identifier
// (lowercase alphanumerics + hyphens, max 49 chars) derived from the
// runner name + GitHub job ID. The runner name itself often contains
// uppercase / dots / underscores that GCP rejects, so we hash it and
// keep the first 8 hex chars + the GitHub job ID's last 6 digits as
// the uniqueness payload.
func jobIDFromRunnerName(runnerName string, githubJobID int64) string {
	h := sha256.Sum256([]byte(runnerName))
	return fmt.Sprintf("gh-%s-%d", hex.EncodeToString(h[:4]), githubJobID%1_000_000)
}

// sanitizeLabelValue ensures a string is GCP-label-value-safe
// (lowercase letters/digits/hyphens/underscores, 63 chars max).
func sanitizeLabelValue(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	out := b.String()
	if len(out) > 63 {
		out = out[:63]
	}
	return out
}

// Managed describes a Cloud Run Job the dispatcher previously created
// (matched via LabelManagedBy). Used by state recovery on startup and
// the cleanup sweep.
type Managed struct {
	JobName    string // full resource name `projects/.../jobs/<id>`
	JobID      int64  // GitHub workflow_job ID from labels
	RunnerName string
	State      string // "active", "deleted", … (Cloud Run Job state)
}

// ListManaged returns every Cloud Run Job under (project, region)
// that carries the dispatcher's managed-by label. Used on startup to
// rebuild the seen-set without on-disk state.
func ListManaged(ctx context.Context, project, region string) ([]Managed, error) {
	cli, err := run.NewJobsRESTClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("new jobs REST client: %w", err)
	}
	defer func() { _ = cli.Close() }()
	parent := fmt.Sprintf("projects/%s/locations/%s", project, region)
	it := cli.ListJobs(ctx, &runpb.ListJobsRequest{Parent: parent})
	var managed []Managed
	for {
		j, err := it.Next()
		if err != nil {
			break
		}
		if j.Labels[LabelManagedBy] != LabelManagedVal {
			continue
		}
		var jobID int64
		fmt.Sscanf(j.Labels[LabelJobID], "%d", &jobID)
		managed = append(managed, Managed{
			JobName:    j.Name,
			JobID:      jobID,
			RunnerName: j.Labels[LabelRunnerName],
			State:      stringifyJobState(j),
		})
	}
	return managed, nil
}

func stringifyJobState(j *runpb.Job) string {
	if j.TerminalCondition != nil {
		return j.TerminalCondition.State.String()
	}
	return "unknown"
}

// Delete removes a Cloud Run Job. Tolerates already-deleted (the
// underlying API returns NOT_FOUND which we treat as success).
func Delete(ctx context.Context, jobName string) error {
	cli, err := run.NewJobsRESTClient(ctx)
	if err != nil {
		return fmt.Errorf("new jobs REST client: %w", err)
	}
	defer func() { _ = cli.Close() }()
	op, err := cli.DeleteJob(ctx, &runpb.DeleteJobRequest{Name: jobName})
	if err != nil {
		if strings.Contains(err.Error(), "NotFound") || strings.Contains(err.Error(), "not found") {
			return nil
		}
		return fmt.Errorf("DeleteJob %s: %w", jobName, err)
	}
	if _, err := op.Wait(ctx); err != nil {
		if strings.Contains(err.Error(), "NotFound") || strings.Contains(err.Error(), "not found") {
			return nil
		}
		return fmt.Errorf("DeleteJob %s wait: %w", jobName, err)
	}
	return nil
}
