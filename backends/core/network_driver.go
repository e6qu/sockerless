package core

import "context"

// CloudNetworkDriver provides cloud-native networking for containers running
// on managed cloud services. It maps Docker network operations to cloud
// networking primitives (VPCs, security groups, firewall rules, etc.).
//
// Each cloud provider implements this with its own networking stack:
//   - AWS: VPC + Security Groups + ENI attachments (ECS awsvpc mode)
//   - GCP: VPC Network + Firewall Rules + Serverless VPC Connectors
//   - Azure: VNet + NSG + Container Apps Environment networking
//
// The default NoOpCloudNetworkDriver uses the core SyntheticNetworkDriver
// for IP allocation and does no cloud-level network configuration.
// Cloud backends can plug in their cloud-native driver to create real
// network isolation between containers.
type CloudNetworkDriver interface {
	// EnsureNetwork creates or validates the cloud network infrastructure
	// (VPC, subnet, security group) for a Docker network.
	// Returns cloud-specific metadata to store with the network.
	EnsureNetwork(ctx context.Context, opts CloudNetworkOpts) (map[string]string, error)

	// DeleteNetwork tears down cloud network infrastructure for a Docker network.
	DeleteNetwork(ctx context.Context, networkID string) error

	// AttachContainer connects a container to a cloud network, returning
	// the cloud-assigned IP and any cloud-specific endpoint metadata.
	AttachContainer(ctx context.Context, networkID, containerID string) (*CloudEndpoint, error)

	// DetachContainer disconnects a container from a cloud network.
	DetachContainer(ctx context.Context, networkID, containerID string) error

	// DriverName returns the cloud network driver name
	// (e.g., "aws-vpc", "gcp-vpc", "azure-vnet").
	DriverName() string
}

// CloudNetworkOpts describes a Docker network to be mapped to cloud infrastructure.
type CloudNetworkOpts struct {
	NetworkID string            // Docker network ID
	Name      string            // Docker network name
	Subnet    string            // CIDR block (e.g., "10.0.0.0/24")
	Labels    map[string]string // Network labels
}

// CloudEndpoint describes a container's attachment to a cloud network.
type CloudEndpoint struct {
	IPAddress  string            // Cloud-assigned IP address
	ResourceID string            // Cloud resource ID (ENI, VPC connector, etc.)
	Metadata   map[string]string // Additional cloud-specific metadata
}

// NoOpCloudNetworkDriver is the default that performs no cloud operations.
// Network operations fall back to the core SyntheticNetworkDriver.
type NoOpCloudNetworkDriver struct{}

func (d *NoOpCloudNetworkDriver) EnsureNetwork(_ context.Context, _ CloudNetworkOpts) (map[string]string, error) {
	return nil, nil
}

func (d *NoOpCloudNetworkDriver) DeleteNetwork(_ context.Context, _ string) error {
	return nil
}

func (d *NoOpCloudNetworkDriver) AttachContainer(_ context.Context, _, _ string) (*CloudEndpoint, error) {
	return nil, nil
}

func (d *NoOpCloudNetworkDriver) DetachContainer(_ context.Context, _, _ string) error {
	return nil
}

func (d *NoOpCloudNetworkDriver) DriverName() string {
	return "local"
}
