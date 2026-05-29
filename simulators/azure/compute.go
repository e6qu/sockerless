package main

import (
	"fmt"
	"net"
	"net/http"
	"strings"

	sim "github.com/sockerless/simulator"
)

type PublicIPAddress struct {
	ID         string                    `json:"id"`
	Name       string                    `json:"name"`
	Type       string                    `json:"type"`
	Location   string                    `json:"location"`
	Tags       map[string]string         `json:"tags,omitempty"`
	Sku        *SkuName                  `json:"sku,omitempty"`
	Properties PublicIPAddressProperties `json:"properties"`
}

type PublicIPAddressProperties struct {
	PublicIPAddress          string `json:"ipAddress,omitempty"`
	PublicIPAllocationMethod string `json:"publicIPAllocationMethod,omitempty"`
	PublicIPAddressVersion   string `json:"publicIPAddressVersion,omitempty"`
	ProvisioningState        string `json:"provisioningState,omitempty"`
}

type LoadBalancer struct {
	ID         string                 `json:"id"`
	Name       string                 `json:"name"`
	Type       string                 `json:"type"`
	Location   string                 `json:"location"`
	Tags       map[string]string      `json:"tags,omitempty"`
	Sku        *SkuName               `json:"sku,omitempty"`
	Properties LoadBalancerProperties `json:"properties"`
}

type LoadBalancerProperties struct {
	FrontendIPConfigurations []LoadBalancerChild `json:"frontendIPConfigurations"`
	BackendAddressPools      []LoadBalancerChild `json:"backendAddressPools"`
	LoadBalancingRules       []LoadBalancerChild `json:"loadBalancingRules"`
	Probes                   []LoadBalancerChild `json:"probes"`
	ProvisioningState        string              `json:"provisioningState,omitempty"`
}

type LoadBalancerChild struct {
	ID         string         `json:"id,omitempty"`
	Name       string         `json:"name,omitempty"`
	Type       string         `json:"type,omitempty"`
	Properties map[string]any `json:"properties,omitempty"`
}

type NetworkInterface struct {
	ID         string                     `json:"id"`
	Name       string                     `json:"name"`
	Type       string                     `json:"type"`
	Location   string                     `json:"location"`
	Tags       map[string]string          `json:"tags,omitempty"`
	Properties NetworkInterfaceProperties `json:"properties"`
}

type NetworkInterfaceProperties struct {
	IPConfigurations            []NetworkInterfaceIPConfiguration `json:"ipConfigurations,omitempty"`
	DNSSettings                 map[string]any                    `json:"dnsSettings,omitempty"`
	EnableAcceleratedNetworking bool                              `json:"enableAcceleratedNetworking,omitempty"`
	EnableIPForwarding          bool                              `json:"enableIPForwarding,omitempty"`
	NetworkSecurityGroup        *SubResource                      `json:"networkSecurityGroup,omitempty"`
	ProvisioningState           string                            `json:"provisioningState,omitempty"`
	MacAddress                  string                            `json:"macAddress,omitempty"`
	Primary                     bool                              `json:"primary,omitempty"`
	VirtualMachine              *SubResource                      `json:"virtualMachine,omitempty"`
}

type NetworkInterfaceIPConfiguration struct {
	ID         string                                    `json:"id,omitempty"`
	Name       string                                    `json:"name"`
	Type       string                                    `json:"type,omitempty"`
	Properties NetworkInterfaceIPConfigurationProperties `json:"properties"`
}

type NetworkInterfaceIPConfigurationProperties struct {
	Subnet                    *SubResource `json:"subnet,omitempty"`
	PublicIPAddress           *SubResource `json:"publicIPAddress,omitempty"`
	PrivateIPAddress          string       `json:"privateIPAddress,omitempty"`
	PrivateIPAllocationMethod string       `json:"privateIPAllocationMethod,omitempty"`
	PrivateIPAddressVersion   string       `json:"privateIPAddressVersion,omitempty"`
	Primary                   bool         `json:"primary,omitempty"`
	ProvisioningState         string       `json:"provisioningState,omitempty"`
}

