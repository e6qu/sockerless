package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	sim "github.com/sockerless/simulator"
)

func registerComputeLoadBalancing(srv *sim.Server) {
	healthChecks := sim.MakeStore[ComputeHealthCheck](srv.DB(), "compute_health_checks")
	backendServices := sim.MakeStore[ComputeBackendService](srv.DB(), "compute_backend_services")
	urlMaps := sim.MakeStore[ComputeURLMap](srv.DB(), "compute_url_maps")
	targetHTTPProxies := sim.MakeStore[ComputeTargetHTTPProxy](srv.DB(), "compute_target_http_proxies")
	forwardingRules := sim.MakeStore[ComputeForwardingRule](srv.DB(), "compute_global_forwarding_rules")

	srv.HandleFunc("POST /compute/v1/projects/{project}/global/healthChecks", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		var hc ComputeHealthCheck
		if err := sim.ReadJSON(r, &hc); err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
			return
		}
		if hc.Name == "" {
			sim.GCPError(w, http.StatusBadRequest, "name is required", "INVALID_ARGUMENT")
			return
		}
		hc.Kind = "compute#healthCheck"
		hc.Id = computeNumericID()
		hc.SelfLink = computeGlobalLink(project, "healthChecks", hc.Name)
		hc.CreationTimestamp = time.Now().UTC().Format(time.RFC3339)
		if hc.Type == "" {
			if hc.TcpHealthCheck != nil {
				hc.Type = "TCP"
			} else {
				hc.Type = "HTTP"
			}
		}
		if hc.CheckIntervalSec == 0 {
			hc.CheckIntervalSec = 5
		}
		if hc.TimeoutSec == 0 {
			hc.TimeoutSec = 5
		}
		if hc.HealthyThreshold == 0 {
			hc.HealthyThreshold = 2
		}
		if hc.UnhealthyThreshold == 0 {
			hc.UnhealthyThreshold = 2
		}
		if hc.Type == "HTTP" && hc.HttpHealthCheck == nil {
			hc.HttpHealthCheck = &ComputeHTTPHealthCheck{Port: 80, RequestPath: "/", ProxyHeader: "NONE"}
		}
		healthChecks.Put(hc.SelfLink, hc)
		sim.WriteJSON(w, http.StatusOK, computeGlobalOp(project, hc.SelfLink, "insert"))
	})
	srv.HandleFunc("GET /compute/v1/projects/{project}/global/healthChecks/{name}", func(w http.ResponseWriter, r *http.Request) {
		computeWriteGlobalResource(w, r, healthChecks, "healthChecks", "health check")
	})
	srv.HandleFunc("GET /compute/v1/projects/{project}/global/healthChecks", func(w http.ResponseWriter, r *http.Request) {
		computeWriteGlobalList(w, r, healthChecks, "compute#healthCheckList")
	})
	srv.HandleFunc("DELETE /compute/v1/projects/{project}/global/healthChecks/{name}", func(w http.ResponseWriter, r *http.Request) {
		computeDeleteGlobalResource(w, r, healthChecks, "healthChecks")
	})

	srv.HandleFunc("POST /compute/v1/projects/{project}/global/backendServices", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		var bs ComputeBackendService
		if err := sim.ReadJSON(r, &bs); err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
			return
		}
		if bs.Name == "" {
			sim.GCPError(w, http.StatusBadRequest, "name is required", "INVALID_ARGUMENT")
			return
		}
		bs.Kind = "compute#backendService"
		bs.Id = computeNumericID()
		bs.SelfLink = computeGlobalLink(project, "backendServices", bs.Name)
		bs.CreationTimestamp = time.Now().UTC().Format(time.RFC3339)
		if bs.Protocol == "" {
			bs.Protocol = "HTTP"
		}
		if bs.LoadBalancingScheme == "" {
			bs.LoadBalancingScheme = "EXTERNAL"
		}
		if bs.TimeoutSec == 0 {
			bs.TimeoutSec = 30
		}
		bs.Fingerprint = computeFingerprint()
		backendServices.Put(bs.SelfLink, bs)
		sim.WriteJSON(w, http.StatusOK, computeGlobalOp(project, bs.SelfLink, "insert"))
	})
	srv.HandleFunc("GET /compute/v1/projects/{project}/global/backendServices/{name}", func(w http.ResponseWriter, r *http.Request) {
		computeWriteGlobalResource(w, r, backendServices, "backendServices", "backend service")
	})
	srv.HandleFunc("GET /compute/v1/projects/{project}/global/backendServices", func(w http.ResponseWriter, r *http.Request) {
		computeWriteGlobalList(w, r, backendServices, "compute#backendServiceList")
	})
	srv.HandleFunc("PATCH /compute/v1/projects/{project}/global/backendServices/{name}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		name := sim.PathParam(r, "name")
		selfLink := computeGlobalLink(project, "backendServices", name)
		var patch ComputeBackendService
		if err := sim.ReadJSON(r, &patch); err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
			return
		}
		if !backendServices.Update(selfLink, func(bs *ComputeBackendService) {
			if patch.Description != "" {
				bs.Description = patch.Description
			}
			if patch.Protocol != "" {
				bs.Protocol = patch.Protocol
			}
			if patch.PortName != "" {
				bs.PortName = patch.PortName
			}
			if patch.TimeoutSec != 0 {
				bs.TimeoutSec = patch.TimeoutSec
			}
			if patch.LoadBalancingScheme != "" {
				bs.LoadBalancingScheme = patch.LoadBalancingScheme
			}
			if patch.HealthChecks != nil {
				bs.HealthChecks = patch.HealthChecks
			}
			if patch.Backends != nil {
				bs.Backends = patch.Backends
			}
		}) {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "backend service %q not found", name)
			return
		}
		sim.WriteJSON(w, http.StatusOK, computeGlobalOp(project, selfLink, "patch"))
	})
	srv.HandleFunc("DELETE /compute/v1/projects/{project}/global/backendServices/{name}", func(w http.ResponseWriter, r *http.Request) {
		computeDeleteGlobalResource(w, r, backendServices, "backendServices")
	})

	srv.HandleFunc("POST /compute/v1/projects/{project}/global/urlMaps", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		var um ComputeURLMap
		if err := sim.ReadJSON(r, &um); err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
			return
		}
		if um.Name == "" {
			sim.GCPError(w, http.StatusBadRequest, "name is required", "INVALID_ARGUMENT")
			return
		}
		um.Kind = "compute#urlMap"
		um.Id = computeNumericID()
		um.SelfLink = computeGlobalLink(project, "urlMaps", um.Name)
		um.CreationTimestamp = time.Now().UTC().Format(time.RFC3339)
		um.Fingerprint = computeFingerprint()
		urlMaps.Put(um.SelfLink, um)
		sim.WriteJSON(w, http.StatusOK, computeGlobalOp(project, um.SelfLink, "insert"))
	})
	srv.HandleFunc("GET /compute/v1/projects/{project}/global/urlMaps/{name}", func(w http.ResponseWriter, r *http.Request) {
		computeWriteGlobalResource(w, r, urlMaps, "urlMaps", "URL map")
	})
	srv.HandleFunc("GET /compute/v1/projects/{project}/global/urlMaps", func(w http.ResponseWriter, r *http.Request) {
		computeWriteGlobalList(w, r, urlMaps, "compute#urlMapList")
	})
	srv.HandleFunc("DELETE /compute/v1/projects/{project}/global/urlMaps/{name}", func(w http.ResponseWriter, r *http.Request) {
		computeDeleteGlobalResource(w, r, urlMaps, "urlMaps")
	})

	srv.HandleFunc("POST /compute/v1/projects/{project}/global/targetHttpProxies", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		var proxy ComputeTargetHTTPProxy
		if err := sim.ReadJSON(r, &proxy); err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
			return
		}
		if proxy.Name == "" {
			sim.GCPError(w, http.StatusBadRequest, "name is required", "INVALID_ARGUMENT")
			return
		}
		proxy.Kind = "compute#targetHttpProxy"
		proxy.Id = computeNumericID()
		proxy.SelfLink = computeGlobalLink(project, "targetHttpProxies", proxy.Name)
		proxy.CreationTimestamp = time.Now().UTC().Format(time.RFC3339)
		targetHTTPProxies.Put(proxy.SelfLink, proxy)
		sim.WriteJSON(w, http.StatusOK, computeGlobalOp(project, proxy.SelfLink, "insert"))
	})
	srv.HandleFunc("GET /compute/v1/projects/{project}/global/targetHttpProxies/{name}", func(w http.ResponseWriter, r *http.Request) {
		computeWriteGlobalResource(w, r, targetHTTPProxies, "targetHttpProxies", "target HTTP proxy")
	})
	srv.HandleFunc("GET /compute/v1/projects/{project}/global/targetHttpProxies", func(w http.ResponseWriter, r *http.Request) {
		computeWriteGlobalList(w, r, targetHTTPProxies, "compute#targetHttpProxyList")
	})
	srv.HandleFunc("DELETE /compute/v1/projects/{project}/global/targetHttpProxies/{name}", func(w http.ResponseWriter, r *http.Request) {
		computeDeleteGlobalResource(w, r, targetHTTPProxies, "targetHttpProxies")
	})

	srv.HandleFunc("POST /compute/v1/projects/{project}/global/forwardingRules", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		var fr ComputeForwardingRule
		if err := sim.ReadJSON(r, &fr); err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
			return
		}
		if fr.Name == "" {
			sim.GCPError(w, http.StatusBadRequest, "name is required", "INVALID_ARGUMENT")
			return
		}
		fr.Kind = "compute#forwardingRule"
		fr.Id = computeNumericID()
		fr.SelfLink = computeGlobalLink(project, "forwardingRules", fr.Name)
		fr.CreationTimestamp = time.Now().UTC().Format(time.RFC3339)
		if fr.IPAddress == "" {
			fr.IPAddress = computeEphemeralIPv4(fr.Id)
		}
		if fr.IPProtocol == "" {
			fr.IPProtocol = "TCP"
		}
		if fr.LoadBalancingScheme == "" {
			fr.LoadBalancingScheme = "EXTERNAL"
		}
		if fr.NetworkTier == "" {
			fr.NetworkTier = "PREMIUM"
		}
		forwardingRules.Put(fr.SelfLink, fr)
		sim.WriteJSON(w, http.StatusOK, computeGlobalOp(project, fr.SelfLink, "insert"))
	})
	srv.HandleFunc("GET /compute/v1/projects/{project}/global/forwardingRules/{name}", func(w http.ResponseWriter, r *http.Request) {
		computeWriteGlobalResource(w, r, forwardingRules, "forwardingRules", "forwarding rule")
	})
	srv.HandleFunc("GET /compute/v1/projects/{project}/global/forwardingRules", func(w http.ResponseWriter, r *http.Request) {
		computeWriteGlobalList(w, r, forwardingRules, "compute#forwardingRuleList")
	})
	srv.HandleFunc("DELETE /compute/v1/projects/{project}/global/forwardingRules/{name}", func(w http.ResponseWriter, r *http.Request) {
		computeDeleteGlobalResource(w, r, forwardingRules, "forwardingRules")
	})
}

