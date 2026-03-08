package aca

import (
	"fmt"
)

// cloudNetworkCreate sets up cloud networking state for a Docker network.
// ACA environments provide built-in networking — containers in the same
// environment can communicate. We map Docker network operations to NSG
// rule tracking, which would control inter-container communication in a
// production deployment with Azure NSGs.
func (s *Server) cloudNetworkCreate(name, networkID string) {
	nsgName := fmt.Sprintf("nsg-%s-%s", s.config.Environment, name)

	s.NetworkState.Put(networkID, NetworkState{
		NSGName:      nsgName,
		NSGRuleNames: []string{},
	})

	s.Logger.Debug().
		Str("network", name).
		Str("networkID", networkID).
		Str("nsg", nsgName).
		Msg("created cloud network state with NSG tracking")

	// TODO: When an Azure Network SDK client is available, create an actual
	// NSG rule allowing traffic within the network's subnet:
	//   s.azure.NSG.CreateOrUpdate(s.ctx(), s.config.ResourceGroup, nsgName, ...)
}

// cloudNetworkDelete removes cloud networking state for a Docker network.
// Cleans up NSG rule tracking and would delete actual NSG rules in a
// production deployment.
func (s *Server) cloudNetworkDelete(networkID string) {
	state, ok := s.NetworkState.Get(networkID)
	if !ok {
		return
	}

	s.Logger.Debug().
		Str("networkID", networkID).
		Str("nsg", state.NSGName).
		Int("rules", len(state.NSGRuleNames)).
		Msg("deleting cloud network state and NSG tracking")

	// TODO: When an Azure Network SDK client is available, delete the NSG
	// rules associated with this network:
	//   for _, ruleName := range state.NSGRuleNames {
	//       s.azure.NSG.Delete(s.ctx(), s.config.ResourceGroup, state.NSGName, ruleName, ...)
	//   }

	s.NetworkState.Delete(networkID)
}

// cloudNetworkAddRule adds an NSG rule name to the network's tracking state.
// In production, this would create an actual Azure NSG rule.
func (s *Server) cloudNetworkAddRule(networkID, ruleName string) {
	s.NetworkState.Update(networkID, func(state *NetworkState) {
		state.NSGRuleNames = append(state.NSGRuleNames, ruleName)
	})

	// TODO: When an Azure Network SDK client is available, create the NSG rule:
	//   s.azure.NSG.CreateOrUpdateRule(s.ctx(), s.config.ResourceGroup,
	//       state.NSGName, ruleName, nsgRule, ...)
}
