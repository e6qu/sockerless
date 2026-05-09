// Per-backend translator: cloud-agnostic core.BackingSpec → Azure
// WebApps AzureStorageInfoValue (Azure Functions consumes Azure Files
// shares via WebApps.UpdateAzureStorageAccounts). Each named-volume
// bind in HostConfig.Binds resolves through s.storageBackings
// (registered with azurecommon.AzureFilesEphemeralDriver +
// core.MemoryDriver at startup); the driver returns a BackingSpec;
// this translator emits the cloud-native AzureStorageInfoValue entry.

package azf

import (
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appservice/armappservice/v5"
	core "github.com/sockerless/backend-core"
)

// resolveStorageInfoForVolume materialises a single bind entry through
// the storage-backing registry. Returns the AzureStorageInfoValue to
// add to the WebApps.UpdateAzureStorageAccounts dictionary. Empty
// Backing on the volume defaults to `azure-files-ephemeral` since
// that's the only backing AZF wires today.
func (s *Server) resolveStorageInfoForVolume(volName, mountPath, shareName, accessKey string) (*armappservice.AzureStorageInfoValue, error) {
	backing := core.BackingAzureFilesEphemeral
	driver, err := s.storageBackings.Resolve(backing)
	if err != nil {
		return nil, fmt.Errorf("resolve storage backing for volume %q: %w", volName, err)
	}
	spec, err := driver.CloudSpec(core.SharedVolumeRef{
		Name:                volName,
		ContainerPath:       mountPath,
		Backing:             backing,
		AzureStorageAccount: s.config.StorageAccount,
		AzureShareName:      shareName,
	})
	if err != nil {
		return nil, fmt.Errorf("CloudSpec for volume %q: %w", volName, err)
	}
	return translateBackingSpecToAZFStorage(spec, mountPath, accessKey)
}

func translateBackingSpecToAZFStorage(spec core.BackingSpec, mountPath, accessKey string) (*armappservice.AzureStorageInfoValue, error) {
	switch spec.Kind {
	case core.BackingAzureFilesEphemeral:
		if spec.AzureFilesEphemeral == nil {
			return nil, fmt.Errorf("azf translator: azure-files-ephemeral spec missing payload")
		}
		return &armappservice.AzureStorageInfoValue{
			Type:        to.Ptr(armappservice.AzureStorageTypeAzureFiles),
			AccountName: to.Ptr(spec.AzureFilesEphemeral.StorageAccount),
			ShareName:   to.Ptr(spec.AzureFilesEphemeral.ShareName),
			AccessKey:   to.Ptr(accessKey),
			MountPath:   to.Ptr(mountPath),
		}, nil
	}
	return nil, fmt.Errorf("azf translator: backing %q not supported on AZF", spec.Kind)
}
