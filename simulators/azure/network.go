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
	Protocol                 string   `json:"protocol"`
	SourcePortRange          string   `json:"sourcePortRange,omitempty"`
	DestinationPortRange     string   `json:"destinationPortRange,omitempty"`
	SourceAddressPrefix      string   `json:"sourceAddressPrefix,omitempty"`
	DestinationAddressPrefix string   `json:"destinationAddressPrefix,omitempty"`
	SourcePortRanges         []string `json:"sourcePortRanges,omitempty"`
	DestinationPortRanges    []string `json:"destinationPortRanges,omitempty"`
	SourceAddressPrefixes    []string `json:"sourceAddressPrefixes,omitempty"`
	DestinationAddressPrefixes []string `json:"destinationAddressPrefixes,omitempty"`
	Access                   string   `json:"access"`
	Priority                 int      `json:"priority"`
	Direction                string   `json:"direction"`
	ProvisioningState        string   `json:"provisioningState,omitempty"`
}

func registerNetwork(srv *sim.Server) {
	vnets := sim.NewStateStore[VirtualNetwork]()
	subnets := sim.NewStateStore[Subnet]()
	nsgs := sim.NewStateStore[NetworkSecurityGroup]()

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
}
