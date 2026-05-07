// Package core — storage backing driver abstraction (Phase 123).
//
// Replaces the vestigial backends/core/storage_driver.go::StorageDriver +
// api/drivers.go::VolumeDriver shells, neither of which had a real
// implementation. The abstraction here is layered:
//
//  1. BACKING SELECTION (this file): operator declares Backing on a volume
//     ("emptyDir" / "gcs-sync" / "gcs-fuse"); the backend resolves a
//     StorageBackingDriver from the Registry.
//
//  2. CLOUD VOLUME SPEC (per-backend translator): driver.CloudSpec returns
//     a cloud-agnostic BackingSpec; the per-backend translator (e.g.
//     backends/cloudrun-functions/volume_translator.go) maps that to the
//     cloud's actual volume protobuf (runpb.Volume, ECS Volume, etc.).
//
//  3. DATA-PLANE SYNC (driver.PreExec / driver.PostExec): sync-style
//     drivers (gcs-sync) tar+upload before exec and download+untar after.
//     Live-filesystem drivers (gcs-fuse, future ephemeral-managed-FS)
//     return nil hooks. emptyDir is single-instance so also returns nil
//     hooks.
//
// User directives 2026-05-07 baked in:
//   - Storage MUST be pluggable so we can test multiple options without
//     re-refactoring each backend.
//   - Zero-scaling, no-cost-when-not-in-use is the paradigm. Acceptable:
//     object storage, in-memory, ephemeral managed FS where sockerless
//     owns the lifecycle. Rejected: NFS / Filestore / Memorystore /
//     persistent-mode PDs (all bill idle).
//   - No FUSE-on-object-store for new SharedVolumes (gcs-fuse retained
//     ONLY for cells 7+8 legacy tar-pack persist).
//   - **No automatic fallbacks** (user directive 2026-05-07): every
//     SharedVolume MUST have an explicitly-set Backing. Resolve()
//     returns an error for empty or unknown backings rather than
//     silently selecting a default. Rationale: each backing has
//     different cost / scale / consistency characteristics; the
//     operator's choice is load-bearing, and silent fallback masks
//     misconfiguration that surfaces as confusing runtime failures
//     (e.g. cells 7+8 expect gcs-fuse for tar-pack persist; cells
//     5+6 expect gcs-sync for per-step propagation; emptyDir would
//     "work" for both up to the first cross-Service read, then break).
//
// See specs/CLOUD_RESOURCE_MAPPING.md § "Storage backing driver
// abstraction (PLANNED — Phase 123)" for the full architectural design.
package core

import (
	"context"
	"fmt"
)

// StorageBacking is the operator-selected workspace storage strategy.
// Each value selects a different driver implementation; SharedVolume.Backing
// chooses the strategy.
type StorageBacking string

const (
	// BackingEmptyDir — local tmpfs in-memory volume. No persistence.
	// Default fallback for non-shared volumes.
	BackingEmptyDir StorageBacking = "emptyDir"

	// BackingGCSSync (DEFAULT for shared workspaces) — emptyDir tmpfs +
	// per-exec tar/untar against a GCS object. No FUSE in the data path.
	// Scale-to-zero. Only candidate satisfying the no-idle-cost directive
	// for cross-Service shared state.
	BackingGCSSync StorageBacking = "gcs-sync"

	// BackingGCSFuse (LEGACY) — direct Cloud Run native Volume{Gcs}
	// FUSE mount. Retained only for cells 7+8's tar-pack persist pattern
	// (sequential whole-tar uploads — FUSE-safe). New SharedVolumes MUST
	// use BackingGCSSync.
	BackingGCSFuse StorageBacking = "gcs-fuse"
)

// SharedVolumeRef is the cloud-agnostic volume reference passed to drivers.
// Each backend's per-cloud SharedVolume struct (e.g. gcf.SharedVolume,
// cloudrun.SharedVolume) converts to this for driver dispatch.
type SharedVolumeRef struct {
	Name          string         // logical volume name
	ContainerPath string         // mount path inside the consumer container
	Backing       StorageBacking // resolves a driver in the Registry
	GCSBucket     string         // for gcs-sync + gcs-fuse drivers
}

// StorageBackingDriver is the pluggable interface. Each driver knows how
// to produce a cloud-agnostic BackingSpec and (optionally) sync data
// before/after an exec. The per-backend volume translator converts the
// BackingSpec to the cloud's actual volume protobuf.
type StorageBackingDriver interface {
	// Backing returns the StorageBacking value this driver implements.
	// Used by the Registry for lookup.
	Backing() StorageBacking

	// CloudSpec returns the cloud-agnostic mount spec the per-backend
	// translator converts. Side-effect free.
	CloudSpec(vol SharedVolumeRef) (BackingSpec, error)

	// PreExec runs before forwarding an exec POST to the bootstrap.
	// Returns environment hints (e.g. SOCKERLESS_WORKSPACE_OBJECT) the
	// backend merges into the envelope's Env field; the bootstrap reads
	// them to drive the matching restore/save logic. Live filesystems
	// (gcs-fuse, emptyDir) return (nil, nil).
	PreExec(ctx context.Context, vol SharedVolumeRef, execID, localPath string) (envHints map[string]string, err error)

	// PostExec runs after the exec response returns to the backend.
	// For sync drivers: pulls the bootstrap's modified state back to
	// localPath. Live filesystems return nil.
	PostExec(ctx context.Context, vol SharedVolumeRef, execID, localPath string) error
}

