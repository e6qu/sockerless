package aca

import (
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/privatedns/armprivatedns"
)

// cloudNetworkCreate sets up cloud networking state for a Docker
// network: a per-network Azure Private DNS zone for service discovery
// (BUG-702) and an NSG placeholder name for future NSG-rule tracking
// (BUG-703 — still open). The zone name is `skls-<name>.local`, matching
// the convention used by the ECS + Cloud Run backends for parity.
func (s *Server) cloudNetworkCreate(name, networkID string) error {
	zoneName := fmt.Sprintf("skls-%s.local", name)
	nsgName := fmt.Sprintf("nsg-%s-%s", s.config.Environment, name)

	// Create the Private DNS zone (BUG-702 fix). Zone creation is a
	// long-running op in real Azure; begin + poll until done.
	poller, err := s.azure.PrivateDNSZones.BeginCreateOrUpdate(
		s.ctx(),
		s.config.ResourceGroup,
		zoneName,
		armprivatedns.PrivateZone{Location: ptrString("global")},
		nil,
	)
	if err != nil {
		return fmt.Errorf("create Private DNS zone %s: %w", zoneName, err)
	}
	if _, err := poller.PollUntilDone(s.ctx(), nil); err != nil {
		return fmt.Errorf("wait Private DNS zone %s: %w", zoneName, err)
	}

	s.NetworkState.Put(networkID, NetworkState{
		NSGName:      nsgName,
		NSGRuleNames: []string{},
		DNSZoneName:  zoneName,
	})

	s.Logger.Debug().
		Str("network", name).
		Str("networkID", networkID).
		Str("nsg", nsgName).
		Str("zone", zoneName).
		Msg("created cloud network state with Private DNS zone + NSG placeholder")

	return nil

	// TODO (BUG-703): When an Azure Network SDK client is wired up,
	// create an NSG for this network allowing inter-container traffic
	// within the zone's address space:
	//   s.azure.NSG.CreateOrUpdate(s.ctx(), s.config.ResourceGroup, nsgName, ...)
}

// cloudNetworkDelete removes cloud networking state for a Docker
// network. Deletes the Private DNS zone (BUG-702) and clears NSG
// tracking. NSG rule deletion is still a TODO pending BUG-703.
func (s *Server) cloudNetworkDelete(networkID string) error {
	state, ok := s.NetworkState.Get(networkID)
	if !ok {
		return nil
	}

	if state.DNSZoneName != "" {
		poller, err := s.azure.PrivateDNSZones.BeginDelete(
			s.ctx(),
			s.config.ResourceGroup,
			state.DNSZoneName,
			nil,
		)
		if err != nil {
			s.Logger.Warn().Err(err).
				Str("zone", state.DNSZoneName).
				Msg("begin delete Private DNS zone failed")
		} else if _, err := poller.PollUntilDone(s.ctx(), nil); err != nil {
			s.Logger.Warn().Err(err).
				Str("zone", state.DNSZoneName).
				Msg("poll delete Private DNS zone failed")
		}
	}

	s.Logger.Debug().
		Str("networkID", networkID).
		Str("nsg", state.NSGName).
		Str("zone", state.DNSZoneName).
		Int("rules", len(state.NSGRuleNames)).
		Msg("deleting cloud network state, DNS zone, and NSG tracking")

	s.NetworkState.Delete(networkID)
	return nil

	// TODO (BUG-703): delete NSG rules + NSG itself when BUG-703 lands.
}

// cloudNetworkAddRule adds an NSG rule name to the network's tracking
// state. Real NSG-rule creation lands with BUG-703.
func (s *Server) cloudNetworkAddRule(networkID, ruleName string) {
	s.NetworkState.Update(networkID, func(state *NetworkState) {
		state.NSGRuleNames = append(state.NSGRuleNames, ruleName)
	})
	// TODO (BUG-703): real NSG rule creation via armnetwork.
}

func ptrString(s string) *string { return &s }
