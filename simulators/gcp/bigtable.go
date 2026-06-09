package main

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	sim "github.com/sockerless/simulator"
)

type bigtableInstance struct {
	Name        string            `json:"name"`
	DisplayName string            `json:"displayName,omitempty"`
	State       string            `json:"state,omitempty"`
	Type        string            `json:"type,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
}

type bigtableCluster struct {
	Name               string `json:"name"`
	Location           string `json:"location,omitempty"`
	State              string `json:"state,omitempty"`
	ServeNodes         int    `json:"serveNodes,omitempty"`
	DefaultStorageType string `json:"defaultStorageType,omitempty"`
}

type bigtableTable struct {
	Name               string                    `json:"name"`
	ClusterStates      map[string]map[string]any `json:"clusterStates,omitempty"`
	ColumnFamilies     map[string]map[string]any `json:"columnFamilies,omitempty"`
	Granularity        string                    `json:"granularity,omitempty"`
	DeletionProtection bool                      `json:"deletionProtection,omitempty"`
}

var (
	bigtableInstances sim.Store[bigtableInstance]
	bigtableClusters  sim.Store[bigtableCluster]
	bigtableTables    sim.Store[bigtableTable]
)

func registerBigtable(srv *sim.Server) {
	bigtableInstances = sim.MakeStore[bigtableInstance](srv.DB(), "bigtable_instances")
	bigtableClusters = sim.MakeStore[bigtableCluster](srv.DB(), "bigtable_clusters")
	bigtableTables = sim.MakeStore[bigtableTable](srv.DB(), "bigtable_tables")

	srv.HandleFunc("POST /v2/projects/{project}/instances", handleBigtableCreateInstance)
	srv.HandleFunc("GET /v2/projects/{project}/instances", handleBigtableListInstances)
	srv.HandleFunc("GET /v2/projects/{project}/instances/{instance}", handleBigtableGetInstance)
	srv.HandleFunc("DELETE /v2/projects/{project}/instances/{instance}", handleBigtableDeleteInstance)

	srv.HandleFunc("POST /v2/projects/{project}/instances/{instance}/clusters", handleBigtableCreateCluster)
	srv.HandleFunc("GET /v2/projects/{project}/instances/{instance}/clusters", handleBigtableListClusters)
	srv.HandleFunc("GET /v2/projects/{project}/instances/{instance}/clusters/{cluster}", handleBigtableGetCluster)
	srv.HandleFunc("DELETE /v2/projects/{project}/instances/{instance}/clusters/{cluster}", handleBigtableDeleteCluster)

	srv.HandleFunc("POST /v2/projects/{project}/instances/{instance}/tables", handleBigtableCreateTable)
	srv.HandleFunc("GET /v2/projects/{project}/instances/{instance}/tables", handleBigtableListTables)
	srv.HandleFunc("GET /v2/projects/{project}/instances/{instance}/tables/{table}", handleBigtableGetTable)
	srv.HandleFunc("DELETE /v2/projects/{project}/instances/{instance}/tables/{table}", handleBigtableDeleteTable)
}

func bigtableInstanceName(project, instance string) string {
	return fmt.Sprintf("projects/%s/instances/%s", project, instance)
}

func bigtableClusterName(project, instance, cluster string) string {
	return fmt.Sprintf("%s/clusters/%s", bigtableInstanceName(project, instance), cluster)
}

func bigtableTableName(project, instance, table string) string {
	return fmt.Sprintf("%s/tables/%s", bigtableInstanceName(project, instance), table)
}

func handleBigtableCreateInstance(w http.ResponseWriter, r *http.Request) {
	project := sim.PathParam(r, "project")
	var req struct {
		InstanceID string                     `json:"instanceId"`
		Instance   bigtableInstance           `json:"instance"`
		Clusters   map[string]bigtableCluster `json:"clusters"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
		return
	}
	if req.InstanceID == "" {
		sim.GCPError(w, http.StatusBadRequest, "instanceId is required", "INVALID_ARGUMENT")
		return
	}
	inst := req.Instance
	inst.Name = bigtableInstanceName(project, req.InstanceID)
	if inst.DisplayName == "" {
		inst.DisplayName = req.InstanceID
	}
	if inst.Type == "" {
		inst.Type = "PRODUCTION"
	}
	inst.State = "READY"
	bigtableInstances.Put(inst.Name, inst)
	for id, cluster := range req.Clusters {
		if cluster.Location == "" {
			cluster.Location = fmt.Sprintf("projects/%s/locations/us-central1-a", project)
		}
		if cluster.ServeNodes == 0 {
			cluster.ServeNodes = 1
		}
		if cluster.DefaultStorageType == "" {
			cluster.DefaultStorageType = "SSD"
		}
		cluster.Name = bigtableClusterName(project, req.InstanceID, id)
		cluster.State = "READY"
		bigtableClusters.Put(cluster.Name, cluster)
	}
	op := newBigtableAdminLRO(project, inst, "type.googleapis.com/google.bigtable.admin.v2.Instance")
	sim.WriteJSON(w, http.StatusOK, op)
}

