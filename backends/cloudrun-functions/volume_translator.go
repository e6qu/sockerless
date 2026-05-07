// Per-backend volume translator (Phase 123 step 3).
//
// Maps a cloud-agnostic core.BackingSpec (produced by a
// core.StorageBackingDriver) to the gcf-specific runpb.Volume protobuf
// the materialize path emits. Replaces the inline runpb.Volume_Gcs{} /
// runpb.Volume_EmptyDir{} constructions that used to live in
// pod_service.go::buildVolumeForBindGCF.
//
// Architecture: only this file knows about runpb.Volume_Gcs vs.
// runpb.Volume_EmptyDir; everything else stays at the BackingSpec
// abstraction. Adding a new driver (gcs-sync, future ephemeral-PD, etc.)
// to the gcf backend means adding one switch case here, NOT touching
// every call site.
package gcf

import (
	"context"
	"fmt"
	"strings"

	runpb "cloud.google.com/go/run/apiv2/runpb"
	core "github.com/sockerless/backend-core"
)

// resolveBackingDriver returns the StorageBackingDriver for a SharedVolume,
// or an error per the no-fallbacks directive. Callers MUST handle the
// error rather than silently defaulting.
func (s *Server) resolveBackingDriver(vol SharedVolume) (core.StorageBackingDriver, error) {
	if s.storageBackings == nil {
		return nil, fmt.Errorf("storage backing registry not initialized (volume %q)", vol.Name)
	}
	ref := vol.AsRef()
	return s.storageBackings.Resolve(ref.Backing)
}

// cloudRunVolumeFromBacking converts a SharedVolume (gcf-side struct)
// to a runpb.Volume the Cloud Run Service revision spec can carry.
// Routes through the backing driver's CloudSpec for the cloud-agnostic
// shape, then maps to the gcf-specific protobuf.
func (s *Server) cloudRunVolumeFromBacking(vol SharedVolume) (*runpb.Volume, error) {
	driver, err := s.resolveBackingDriver(vol)
	if err != nil {
		return nil, fmt.Errorf("volume %q: %w", vol.Name, err)
	}
	spec, err := driver.CloudSpec(vol.AsRef())
	if err != nil {
		return nil, fmt.Errorf("backing %q CloudSpec for volume %q: %w", driver.Backing(), vol.Name, err)
	}
	return runpbVolumeFromBackingSpec(vol.Name, spec)
}

// runpbVolumeFromBackingSpec — pure-function translator (no Server
// receiver) so test code can drive it without spinning up a server.
// Exactly one of EmptyDir / GCS is set on a valid BackingSpec.
func runpbVolumeFromBackingSpec(name string, spec core.BackingSpec) (*runpb.Volume, error) {
	switch spec.Kind {
	case core.BackingEmptyDir, core.BackingGCSSync:
		// gcs-sync also uses an emptyDir mount on the JOB pod-Service
		// side — the GCS sync happens out-of-band via PreExec/PostExec
		// on the runner-task and SOCKERLESS_WORKSPACE_OBJECT env hint
		// on the bootstrap.
		medium := runpb.EmptyDirVolumeSource_MEMORY
		if spec.EmptyDir != nil && spec.EmptyDir.Medium != "Memory" && spec.EmptyDir.Medium != "" {
			medium = runpb.EmptyDirVolumeSource_MEDIUM_UNSPECIFIED
		}
		return &runpb.Volume{
			Name:       name,
			VolumeType: &runpb.Volume_EmptyDir{EmptyDir: &runpb.EmptyDirVolumeSource{Medium: medium}},
		}, nil

	case core.BackingGCSFuse:
		if spec.GCS == nil || spec.GCS.Bucket == "" {
			return nil, fmt.Errorf("gcs-fuse backing for volume %q missing bucket", name)
		}
		return &runpb.Volume{
			Name: name,
			VolumeType: &runpb.Volume_Gcs{
				Gcs: &runpb.GCSVolumeSource{
					Bucket:       spec.GCS.Bucket,
					MountOptions: append([]string{}, spec.GCS.MountOptions...),
					ReadOnly:     spec.GCS.ReadOnly,
				},
			},
		}, nil
	}
	return nil, fmt.Errorf("volume %q: unsupported backing kind %q", name, spec.Kind)
}

// preExecHintsForVolumes runs PreExec on each SharedVolume's driver
// and merges per-key list-valued hints. The GCS drivers emit just
// `name=gs://bucket/object` pairs under SOCKERLESS_SYNC_VOLUMES; the
// bind target is NOT included here because the runner-task can't
// reliably know it (api.HostConfig.Binds is empty after the stateless
// cloud_state round-trip — BUG-967). Instead, the bind-target map is
// recorded at materialize time on the JOB container as
// SOCKERLESS_SYNC_MOUNTS, and the bootstrap joins by volume name.
//
// `binds` is accepted for forward-compatibility with drivers that
// want bind-target context (e.g. in-process backends that bypass
// cloud_state); GCS drivers ignore it. Returns one comma-joined env
// entry per key, ready to append to the envelope's Env.
//
// No-fallbacks: a volume with an unresolvable Backing fails loudly.
func (s *Server) preExecHintsForVolumes(ctx context.Context, vols []SharedVolume, binds []string, execID string) (map[string]string, error) {
	_ = binds // see comment above
	merged := map[string][]string{}
	for _, v := range vols {
		driver, err := s.resolveBackingDriver(v)
		if err != nil {
			return nil, fmt.Errorf("volume %q: %w", v.Name, err)
		}
		hints, err := driver.PreExec(ctx, v.AsRef(), execID, v.ContainerPath, "")
		if err != nil {
			return nil, fmt.Errorf("PreExec %s (backing=%q): %w", v.Name, driver.Backing(), err)
		}
		for k, vals := range hints {
			merged[k] = append(merged[k], vals...)
		}
	}
	out := make(map[string]string, len(merged))
	for k, vals := range merged {
		out[k] = strings.Join(vals, ",")
	}
	return out, nil
}

// postExecForVolumes runs PostExec on each SharedVolume's driver. The
// data plane has already returned to the caller; PostExec cleanup
// errors are wrapped and surfaced rather than swallowed.
func (s *Server) postExecForVolumes(ctx context.Context, vols []SharedVolume, execID string) error {
	for _, v := range vols {
		driver, err := s.resolveBackingDriver(v)
		if err != nil {
			return fmt.Errorf("volume %q: %w", v.Name, err)
		}
		if err := driver.PostExec(ctx, v.AsRef(), execID, v.ContainerPath); err != nil {
			return fmt.Errorf("PostExec %s (backing=%q): %w", v.Name, driver.Backing(), err)
		}
	}
	return nil
}
