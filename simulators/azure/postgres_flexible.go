package main

import (
	"fmt"
	"net/http"
	"strings"

	sim "github.com/sockerless/simulator"
)

// Microsoft.DBforPostgreSQL/flexibleServers ARM control plane.
// Surface scoped to server-instance lifecycle + the immediately-
// nested Database + FirewallRule resources. Configurations are
// stubbed (real Azure exposes ~50 server-parameter knobs).

type PGFlexibleServer struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Type       string            `json:"type"`
	Location   string            `json:"location,omitempty"`
	Properties map[string]any    `json:"properties,omitempty"`
	Tags       map[string]string `json:"tags,omitempty"`
}

type PGDatabase struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	Type       string         `json:"type"`
	Properties map[string]any `json:"properties,omitempty"`
}

type PGFirewallRule struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	Type       string         `json:"type"`
	Properties map[string]any `json:"properties,omitempty"`
}

var (
	pgServers       sim.Store[PGFlexibleServer]
	pgDatabases     sim.Store[PGDatabase]
	pgFirewallRules sim.Store[PGFirewallRule]
)

func registerPGFlexibleServer(srv *sim.Server) {
	pgServers = sim.MakeStore[PGFlexibleServer](srv.DB(), "pg_servers")
	pgDatabases = sim.MakeStore[PGDatabase](srv.DB(), "pg_databases")
	pgFirewallRules = sim.MakeStore[PGFirewallRule](srv.DB(), "pg_firewall_rules")

	const armBase = "/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.DBforPostgreSQL/flexibleServers"

	srv.HandleFunc("PUT "+armBase+"/{name}", handlePGCreateServer)
	srv.HandleFunc("GET "+armBase+"/{name}", handlePGGetServer)
	srv.HandleFunc("DELETE "+armBase+"/{name}", handlePGDeleteServer)
	srv.HandleFunc("GET "+armBase, handlePGListServersByRG)

	srv.HandleFunc("PUT "+armBase+"/{name}/databases/{db}", handlePGCreateDatabase)
	srv.HandleFunc("GET "+armBase+"/{name}/databases/{db}", handlePGGetDatabase)
	srv.HandleFunc("DELETE "+armBase+"/{name}/databases/{db}", handlePGDeleteDatabase)
	srv.HandleFunc("GET "+armBase+"/{name}/databases", handlePGListDatabases)

	srv.HandleFunc("PUT "+armBase+"/{name}/firewallRules/{rule}", handlePGCreateFirewallRule)
	srv.HandleFunc("GET "+armBase+"/{name}/firewallRules/{rule}", handlePGGetFirewallRule)
	srv.HandleFunc("DELETE "+armBase+"/{name}/firewallRules/{rule}", handlePGDeleteFirewallRule)
	srv.HandleFunc("GET "+armBase+"/{name}/firewallRules", handlePGListFirewallRules)
}

func pgServerID(sub, rg, name string) string {
	return fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.DBforPostgreSQL/flexibleServers/%s", sub, rg, name)
}

func handlePGCreateServer(w http.ResponseWriter, r *http.Request) {
	sub := sim.PathParam(r, "subscriptionId")
	rg := sim.PathParam(r, "resourceGroupName")
	name := sim.PathParam(r, "name")
	var req PGFlexibleServer
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AzureErrorf(w, "BadRequest", http.StatusBadRequest, "invalid request body: %v", err)
		return
	}
	id := pgServerID(sub, rg, name)
	s := PGFlexibleServer{
		ID:       id,
		Name:     name,
		Type:     "Microsoft.DBforPostgreSQL/flexibleServers",
		Location: req.Location,
		Tags:     req.Tags,
		Properties: map[string]any{
			"state":                    "Ready",
			"version":                  "15",
			"fullyQualifiedDomainName": name + ".postgres.database.azure.com",
			"administratorLogin":       "psqladmin",
			"storage": map[string]any{
				"storageSizeGB": 32,
			},
			"backup": map[string]any{
				"backupRetentionDays": 7,
				"geoRedundantBackup":  "Disabled",
			},
		},
	}
	if req.Properties != nil {
		for k, v := range req.Properties {
			s.Properties[k] = v
		}
		s.Properties["state"] = "Ready"
	}
	pgServers.Put(id, s)
	sim.WriteJSON(w, http.StatusOK, s)
}

func handlePGGetServer(w http.ResponseWriter, r *http.Request) {
	id := pgServerID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "name"))
	s, ok := pgServers.Get(id)
	if !ok {
		sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "server not found: %s", id)
		return
	}
	sim.WriteJSON(w, http.StatusOK, s)
}

