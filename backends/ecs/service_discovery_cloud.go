package ecs

import (
	"fmt"
	"time"

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
// Idempotent: if a namespace with the same name already
// exists in the same VPC, reuse it instead of erroring.
func (s *Server) cloudNamespaceCreate(name, networkID string) error {
	// Resolve VPC ID for private DNS namespace.
	vpcID, err := s.resolveVPCID()
	if err != nil {
		return fmt.Errorf("failed to resolve VPC ID for namespace: %w", err)
	}

	nsName := "skls-" + name + ".local"

	if existing, err := s.findNamespaceByName(nsName); err == nil && existing != "" {
		s.Logger.Info().Str("network", name).Str("namespace", existing).Msg("reusing existing Cloud Map namespace")
		s.NetworkState.Update(networkID, func(ns *NetworkState) {
			ns.NamespaceID = existing
		})
		return nil
	}

	nsOut, err := s.aws.ServiceDiscovery.CreatePrivateDnsNamespace(s.ctx(),
		&servicediscovery.CreatePrivateDnsNamespaceInput{
			Name:        aws.String(nsName),
			Vpc:         aws.String(vpcID),
			Description: aws.String(fmt.Sprintf("Sockerless network: %s", name)),
			Tags: []sdtypes.Tag{
				// tag with network-id so resolveNetworkState
				// can recover NamespaceID from cloud after restart.
				{Key: aws.String("sockerless:network-id"), Value: aws.String(networkID)},
				{Key: aws.String("sockerless:network"), Value: aws.String(name)},
				{Key: aws.String("sockerless-managed"), Value: aws.String("true")},
			},
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

// findNamespaceByName looks up a Cloud Map namespace by exact name and
// returns its ID, or "" if not found. Used for idempotent creation
func (s *Server) findNamespaceByName(name string) (string, error) {
	listOut, err := s.aws.ServiceDiscovery.ListNamespaces(s.ctx(),
		&servicediscovery.ListNamespacesInput{
			Filters: []sdtypes.NamespaceFilter{
				{
					Name:      sdtypes.NamespaceFilterNameType,
					Values:    []string{string(sdtypes.NamespaceTypeDnsPrivate)},
					Condition: sdtypes.FilterConditionEq,
				},
			},
		},
	)
	if err != nil {
		return "", err
	}
	for _, ns := range listOut.Namespaces {
		if aws.ToString(ns.Name) == name {
			return aws.ToString(ns.Id), nil
		}
	}
	return "", nil
}

// waitForOperation polls a Cloud Map operation until it completes and
// returns the target resource ID (e.g. namespace ID). Real Cloud Map
// namespace creation needs ~30-60s; sleep between polls so 60 iterations
// span 120s of wall time, not 60 back-to-back API calls in <10s.
func (s *Server) waitForOperation(operationID string) (string, error) {
	return pollOperation(operationID, 2*time.Second, 60, time.Sleep, func(id string) (sdtypes.OperationStatus, string, error) {
		result, err := s.aws.ServiceDiscovery.GetOperation(s.ctx(),
			&servicediscovery.GetOperationInput{OperationId: aws.String(id)},
		)
		if err != nil {
			return "", "", err
		}
		if result.Operation == nil {
			return "", "", nil
		}
		var nsTarget string
		for k, v := range result.Operation.Targets {
			if string(k) == string(sdtypes.OperationTargetTypeNamespace) {
				nsTarget = v
			}
		}
		if result.Operation.Status == sdtypes.OperationStatusFail {
			return result.Operation.Status, aws.ToString(result.Operation.ErrorMessage), nil
		}
		return result.Operation.Status, nsTarget, nil
	})
}

// pollOperation drives a status loop without any AWS dependency so its
// timing + termination behaviour is unit-testable. Callbacks:
// - sleep: invoked between polls (use time.Sleep in production; a
// counter in tests)
// - poll: returns (status, payload, err) where payload is the
// namespace ID on SUCCESS or the error message on FAIL.
// Returns the namespace ID on SUCCESS, an error on FAIL/exhaustion.
func pollOperation(
	operationID string,
	interval time.Duration,
	maxAttempts int,
	sleep func(time.Duration),
	poll func(string) (sdtypes.OperationStatus, string, error),
) (string, error) {
	for i := 0; i < maxAttempts; i++ {
		status, payload, err := poll(operationID)
		if err != nil {
			return "", err
		}
		switch status {
		case sdtypes.OperationStatusSuccess:
			if payload == "" {
				return "", fmt.Errorf("operation succeeded but no namespace target found")
			}
			return payload, nil
		case sdtypes.OperationStatusFail:
			return "", fmt.Errorf("operation failed: %s", payload)
		}
		sleep(interval)
	}
	return "", fmt.Errorf("timeout waiting for operation %s after %s", operationID, time.Duration(maxAttempts)*interval)
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
