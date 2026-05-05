package gcf

import (
	"context"

	"cloud.google.com/go/storage"
	gcpcommon "github.com/sockerless/gcp-common"
)

// GCS-backed named-volume provisioning for Cloud Functions v2.
//
// Cloud Functions v2's public API (functionspb.ServiceConfig) exposes
// only SecretVolumes — no first-class GCS volume primitive. But v2 IS a
// Cloud Run Service underneath: `Function.ServiceConfig.Service` returns
// the resource name of the backing service. The documented escape hatch
// is to call Services.GetService on that name, modify the
// RevisionTemplate.Volumes + Container.VolumeMounts, and UpdateService.
//
// Volume CRUD reuses gcpcommon.BucketManager shared with the Cloud Run
// backend. ContainerStart (backend_impl.go) calls this file's helpers
// to provision buckets up-front and attach them to the underlying CR
// Service after Functions.CreateFunction returns.
//
// Host-path bind specs (`/h:/c`) stay rejected — Cloud Functions
// containers have no host filesystem to bind from.

// gcsVolumeState embeds the shared BucketManager. Initialised by
// NewServer once the storage client is available.
type gcsVolumeState struct {
	buckets *gcpcommon.BucketManager
}

func (s *Server) bucketForVolume(ctx context.Context, volName string) (string, error) {
	// SharedVolumes (operator-configured via SOCKERLESS_GCP_SHARED_VOLUMES)
	// pin the GCS bucket directly so the runner-task and sub-task land on
	// the same pre-created bucket the dispatcher mounted.
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

// attachVolumesToFunctionService parses a slice of Docker bind specs
// (`volName:/mnt[:ro]`), provisions a GCS bucket per unique named
// volume, fetches the underlying Cloud Run Service backing the
// function, and ensures the matching Volume + VolumeMount entries are
// present in its RevisionTemplate. Escape hatch because Functions v2's
// ServiceConfig has no first-class Volumes primitive.
//
// Idempotent: existing volumes/mounts with matching names are kept
// (not duplicated); missing ones are appended; non-matching entries
// stay untouched. Required because the pool-reuse path calls this on
// functions that may already carry volumes from prior allocations —
// before this change the pool path skipped attach entirely (leaving
// `volumes: null` on reused functions, which broke /__w bind mounts
// for the runner-task → spawned container script handoff).
//
// Skips the UpdateService call entirely when the existing service
// already has every requested volume + mount — saves a pointless
// revision rollout that would only re-confirm the current state.

// volumeForBind returns the runpb.Volume to attach for a bind. Shared
// volumes use raw GCSFuse (Volume_Gcs); ad-hoc volumes use in-memory
// tmpfs (Volume_EmptyDir{MEMORY}) with tar-pack persistence handled by
// the bootstrap's persist module. See BUG-947 for the rationale.

// setEnvVar adds or replaces a literal-value env var on a container
// (skipping EnvVar_ValueSource entries — those are secret refs and
// outside our concern). Returns true if the container's Env list was
// modified. An empty `value` removes the variable entirely.
