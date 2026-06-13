package main

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"sort"
	"strconv"
	"strings"

	sim "github.com/sockerless/simulator"
)

// Cloud DNS types

// ManagedZone represents a Cloud DNS managed zone.
type ManagedZone struct {
	Name                    string            `json:"name"`
	DNSName                 string            `json:"dnsName"`
	Description             string            `json:"description,omitempty"`
	ID                      string            `json:"id,omitempty"`
	Visibility              string            `json:"visibility,omitempty"`
	PrivateVisibilityConfig map[string]any    `json:"privateVisibilityConfig,omitempty"`
	Labels                  map[string]string `json:"labels,omitempty"`
	// Nested writable configs the sim doesn't otherwise interpret are
	// stored verbatim so create→get round-trips byte-exact and the
	// terraform-provider-google read path doesn't perpetually drift.
	DnssecConfig       json.RawMessage `json:"dnssecConfig,omitempty"`
	ForwardingConfig   json.RawMessage `json:"forwardingConfig,omitempty"`
	PeeringConfig      json.RawMessage `json:"peeringConfig,omitempty"`
	CloudLoggingConfig json.RawMessage `json:"cloudLoggingConfig,omitempty"`
}

// storedManagedZone is the persisted row backing a managed zone: the
// wire-shape ManagedZone (what handlers emit — real Cloud DNS's
// ManagedZone has no dockerNetworkName member) plus sockerless wiring
// that must survive a simulator restart. The embedding flattens on
// json.Marshal, so sim.Store persistence keeps the same row shape the
// wiring has always been recovered from.
type storedManagedZone struct {
	ManagedZone
	// DockerNetworkName is the real Docker user-defined network backing
	// this private zone. Containers referenced by A records inside the
	// zone are connected to this network with the record's short name
	// as DNS alias, so cross-container DNS resolves via Docker's
	// embedded DNS. Empty for public zones. Store-only: never emitted
	// on the wire.
	DockerNetworkName string `json:"dockerNetworkName,omitempty"`
}

// pruneEmptyPrivateVisibilityConfig returns nil when the config carries no
// networks and no GKE clusters, matching real Cloud DNS (which omits an empty
// privateVisibilityConfig). A populated config is returned unchanged.
func pruneEmptyPrivateVisibilityConfig(pvc map[string]any) map[string]any {
	if len(pvc) == 0 {
		return nil
	}
	hasItems := func(key string) bool {
		v, ok := pvc[key]
		if !ok {
			return false
		}
		s, ok := v.([]any)
		return ok && len(s) > 0
	}
	if hasItems("networks") || hasItems("gkeClusters") {
		return pvc
	}
	return nil
}

// ResourceRecordSet represents a DNS record set.
type ResourceRecordSet struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	TTL     int      `json:"ttl"`
	Rrdatas []string `json:"rrdatas"`
}

type storedResourceRecordSet struct {
	Project string `json:"project"`
	Zone    string `json:"zone"`
	Record  ResourceRecordSet
}

type DNSChange struct {
	Additions []ResourceRecordSet `json:"additions,omitempty"`
	Deletions []ResourceRecordSet `json:"deletions,omitempty"`
	StartTime string              `json:"startTime,omitempty"`
	ID        string              `json:"id,omitempty"`
	Status    string              `json:"status,omitempty"`
	IsServing bool                `json:"isServing,omitempty"`
	Kind      string              `json:"kind,omitempty"`
}

type storedDNSChange struct {
	Project string    `json:"project"`
	Zone    string    `json:"zone"`
	Change  DNSChange `json:"change"`
}

