package aca

import (
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v7"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/privatedns/armprivatedns"
)

// cloudNetworkCreate sets up cloud networking state for a Docker
// network: a per-network Azure Private DNS zone for service discovery,
// a per-network Azure NSG for cross-container isolation, and state
// tracking tying the two together.
// The zone name is `skls-<name>.local`, matching the convention used
// by the ECS + Cloud Run backends for parity.
func (s *Server) cloudNetworkCreate(name, networkID string) error {
	zoneName := fmt.Sprintf("skls-%s.local", name)
	nsgName := fmt.Sprintf("nsg-%s-%s", s.config.Environment, name)

	// Private DNS zone.
	zonePoller, err := s.azure.PrivateDNSZones.BeginCreateOrUpdate(
		s.ctx(),
		s.config.ResourceGroup,
		zoneName,
		armprivatedns.PrivateZone{Location: to.Ptr("global")},
		nil,
	)
	if err != nil {
		return fmt.Errorf("create Private DNS zone %s: %w", zoneName, err)
	}
	if _, err := zonePoller.PollUntilDone(s.ctx(), nil); err != nil {
		return fmt.Errorf("wait Private DNS zone %s: %w", zoneName, err)
	}

	// NSG. Create an empty NSG; rules are added via
	// cloudNetworkAddRule when containers connect.
	nsgPoller, err := s.azure.NSG.BeginCreateOrUpdate(
		s.ctx(),
		s.config.ResourceGroup,
		nsgName,
		armnetwork.SecurityGroup{Location: to.Ptr(s.config.Location)},
		nil,
	)
	if err != nil {
		return fmt.Errorf("create NSG %s: %w", nsgName, err)
	}
	if _, err := nsgPoller.PollUntilDone(s.ctx(), nil); err != nil {
		return fmt.Errorf("wait NSG %s: %w", nsgName, err)
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
		Msg("created cloud network state with Private DNS zone + NSG")

	return nil
}

// cloudNetworkDelete removes cloud networking state for a Docker
// network — deletes the NSG (with its rules) and the Private DNS zone.
func (s *Server) cloudNetworkDelete(networkID string) error {
	state, ok := s.NetworkState.Get(networkID)
	if !ok {
		return nil
	}

	// Tear down NSG — deleting the NSG cascades its rules.
	if state.NSGName != "" {
		nsgPoller, err := s.azure.NSG.BeginDelete(
			s.ctx(),
			s.config.ResourceGroup,
			state.NSGName,
			nil,
		)
		if err != nil {
			s.Logger.Warn().Err(err).
				Str("nsg", state.NSGName).
				Msg("begin delete NSG failed")
		} else if _, err := nsgPoller.PollUntilDone(s.ctx(), nil); err != nil {
			s.Logger.Warn().Err(err).
				Str("nsg", state.NSGName).
				Msg("poll delete NSG failed")
		}
	}

	// Tear down DNS zone.
	if state.DNSZoneName != "" {
		zonePoller, err := s.azure.PrivateDNSZones.BeginDelete(
			s.ctx(),
			s.config.ResourceGroup,
			state.DNSZoneName,
			nil,
		)
		if err != nil {
			s.Logger.Warn().Err(err).
				Str("zone", state.DNSZoneName).
				Msg("begin delete Private DNS zone failed")
		} else if _, err := zonePoller.PollUntilDone(s.ctx(), nil); err != nil {
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
		Msg("deleted cloud network state, NSG, and DNS zone")

	s.NetworkState.Delete(networkID)
	return nil
}

// cloudNetworkAddRule creates an NSG allow rule for a container joining
// the network, via the real `armnetwork.SecurityRulesClient`.
// Rule priority is derived from the number of existing rules + 100
// (Azure requires priorities between 100-4096).
func (s *Server) cloudNetworkAddRule(networkID, ruleName string) error {
	state, ok := s.NetworkState.Get(networkID)
	if !ok || state.NSGName == "" {
		return nil
	}

	priority := int32(100 + len(state.NSGRuleNames))
	rule := armnetwork.SecurityRule{
		Properties: &armnetwork.SecurityRulePropertiesFormat{
			Protocol:                 to.Ptr(armnetwork.SecurityRuleProtocolAsterisk),
			SourcePortRange:          to.Ptr("*"),
			DestinationPortRange:     to.Ptr("*"),
			SourceAddressPrefix:      to.Ptr("VirtualNetwork"),
			DestinationAddressPrefix: to.Ptr("VirtualNetwork"),
			Access:                   to.Ptr(armnetwork.SecurityRuleAccessAllow),
			Priority:                 to.Ptr(priority),
			Direction:                to.Ptr(armnetwork.SecurityRuleDirectionInbound),
		},
	}

	poller, err := s.azure.NSGRules.BeginCreateOrUpdate(
		s.ctx(),
		s.config.ResourceGroup,
		state.NSGName,
		ruleName,
		rule,
		nil,
	)
	if err != nil {
		return fmt.Errorf("create NSG rule %s: %w", ruleName, err)
	}
	if _, err := poller.PollUntilDone(s.ctx(), nil); err != nil {
		return fmt.Errorf("wait NSG rule %s: %w", ruleName, err)
	}

	s.NetworkState.Update(networkID, func(state *NetworkState) {
		state.NSGRuleNames = append(state.NSGRuleNames, ruleName)
	})
	return nil
}
