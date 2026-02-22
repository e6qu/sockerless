package main

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"net/http"
	"strings"
	"time"

	sim "github.com/sockerless/simulator"
)

func computeNumericID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return fmt.Sprintf("%d", binary.BigEndian.Uint64(b)>>1)
}

func newComputeOp(project, scope string, targetLink string) map[string]any {
	opID := generateUUID()[:8]
	return map[string]any{
		"kind":       "compute#operation",
		"id":         computeNumericID(),
		"name":       "operation-" + opID,
		"status":     "DONE",
		"selfLink":   fmt.Sprintf("projects/%s/%s/operations/operation-%s", project, scope, opID),
		"targetLink": targetLink,
		"progress":   100,
	}
}

type ComputeNetwork struct {
	Kind                  string `json:"kind"`
	Id                    string `json:"id"`
	Name                  string `json:"name"`
	SelfLink              string `json:"selfLink"`
	AutoCreateSubnetworks bool   `json:"autoCreateSubnetworks"`
	RoutingConfig         struct {
		RoutingMode string `json:"routingMode"`
	} `json:"routingConfig"`
	CreationTimestamp string `json:"creationTimestamp"`
}

type ComputeSubnetwork struct {
	Kind                  string `json:"kind"`
	Id                    string `json:"id"`
	Name                  string `json:"name"`
	SelfLink              string `json:"selfLink"`
	Network               string `json:"network"`
	IpCidrRange           string `json:"ipCidrRange"`
	Region                string `json:"region"`
	GatewayAddress        string `json:"gatewayAddress"`
	PrivateIpGoogleAccess bool   `json:"privateIpGoogleAccess"`
	CreationTimestamp     string `json:"creationTimestamp"`
}