func registerCloudDNS(srv *sim.Server) {
	zones := sim.MakeStore[storedManagedZone](srv.DB(), "dns_zones")
	recordSets := sim.MakeStore[storedResourceRecordSet](srv.DB(), "dns_record_sets")
	changes := sim.MakeStore[storedDNSChange](srv.DB(), "dns_changes")

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
		// terraform-provider-google always sends privateVisibilityConfig with an
		// empty networks list (even for public zones). Real Cloud DNS drops an
		// empty privateVisibilityConfig from the read-back; echoing it makes the
		// provider's flatten materialize a phantom block on every refresh. Strip
		// it unless it actually carries networks or GKE clusters.
		zone.PrivateVisibilityConfig = pruneEmptyPrivateVisibilityConfig(zone.PrivateVisibilityConfig)

		// Back every private zone with a real Docker network.
		// Containers registered in the zone via A records (sockerless's
		// service-register step) get connected to this network with
		// their record short-name as DNS alias, so cross-container DNS
		// works via Docker's embedded resolver. Public zones get no
		// Docker network.
		stored := storedManagedZone{ManagedZone: zone}
		if zone.Visibility == "private" {
			netName := "sim-" + zone.ID
			if _, err := sim.EnsureDockerNetwork(netName); err == nil {
				stored.DockerNetworkName = netName
			}
		}

		zones.Put(key, stored)
		sim.WriteJSON(w, http.StatusOK, stored.ManagedZone)
	})

	// List managed zones
	srv.HandleFunc("GET /dns/v1/projects/{project}/managedZones", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		prefix := project + "/"

		stored := zones.Filter(func(z storedManagedZone) bool {
			key := project + "/" + z.Name
			return strings.HasPrefix(key, prefix)
		})
		items := make([]ManagedZone, 0, len(stored))
		for _, z := range stored {
			items = append(items, z.ManagedZone)
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
		sim.WriteJSON(w, http.StatusOK, zone.ManagedZone)
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
		for _, stored := range recordSets.List() {
			if stored.Project == project && stored.Zone == zoneName {
				recordSets.Delete(dnsRecordSetKey(project, zoneName, stored.Record.Name, stored.Record.Type))
			}
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

		// Real Cloud DNS rrsets.list filters on the optional name +
		// type query params (the Go SDK's .Name()/.Type() builders);
		// the Cloud Run service-discovery path relies on this.
		nameFilter := r.URL.Query().Get("name")
		typeFilter := r.URL.Query().Get("type")
		var filtered []ResourceRecordSet
		for _, stored := range recordSets.List() {
			if stored.Project != project || stored.Zone != zoneName {
				continue
			}
			if nameFilter != "" && stored.Record.Name != nameFilter {
				continue
			}
			if typeFilter != "" && stored.Record.Type != typeFilter {
				continue
			}
			filtered = append(filtered, stored.Record)
		}
		if filtered == nil {
			filtered = []ResourceRecordSet{}
		}
		sort.Slice(filtered, func(i, j int) bool {
			if filtered[i].Name != filtered[j].Name {
				return filtered[i].Name < filtered[j].Name
			}
			return filtered[i].Type < filtered[j].Type
		})

		page, next, ok := paginateListCompute(w, r, filtered)
		if !ok {
			return
		}
		resp := map[string]any{
			"kind":   "dns#resourceRecordSetsListResponse",
			"rrsets": page,
		}
		if next != "" {
			resp["nextPageToken"] = next
		}
		sim.WriteJSON(w, http.StatusOK, resp)
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

		key := dnsRecordSetKey(project, zoneName, rs.Name, rs.Type)
		if _, exists := recordSets.Get(key); exists {
			sim.GCPErrorf(w, http.StatusConflict, "ALREADY_EXISTS", "record set %s/%s already exists", rs.Name, rs.Type)
			return
		}

		recordSets.Put(key, storedResourceRecordSet{Project: project, Zone: zoneName, Record: rs})

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

	// Get record set
	srv.HandleFunc("GET /dns/v1/projects/{project}/managedZones/{zone}/rrsets/{name}/{type}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		zoneName := sim.PathParam(r, "zone")
		rrName := sim.PathParam(r, "name")
		rrType := sim.PathParam(r, "type")
		if _, ok := zones.Get(project + "/" + zoneName); !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "managed zone %q not found", zoneName)
			return
		}
		stored, ok := recordSets.Get(dnsRecordSetKey(project, zoneName, rrName, rrType))
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "record set %s/%s not found", rrName, rrType)
			return
		}
		sim.WriteJSON(w, http.StatusOK, stored.Record)
	})

	srv.HandleFunc("DELETE /dns/v1/projects/{project}/managedZones/{zone}/rrsets/{name}/{type}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		zoneName := sim.PathParam(r, "zone")
		rrName := sim.PathParam(r, "name")
		rrType := sim.PathParam(r, "type")
		zoneKey := project + "/" + zoneName
		key := dnsRecordSetKey(project, zoneName, rrName, rrType)

		stored, rsOk := recordSets.Get(key)
		if !recordSets.Delete(key) {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "record set %s/%s not found", rrName, rrType)
			return
		}

		// Disconnect the container that was connected when the
		// record was created. Best-effort — container shutdown
		// already cleans up Docker-side network memberships.
		if rsOk && stored.Record.Type == "A" && len(stored.Record.Rrdatas) > 0 {
			if zone, ok := zones.Get(zoneKey); ok && zone.DockerNetworkName != "" {
				if containerName := sim.FindContainerByIP(stored.Record.Rrdatas[0]); containerName != "" {
					_ = sim.DisconnectContainerFromNetwork(containerName, zone.DockerNetworkName)
				}
			}
		}

		sim.WriteJSON(w, http.StatusOK, map[string]any{})
	})

	srv.HandleFunc("PATCH /dns/v1/projects/{project}/managedZones/{zone}/rrsets/{name}/{type}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		zoneName := sim.PathParam(r, "zone")
		rrName := sim.PathParam(r, "name")
		rrType := sim.PathParam(r, "type")
		zoneKey := project + "/" + zoneName
		if _, ok := zones.Get(zoneKey); !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "managed zone %q not found", zoneName)
			return
		}

		var patch ResourceRecordSet
		if err := sim.ReadJSON(r, &patch); err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
			return
		}
		key := dnsRecordSetKey(project, zoneName, rrName, rrType)
		stored, ok := recordSets.Get(key)
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "record set %s/%s not found", rrName, rrType)
			return
		}
		updated := stored.Record
		if patch.Name != "" {
			updated.Name = patch.Name
		}
		if patch.Type != "" {
			updated.Type = patch.Type
		}
		if patch.TTL != 0 {
			updated.TTL = patch.TTL
		}
		if patch.Rrdatas != nil {
			updated.Rrdatas = patch.Rrdatas
		}
		recordSets.Delete(key)
		recordSets.Put(dnsRecordSetKey(project, zoneName, updated.Name, updated.Type),
			storedResourceRecordSet{Project: project, Zone: zoneName, Record: updated})
		sim.WriteJSON(w, http.StatusOK, updated)
	})

	srv.HandleFunc("POST /dns/v1/projects/{project}/managedZones/{zone}/changes", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		zoneName := sim.PathParam(r, "zone")
		zoneKey := project + "/" + zoneName
		zone, ok := zones.Get(zoneKey)
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "managed zone %q not found", zoneName)
			return
		}

		var change DNSChange
		if err := sim.ReadJSON(r, &change); err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
			return
		}
		for _, deletion := range change.Deletions {
			key := dnsRecordSetKey(project, zoneName, deletion.Name, deletion.Type)
			stored, ok := recordSets.Get(key)
			if !ok {
				sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "record set %s/%s not found", deletion.Name, deletion.Type)
				return
			}
			if !dnsRecordSetsEqual(stored.Record, deletion) {
				writeDNSChangeError(w, http.StatusPreconditionFailed, "FAILED_PRECONDITION",
					fmt.Sprintf("record set %s/%s does not match existing data", deletion.Name, deletion.Type))
				return
			}
		}
		for _, addition := range change.Additions {
			if addition.Name == "" || addition.Type == "" {
				sim.GCPError(w, http.StatusBadRequest, "name and type are required", "INVALID_ARGUMENT")
				return
			}
			if _, exists := recordSets.Get(dnsRecordSetKey(project, zoneName, addition.Name, addition.Type)); exists &&
				!dnsChangeDeletesRecord(change, addition) {
				sim.GCPErrorf(w, http.StatusConflict, "ALREADY_EXISTS", "record set %s/%s already exists", addition.Name, addition.Type)
				return
			}
		}

		for _, deletion := range change.Deletions {
			key := dnsRecordSetKey(project, zoneName, deletion.Name, deletion.Type)
			recordSets.Delete(key)
			disconnectDNSRecordFromZone(zone, deletion)
		}
		for _, addition := range change.Additions {
			recordSets.Put(dnsRecordSetKey(project, zoneName, addition.Name, addition.Type),
				storedResourceRecordSet{Project: project, Zone: zoneName, Record: addition})
			connectDNSRecordToZone(zone, addition)
		}

		change.ID = nextDNSChangeID(changes, project, zoneName)
		change.StartTime = nowTimestamp()
		change.Status = "done"
		change.IsServing = true
		change.Kind = "dns#change"
		changes.Put(dnsChangeKey(project, zoneName, change.ID),
			storedDNSChange{Project: project, Zone: zoneName, Change: change})
		sim.WriteJSON(w, http.StatusOK, change)
	})

	srv.HandleFunc("GET /dns/v1/projects/{project}/managedZones/{zone}/changes/{change}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		zoneName := sim.PathParam(r, "zone")
		id := sim.PathParam(r, "change")
		if _, ok := zones.Get(project + "/" + zoneName); !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "managed zone %q not found", zoneName)
			return
		}
		stored, ok := changes.Get(dnsChangeKey(project, zoneName, id))
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "change %q not found", id)
			return
		}
		sim.WriteJSON(w, http.StatusOK, stored.Change)
	})

	srv.HandleFunc("GET /dns/v1/projects/{project}/managedZones/{zone}/changes", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		zoneName := sim.PathParam(r, "zone")
		if _, ok := zones.Get(project + "/" + zoneName); !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "managed zone %q not found", zoneName)
			return
		}
		var out []DNSChange
		for _, stored := range changes.List() {
			if stored.Project == project && stored.Zone == zoneName {
				out = append(out, stored.Change)
			}
		}
		sort.Slice(out, func(i, j int) bool {
			left, _ := strconv.Atoi(out[i].ID)
			right, _ := strconv.Atoi(out[j].ID)
			return left < right
		})
		if out == nil {
			out = []DNSChange{}
		}
		sim.WriteJSON(w, http.StatusOK, map[string]any{
			"kind":    "dns#changesListResponse",
			"changes": out,
		})
	})
}

