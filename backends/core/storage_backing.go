// Package core — storage backing driver abstraction.
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
// Project directives baked in:
//   - Storage MUST be pluggable so we can test multiple options without
//     re-refactoring each backend.
//   - Zero-scaling, no-cost-when-not-in-use is the paradigm. Acceptable:
//     object storage, in-memory, ephemeral managed FS where sockerless
//     owns the lifecycle. Rejected: NFS / Filestore / Memorystore /
//     persistent-mode PDs (all bill idle).
//   - No FUSE-on-object-store for new SharedVolumes (gcs-fuse retained
//     ONLY for legacy tar-pack persist mounts).
//   - **No automatic fallbacks**: every SharedVolume MUST have an
//     explicitly-set Backing. Resolve() returns an error for empty or
//     unknown backings rather than silently selecting a default.
//     Rationale: each backing has different cost / scale / consistency
//     characteristics; the operator's choice is load-bearing, and
//     silent fallback masks misconfiguration that surfaces as confusing
//     runtime failures (gcs-fuse expects whole-tar persist semantics;
//     gcs-sync expects per-step propagation; emptyDir would "work" for
//     both up to the first cross-Service read, then break).
//
// See specs/CLOUD_RESOURCE_MAPPING.md § "Storage backing driver
// abstraction" for the full architectural design.
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

	// BackingPDEphemeral — Compute Engine Persistent Disk attached to a
	// Cloud Run / Cloud Run Jobs instance as an ephemeral volume. The
	// underlying PD is sockerless-managed (parent cost = $/GiB-stored);
	// the per-task attachment lives only for the task's lifecycle.
	BackingPDEphemeral StorageBacking = "pd-ephemeral"

	// BackingEFSEphemeral — EFS access point on a sockerless-managed
	// filesystem, attached to an ECS task / Lambda function as an
	// ephemeral mount. Parent EFS bills per-GiB-stored; the access-point
	// attachment is task-scoped.
	BackingEFSEphemeral StorageBacking = "efs-ephemeral"

	// BackingAzureFilesEphemeral — Azure Files share on a
	// sockerless-managed storage account, attached to an ACA job /
	// Azure Function App as an ephemeral mount. Parent share bills
	// per-GiB-stored; the per-task attachment is task-scoped.
	BackingAzureFilesEphemeral StorageBacking = "azure-files-ephemeral"

	// BackingMemory — pure RAM-backed mount inside the workload
	// container. Cloud-agnostic; no cloud-side resource is provisioned.
	// Each backend translates this to its cloud-native tmpfs primitive
	// (EmptyDir{Medium: MEMORY} on Cloud Run / GCF / ACA, tmpfs mount
	// on ECS task definition, /tmp scratch on Lambda). Sibling to the
	// ephemeral managed-FS backings — use this when the workload only
	// needs scratch space the cloud's RAM can hold and no parent
	// resource (PD, EFS, Azure Files) is required.
	BackingMemory StorageBacking = "memory"
)

// SharedVolumeRef is the cloud-agnostic volume reference passed to drivers.
// Each backend's per-cloud SharedVolume struct (e.g. gcf.SharedVolume,
// cloudrun.SharedVolume) converts to this for driver dispatch.
type SharedVolumeRef struct {
	Name          string         // logical volume name
	ContainerPath string         // mount path inside the consumer container
	Backing       StorageBacking // resolves a driver in the Registry
	ReadOnly      bool           // read-only mount

	// GCS-family fields (gcs-sync, gcs-fuse)
	GCSBucket string

	// pd-ephemeral fields
	PDDiskSizeGB int
	PDZone       string

	// efs-ephemeral fields
	EFSFileSystemID  string
	EFSAccessPointID string

	// azure-files-ephemeral fields
	AzureStorageAccount string
	AzureShareName      string

	// memory backing fields
	MemorySizeMB int
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
	// Returns list-valued environment hints (e.g.
	// SOCKERLESS_SYNC_VOLUMES → [triple]) the backend merges into the
	// envelope's Env. Multi-volume callers concatenate per-key slices
	// before serialising so hints from N volumes don't clobber. Live
	// filesystems (gcs-fuse, emptyDir) return (nil, nil).
	//
	// localPath is the path on the runner-task where the data lives
	// (the volume source — e.g. /tmp/runner-work). remotePath is the
	// path on the JOB pod-Service where the bootstrap should restore
	// to (the bind target — e.g. /__w). The two differ because docker
	// `-v src:dst` binds: tar from src locally, untar to dst remotely.
	PreExec(ctx context.Context, vol SharedVolumeRef, execID, localPath, remotePath string) (envHints map[string][]string, err error)

	// PostExec runs after the exec response returns to the backend.
	// For sync drivers: pulls the bootstrap's modified state back to
	// localPath. Live filesystems return nil.
	PostExec(ctx context.Context, vol SharedVolumeRef, execID, localPath string) error
}

