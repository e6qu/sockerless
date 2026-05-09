package main

import (
	"net/http"
	"strings"
)

// registerTopologyAPI wires the sockerless.yaml topology surface.
//
// Routes (mirror of the admin URL convention used by other resources):
//
//	GET    /api/v1/topology
//	PUT    /api/v1/topology
//	GET    /api/v1/topology/instances
//	GET    /api/v1/topology/projects/{project}/instances/{instance}
//
// Lifecycle endpoints (start / stop / rebuild) wait for Phase 79 step 4
// when the per-component make targets are in place; this commit only
// ships the read + replace surface.
func registerTopologyAPI(mux *http.ServeMux, mgr *TopologyManager) {
	mux.HandleFunc("GET /api/v1/topology", handleTopologyGet(mgr))
	mux.HandleFunc("PUT /api/v1/topology", handleTopologyPut(mgr))
	mux.HandleFunc("GET /api/v1/topology/instances", handleTopologyInstances(mgr))
	mux.HandleFunc("GET /api/v1/topology/projects/{project}/instances/{instance}", handleInstanceGet(mgr))
}

func handleTopologyGet(mgr *TopologyManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t := mgr.Get()
		writeJSON(w, http.StatusOK, t)
	}
}

func handleTopologyPut(mgr *TopologyManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var next Topology
		if err := decodeJSON(r.Body, &next); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body: " + err.Error()})
			return
		}
		if err := mgr.Replace(next); err != nil {
			writeJSON(w, topologyReplaceStatus(err), map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, mgr.Get())
	}
}

func handleTopologyInstances(mgr *TopologyManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, mgr.Instances())
	}
}

func handleInstanceGet(mgr *TopologyManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		project := r.PathValue("project")
		instance := r.PathValue("instance")
		ref, ok := mgr.FindInstance(project, instance)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{
				"error": "instance " + project + "/" + instance + " not found",
			})
			return
		}
		writeJSON(w, http.StatusOK, ref)
	}
}

// topologyReplaceStatus maps Replace errors to HTTP status codes.
func topologyReplaceStatus(err error) int {
	if err == nil {
		return http.StatusOK
	}
	msg := err.Error()
	// Validation errors (duplicate / unknown / out-of-range) are caller
	// errors. Anything else (file write, validation that wasn't possible
	// to surface) is a server error.
	if strings.HasPrefix(msg, "validate:") {
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}
