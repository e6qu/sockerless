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
	_, _ = rand.Read(b)
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

// ComputeFirewall mirrors `compute#firewall`. Field set covers what
// terraform-provider-google's `google_compute_firewall` and the Go
// SDK's `compute.NewFirewallsRESTClient` round-trip; runner setup
// flows that grant ingress to the build host hit Create/Get/Delete.
type ComputeFirewall struct {
	Kind                  string                  `json:"kind,omitempty"`
	Id                    string                  `json:"id,omitempty"`
	Name                  string                  `json:"name"`
	SelfLink              string                  `json:"selfLink,omitempty"`
	CreationTimestamp     string                  `json:"creationTimestamp,omitempty"`
	Description           string                  `json:"description,omitempty"`
	Network               string                  `json:"network,omitempty"`
	Direction             string                  `json:"direction,omitempty"` // INGRESS / EGRESS
	Priority              int32                   `json:"priority,omitempty"`
	Disabled              bool                    `json:"disabled,omitempty"`
	SourceRanges          []string                `json:"sourceRanges,omitempty"`
	DestinationRanges     []string                `json:"destinationRanges,omitempty"`
	SourceTags            []string                `json:"sourceTags,omitempty"`
	TargetTags            []string                `json:"targetTags,omitempty"`
	SourceServiceAccounts []string                `json:"sourceServiceAccounts,omitempty"`
	TargetServiceAccounts []string                `json:"targetServiceAccounts,omitempty"`
	Allowed               []ComputeFirewallAction `json:"allowed,omitempty"`
	Denied                []ComputeFirewallAction `json:"denied,omitempty"`
	LogConfig             *ComputeFirewallLog     `json:"logConfig,omitempty"`
}

// ComputeFirewallAction is the allow/deny rule shape — protocol +
// optional port list. Matches `compute#firewallAllowed` / `Denied`.
type ComputeFirewallAction struct {
	IPProtocol string   `json:"IPProtocol"`
	Ports      []string `json:"ports,omitempty"`
}

// ComputeFirewallLog enables logging on a firewall rule.
type ComputeFirewallLog struct {
	Enable   bool   `json:"enable"`
	Metadata string `json:"metadata,omitempty"`
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
	networks := sim.MakeStore[ComputeNetwork](srv.DB(), "compute_networks")
	subnetworks := sim.MakeStore[ComputeSubnetwork](srv.DB(), "compute_subnetworks")

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
		if err := sim.ReadJSON(r, &req); err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
			return
		}

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
		if err := sim.ReadJSON(r, &req); err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
			return
		}

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
		if err := sim.ReadJSON(r, &req); err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
			return
		}

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

	// Firewalls — `compute#firewall` resource. Real GCP scopes firewall
	// rules to a Network (VPC) and tracks ingress/egress separately.
	// Sockerless workloads provision firewall rules to allow runner
	// traffic between VPCs and the build host; without this surface,
	// terraform's `google_compute_firewall` and runner setup scripts
	// hit a 404 against the sim.
	firewalls := sim.MakeStore[ComputeFirewall](srv.DB(), "compute_firewalls")

	srv.HandleFunc("POST /compute/v1/projects/{project}/global/firewalls", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		var fw ComputeFirewall
		if err := sim.ReadJSON(r, &fw); err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
			return
		}
		if fw.Name == "" {
			sim.GCPError(w, http.StatusBadRequest, "name is required", "INVALID_ARGUMENT")
			return
		}
		fw.Kind = "compute#firewall"
		fw.Id = computeNumericID()
		fw.SelfLink = fmt.Sprintf("projects/%s/global/firewalls/%s", project, fw.Name)
		fw.CreationTimestamp = time.Now().UTC().Format(time.RFC3339)
		if fw.Direction == "" {
			fw.Direction = "INGRESS"
		}
		if fw.Priority == 0 {
			fw.Priority = 1000
		}
		firewalls.Put(fw.SelfLink, fw)
		op := newComputeOp(project, "global", fw.SelfLink)
		sim.WriteJSON(w, http.StatusOK, op)
	})

	srv.HandleFunc("GET /compute/v1/projects/{project}/global/firewalls/{name}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		name := sim.PathParam(r, "name")
		selfLink := fmt.Sprintf("projects/%s/global/firewalls/%s", project, name)
		fw, ok := firewalls.Get(selfLink)
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "firewall %q not found", name)
			return
		}
		sim.WriteJSON(w, http.StatusOK, fw)
	})

	srv.HandleFunc("GET /compute/v1/projects/{project}/global/firewalls", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		prefix := fmt.Sprintf("projects/%s/global/firewalls/", project)
		all := firewalls.Filter(func(f ComputeFirewall) bool {
			return strings.HasPrefix(f.SelfLink, prefix)
		})
		if all == nil {
			all = []ComputeFirewall{}
		}
		sim.WriteJSON(w, http.StatusOK, map[string]any{
			"kind":  "compute#firewallList",
			"items": all,
		})
	})

	srv.HandleFunc("DELETE /compute/v1/projects/{project}/global/firewalls/{name}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		name := sim.PathParam(r, "name")
		selfLink := fmt.Sprintf("projects/%s/global/firewalls/%s", project, name)
		firewalls.Delete(selfLink)
		op := newComputeOp(project, "global", selfLink)
		sim.WriteJSON(w, http.StatusOK, op)
	})

	srv.HandleFunc("PATCH /compute/v1/projects/{project}/global/firewalls/{name}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		name := sim.PathParam(r, "name")
		selfLink := fmt.Sprintf("projects/%s/global/firewalls/%s", project, name)
		var patch ComputeFirewall
		if err := sim.ReadJSON(r, &patch); err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
			return
		}
		ok := firewalls.Update(selfLink, func(fw *ComputeFirewall) {
			if patch.Description != "" {
				fw.Description = patch.Description
			}
			if patch.SourceRanges != nil {
				fw.SourceRanges = patch.SourceRanges
			}
			if patch.SourceTags != nil {
				fw.SourceTags = patch.SourceTags
			}
			if patch.TargetTags != nil {
				fw.TargetTags = patch.TargetTags
			}
			if patch.Allowed != nil {
				fw.Allowed = patch.Allowed
			}
			if patch.Denied != nil {
				fw.Denied = patch.Denied
			}
			if patch.Direction != "" {
				fw.Direction = patch.Direction
			}
			if patch.Priority != 0 {
				fw.Priority = patch.Priority
			}
			fw.Disabled = patch.Disabled
		})
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "firewall %q not found", name)
			return
		}
		op := newComputeOp(project, "global", selfLink)
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
