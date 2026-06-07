package main

import (
	"fmt"
	"hash/fnv"
	"net/http"
	"strings"

	sim "github.com/sockerless/simulator"
)

// simRedisHost derives a deterministic RFC1918 address from the
// instance ID so terraform-provider-google reads + redis-cli probes
// see a syntactically valid IP rather than a `.example` placeholder.
func simRedisHost(id string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(id))
	v := h.Sum32()
	return fmt.Sprintf("10.%d.%d.%d", (v>>16)&0xff, (v>>8)&0xff, v&0xff)
}

// Cloud Memorystore for Redis v1 — REST surface scoped to instance
// lifecycle. Real API: https://redis.googleapis.com/$discovery/rest?version=v1
// The Redis engine itself is not simulated; the sim reports
// State=READY immediately after Create.

type MSRedisInstance struct {
	Name              string            `json:"name"` // projects/{p}/locations/{loc}/instances/{id}
	DisplayName       string            `json:"displayName,omitempty"`
	Tier              string            `json:"tier,omitempty"`
	RedisVersion      string            `json:"redisVersion,omitempty"`
	MemorySizeGb      int               `json:"memorySizeGb,omitempty"`
	Host              string            `json:"host,omitempty"`
	Port              int               `json:"port,omitempty"`
	State             string            `json:"state,omitempty"`
	CreateTime        string            `json:"createTime,omitempty"`
	Labels            map[string]string `json:"labels,omitempty"`
	AuthorizedNetwork string            `json:"authorizedNetwork,omitempty"`
	RedisConfigs      map[string]string `json:"redisConfigs,omitempty"`
	// connectMode + transitEncryptionMode have provider defaults; the read-back
	// must echo them or terraform-provider-google plans a replacement.
	ConnectMode           string `json:"connectMode,omitempty"`
	TransitEncryptionMode string `json:"transitEncryptionMode,omitempty"`
}

var msRedisInstances sim.Store[MSRedisInstance]

func registerMemorystoreRedis(srv *sim.Server) {
	msRedisInstances = sim.MakeStore[MSRedisInstance](srv.DB(), "memorystore_redis")

	srv.HandleFunc("POST /v1/projects/{project}/locations/{location}/instances", handleMSRedisCreate)
	srv.HandleFunc("GET /v1/projects/{project}/locations/{location}/instances/{id}", handleMSRedisGet)
	srv.HandleFunc("GET /v1/projects/{project}/locations/{location}/instances", handleMSRedisList)
	srv.HandleFunc("PATCH /v1/projects/{project}/locations/{location}/instances/{id}", handleMSRedisPatch)
	srv.HandleFunc("DELETE /v1/projects/{project}/locations/{location}/instances/{id}", handleMSRedisDelete)
	// Maintenance state machines:
	//   READY → UPGRADING → READY        (upgrade)
	//   READY → FAILING_OVER → READY     (failover)
	// Sim collapses both transitions inline (no async work to wait
	// on), but the State field is set + restored so SDKs reading
	// the instance during the LRO see a value other than zero.
	//
	// Go ServeMux can't parse `{id}:upgrade`; capture the action
	// suffix in a single wildcard and split on `:` in the handler.
	srv.HandleFunc("POST /v1/projects/{project}/locations/{location}/instances/{idAction}", handleMSRedisAction)
}

func handleMSRedisAction(w http.ResponseWriter, r *http.Request) {
	idAction := sim.PathParam(r, "idAction")
	id, action, found := strings.Cut(idAction, ":")
	if !found {
		sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND",
			"unknown action on memorystore instance %q", idAction)
		return
	}
	switch action {
	case "upgrade":
		handleMSRedisUpgrade(w, r, id)
	case "failover":
		handleMSRedisFailover(w, r, id)
	default:
		sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND",
			"unknown action %q on memorystore instance %q", action, id)
	}
}

func handleMSRedisUpgrade(w http.ResponseWriter, r *http.Request, id string) {
	project := sim.PathParam(r, "project")
	location := sim.PathParam(r, "location")
	key := msRedisInstanceName(project, location, id)
	inst, ok := msRedisInstances.Get(key)
	if !ok {
		sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND",
			"Memorystore instance %q not found", id)
		return
	}
	var req struct {
		RedisVersion string `json:"redisVersion"`
	}
	_ = sim.ReadJSON(r, &req)
	if req.RedisVersion != "" {
		inst.RedisVersion = req.RedisVersion
	}
	inst.State = "READY"
	msRedisInstances.Put(key, inst)
	now := nowTimestamp()
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"name":     "operations/upgrade-" + generateUUID(),
		"done":     true,
		"metadata": map[string]any{"operationType": "UPGRADE_INSTANCE", "startTime": now, "endTime": now},
		"response": inst,
	})
}

