// Package gcpcommon — gcs-fuse storage backing driver (LEGACY).
//
// gcs-fuse exposes a GCS bucket as a Cloud Run native Volume{Gcs}
// FUSE mount. The driver itself is trivial — just emits a BackingSpec.GCS
// for the per-backend translator to wrap in runpb.Volume{Gcs{Bucket}}.
//
// FUSE-on-object-store has known semantic gaps: stale handles on
// per-step rewrites of the same object, and git-checkout
// incompatibility. This driver is retained ONLY for legacy tar-pack
// persist mounting (sequential whole-tar uploads — the FUSE-safe
// pattern). New SharedVolumes MUST use the gcs-sync driver
// (storage_gcssync.go) instead.
package gcpcommon

import (
	"context"

	core "github.com/sockerless/backend-core"
)

// GCSFuseDriver implements core.StorageBackingDriver for direct
// Volume{Gcs} FUSE mounts. No data-plane sync — FUSE handles reads/
// writes live (with the documented semantic caveats).
type GCSFuseDriver struct {
	// MountOptions is appended to every emitted GCS spec. Backends pass
	// in their per-cloud defaults; e.g. RunnerWorkspaceMountOptions().
	MountOptions []string
}

func (d *GCSFuseDriver) Backing() core.StorageBacking { return core.BackingGCSFuse }

func (d *GCSFuseDriver) CloudSpec(vol core.SharedVolumeRef) (core.BackingSpec, error) {
	if vol.GCSBucket == "" {
		return core.BackingSpec{}, errMissingBucket("gcs-fuse", vol.Name)
	}
	return core.BackingSpec{
		Kind: core.BackingGCSFuse,
		GCS: &core.GCSSpec{
			Bucket:       vol.GCSBucket,
			MountOptions: append([]string{}, d.MountOptions...),
		},
	}, nil
}

// PreExec / PostExec: no-op. FUSE handles the data plane live.

func (d *GCSFuseDriver) PreExec(ctx context.Context, vol core.SharedVolumeRef, execID, localPath, remotePath string) (map[string][]string, error) {
	return nil, nil
}

func (d *GCSFuseDriver) PostExec(ctx context.Context, vol core.SharedVolumeRef, execID, localPath string) error {
	return nil
}
