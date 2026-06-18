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
	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"google.golang.org/api/iterator"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
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

// OwnerRunnerTaskLabel is the GCP label key sockerless writes on every
// pod-Service it creates. The value is the Cloud Run Job ID from the
// Cloud-Run-injected `CLOUD_RUN_JOB` env var that sockerless reads
// inside the runner-task (no dispatcher-side env injection needed —
// keeps the dispatcher generic per the dispatcher-generic project
// rule). The dispatcher's orphan sweep reads this label off each
// listed Service and compares against ListManaged's known live Cloud
// Run Jobs.
const OwnerRunnerTaskLabel = "sockerless_owner_runner_task"

// Execution-derived job states. Same vocabulary as the Azure spawner
// so the two dispatch loops classify identically.
const (
	StateNoExecution        = "NO_EXECUTION"
	StateExecutionRunning   = "EXECUTION_RUNNING"
	StateExecutionSucceeded = "EXECUTION_SUCCEEDED"
	StateExecutionFailed    = "EXECUTION_FAILED"
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
	// RunnerWorkspaceBucket — when set, attach a Cloud Run native
	// `Volume{Gcs{Bucket}}` at /tmp/runner-work on the spawned
	// runner-task so step-script files written by the runner agent
	// propagate to the JOB container's pod-Service GCSFuse mount.
	// Empty = no mount (legacy behaviour).
	RunnerWorkspaceBucket string
	// RunnerWorkspaceBacking — operator's storage backing choice.
	// "gcs-fuse" (legacy) attaches the bucket directly to the
	// runner-task. "gcs-sync" leaves the runner-task tmpfs and lets
	// sockerless-backend tar/upload per-exec — required to avoid
	// GCSFuse stale-handle on per-step event.json rewrites. Validated
	// by config.Load when bucket is set.
	RunnerWorkspaceBacking string
	// JobTimeoutSeconds bounds the runner-task via the Cloud Run
	// Job's task timeout (the cloud-native field, per the
	// dispatcher-generic rule — never via env). Comes from the
	// config's `runner_job_timeout`.
	JobTimeoutSeconds int
}

// regTokenSecretID returns the Secret Manager secret ID holding the
// runner registration token for a given Cloud Run Job ID. Pairing the
// names lets Delete reap the secret with its Job.
func regTokenSecretID(jobID string) string {
	return jobID + "-regtoken"
}

// regTokenTTL bounds the registration-token secret's lifetime in
// Secret Manager. The token itself expires after 1h on GitHub's side;
// the TTL is GCP-native defense-in-depth for crash paths where Delete
// never ran.
const regTokenTTL = 2 * time.Hour

// createRegTokenSecret stores the registration token in Secret
// Manager (TTL-bounded) and returns the secret ID the Job's env
// references. The runner-task reads it via the Cloud Run secret env
// binding — the token never appears in the Job resource's plain env,
// which any `run.jobs.get` reader can see. The Job's service account
// needs `roles/secretmanager.secretAccessor` (see README IAM table).
func createRegTokenSecret(ctx context.Context, project, jobID, token string) (string, error) {
	cli, err := secretmanager.NewClient(ctx)
	if err != nil {
		return "", fmt.Errorf("new secretmanager client: %w", err)
	}
	defer func() { _ = cli.Close() }()
	secretID := regTokenSecretID(jobID)
	secret, err := cli.CreateSecret(ctx, &secretmanagerpb.CreateSecretRequest{
		Parent:   "projects/" + project,
		SecretId: secretID,
		Secret: &secretmanagerpb.Secret{
			Replication: &secretmanagerpb.Replication{
				Replication: &secretmanagerpb.Replication_Automatic_{Automatic: &secretmanagerpb.Replication_Automatic{}},
			},
			Expiration: &secretmanagerpb.Secret_ExpireTime{
				ExpireTime: timestamppb.New(time.Now().Add(regTokenTTL)),
			},
			Labels: map[string]string{LabelManagedBy: LabelManagedVal},
		},
	})
	if err != nil {
		return "", fmt.Errorf("CreateSecret %s: %w", secretID, err)
	}
	if _, err := cli.AddSecretVersion(ctx, &secretmanagerpb.AddSecretVersionRequest{
		Parent:  secret.Name,
		Payload: &secretmanagerpb.SecretPayload{Data: []byte(token)},
	}); err != nil {
		return "", fmt.Errorf("AddSecretVersion %s: %w", secretID, err)
	}
	return secretID, nil
}

