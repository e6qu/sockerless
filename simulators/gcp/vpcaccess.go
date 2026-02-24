package main

import (
	"fmt"
	"net/http"

	sim "github.com/sockerless/simulator"
)

type VPCAccessConnector struct {
	Name          string `json:"name"`
	Network       string `json:"network"`
	IpCidrRange   string `json:"ipCidrRange"`
	Subnet        *struct {
		Name      string `json:"name,omitempty"`
		ProjectId string `json:"projectId,omitempty"`
	} `json:"subnet,omitempty"`
	MachineType   string `json:"machineType"`
	MinInstances  int    `json:"minInstances"`
	MaxInstances  int    `json:"maxInstances"`
	MinThroughput int    `json:"minThroughput"`
	MaxThroughput int    `json:"maxThroughput"`
	State         string `json:"state"`
}

func registerVPCAccess(srv *sim.Server) {
	connectors := sim.NewStateStore[VPCAccessConnector]()

	// Create connector
	srv.HandleFunc("POST /v1/projects/{project}/locations/{location}/connectors", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		location := sim.PathParam(r, "location")

		var req VPCAccessConnector
		if err := sim.ReadJSON(r, &req); err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
			return
		}

		connectorId := r.URL.Query().Get("connectorId")
		if connectorId == "" {
			connectorId = "connector"
		}

		name := fmt.Sprintf("projects/%s/locations/%s/connectors/%s", project, location, connectorId)
		req.Name = name
		req.State = "READY"
		if req.MachineType == "" {
			req.MachineType = "e2-micro"
		}
		if req.MinInstances == 0 {
			req.MinInstances = 2
		}
		if req.MaxInstances == 0 {
			req.MaxInstances = 3
		}
		if req.MinThroughput == 0 {
			req.MinThroughput = 200
		}
		if req.MaxThroughput == 0 {
			req.MaxThroughput = 300
		}

		connectors.Put(name, req)

		op := newLRO(project, location, req, "type.googleapis.com/google.cloud.vpcaccess.v1.Connector")
		sim.WriteJSON(w, http.StatusOK, op)
	})

	// Get connector
	srv.HandleFunc("GET /v1/projects/{project}/locations/{location}/connectors/{name}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		location := sim.PathParam(r, "location")
		connName := sim.PathParam(r, "name")
		name := fmt.Sprintf("projects/%s/locations/%s/connectors/%s", project, location, connName)

		conn, ok := connectors.Get(name)
		if !ok {
			sim.GCPErrorf(w, 404, "NOT_FOUND", "Connector %s not found", connName)
			return
		}
		sim.WriteJSON(w, http.StatusOK, conn)
	})

	// Delete connector
	srv.HandleFunc("DELETE /v1/projects/{project}/locations/{location}/connectors/{name}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		location := sim.PathParam(r, "location")
		connName := sim.PathParam(r, "name")
		name := fmt.Sprintf("projects/%s/locations/%s/connectors/%s", project, location, connName)

		connectors.Delete(name)
		op := newLRO(project, location, nil, "type.googleapis.com/google.cloud.vpcaccess.v1.OperationMetadata")
		sim.WriteJSON(w, http.StatusOK, op)
	})

	// List connectors
	srv.HandleFunc("GET /v1/projects/{project}/locations/{location}/connectors", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		location := sim.PathParam(r, "location")
		prefix := fmt.Sprintf("projects/%s/locations/%s/connectors/", project, location)

		conns := connectors.Filter(func(c VPCAccessConnector) bool {
			return len(c.Name) > len(prefix) && c.Name[:len(prefix)] == prefix
		})

		sim.WriteJSON(w, http.StatusOK, map[string]any{
			"connectors": conns,
		})
	})
}
