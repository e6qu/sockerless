package core

import "context"

// ServiceDiscoveryDriver provides cloud-native service discovery for container
// networking, enabling the equivalent of Docker "services" (DNS-based resolution
// of container names to IPs within a network).
//
// Each cloud provider implements this with its own service discovery mechanism:
//   - AWS: AWS Cloud Map (service discovery for ECS)
//   - GCP: Cloud DNS private zones or Cloud Run service URLs
//   - Azure: Azure DNS private zones or Container Apps internal ingress
//
// The default NoOpServiceDiscoveryDriver is used when no cloud service
// discovery is configured — containers use simulated IPs from the core
// network driver.
type ServiceDiscoveryDriver interface {
	// Register creates a service discovery entry for a container, mapping
	// its hostname (and optional aliases) to an address resolvable by
	// sibling containers in the same network.
	Register(ctx context.Context, opts ServiceRegistration) error

	// Deregister removes a container's service discovery entry.
	Deregister(ctx context.Context, containerID string) error

	// Resolve looks up a service name and returns the address(es) for it.
	// Returns an empty slice if the name is not registered.
	Resolve(ctx context.Context, serviceName string) ([]string, error)

	// DriverName returns the service discovery driver name
	// (e.g., "cloud-map", "cloud-dns", "azure-dns").
	DriverName() string
}

// ServiceRegistration describes a container to register for service discovery.
type ServiceRegistration struct {
	ContainerID string   // Container ID
	Hostname    string   // Primary hostname for the container
	Aliases     []string // Additional DNS names that resolve to this container
	IPAddress   string   // IP address to register
	NetworkID   string   // Network in which this registration is visible
}

// NoOpServiceDiscoveryDriver is the default that performs no cloud operations.
// Service resolution falls back to the core network driver's simulated IPs.
type NoOpServiceDiscoveryDriver struct{}

func (d *NoOpServiceDiscoveryDriver) Register(_ context.Context, _ ServiceRegistration) error {
	return nil
}

func (d *NoOpServiceDiscoveryDriver) Deregister(_ context.Context, _ string) error {
	return nil
}

func (d *NoOpServiceDiscoveryDriver) Resolve(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}

func (d *NoOpServiceDiscoveryDriver) DriverName() string {
	return "local"
}