// deleteRegTokenSecret removes the Job's companion token secret.
// Tolerates already-deleted (TTL expiry beats the sweep) — NOT_FOUND
// is success.
func deleteRegTokenSecret(ctx context.Context, project, jobID string) error {
	cli, err := secretmanager.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("new secretmanager client: %w", err)
	}
	defer func() { _ = cli.Close() }()
	name := fmt.Sprintf("projects/%s/secrets/%s", project, regTokenSecretID(jobID))
	if err := cli.DeleteSecret(ctx, &secretmanagerpb.DeleteSecretRequest{Name: name}); err != nil {
		if strings.Contains(err.Error(), "NotFound") || strings.Contains(err.Error(), "not found") {
			return nil
		}
		return fmt.Errorf("DeleteSecret %s: %w", name, err)
	}
	return nil
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
	if req.JobTimeoutSeconds <= 0 {
		return "", fmt.Errorf("job timeout required (config runner_job_timeout)")
	}
	cli, err := run.NewJobsRESTClient(ctx)
	if err != nil {
		return "", fmt.Errorf("new jobs REST client: %w", err)
	}
	defer func() { _ = cli.Close() }()

	parent := fmt.Sprintf("projects/%s/locations/%s", req.Project, req.Region)
	jobID := jobIDFromRunnerName(req.RunnerName, req.JobID)
	fullName := fmt.Sprintf("%s/jobs/%s", parent, jobID)

	// Registration token rides as a Secret Manager secret referenced by
	// the Job env, never as a plain env value (readable by any
	// run.jobs.get reader). Created before the Job so the reference is
	// valid at CreateJob validation time; reaped with the Job by
	// Delete, with the secret TTL as the crash-path backstop.
	tokenSecretID, err := createRegTokenSecret(ctx, req.Project, jobID, req.RegToken)
	if err != nil {
		return "", err
	}

	// Dispatcher scope (per CLOUD_RESOURCE_MAPPING.md § "Adjustment to
	// dispatcher scope"): only the runner-side env (GitHub PAT-derived
	// token, repo, name, labels). NO sockerless-shaped env or volumes
	// — the runner image owns its own backend config + workspace
	// mounting internally. The TaskTemplate.Timeout below stays
	// because it's a Cloud Run resource concern, not runner-internal.
	// Cloud Run Job default is 512Mi/1cpu — too small for a runner that
	// compiles real workloads. 4Gi/2cpu fits a Go compile + sidecar
	// postgres + the runner agent with headroom.
	containerCfg := &runpb.Container{
		Image: req.Image,
		Env: []*runpb.EnvVar{
			{Name: "RUNNER_REG_TOKEN", Values: &runpb.EnvVar_ValueSource{ValueSource: &runpb.EnvVarSource{
				SecretKeyRef: &runpb.SecretKeySelector{Secret: tokenSecretID, Version: "1"},
			}}},
			{Name: "RUNNER_REPO", Values: &runpb.EnvVar_Value{Value: req.Repo}},
			{Name: "RUNNER_NAME", Values: &runpb.EnvVar_Value{Value: req.RunnerName}},
			{Name: "RUNNER_LABELS", Values: &runpb.EnvVar_Value{Value: strings.Join(req.Labels, ",")}},
		},
		Resources: &runpb.ResourceRequirements{
			Limits: map[string]string{
				"cpu":    "2",
				"memory": "4Gi",
			},
		},
	}

	// Optional runner-workspace volume on the runner-task. Behaviour
	// depends on RunnerWorkspaceBacking:
	//
	//   - "gcs-fuse": mount the GCS bucket directly via Cloud Run
	//     native Volume{Gcs} so the runner agent's writes propagate to
	//     the bucket the JOB pod-Service reads. Used by sequential
	//     whole-tar upload patterns (FUSE-safe).
	//
	//   - "gcs-sync": leave the runner-task tmpfs at /tmp/runner-work —
	//     no GCS mount on the runner-task side. Sockerless-backend on
	//     the runner-task tars + uploads to GCS per-exec (PreExec hook);
	//     the JOB pod-Service bootstrap restores from GCS pre-subprocess.
	//     Avoids the FUSE stale-handle path entirely (no FUSE in the
	//     data plane).
	//
	// The empty-bucket case = no mount, no validation — legacy
	// pre-shared-workspace behaviour. config.Load enforces backing when
	// bucket is set per the no-fallbacks directive.
	var taskVolumes []*runpb.Volume
	if req.RunnerWorkspaceBucket != "" && req.RunnerWorkspaceBacking == "gcs-fuse" {
		const volName = "runner-workspace"
		taskVolumes = append(taskVolumes, &runpb.Volume{
			Name: volName,
			VolumeType: &runpb.Volume_Gcs{
				Gcs: &runpb.GCSVolumeSource{
					Bucket: req.RunnerWorkspaceBucket,
				},
			},
		})
		containerCfg.VolumeMounts = append(containerCfg.VolumeMounts, &runpb.VolumeMount{
			Name:      volName,
			MountPath: "/tmp/runner-work",
		})
	}

	template := &runpb.ExecutionTemplate{
		Template: &runpb.TaskTemplate{
			Containers: []*runpb.Container{containerCfg},
			Volumes:    taskVolumes,
			// One-shot: failed job → failed execution, no retries.
			Retries: &runpb.TaskTemplate_MaxRetries{MaxRetries: 0},
			// Cloud Run Job task_timeout default 10 min; the config's
			// `runner_job_timeout` (default 3600) bounds the
			// runner-task. This is a Cloud Run resource limit, not a
			// sockerless-shaped concern.
			Timeout: durationpb.New(time.Duration(req.JobTimeoutSeconds) * time.Second),
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

	// Do NOT runOp.Wait — the operation completes when the
	// EXECUTION ends (which can take an hour for a CI pipeline), and
	// blocking here serializes the dispatcher's poll loop. RunJob's
	// own return is the synchronous "execution accepted" ack we need;
	// state recovery picks up the job's terminal condition on the
	// next poll cycle.
	if _, err := cli.RunJob(ctx, &runpb.RunJobRequest{Name: fullName}); err != nil {
		// Best-effort: leave the job orphaned rather than retry.
		// State recovery sweep will catch it.
		return fullName, fmt.Errorf("RunJob %s: %w", fullName, err)
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
	State      string    // execution-derived: EXECUTION_* / NO_EXECUTION
	CreateTime time.Time // Job resource creation time (orphan-grace input)
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
		if err == iterator.Done {
			break
		}
		if err != nil {
			// A truncated list returned as complete would make Cleanup
			// treat live runner Jobs as orphaned and reap their Services —
			// fail loudly instead of silently dropping entries.
			return nil, fmt.Errorf("list jobs: %w", err)
		}
		if j.Labels[LabelManagedBy] != LabelManagedVal {
			continue
		}
		var jobID int64
		fmt.Sscanf(j.Labels[LabelJobID], "%d", &jobID)
		createTime := time.Time{}
		if j.CreateTime != nil {
			createTime = j.CreateTime.AsTime()
		}
		managed = append(managed, Managed{
			JobName:    j.Name,
			JobID:      jobID,
			RunnerName: j.Labels[LabelRunnerName],
			State:      executionStateForJob(ctx, j),
			CreateTime: createTime,
		})
	}
	return managed, nil
}

// executionStateForJob returns the state of the Job's most-recent
// Execution. A Cloud Run Job's `TerminalCondition` reflects the JOB
// DEFINITION's reconciliation state (Ready / NotReady), NOT the
// execution outcome. Cleanup keyed off `TerminalCondition.State` would
// delete every Job whose DEFINITION is Ready, including jobs whose
// Execution is still RUNNING. Doing so caused runner-tasks to be
// deleted shortly after spawn while the github runner was still
// bootstrapping. Real fix: query the latest Execution.
func executionStateForJob(ctx context.Context, j *runpb.Job) string {
	cli, err := run.NewExecutionsRESTClient(ctx)
	if err != nil {
		return "UNKNOWN"
	}
	defer func() { _ = cli.Close() }()
	it := cli.ListExecutions(ctx, &runpb.ListExecutionsRequest{Parent: j.Name})
	var latest *runpb.Execution
	for {
		ex, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			// A transient ListExecutions error must NOT read as "no
			// execution": that state is reap-eligible, so a network/throttle
			// blip during a long-running CI job would delete the live runner
			// Job. Return UNKNOWN (non-terminal, never reaped), matching the
			// Azure twin's "never delete on partial information."
			return "UNKNOWN"
		}
		if latest == nil || (ex.CreateTime != nil && latest.CreateTime != nil && ex.CreateTime.AsTime().After(latest.CreateTime.AsTime())) {
			latest = ex
		}
	}
	if latest == nil {
		return StateNoExecution
	}
	if latest.CompletionTime != nil {
		if latest.SucceededCount > 0 {
			return StateExecutionSucceeded
		}
		return StateExecutionFailed
	}
	return StateExecutionRunning
}