func dnsRecordSetKey(project, zone, name, typ string) string {
	return fmt.Sprintf("%s/%s:%s:%s", project, zone, name, typ)
}

func writeDNSChangeError(w http.ResponseWriter, code int, status, message string) {
	sim.WriteJSON(w, code, map[string]any{
		"error": map[string]any{
			"code":    code,
			"message": message,
			"status":  status,
			"errors": []map[string]string{{
				"domain":  "global",
				"reason":  status,
				"message": message,
			}},
		},
	})
}

func dnsChangeKey(project, zone, id string) string {
	return fmt.Sprintf("%s/%s:%s", project, zone, id)
}

func nextDNSChangeID(changes sim.Store[storedDNSChange], project, zone string) string {
	maxID := 0
	for _, stored := range changes.List() {
		if stored.Project != project || stored.Zone != zone {
			continue
		}
		id, err := strconv.Atoi(stored.Change.ID)
		if err == nil && id > maxID {
			maxID = id
		}
	}
	return strconv.Itoa(maxID + 1)
}

func dnsChangeDeletesRecord(change DNSChange, addition ResourceRecordSet) bool {
	for _, deletion := range change.Deletions {
		if deletion.Name == addition.Name && deletion.Type == addition.Type {
			return true
		}
	}
	return false
}

func dnsRecordSetsEqual(a, b ResourceRecordSet) bool {
	return a.Name == b.Name &&
		a.Type == b.Type &&
		a.TTL == b.TTL &&
		reflect.DeepEqual(a.Rrdatas, b.Rrdatas)
}

func connectDNSRecordToZone(zone storedManagedZone, rs ResourceRecordSet) {
	if zone.DockerNetworkName == "" || rs.Type != "A" || len(rs.Rrdatas) == 0 {
		return
	}
	if containerName := sim.FindContainerByIP(rs.Rrdatas[0]); containerName != "" {
		alias := shortHostnameFromDNS(rs.Name, zone.DNSName)
		_ = sim.ConnectContainerToNetwork(containerName, zone.DockerNetworkName, []string{alias})
	}
}

func disconnectDNSRecordFromZone(zone storedManagedZone, rs ResourceRecordSet) {
	if zone.DockerNetworkName == "" || rs.Type != "A" || len(rs.Rrdatas) == 0 {
		return
	}
	if containerName := sim.FindContainerByIP(rs.Rrdatas[0]); containerName != "" {
		_ = sim.DisconnectContainerFromNetwork(containerName, zone.DockerNetworkName)
	}
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
