package main

import (
	"fmt"
	"net/http"
	"strings"

	sim "github.com/sockerless/simulator"
)

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
}

var msRedisInstances sim.Store[MSRedisInstance]

func registerMemorystoreRedis(srv *sim.Server) {
	msRedisInstances = sim.MakeStore[MSRedisInstance](srv.DB(), "memorystore_redis")

	srv.HandleFunc("POST /v1/projects/{project}/locations/{location}/instances", handleMSRedisCreate)
	srv.HandleFunc("GET /v1/projects/{project}/locations/{location}/instances/{id}", handleMSRedisGet)
	srv.HandleFunc("GET /v1/projects/{project}/locations/{location}/instances", handleMSRedisList)
	srv.HandleFunc("PATCH /v1/projects/{project}/locations/{location}/instances/{id}", handleMSRedisPatch)
	srv.HandleFunc("DELETE /v1/projects/{project}/locations/{location}/instances/{id}", handleMSRedisDelete)
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
	_ = sim.ReadJSON(r, &req)
	inst := MSRedisInstance{
		Name:              msRedisInstanceName(project, location, id),
		DisplayName:       req.DisplayName,
		Tier:              defaultStr(req.Tier, "BASIC"),
		RedisVersion:      defaultStr(req.RedisVersion, "REDIS_7_0"),
		MemorySizeGb:      defaultInt(req.MemorySizeGb, 1),
		Host:              fmt.Sprintf("%s.redis.example", id),
		Port:              6379,
		State:             "READY",
		CreateTime:        nowTimestamp(),
		Labels:            req.Labels,
		AuthorizedNetwork: req.AuthorizedNetwork,
		RedisConfigs:      req.RedisConfigs,
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
	_ = sim.ReadJSON(r, &req)
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
