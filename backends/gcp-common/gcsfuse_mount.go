package gcpcommon

// RunnerWorkspaceMountOptions returns the GCS-Fuse mount options that
// make a Cloud Run / Cloud Functions GCSVolumeSource behave like a
// strongly-consistent shared filesystem between the runner-task and
// the spawned step containers — the AWS-side EFS-access-point analog.
//
// Default GCS-Fuse on Cloud Run uses:
//   - implicit-dirs OFF (only explicit "directory placeholder" objects
//     are listable; subdirectory creates are invisible)
//   - metadata-cache:ttl-secs=60 (positive cache: stat() of an existing
//     file uses cached value for up to 60s)
//   - metadata-cache:negative-ttl-secs=5 (negative cache: stat() of a
//     missing file caches the "doesn't exist" answer for 5s)
//
// For our runner-task → spawned-container script handoff, the runner-
// task writes a fresh file to /__w/_temp/_runner_file_commands/<id>
// and IMMEDIATELY tells the container `docker exec /bin/sh <id>`. With
// the 5s negative-cache, the container's gcsfuse instance still has
// "file does not exist" cached → docker exec returns exit 126 (command
// not invokable). Surfaced as BUG-944 (cell 6 SOLO 2026-05-04).
//
// We turn metadata caching OFF (ttl=0) and enable implicit-dirs so the
// behaviour matches EFS's strong consistency. Per
// https://cloud.google.com/run/docs/configuring/services/cloud-storage-volume-mounts
// MountOptions are passed without leading "--" to the gcsfuse CLI.
func RunnerWorkspaceMountOptions() []string {
	return []string{
		// GitHub Actions and gitlab-runner both create subdirectories
		// (workspaces, _temp, _actions). Without implicit-dirs the
		// container can't `cd` into them.
		"implicit-dirs",
		// Disable both positive and negative metadata caches so the
		// spawned container sees the runner-task's writes within one
		// stat() round-trip. AWS EFS gives us this for free; GCS-Fuse
		// requires explicit opt-out.
		"metadata-cache:ttl-secs=0",
		"metadata-cache:negative-ttl-secs=0",
	}
}
