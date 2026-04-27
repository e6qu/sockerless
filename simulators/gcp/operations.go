package main

import (
	"fmt"
	"net/http"

	sim "github.com/sockerless/simulator"
)

func registerOperations(srv *sim.Server) {
	if crOperations == nil {
		crOperations = sim.MakeStore[Operation](srv.DB(), "operations")
	}

	// Get operation - v1 prefix
	srv.HandleFunc("GET /v1/projects/{project}/locations/{location}/operations/{operation}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		location := sim.PathParam(r, "location")
		opID := sim.PathParam(r, "operation")
		name := fmt.Sprintf("projects/%s/locations/%s/operations/%s", project, location, opID)

		if op, ok := crOperations.Get(name); ok {
			sim.WriteJSON(w, http.StatusOK, op)
			return
		}
		// Real GCP returns NOT_FOUND when an operation doesn't exist; the
		// previous synthetic done=true response masked client-side bugs.
		sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "operation %q not found", name)
	})

	// Get operation - v2 prefix
	srv.HandleFunc("GET /v2/projects/{project}/locations/{location}/operations/{operation}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		location := sim.PathParam(r, "location")
		opID := sim.PathParam(r, "operation")
		name := fmt.Sprintf("projects/%s/locations/%s/operations/%s", project, location, opID)

		if op, ok := crOperations.Get(name); ok {
			sim.WriteJSON(w, http.StatusOK, op)
			return
		}
		sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "operation %q not found", name)
	})
}
