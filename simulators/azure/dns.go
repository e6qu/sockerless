package main

import (
	"fmt"
	"net/http"
	"strings"

	sim "github.com/sockerless/simulator"
)

// PrivateDnsZone represents an Azure Private DNS Zone.
type PrivateDnsZone struct {
	ID         string                `json:"id"`
	Name       string                `json:"name"`
	Type       string                `json:"type"`
	Location   string                `json:"location"`
	Etag       string                `json:"etag,omitempty"`
	Tags       map[string]string     `json:"tags,omitempty"`
	Properties DnsZoneProperties     `json:"properties"`
}

// DnsZoneProperties holds the properties of a Private DNS Zone.
type DnsZoneProperties struct {
	MaxNumberOfRecordSets                     int    `json:"maxNumberOfRecordSets"`
	NumberOfRecordSets                        int    `json:"numberOfRecordSets"`
	MaxNumberOfVirtualNetworkLinks            int    `json:"maxNumberOfVirtualNetworkLinks"`
	NumberOfVirtualNetworkLinks               int    `json:"numberOfVirtualNetworkLinks"`
	MaxNumberOfVirtualNetworkLinksWithReg      int    `json:"maxNumberOfVirtualNetworkLinksWithRegistration"`
	NumberOfVirtualNetworkLinksWithReg         int    `json:"numberOfVirtualNetworkLinksWithRegistration"`
	ProvisioningState                         string `json:"provisioningState"`
}

// RecordSet represents a DNS record set.
type RecordSet struct {
	ID         string              `json:"id"`
	Name       string              `json:"name"`
	Type       string              `json:"type"`
	Etag       string              `json:"etag,omitempty"`
	Properties RecordSetProperties `json:"properties"`
}

// RecordSetProperties holds the properties of a DNS record set.
type RecordSetProperties struct {
	TTL             int        `json:"ttl,omitempty"`
	Fqdn            string     `json:"fqdn,omitempty"`
	IsAutoRegistered bool      `json:"isAutoRegistered"`
	ARecords        []ARecord  `json:"aRecords,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty"`
}

// ARecord represents an A record.
type ARecord struct {
	IPv4Address string `json:"ipv4Address"`
}

// VNetLink represents a Virtual Network Link to a Private DNS Zone.
type VNetLink struct {
	ID         string             `json:"id"`
	Name       string             `json:"name"`
	Type       string             `json:"type"`
	Location   string             `json:"location"`
	Tags       map[string]string  `json:"tags,omitempty"`
	Properties VNetLinkProperties `json:"properties"`
}

type VNetLinkProperties struct {
	VirtualNetwork          *VNetLinkVNet `json:"virtualNetwork,omitempty"`
	RegistrationEnabled     bool          `json:"registrationEnabled"`
	VirtualNetworkLinkState string        `json:"virtualNetworkLinkState"`
	ProvisioningState       string        `json:"provisioningState"`
}

type VNetLinkVNet struct {
	ID string `json:"id"`
}

