package cloudrun

import (
	"context"

	"cloud.google.com/go/storage"
	gcpcommon "github.com/sockerless/gcp-common"
)

// GCS-backed named-volume + named-volume bind-mount provisioning for
// Cloud Run (Jobs today; Services when the v2 sim route ships).
//
// Docker volume semantics on Cloud Run map to GCS buckets: one bucket
// per named volume, labelled so VolumeList / VolumePrune can identify
// sockerless-owned buckets. Bind specs `volName:/mnt[:ro]` translate at
// task-launch time into a RevisionTemplate Volume with a `Gcs{Bucket}`
// source + a Container VolumeMount at `/mnt`.
//
// Host-path bind specs (`/h:/c`) stay rejected — Cloud Run containers
// have no host filesystem to bind from.
//
// Implementation lives in backends/gcp-common/volumes.go as
// gcpcommon.BucketManager so GCF can share it.

// gcsVolumeState embeds the shared BucketManager. Initialised by
// NewServer once the storage client is available.
type gcsVolumeState struct {
	buckets *gcpcommon.BucketManager
}

func (s *Server) bucketForVolume(ctx context.Context, volName string) (string, error) {
	// SharedVolumes (operator-configured via SOCKERLESS_GCP_SHARED_VOLUMES)
	// pin the GCS bucket directly — used by the runner-task / sub-task
	// shared workspace. Skip the BucketManager auto-provisioning path
	// for these so the runner-task and the sub-task land on the SAME
	// pre-created bucket the dispatcher mounted on the runner.
	if sv := s.config.LookupSharedVolumeByName(volName); sv != nil {
		return sv.Bucket, nil
	}
	return s.buckets.ForVolume(ctx, volName)
}

func (s *Server) deleteBucketForVolume(ctx context.Context, volName string, force bool) error {
	return s.buckets.DeleteForVolume(ctx, volName, force)
}

func (s *Server) listManagedBuckets(ctx context.Context) ([]*storage.BucketAttrs, error) {
	return s.buckets.ListManaged(ctx)
}
