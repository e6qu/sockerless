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

// ComputeRouter mirrors `compute#router`. Cloud NAT lives on a Router
// — a workload that needs serverless egress (Cloud Run, Cloud
// Functions with VPC connector) provisions a Router with a `nats[]`
// entry. Without router CRUD, terraform's `google_compute_router` and
// `google_compute_router_nat` 404 against the sim.
type ComputeRouter struct {
	Kind              string             `json:"kind,omitempty"`
	Id                string             `json:"id,omitempty"`
	Name              string             `json:"name"`
	SelfLink          string             `json:"selfLink,omitempty"`
	CreationTimestamp string             `json:"creationTimestamp,omitempty"`
	Description       string             `json:"description,omitempty"`
	Network           string             `json:"network,omitempty"`
	Region            string             `json:"region,omitempty"`
	Bgp               *ComputeRouterBgp  `json:"bgp,omitempty"`
	Nats              []ComputeRouterNAT `json:"nats,omitempty"`
}

// ComputeRouterBgp mirrors `compute#router.bgp` — the Border Gateway
// Protocol settings for the router. Sockerless's Cloud NAT use case
// doesn't need real BGP routing, just round-trip storage.
type ComputeRouterBgp struct {
	Asn               int32  `json:"asn,omitempty"`
	AdvertiseMode     string `json:"advertiseMode,omitempty"`
	KeepaliveInterval int32  `json:"keepaliveInterval,omitempty"`
}

// ComputeRouterNAT mirrors `compute#routerNat` — a Cloud NAT config
// embedded in a router. Real GCP supports per-NAT IP allocation
// (auto/manual), source-subnetwork-IP-ranges-to-NAT (LIST_OF_SUBNET-
// WORKS / ALL_SUBNETWORKS_ALL_IP_RANGES), TCP/UDP timeout overrides.
// The fields below cover what `google_compute_router_nat` round-trips.
type ComputeRouterNAT struct {
	Name                          string                       `json:"name"`
	NatIpAllocateOption           string                       `json:"natIpAllocateOption,omitempty"`
	NatIps                        []string                     `json:"natIps,omitempty"`
	SourceSubnetworkIpRangesToNat string                       `json:"sourceSubnetworkIpRangesToNat,omitempty"`
	Subnetworks                   []ComputeRouterNATSubnetwork `json:"subnetworks,omitempty"`
	MinPortsPerVm                 int32                        `json:"minPortsPerVm,omitempty"`
	UdpIdleTimeoutSec             int32                        `json:"udpIdleTimeoutSec,omitempty"`
	TcpEstablishedIdleTimeoutSec  int32                        `json:"tcpEstablishedIdleTimeoutSec,omitempty"`
	IcmpIdleTimeoutSec            int32                        `json:"icmpIdleTimeoutSec,omitempty"`
	LogConfig                     *ComputeRouterNATLogConfig   `json:"logConfig,omitempty"`
}

// ComputeRouterNATSubnetwork picks a specific subnet for NAT'ing.
type ComputeRouterNATSubnetwork struct {
	Name                  string   `json:"name"`
	SourceIpRangesToNat   []string `json:"sourceIpRangesToNat,omitempty"`
	SecondaryIpRangeNames []string `json:"secondaryIpRangeNames,omitempty"`
}