func handlePGDeleteServer(w http.ResponseWriter, r *http.Request) {
	id := pgServerID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "name"))
	if !pgServers.Delete(id) {
		sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "server not found: %s", id)
		return
	}
	// Cascade-delete owned databases + firewall rules.
	prefix := id + "/"
	for _, d := range pgDatabases.List() {
		if strings.HasPrefix(d.ID, prefix) {
			pgDatabases.Delete(d.ID)
		}
	}
	for _, fr := range pgFirewallRules.List() {
		if strings.HasPrefix(fr.ID, prefix) {
			pgFirewallRules.Delete(fr.ID)
		}
	}
	w.WriteHeader(http.StatusAccepted)
}

func handlePGListServersByRG(w http.ResponseWriter, r *http.Request) {
	sub := sim.PathParam(r, "subscriptionId")
	rg := sim.PathParam(r, "resourceGroupName")
	prefix := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.DBforPostgreSQL/flexibleServers/", sub, rg)
	var out []PGFlexibleServer
	for _, s := range pgServers.List() {
		if strings.HasPrefix(s.ID, prefix) {
			out = append(out, s)
		}
	}
	if out == nil {
		out = []PGFlexibleServer{}
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"value": out})
}

func handlePGCreateDatabase(w http.ResponseWriter, r *http.Request) {
	parent := pgServerID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "name"))
	if _, ok := pgServers.Get(parent); !ok {
		sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "server not found")
		return
	}
	dbName := sim.PathParam(r, "db")
	var req PGDatabase
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AzureErrorf(w, "BadRequest", http.StatusBadRequest, "invalid request body: %v", err)
		return
	}
	id := parent + "/databases/" + dbName
	d := PGDatabase{
		ID:   id,
		Name: dbName,
		Type: "Microsoft.DBforPostgreSQL/flexibleServers/databases",
		Properties: map[string]any{
			"charset":   "UTF8",
			"collation": "en_US.utf8",
		},
	}
	if req.Properties != nil {
		for k, v := range req.Properties {
			d.Properties[k] = v
		}
	}
	pgDatabases.Put(id, d)
	sim.WriteJSON(w, http.StatusOK, d)
}

func handlePGGetDatabase(w http.ResponseWriter, r *http.Request) {
	id := pgServerID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "name")) +
		"/databases/" + sim.PathParam(r, "db")
	d, ok := pgDatabases.Get(id)
	if !ok {
		sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "database not found")
		return
	}
	sim.WriteJSON(w, http.StatusOK, d)
}

func handlePGDeleteDatabase(w http.ResponseWriter, r *http.Request) {
	id := pgServerID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "name")) +
		"/databases/" + sim.PathParam(r, "db")
	if !pgDatabases.Delete(id) {
		sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "database not found")
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func handlePGListDatabases(w http.ResponseWriter, r *http.Request) {
	prefix := pgServerID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "name")) + "/databases/"
	var out []PGDatabase
	for _, d := range pgDatabases.List() {
		if strings.HasPrefix(d.ID, prefix) {
			out = append(out, d)
		}
	}
	if out == nil {
		out = []PGDatabase{}
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"value": out})
}

func handlePGCreateFirewallRule(w http.ResponseWriter, r *http.Request) {
	parent := pgServerID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "name"))
	if _, ok := pgServers.Get(parent); !ok {
		sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "server not found")
		return
	}
	ruleName := sim.PathParam(r, "rule")
	var req PGFirewallRule
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AzureErrorf(w, "BadRequest", http.StatusBadRequest, "invalid request body: %v", err)
		return
	}
	id := parent + "/firewallRules/" + ruleName
	fr := PGFirewallRule{
		ID:         id,
		Name:       ruleName,
		Type:       "Microsoft.DBforPostgreSQL/flexibleServers/firewallRules",
		Properties: req.Properties,
	}
	pgFirewallRules.Put(id, fr)
	sim.WriteJSON(w, http.StatusOK, fr)
}

func handlePGGetFirewallRule(w http.ResponseWriter, r *http.Request) {
	id := pgServerID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "name")) +
		"/firewallRules/" + sim.PathParam(r, "rule")
	fr, ok := pgFirewallRules.Get(id)
	if !ok {
		sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "firewall rule not found")
		return
	}
	sim.WriteJSON(w, http.StatusOK, fr)
}

func handlePGDeleteFirewallRule(w http.ResponseWriter, r *http.Request) {
	id := pgServerID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "name")) +
		"/firewallRules/" + sim.PathParam(r, "rule")
	if !pgFirewallRules.Delete(id) {
		sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "firewall rule not found")
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func handlePGListFirewallRules(w http.ResponseWriter, r *http.Request) {
	prefix := pgServerID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "name")) + "/firewallRules/"
	var out []PGFirewallRule
	for _, fr := range pgFirewallRules.List() {
		if strings.HasPrefix(fr.ID, prefix) {
			out = append(out, fr)
		}
	}
	if out == nil {
		out = []PGFirewallRule{}
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"value": out})
}