type VirtualMachine struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Type       string            `json:"type"`
	Location   string            `json:"location"`
	Tags       map[string]string `json:"tags,omitempty"`
	Properties VMProperties      `json:"properties"`
}

type VMProperties struct {
	HardwareProfile   map[string]any   `json:"hardwareProfile,omitempty"`
	StorageProfile    map[string]any   `json:"storageProfile,omitempty"`
	OSProfile         map[string]any   `json:"osProfile,omitempty"`
	NetworkProfile    VMNetworkProfile `json:"networkProfile,omitempty"`
	ProvisioningState string           `json:"provisioningState,omitempty"`
	VMID              string           `json:"vmId,omitempty"`
	InstanceView      *VMInstanceView  `json:"instanceView,omitempty"`
}

type VMNetworkProfile struct {
	NetworkInterfaces []VMNetworkInterfaceRef `json:"networkInterfaces,omitempty"`
}

type VMNetworkInterfaceRef struct {
	ID         string         `json:"id"`
	Properties map[string]any `json:"properties,omitempty"`
}

type VMInstanceView struct {
	Statuses []VMStatus `json:"statuses,omitempty"`
}

type VMStatus struct {
	Code          string `json:"code"`
	Level         string `json:"level"`
	DisplayStatus string `json:"displayStatus"`
	Message       string `json:"message,omitempty"`
	Time          string `json:"time,omitempty"`
}

var (
	azurePublicIPs sim.Store[PublicIPAddress]
	azureLBs       sim.Store[LoadBalancer]
	azureNICs      sim.Store[NetworkInterface]
	azureVMs       sim.Store[VirtualMachine]
	azureVMStates  sim.Store[string]
)

func registerCompute(srv *sim.Server) {
	azurePublicIPs = sim.MakeStore[PublicIPAddress](srv.DB(), "network_public_ips")
	azureLBs = sim.MakeStore[LoadBalancer](srv.DB(), "network_load_balancers")
	azureNICs = sim.MakeStore[NetworkInterface](srv.DB(), "network_interfaces")
	azureVMs = sim.MakeStore[VirtualMachine](srv.DB(), "compute_virtual_machines")
	azureVMStates = sim.MakeStore[string](srv.DB(), "compute_virtual_machine_states")

	registerComputeCatalog(srv)
	registerPublicIPAddresses(srv)
	registerLoadBalancers(srv)
	registerNetworkInterfaces(srv)
	registerVirtualMachines(srv)
}

func registerComputeCatalog(srv *sim.Server) {
	srv.HandleFunc("GET /subscriptions/{subscriptionId}/providers/Microsoft.Compute/locations/{location}/vmSizes", func(w http.ResponseWriter, r *http.Request) {
		sim.WriteJSON(w, http.StatusOK, map[string]any{
			"value": []map[string]any{
				{
					"name":                 "Standard_B1s",
					"numberOfCores":        1,
					"osDiskSizeInMB":       1047552,
					"resourceDiskSizeInMB": 4096,
					"memoryInMB":           1024,
					"maxDataDiskCount":     2,
				},
				{
					"name":                 "Standard_B2s",
					"numberOfCores":        2,
					"osDiskSizeInMB":       1047552,
					"resourceDiskSizeInMB": 8192,
					"memoryInMB":           4096,
					"maxDataDiskCount":     4,
				},
			},
		})
	})

	srv.HandleFunc("GET /subscriptions/{subscriptionId}/providers/Microsoft.Compute/skus", func(w http.ResponseWriter, r *http.Request) {
		sim.WriteJSON(w, http.StatusOK, map[string]any{
			"value": []map[string]any{
				{
					"resourceType": "virtualMachines",
					"name":         "Standard_B1s",
					"tier":         "Standard",
					"size":         "B1s",
					"family":       "standardBSFamily",
					"locations":    []string{"eastus", "westeurope"},
					"capabilities": []map[string]string{
						{"name": "vCPUs", "value": "1"},
						{"name": "MemoryGB", "value": "1"},
					},
				},
			},
		})
	})
}

