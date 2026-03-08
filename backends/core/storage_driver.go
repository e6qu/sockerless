package core

import "context"

// StorageDriver provides cloud-native volume mount capabilities.
// Each cloud provider implements this with its own storage service:
//   - AWS: EBS volumes, EFS file systems
//   - GCP: Persistent Disks, Cloud Storage FUSE (gcsfuse)
//   - Azure: Azure Files, Azure Disks
//
// The default NoOpStorageDriver is used when no cloud storage is configured.
type StorageDriver interface {
	// CreateVolume provisions a cloud storage resource for use as a container mount.
	// Returns a mount spec that the backend can pass to the cloud container service.
	CreateVolume(ctx context.Context, opts VolumeCreateOptions) (*CloudVolume, error)

	// DeleteVolume removes a cloud storage resource.
	DeleteVolume(ctx context.Context, volumeID string) error

	// MountSpec returns the cloud-specific mount configuration for a volume,
	// suitable for passing to the cloud container/job API (e.g., ECS volume
	// configuration, Cloud Run volume mount, ACA volume mount).
	MountSpec(ctx context.Context, volumeID string, mountPath string) (any, error)

	// DriverName returns the storage driver name (e.g., "ebs", "gcsfuse", "azure-files").
	DriverName() string
}

// VolumeCreateOptions specifies parameters for creating a cloud volume.
type VolumeCreateOptions struct {
	Name     string            // Volume name
	SizeGB   int               // Requested size in GB (0 = provider default)
	Labels   map[string]string // Labels/tags for the cloud resource
	ReadOnly bool              // Whether the volume should be read-only
}

// CloudVolume represents a provisioned cloud storage resource.
type CloudVolume struct {
	ID         string // Cloud resource ID (e.g., EBS volume ID, PD name)
	Name       string // User-facing name
	DriverName string // Storage driver that created this volume
	SizeGB     int    // Actual provisioned size
}

// NoOpStorageDriver is the default StorageDriver that performs no cloud operations.
// Used when no cloud storage integration is configured.
type NoOpStorageDriver struct{}

func (d *NoOpStorageDriver) CreateVolume(_ context.Context, opts VolumeCreateOptions) (*CloudVolume, error) {
	return &CloudVolume{
		ID:         opts.Name,
		Name:       opts.Name,
		DriverName: "local",
	}, nil
}

func (d *NoOpStorageDriver) DeleteVolume(_ context.Context, _ string) error {
	return nil
}

func (d *NoOpStorageDriver) MountSpec(_ context.Context, _ string, _ string) (any, error) {
	return nil, nil
}

func (d *NoOpStorageDriver) DriverName() string {
	return "local"
}