func registerPrivateDNS(srv *sim.Server) {
	zones := sim.NewStateStore[PrivateDnsZone]()
	recordSets := sim.NewStateStore[RecordSet]()

	const armBase = "/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.Network"

	// PUT - Create or update zone
	srv.HandleFunc("PUT "+armBase+"/privateDnsZones/{zoneName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		zoneName := sim.PathParam(r, "zoneName")

		var req PrivateDnsZone
		if err := sim.ReadJSON(r, &req); err != nil {
			sim.AzureError(w, "InvalidRequestContent", "Failed to parse request body: "+err.Error(), http.StatusBadRequest)
			return
		}

		location := req.Location
		if location == "" {
			location = "global"
		}

		resourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/privateDnsZones/%s",
			sub, rg, zoneName)

		_, exists := zones.Get(resourceID)

		zone := PrivateDnsZone{
			ID:       resourceID,
			Name:     zoneName,
			Type:     "Microsoft.Network/privateDnsZones",
			Location: strings.ToLower(location),
			Etag:     generateUUID(),
			Tags:     req.Tags,
			Properties: DnsZoneProperties{
				MaxNumberOfRecordSets:                25000,
				NumberOfRecordSets:                   0,
				MaxNumberOfVirtualNetworkLinks:        1000,
				NumberOfVirtualNetworkLinks:           0,
				MaxNumberOfVirtualNetworkLinksWithReg: 100,
				NumberOfVirtualNetworkLinksWithReg:    0,
				ProvisioningState:                    "Succeeded",
			},
		}

		// Count existing record sets for this zone
		records := recordSets.Filter(func(rs RecordSet) bool {
			return strings.HasPrefix(rs.ID, resourceID+"/")
		})
		zone.Properties.NumberOfRecordSets = len(records)

		zones.Put(resourceID, zone)

		// Auto-create SOA record (azurerm provider reads this after zone creation)
		if !exists {
			soaID := fmt.Sprintf("%s/SOA/@", resourceID)
			if _, soaExists := recordSets.Get(soaID); !soaExists {
				recordSets.Put(soaID, RecordSet{
					ID:   soaID,
					Name: "@",
					Type: "Microsoft.Network/privateDnsZones/SOA",
					Etag: generateUUID(),
					Properties: RecordSetProperties{
						TTL:  3600,
						Fqdn: zoneName + ".",
					},
				})
				zone.Properties.NumberOfRecordSets = 1
				zones.Put(resourceID, zone)
			}
		}

		// go-azure-sdk expects 200 for sync creates
		sim.WriteJSON(w, http.StatusOK, zone)
	})

	// GET - Get zone
	srv.HandleFunc("GET "+armBase+"/privateDnsZones/{zoneName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		zoneName := sim.PathParam(r, "zoneName")

		resourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/privateDnsZones/%s",
			sub, rg, zoneName)

		zone, ok := zones.Get(resourceID)
		if !ok {
			sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound,
				"The Resource 'Microsoft.Network/privateDnsZones/%s' under resource group '%s' was not found.", zoneName, rg)
			return
		}

		sim.WriteJSON(w, http.StatusOK, zone)
	})

	// DELETE - Delete zone
	srv.HandleFunc("DELETE "+armBase+"/privateDnsZones/{zoneName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		zoneName := sim.PathParam(r, "zoneName")

		resourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/privateDnsZones/%s",
			sub, rg, zoneName)

		if zones.Delete(resourceID) {
			// Clean up associated record sets
			records := recordSets.Filter(func(rs RecordSet) bool {
				return strings.HasPrefix(rs.ID, resourceID+"/")
			})
			for _, rs := range records {
				recordSets.Delete(rs.ID)
			}
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusNoContent)
		}
	})

	// GET - Get SOA record
	srv.HandleFunc("GET "+armBase+"/privateDnsZones/{zoneName}/SOA/{recordName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		zoneName := sim.PathParam(r, "zoneName")
		recordName := sim.PathParam(r, "recordName")

		recordID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/privateDnsZones/%s/SOA/%s",
			sub, rg, zoneName, recordName)

		rs, ok := recordSets.Get(recordID)
		if !ok {
			sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound,
				"The record set '%s' of type 'SOA' in zone '%s' was not found.", recordName, zoneName)
			return
		}

		sim.WriteJSON(w, http.StatusOK, rs)
	})

	// PUT - Create or update A record
	srv.HandleFunc("PUT "+armBase+"/privateDnsZones/{zoneName}/A/{recordName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		zoneName := sim.PathParam(r, "zoneName")
		recordName := sim.PathParam(r, "recordName")

		// Verify zone exists
		zoneID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/privateDnsZones/%s",
			sub, rg, zoneName)
		if _, ok := zones.Get(zoneID); !ok {
			sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound,
				"The Resource 'Microsoft.Network/privateDnsZones/%s' under resource group '%s' was not found.", zoneName, rg)
			return
		}

		var req RecordSet
		if err := sim.ReadJSON(r, &req); err != nil {
			sim.AzureError(w, "InvalidRequestContent", "Failed to parse request body: "+err.Error(), http.StatusBadRequest)
			return
		}

		recordID := fmt.Sprintf("%s/A/%s", zoneID, recordName)

		ttl := req.Properties.TTL
		if ttl == 0 {
			ttl = 3600
		}

		rs := RecordSet{
			ID:   recordID,
			Name: recordName,
			Type: "Microsoft.Network/privateDnsZones/A",
			Etag: generateUUID(),
			Properties: RecordSetProperties{
				TTL:              ttl,
				Fqdn:             recordName + "." + zoneName + ".",
				IsAutoRegistered: false,
				ARecords:         req.Properties.ARecords,
				Metadata:         req.Properties.Metadata,
			},
		}

		recordSets.Put(recordID, rs)

		// Update zone record count
		zones.Update(zoneID, func(z *PrivateDnsZone) {
			records := recordSets.Filter(func(r RecordSet) bool {
				return strings.HasPrefix(r.ID, zoneID+"/")
			})
			z.Properties.NumberOfRecordSets = len(records)
		})

		// go-azure-sdk expects 200 for sync creates
		sim.WriteJSON(w, http.StatusOK, rs)
	})

	// GET - Get A record
	srv.HandleFunc("GET "+armBase+"/privateDnsZones/{zoneName}/A/{recordName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		zoneName := sim.PathParam(r, "zoneName")
		recordName := sim.PathParam(r, "recordName")

		recordID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/privateDnsZones/%s/A/%s",
			sub, rg, zoneName, recordName)

		rs, ok := recordSets.Get(recordID)
		if !ok {
			sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound,
				"The record set '%s' of type 'A' in zone '%s' was not found.", recordName, zoneName)
			return
		}

		sim.WriteJSON(w, http.StatusOK, rs)
	})

	// DELETE - Delete A record
	srv.HandleFunc("DELETE "+armBase+"/privateDnsZones/{zoneName}/A/{recordName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		zoneName := sim.PathParam(r, "zoneName")
		recordName := sim.PathParam(r, "recordName")

		zoneID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/privateDnsZones/%s",
			sub, rg, zoneName)
		recordID := fmt.Sprintf("%s/A/%s", zoneID, recordName)

		if recordSets.Delete(recordID) {
			// Update zone record count
			zones.Update(zoneID, func(z *PrivateDnsZone) {
				records := recordSets.Filter(func(r RecordSet) bool {
					return strings.HasPrefix(r.ID, zoneID+"/")
				})
				z.Properties.NumberOfRecordSets = len(records)
			})
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusNoContent)
		}
	})

	// GET - List A records
	srv.HandleFunc("GET "+armBase+"/privateDnsZones/{zoneName}/A", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		zoneName := sim.PathParam(r, "zoneName")

		prefix := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/privateDnsZones/%s/A/",
			sub, rg, zoneName)

		filtered := recordSets.Filter(func(rs RecordSet) bool {
			return strings.HasPrefix(rs.ID, prefix)
		})

		sim.WriteJSON(w, http.StatusOK, map[string]any{
			"value": filtered,
		})
	})

	// --- Virtual Network Links ---

	vnetLinks := sim.NewStateStore[VNetLink]()

	// PUT - Create or update VNet link
	srv.HandleFunc("PUT "+armBase+"/privateDnsZones/{zoneName}/virtualNetworkLinks/{linkName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		zoneName := sim.PathParam(r, "zoneName")
		linkName := sim.PathParam(r, "linkName")

		var req VNetLink
		if err := sim.ReadJSON(r, &req); err != nil {
			sim.AzureError(w, "InvalidRequestContent", "Failed to parse request body", http.StatusBadRequest)
			return
		}

		resourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/privateDnsZones/%s/virtualNetworkLinks/%s",
			sub, rg, zoneName, linkName)

		link := VNetLink{
			ID:       resourceID,
			Name:     linkName,
			Type:     "Microsoft.Network/privateDnsZones/virtualNetworkLinks",
			Location: "global",
			Tags:     req.Tags,
			Properties: VNetLinkProperties{
				VirtualNetwork:        req.Properties.VirtualNetwork,
				RegistrationEnabled:   req.Properties.RegistrationEnabled,
				VirtualNetworkLinkState: "Completed",
				ProvisioningState:     "Succeeded",
			},
		}
		vnetLinks.Put(resourceID, link)

		// go-azure-sdk expects 200 for sync creates
		sim.WriteJSON(w, http.StatusOK, link)
	})

	// GET - Get VNet link
	srv.HandleFunc("GET "+armBase+"/privateDnsZones/{zoneName}/virtualNetworkLinks/{linkName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		zoneName := sim.PathParam(r, "zoneName")
		linkName := sim.PathParam(r, "linkName")
		resourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/privateDnsZones/%s/virtualNetworkLinks/%s",
			sub, rg, zoneName, linkName)

		link, ok := vnetLinks.Get(resourceID)
		if !ok {
			sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound,
				"Virtual network link '%s' not found.", linkName)
			return
		}
		sim.WriteJSON(w, http.StatusOK, link)
	})

	// DELETE - Delete VNet link
	srv.HandleFunc("DELETE "+armBase+"/privateDnsZones/{zoneName}/virtualNetworkLinks/{linkName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		zoneName := sim.PathParam(r, "zoneName")
		linkName := sim.PathParam(r, "linkName")
		resourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/privateDnsZones/%s/virtualNetworkLinks/%s",
			sub, rg, zoneName, linkName)

		vnetLinks.Delete(resourceID)
		w.WriteHeader(http.StatusAccepted)
	})
}