func registerPublicIPAddresses(srv *sim.Server) {
	const armBase = "/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.Network"

	srv.HandleFunc("PUT "+armBase+"/publicIPAddresses/{publicIPName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		name := sim.PathParam(r, "publicIPName")
		var req PublicIPAddress
		if err := sim.ReadJSON(r, &req); err != nil {
			sim.AzureError(w, "InvalidRequestContent", "Failed to parse request body: "+err.Error(), http.StatusBadRequest)
			return
		}
		id := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/publicIPAddresses/%s", sub, rg, name)
		pip := PublicIPAddress{
			ID:       id,
			Name:     name,
			Type:     "Microsoft.Network/publicIPAddresses",
			Location: req.Location,
			Tags:     req.Tags,
			Sku:      req.Sku,
			Properties: PublicIPAddressProperties{
				PublicIPAddress:          req.Properties.PublicIPAddress,
				PublicIPAllocationMethod: req.Properties.PublicIPAllocationMethod,
				PublicIPAddressVersion:   req.Properties.PublicIPAddressVersion,
				ProvisioningState:        "Succeeded",
			},
		}
		if pip.Properties.PublicIPAllocationMethod == "" {
			pip.Properties.PublicIPAllocationMethod = "Dynamic"
		}
		if pip.Properties.PublicIPAddressVersion == "" {
			pip.Properties.PublicIPAddressVersion = "IPv4"
		}
		if pip.Properties.PublicIPAddress == "" && strings.EqualFold(pip.Properties.PublicIPAllocationMethod, "Static") {
			pip.Properties.PublicIPAddress = fmt.Sprintf("203.0.113.%d", len(azurePublicIPs.List())+4)
		}
		azurePublicIPs.Put(id, pip)
		sim.WriteJSON(w, http.StatusOK, pip)
	})

	srv.HandleFunc("GET "+armBase+"/publicIPAddresses/{publicIPName}", func(w http.ResponseWriter, r *http.Request) {
		id := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/publicIPAddresses/%s",
			sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "publicIPName"))
		pip, ok := azurePublicIPs.Get(id)
		if !ok {
			sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "The Resource %q was not found.", id)
			return
		}
		sim.WriteJSON(w, http.StatusOK, pip)
	})

	srv.HandleFunc("GET "+armBase+"/publicIPAddresses", func(w http.ResponseWriter, r *http.Request) {
		prefix := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/publicIPAddresses/",
			sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"))
		items := azurePublicIPs.Filter(func(p PublicIPAddress) bool { return strings.HasPrefix(p.ID, prefix) })
		if items == nil {
			items = []PublicIPAddress{}
		}
		sim.WriteJSON(w, http.StatusOK, map[string]any{"value": items})
	})

	srv.HandleFunc("DELETE "+armBase+"/publicIPAddresses/{publicIPName}", func(w http.ResponseWriter, r *http.Request) {
		id := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/publicIPAddresses/%s",
			sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "publicIPName"))
		azurePublicIPs.Delete(id)
		w.WriteHeader(http.StatusOK)
	})
}

