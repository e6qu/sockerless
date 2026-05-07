// Per-backend volume translator (Phase 123 step 3, cloudrun mirror).
//
// Maps a cloud-agnostic core.BackingSpec (produced by a
// core.StorageBackingDriver) to the cloudrun-specific runpb.Volume
// protobuf. Replaces the inline runpb.Volume_Gcs{} / runpb.Volume_EmptyDir{}
// constructions that used to live in volumes.go::buildVolumeForBind.
package cloudrun

import (
	"context"
	"fmt"

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
	return s.storageBackings.Resolve(vol.AsRef().Backing)
}

// cloudRunVolumeFromBacking converts a SharedVolume to a runpb.Volume.
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

// runpbVolumeFromBackingSpec — pure-function translator.
func runpbVolumeFromBackingSpec(name string, spec core.BackingSpec) (*runpb.Volume, error) {
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

// preExecHintsForVolumes runs PreExec on each SharedVolume's driver and
// merges env hints. Fails loudly on unresolvable backings.
func (s *Server) preExecHintsForVolumes(ctx context.Context, vols []SharedVolume, execID string) (map[string]string, error) {
	merged := map[string]string{}
	for _, v := range vols {
		driver, err := s.resolveBackingDriver(v)
		if err != nil {
			return nil, fmt.Errorf("volume %q: %w", v.Name, err)
		}
		hints, err := driver.PreExec(ctx, v.AsRef(), execID, v.ContainerPath)
		if err != nil {
			return nil, fmt.Errorf("PreExec %s (backing=%q): %w", v.Name, driver.Backing(), err)
		}
		for k, val := range hints {
			merged[k] = val
		}
	}
	return merged, nil
}

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
