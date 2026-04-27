package main

import (
	"fmt"
	"net/http"
	"strings"

	sim "github.com/sockerless/simulator"
)

// Virtual Network types

type VirtualNetwork struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Type       string            `json:"type"`
	Location   string            `json:"location"`
	Tags       map[string]string `json:"tags,omitempty"`
	Properties VNetProperties    `json:"properties"`
}

type VNetProperties struct {
	AddressSpace      AddressSpace `json:"addressSpace"`
	Subnets           []SubnetRef  `json:"subnets,omitempty"`
	ProvisioningState string       `json:"provisioningState"`
}

type AddressSpace struct {
	AddressPrefixes []string `json:"addressPrefixes"`
}

type SubnetRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Subnet types

type Subnet struct {
	ID         string           `json:"id"`
	Name       string           `json:"name"`
	Type       string           `json:"type"`
	Properties SubnetProperties `json:"properties"`
}

type SubnetProperties struct {
	AddressPrefix                     string             `json:"addressPrefix"`
	NetworkSecurityGroup              *NSGReference      `json:"networkSecurityGroup,omitempty"`
	Delegations                       []SubnetDelegation `json:"delegations,omitempty"`
	ProvisioningState                 string             `json:"provisioningState"`
	PrivateEndpointNetworkPolicies    string             `json:"privateEndpointNetworkPolicies,omitempty"`
	PrivateLinkServiceNetworkPolicies string             `json:"privateLinkServiceNetworkPolicies,omitempty"`
}

type NSGReference struct {
	ID string `json:"id"`
}

type SubnetDelegation struct {
	ID         string `json:"id,omitempty"`
	Name       string `json:"name"`
	Properties struct {
		ServiceName string `json:"serviceName"`
	} `json:"properties"`
}

// Network Security Group types

type NetworkSecurityGroup struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Type       string            `json:"type"`
	Location   string            `json:"location"`
	Tags       map[string]string `json:"tags,omitempty"`
	Properties NSGProperties     `json:"properties"`
}

type NSGProperties struct {
	SecurityRules     []SecurityRule `json:"securityRules,omitempty"`
	ProvisioningState string         `json:"provisioningState"`
}

type SecurityRule struct {
	ID         string                 `json:"id,omitempty"`
	Name       string                 `json:"name"`
	Type       string                 `json:"type,omitempty"`
	Properties SecurityRuleProperties `json:"properties"`
}

type SecurityRuleProperties struct {
	Protocol                   string   `json:"protocol"`
	SourcePortRange            string   `json:"sourcePortRange,omitempty"`
	DestinationPortRange       string   `json:"destinationPortRange,omitempty"`
	SourceAddressPrefix        string   `json:"sourceAddressPrefix,omitempty"`
	DestinationAddressPrefix   string   `json:"destinationAddressPrefix,omitempty"`
	SourcePortRanges           []string `json:"sourcePortRanges,omitempty"`
	DestinationPortRanges      []string `json:"destinationPortRanges,omitempty"`
	SourceAddressPrefixes      []string `json:"sourceAddressPrefixes,omitempty"`
	DestinationAddressPrefixes []string `json:"destinationAddressPrefixes,omitempty"`
	Access                     string   `json:"access"`
	Priority                   int      `json:"priority"`
	Direction                  string   `json:"direction"`
	ProvisioningState          string   `json:"provisioningState,omitempty"`
}

// NatGateway mirrors Microsoft.Network/natGateways. Sockerless flows
// using ACA Apps with VNet integration provision a NAT gateway for
// outbound connectivity. Field set covers what `azurerm_nat_gateway`
// and `armnetwork.NewNatGatewaysClient` round-trip on Get/List.
type NatGateway struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Type       string            `json:"type"`
	Location   string            `json:"location"`
	Tags       map[string]string `json:"tags,omitempty"`
	Sku        *SkuName          `json:"sku,omitempty"`
	Properties NatGatewayProps   `json:"properties"`
}