// BackingSpec is the cloud-agnostic mount description. Per-backend
// volume translators consume this and emit the cloud's actual volume
// protobuf (runpb.Volume for Cloud Run, ECS Volume{} for AWS, etc.).
// Exactly one payload field is set; Kind says which.
type BackingSpec struct {
	Kind                StorageBacking
	EmptyDir            *EmptyDirSpec            // emptyDir + gcs-sync (which uses emptyDir as the local mount)
	GCS                 *GCSSpec                 // gcs-fuse (direct FUSE mount)
	PDEphemeral         *PDEphemeralSpec         // pd-ephemeral (Compute Engine PD ephemeral attach)
	EFSEphemeral        *EFSEphemeralSpec        // efs-ephemeral (EFS access point on managed FS)
	AzureFilesEphemeral *AzureFilesEphemeralSpec // azure-files-ephemeral (Azure Files share on managed account)
	Memory              *MemorySpec              // memory (RAM-backed tmpfs in workload container)
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

// PDEphemeralSpec describes a Compute Engine Persistent Disk attached
// as an ephemeral volume to a Cloud Run / Cloud Run Jobs instance.
// Lifecycle = task lifecycle. Translators map this to the cloud's
// per-instance disk attach API.
type PDEphemeralSpec struct {
	DiskSizeGB int    // requested disk size; cloud rounds up to next valid quantum
	Zone       string // zone for the underlying PD; empty = backend-region default
}

// EFSEphemeralSpec describes an EFS access point on a sockerless-managed
// filesystem, attached as an ephemeral mount to an ECS task / Lambda
// function. Lifecycle = task lifecycle.
type EFSEphemeralSpec struct {
	FileSystemID  string // EFS filesystem ID (fs-...)
	AccessPointID string // EFS access point ID (fsap-...)
	ReadOnly      bool
}

// AzureFilesEphemeralSpec describes an Azure Files share on a
// sockerless-managed storage account, attached as an ephemeral mount
// to an ACA job / Azure Function App. Lifecycle = task lifecycle.
type AzureFilesEphemeralSpec struct {
	StorageAccount string // storage account hosting the share
	ShareName      string // share name within the account
	ReadOnly       bool
}

// MemorySpec describes a RAM-backed tmpfs mount inside the workload
// container. SizeMB is the requested upper bound; backends translate
// to the cloud's native memory-backed primitive (EmptyDir{Medium:
// MEMORY}, ECS tmpfs, Lambda /tmp scratch). Lifecycle = container
// lifecycle; nothing persists past container exit.
type MemorySpec struct {
	SizeMB int // requested cap; backend may round up to the next valid quantum
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
		return nil, fmt.Errorf("storage backing: SharedVolume.Backing is required (no default — set explicitly: %q, %q, %q, %q, %q, %q, or %q)",
			BackingEmptyDir, BackingGCSSync, BackingGCSFuse,
			BackingPDEphemeral, BackingEFSEphemeral, BackingAzureFilesEphemeral,
			BackingMemory)
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

func (d *EmptyDirDriver) PreExec(ctx context.Context, vol SharedVolumeRef, execID, localPath, remotePath string) (map[string][]string, error) {
	return nil, nil
}

func (d *EmptyDirDriver) PostExec(ctx context.Context, vol SharedVolumeRef, execID, localPath string) error {
	return nil
}

// MemoryDriver implements StorageBackingDriver for the RAM-backed
// tmpfs mount. Cloud-agnostic; no cloud SDK call. Each backend's
// volume translator emits the cloud-native memory primitive
// (EmptyDir{Medium: MEMORY} on Cloud Run / GCF / ACA, tmpfs mount
// on ECS task definition, /tmp scratch on Lambda).
//
// Use this when the workload only needs scratch space the cloud's
// RAM can hold and no parent resource (PD, EFS, Azure Files) is
// required — e.g. small build caches, temporary artifacts, IPC
// scratch. Distinct from emptyDir in that the operator opts in
// explicitly to RAM-backed (not disk-backed) ephemeral storage,
// and the SizeMB cap is surfaced in the cloud-native materializer.
type MemoryDriver struct {
	// DefaultSizeMB applies when SharedVolumeRef.MemorySizeMB is zero.
	// Set by backend startup; conservative default = 64 MiB.
	DefaultSizeMB int
}

// NewMemoryDriver returns a driver with the given default size cap.
// A zero default leaves the cap unset; the per-volume override is
// then required at CloudSpec time.
func NewMemoryDriver(defaultSizeMB int) *MemoryDriver {
	return &MemoryDriver{DefaultSizeMB: defaultSizeMB}
}

func (d *MemoryDriver) Backing() StorageBacking { return BackingMemory }

func (d *MemoryDriver) CloudSpec(vol SharedVolumeRef) (BackingSpec, error) {
	size := vol.MemorySizeMB
	if size == 0 {
		size = d.DefaultSizeMB
	}
	if size <= 0 {
		return BackingSpec{}, fmt.Errorf("memory: size must be > 0 MiB (vol=%q)", vol.Name)
	}
	return BackingSpec{
		Kind:   BackingMemory,
		Memory: &MemorySpec{SizeMB: size},
	}, nil
}

func (d *MemoryDriver) PreExec(ctx context.Context, vol SharedVolumeRef, execID, localPath, remotePath string) (map[string][]string, error) {
	return nil, nil
}

func (d *MemoryDriver) PostExec(ctx context.Context, vol SharedVolumeRef, execID, localPath string) error {
	return nil
}

// (validateRef helper removed — drivers do their own validation since
// the required fields differ per backing.)
