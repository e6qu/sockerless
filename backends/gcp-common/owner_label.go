package gcpcommon

import (
	"os"
	"strings"
)

// OwnerRunnerTaskEnv is the Cloud-Run-injected env var sockerless reads
// to discover the Cloud Run Job it is currently running inside. Cloud
// Run sets this automatically on every Job execution; sockerless reads
// it (no dispatcher-side cooperation needed) and stamps every pod-
// Service with [OwnerRunnerTaskLabel]. The dispatcher's orphan-Service
// sweep then deletes any Service whose owner Cloud Run Job is gone or
// terminal.
//
// Reading a Cloud-Run-injected env var instead of a sockerless-shaped
// env var keeps `github-runner-dispatcher-gcp` fully generic per the
// dispatcher-generic project rule: the dispatcher submits a vanilla
// runner image and never injects any `SOCKERLESS_*` config. Owner
// discovery is sockerless-side, end-to-end.
//
// When sockerless runs outside a Cloud Run Job (laptop / sim / regular
// Cloud Run Service), the env var is unset and no owner label is
// written — the dispatcher's sweep then leaves those Services alone
// (it only acts on Services that carry an owner label).
const OwnerRunnerTaskEnv = "CLOUD_RUN_JOB"

// OwnerRunnerTaskLabel is the GCP label key the dispatcher uses to
// associate orphan `sockerless-svc-*` Services with their owning
// runner-task at GC time. Underscore form (GCP labels charset).
const OwnerRunnerTaskLabel = "sockerless_owner_runner_task"

// OwnerRunnerTaskLabelValue returns the sanitized owner identifier from
// the [OwnerRunnerTaskEnv] env var, or "" if unset. Sanitization strips
// to the GCP label-value charset ([a-z0-9_-], 63 chars max). Returning
// "" means "no owner label should be stamped on this resource" — the
// caller (servicespec.go / pod_service.go) skips the label entirely.
func OwnerRunnerTaskLabelValue() string {
	return sanitizeOwnerLabel(os.Getenv(OwnerRunnerTaskEnv))
}

func sanitizeOwnerLabel(s string) string {
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