func registerLoadBalancers(srv *sim.Server) {
	const armBase = "/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.Network"

	srv.HandleFunc("PUT "+armBase+"/loadBalancers/{loadBalancerName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		name := sim.PathParam(r, "loadBalancerName")
		var req LoadBalancer
		if err := sim.ReadJSON(r, &req); err != nil {
			sim.AzureError(w, "InvalidRequestContent", "Failed to parse request body: "+err.Error(), http.StatusBadRequest)
			return
		}
		id := azureLoadBalancerID(sub, rg, name)
		lb := LoadBalancer{
			ID:       id,
			Name:     name,
			Type:     "Microsoft.Network/loadBalancers",
			Location: req.Location,
			Tags:     req.Tags,
			Sku:      req.Sku,
			Properties: LoadBalancerProperties{
				FrontendIPConfigurations: normalizeLoadBalancerChildren(id, "frontendIPConfigurations", "Microsoft.Network/loadBalancers/frontendIPConfigurations", req.Properties.FrontendIPConfigurations),
				BackendAddressPools:      normalizeLoadBalancerChildren(id, "backendAddressPools", "Microsoft.Network/loadBalancers/backendAddressPools", req.Properties.BackendAddressPools),
				LoadBalancingRules:       normalizeLoadBalancerChildren(id, "loadBalancingRules", "Microsoft.Network/loadBalancers/loadBalancingRules", req.Properties.LoadBalancingRules),
				Probes:                   normalizeLoadBalancerChildren(id, "probes", "Microsoft.Network/loadBalancers/probes", req.Properties.Probes),
				ProvisioningState:        "Succeeded",
			},
		}
		azureLBs.Put(id, lb)
		sim.WriteJSON(w, http.StatusOK, lb)
	})

	srv.HandleFunc("GET "+armBase+"/loadBalancers/{loadBalancerName}", func(w http.ResponseWriter, r *http.Request) {
		id := azureLoadBalancerID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "loadBalancerName"))
		lb, ok := azureLBs.Get(id)
		if !ok {
			sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "The Resource %q was not found.", id)
			return
		}
		sim.WriteJSON(w, http.StatusOK, lb)
	})

	srv.HandleFunc("GET "+armBase+"/loadBalancers", func(w http.ResponseWriter, r *http.Request) {
		prefix := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/loadBalancers/",
			sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"))
		items := azureLBs.Filter(func(lb LoadBalancer) bool { return strings.HasPrefix(lb.ID, prefix) })
		if items == nil {
			items = []LoadBalancer{}
		}
		sim.WriteJSON(w, http.StatusOK, map[string]any{"value": items})
	})

	srv.HandleFunc("DELETE "+armBase+"/loadBalancers/{loadBalancerName}", func(w http.ResponseWriter, r *http.Request) {
		id := azureLoadBalancerID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "loadBalancerName"))
		azureLBs.Delete(id)
		w.WriteHeader(http.StatusOK)
	})

	registerLoadBalancerChild(srv, "backendAddressPools", "backendAddressPoolName", "Microsoft.Network/loadBalancers/backendAddressPools",
		func(lb *LoadBalancer) *[]LoadBalancerChild { return &lb.Properties.BackendAddressPools })
	registerLoadBalancerChild(srv, "probes", "probeName", "Microsoft.Network/loadBalancers/probes",
		func(lb *LoadBalancer) *[]LoadBalancerChild { return &lb.Properties.Probes })
	registerLoadBalancerChild(srv, "loadBalancingRules", "loadBalancingRuleName", "Microsoft.Network/loadBalancers/loadBalancingRules",
		func(lb *LoadBalancer) *[]LoadBalancerChild { return &lb.Properties.LoadBalancingRules })
}

