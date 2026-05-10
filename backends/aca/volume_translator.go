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

// resolveVolumeForName provisions the underlying Azure Files share (or
// other backing) and returns the cloud-native Volume entry to attach to
// the JobTemplate. Empty Backing on the SharedVolume defaults to
// `azure-files-ephemeral` since that's the only backing ACA wires today.
func (s *Server) resolveVolumeForName(ctx context.Context, volName string) (*armappcontainers.Volume, error) {
	share, err := s.shareForVolume(ctx, volName)
	if err != nil {
		return nil, fmt.Errorf("provision Azure Files share for volume %q: %w", volName, err)
	}
	// ACA config doesn't carry a per-volume Backing today (single
	// backing supported); the registry path lets operators override
	// once the config grows that field.
	backing := core.BackingAzureFilesEphemeral
	driver, err := s.storageBackings.Resolve(backing)
	if err != nil {
		return nil, fmt.Errorf("resolve storage backing for volume %q: %w", volName, err)
	}
	spec, err := driver.CloudSpec(core.SharedVolumeRef{
		Name:                volName,
		Backing:             backing,
		AzureStorageAccount: s.config.StorageAccount,
		AzureShareName:      share,
	})
	if err != nil {
		return nil, fmt.Errorf("CloudSpec for volume %q: %w", volName, err)
	}
	return translateBackingSpecToACAVolume(volName, share, spec)
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
