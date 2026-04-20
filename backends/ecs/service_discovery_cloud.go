package ecs

import (
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/servicediscovery"
	sdtypes "github.com/aws/aws-sdk-go-v2/service/servicediscovery/types"
	"github.com/sockerless/api"
)

// Ensure resolve methods are available for DNS lookup integration.
var _ = (*Server).cloudServiceResolve

// cloudNamespaceCreate creates a Cloud Map private DNS namespace for a
// Docker network. Services within the namespace are created on demand,
// one per container hostname, so other containers can resolve each
// other by short name via the namespace as a DNS search domain.
func (s *Server) cloudNamespaceCreate(name, networkID string) error {
	// Resolve VPC ID for private DNS namespace.
	vpcID, err := s.resolveVPCID()
	if err != nil {
		return fmt.Errorf("failed to resolve VPC ID for namespace: %w", err)
	}

	nsName := "skls-" + name + ".local"

	nsOut, err := s.aws.ServiceDiscovery.CreatePrivateDnsNamespace(s.ctx(),
		&servicediscovery.CreatePrivateDnsNamespaceInput{
			Name:        aws.String(nsName),
			Vpc:         aws.String(vpcID),
			Description: aws.String(fmt.Sprintf("Sockerless network: %s", name)),
		},
	)
	if err != nil {
		return fmt.Errorf("failed to create namespace %s: %w", nsName, err)
	}

	// CreatePrivateDnsNamespace returns an OperationId. Poll for the
	// namespace ID via GetOperation.
	nsID, err := s.waitForOperation(aws.ToString(nsOut.OperationId))
	if err != nil {
		return fmt.Errorf("namespace creation failed: %w", err)
	}

	s.Logger.Debug().
		Str("network", name).
		Str("namespace", nsID).
		Msg("created Cloud Map namespace")

	s.NetworkState.Update(networkID, func(ns *NetworkState) {
		ns.NamespaceID = nsID
	})

	return nil
}

// cloudNamespaceDelete removes the Cloud Map namespace and its service
// for the given network.
func (s *Server) cloudNamespaceDelete(networkID string) error {
	ns, ok := s.NetworkState.Get(networkID)
	if !ok || ns.NamespaceID == "" {
		return nil
	}

	// List and delete all services in the namespace first.
	listOut, err := s.aws.ServiceDiscovery.ListServices(s.ctx(),
		&servicediscovery.ListServicesInput{
			Filters: []sdtypes.ServiceFilter{
				{
					Name:      sdtypes.ServiceFilterNameNamespaceId,
					Values:    []string{ns.NamespaceID},
					Condition: sdtypes.FilterConditionEq,
				},
			},
		},
	)
	if err == nil {
		for _, svc := range listOut.Services {
			_, _ = s.aws.ServiceDiscovery.DeleteService(s.ctx(),
				&servicediscovery.DeleteServiceInput{
					Id: svc.Id,
				},
			)
		}
	}

	// Delete the namespace.
	_, err = s.aws.ServiceDiscovery.DeleteNamespace(s.ctx(),
		&servicediscovery.DeleteNamespaceInput{
			Id: aws.String(ns.NamespaceID),
		},
	)
	if err != nil {
		return fmt.Errorf("failed to delete namespace %s: %w", ns.NamespaceID, err)
	}

	s.Logger.Debug().
		Str("namespace", ns.NamespaceID).
		Msg("deleted Cloud Map namespace")

	s.NetworkState.Update(networkID, func(ns *NetworkState) {
		ns.NamespaceID = ""
	})

	return nil
}

// cloudServiceRegister registers a container instance in Cloud Map so
// other containers on the same network can resolve it by hostname.
// A Cloud Map service per hostname is created on demand — service name
// "postgres" in namespace "skls-foo.local" gives DNS name
// "postgres.skls-foo.local" → instance IP. Containers add the namespace
// as a DNS search domain (see DnsSearchDomains on the task def), so a
// bare "postgres" lookup resolves.
func (s *Server) cloudServiceRegister(containerID, hostname, ip, networkID string) error {
	ns, ok := s.NetworkState.Get(networkID)
	if !ok || ns.NamespaceID == "" {
		return fmt.Errorf("network %s has no Cloud Map namespace", networkID)
	}

	serviceID, err := s.findOrCreateServiceForHostname(ns.NamespaceID, hostname)
	if err != nil {
		return err
	}

	_, err = s.aws.ServiceDiscovery.RegisterInstance(s.ctx(),
		&servicediscovery.RegisterInstanceInput{
			ServiceId:  aws.String(serviceID),
			InstanceId: aws.String(containerID[:12]),
			Attributes: map[string]string{
				"AWS_INSTANCE_IPV4": ip,
			},
		},
	)
	if err != nil {
		return fmt.Errorf("failed to register instance %s: %w", hostname, err)
	}

	s.ECS.Update(containerID, func(state *ECSState) {
		state.ServiceID = serviceID
	})

	s.Logger.Debug().
		Str("hostname", hostname).
		Str("ip", ip).
		Str("container", containerID[:12]).
		Str("service", serviceID).
		Msg("registered container in Cloud Map")

	return nil
}