func newBigtableAdminLRO(project string, resource any, typeName string) Operation {
	op := newLRO(project, "global", resource, typeName)
	return renameGCPOperation(op, "operations")
}

func handleBigtableListInstances(w http.ResponseWriter, r *http.Request) {
	prefix := fmt.Sprintf("projects/%s/instances/", sim.PathParam(r, "project"))
	out := bigtableInstances.Filter(func(inst bigtableInstance) bool { return strings.HasPrefix(inst.Name, prefix) })
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	sim.WriteJSON(w, http.StatusOK, map[string]any{"instances": out})
}

func handleBigtableGetInstance(w http.ResponseWriter, r *http.Request) {
	name := bigtableInstanceName(sim.PathParam(r, "project"), sim.PathParam(r, "instance"))
	inst, ok := bigtableInstances.Get(name)
	if !ok {
		sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "instance %q not found", name)
		return
	}
	sim.WriteJSON(w, http.StatusOK, inst)
}

func handleBigtableDeleteInstance(w http.ResponseWriter, r *http.Request) {
	name := bigtableInstanceName(sim.PathParam(r, "project"), sim.PathParam(r, "instance"))
	if !bigtableInstances.Delete(name) {
		sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "instance %q not found", name)
		return
	}
	for _, cluster := range bigtableClusters.List() {
		if strings.HasPrefix(cluster.Name, name+"/clusters/") {
			bigtableClusters.Delete(cluster.Name)
		}
	}
	for _, table := range bigtableTables.List() {
		if strings.HasPrefix(table.Name, name+"/tables/") {
			bigtableTables.Delete(table.Name)
		}
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{})
}

func handleBigtableCreateCluster(w http.ResponseWriter, r *http.Request) {
	project, instance := sim.PathParam(r, "project"), sim.PathParam(r, "instance")
	if _, ok := bigtableInstances.Get(bigtableInstanceName(project, instance)); !ok {
		sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "instance %q not found", instance)
		return
	}
	clusterID := r.URL.Query().Get("clusterId")
	if clusterID == "" {
		sim.GCPError(w, http.StatusBadRequest, "clusterId is required", "INVALID_ARGUMENT")
		return
	}
	var cluster bigtableCluster
	if err := sim.ReadJSON(r, &cluster); err != nil {
		sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
		return
	}
	cluster.Name = bigtableClusterName(project, instance, clusterID)
	cluster.State = "READY"
	if cluster.DefaultStorageType == "" {
		cluster.DefaultStorageType = "SSD"
	}
	bigtableClusters.Put(cluster.Name, cluster)
	op := newBigtableAdminLRO(project, cluster, "type.googleapis.com/google.bigtable.admin.v2.Cluster")
	sim.WriteJSON(w, http.StatusOK, op)
}

