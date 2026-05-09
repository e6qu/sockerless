// Package azurecommon — azure-files-ephemeral storage backing driver.
//
// Azure Files share on a sockerless-managed storage account, attached
// as an ephemeral mount to an ACA job / Azure Function App. Lifecycle
// is bound to the task — the share survives across tasks (durable
// workspace), but each task's mount is per-task.
//
// PreExec / PostExec are no-ops: the cloud attaches the share during
// task startup and detaches at task end; sockerless does no data sync.
package azurecommon

import (
	"context"
	"fmt"

	core "github.com/sockerless/backend-core"
)

// AzureFilesEphemeralDriver implements core.StorageBackingDriver for the
// azure-files-ephemeral mount type.
type AzureFilesEphemeralDriver struct {
	// DefaultStorageAccount is used when SharedVolumeRef.AzureStorageAccount
	// is empty. Set by backend startup to the configured sockerless-managed
	// storage account name.
	DefaultStorageAccount string
}

// NewAzureFilesEphemeralDriver constructs the driver with the operator's
// default storage account. The default may be empty; CloudSpec rejects
// a vol whose resolved storage account is empty.
func NewAzureFilesEphemeralDriver(defaultAccount string) *AzureFilesEphemeralDriver {
	return &AzureFilesEphemeralDriver{DefaultStorageAccount: defaultAccount}
}

func (d *AzureFilesEphemeralDriver) Backing() core.StorageBacking {
	return core.BackingAzureFilesEphemeral
}

func (d *AzureFilesEphemeralDriver) CloudSpec(vol core.SharedVolumeRef) (core.BackingSpec, error) {
	account := vol.AzureStorageAccount
	if account == "" {
		account = d.DefaultStorageAccount
	}
	if account == "" {
		return core.BackingSpec{}, fmt.Errorf("azure-files-ephemeral: StorageAccount required (vol=%q)", vol.Name)
	}
	if vol.AzureShareName == "" {
		return core.BackingSpec{}, fmt.Errorf("azure-files-ephemeral: ShareName required (vol=%q)", vol.Name)
	}
	return core.BackingSpec{
		Kind: core.BackingAzureFilesEphemeral,
		AzureFilesEphemeral: &core.AzureFilesEphemeralSpec{
			StorageAccount: account,
			ShareName:      vol.AzureShareName,
			ReadOnly:       vol.ReadOnly,
		},
	}, nil
}

func (d *AzureFilesEphemeralDriver) PreExec(ctx context.Context, vol core.SharedVolumeRef, execID, localPath, remotePath string) (map[string][]string, error) {
	return nil, nil
}

func (d *AzureFilesEphemeralDriver) PostExec(ctx context.Context, vol core.SharedVolumeRef, execID, localPath string) error {
	return nil
}
