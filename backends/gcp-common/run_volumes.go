package gcpcommon

import (
	"context"
	"fmt"
	"strings"
	"time"

	runpb "cloud.google.com/go/run/apiv2/runpb"
	"cloud.google.com/go/storage"
	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
)

// RunVolumes maps Docker named volumes onto Google Cloud Run revision
// volumes for the Cloud Run and Cloud Run Functions backends (a Cloud Run
// Functions function is a Cloud Run service underneath). A named volume is
// a Cloud Storage bucket the BucketManager provisions; an operator-declared
// shared volume pins its own bucket and its storage backing.
type RunVolumes struct {
	Buckets  *BucketManager
	Shared   core.SharedVolumes
	Backings *core.StorageBackingRegistry
	// Platform is the proper product name for error text: "Cloud Run" or
	// "Cloud Run Functions".
	Platform string
}

// BucketForVolume returns the bucket backing volName: the pinned bucket
// of a shared volume, otherwise the managed bucket, provisioned on first
// use. A shared volume's bucket is the one the runner workload already
// mounts, so caller and container land on the same data.
func (v *RunVolumes) BucketForVolume(ctx context.Context, volName string) (string, error) {
	if sv := v.Shared.ByName(volName); sv != nil {
		return sv.GCSBucket, nil
	}
	return v.Buckets.ForVolume(ctx, volName)
}

// VolumeForBind returns the revision volume for a `volName:mountPath` bind
// plus, for an ad-hoc volume, a SOCKERLESS_PERSIST_VOLUMES entry
// (`name=mountPath=bucket`) so the bootstrap tar-packs the volume to its
// bucket at every exec boundary.
//
// A shared volume goes through its declared storage backing. An ad-hoc
// volume (a CI runner's build directory, `docker volume create`) is an
// in-memory emptyDir with bootstrap-side persistence, because a FUSE
// mount is two orders of magnitude slower than tmpfs for git operations.
func (v *RunVolumes) VolumeForBind(ctx context.Context, volName, mountPath string) (*runpb.Volume, string, error) {
	bucket, err := v.BucketForVolume(ctx, volName)
	if err != nil {
		return nil, "", fmt.Errorf("provision GCS bucket for volume %q: %w", volName, err)
	}
	if shared := v.Shared.ByName(volName); shared != nil {
		vol := *shared
		if vol.GCSBucket == "" {
			vol.GCSBucket = bucket
		}
		runVol, err := v.VolumeFromBacking(vol)
		if err != nil {
			return nil, "", err
		}
		return runVol, "", nil
	}
	return &runpb.Volume{
		Name: volName,
		VolumeType: &runpb.Volume_EmptyDir{
			EmptyDir: &runpb.EmptyDirVolumeSource{Medium: runpb.EmptyDirVolumeSource_MEMORY},
		},
	}, fmt.Sprintf("%s=%s=%s", volName, mountPath, bucket), nil
}

// VolumeFromBacking converts a shared volume to its revision volume
// through the storage backing driver its declaration names. An empty or
// unknown backing fails at Resolve; there is no default.
func (v *RunVolumes) VolumeFromBacking(vol core.SharedVolumeRef) (*runpb.Volume, error) {
	if v.Backings == nil {
		return nil, fmt.Errorf("storage backing registry not initialized (volume %q)", vol.Name)
	}
	driver, err := v.Backings.Resolve(vol.Backing)
	if err != nil {
		return nil, fmt.Errorf("volume %q: %w", vol.Name, err)
	}
	spec, err := driver.CloudSpec(vol)
	if err != nil {
		return nil, fmt.Errorf("backing %q CloudSpec for volume %q: %w", driver.Backing(), vol.Name, err)
	}
	return RunVolumeFromBackingSpec(vol.Name, spec, v.Platform)
}

// RunVolumeFromBackingSpec translates a cloud-agnostic BackingSpec to the
// Cloud Run revision volume that implements it.
func RunVolumeFromBackingSpec(name string, spec core.BackingSpec, platform string) (*runpb.Volume, error) {
	switch spec.Kind {
	case core.BackingEmptyDir, core.BackingGCSSync:
		medium := runpb.EmptyDirVolumeSource_MEMORY
		if spec.EmptyDir != nil && spec.EmptyDir.Medium != "Memory" && spec.EmptyDir.Medium != "" {
			medium = runpb.EmptyDirVolumeSource_MEDIUM_UNSPECIFIED
		}
		return &runpb.Volume{
			Name:       name,
			VolumeType: &runpb.Volume_EmptyDir{EmptyDir: &runpb.EmptyDirVolumeSource{Medium: medium}},
		}, nil

	case core.BackingMemory:
		// A RAM-backed mount is an emptyDir with the memory medium; the
		// operator's explicit `memory` backing is the cross-cloud spelling.
		emptyDir := &runpb.EmptyDirVolumeSource{Medium: runpb.EmptyDirVolumeSource_MEMORY}
		if spec.Memory != nil && spec.Memory.SizeMB > 0 {
			emptyDir.SizeLimit = fmt.Sprintf("%dMi", spec.Memory.SizeMB)
		}
		return &runpb.Volume{Name: name, VolumeType: &runpb.Volume_EmptyDir{EmptyDir: emptyDir}}, nil

	case core.BackingGCSFuse:
		// Cloud Run wraps gcsfuse and rejects the metadata-cache TTL flags
		// that make a FUSE mount safe to share across tasks; without them
		// the default negative cache hides freshly written files from
		// sibling containers. gcs-sync has no FUSE and strong consistency.
		return nil, fmt.Errorf(
			"volume %q: backing %q is unsupported on %s — Cloud Run rejects the cache-TTL gcsfuse flags needed for cross-task safety. Use Backing: gcs-sync instead (per-exec tar sync, no FUSE)",
			name, spec.Kind, platform)

	case core.BackingPDEphemeral:
		// The Cloud Run volume union has no persistent-disk member.
		return nil, fmt.Errorf(
			"volume %q: backing %q not supported on %s — Cloud Run services lack a first-class persistent-disk volume primitive. Use Backing: gcs-sync for cross-task workspace sharing",
			name, spec.Kind, platform)
	}
	return nil, fmt.Errorf("volume %q: unsupported backing kind %q", name, spec.Kind)
}

// InjectPersistEnv appends SOCKERLESS_PERSIST_VOLUMES to the main
// container, which is always specs[0]. Sidecars deliberately carry no
// persistence: the bootstrap restores and saves only in the ingress
// container.
func InjectPersistEnv(specs []*runpb.Container, entries []string) {
	if len(entries) == 0 || len(specs) == 0 {
		return
	}
	specs[0].Env = append(specs[0].Env, &runpb.EnvVar{
		Name:   "SOCKERLESS_PERSIST_VOLUMES",
		Values: &runpb.EnvVar_Value{Value: strings.Join(entries, ",")},
	})
}

// BucketToVolume shapes a sockerless-managed bucket as the Docker volume
// it backs.
func BucketToVolume(name string, b *storage.BucketAttrs) *api.Volume {
	return &api.Volume{
		Name:       name,
		Driver:     "gcs",
		Mountpoint: "gs://" + b.Name,
		Scope:      "local",
		Options:    map[string]string{"bucket": b.Name},
		CreatedAt:  b.Created.UTC().Format(time.RFC3339Nano),
	}
}