// BackingSpec is the cloud-agnostic mount description. Per-backend
// volume translators consume this and emit the cloud's actual volume
// protobuf (runpb.Volume for Cloud Run, ECS Volume{} for AWS, etc.).
// Exactly one of EmptyDir / GCS / Ephemeral is set.
type BackingSpec struct {
	Kind     StorageBacking
	EmptyDir *EmptyDirSpec // emptyDir + gcs-sync (which uses emptyDir as the local mount)
	GCS      *GCSSpec      // gcs-fuse (direct FUSE mount)
}

// EmptyDirSpec describes an in-memory tmpfs volume.
type EmptyDirSpec struct {
	// Medium is a cloud-agnostic hint: "" = cloud default; "Memory" =
	// tmpfs-style in-memory. Translators map to runpb.EmptyDirVolumeSource_MEMORY etc.
	Medium string
}

// GCSSpec describes a direct GCSFuse mount (legacy).
type GCSSpec struct {
	Bucket       string
	MountOptions []string // e.g. ["implicit-dirs", "metadata-cache:ttl-secs=0"]
	ReadOnly     bool
}

// StorageBackingRegistry resolves a StorageBacking to its driver. Backends
// build the registry at server startup (NewStorageBackingRegistry +
// Register per driver) and look up by SharedVolume.Backing at materialize
// + exec time.
type StorageBackingRegistry struct {
	drivers map[StorageBacking]StorageBackingDriver
}

// NewStorageBackingRegistry returns a registry pre-populated with the
// EmptyDirDriver (cloud-agnostic; always available). Backend wiring adds
// cloud-specific drivers (gcs-sync, gcs-fuse, etc.) on top.
func NewStorageBackingRegistry() *StorageBackingRegistry {
	r := &StorageBackingRegistry{drivers: map[StorageBacking]StorageBackingDriver{}}
	r.Register(&EmptyDirDriver{})
	return r
}

// Register adds a driver to the registry, keyed by driver.Backing().
// Last writer wins (operator-override semantics). Panics if driver is nil.
func (r *StorageBackingRegistry) Register(d StorageBackingDriver) {
	if d == nil {
		panic("storage backing registry: nil driver")
	}
	r.drivers[d.Backing()] = d
}

// Resolve returns the driver for the requested backing, or an error if
// the backing isn't registered. Per the no-fallbacks directive, empty
// or unknown values fail loudly — operators MUST explicitly configure
// SharedVolume.Backing. The empty-Backing case in particular is
// rejected because every cell has known cost/lifecycle requirements
// and silent default selection masks operator misconfiguration.
func (r *StorageBackingRegistry) Resolve(b StorageBacking) (StorageBackingDriver, error) {
	if b == "" {
		return nil, fmt.Errorf("storage backing: SharedVolume.Backing is required (no default — set explicitly: %q, %q, or %q)",
			BackingEmptyDir, BackingGCSSync, BackingGCSFuse)
	}
	if d, ok := r.drivers[b]; ok {
		return d, nil
	}
	registered := make([]string, 0, len(r.drivers))
	for k := range r.drivers {
		registered = append(registered, string(k))
	}
	return nil, fmt.Errorf("storage backing %q: no driver registered (registered: %v)", b, registered)
}

// EmptyDirDriver implements StorageBackingDriver for the in-memory tmpfs
// fallback. Always available; cloud-agnostic; zero idle cost.
type EmptyDirDriver struct{}

func (d *EmptyDirDriver) Backing() StorageBacking { return BackingEmptyDir }

func (d *EmptyDirDriver) CloudSpec(vol SharedVolumeRef) (BackingSpec, error) {
	return BackingSpec{
		Kind:     BackingEmptyDir,
		EmptyDir: &EmptyDirSpec{Medium: "Memory"},
	}, nil
}

func (d *EmptyDirDriver) PreExec(ctx context.Context, vol SharedVolumeRef, execID, localPath string) (map[string]string, error) {
	return nil, nil
}

func (d *EmptyDirDriver) PostExec(ctx context.Context, vol SharedVolumeRef, execID, localPath string) error {
	return nil
}

// (validateRef helper removed — drivers do their own validation since
// the required fields differ per backing.)