func registerLoadBalancerChild(srv *sim.Server, collection, paramName, resourceType string, children func(*LoadBalancer) *[]LoadBalancerChild) {
	const armBase = "/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.Network"
	pattern := armBase + "/loadBalancers/{loadBalancerName}/" + collection + "/{" + paramName + "}"

	srv.HandleFunc("PUT "+pattern, func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		lbName := sim.PathParam(r, "loadBalancerName")
		childName := sim.PathParam(r, paramName)
		lbID := azureLoadBalancerID(sub, rg, lbName)
		var req LoadBalancerChild
		if err := sim.ReadJSON(r, &req); err != nil {
			sim.AzureError(w, "InvalidRequestContent", "Failed to parse request body: "+err.Error(), http.StatusBadRequest)
			return
		}
		child := normalizeLoadBalancerChild(lbID, collection, resourceType, childName, req)
		if !azureLBs.Update(lbID, func(lb *LoadBalancer) {
			upsertLoadBalancerChild(children(lb), child)
		}) {
			sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "The Resource %q was not found.", lbID)
			return
		}
		sim.WriteJSON(w, http.StatusOK, child)
	})

	srv.HandleFunc("GET "+pattern, func(w http.ResponseWriter, r *http.Request) {
		lbID := azureLoadBalancerID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "loadBalancerName"))
		childName := sim.PathParam(r, paramName)
		lb, ok := azureLBs.Get(lbID)
		if !ok {
			sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "The Resource %q was not found.", lbID)
			return
		}
		for _, child := range *children(&lb) {
			if strings.EqualFold(child.Name, childName) {
				sim.WriteJSON(w, http.StatusOK, child)
				return
			}
		}
		sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "The Resource %q was not found.", lbID+"/"+collection+"/"+childName)
	})

	srv.HandleFunc("DELETE "+pattern, func(w http.ResponseWriter, r *http.Request) {
		lbID := azureLoadBalancerID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "loadBalancerName"))
		childName := sim.PathParam(r, paramName)
		azureLBs.Update(lbID, func(lb *LoadBalancer) {
			removeLoadBalancerChild(children(lb), childName)
		})
		w.WriteHeader(http.StatusOK)
	})
}

func azureLoadBalancerID(sub, rg, name string) string {
	return fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/loadBalancers/%s", sub, rg, name)
}

func normalizeLoadBalancerChildren(lbID, collection, resourceType string, children []LoadBalancerChild) []LoadBalancerChild {
	out := make([]LoadBalancerChild, 0, len(children))
	for _, child := range children {
		out = append(out, normalizeLoadBalancerChild(lbID, collection, resourceType, child.Name, child))
	}
	return out
}

func normalizeLoadBalancerChild(lbID, collection, resourceType, name string, child LoadBalancerChild) LoadBalancerChild {
	if name == "" {
		name = child.Name
	}
	child.ID = lbID + "/" + collection + "/" + name
	child.Name = name
	child.Type = resourceType
	if child.Properties == nil {
		child.Properties = map[string]any{}
	}
	child.Properties["provisioningState"] = "Succeeded"
	return child
}

func upsertLoadBalancerChild(children *[]LoadBalancerChild, child LoadBalancerChild) {
	for i := range *children {
		if strings.EqualFold((*children)[i].Name, child.Name) {
			(*children)[i] = child
			return
		}
	}
	*children = append(*children, child)
}

func removeLoadBalancerChild(children *[]LoadBalancerChild, name string) {
	filtered := (*children)[:0]
	for _, child := range *children {
		if !strings.EqualFold(child.Name, name) {
			filtered = append(filtered, child)
		}
	}
	*children = filtered
}

