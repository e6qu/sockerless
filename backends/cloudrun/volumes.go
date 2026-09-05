package cloudrun

import gcpcommon "github.com/sockerless/gcp-common"

// Docker volume semantics on Cloud Run map to Cloud Storage buckets: one
// bucket per named volume, labelled so VolumeList / VolumePrune can
// identify sockerless-owned buckets. Bind specs `volName:/mnt[:ro]`
// translate at launch time into a RevisionTemplate volume plus a
// container VolumeMount at `/mnt`. Host-path binds are rejected — a Cloud
// Run container has no host filesystem to bind from. The mapping itself
// is gcpcommon.RunVolumes, shared with the Cloud Run Functions backend.

// gcsVolumeState carries the shared volume mapper. Initialised by NewServer
// once the storage client and the storage-backing registry exist.
type gcsVolumeState struct {
	volumes *gcpcommon.RunVolumes
}
