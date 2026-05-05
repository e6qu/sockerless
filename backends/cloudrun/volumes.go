package cloudrun

import (
	"context"
	"fmt"
	"strings"

	runpb "cloud.google.com/go/run/apiv2/runpb"
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

// buildVolumeForBind returns the runpb.Volume to attach for a bind
// reference plus, when applicable, a SOCKERLESS_PERSIST_VOLUMES entry
// (`name=mountPath=bucket`) so the bootstrap can tar-pack the tmpfs
// volume to GCS at every exec boundary.
//
// SharedVolume names (operator-pinned via SOCKERLESS_GCP_SHARED_VOLUMES)
// keep the raw GCSFuse mount — those buckets are written out-of-band
// and are not git-heavy. Ad-hoc volume names (gitlab-runner build dirs,
// docker volume create, ...) get an in-memory tmpfs (Volume_EmptyDir)
// because GCSFuse is ~200x slower than tmpfs for git operations
// (BUG-947). Persistence across Cloud Run revision instances is then
// achieved via the bootstrap's tar-pack module.
func (s *Server) buildVolumeForBind(ctx context.Context, volName, mountPath string) (*runpb.Volume, string, error) {
	bucket, err := s.bucketForVolume(ctx, volName)
	if err != nil {
		return nil, "", fmt.Errorf("provision GCS bucket for volume %q: %w", volName, err)
	}
	if s.config.LookupSharedVolumeByName(volName) != nil {
		return &runpb.Volume{
			Name: volName,
			VolumeType: &runpb.Volume_Gcs{
				Gcs: &runpb.GCSVolumeSource{
					Bucket:       bucket,
					MountOptions: gcpcommon.RunnerWorkspaceMountOptions(),
				},
			},
		}, "", nil
	}
	return &runpb.Volume{
		Name: volName,
		VolumeType: &runpb.Volume_EmptyDir{
			EmptyDir: &runpb.EmptyDirVolumeSource{
				Medium: runpb.EmptyDirVolumeSource_MEMORY,
			},
		},
	}, fmt.Sprintf("%s=%s=%s", volName, mountPath, bucket), nil
}

// injectPersistEnv appends SOCKERLESS_PERSIST_VOLUMES to the main
// container's env. specs[0] is always the main container (see
// start_service.go where IsMain is set on i==0).
//
// Sidecars deliberately skip persist (they SOCKERLESS_SIDECAR=1 and the
// bootstrap's restoreAll/saveAll only run in the ingress container) so
// the env var stays main-only.
func injectPersistEnv(specs []*runpb.Container, entries []string) {
	if len(entries) == 0 || len(specs) == 0 {
		return
	}
	specs[0].Env = append(specs[0].Env, &runpb.EnvVar{
		Name:   "SOCKERLESS_PERSIST_VOLUMES",
		Values: &runpb.EnvVar_Value{Value: strings.Join(entries, ",")},
	})
}
