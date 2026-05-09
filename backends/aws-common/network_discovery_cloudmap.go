// service-mesh network-discovery driver shared by every AWS-product
// backend. Pattern B — backend-specific NetworkState lookups arrive
// via callbacks captured at construction; the driver itself owns the
// AWS Cloud Map (servicediscovery) SDK calls.
//
// Per-backend wiring: backend startup constructs CloudMapDiscovery
// with the servicediscovery client, logger, and four callbacks
// (network → namespace ID, container → service ID get/set, and a
// container-ID → short-form mapper since AWS Cloud Map instance IDs
// are bounded to 64 chars).

package awscommon

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/servicediscovery"
	sdtypes "github.com/aws/aws-sdk-go-v2/service/servicediscovery/types"
	"github.com/rs/zerolog"
	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
)

// CloudMapDiscoveryConfig is the constructor arg.
type CloudMapDiscoveryConfig struct {
	ServiceDiscovery *servicediscovery.Client
	Logger           zerolog.Logger
	// GetNetworkNamespaceID returns the Cloud Map namespace ID for the
	// given network (false when no namespace is registered).
	GetNetworkNamespaceID func(networkID string) (namespaceID string, ok bool)
	// GetContainerServiceID returns the Cloud Map service ID associated
	// with the container (false when not yet registered).
	GetContainerServiceID func(containerID string) (serviceID string, ok bool)
	// SetContainerServiceID records the container → service binding so
	// deregister can find which service to deregister from.
	SetContainerServiceID func(containerID, serviceID string)
}

// CloudMapDiscovery satisfies core.NetworkDiscoveryDriver via AWS
// Cloud Map (servicediscovery).
type CloudMapDiscovery struct {
	cfg CloudMapDiscoveryConfig
}

// NewCloudMapDiscovery wraps the per-backend SDK client + state
// callbacks in the discovery-driver shape.
func NewCloudMapDiscovery(cfg CloudMapDiscoveryConfig) *CloudMapDiscovery {
	return &CloudMapDiscovery{cfg: cfg}
}

func (d *CloudMapDiscovery) RegisterContainer(ctx context.Context, networkID, name, containerID string, endpoint *core.CloudEndpoint) error {
	if endpoint == nil {
		return nil
	}
	return d.registerInstance(ctx, containerID, name, endpoint.IPAddress, networkID)
}

// DeregisterContainer uses the cached container → serviceID mapping
// (Cloud Map keys instances by container-ID at register time).
func (d *CloudMapDiscovery) DeregisterContainer(ctx context.Context, networkID, name, containerID string) error {
	return d.deregisterInstance(ctx, containerID, networkID)
}

func (d *CloudMapDiscovery) ResolveName(ctx context.Context, networkID, name string) (*core.CloudEndpoint, error) {
	ips, err := d.resolveInstances(ctx, name, networkID)
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, nil
	}
	return &core.CloudEndpoint{IPAddress: ips[0]}, nil
}

func (d *CloudMapDiscovery) Kind() api.NetworkDiscoveryKind {
	return api.NetworkDiscoveryServiceMesh
}

// registerInstance creates a Cloud Map service for the hostname (if it
// doesn't exist yet in the network's namespace) and registers the
// container as an instance with its IP. Each hostname gets its own
// per-network A-record service so cross-container DNS resolves
// predictably (e.g. `postgres.skls-foo.local` → instance IP).
func (d *CloudMapDiscovery) registerInstance(ctx context.Context, containerID, hostname, ip, networkID string) error {
	namespaceID, ok := d.cfg.GetNetworkNamespaceID(networkID)
	if !ok || namespaceID == "" {
		return fmt.Errorf("network %s has no Cloud Map namespace", networkID)
	}

	serviceID, err := d.findOrCreateServiceForHostname(ctx, namespaceID, hostname)
	if err != nil {
		return err
	}

	cidShort := containerID
	if len(cidShort) > 12 {
		cidShort = cidShort[:12]
	}

	_, err = d.cfg.ServiceDiscovery.RegisterInstance(ctx,
		&servicediscovery.RegisterInstanceInput{
			ServiceId:  aws.String(serviceID),
			InstanceId: aws.String(cidShort),
			Attributes: map[string]string{
				"AWS_INSTANCE_IPV4": ip,
			},
		},
	)
	if err != nil {
		return fmt.Errorf("failed to register instance %s: %w", hostname, err)
	}

	d.cfg.SetContainerServiceID(containerID, serviceID)

	d.cfg.Logger.Debug().
		Str("hostname", hostname).
		Str("ip", ip).
		Str("container", cidShort).
		Str("service", serviceID).
		Msg("registered container in Cloud Map")
	return nil
}