func handleBigtableListClusters(w http.ResponseWriter, r *http.Request) {
	prefix := bigtableInstanceName(sim.PathParam(r, "project"), sim.PathParam(r, "instance")) + "/clusters/"
	out := bigtableClusters.Filter(func(cluster bigtableCluster) bool { return strings.HasPrefix(cluster.Name, prefix) })
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	sim.WriteJSON(w, http.StatusOK, map[string]any{"clusters": out})
}

func handleBigtableGetCluster(w http.ResponseWriter, r *http.Request) {
	name := bigtableClusterName(sim.PathParam(r, "project"), sim.PathParam(r, "instance"), sim.PathParam(r, "cluster"))
	cluster, ok := bigtableClusters.Get(name)
	if !ok {
		sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "cluster %q not found", name)
		return
	}
	sim.WriteJSON(w, http.StatusOK, cluster)
}

func handleBigtableDeleteCluster(w http.ResponseWriter, r *http.Request) {
	name := bigtableClusterName(sim.PathParam(r, "project"), sim.PathParam(r, "instance"), sim.PathParam(r, "cluster"))
	if !bigtableClusters.Delete(name) {
		sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "cluster %q not found", name)
		return
	}
	op := newBigtableAdminLRO(sim.PathParam(r, "project"), nil, "type.googleapis.com/google.protobuf.Empty")
	sim.WriteJSON(w, http.StatusOK, op)
}

func handleBigtableCreateTable(w http.ResponseWriter, r *http.Request) {
	project, instance := sim.PathParam(r, "project"), sim.PathParam(r, "instance")
	if _, ok := bigtableInstances.Get(bigtableInstanceName(project, instance)); !ok {
		sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "instance %q not found", instance)
		return
	}
	var req struct {
		TableID string        `json:"tableId"`
		Table   bigtableTable `json:"table"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
		return
	}
	if req.TableID == "" {
		sim.GCPError(w, http.StatusBadRequest, "tableId is required", "INVALID_ARGUMENT")
		return
	}
	table := req.Table
	table.Name = bigtableTableName(project, instance, req.TableID)
	if table.Granularity == "" {
		table.Granularity = "MILLIS"
	}
	if table.ColumnFamilies == nil {
		table.ColumnFamilies = map[string]map[string]any{}
	}
	if table.ClusterStates == nil {
		table.ClusterStates = map[string]map[string]any{}
	}
	for _, cluster := range bigtableClusters.List() {
		if strings.HasPrefix(cluster.Name, bigtableInstanceName(project, instance)+"/clusters/") {
			table.ClusterStates[cluster.Name] = map[string]any{"replicationState": "READY"}
		}
	}
	bigtableTables.Put(table.Name, table)
	sim.WriteJSON(w, http.StatusOK, table)
}

func handleBigtableListTables(w http.ResponseWriter, r *http.Request) {
	prefix := bigtableInstanceName(sim.PathParam(r, "project"), sim.PathParam(r, "instance")) + "/tables/"
	out := bigtableTables.Filter(func(table bigtableTable) bool { return strings.HasPrefix(table.Name, prefix) })
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	sim.WriteJSON(w, http.StatusOK, map[string]any{"tables": out})
}

func handleBigtableGetTable(w http.ResponseWriter, r *http.Request) {
	name := bigtableTableName(sim.PathParam(r, "project"), sim.PathParam(r, "instance"), sim.PathParam(r, "table"))
	table, ok := bigtableTables.Get(name)
	if !ok {
		sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "table %q not found", name)
		return
	}
	sim.WriteJSON(w, http.StatusOK, table)
}

func handleBigtableDeleteTable(w http.ResponseWriter, r *http.Request) {
	name := bigtableTableName(sim.PathParam(r, "project"), sim.PathParam(r, "instance"), sim.PathParam(r, "table"))
	if !bigtableTables.Delete(name) {
		sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "table %q not found", name)
		return
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{})
}
