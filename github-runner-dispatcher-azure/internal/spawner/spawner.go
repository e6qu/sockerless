// Package spawner runs a GitHub Actions runner image as an Azure
// Container Apps Job execution using `armappcontainers`. One ACA Job
// (provisioning the template once per runner_name) and one execution
// per queued workflow_job. The runner image's entrypoint registers
// with GitHub using `RUNNER_REG_TOKEN`, runs the job, and exits —
// ACA terminates the execution on subprocess exit.
//
// Mirror of `github-runner-dispatcher-gcp/internal/spawner` adapted
// to ACA's two-step shape (Job is the template; JobExecution is the
// running instance). State recovery uses the Job's resource tags.
//
// State strings use the same vocabulary as the GCP spawner
// (EXECUTION_SUCCEEDED / EXECUTION_FAILED / EXECUTION_RUNNING /
// NO_EXECUTION) and come from the Job's latest EXECUTION, not the ARM
// resource's `ProvisioningState`. ProvisioningState tracks the
// resource template's provisioning — it reads `Succeeded` the moment
// BeginCreateOrUpdate completes, while the execution is still
// running; keying cleanup off it deletes runner Jobs mid-CI-job (the
// ACA mirror of the Cloud Run TerminalCondition pitfall the GCP
// spawner documents on its executionStateForJob).
package spawner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appcontainers/armappcontainers/v3"
)

// Tags stamped on every ACA Job so a restarted dispatcher can
// rediscover its state via ListByResourceGroup. Azure tag keys are
// case-insensitive but ARM round-trips them as-supplied; using
// lowercase keys keeps the wire shape consistent across Azure SDK
// versions.
const (
	TagJobID      = "sockerless-dispatcher-job-id"
	TagRunnerName = "sockerless-dispatcher-runner-name"
	TagManagedBy  = "sockerless-dispatcher-managed-by"
	TagManagedVal = "github-runner-dispatcher-azure"
)

// Execution-derived job states. Same vocabulary as the GCP spawner so
// the two dispatch loops classify identically.
const (
	StateNoExecution        = "NO_EXECUTION"
	StateExecutionRunning   = "EXECUTION_RUNNING"
	StateExecutionSucceeded = "EXECUTION_SUCCEEDED"
	StateExecutionFailed    = "EXECUTION_FAILED"
)

// regTokenSecretName is the ACA Job secret holding the GitHub runner
// registration token. The token rides as a Job-level secret with a
// `secretRef` env binding instead of a plain env value — plain env is
// readable by any ARM reader on the resource, while secret values are
// write-only on GET (reading them back needs the separately-RBAC'd
// listSecrets action).
const regTokenSecretName = "runner-reg-token"

// Request is one spawn directive.
type Request struct {
	SubscriptionID string // Azure subscription
	ResourceGroup  string // resource group hosting the Container Apps Environment
	Environment    string // Container Apps Environment resource ID (full ARM ID)
	Location       string // Azure region (e.g. "eastus2")
	Image          string // runner image URI (Azure Container Registry)
	RegToken       string // GitHub ephemeral runner registration token
	Repo           string // owner/repo for runner registration
	RunnerName     string // unique name; logs / Actions UI uses it
	Labels         []string
	JobID          int64 // GitHub workflow_job ID
	// ManagedIdentity is the resource ID of a user-assigned managed
	// identity the Job execution runs as. Required for the Job to
	// pull from a private ACR or write to other ARM resources.
	ManagedIdentity string
	// JobTimeoutSeconds bounds the runner-task itself via the ACA
	// Job's ReplicaTimeout (the cloud-native field, per the
	// dispatcher-generic rule — never via env). Comes from the
	// config's `runner_job_timeout`.
	JobTimeoutSeconds int32
}