func (d *CloudMapDiscovery) deregisterInstance(ctx context.Context, containerID, networkID string) error {
	serviceID, ok := d.cfg.GetContainerServiceID(containerID)
	if !ok || serviceID == "" {
		return nil
	}

	cidShort := containerID
	if len(cidShort) > 12 {
		cidShort = cidShort[:12]
	}

	_, err := d.cfg.ServiceDiscovery.DeregisterInstance(ctx,
		&servicediscovery.DeregisterInstanceInput{
			ServiceId:  aws.String(serviceID),
			InstanceId: aws.String(cidShort),
		},
	)
	if err != nil {
		return fmt.Errorf("failed to deregister instance %s: %w", cidShort, err)
	}

	d.cfg.SetContainerServiceID(containerID, "")

	// Best-effort: delete the service if empty so the DNS name is reclaimed.
	if empty, err := d.serviceHasNoInstances(ctx, serviceID); err == nil && empty {
		_, _ = d.cfg.ServiceDiscovery.DeleteService(ctx,
			&servicediscovery.DeleteServiceInput{Id: aws.String(serviceID)},
		)
	}

	d.cfg.Logger.Debug().
		Str("container", cidShort).
		Str("service", serviceID).
		Msg("deregistered container from Cloud Map")
	return nil
}

func (d *CloudMapDiscovery) resolveInstances(ctx context.Context, serviceName, networkID string) ([]string, error) {
	namespaceID, ok := d.cfg.GetNetworkNamespaceID(networkID)
	if !ok || namespaceID == "" {
		return nil, fmt.Errorf("network %s has no Cloud Map namespace", networkID)
	}

	nsName, err := d.namespaceName(ctx, namespaceID)
	if err != nil {
		return nil, fmt.Errorf("resolve namespace for network %s: %w", networkID, err)
	}

	result, err := d.cfg.ServiceDiscovery.DiscoverInstances(ctx,
		&servicediscovery.DiscoverInstancesInput{
			NamespaceName: aws.String(nsName),
			ServiceName:   aws.String(serviceName),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to discover instances for %s: %w", serviceName, err)
	}

	var ips []string
	for _, inst := range result.Instances {
		if ip, ok := inst.Attributes["AWS_INSTANCE_IPV4"]; ok {
			ips = append(ips, ip)
		}
	}
	return ips, nil
}

func (d *CloudMapDiscovery) findOrCreateServiceForHostname(ctx context.Context, namespaceID, hostname string) (string, error) {
	listOut, err := d.cfg.ServiceDiscovery.ListServices(ctx,
		&servicediscovery.ListServicesInput{
			Filters: []sdtypes.ServiceFilter{
				{
					Name:      sdtypes.ServiceFilterNameNamespaceId,
					Values:    []string{namespaceID},
					Condition: sdtypes.FilterConditionEq,
				},
			},
		},
	)
	if err != nil {
		return "", fmt.Errorf("failed to list services in namespace: %w", err)
	}
	for _, svc := range listOut.Services {
		if aws.ToString(svc.Name) == hostname {
			return aws.ToString(svc.Id), nil
		}
	}

	svcOut, err := d.cfg.ServiceDiscovery.CreateService(ctx,
		&servicediscovery.CreateServiceInput{
			Name:        aws.String(hostname),
			NamespaceId: aws.String(namespaceID),
			DnsConfig: &sdtypes.DnsConfig{
				DnsRecords: []sdtypes.DnsRecord{
					{Type: sdtypes.RecordTypeA, TTL: aws.Int64(10)},
				},
			},
		},
	)
	if err != nil {
		return "", fmt.Errorf("failed to create Cloud Map service %s: %w", hostname, err)
	}
	return aws.ToString(svcOut.Service.Id), nil
}

func (d *CloudMapDiscovery) serviceHasNoInstances(ctx context.Context, serviceID string) (bool, error) {
	out, err := d.cfg.ServiceDiscovery.ListInstances(ctx,
		&servicediscovery.ListInstancesInput{
			ServiceId: aws.String(serviceID),
		},
	)
	if err != nil {
		return false, err
	}
	return len(out.Instances) == 0, nil
}

func (d *CloudMapDiscovery) namespaceName(ctx context.Context, namespaceID string) (string, error) {
	result, err := d.cfg.ServiceDiscovery.GetNamespace(ctx,
		&servicediscovery.GetNamespaceInput{
			Id: aws.String(namespaceID),
		},
	)
	if err != nil {
		return "", fmt.Errorf("GetNamespace(%s): %w", namespaceID, err)
	}
	if result.Namespace == nil || result.Namespace.Name == nil {
		return "", fmt.Errorf("GetNamespace(%s): response missing name", namespaceID)
	}
	return aws.ToString(result.Namespace.Name), nil
}
