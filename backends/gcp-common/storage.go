package gcpcommon

import "context"

// GCSFuseStorageDriver provides GCS FUSE volume mounts for Cloud Run containers.
// TODO: Implement with GCS SDK calls when cloud-native volumes are needed.
type GCSFuseStorageDriver struct{}

func (d *GCSFuseStorageDriver) CreateVolume(_ context.Context, _ any) (any, error) {
	return nil, nil
}

func (d *GCSFuseStorageDriver) DeleteVolume(_ context.Context, _ string) error {
	return nil
}

func (d *GCSFuseStorageDriver) MountSpec(_ context.Context, _ string, _ string) (any, error) {
	return nil, nil
}

func (d *GCSFuseStorageDriver) DriverName() string {
	return "gcsfuse"
}

// PersistentDiskStorageDriver provides GCE Persistent Disk mounts.
// TODO: Implement with Compute Engine SDK calls when block storage is needed.
type PersistentDiskStorageDriver struct{}

func (d *PersistentDiskStorageDriver) DriverName() string {
	return "persistent-disk"
}
