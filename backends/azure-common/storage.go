package azurecommon

import "context"

// AzureFilesStorageDriver provides Azure Files volume mounts for containers.
// TODO: Implement with Azure Storage SDK calls when cloud-native volumes are needed.
type AzureFilesStorageDriver struct{}

func (d *AzureFilesStorageDriver) CreateVolume(_ context.Context, _ any) (any, error) {
	return nil, nil
}

func (d *AzureFilesStorageDriver) DeleteVolume(_ context.Context, _ string) error {
	return nil
}

func (d *AzureFilesStorageDriver) MountSpec(_ context.Context, _ string, _ string) (any, error) {
	return nil, nil
}

func (d *AzureFilesStorageDriver) DriverName() string {
	return "azure-files"
}

// AzureDiskStorageDriver provides Azure Managed Disk mounts.
// TODO: Implement with Azure Compute SDK calls when block storage is needed.
type AzureDiskStorageDriver struct{}

func (d *AzureDiskStorageDriver) DriverName() string {
	return "azure-disk"
}
