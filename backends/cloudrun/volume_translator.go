// Per-backend volume translator (cloudrun mirror).
//
// Maps a cloud-agnostic core.BackingSpec (produced by a
// core.StorageBackingDriver) to the cloudrun-specific runpb.Volume
// protobuf. Replaces the inline runpb.Volume_Gcs{} / runpb.Volume_EmptyDir{}
// constructions that used to live in volumes.go::buildVolumeForBind.
package cloudrun

import (
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

	case core.BackingMemory:
		// memory == cloud-agnostic RAM-backed mount. On Cloud Run,
		// that's EmptyDir with Memory medium — same primitive as
		// emptyDir, but the operator's intent is explicit
		// (cross-cloud uniform `Backing: memory` rather than
		// per-cloud emptyDir variations). SizeMB is honoured by
		// Cloud Run via the volume's size limit when set; zero =
		// container's memory limit.
		v := &runpb.Volume{
			Name: name,
			VolumeType: &runpb.Volume_EmptyDir{EmptyDir: &runpb.EmptyDirVolumeSource{
				Medium: runpb.EmptyDirVolumeSource_MEMORY,
			}},
		}
		if spec.Memory != nil && spec.Memory.SizeMB > 0 {
			v.VolumeType.(*runpb.Volume_EmptyDir).EmptyDir.SizeLimit =
				fmt.Sprintf("%dMi", spec.Memory.SizeMB)
		}
		return v, nil

	case core.BackingGCSFuse:
		// Cloud Run wraps gcsfuse and rejects the cache-TTL flags
		// (`metadata-cache:ttl-secs`, `metadata-cache:negative-ttl-secs`)
		// that are MANDATORY for safe cross-task workspace use — without
		// them, the default 5s negative-cache hides freshly-written files
		// from sibling containers. Backing: gcs-sync sidesteps FUSE
		// entirely (per-exec tar/untar against a single GCS object) and
		// has strong consistency. See storage_gcsfuse.go for the driver.
		return nil, fmt.Errorf(
			"volume %q: backing %q is unsupported on Cloud Run — "+
				"Cloud Run rejects the cache-TTL gcsfuse flags needed for "+
				"cross-task safety. Use Backing: gcs-sync instead "+
				"(per-exec tar sync, no FUSE)",
			name, spec.Kind)

	case core.BackingPDEphemeral:
		// Cloud Run Services don't expose Compute Engine PD as a
		// first-class volume primitive. The runpb.Volume union has
		// EmptyDir / Secret / CloudSqlInstance / Gcs / Nfs — no PD.
		// Real implementation would require sockerless to manage
		// `disks.create` + revision-update for `disks.attach` (the
		// Cloud Run Admin API doesn't have a PD attach surface as of
		// this writing). See specs/CLOUD_RESOURCE_MAPPING.md line
		// 567 for the design bookmark.
		return nil, fmt.Errorf(
			"volume %q: backing %q not supported on Cloud Run — "+
				"Cloud Run Services lack a first-class PD volume primitive. "+
				"Use Backing: gcs-sync for cross-task workspace sharing. "+
				"A future GCE-style backend would unlock real PD attach "+
				"(specs/CLOUD_RESOURCE_MAPPING.md line 567)",
			name, spec.Kind)
	}
	return nil, fmt.Errorf("volume %q: unsupported backing kind %q", name, spec.Kind)
}
