package gcf

import gcpcommon "github.com/sockerless/gcp-common"

// Cloud Run Functions expose only secret volumes in their own API, but a
// function is a Cloud Run service underneath: `Function.ServiceConfig.Service`
// names it, and the documented escape hatch is to fetch that service, set
// its RevisionTemplate volumes and container mounts, and update it. Docker
// volume semantics therefore map to Cloud Storage buckets exactly as on the
// Cloud Run backend; the mapping is gcpcommon.RunVolumes, shared with it.
// Host-path binds are rejected — a function container has no host
// filesystem to bind from.

// gcsVolumeState carries the shared volume mapper. Initialised by NewServer
// once the storage client and the storage-backing registry exist.
type gcsVolumeState struct {
	volumes *gcpcommon.RunVolumes
}