func handleMSRedisFailover(w http.ResponseWriter, r *http.Request, id string) {
	project := sim.PathParam(r, "project")
	location := sim.PathParam(r, "location")
	key := msRedisInstanceName(project, location, id)
	inst, ok := msRedisInstances.Get(key)
	if !ok {
		sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND",
			"Memorystore instance %q not found", id)
		return
	}
	inst.State = "READY"
	msRedisInstances.Put(key, inst)
	now := nowTimestamp()
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"name":     "operations/failover-" + generateUUID(),
		"done":     true,
		"metadata": map[string]any{"operationType": "FAILOVER_INSTANCE", "startTime": now, "endTime": now},
		"response": inst,
	})
}

func msRedisInstanceName(project, location, id string) string {
	return fmt.Sprintf("projects/%s/locations/%s/instances/%s", project, location, id)
}

func handleMSRedisCreate(w http.ResponseWriter, r *http.Request) {
	project := sim.PathParam(r, "project")
	location := sim.PathParam(r, "location")
	id := r.URL.Query().Get("instanceId")
	if id == "" {
		sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "instanceId query parameter is required")
		return
	}
	var req MSRedisInstance
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "%s", err.Error())
		return
	}
	inst := MSRedisInstance{
		Name:         msRedisInstanceName(project, location, id),
		DisplayName:  req.DisplayName,
		Tier:         defaultStr(req.Tier, "BASIC"),
		RedisVersion: defaultStr(req.RedisVersion, "REDIS_7_0"),
		MemorySizeGb: defaultInt(req.MemorySizeGb, 1),
		// Real Memorystore returns an RFC1918 address that the workload
		// connects to; emit a deterministic 10.x.x.x derived from the
		// instance ID so callers and terraform-provider-google reads
		// see a syntactically valid IP rather than a `.example` placeholder
		// that resolves to NXDOMAIN.
		Host:                  simRedisHost(id),
		Port:                  6379,
		State:                 "READY",
		CreateTime:            nowTimestamp(),
		Labels:                req.Labels,
		AuthorizedNetwork:     req.AuthorizedNetwork,
		RedisConfigs:          req.RedisConfigs,
		ConnectMode:           defaultStr(req.ConnectMode, "DIRECT_PEERING"),
		TransitEncryptionMode: defaultStr(req.TransitEncryptionMode, "DISABLED"),
	}
	msRedisInstances.Put(inst.Name, inst)
	op := newLRO(project, location, inst, "type.googleapis.com/google.cloud.redis.v1.Instance")
	sim.WriteJSON(w, http.StatusOK, op)
}

func handleMSRedisGet(w http.ResponseWriter, r *http.Request) {
	name := msRedisInstanceName(sim.PathParam(r, "project"), sim.PathParam(r, "location"), sim.PathParam(r, "id"))
	inst, ok := msRedisInstances.Get(name)
	if !ok {
		sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "instance not found: %s", name)
		return
	}
	sim.WriteJSON(w, http.StatusOK, inst)
}

func handleMSRedisList(w http.ResponseWriter, r *http.Request) {
	prefix := fmt.Sprintf("projects/%s/locations/%s/instances/", sim.PathParam(r, "project"), sim.PathParam(r, "location"))
	var out []MSRedisInstance
	for _, i := range msRedisInstances.List() {
		if strings.HasPrefix(i.Name, prefix) {
			out = append(out, i)
		}
	}
	if out == nil {
		out = []MSRedisInstance{}
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"instances": out})
}

func handleMSRedisPatch(w http.ResponseWriter, r *http.Request) {
	project := sim.PathParam(r, "project")
	location := sim.PathParam(r, "location")
	name := msRedisInstanceName(project, location, sim.PathParam(r, "id"))
	if _, ok := msRedisInstances.Get(name); !ok {
		sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "instance not found: %s", name)
		return
	}
	var req MSRedisInstance
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "%s", err.Error())
		return
	}
	msRedisInstances.Update(name, func(i *MSRedisInstance) {
		if req.DisplayName != "" {
			i.DisplayName = req.DisplayName
		}
		if req.MemorySizeGb != 0 {
			i.MemorySizeGb = req.MemorySizeGb
		}
		if req.Labels != nil {
			i.Labels = req.Labels
		}
		if req.RedisConfigs != nil {
			i.RedisConfigs = req.RedisConfigs
		}
	})
	updated, _ := msRedisInstances.Get(name)
	op := newLRO(project, location, updated, "type.googleapis.com/google.cloud.redis.v1.Instance")
	sim.WriteJSON(w, http.StatusOK, op)
}

func handleMSRedisDelete(w http.ResponseWriter, r *http.Request) {
	project := sim.PathParam(r, "project")
	location := sim.PathParam(r, "location")
	name := msRedisInstanceName(project, location, sim.PathParam(r, "id"))
	if !msRedisInstances.Delete(name) {
		sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "instance not found: %s", name)
		return
	}
	op := newLRO(project, location, nil, "type.googleapis.com/google.protobuf.Empty")
	sim.WriteJSON(w, http.StatusOK, op)
}

func defaultStr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
func defaultInt(v, def int) int {
	if v == 0 {
		return def
	}
	return v
}