// ManagedService describes a sockerless-managed pod-Service the
// dispatcher's orphan sweep needs to consider for deletion. The
// dispatcher does not create these Services itself — sockerless on
// the runner-task does. The dispatcher's role is GC after the
// runner-task is gone.
type ManagedService struct {
	Name           string // full resource name `projects/.../services/sockerless-svc-<id12>`
	OwnerRunnerJob string // value of the `sockerless_owner_runner_task` label (Cloud Run Job ID, possibly empty for legacy)
}

// ListManagedServices returns every Cloud Run Service under (project,
// region) that is sockerless-managed AND whose name starts with
// `sockerless-svc-` (the prefix sockerless uses for pod-Services). The
// `sockerless_owner_runner_task` label is read off each match so the
// caller can decide whether to keep it or delete it as an orphan.
//
// Filters in code rather than a Cloud Run server-side filter because
// labels in the Cloud Run REST API can only be filtered on Job/Service
// state (not arbitrary user labels), so we do the prefix + label check
// client-side.
func ListManagedServices(ctx context.Context, project, region string) ([]ManagedService, error) {
	cli, err := run.NewServicesRESTClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("new services REST client: %w", err)
	}
	defer func() { _ = cli.Close() }()
	parent := fmt.Sprintf("projects/%s/locations/%s", project, region)
	it := cli.ListServices(ctx, &runpb.ListServicesRequest{Parent: parent})
	var out []ManagedService
	for {
		svc, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("list services: %w", err)
		}
		if svc.Labels["sockerless_managed"] != "true" {
			continue
		}
		if !strings.Contains(svc.Name, "/services/sockerless-svc-") {
			continue
		}
		out = append(out, ManagedService{
			Name:           svc.Name,
			OwnerRunnerJob: svc.Labels[OwnerRunnerTaskLabel],
		})
	}
	return out, nil
}

