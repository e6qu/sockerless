package main

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"net/http"
	"strings"

	sim "github.com/sockerless/simulator"
)

// Cloud DNS types

// ManagedZone represents a Cloud DNS managed zone.
type ManagedZone struct {
	Name                    string         `json:"name"`
	DNSName                 string         `json:"dnsName"`
	Description             string         `json:"description,omitempty"`
	ID                      string         `json:"id,omitempty"`
	Visibility              string         `json:"visibility,omitempty"`
	PrivateVisibilityConfig map[string]any `json:"privateVisibilityConfig,omitempty"`
	// DockerNetworkName is the real Docker user-defined network backing
	// this private zone. Containers referenced by A records inside the
	// zone are connected to this network with the record's short name
	// as DNS alias, so cross-container DNS resolves via Docker's
	// embedded DNS. Empty for public zones.
	DockerNetworkName string `json:"dockerNetworkName,omitempty"`
}

// ResourceRecordSet represents a DNS record set.
type ResourceRecordSet struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	TTL     int      `json:"ttl"`
	Rrdatas []string `json:"rrdatas"`
}

func registerCloudDNS(srv *sim.Server) {
	zones := sim.MakeStore[ManagedZone](srv.DB(), "dns_zones")
	recordSets := sim.MakeStore[ResourceRecordSet](srv.DB(), "dns_record_sets")

	// Create managed zone
	srv.HandleFunc("POST /dns/v1/projects/{project}/managedZones", func(w http.ResponseWriter, r *http.Request) {
		var zone ManagedZone
		if err := sim.ReadJSON(r, &zone); err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
			return
		}

		if zone.Name == "" {
			sim.GCPError(w, http.StatusBadRequest, "name is required", "INVALID_ARGUMENT")
			return
		}
		if zone.DNSName == "" {
			sim.GCPError(w, http.StatusBadRequest, "dnsName is required", "INVALID_ARGUMENT")
			return
		}

		project := sim.PathParam(r, "project")
		key := project + "/" + zone.Name

		if _, exists := zones.Get(key); exists {
			sim.GCPErrorf(w, http.StatusConflict, "ALREADY_EXISTS", "managed zone %q already exists", zone.Name)
			return
		}

		if zone.ID == "" {
			// DNS API expects a numeric uint64 ID
			b := make([]byte, 8)
			_, _ = rand.Read(b)
			zone.ID = fmt.Sprintf("%d", binary.BigEndian.Uint64(b)>>1)
		}
		if zone.Visibility == "" {
			zone.Visibility = "public"
		}

		// Back every private zone with a real Docker network.
		// Containers registered in the zone via A records (sockerless's
		// service-register step) get connected to this network with
		// their record short-name as DNS alias, so cross-container DNS
		// works via Docker's embedded resolver. Public zones keep
		// today's behavior (no Docker network).
		if zone.Visibility == "private" {
			netName := "sim-" + zone.ID
			if _, err := sim.EnsureDockerNetwork(netName); err == nil {
				zone.DockerNetworkName = netName
			}
		}

		zones.Put(key, zone)
		sim.WriteJSON(w, http.StatusOK, zone)
	})

	// List managed zones
	srv.HandleFunc("GET /dns/v1/projects/{project}/managedZones", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		prefix := project + "/"

		items := zones.Filter(func(z ManagedZone) bool {
			key := project + "/" + z.Name
			return strings.HasPrefix(key, prefix)
		})
		if items == nil {
			items = []ManagedZone{}
		}

		sim.WriteJSON(w, http.StatusOK, map[string]any{
			"managedZones": items,
		})
	})

	// Get managed zone
	srv.HandleFunc("GET /dns/v1/projects/{project}/managedZones/{zone}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		zoneName := sim.PathParam(r, "zone")
		key := project + "/" + zoneName

		zone, ok := zones.Get(key)
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "managed zone %q not found", zoneName)
			return
		}
		sim.WriteJSON(w, http.StatusOK, zone)
	})

	// Delete managed zone
	srv.HandleFunc("DELETE /dns/v1/projects/{project}/managedZones/{zone}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		zoneName := sim.PathParam(r, "zone")
		key := project + "/" + zoneName

		zone, ok := zones.Get(key)
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "managed zone %q not found", zoneName)
			return
		}
		zones.Delete(key)

		// Delete associated record sets for this zone.
		// Record set keys are formatted as "project/zone:name:type".
		// We try deleting every record set with a key prefixed by this zone.
		allRRS := recordSets.List()
		for _, rs := range allRRS {
			rsKey := fmt.Sprintf("%s:%s:%s", key, rs.Name, rs.Type)
			// This will be a no-op if the key doesn't exist (wrong zone)
			recordSets.Delete(rsKey)
		}

		// Drop the Docker network backing the private zone.
		if zone.DockerNetworkName != "" {
			_ = sim.RemoveDockerNetwork(zone.DockerNetworkName)
		}

		sim.WriteJSON(w, http.StatusOK, map[string]any{})
	})

	// List record sets
	srv.HandleFunc("GET /dns/v1/projects/{project}/managedZones/{zone}/rrsets", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		zoneName := sim.PathParam(r, "zone")
		zoneKey := project + "/" + zoneName

		if _, ok := zones.Get(zoneKey); !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "managed zone %q not found", zoneName)
			return
		}

		prefix := zoneKey + ":"

		// Filter record sets belonging to this zone by reconstructing the key
		var filtered []ResourceRecordSet
		all := recordSets.List()
		for _, rs := range all {
			rsKey := zoneKey + ":" + rs.Name + ":" + rs.Type
			if strings.HasPrefix(rsKey, prefix) {
				filtered = append(filtered, rs)
			}
		}
		if filtered == nil {
			filtered = []ResourceRecordSet{}
		}

		sim.WriteJSON(w, http.StatusOK, map[string]any{
			"rrsets": filtered,
		})
	})

	// Create record set
	srv.HandleFunc("POST /dns/v1/projects/{project}/managedZones/{zone}/rrsets", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		zoneName := sim.PathParam(r, "zone")
		zoneKey := project + "/" + zoneName

		zone, ok := zones.Get(zoneKey)
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "managed zone %q not found", zoneName)
			return
		}

		var rs ResourceRecordSet
		if err := sim.ReadJSON(r, &rs); err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
			return
		}

		if rs.Name == "" || rs.Type == "" {
			sim.GCPError(w, http.StatusBadRequest, "name and type are required", "INVALID_ARGUMENT")
			return
		}

		key := fmt.Sprintf("%s:%s:%s", zoneKey, rs.Name, rs.Type)
		if _, exists := recordSets.Get(key); exists {
			sim.GCPErrorf(w, http.StatusConflict, "ALREADY_EXISTS", "record set %s/%s already exists", rs.Name, rs.Type)
			return
		}

		recordSets.Put(key, rs)

		// For A records on a private zone, connect the container
		// identified by Rrdatas[0] (its bridge-network IP) to the
		// zone's Docker network, with the record's short name as
		// DNS alias. Cross-container DNS resolves via Docker's
		// embedded resolver from that point on.
		if zone.DockerNetworkName != "" && rs.Type == "A" && len(rs.Rrdatas) > 0 {
			if containerName := sim.FindContainerByIP(rs.Rrdatas[0]); containerName != "" {
				alias := shortHostnameFromDNS(rs.Name, zone.DNSName)
				_ = sim.ConnectContainerToNetwork(containerName, zone.DockerNetworkName, []string{alias})
			}
		}

		sim.WriteJSON(w, http.StatusOK, rs)
	})

	// Delete record set
	srv.HandleFunc("DELETE /dns/v1/projects/{project}/managedZones/{zone}/rrsets/{name}/{type}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		zoneName := sim.PathParam(r, "zone")
		rrName := sim.PathParam(r, "name")
		rrType := sim.PathParam(r, "type")
		zoneKey := project + "/" + zoneName
		key := fmt.Sprintf("%s:%s:%s", zoneKey, rrName, rrType)

		rs, rsOk := recordSets.Get(key)
		if !recordSets.Delete(key) {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "record set %s/%s not found", rrName, rrType)
			return
		}

		// Disconnect the container that was connected when the
		// record was created. Best-effort — container shutdown
		// already cleans up Docker-side network memberships.
		if rsOk && rs.Type == "A" && len(rs.Rrdatas) > 0 {
			if zone, ok := zones.Get(zoneKey); ok && zone.DockerNetworkName != "" {
				if containerName := sim.FindContainerByIP(rs.Rrdatas[0]); containerName != "" {
					_ = sim.DisconnectContainerFromNetwork(containerName, zone.DockerNetworkName)
				}
			}
		}

		sim.WriteJSON(w, http.StatusOK, map[string]any{})
	})
}

// shortHostnameFromDNS strips the zone's DNS suffix from a record name
// so we can use the short hostname as a Docker DNS alias. Cloud DNS
// names are always FQDNs with a trailing dot, e.g. "alpha.test.local."
// for a zone whose DNSName is "test.local." → "alpha". Docker's
// embedded DNS resolves short names via aliases, so this is what we
// want containers inside the network to use as `getent hosts alpha`.
func shortHostnameFromDNS(recordName, zoneDNS string) string {
	name := strings.TrimSuffix(recordName, ".")
	suffix := strings.TrimSuffix(zoneDNS, ".")
	if suffix != "" && strings.HasSuffix(name, "."+suffix) {
		name = strings.TrimSuffix(name, "."+suffix)
	}
	return name
}
