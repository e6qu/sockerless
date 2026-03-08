package awscommon

import "context"

// EBSStorageDriver provides AWS EBS volume management for container mounts.
// TODO: Implement with AWS EBS SDK calls when cloud-native volumes are needed.
type EBSStorageDriver struct{}

func (d *EBSStorageDriver) CreateVolume(_ context.Context, opts any) (any, error) {
	return nil, nil
}

func (d *EBSStorageDriver) DeleteVolume(_ context.Context, _ string) error {
	return nil
}

func (d *EBSStorageDriver) MountSpec(_ context.Context, _ string, _ string) (any, error) {
	return nil, nil
}

func (d *EBSStorageDriver) DriverName() string {
	return "ebs"
}

// EFSStorageDriver provides AWS EFS file system mounts for containers.
// TODO: Implement with AWS EFS SDK calls when shared file system mounts are needed.
type EFSStorageDriver struct{}

func (d *EFSStorageDriver) DriverName() string {
	return "efs"
}