// NatGatewayProps holds the per-instance configuration for a NAT
// gateway: idle timeout, public IPs, attached subnets.
type NatGatewayProps struct {
	IdleTimeoutInMinutes int           `json:"idleTimeoutInMinutes,omitempty"`
	PublicIPAddresses    []SubResource `json:"publicIpAddresses,omitempty"`
	PublicIPPrefixes     []SubResource `json:"publicIpPrefixes,omitempty"`
	Subnets              []SubResource `json:"subnets,omitempty"`
	ProvisioningState    string        `json:"provisioningState,omitempty"`
}

// SkuName is the standard Azure SKU envelope used by NAT gateways and
// other resources that carry just a name.
type SkuName struct {
	Name string `json:"name"`
}

// SubResource is the standard Azure ARM reference shape — `{"id": "..."}`.
type SubResource struct {
	ID string `json:"id"`
}

// RouteTable mirrors Microsoft.Network/routeTables. Custom routing
// is provisioned by attaching a route table to a subnet.
type RouteTable struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Type       string            `json:"type"`
	Location   string            `json:"location"`
	Tags       map[string]string `json:"tags,omitempty"`
	Properties RouteTableProps   `json:"properties"`
}

// RouteTableProps holds the per-table configuration.
type RouteTableProps struct {
	DisableBgpRoutePropagation bool         `json:"disableBgpRoutePropagation,omitempty"`
	Routes                     []RouteEntry `json:"routes,omitempty"`
	ProvisioningState          string       `json:"provisioningState,omitempty"`
}

// RouteEntry is one row in a route table.
type RouteEntry struct {
	Name       string          `json:"name,omitempty"`
	Properties RouteEntryProps `json:"properties"`
}

// RouteEntryProps holds the per-route configuration: address prefix +
// next-hop type / IP.
type RouteEntryProps struct {
	AddressPrefix    string `json:"addressPrefix"`
	NextHopType      string `json:"nextHopType"`
	NextHopIPAddress string `json:"nextHopIpAddress,omitempty"`
}

