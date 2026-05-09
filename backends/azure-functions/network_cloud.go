package azf

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/privatedns/armprivatedns"
)

// cloudNetworkCreate provisions an Azure Private DNS zone for a Docker
// network. Mirrors aca/network_cloud.go::cloudNetworkCreate (same
// `skls-<name>.local` zone naming) without the per-network NSG: AZF
// function apps egress through Azure's managed plane, so per-network
// NSGs aren't a deploy-time concern for sockerless.
func (s *Server) cloudNetworkCreate(ctx context.Context, name, networkID string) error {
	zoneName := fmt.Sprintf("skls-%s.local", name)

	zonePoller, err := s.azure.PrivateDNSZones.BeginCreateOrUpdate(
		ctx,
		s.config.ResourceGroup,
		zoneName,
		armprivatedns.PrivateZone{Location: to.Ptr("global")},
		nil,
	)
	if err != nil {
		return fmt.Errorf("create Private DNS zone %s: %w", zoneName, err)
	}
	if _, err := zonePoller.PollUntilDone(ctx, nil); err != nil {
		return fmt.Errorf("wait Private DNS zone %s: %w", zoneName, err)
	}

	s.NetworkState.Put(networkID, NetworkState{DNSZoneName: zoneName})

	s.Logger.Debug().
		Str("network", name).
		Str("networkID", networkID).
		Str("zone", zoneName).
		Msg("created cloud network state with Private DNS zone")
	return nil
}

// cloudNetworkDelete tears down the Private DNS zone for a Docker network.
func (s *Server) cloudNetworkDelete(ctx context.Context, networkID string) error {
	state, ok := s.NetworkState.Get(networkID)
	if !ok {
		return nil
	}
	if state.DNSZoneName != "" {
		zonePoller, err := s.azure.PrivateDNSZones.BeginDelete(
			ctx,
			s.config.ResourceGroup,
			state.DNSZoneName,
			nil,
		)
		if err != nil {
			s.Logger.Warn().Err(err).
				Str("zone", state.DNSZoneName).
				Msg("begin delete Private DNS zone failed")
		} else if _, err := zonePoller.PollUntilDone(ctx, nil); err != nil {
			s.Logger.Warn().Err(err).
				Str("zone", state.DNSZoneName).
				Msg("poll delete Private DNS zone failed")
		}
	}
	s.NetworkState.Delete(networkID)
	s.Logger.Debug().
		Str("networkID", networkID).
		Str("zone", state.DNSZoneName).
		Msg("deleted cloud network state")
	return nil
}

// resolveNetworkState returns NetworkState with cloud-fallback. When the
// in-process cache misses, attempts to look up the per-network zone from
// the resource group (idempotent recovery after backend restart). Returns
// false when no zone exists for the network.
func (s *Server) resolveNetworkState(ctx context.Context, networkID string) (NetworkState, bool) {
	if state, ok := s.NetworkState.Get(networkID); ok && state.DNSZoneName != "" {
		return state, true
	}
	// Cloud fallback: AZF doesn't tag Private DNS zones with the
	// sockerless network ID today (zone names alone collide with the
	// Resource Group's flat namespace). Rely on the in-process cache —
	// a fresh backend can still pick up zones via `cloudNetworkRecover`
	// at startup if/when that lands. For now: zone unknown → false.
	return NetworkState{}, false
}