// ComputeRouterNATLogConfig enables NAT logging.
type ComputeRouterNATLogConfig struct {
	Enable bool   `json:"enable"`
	Filter string `json:"filter,omitempty"`
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

// ComputeDisk mirrors `compute#disk` — the zonal persistent-disk
// resource. Phase 127's `pd-ephemeral` storage driver provisions one
// disk per runner-task and attaches it to the runner's compute
// instance for the duration of the task. Field set covers what the Go
// SDK's `compute.NewDisksRESTClient` round-trips for create / get /
// list / delete / resize / setLabels — the subset terraform's
// `google_compute_disk` exercises.
type ComputeDisk struct {
	Kind              string            `json:"kind,omitempty"`
	Id                string            `json:"id,omitempty"`
	Name              string            `json:"name"`
	SelfLink          string            `json:"selfLink,omitempty"`
	CreationTimestamp string            `json:"creationTimestamp,omitempty"`
	Description       string            `json:"description,omitempty"`
	SizeGb            string            `json:"sizeGb,omitempty"`
	Zone              string            `json:"zone,omitempty"`
	Status            string            `json:"status,omitempty"`
	Type              string            `json:"type,omitempty"`
	SourceImage       string            `json:"sourceImage,omitempty"`
	SourceImageId     string            `json:"sourceImageId,omitempty"`
	SourceSnapshot    string            `json:"sourceSnapshot,omitempty"`
	Users             []string          `json:"users,omitempty"`
	Labels            map[string]string `json:"labels,omitempty"`
	LabelFingerprint  string            `json:"labelFingerprint,omitempty"`
	PhysicalBlockSize string            `json:"physicalBlockSizeBytes,omitempty"`
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

	// Routers + Cloud NAT — `compute#router` is a regional resource;
	// Cloud NAT configs are embedded in `router.nats[]`. Sockerless's
	// serverless egress flows (Cloud Run / Cloud Functions reaching
	// Internet via a VPC connector) provision a Router with a NAT;
	// without these handlers, terraform's `google_compute_router` and
	// `google_compute_router_nat` 404.
	routers := sim.MakeStore[ComputeRouter](srv.DB(), "compute_routers")

	srv.HandleFunc("POST /compute/v1/projects/{project}/regions/{region}/routers", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		region := sim.PathParam(r, "region")
		var rt ComputeRouter
		if err := sim.ReadJSON(r, &rt); err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
			return
		}
		if rt.Name == "" {
			sim.GCPError(w, http.StatusBadRequest, "name is required", "INVALID_ARGUMENT")
			return
		}
		rt.Kind = "compute#router"
		rt.Id = computeNumericID()
		rt.SelfLink = fmt.Sprintf("projects/%s/regions/%s/routers/%s", project, region, rt.Name)
		rt.Region = fmt.Sprintf("projects/%s/regions/%s", project, region)
		rt.CreationTimestamp = time.Now().UTC().Format(time.RFC3339)
		routers.Put(rt.SelfLink, rt)
		op := newComputeOp(project, "regions/"+region, rt.SelfLink)
		sim.WriteJSON(w, http.StatusOK, op)
	})

	srv.HandleFunc("GET /compute/v1/projects/{project}/regions/{region}/routers/{name}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		region := sim.PathParam(r, "region")
		name := sim.PathParam(r, "name")
		selfLink := fmt.Sprintf("projects/%s/regions/%s/routers/%s", project, region, name)
		rt, ok := routers.Get(selfLink)
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "router %q not found", name)
			return
		}
		sim.WriteJSON(w, http.StatusOK, rt)
	})

	srv.HandleFunc("GET /compute/v1/projects/{project}/regions/{region}/routers", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		region := sim.PathParam(r, "region")
		prefix := fmt.Sprintf("projects/%s/regions/%s/routers/", project, region)
		all := routers.Filter(func(rt ComputeRouter) bool {
			return strings.HasPrefix(rt.SelfLink, prefix)
		})
		if all == nil {
			all = []ComputeRouter{}
		}
		sim.WriteJSON(w, http.StatusOK, map[string]any{
			"kind":  "compute#routerList",
			"items": all,
		})
	})

	srv.HandleFunc("DELETE /compute/v1/projects/{project}/regions/{region}/routers/{name}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		region := sim.PathParam(r, "region")
		name := sim.PathParam(r, "name")
		selfLink := fmt.Sprintf("projects/%s/regions/%s/routers/%s", project, region, name)
		routers.Delete(selfLink)
		op := newComputeOp(project, "regions/"+region, selfLink)
		sim.WriteJSON(w, http.StatusOK, op)
	})

	srv.HandleFunc("PATCH /compute/v1/projects/{project}/regions/{region}/routers/{name}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		region := sim.PathParam(r, "region")
		name := sim.PathParam(r, "name")
		selfLink := fmt.Sprintf("projects/%s/regions/%s/routers/%s", project, region, name)
		var patch ComputeRouter
		if err := sim.ReadJSON(r, &patch); err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
			return
		}
		ok := routers.Update(selfLink, func(rt *ComputeRouter) {
			if patch.Description != "" {
				rt.Description = patch.Description
			}
			if patch.Network != "" {
				rt.Network = patch.Network
			}
			if patch.Bgp != nil {
				rt.Bgp = patch.Bgp
			}
			if patch.Nats != nil {
				rt.Nats = patch.Nats
			}
		})
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "router %q not found", name)
			return
		}
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

	registerComputeDisks(srv)
}

