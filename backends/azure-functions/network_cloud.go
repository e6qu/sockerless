package azf

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v8"
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

	// Provision the VNet + a subnet delegated to Microsoft.Web/serverFarms, so
	// the per-build sites can join it via App Service regional VNet integration
	// and reach each other by name. Link the Private DNS zone to the VNet so
	// names registered in the zone resolve for the integrated sites.
	vnetName := truncate("skls-vnet-"+name, 64)
	subnetID, err := s.cloudNetworkProvisionVNet(ctx, vnetName, zoneName)
	if err != nil {
		return err
	}

	s.NetworkState.Put(networkID, NetworkState{
		DNSZoneName: zoneName,
		VNetName:    vnetName,
		SubnetID:    subnetID,
	})

	s.Logger.Debug().
		Str("network", name).
		Str("networkID", networkID).
		Str("zone", zoneName).
		Str("vnet", vnetName).
		Msg("created cloud network state with Private DNS zone + VNet")
	return nil
}

// cloudNetworkProvisionVNet creates a VNet, a subnet delegated to
// Microsoft.Web/serverFarms (the App Service regional-VNet-integration subnet),
// and links the Private DNS zone to the VNet. Returns the subnet's resource ID.
func (s *Server) cloudNetworkProvisionVNet(ctx context.Context, vnetName, zoneName string) (string, error) {
	const subnetName = "appservice"

	vnetPoller, err := s.azure.VirtualNetworks.BeginCreateOrUpdate(ctx, s.config.ResourceGroup, vnetName, armnetwork.VirtualNetwork{
		Location: to.Ptr(s.config.Location),
		Properties: &armnetwork.VirtualNetworkPropertiesFormat{
			AddressSpace: &armnetwork.AddressSpace{AddressPrefixes: []*string{to.Ptr("10.40.0.0/16")}},
		},
	}, nil)
	if err != nil {
		return "", fmt.Errorf("create VNet %s: %w", vnetName, err)
	}
	vnetResp, err := vnetPoller.PollUntilDone(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("wait VNet %s: %w", vnetName, err)
	}
	vnetID := ""
	if vnetResp.ID != nil {
		vnetID = *vnetResp.ID
	}

	subnetPoller, err := s.azure.Subnets.BeginCreateOrUpdate(ctx, s.config.ResourceGroup, vnetName, subnetName, armnetwork.Subnet{
		Properties: &armnetwork.SubnetPropertiesFormat{
			AddressPrefix: to.Ptr("10.40.1.0/24"),
			Delegations: []*armnetwork.Delegation{{
				Name:       to.Ptr("appservice-delegation"),
				Properties: &armnetwork.ServiceDelegationPropertiesFormat{ServiceName: to.Ptr("Microsoft.Web/serverFarms")},
			}},
		},
	}, nil)
	if err != nil {
		return "", fmt.Errorf("create subnet %s/%s: %w", vnetName, subnetName, err)
	}
	subnetResp, err := subnetPoller.PollUntilDone(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("wait subnet %s/%s: %w", vnetName, subnetName, err)
	}
	subnetID := ""
	if subnetResp.ID != nil {
		subnetID = *subnetResp.ID
	}

	// Link the Private DNS zone to the VNet so the integrated sites resolve
	// names registered in the zone.
	linkPoller, err := s.azure.PrivateDNSLinks.BeginCreateOrUpdate(ctx, s.config.ResourceGroup, zoneName, vnetName+"-link", armprivatedns.VirtualNetworkLink{
		Location: to.Ptr("global"),
		Properties: &armprivatedns.VirtualNetworkLinkProperties{
			VirtualNetwork:      &armprivatedns.SubResource{ID: to.Ptr(vnetID)},
			RegistrationEnabled: to.Ptr(false),
		},
	}, nil)
	if err != nil {
		return "", fmt.Errorf("link Private DNS zone %s to VNet %s: %w", zoneName, vnetName, err)
	}
	if _, err := linkPoller.PollUntilDone(ctx, nil); err != nil {
		return "", fmt.Errorf("wait Private DNS zone link %s: %w", zoneName, err)
	}

	return subnetID, nil
}

// truncate returns s capped to n bytes.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
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
	if state.VNetName != "" {
		if vnetPoller, err := s.azure.VirtualNetworks.BeginDelete(ctx, s.config.ResourceGroup, state.VNetName, nil); err != nil {
			s.Logger.Warn().Err(err).Str("vnet", state.VNetName).Msg("begin delete VNet failed")
		} else if _, err := vnetPoller.PollUntilDone(ctx, nil); err != nil {
			s.Logger.Warn().Err(err).Str("vnet", state.VNetName).Msg("poll delete VNet failed")
		}
	}
	s.NetworkState.Delete(networkID)
	s.Logger.Debug().
		Str("networkID", networkID).
		Str("zone", state.DNSZoneName).
		Str("vnet", state.VNetName).
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