func registerCompute(srv *sim.Server) {
	networks := sim.NewStateStore[ComputeNetwork]()
	subnetworks := sim.NewStateStore[ComputeSubnetwork]()

	// Create network
	srv.HandleFunc("POST /compute/v1/projects/{project}/global/networks", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")

		var req struct {
			Name                  string `json:"name"`
			AutoCreateSubnetworks bool   `json:"autoCreateSubnetworks"`
			RoutingConfig         struct {
				RoutingMode string `json:"routingMode"`
			} `json:"routingConfig"`
		}
		sim.ReadJSON(r, &req)

		selfLink := fmt.Sprintf("projects/%s/global/networks/%s", project, req.Name)
		net := ComputeNetwork{
			Kind:                  "compute#network",
			Id:                    computeNumericID(),
			Name:                  req.Name,
			SelfLink:              selfLink,
			AutoCreateSubnetworks: req.AutoCreateSubnetworks,
			CreationTimestamp:     time.Now().UTC().Format(time.RFC3339),
		}
		net.RoutingConfig.RoutingMode = req.RoutingConfig.RoutingMode
		if net.RoutingConfig.RoutingMode == "" {
			net.RoutingConfig.RoutingMode = "REGIONAL"
		}
		networks.Put(selfLink, net)

		op := newComputeOp(project, "global", selfLink)
		sim.WriteJSON(w, http.StatusOK, op)
	})

	// Get network
	srv.HandleFunc("GET /compute/v1/projects/{project}/global/networks/{name}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		name := sim.PathParam(r, "name")
		selfLink := fmt.Sprintf("projects/%s/global/networks/%s", project, name)

		net, ok := networks.Get(selfLink)
		if !ok {
			sim.GCPErrorf(w, 404, "NOT_FOUND", "Network %s not found", name)
			return
		}
		sim.WriteJSON(w, http.StatusOK, net)
	})

	// List networks
	srv.HandleFunc("GET /compute/v1/projects/{project}/global/networks", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		prefix := fmt.Sprintf("projects/%s/global/networks/", project)

		items := networks.Filter(func(n ComputeNetwork) bool {
			return strings.HasPrefix(n.SelfLink, prefix)
		})
		if items == nil {
			items = []ComputeNetwork{}
		}

		sim.WriteJSON(w, http.StatusOK, map[string]any{
			"kind":  "compute#networkList",
			"items": items,
		})
	})

	// Delete network
	srv.HandleFunc("DELETE /compute/v1/projects/{project}/global/networks/{name}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		name := sim.PathParam(r, "name")
		selfLink := fmt.Sprintf("projects/%s/global/networks/%s", project, name)

		networks.Delete(selfLink)
		op := newComputeOp(project, "global", selfLink)
		sim.WriteJSON(w, http.StatusOK, op)
	})

	// Patch network (for updates)
	srv.HandleFunc("PATCH /compute/v1/projects/{project}/global/networks/{name}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		name := sim.PathParam(r, "name")
		selfLink := fmt.Sprintf("projects/%s/global/networks/%s", project, name)

		var req struct {
			RoutingConfig struct {
				RoutingMode string `json:"routingMode"`
			} `json:"routingConfig"`
		}
		sim.ReadJSON(r, &req)

		networks.Update(selfLink, func(n *ComputeNetwork) {
			if req.RoutingConfig.RoutingMode != "" {
				n.RoutingConfig.RoutingMode = req.RoutingConfig.RoutingMode
			}
		})

		op := newComputeOp(project, "global", selfLink)
		sim.WriteJSON(w, http.StatusOK, op)
	})

	// Create subnetwork
	srv.HandleFunc("POST /compute/v1/projects/{project}/regions/{region}/subnetworks", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		region := sim.PathParam(r, "region")

		var req struct {
			Name                  string `json:"name"`
			Network               string `json:"network"`
			IpCidrRange           string `json:"ipCidrRange"`
			PrivateIpGoogleAccess bool   `json:"privateIpGoogleAccess"`
		}
		sim.ReadJSON(r, &req)

		selfLink := fmt.Sprintf("projects/%s/regions/%s/subnetworks/%s", project, region, req.Name)
		subnet := ComputeSubnetwork{
			Kind:                  "compute#subnetwork",
			Id:                    computeNumericID(),
			Name:                  req.Name,
			SelfLink:              selfLink,
			Network:               req.Network,
			IpCidrRange:           req.IpCidrRange,
			Region:                fmt.Sprintf("projects/%s/regions/%s", project, region),
			GatewayAddress:        "10.0.0.1",
			PrivateIpGoogleAccess: req.PrivateIpGoogleAccess,
			CreationTimestamp:     time.Now().UTC().Format(time.RFC3339),
		}
		subnetworks.Put(selfLink, subnet)

		op := newComputeOp(project, "regions/"+region, selfLink)
		sim.WriteJSON(w, http.StatusOK, op)
	})

	// Get subnetwork
	srv.HandleFunc("GET /compute/v1/projects/{project}/regions/{region}/subnetworks/{name}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		region := sim.PathParam(r, "region")
		name := sim.PathParam(r, "name")
		selfLink := fmt.Sprintf("projects/%s/regions/%s/subnetworks/%s", project, region, name)

		subnet, ok := subnetworks.Get(selfLink)
		if !ok {
			sim.GCPErrorf(w, 404, "NOT_FOUND", "Subnetwork %s not found", name)
			return
		}
		sim.WriteJSON(w, http.StatusOK, subnet)
	})

	// Delete subnetwork
	srv.HandleFunc("DELETE /compute/v1/projects/{project}/regions/{region}/subnetworks/{name}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		region := sim.PathParam(r, "region")
		name := sim.PathParam(r, "name")
		selfLink := fmt.Sprintf("projects/%s/regions/%s/subnetworks/%s", project, region, name)

		subnetworks.Delete(selfLink)
		op := newComputeOp(project, "regions/"+region, selfLink)
		sim.WriteJSON(w, http.StatusOK, op)
	})

	// Global operations (for network creates, deletes, etc.)
	srv.HandleFunc("GET /compute/v1/projects/{project}/global/operations/{name}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		name := sim.PathParam(r, "name")
		sim.WriteJSON(w, http.StatusOK, map[string]any{
			"kind":     "compute#operation",
			"id":       computeNumericID(),
			"name":     name,
			"status":   "DONE",
			"selfLink": fmt.Sprintf("projects/%s/global/operations/%s", project, name),
			"progress": 100,
		})
	})

	// Regional operations (for subnetwork creates, deletes, etc.)
	srv.HandleFunc("GET /compute/v1/projects/{project}/regions/{region}/operations/{name}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		region := sim.PathParam(r, "region")
		name := sim.PathParam(r, "name")
		sim.WriteJSON(w, http.StatusOK, map[string]any{
			"kind":     "compute#operation",
			"id":       computeNumericID(),
			"name":     name,
			"status":   "DONE",
			"selfLink": fmt.Sprintf("projects/%s/regions/%s/operations/%s", project, region, name),
			"progress": 100,
		})
	})
}