// cloudServiceDeregister removes a container's Cloud Map instance and,
// if the service has no other instances, deletes the service itself.
func (s *Server) cloudServiceDeregister(containerID, networkID string) error {
	ecsState, ok := s.ECS.Get(containerID)
	if !ok || ecsState.ServiceID == "" {
		return nil
	}

	serviceID := ecsState.ServiceID

	_, err := s.aws.ServiceDiscovery.DeregisterInstance(s.ctx(),
		&servicediscovery.DeregisterInstanceInput{
			ServiceId:  aws.String(serviceID),
			InstanceId: aws.String(containerID[:12]),
		},
	)
	if err != nil {
		return fmt.Errorf("failed to deregister instance %s: %w", containerID[:12], err)
	}

	s.ECS.Update(containerID, func(state *ECSState) {
		state.ServiceID = ""
	})

	// Best-effort: delete the service if empty so the DNS name is reclaimed.
	if empty, err := s.serviceHasNoInstances(serviceID); err == nil && empty {
		_, _ = s.aws.ServiceDiscovery.DeleteService(s.ctx(),
			&servicediscovery.DeleteServiceInput{Id: aws.String(serviceID)},
		)
	}

	s.Logger.Debug().
		Str("container", containerID[:12]).
		Str("service", serviceID).
		Msg("deregistered container from Cloud Map")

	return nil
}

// cloudServiceResolve discovers IPs for a service name within a network's
// Cloud Map namespace.
func (s *Server) cloudServiceResolve(serviceName, networkID string) ([]string, error) {
	ns, ok := s.NetworkState.Get(networkID)
	if !ok || ns.NamespaceID == "" {
		return nil, fmt.Errorf("network %s has no Cloud Map namespace", networkID)
	}

	nsName := s.getNamespaceName(ns.NamespaceID)

	result, err := s.aws.ServiceDiscovery.DiscoverInstances(s.ctx(),
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

// waitForOperation polls a Cloud Map operation until it completes and
// returns the target resource ID (e.g. namespace ID).
func (s *Server) waitForOperation(operationID string) (string, error) {
	for i := 0; i < 60; i++ {
		result, err := s.aws.ServiceDiscovery.GetOperation(s.ctx(),
			&servicediscovery.GetOperationInput{
				OperationId: aws.String(operationID),
			},
		)
		if err != nil {
			return "", err
		}
		if result.Operation == nil {
			continue
		}

		switch result.Operation.Status {
		case sdtypes.OperationStatusSuccess:
			// Targets is map[OperationTargetType]string
			for k, v := range result.Operation.Targets {
				if string(k) == string(sdtypes.OperationTargetTypeNamespace) {
					return v, nil
				}
			}
			return "", fmt.Errorf("operation succeeded but no namespace target found")
		case sdtypes.OperationStatusFail:
			msg := aws.ToString(result.Operation.ErrorMessage)
			return "", fmt.Errorf("operation failed: %s", msg)
		}
		// SUBMITTED or PENDING — keep polling.
	}
	return "", fmt.Errorf("timeout waiting for operation %s", operationID)
}

// findOrCreateServiceForHostname returns the service ID for the given
// hostname in the namespace, creating a new Cloud Map service if none
// exists. Each hostname gets its own A-record service so cross-container
// DNS resolves per-name.
func (s *Server) findOrCreateServiceForHostname(namespaceID, hostname string) (string, error) {
	listOut, err := s.aws.ServiceDiscovery.ListServices(s.ctx(),
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

	svcOut, err := s.aws.ServiceDiscovery.CreateService(s.ctx(),
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

// serviceHasNoInstances reports whether a Cloud Map service has zero
// registered instances (used to decide whether to delete a service on
// the last deregistration).
func (s *Server) serviceHasNoInstances(serviceID string) (bool, error) {
	out, err := s.aws.ServiceDiscovery.ListInstances(s.ctx(),
		&servicediscovery.ListInstancesInput{
			ServiceId: aws.String(serviceID),
		},
	)
	if err != nil {
		return false, err
	}
	return len(out.Instances) == 0, nil
}

// searchDomainsForContainer collects the Cloud Map namespace names
// (one per attached user-defined network) so they can be set as DNS
// search domains on the ECS task definition. Pre-defined Docker
// networks (bridge/host/none) are skipped — they have no namespace.
func (s *Server) searchDomainsForContainer(c *api.Container) []string {
	if c == nil {
		return nil
	}
	var domains []string
	seen := make(map[string]struct{})
	for netName, ep := range c.NetworkSettings.Networks {
		if ep == nil || ep.NetworkID == "" {
			continue
		}
		if netName == "bridge" || netName == "host" || netName == "none" {
			continue
		}
		ns, ok := s.NetworkState.Get(ep.NetworkID)
		if !ok || ns.NamespaceID == "" {
			continue
		}
		nsName := s.getNamespaceName(ns.NamespaceID)
		if nsName == "" {
			continue
		}
		if _, dup := seen[nsName]; dup {
			continue
		}
		seen[nsName] = struct{}{}
		domains = append(domains, nsName)
	}
	return domains
}

// getNamespaceName fetches the namespace name from Cloud Map by ID.
func (s *Server) getNamespaceName(namespaceID string) string {
	result, err := s.aws.ServiceDiscovery.GetNamespace(s.ctx(),
		&servicediscovery.GetNamespaceInput{
			Id: aws.String(namespaceID),
		},
	)
	if err != nil || result.Namespace == nil {
		return namespaceID // fallback to ID
	}
	return aws.ToString(result.Namespace.Name)
}
