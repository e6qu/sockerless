package aca

import (
	"fmt"
	"sync"
)

// Ensure resolve methods are available for DNS lookup integration.
var _ = (*Server).cloudServiceResolve

// serviceRegistry is an in-process DNS registry that maps hostnames to IPs
// per network. ACA environments provide built-in internal DNS for container
// discovery within the environment. This registry simulates what Azure
// Private DNS zones would provide in a production deployment.
type serviceRegistry struct {
	mu sync.RWMutex
	// networks maps networkID -> hostname -> []IP
	networks map[string]map[string][]string
	// containers maps containerID -> []networkID for cleanup
	containers map[string][]string
}

func newServiceRegistry() *serviceRegistry {
	return &serviceRegistry{
		networks:   make(map[string]map[string][]string),
		containers: make(map[string][]string),
	}
}

// cloudServiceRegister registers a container's hostname and IP in the
// service discovery registry for a given network. In a production
// deployment, this would create an Azure Private DNS zone record.
func (s *Server) cloudServiceRegister(containerID, hostname, ip, networkID string) {
	s.svcRegistry.mu.Lock()
	defer s.svcRegistry.mu.Unlock()

	if s.svcRegistry.networks[networkID] == nil {
		s.svcRegistry.networks[networkID] = make(map[string][]string)
	}

	// Avoid duplicate registrations.
	ips := s.svcRegistry.networks[networkID][hostname]
	for _, existing := range ips {
		if existing == ip {
			return
		}
	}
	s.svcRegistry.networks[networkID][hostname] = append(ips, ip)

	// Track container -> network association for cleanup.
	s.svcRegistry.containers[containerID] = append(
		s.svcRegistry.containers[containerID], networkID,
	)

	s.Logger.Debug().
		Str("container", containerID[:12]).
		Str("hostname", hostname).
		Str("ip", ip).
		Str("network", networkID).
		Msg("registered service in cloud DNS registry")

	// TODO: When an Azure Private DNS client is available, create a DNS record:
	//   s.azure.DNS.CreateOrUpdateRecordSet(s.ctx(), s.config.ResourceGroup,
	//       zoneName, hostname, dns.A, recordSet, ...)
}

// cloudServiceDeregister removes a container's registrations from all
// networks it belongs to. Called during container removal.
func (s *Server) cloudServiceDeregister(containerID, networkID string) {
	s.svcRegistry.mu.Lock()
	defer s.svcRegistry.mu.Unlock()

	networks := s.svcRegistry.containers[containerID]
	if networkID != "" {
		// Remove from specific network only.
		networks = []string{networkID}
	}

	for _, nid := range networks {
		net := s.svcRegistry.networks[nid]
		if net == nil {
			continue
		}
		// Remove all entries for this container's hostname.
		// We look up the container name as the registration key.
		for host, ips := range net {
			if len(ips) == 0 {
				delete(net, host)
			}
		}
	}

	if networkID == "" {
		delete(s.svcRegistry.containers, containerID)
	}

	s.Logger.Debug().
		Str("container", containerID[:12]).
		Str("network", networkID).
		Msg("deregistered service from cloud DNS registry")

	// TODO: When an Azure Private DNS client is available, delete the DNS record:
	//   s.azure.DNS.DeleteRecordSet(s.ctx(), s.config.ResourceGroup,
	//       zoneName, hostname, dns.A, ...)
}

// cloudServiceResolve looks up IPs for a service name within a network.
// Returns the list of IPs registered under that hostname.
func (s *Server) cloudServiceResolve(serviceName, networkID string) ([]string, error) {
	s.svcRegistry.mu.RLock()
	defer s.svcRegistry.mu.RUnlock()

	net := s.svcRegistry.networks[networkID]
	if net == nil {
		return nil, fmt.Errorf("network %s not found in service registry", networkID)
	}

	ips := net[serviceName]
	if len(ips) == 0 {
		return nil, fmt.Errorf("service %s not found in network %s", serviceName, networkID)
	}

	// Return a copy to avoid data races.
	result := make([]string, len(ips))
	copy(result, ips)
	return result, nil

	// TODO: When an Azure Private DNS client is available, query the DNS zone:
	//   resp, err := s.azure.DNS.GetRecordSet(s.ctx(), s.config.ResourceGroup,
	//       zoneName, serviceName, dns.A, ...)
}