// DeleteService removes a Cloud Run Service. Tolerates already-deleted
// (NOT_FOUND treated as success) — the orphan sweep is best-effort.
func DeleteService(ctx context.Context, serviceName string) error {
	cli, err := run.NewServicesRESTClient(ctx)
	if err != nil {
		return fmt.Errorf("new services REST client: %w", err)
	}
	defer func() { _ = cli.Close() }()
	op, err := cli.DeleteService(ctx, &runpb.DeleteServiceRequest{Name: serviceName})
	if err != nil {
		if strings.Contains(err.Error(), "NotFound") || strings.Contains(err.Error(), "not found") {
			return nil
		}
		return fmt.Errorf("DeleteService %s: %w", serviceName, err)
	}
	if _, err := op.Wait(ctx); err != nil {
		if strings.Contains(err.Error(), "NotFound") || strings.Contains(err.Error(), "not found") {
			return nil
		}
		return fmt.Errorf("DeleteService %s wait: %w", serviceName, err)
	}
	return nil
}

// JobIDFromName extracts the trailing `<jobID>` segment from a Cloud
// Run Job's full resource name (`projects/.../jobs/<jobID>`). Used by
// the orphan-Service sweep to compare a Service's
// `sockerless_owner_runner_task` label (which holds a bare jobID
// produced by [jobIDFromRunnerName]) against the dispatcher's known
// list of running Cloud Run Jobs (whose ListManaged returns full
// resource names).
func JobIDFromName(jobResourceName string) string {
	if i := strings.LastIndex(jobResourceName, "/jobs/"); i >= 0 {
		return jobResourceName[i+len("/jobs/"):]
	}
	return jobResourceName
}

// Delete removes a Cloud Run Job and its companion registration-token
// secret. Tolerates already-deleted (the underlying APIs return
// NOT_FOUND which we treat as success).
func Delete(ctx context.Context, jobName string) error {
	cli, err := run.NewJobsRESTClient(ctx)
	if err != nil {
		return fmt.Errorf("new jobs REST client: %w", err)
	}
	defer func() { _ = cli.Close() }()
	op, err := cli.DeleteJob(ctx, &runpb.DeleteJobRequest{Name: jobName})
	if err != nil {
		if !strings.Contains(err.Error(), "NotFound") && !strings.Contains(err.Error(), "not found") {
			return fmt.Errorf("DeleteJob %s: %w", jobName, err)
		}
	} else if _, err := op.Wait(ctx); err != nil {
		if !strings.Contains(err.Error(), "NotFound") && !strings.Contains(err.Error(), "not found") {
			return fmt.Errorf("DeleteJob %s wait: %w", jobName, err)
		}
	}
	if project := projectFromName(jobName); project != "" {
		if err := deleteRegTokenSecret(ctx, project, JobIDFromName(jobName)); err != nil {
			return fmt.Errorf("job %s deleted but token secret remains (TTL will reap): %w", jobName, err)
		}
	}
	return nil
}

// projectFromName extracts the project segment from a full Cloud Run
// resource name (`projects/<p>/locations/...`).
func projectFromName(resourceName string) string {
	rest, ok := strings.CutPrefix(resourceName, "projects/")
	if !ok {
		return ""
	}
	if i := strings.Index(rest, "/"); i > 0 {
		return rest[:i]
	}
	return ""
}
