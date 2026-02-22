package main

import (
	"fmt"
	"net/http"

	sim "github.com/sockerless/simulator"
)

func registerOperations(srv *sim.Server) {
	operations := sim.NewStateStore[Operation]()

	// Get operation - v1 prefix
	srv.HandleFunc("GET /v1/projects/{project}/locations/{location}/operations/{operation}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		location := sim.PathParam(r, "location")
		opID := sim.PathParam(r, "operation")
		name := fmt.Sprintf("projects/%s/locations/%s/operations/%s", project, location, opID)

		op, ok := operations.Get(name)
		if !ok {
			// Since the simulator returns all operations as immediately done,
			// and we don't persist them, return a generic done operation.
			op = Operation{
				Name: name,
				Done: true,
			}
		}
		sim.WriteJSON(w, http.StatusOK, op)
	})

	// Get operation - v2 prefix
	srv.HandleFunc("GET /v2/projects/{project}/locations/{location}/operations/{operation}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		location := sim.PathParam(r, "location")
		opID := sim.PathParam(r, "operation")
		name := fmt.Sprintf("projects/%s/locations/%s/operations/%s", project, location, opID)

		op, ok := operations.Get(name)
		if !ok {
			// Return a generic done operation since all simulator operations complete immediately
			op = Operation{
				Name: name,
				Done: true,
			}
		}
		sim.WriteJSON(w, http.StatusOK, op)
	})
}
