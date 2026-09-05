// Per-backend translator: cloud-agnostic core.BackingSpec → ACA
// JobTemplate.Volumes element. Each named-volume bind in
// HostConfig.Binds resolves through s.storageBackings (registered
// with azurecommon.AzureFilesEphemeralDriver + core.MemoryDriver at
// startup), the driver returns a BackingSpec, and this translator
// materialises the cloud-native armappcontainers.Volume.

package aca

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appcontainers/armappcontainers/v3"
	core "github.com/sockerless/backend-core"
)

// resolveVolumeForName returns the cloud-native Volume entry to attach
// to the JobTemplate. Operator-declared SharedVolumes
// (SOCKERLESS_ACA_SHARED_VOLUMES) carry an explicit Backing + a
// pre-provisioned Azure Files share; ad-hoc named volumes honor the
// storage-backing registry's default (BackingMemory for ACA,
// materialising as StorageTypeEmptyDir / tmpfs). Operators wanting
// persistence for ad-hoc volumes pick it up by overriding the registry
// default at NewServer.
//
// Azure Files share provisioning / env-storage linkage only happens
// when the resolved backing actually needs it
// (BackingAzureFilesEphemeral). Memory-backed volumes don't need a share.
func (s *Server) resolveVolumeForName(ctx context.Context, volName string) (*armappcontainers.Volume, error) {
	ref := core.SharedVolumeRef{
		Name:                volName,
		AzureStorageAccount: s.config.StorageAccount,
	}
	// An ad-hoc named volume (`docker volume create <name>` or an auto-created
	// named-volume bind) is shared across the containers that mount it and
	// persists for the run, so back it with a shared Azure Files share — the
	// same share VolumeCreate provisions. A per-container EmptyDir would give
	// each container its own empty mount (e.g. a gitlab-runner build container
	// couldn't see the workspace its helper container cloned into /builds).
	requested := core.BackingAzureFilesEphemeral
	if sv := s.config.SharedVolumes.ByName(volName); sv != nil {
		// Explicit Backing, no default — empty/unknown fails loudly in
		// Resolve per the no-fallbacks directive.
		ref = sv.WithAzureAccountDefault(s.config.StorageAccount)
		requested = ref.Backing
	}
	driver, err := s.storageBackings.Resolve(requested)
	if err != nil {
		return nil, fmt.Errorf("resolve storage backing for volume %q: %w", volName, err)
	}
	ref.Backing = driver.Backing()
	if ref.Backing == core.BackingAzureFilesEphemeral {
		share, err := s.shareForVolume(ctx, volName)
		if err != nil {
			return nil, fmt.Errorf("provision Azure Files share for volume %q: %w", volName, err)
		}
		ref.AzureShareName = share
	}
	spec, err := driver.CloudSpec(ref)
	if err != nil {
		return nil, fmt.Errorf("CloudSpec for volume %q: %w", volName, err)
	}
	return translateBackingSpecToACAVolume(volName, ref.AzureShareName, spec)
}

func translateBackingSpecToACAVolume(name, share string, spec core.BackingSpec) (*armappcontainers.Volume, error) {
	switch spec.Kind {
	case core.BackingAzureFilesEphemeral:
		t := armappcontainers.StorageTypeAzureFile
		return &armappcontainers.Volume{
			Name:        ptr(name),
			StorageType: &t,
			StorageName: ptr(share),
		}, nil

	case core.BackingMemory:
		// Azure Container Apps revisions support EmptyDir as a
		// first-class storage type — direct match for the cloud-
		// agnostic memory backing. StorageName is unused for
		// EmptyDir; size-cap honoring is left to the cloud (ACA
		// scopes EmptyDir to the container's memory limit).
		t := armappcontainers.StorageTypeEmptyDir
		return &armappcontainers.Volume{
			Name:        ptr(name),
			StorageType: &t,
		}, nil
	}
	return nil, fmt.Errorf("aca translator: backing %q not supported on ACA", spec.Kind)
}
