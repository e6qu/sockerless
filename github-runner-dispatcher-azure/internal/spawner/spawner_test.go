package spawner

import (
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appcontainers/armappcontainers/v3"
)

func TestJobNameFromRunnerName(t *testing.T) {
	name := jobNameFromRunnerName("dispatcher-azure-987654321-1717000000", 987654321)
	if !strings.HasPrefix(name, "gh-") {
		t.Fatalf("job name %q missing gh- prefix", name)
	}
	if len(name) > 32 {
		t.Fatalf("job name %q exceeds ACA's 32-char cap", name)
	}
	for _, r := range name {
		valid := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-'
		if !valid {
			t.Fatalf("job name %q contains ACA-invalid rune %q", name, r)
		}
	}
	// Deterministic for state recovery.
	if again := jobNameFromRunnerName("dispatcher-azure-987654321-1717000000", 987654321); again != name {
		t.Fatalf("job name not deterministic: %q vs %q", name, again)
	}
}

func TestSanitizeTagValue(t *testing.T) {
	if got := sanitizeTagValue("short"); got != "short" {
		t.Fatalf("sanitizeTagValue(short) = %q", got)
	}
	long := strings.Repeat("x", 300)
	if got := sanitizeTagValue(long); len(got) != 128 {
		t.Fatalf("sanitizeTagValue long = %d chars, want 128", len(got))
	}
}

func exec(status armappcontainers.JobExecutionRunningState, start time.Time) *armappcontainers.JobExecution {
	return &armappcontainers.JobExecution{
		Properties: &armappcontainers.JobExecutionProperties{
			Status:    to.Ptr(status),
			StartTime: to.Ptr(start),
		},
	}
}

func TestClassifyExecutions(t *testing.T) {
	t0 := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
	t1 := t0.Add(10 * time.Minute)

	cases := []struct {
		name  string
		execs []*armappcontainers.JobExecution
		want  string
	}{
		{"no executions", nil, StateNoExecution},
		{"nil entries only", []*armappcontainers.JobExecution{nil, {}}, StateNoExecution},
		{"running", []*armappcontainers.JobExecution{exec(armappcontainers.JobExecutionRunningStateRunning, t0)}, StateExecutionRunning},
		{"processing", []*armappcontainers.JobExecution{exec(armappcontainers.JobExecutionRunningStateProcessing, t0)}, StateExecutionRunning},
		{"unknown status", []*armappcontainers.JobExecution{exec(armappcontainers.JobExecutionRunningStateUnknown, t0)}, StateExecutionRunning},
		{"succeeded", []*armappcontainers.JobExecution{exec(armappcontainers.JobExecutionRunningStateSucceeded, t0)}, StateExecutionSucceeded},
		{"failed", []*armappcontainers.JobExecution{exec(armappcontainers.JobExecutionRunningStateFailed, t0)}, StateExecutionFailed},
		{"stopped", []*armappcontainers.JobExecution{exec(armappcontainers.JobExecutionRunningStateStopped, t0)}, StateExecutionFailed},
		{"degraded", []*armappcontainers.JobExecution{exec(armappcontainers.JobExecutionRunningStateDegraded, t0)}, StateExecutionFailed},
		{
			// The latest execution decides — an old success must not
			// make a Job with a live re-run reapable.
			"latest wins",
			[]*armappcontainers.JobExecution{
				exec(armappcontainers.JobExecutionRunningStateSucceeded, t0),
				exec(armappcontainers.JobExecutionRunningStateRunning, t1),
			},
			StateExecutionRunning,
		},
		{
			"status not yet populated",
			[]*armappcontainers.JobExecution{{
				Properties: &armappcontainers.JobExecutionProperties{StartTime: to.Ptr(t0)},
			}},
			StateExecutionRunning,
		},
	}
	for _, tc := range cases {
		if got := ClassifyExecutions(tc.execs); got != tc.want {
			t.Errorf("%s: ClassifyExecutions = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestSpawnValidation pins the fail-loudly contract: every required
// field error fires before any Azure client is constructed.
func TestSpawnValidation(t *testing.T) {
	base := Request{
		SubscriptionID:    "sub",
		ResourceGroup:     "rg",
		Environment:       "/subscriptions/s/rg/r/env",
		Location:          "eastus2",
		Image:             "acr.azurecr.io/runner:latest",
		RegToken:          "tok",
		Repo:              "owner/repo",
		RunnerName:        "dispatcher-azure-1-1",
		JobTimeoutSeconds: 3600,
	}
	cases := []struct {
		name   string
		mutate func(*Request)
		want   string
	}{
		{"subscription", func(r *Request) { r.SubscriptionID = "" }, "subscription_id required"},
		{"resource group", func(r *Request) { r.ResourceGroup = "" }, "resource_group required"},
		{"environment", func(r *Request) { r.Environment = "" }, "environment required"},
		{"location", func(r *Request) { r.Location = "" }, "location required"},
		{"image", func(r *Request) { r.Image = "" }, "image required"},
		{"token", func(r *Request) { r.RegToken = "" }, "registration token required"},
		{"repo", func(r *Request) { r.Repo = "" }, "repo required"},
		{"runner name", func(r *Request) { r.RunnerName = "" }, "runner name required"},
		{"job timeout", func(r *Request) { r.JobTimeoutSeconds = 0 }, "job timeout required"},
	}
	for _, tc := range cases {
		req := base
		tc.mutate(&req)
		_, err := Spawn(t.Context(), req)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: Spawn err = %v, want containing %q", tc.name, err, tc.want)
		}
	}
}
