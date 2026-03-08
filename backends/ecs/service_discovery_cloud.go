package ecs

import (
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/servicediscovery"
	sdtypes "github.com/aws/aws-sdk-go-v2/service/servicediscovery/types"
)

// Ensure resolve methods are available for DNS lookup integration.
var _ = (*Server).cloudServiceResolve

// cloudNamespaceCreate creates a Cloud Map private DNS namespace for a
// Docker network and a service within it for container registration.
// Stores the namespace ID in NetworkState.
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

	// CreatePrivateDnsNamespace returns an OperationId. We need to poll for
	// the namespace ID via GetOperation.
	nsID, err := s.waitForOperation(aws.ToString(nsOut.OperationId))
	if err != nil {
		return fmt.Errorf("namespace creation failed: %w", err)
	}

	s.Logger.Debug().
		Str("network", name).
		Str("namespace", nsID).
		Msg("created Cloud Map namespace")

	// Create a service within the namespace for container instance registration.
	svcOut, err := s.aws.ServiceDiscovery.CreateService(s.ctx(),
		&servicediscovery.CreateServiceInput{
			Name:        aws.String("containers"),
			NamespaceId: aws.String(nsID),
			DnsConfig: &sdtypes.DnsConfig{
				DnsRecords: []sdtypes.DnsRecord{
					{Type: sdtypes.RecordTypeA, TTL: aws.Int64(10)},
				},
			},
		},
	)
	if err != nil {
		return fmt.Errorf("failed to create Cloud Map service: %w", err)
	}

	serviceID := aws.ToString(svcOut.Service.Id)
	s.Logger.Debug().Str("service", serviceID).Msg("created Cloud Map service")

	// Store namespace and service in network state.
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
// other containers can discover it by hostname.
func (s *Server) cloudServiceRegister(containerID, hostname, ip, networkID string) error {
	ns, ok := s.NetworkState.Get(networkID)
	if !ok || ns.NamespaceID == "" {
		return fmt.Errorf("network %s has no Cloud Map namespace", networkID)
	}

	// Find the service ID for this namespace.
	serviceID, err := s.findServiceInNamespace(ns.NamespaceID)
	if err != nil {
		return err
	}

	_, err = s.aws.ServiceDiscovery.RegisterInstance(s.ctx(),
		&servicediscovery.RegisterInstanceInput{
			ServiceId:  aws.String(serviceID),
			InstanceId: aws.String(containerID[:12]),
			Attributes: map[string]string{
				"AWS_INSTANCE_IPV4": ip,
				"HOSTNAME":         hostname,
			},
		},
	)
	if err != nil {
		return fmt.Errorf("failed to register instance %s: %w", hostname, err)
	}

	// Store service ID on the container for deregistration.
	s.ECS.Update(containerID, func(state *ECSState) {
		state.ServiceID = serviceID
	})

	s.Logger.Debug().
		Str("hostname", hostname).
		Str("ip", ip).
		Str("container", containerID[:12]).
		Msg("registered container in Cloud Map")

	return nil
}

// cloudServiceDeregister removes a container's Cloud Map instance.
func (s *Server) cloudServiceDeregister(containerID, networkID string) error {
	ecsState, ok := s.ECS.Get(containerID)
	if !ok || ecsState.ServiceID == "" {
		return nil
	}

	_, err := s.aws.ServiceDiscovery.DeregisterInstance(s.ctx(),
		&servicediscovery.DeregisterInstanceInput{
			ServiceId:  aws.String(ecsState.ServiceID),
			InstanceId: aws.String(containerID[:12]),
		},
	)
	if err != nil {
		return fmt.Errorf("failed to deregister instance %s: %w", containerID[:12], err)
	}

	s.ECS.Update(containerID, func(state *ECSState) {
		state.ServiceID = ""
	})

	s.Logger.Debug().
		Str("container", containerID[:12]).
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

// findServiceInNamespace returns the first Cloud Map service ID in the
// given namespace.
func (s *Server) findServiceInNamespace(namespaceID string) (string, error) {
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
	if len(listOut.Services) == 0 {
		return "", fmt.Errorf("no services found in namespace %s", namespaceID)
	}
	return aws.ToString(listOut.Services[0].Id), nil
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