// registerComputeDisks wires the zonal Compute Disks REST surface that
// Phase 127's `pd-ephemeral` storage driver provisions against. Real
// GCP exposes Disks via `compute#disk` at
// `/compute/v1/projects/{p}/zones/{z}/disks` plus an aggregated list
// across zones at `/compute/v1/projects/{p}/aggregated/disks`. The
// sim mirrors create / get / list / delete / resize / setLabels +
// aggregated-list, all returning zonal operations the SDK polls.
func registerComputeDisks(srv *sim.Server) {
	disks := sim.MakeStore[ComputeDisk](srv.DB(), "compute_disks")

	zoneOp := func(project, zone, target string) map[string]any {
		opID := generateUUID()[:8]
		return map[string]any{
			"kind":       "compute#operation",
			"id":         computeNumericID(),
			"name":       "operation-" + opID,
			"status":     "DONE",
			"selfLink":   fmt.Sprintf("projects/%s/zones/%s/operations/operation-%s", project, zone, opID),
			"targetLink": target,
			"progress":   100,
		}
	}

	// Insert (create disk) — POST .../zones/{zone}/disks
	srv.HandleFunc("POST /compute/v1/projects/{project}/zones/{zone}/disks", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		zone := sim.PathParam(r, "zone")
		var d ComputeDisk
		if err := sim.ReadJSON(r, &d); err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
			return
		}
		if d.Name == "" {
			sim.GCPError(w, http.StatusBadRequest, "name is required", "INVALID_ARGUMENT")
			return
		}
		d.Kind = "compute#disk"
		d.Id = computeNumericID()
		d.SelfLink = fmt.Sprintf("projects/%s/zones/%s/disks/%s", project, zone, d.Name)
		d.Zone = fmt.Sprintf("projects/%s/zones/%s", project, zone)
		d.CreationTimestamp = time.Now().UTC().Format(time.RFC3339)
		d.Status = "READY"
		if d.SizeGb == "" {
			d.SizeGb = "10"
		}
		if d.Type == "" {
			d.Type = fmt.Sprintf("projects/%s/zones/%s/diskTypes/pd-standard", project, zone)
		}
		d.LabelFingerprint = generateUUID()[:8]
		disks.Put(d.SelfLink, d)
		sim.WriteJSON(w, http.StatusOK, zoneOp(project, zone, d.SelfLink))
	})

	// Get
	srv.HandleFunc("GET /compute/v1/projects/{project}/zones/{zone}/disks/{name}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		zone := sim.PathParam(r, "zone")
		name := sim.PathParam(r, "name")
		selfLink := fmt.Sprintf("projects/%s/zones/%s/disks/%s", project, zone, name)
		d, ok := disks.Get(selfLink)
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "disk %q not found in zone %q", name, zone)
			return
		}
		sim.WriteJSON(w, http.StatusOK, d)
	})

	// List (zonal)
	srv.HandleFunc("GET /compute/v1/projects/{project}/zones/{zone}/disks", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		zone := sim.PathParam(r, "zone")
		prefix := fmt.Sprintf("projects/%s/zones/%s/disks/", project, zone)
		items := disks.Filter(func(d ComputeDisk) bool {
			return strings.HasPrefix(d.SelfLink, prefix)
		})
		if items == nil {
			items = []ComputeDisk{}
		}
		sim.WriteJSON(w, http.StatusOK, map[string]any{
			"kind":  "compute#diskList",
			"items": items,
		})
	})

	// Delete
	srv.HandleFunc("DELETE /compute/v1/projects/{project}/zones/{zone}/disks/{name}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		zone := sim.PathParam(r, "zone")
		name := sim.PathParam(r, "name")
		selfLink := fmt.Sprintf("projects/%s/zones/%s/disks/%s", project, zone, name)
		if _, ok := disks.Get(selfLink); !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "disk %q not found in zone %q", name, zone)
			return
		}
		disks.Delete(selfLink)
		sim.WriteJSON(w, http.StatusOK, zoneOp(project, zone, selfLink))
	})

	// Resize — POST .../disks/{name}/resize with body {sizeGb}
	srv.HandleFunc("POST /compute/v1/projects/{project}/zones/{zone}/disks/{name}/resize", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		zone := sim.PathParam(r, "zone")
		name := sim.PathParam(r, "name")
		selfLink := fmt.Sprintf("projects/%s/zones/%s/disks/%s", project, zone, name)
		var req struct {
			SizeGb string `json:"sizeGb"`
		}
		if err := sim.ReadJSON(r, &req); err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
			return
		}
		ok := disks.Update(selfLink, func(d *ComputeDisk) {
			if req.SizeGb != "" {
				d.SizeGb = req.SizeGb
			}
		})
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "disk %q not found in zone %q", name, zone)
			return
		}
		sim.WriteJSON(w, http.StatusOK, zoneOp(project, zone, selfLink))
	})

	// SetLabels — POST .../disks/{name}/setLabels with body {labels, labelFingerprint}
	srv.HandleFunc("POST /compute/v1/projects/{project}/zones/{zone}/disks/{name}/setLabels", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		zone := sim.PathParam(r, "zone")
		name := sim.PathParam(r, "name")
		selfLink := fmt.Sprintf("projects/%s/zones/%s/disks/%s", project, zone, name)
		var req struct {
			Labels           map[string]string `json:"labels"`
			LabelFingerprint string            `json:"labelFingerprint"`
		}
		if err := sim.ReadJSON(r, &req); err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
			return
		}
		ok := disks.Update(selfLink, func(d *ComputeDisk) {
			d.Labels = req.Labels
			d.LabelFingerprint = generateUUID()[:8]
		})
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "disk %q not found in zone %q", name, zone)
			return
		}
		sim.WriteJSON(w, http.StatusOK, zoneOp(project, zone, selfLink))
	})

	// Aggregated list — GET /compute/v1/projects/{p}/aggregated/disks.
	// Real GCP returns map[zone-key]{disks:[…]}; sim groups by zone.
	srv.HandleFunc("GET /compute/v1/projects/{project}/aggregated/disks", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		prefix := fmt.Sprintf("projects/%s/zones/", project)
		all := disks.Filter(func(d ComputeDisk) bool {
			return strings.HasPrefix(d.SelfLink, prefix)
		})
		grouped := map[string]map[string]any{}
		for _, d := range all {
			rest := strings.TrimPrefix(d.SelfLink, prefix)
			zone, _, ok := strings.Cut(rest, "/")
			if !ok {
				continue
			}
			key := "zones/" + zone
			entry, exists := grouped[key]
			if !exists {
				entry = map[string]any{"disks": []ComputeDisk{}}
				grouped[key] = entry
			}
			entry["disks"] = append(entry["disks"].([]ComputeDisk), d)
		}
		sim.WriteJSON(w, http.StatusOK, map[string]any{
			"kind":  "compute#diskAggregatedList",
			"items": grouped,
		})
	})

	// Zonal operations endpoint (disks return zonal ops the SDK polls).
	srv.HandleFunc("GET /compute/v1/projects/{project}/zones/{zone}/operations/{name}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		zone := sim.PathParam(r, "zone")
		name := sim.PathParam(r, "name")
		sim.WriteJSON(w, http.StatusOK, map[string]any{
			"kind":     "compute#operation",
			"id":       computeNumericID(),
			"name":     name,
			"status":   "DONE",
			"selfLink": fmt.Sprintf("projects/%s/zones/%s/operations/%s", project, zone, name),
			"progress": 100,
		})
	})
}
