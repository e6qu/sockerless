package azf

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appservice/armappservice/v5"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage"
	azurecommon "github.com/sockerless/azure-common"
)

// Azure Files-backed named-volume provisioning for Azure Functions
// Flex Consumption / Premium plan.
//
// Unlike ACA (which uses `managedEnvironmentsStorages`), Azure Functions
// attaches file shares via the `sites/<siteName>/config/azurestorageaccounts/`
// sub-resource. Each entry is an AzureStorageInfoValue carrying
// `{accountName, shareName, accessKey, mountPath, protocol}`. The access
// key is embedded in plaintext, so the backend fetches the freshest key
// via StorageAccounts.ListKeys at attach-time rather than caching.
//
// Volume CRUD reuses azurecommon.FileShareManager shared with the ACA
// backend. ContainerStart (backend_impl.go) calls this file's helper to
// attach shares after WebApps.BeginCreateOrUpdate returns.
//
// Host-path bind specs (`/h:/c`) stay rejected — AZF containers have no
// host filesystem to bind from.

// azfVolumeState embeds the shared FileShareManager. Initialised by
// NewServer once the FileShares client is available.
type azfVolumeState struct {
	shares *azurecommon.FileShareManager
}

// shareForVolume returns the share name bound to a Docker volume,
// provisioning the share on first call. Operator-declared
// SharedVolumes (SOCKERLESS_AZF_SHARED_VOLUMES) pin the Azure Files
// share directly — that's how the runner app and the spawned sub-task
// land on the SAME pre-provisioned share without sockerless creating a
// fresh one per docker run.
func (s *Server) shareForVolume(ctx context.Context, volName string) (string, error) {
	if sv := s.config.SharedVolumes.ByName(volName); sv != nil {
		return sv.AzureShareName, nil
	}
	return s.shares.EnsureShare(ctx, volName)
}

// deleteShareForVolume removes the underlying Azure Files share.
// Operator-declared SharedVolumes are operator-owned — sockerless
// never deletes their shares.
func (s *Server) deleteShareForVolume(ctx context.Context, volName string) error {
	if s.config.SharedVolumes.ByName(volName) != nil {
		return nil
	}
	return s.shares.DeleteShare(ctx, volName)
}

func (s *Server) listManagedShares(ctx context.Context) ([]*armstorage.FileShareItem, error) {
	return s.shares.ListManaged(ctx)
}

// attachVolumesToFunctionSite parses a slice of Docker bind specs
// (`volName:/mnt[:ro]`), provisions a file share per unique named
// volume, fetches the fresh storage-account access key, and calls
// `WebApps.UpdateAzureStorageAccounts` to register each share → mount
// path pair on the site.
func (s *Server) attachVolumesToFunctionSite(ctx context.Context, siteName string, binds []string) error {
	if len(binds) == 0 {
		return nil
	}
	if s.config.StorageAccount == "" {
		return fmt.Errorf("SOCKERLESS_AZF_STORAGE_ACCOUNT must be set to attach file-share volumes")
	}

	// Fetch the freshest storage-account keys (lazily, per distinct
	// account — SharedVolumes may pin shares on a different account
	// than the sockerless-managed one) so rotated keys take effect
	// without a backend restart.
	keyByAccount := map[string]string{}
	keyForAccount := func(account string) (string, error) {
		if key, ok := keyByAccount[account]; ok {
			return key, nil
		}
		keys, err := s.azure.StorageAccounts.ListKeys(ctx, s.config.ResourceGroup, account, nil)
		if err != nil {
			return "", fmt.Errorf("list storage account keys for %q: %w", account, err)
		}
		if len(keys.Keys) == 0 || keys.Keys[0] == nil || keys.Keys[0].Value == nil {
			return "", fmt.Errorf("storage account %q has no access keys", account)
		}
		keyByAccount[account] = *keys.Keys[0].Value
		return keyByAccount[account], nil
	}

	dict := map[string]*armappservice.AzureStorageInfoValue{}
	for _, b := range binds {
		parts := strings.SplitN(b, ":", 3)
		if len(parts) < 2 {
			return fmt.Errorf("invalid bind %q", b)
		}
		volName, mountPath := parts[0], parts[1]
		shareName, err := s.shareForVolume(ctx, volName)
		if err != nil {
			return fmt.Errorf("provision share for %q: %w", volName, err)
		}
		account := s.config.StorageAccount
		if sv := s.config.SharedVolumes.ByName(volName); sv != nil {
			account = sv.AzureAccountOrDefault(s.config.StorageAccount)
		}
		accessKey, err := keyForAccount(account)
		if err != nil {
			return err
		}
		info, err := s.resolveStorageInfoForVolume(volName, mountPath, shareName, account, accessKey)
		if err != nil {
			return err
		}
		dict[volName] = info
	}

	_, err := s.azure.WebApps.UpdateAzureStorageAccounts(ctx, s.config.ResourceGroup, siteName,
		armappservice.AzureStoragePropertyDictionaryResource{Properties: dict}, nil)
	if err != nil {
		return fmt.Errorf("update azurestorageaccounts on site %q: %w", siteName, err)
	}
	return nil
}