func registerNetworkInterfaces(srv *sim.Server) {
	const armBase = "/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.Network"

	srv.HandleFunc("PUT "+armBase+"/networkInterfaces/{networkInterfaceName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		name := sim.PathParam(r, "networkInterfaceName")
		var req NetworkInterface
		if err := sim.ReadJSON(r, &req); err != nil {
			sim.AzureError(w, "InvalidRequestContent", "Failed to parse request body: "+err.Error(), http.StatusBadRequest)
			return
		}
		id := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/networkInterfaces/%s", sub, rg, name)
		nic := NetworkInterface{
			ID:         id,
			Name:       name,
			Type:       "Microsoft.Network/networkInterfaces",
			Location:   req.Location,
			Tags:       req.Tags,
			Properties: req.Properties,
		}
		nic.Properties.ProvisioningState = "Succeeded"
		nic.Properties.MacAddress = "00-0D-3A-00-00-01"
		for i := range nic.Properties.IPConfigurations {
			ipcfg := &nic.Properties.IPConfigurations[i]
			if ipcfg.Name == "" {
				ipcfg.Name = fmt.Sprintf("ipconfig%d", i+1)
			}
			ipcfg.ID = id + "/ipConfigurations/" + ipcfg.Name
			ipcfg.Type = "Microsoft.Network/networkInterfaces/ipConfigurations"
			ipcfg.Properties.ProvisioningState = "Succeeded"
			if ipcfg.Properties.PrivateIPAllocationMethod == "" {
				ipcfg.Properties.PrivateIPAllocationMethod = "Dynamic"
			}
			if ipcfg.Properties.PrivateIPAddressVersion == "" {
				ipcfg.Properties.PrivateIPAddressVersion = "IPv4"
			}
			if i == 0 {
				ipcfg.Properties.Primary = true
			}
			if ipcfg.Properties.PrivateIPAddress == "" {
				ipcfg.Properties.PrivateIPAddress = allocateAzurePrivateIP(ipcfg.Properties.Subnet)
			}
		}
		azureNICs.Put(id, nic)
		sim.WriteJSON(w, http.StatusOK, nic)
	})

	srv.HandleFunc("GET "+armBase+"/networkInterfaces/{networkInterfaceName}", func(w http.ResponseWriter, r *http.Request) {
		id := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/networkInterfaces/%s",
			sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "networkInterfaceName"))
		nic, ok := azureNICs.Get(id)
		if !ok {
			sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "The Resource %q was not found.", id)
			return
		}
		sim.WriteJSON(w, http.StatusOK, nic)
	})

	srv.HandleFunc("GET "+armBase+"/networkInterfaces", func(w http.ResponseWriter, r *http.Request) {
		prefix := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/networkInterfaces/",
			sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"))
		items := azureNICs.Filter(func(n NetworkInterface) bool { return strings.HasPrefix(n.ID, prefix) })
		if items == nil {
			items = []NetworkInterface{}
		}
		sim.WriteJSON(w, http.StatusOK, map[string]any{"value": items})
	})

	srv.HandleFunc("DELETE "+armBase+"/networkInterfaces/{networkInterfaceName}", func(w http.ResponseWriter, r *http.Request) {
		id := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/networkInterfaces/%s",
			sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "networkInterfaceName"))
		azureNICs.Delete(id)
		w.WriteHeader(http.StatusOK)
	})
}

func allocateAzurePrivateIP(subnetRef *SubResource) string {
	if subnetRef != nil {
		if subnet, ok := azureSubnets.Get(subnetRef.ID); ok {
			if _, cidr, err := net.ParseCIDR(subnet.Properties.AddressPrefix); err == nil {
				ip := cidr.IP.To4()
				if ip != nil {
					out := make(net.IP, len(ip))
					copy(out, ip)
					out[3] += byte(len(azureNICs.List()) + 4)
					return out.String()
				}
			}
		}
	}
	return fmt.Sprintf("10.0.0.%d", len(azureNICs.List())+4)
}