func registerNetwork(srv *sim.Server) {
	vnets := sim.MakeStore[VirtualNetwork](srv.DB(), "network_vnets")
	subnets := sim.MakeStore[Subnet](srv.DB(), "network_subnets")
	nsgs := sim.MakeStore[NetworkSecurityGroup](srv.DB(), "network_nsgs")

	const armBase = "/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.Network"

	// --- Virtual Networks ---

	srv.HandleFunc("PUT "+armBase+"/virtualNetworks/{vnetName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		vnetName := sim.PathParam(r, "vnetName")

		var req VirtualNetwork
		if err := sim.ReadJSON(r, &req); err != nil {
			sim.AzureError(w, "InvalidRequestContent", "Failed to parse request body: "+err.Error(), http.StatusBadRequest)
			return
		}

		resourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/virtualNetworks/%s",
			sub, rg, vnetName)

		vnet := VirtualNetwork{
			ID:       resourceID,
			Name:     vnetName,
			Type:     "Microsoft.Network/virtualNetworks",
			Location: req.Location,
			Tags:     req.Tags,
			Properties: VNetProperties{
				AddressSpace:      req.Properties.AddressSpace,
				ProvisioningState: "Succeeded",
			},
		}

		// Collect subnet refs
		subnetList := subnets.Filter(func(s Subnet) bool {
			return strings.HasPrefix(s.ID, resourceID+"/subnets/")
		})
		for _, s := range subnetList {
			vnet.Properties.Subnets = append(vnet.Properties.Subnets, SubnetRef{ID: s.ID, Name: s.Name})
		}

		vnets.Put(resourceID, vnet)

		// go-azure-sdk expects 200 for sync creates
		sim.WriteJSON(w, http.StatusOK, vnet)
	})

	srv.HandleFunc("GET "+armBase+"/virtualNetworks/{vnetName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		vnetName := sim.PathParam(r, "vnetName")
		resourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/virtualNetworks/%s",
			sub, rg, vnetName)

		vnet, ok := vnets.Get(resourceID)
		if !ok {
			sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound,
				"The Resource 'Microsoft.Network/virtualNetworks/%s' under resource group '%s' was not found.", vnetName, rg)
			return
		}

		// Refresh subnet refs
		vnet.Properties.Subnets = nil
		subnetList := subnets.Filter(func(s Subnet) bool {
			return strings.HasPrefix(s.ID, resourceID+"/subnets/")
		})
		for _, s := range subnetList {
			vnet.Properties.Subnets = append(vnet.Properties.Subnets, SubnetRef{ID: s.ID, Name: s.Name})
		}

		sim.WriteJSON(w, http.StatusOK, vnet)
	})

	srv.HandleFunc("DELETE "+armBase+"/virtualNetworks/{vnetName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		vnetName := sim.PathParam(r, "vnetName")
		resourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/virtualNetworks/%s",
			sub, rg, vnetName)

		vnets.Delete(resourceID)
		// Clean up subnets
		subnetList := subnets.Filter(func(s Subnet) bool {
			return strings.HasPrefix(s.ID, resourceID+"/subnets/")
		})
		for _, s := range subnetList {
			subnets.Delete(s.ID)
		}
		w.WriteHeader(http.StatusOK)
	})

	// --- Subnets ---

	srv.HandleFunc("PUT "+armBase+"/virtualNetworks/{vnetName}/subnets/{subnetName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		vnetName := sim.PathParam(r, "vnetName")
		subnetName := sim.PathParam(r, "subnetName")

		var req Subnet
		if err := sim.ReadJSON(r, &req); err != nil {
			sim.AzureError(w, "InvalidRequestContent", "Failed to parse request body: "+err.Error(), http.StatusBadRequest)
			return
		}

		resourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/virtualNetworks/%s/subnets/%s",
			sub, rg, vnetName, subnetName)

		privateEndpointPolicies := req.Properties.PrivateEndpointNetworkPolicies
		if privateEndpointPolicies == "" {
			privateEndpointPolicies = "Disabled"
		}
		privateLinkPolicies := req.Properties.PrivateLinkServiceNetworkPolicies
		if privateLinkPolicies == "" {
			privateLinkPolicies = "Enabled"
		}

		sn := Subnet{
			ID:   resourceID,
			Name: subnetName,
			Type: "Microsoft.Network/virtualNetworks/subnets",
			Properties: SubnetProperties{
				AddressPrefix:                     req.Properties.AddressPrefix,
				NetworkSecurityGroup:              req.Properties.NetworkSecurityGroup,
				Delegations:                       req.Properties.Delegations,
				ProvisioningState:                 "Succeeded",
				PrivateEndpointNetworkPolicies:    privateEndpointPolicies,
				PrivateLinkServiceNetworkPolicies: privateLinkPolicies,
			},
		}
		subnets.Put(resourceID, sn)

		// go-azure-sdk expects 200 for sync creates
		sim.WriteJSON(w, http.StatusOK, sn)
	})

	srv.HandleFunc("GET "+armBase+"/virtualNetworks/{vnetName}/subnets/{subnetName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		vnetName := sim.PathParam(r, "vnetName")
		subnetName := sim.PathParam(r, "subnetName")
		resourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/virtualNetworks/%s/subnets/%s",
			sub, rg, vnetName, subnetName)

		sn, ok := subnets.Get(resourceID)
		if !ok {
			sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound,
				"The Resource 'subnets/%s' under virtualNetworks '%s' was not found.", subnetName, vnetName)
			return
		}
		sim.WriteJSON(w, http.StatusOK, sn)
	})

	srv.HandleFunc("DELETE "+armBase+"/virtualNetworks/{vnetName}/subnets/{subnetName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		vnetName := sim.PathParam(r, "vnetName")
		subnetName := sim.PathParam(r, "subnetName")
		resourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/virtualNetworks/%s/subnets/%s",
			sub, rg, vnetName, subnetName)

		subnets.Delete(resourceID)
		w.WriteHeader(http.StatusOK)
	})

	// --- Network Security Groups ---

	srv.HandleFunc("PUT "+armBase+"/networkSecurityGroups/{nsgName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		nsgName := sim.PathParam(r, "nsgName")

		var req NetworkSecurityGroup
		if err := sim.ReadJSON(r, &req); err != nil {
			sim.AzureError(w, "InvalidRequestContent", "Failed to parse request body: "+err.Error(), http.StatusBadRequest)
			return
		}

		resourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/networkSecurityGroups/%s",
			sub, rg, nsgName)

		// Set IDs on security rules
		for i := range req.Properties.SecurityRules {
			req.Properties.SecurityRules[i].ID = fmt.Sprintf("%s/securityRules/%s", resourceID, req.Properties.SecurityRules[i].Name)
			req.Properties.SecurityRules[i].Type = "Microsoft.Network/networkSecurityGroups/securityRules"
			req.Properties.SecurityRules[i].Properties.ProvisioningState = "Succeeded"
		}

		nsg := NetworkSecurityGroup{
			ID:       resourceID,
			Name:     nsgName,
			Type:     "Microsoft.Network/networkSecurityGroups",
			Location: req.Location,
			Tags:     req.Tags,
			Properties: NSGProperties{
				SecurityRules:     req.Properties.SecurityRules,
				ProvisioningState: "Succeeded",
			},
		}
		nsgs.Put(resourceID, nsg)

		// go-azure-sdk expects 200 for sync creates
		sim.WriteJSON(w, http.StatusOK, nsg)
	})

	srv.HandleFunc("GET "+armBase+"/networkSecurityGroups/{nsgName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		nsgName := sim.PathParam(r, "nsgName")
		resourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/networkSecurityGroups/%s",
			sub, rg, nsgName)

		nsg, ok := nsgs.Get(resourceID)
		if !ok {
			sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound,
				"The Resource 'Microsoft.Network/networkSecurityGroups/%s' under resource group '%s' was not found.", nsgName, rg)
			return
		}
		sim.WriteJSON(w, http.StatusOK, nsg)
	})

	srv.HandleFunc("DELETE "+armBase+"/networkSecurityGroups/{nsgName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		nsgName := sim.PathParam(r, "nsgName")
		resourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/networkSecurityGroups/%s",
			sub, rg, nsgName)

		nsgs.Delete(resourceID)
		w.WriteHeader(http.StatusOK)
	})

	// --- NSG Subnet Association ---
	// This is handled via the subnet PUT endpoint above (networkSecurityGroup property on subnet).
	// The azurerm_subnet_network_security_group_association resource just PUTs the subnet
	// with the networkSecurityGroup reference set.

	// --- NSG Security Rules (sub-resource) ---
	//
	// The NSG handlers above let callers embed SecurityRules in the
	// NSG body, but armnetwork.SecurityRulesClient CRUDs each rule
	// via its sub-resource path. Expose those so the backend can use
	// the per-rule client.

	// PUT rule
	srv.HandleFunc("PUT "+armBase+"/networkSecurityGroups/{nsgName}/securityRules/{ruleName}",
		func(w http.ResponseWriter, r *http.Request) {
			sub := sim.PathParam(r, "subscriptionId")
			rg := sim.PathParam(r, "resourceGroupName")
			nsgName := sim.PathParam(r, "nsgName")
			ruleName := sim.PathParam(r, "ruleName")
			nsgID := fmt.Sprintf(
				"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/networkSecurityGroups/%s",
				sub, rg, nsgName)
			ruleID := fmt.Sprintf("%s/securityRules/%s", nsgID, ruleName)

			nsg, ok := nsgs.Get(nsgID)
			if !ok {
				sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound,
					"The Resource 'Microsoft.Network/networkSecurityGroups/%s' under resource group '%s' was not found.",
					nsgName, rg)
				return
			}

			var req SecurityRule
			if err := sim.ReadJSON(r, &req); err != nil {
				sim.AzureError(w, "InvalidRequestContent",
					"Failed to parse request body: "+err.Error(), http.StatusBadRequest)
				return
			}
			req.ID = ruleID
			req.Name = ruleName
			req.Type = "Microsoft.Network/networkSecurityGroups/securityRules"
			req.Properties.ProvisioningState = "Succeeded"

			// Real Azure rejects duplicate priorities within an NSG +
			// direction (a rule cannot share a (Priority, Direction)
			// pair with another rule on the same NSG). The error code
			// is `SecurityRuleParameterPriorityAlreadyTaken`. Match
			// that contract so terraform configurations and SDK callers
			// see the real validation flow instead of a silent overwrite.
			for _, existing := range nsg.Properties.SecurityRules {
				if existing.Name == ruleName {
					continue
				}
				if existing.Properties.Priority == req.Properties.Priority &&
					strings.EqualFold(existing.Properties.Direction, req.Properties.Direction) {
					sim.AzureErrorf(w, "SecurityRuleParameterPriorityAlreadyTaken",
						http.StatusBadRequest,
						"Priority %d is already in use for direction %s by rule %q in NSG %q",
						req.Properties.Priority, req.Properties.Direction, existing.Name, nsgName)
					return
				}
			}

			// Upsert inside NSG.Properties.SecurityRules so GET NSG stays consistent.
			found := false
			for i := range nsg.Properties.SecurityRules {
				if nsg.Properties.SecurityRules[i].Name == ruleName {
					nsg.Properties.SecurityRules[i] = req
					found = true
					break
				}
			}
			if !found {
				nsg.Properties.SecurityRules = append(nsg.Properties.SecurityRules, req)
			}
			nsgs.Put(nsgID, nsg)

			sim.WriteJSON(w, http.StatusOK, req)
		})

	// GET rule
	srv.HandleFunc("GET "+armBase+"/networkSecurityGroups/{nsgName}/securityRules/{ruleName}",
		func(w http.ResponseWriter, r *http.Request) {
			sub := sim.PathParam(r, "subscriptionId")
			rg := sim.PathParam(r, "resourceGroupName")
			nsgName := sim.PathParam(r, "nsgName")
			ruleName := sim.PathParam(r, "ruleName")
			nsgID := fmt.Sprintf(
				"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/networkSecurityGroups/%s",
				sub, rg, nsgName)

			nsg, ok := nsgs.Get(nsgID)
			if !ok {
				sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound,
					"NSG '%s' not found in resource group '%s'.", nsgName, rg)
				return
			}
			for _, rule := range nsg.Properties.SecurityRules {
				if rule.Name == ruleName {
					sim.WriteJSON(w, http.StatusOK, rule)
					return
				}
			}
			sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound,
				"Security rule '%s' not found in NSG '%s'.", ruleName, nsgName)
		})

	// DELETE rule
	srv.HandleFunc("DELETE "+armBase+"/networkSecurityGroups/{nsgName}/securityRules/{ruleName}",
		func(w http.ResponseWriter, r *http.Request) {
			sub := sim.PathParam(r, "subscriptionId")
			rg := sim.PathParam(r, "resourceGroupName")
			nsgName := sim.PathParam(r, "nsgName")
			ruleName := sim.PathParam(r, "ruleName")
			nsgID := fmt.Sprintf(
				"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/networkSecurityGroups/%s",
				sub, rg, nsgName)

			nsg, ok := nsgs.Get(nsgID)
			if !ok {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			filtered := nsg.Properties.SecurityRules[:0]
			for _, rule := range nsg.Properties.SecurityRules {
				if rule.Name != ruleName {
					filtered = append(filtered, rule)
				}
			}
			nsg.Properties.SecurityRules = filtered
			nsgs.Put(nsgID, nsg)
			w.WriteHeader(http.StatusOK)
		})

	// LIST rules
	srv.HandleFunc("GET "+armBase+"/networkSecurityGroups/{nsgName}/securityRules",
		func(w http.ResponseWriter, r *http.Request) {
			sub := sim.PathParam(r, "subscriptionId")
			rg := sim.PathParam(r, "resourceGroupName")
			nsgName := sim.PathParam(r, "nsgName")
			nsgID := fmt.Sprintf(
				"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/networkSecurityGroups/%s",
				sub, rg, nsgName)

			nsg, ok := nsgs.Get(nsgID)
			if !ok {
				sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound,
					"NSG '%s' not found.", nsgName)
				return
			}
			sim.WriteJSON(w, http.StatusOK, map[string]any{
				"value": nsg.Properties.SecurityRules,
			})
		})

	// --- NAT Gateways + Route Tables ---
	// Real Azure: `Microsoft.Network/natGateways` provides outbound
	// connectivity for subnets that need explicit egress; route tables
	// (`Microsoft.Network/routeTables`) override default-route behavior.
	// Sockerless's serverless egress flows (ACA Apps with VNet integration
	// reaching the Internet) provision a NAT gateway; without these
	// handlers, terraform's `azurerm_nat_gateway` and `azurerm_route_table`
	// 404. Field set covers what the SDK round-trips on Get/List.
	natGateways := sim.MakeStore[NatGateway](srv.DB(), "nat_gateways")
	routeTables := sim.MakeStore[RouteTable](srv.DB(), "route_tables")

	srv.HandleFunc("PUT "+armBase+"/natGateways/{name}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		name := sim.PathParam(r, "name")
		var req NatGateway
		_ = sim.ReadJSON(r, &req)
		resourceID := fmt.Sprintf(
			"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/natGateways/%s",
			sub, rg, name)
		gw := NatGateway{
			ID:       resourceID,
			Name:     name,
			Type:     "Microsoft.Network/natGateways",
			Location: req.Location,
			Tags:     req.Tags,
			Sku:      req.Sku,
			Properties: NatGatewayProps{
				IdleTimeoutInMinutes: req.Properties.IdleTimeoutInMinutes,
				PublicIPAddresses:    req.Properties.PublicIPAddresses,
				PublicIPPrefixes:     req.Properties.PublicIPPrefixes,
				Subnets:              req.Properties.Subnets,
				ProvisioningState:    "Succeeded",
			},
		}
		if gw.Properties.IdleTimeoutInMinutes == 0 {
			gw.Properties.IdleTimeoutInMinutes = 4 // real Azure default
		}
		if gw.Sku == nil {
			gw.Sku = &SkuName{Name: "Standard"}
		}
		natGateways.Put(resourceID, gw)
		sim.WriteJSON(w, http.StatusOK, gw)
	})

	srv.HandleFunc("GET "+armBase+"/natGateways/{name}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		name := sim.PathParam(r, "name")
		resourceID := fmt.Sprintf(
			"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/natGateways/%s",
			sub, rg, name)
		gw, ok := natGateways.Get(resourceID)
		if !ok {
			sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound,
				"NAT gateway %q not found.", name)
			return
		}
		sim.WriteJSON(w, http.StatusOK, gw)
	})

	srv.HandleFunc("DELETE "+armBase+"/natGateways/{name}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		name := sim.PathParam(r, "name")
		resourceID := fmt.Sprintf(
			"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/natGateways/%s",
			sub, rg, name)
		natGateways.Delete(resourceID)
		w.WriteHeader(http.StatusOK)
	})

	srv.HandleFunc("PUT "+armBase+"/routeTables/{name}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		name := sim.PathParam(r, "name")
		var req RouteTable
		_ = sim.ReadJSON(r, &req)
		resourceID := fmt.Sprintf(
			"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/routeTables/%s",
			sub, rg, name)
		rt := RouteTable{
			ID:       resourceID,
			Name:     name,
			Type:     "Microsoft.Network/routeTables",
			Location: req.Location,
			Tags:     req.Tags,
			Properties: RouteTableProps{
				DisableBgpRoutePropagation: req.Properties.DisableBgpRoutePropagation,
				Routes:                     req.Properties.Routes,
				ProvisioningState:          "Succeeded",
			},
		}
		routeTables.Put(resourceID, rt)
		sim.WriteJSON(w, http.StatusOK, rt)
	})

	srv.HandleFunc("GET "+armBase+"/routeTables/{name}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		name := sim.PathParam(r, "name")
		resourceID := fmt.Sprintf(
			"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/routeTables/%s",
			sub, rg, name)
		rt, ok := routeTables.Get(resourceID)
		if !ok {
			sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound,
				"Route table %q not found.", name)
			return
		}
		sim.WriteJSON(w, http.StatusOK, rt)
	})

	srv.HandleFunc("DELETE "+armBase+"/routeTables/{name}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		name := sim.PathParam(r, "name")
		resourceID := fmt.Sprintf(
			"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/routeTables/%s",
			sub, rg, name)
		routeTables.Delete(resourceID)
		w.WriteHeader(http.StatusOK)
	})
}