// Spawn creates an ACA Job (idempotent on the deterministic name) and
// starts an execution. Returns the Job's full ARM ID for state
// recovery + cleanup.
func Spawn(ctx context.Context, req Request) (string, error) {
	if req.SubscriptionID == "" {
		return "", fmt.Errorf("subscription_id required")
	}
	if req.ResourceGroup == "" {
		return "", fmt.Errorf("resource_group required")
	}
	if req.Environment == "" {
		return "", fmt.Errorf("environment required")
	}
	if req.Location == "" {
		return "", fmt.Errorf("location required")
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

	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return "", fmt.Errorf("azure credential: %w", err)
	}
	jobsClient, err := armappcontainers.NewJobsClient(req.SubscriptionID, cred, nil)
	if err != nil {
		return "", fmt.Errorf("jobs client: %w", err)
	}

	jobName := jobNameFromRunnerName(req.RunnerName, req.JobID)

	job := armappcontainers.Job{
		Location: to.Ptr(req.Location),
		Tags: map[string]*string{
			TagManagedBy:  to.Ptr(TagManagedVal),
			TagJobID:      to.Ptr(fmt.Sprintf("%d", req.JobID)),
			TagRunnerName: to.Ptr(sanitizeTagValue(req.RunnerName)),
		},
		Properties: &armappcontainers.JobProperties{
			EnvironmentID: to.Ptr(req.Environment),
			Configuration: &armappcontainers.JobConfiguration{
				TriggerType:         to.Ptr(armappcontainers.TriggerTypeManual),
				ReplicaTimeout:      to.Ptr(req.JobTimeoutSeconds),
				ReplicaRetryLimit:   to.Ptr[int32](0), // one shot
				ManualTriggerConfig: &armappcontainers.JobConfigurationManualTriggerConfig{Parallelism: to.Ptr[int32](1), ReplicaCompletionCount: to.Ptr[int32](1)},
				Secrets: []*armappcontainers.Secret{
					{Name: to.Ptr(regTokenSecretName), Value: to.Ptr(req.RegToken)},
				},
			},
			Template: &armappcontainers.JobTemplate{
				Containers: []*armappcontainers.Container{{
					Name:  to.Ptr("runner"),
					Image: to.Ptr(req.Image),
					Env: []*armappcontainers.EnvironmentVar{
						{Name: to.Ptr("RUNNER_REG_TOKEN"), SecretRef: to.Ptr(regTokenSecretName)},
						{Name: to.Ptr("RUNNER_REPO"), Value: to.Ptr(req.Repo)},
						{Name: to.Ptr("RUNNER_NAME"), Value: to.Ptr(req.RunnerName)},
						{Name: to.Ptr("RUNNER_LABELS"), Value: to.Ptr(strings.Join(req.Labels, ","))},
					},
					// ACA default (0.5 vCPU / 1 Gi) is too small for a
					// runner that compiles real workloads — same
					// reasoning as the GCP spawner's 2 cpu / 4 Gi. The
					// 2.0/4Gi pair is a valid Consumption-plan combo.
					Resources: &armappcontainers.ContainerResources{
						CPU:    to.Ptr(2.0),
						Memory: to.Ptr("4Gi"),
					},
				}},
			},
		},
	}

	if req.ManagedIdentity != "" {
		job.Identity = &armappcontainers.ManagedServiceIdentity{
			Type:                   to.Ptr(armappcontainers.ManagedServiceIdentityTypeUserAssigned),
			UserAssignedIdentities: map[string]*armappcontainers.UserAssignedIdentity{req.ManagedIdentity: {}},
		}
	}

	createPoller, err := jobsClient.BeginCreateOrUpdate(ctx, req.ResourceGroup, jobName, job, nil)
	if err != nil {
		return "", fmt.Errorf("BeginCreateOrUpdate %s: %w", jobName, err)
	}
	created, err := createPoller.PollUntilDone(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("CreateOrUpdate %s wait: %w", jobName, err)
	}
	jobARMID := ""
	if created.ID != nil {
		jobARMID = *created.ID
	}

	startPoller, err := jobsClient.BeginStart(ctx, req.ResourceGroup, jobName, nil)
	if err != nil {
		return jobARMID, fmt.Errorf("BeginStart %s: %w", jobName, err)
	}
	if _, err := startPoller.PollUntilDone(ctx, nil); err != nil {
		return jobARMID, fmt.Errorf("start %s wait: %w", jobName, err)
	}
	return jobARMID, nil
}

// jobNameFromRunnerName produces an ACA-Job-name-safe identifier
// (lowercase alphanumerics + hyphens, max 32 chars). Same shape as
// the GCP-side `jobIDFromRunnerName`.
func jobNameFromRunnerName(runnerName string, githubJobID int64) string {
	h := sha256.Sum256([]byte(runnerName))
	return fmt.Sprintf("gh-%s-%d", hex.EncodeToString(h[:4]), githubJobID%1_000_000)
}

// sanitizeTagValue trims an Azure-tag-value-safe string. ARM allows
// most printable characters but caps tag values at 256 chars; we trim
// to 128 to leave headroom for systems that cap lower.
func sanitizeTagValue(s string) string {
	if len(s) > 128 {
		return s[:128]
	}
	return s
}

// Managed describes an ACA Job the dispatcher previously created.
type Managed struct {
	JobARMID   string
	JobName    string
	JobID      int64
	RunnerName string
	State      string // execution-derived: EXECUTION_* / NO_EXECUTION
	CreatedAt  time.Time
}