func registerVirtualMachines(srv *sim.Server) {
	const armBase = "/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.Compute"

	srv.HandleFunc("PUT "+armBase+"/virtualMachines/{vmName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		name := sim.PathParam(r, "vmName")
		var req VirtualMachine
		if err := sim.ReadJSON(r, &req); err != nil {
			sim.AzureError(w, "InvalidRequestContent", "Failed to parse request body: "+err.Error(), http.StatusBadRequest)
			return
		}
		id := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Compute/virtualMachines/%s", sub, rg, name)
		vm := VirtualMachine{
			ID:         id,
			Name:       name,
			Type:       "Microsoft.Compute/virtualMachines",
			Location:   req.Location,
			Tags:       req.Tags,
			Properties: req.Properties,
		}
		vm.Properties.ProvisioningState = "Succeeded"
		if vm.Properties.VMID == "" {
			vm.Properties.VMID = generateUUID()
		}
		azureVMs.Put(id, vm)
		azureVMStates.Put(id, "PowerState/running")
		for _, nicRef := range vm.Properties.NetworkProfile.NetworkInterfaces {
			azureNICs.Update(nicRef.ID, func(nic *NetworkInterface) {
				nic.Properties.VirtualMachine = &SubResource{ID: id}
			})
		}
		sim.WriteJSON(w, http.StatusOK, virtualMachineWithInstanceView(vm))
	})

	srv.HandleFunc("GET "+armBase+"/virtualMachines/{vmName}", func(w http.ResponseWriter, r *http.Request) {
		id := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Compute/virtualMachines/%s",
			sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "vmName"))
		vm, ok := azureVMs.Get(id)
		if !ok {
			sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "The Resource %q was not found.", id)
			return
		}
		if strings.EqualFold(r.URL.Query().Get("$expand"), "instanceView") {
			vm = virtualMachineWithInstanceView(vm)
		}
		sim.WriteJSON(w, http.StatusOK, vm)
	})

	srv.HandleFunc("GET "+armBase+"/virtualMachines/{vmName}/instanceView", func(w http.ResponseWriter, r *http.Request) {
		id := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Compute/virtualMachines/%s",
			sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "vmName"))
		vm, ok := azureVMs.Get(id)
		if !ok {
			sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "The Resource %q was not found.", id)
			return
		}
		sim.WriteJSON(w, http.StatusOK, virtualMachineWithInstanceView(vm).Properties.InstanceView)
	})

	srv.HandleFunc("GET "+armBase+"/virtualMachines", func(w http.ResponseWriter, r *http.Request) {
		prefix := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Compute/virtualMachines/",
			sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"))
		items := azureVMs.Filter(func(vm VirtualMachine) bool { return strings.HasPrefix(vm.ID, prefix) })
		if items == nil {
			items = []VirtualMachine{}
		}
		sim.WriteJSON(w, http.StatusOK, map[string]any{"value": items})
	})

	srv.HandleFunc("DELETE "+armBase+"/virtualMachines/{vmName}", func(w http.ResponseWriter, r *http.Request) {
		id := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Compute/virtualMachines/%s",
			sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "vmName"))
		azureVMs.Delete(id)
		azureVMStates.Delete(id)
		w.WriteHeader(http.StatusOK)
	})

	for _, action := range []string{"start", "powerOff", "restart", "deallocate"} {
		action := action
		srv.HandleFunc("POST "+armBase+"/virtualMachines/{vmName}/"+action, func(w http.ResponseWriter, r *http.Request) {
			id := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Compute/virtualMachines/%s",
				sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "vmName"))
			if _, ok := azureVMs.Get(id); !ok {
				sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "The Resource %q was not found.", id)
				return
			}
			state := "PowerState/running"
			if action == "powerOff" {
				state = "PowerState/stopped"
			}
			if action == "deallocate" {
				state = "PowerState/deallocated"
			}
			azureVMStates.Put(id, state)
			sim.WriteJSON(w, http.StatusOK, map[string]any{"status": "Succeeded"})
		})
	}
}

func virtualMachineWithInstanceView(vm VirtualMachine) VirtualMachine {
	state, ok := azureVMStates.Get(vm.ID)
	if !ok {
		state = "PowerState/running"
	}
	display := strings.TrimPrefix(state, "PowerState/")
	vm.Properties.InstanceView = &VMInstanceView{
		Statuses: []VMStatus{
			{Code: "ProvisioningState/succeeded", Level: "Info", DisplayStatus: "Provisioning succeeded"},
			{Code: state, Level: "Info", DisplayStatus: "VM " + display},
		},
	}
	return vm
}