type computeNamedResource interface {
	ComputeHealthCheck | ComputeBackendService | ComputeURLMap | ComputeTargetHTTPProxy | ComputeForwardingRule
}

func computeGlobalLink(project, collection, name string) string {
	return fmt.Sprintf("projects/%s/global/%s/%s", project, collection, name)
}

func computeGlobalOp(project, target, opType string) map[string]any {
	op := newComputeOp(project, "global", target)
	op["operationType"] = opType
	return op
}

func computeWriteGlobalResource[T computeNamedResource](w http.ResponseWriter, r *http.Request, store sim.Store[T], collection, label string) {
	project := sim.PathParam(r, "project")
	name := sim.PathParam(r, "name")
	selfLink := computeGlobalLink(project, collection, name)
	resource, ok := store.Get(selfLink)
	if !ok {
		sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "%s %q not found", label, name)
		return
	}
	sim.WriteJSON(w, http.StatusOK, resource)
}

func computeWriteGlobalList[T computeNamedResource](w http.ResponseWriter, r *http.Request, store sim.Store[T], kind string) {
	project := sim.PathParam(r, "project")
	prefix := fmt.Sprintf("projects/%s/global/", project)
	items := store.Filter(func(resource T) bool {
		return strings.HasPrefix(computeResourceSelfLink(resource), prefix)
	})
	if items == nil {
		items = []T{}
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"kind":  kind,
		"items": items,
	})
}

func computeResourceSelfLink[T computeNamedResource](resource T) string {
	switch v := any(resource).(type) {
	case ComputeHealthCheck:
		return v.SelfLink
	case ComputeBackendService:
		return v.SelfLink
	case ComputeURLMap:
		return v.SelfLink
	case ComputeTargetHTTPProxy:
		return v.SelfLink
	case ComputeForwardingRule:
		return v.SelfLink
	default:
		return ""
	}
}

func computeDeleteGlobalResource[T computeNamedResource](w http.ResponseWriter, r *http.Request, store sim.Store[T], collection string) {
	project := sim.PathParam(r, "project")
	name := sim.PathParam(r, "name")
	selfLink := computeGlobalLink(project, collection, name)
	store.Delete(selfLink)
	sim.WriteJSON(w, http.StatusOK, computeGlobalOp(project, selfLink, "delete"))
}

func computeEphemeralIPv4(id string) string {
	if len(id) < 6 {
		return "34.0.0.1"
	}
	return fmt.Sprintf("34.%d.%d.%d", id[0], id[1], id[2])
}

func computeFingerprint() string {
	return "c29ja2VybGVzcw=="
}