// ListManaged returns every ACA Job under (subscription, resource
// group) carrying the dispatcher's managed-by tag. State comes from
// the Job's latest execution (see package doc on why ProvisioningState
// is the wrong dimension).
func ListManaged(ctx context.Context, subscriptionID, resourceGroup string) ([]Managed, error) {
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("azure credential: %w", err)
	}
	jobsClient, err := armappcontainers.NewJobsClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("jobs client: %w", err)
	}
	execClient, err := armappcontainers.NewJobsExecutionsClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("jobs executions client: %w", err)
	}
	pager := jobsClient.NewListByResourceGroupPager(resourceGroup, nil)
	var managed []Managed
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return managed, fmt.Errorf("list page: %w", err)
		}
		for _, j := range page.Value {
			if j.Tags[TagManagedBy] == nil || *j.Tags[TagManagedBy] != TagManagedVal {
				continue
			}
			var jobID int64
			if j.Tags[TagJobID] != nil {
				fmt.Sscanf(*j.Tags[TagJobID], "%d", &jobID)
			}
			runnerName := ""
			if j.Tags[TagRunnerName] != nil {
				runnerName = *j.Tags[TagRunnerName]
			}
			createdAt := time.Time{}
			if j.SystemData != nil && j.SystemData.CreatedAt != nil {
				createdAt = *j.SystemData.CreatedAt
			}
			managed = append(managed, Managed{
				JobARMID:   ptrStr(j.ID),
				JobName:    ptrStr(j.Name),
				JobID:      jobID,
				RunnerName: runnerName,
				State:      executionStateForJob(ctx, execClient, resourceGroup, ptrStr(j.Name)),
				CreatedAt:  createdAt,
			})
		}
	}
	return managed, nil
}

// executionStateForJob returns the state of the Job's most-recent
// execution. On an API error it returns UNKNOWN, which no caller
// treats as terminal — never delete on partial information.
func executionStateForJob(ctx context.Context, execClient *armappcontainers.JobsExecutionsClient, resourceGroup, jobName string) string {
	if jobName == "" {
		return "UNKNOWN"
	}
	pager := execClient.NewListPager(resourceGroup, jobName, nil)
	var execs []*armappcontainers.JobExecution
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return "UNKNOWN"
		}
		execs = append(execs, page.Value...)
	}
	return ClassifyExecutions(execs)
}

// ClassifyExecutions maps a Job's execution list to the dispatcher's
// state vocabulary, keyed off the LATEST execution (by StartTime).
// Exported for the dispatch loop's tests — this classification is
// exactly what decides whether cleanup may delete a runner Job, and
// getting it wrong kills in-flight CI jobs.
func ClassifyExecutions(execs []*armappcontainers.JobExecution) string {
	var latest *armappcontainers.JobExecution
	for _, ex := range execs {
		if ex == nil || ex.Properties == nil {
			continue
		}
		if latest == nil {
			latest = ex
			continue
		}
		lt := latest.Properties.StartTime
		et := ex.Properties.StartTime
		if lt == nil || (et != nil && et.After(*lt)) {
			latest = ex
		}
	}
	if latest == nil {
		return StateNoExecution
	}
	if latest.Properties.Status == nil {
		// Status not yet populated — treat as running; the next sweep
		// re-checks.
		return StateExecutionRunning
	}
	switch *latest.Properties.Status {
	case armappcontainers.JobExecutionRunningStateSucceeded:
		return StateExecutionSucceeded
	case armappcontainers.JobExecutionRunningStateFailed,
		armappcontainers.JobExecutionRunningStateStopped,
		armappcontainers.JobExecutionRunningStateDegraded:
		return StateExecutionFailed
	default:
		// Running / Processing / Unknown — not terminal.
		return StateExecutionRunning
	}
}

// Delete removes an ACA Job. Tolerates NOT_FOUND.
func Delete(ctx context.Context, subscriptionID, resourceGroup, jobName string) error {
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return fmt.Errorf("azure credential: %w", err)
	}
	jobsClient, err := armappcontainers.NewJobsClient(subscriptionID, cred, nil)
	if err != nil {
		return fmt.Errorf("jobs client: %w", err)
	}
	poller, err := jobsClient.BeginDelete(ctx, resourceGroup, jobName, nil)
	if err != nil {
		if strings.Contains(err.Error(), "NotFound") || strings.Contains(err.Error(), "not found") {
			return nil
		}
		return fmt.Errorf("BeginDelete %s: %w", jobName, err)
	}
	if _, err := poller.PollUntilDone(ctx, nil); err != nil {
		if strings.Contains(err.Error(), "NotFound") || strings.Contains(err.Error(), "not found") {
			return nil
		}
		return fmt.Errorf("Delete %s wait: %w", jobName, err)
	}
	return nil
}

func ptrStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
