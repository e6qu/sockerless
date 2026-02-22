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
	Name                    string                   `json:"name"`
	DNSName                 string                   `json:"dnsName"`
	Description             string                   `json:"description,omitempty"`
	ID                      string                   `json:"id,omitempty"`
	Visibility              string                   `json:"visibility,omitempty"`
	PrivateVisibilityConfig map[string]any           `json:"privateVisibilityConfig,omitempty"`
}

// ResourceRecordSet represents a DNS record set.
type ResourceRecordSet struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	TTL     int      `json:"ttl"`
	Rrdatas []string `json:"rrdatas"`
}

func registerCloudDNS(srv *sim.Server) {
	zones := sim.NewStateStore[ManagedZone]()
	recordSets := sim.NewStateStore[ResourceRecordSet]()

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
			rand.Read(b)
			zone.ID = fmt.Sprintf("%d", binary.BigEndian.Uint64(b)>>1)
		}
		if zone.Visibility == "" {
			zone.Visibility = "public"
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

		if !zones.Delete(key) {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "managed zone %q not found", zoneName)
			return
		}

		// Delete associated record sets for this zone.
		// Record set keys are formatted as "project/zone:name:type".
		// We try deleting every record set with a key prefixed by this zone.
		allRRS := recordSets.List()
		for _, rs := range allRRS {
			rsKey := fmt.Sprintf("%s:%s:%s", key, rs.Name, rs.Type)
			// This will be a no-op if the key doesn't exist (wrong zone)
			recordSets.Delete(rsKey)
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

		if _, ok := zones.Get(zoneKey); !ok {
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

		if !recordSets.Delete(key) {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "record set %s/%s not found", rrName, rrType)
			return
		}

		sim.WriteJSON(w, http.StatusOK, map[string]any{})
	})
}
