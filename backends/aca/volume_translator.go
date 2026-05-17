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
// to the JobTemplate. Honors the storage-backing registry's default
// (after P168.5 that's BackingMemory for ACA, materialising as
// StorageTypeEmptyDir / tmpfs). Operators wanting persistence pick
// it up by overriding the registry default at NewServer.
//
// Azure Files share provisioning only happens when the resolved
// backing actually needs it (BackingAzureFilesEphemeral). Memory-
// backed volumes don't need a share.
func (s *Server) resolveVolumeForName(ctx context.Context, volName string) (*armappcontainers.Volume, error) {
	driver, err := s.storageBackings.Resolve("")
	if err != nil {
		return nil, fmt.Errorf("resolve default storage backing for volume %q: %w", volName, err)
	}
	backing := driver.Backing()
	share := ""
	if backing == core.BackingAzureFilesEphemeral {
		share, err = s.shareForVolume(ctx, volName)
		if err != nil {
			return nil, fmt.Errorf("provision Azure Files share for volume %q: %w", volName, err)
		}
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
