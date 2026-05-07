package gcpcommon

// RunnerWorkspaceMountOptions returns the GCS-Fuse mount options Cloud
// Run accepts for runner-workspace volumes.
//
// Cloud Run wraps gcsfuse and exposes a CONSTRAINED ALLOWLIST — the
// raw gcsfuse CLI flags (including `metadata-cache:ttl-secs=N`) are
// REJECTED by `Run.Jobs.CreateJob` and `Run.Services.UpdateService`
// with `Unsupported or unrecognized flag for Cloud Storage volume`.
// The accepted set per https://cloud.google.com/run/docs/configuring/services/cloud-storage-volume-mounts
// is: `implicit-dirs`, `o=ro` / `o=rw`, `file-mode=NNN`, `dir-mode=NNN`,
// `uid=N`, `gid=N`. Anything outside that allowlist breaks the deploy.
//
// `implicit-dirs` is required so subdirectory listings work (github-
// runner + gitlab-runner both rely on standard FS subdir traversal).
// Cross-execution gcsfuse cache TTL is left at Cloud Run's default
// since we can't tune it via the Run API.
func RunnerWorkspaceMountOptions() []string {
	return []string{"implicit-dirs"}
}
