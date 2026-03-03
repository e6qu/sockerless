package main

import (
	"net/http"
	"time"
)

// registerAPI registers all admin API routes.
func registerAPI(mux *http.ServeMux, reg *Registry, procMgr *ProcessManager, projectMgr *ProjectManager) {
	client := &http.Client{Timeout: 5 * time.Second}

	mux.HandleFunc("GET /api/v1/components", handleComponents(reg))
	mux.HandleFunc("GET /api/v1/components/{name}/health", handleComponentProxy(reg, client, "health"))
	mux.HandleFunc("GET /api/v1/components/{name}/status", handleComponentProxy(reg, client, "status"))
	mux.HandleFunc("GET /api/v1/components/{name}/metrics", handleComponentProxy(reg, client, "metrics"))
	mux.HandleFunc("GET /api/v1/components/{name}/provider", handleComponentProvider(reg, client))
	mux.HandleFunc("POST /api/v1/components/{name}/reload", handleComponentReload(reg, client))
	mux.HandleFunc("GET /api/v1/overview", handleOverview(reg, client))
	mux.HandleFunc("GET /api/v1/containers", handleContainers(reg, client))
	mux.HandleFunc("GET /api/v1/resources", handleResources(reg, client))
	mux.HandleFunc("POST /api/v1/resources/cleanup", handleResourceCleanup(reg, client))
	mux.HandleFunc("GET /api/v1/contexts", handleContexts())

	// Process management
	registerProcessAPI(mux, procMgr)

	// Cleanup
	registerCleanupAPI(mux, reg, client)

	// Projects
	registerProjectAPI(mux, projectMgr)
}

// handleComponents returns all registered components with health status.
func handleComponents(reg *Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, reg.List())
	}
}

// statusEndpoint returns the status path for a component type.
func statusEndpoint(typ string) string {
	switch typ {
	case "backend":
		return "/internal/v1/status"
	case "frontend":
		return "/status"
	default:
		return "/health"
	}
}

// metricsEndpoint returns the metrics path for a component type.
func metricsEndpoint(typ string) string {
	switch typ {
	case "backend":
		return "/internal/v1/metrics"
	case "frontend":
		return "/metrics"
	default:
		return "/health"
	}
}

// reloadEndpoint returns the reload path for a component type.
func reloadEndpoint(typ string) string {
	switch typ {
	case "backend":
		return "/internal/v1/reload"
	case "frontend":
		return "/reload"
	default:
		return ""
	}
}

// handleComponentProxy proxies a specific endpoint to a component.
func handleComponentProxy(reg *Registry, client *http.Client, endpoint string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		comp, ok := reg.Get(name)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "component not found"})
			return
		}

		var path string
		switch endpoint {
		case "health":
			path = healthEndpoint(comp.Type)
		case "status":
			path = statusEndpoint(comp.Type)
		case "metrics":
			path = metricsEndpoint(comp.Type)
		}

		body, status, err := proxyGET(client, comp.Addr, path)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{
				"error":     err.Error(),
				"component": name,
			})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Source-Component", name)
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}
}

// providerEndpoint returns the provider info path for a component type.
func providerEndpoint(typ string) string {
	switch typ {
	case "backend":
		return "/internal/v1/provider"
	default:
		return ""
	}
}

// handleComponentProvider proxies the provider info from a backend.
func handleComponentProvider(reg *Registry, client *http.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		comp, ok := reg.Get(name)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "component not found"})
			return
		}

		path := providerEndpoint(comp.Type)
		if path == "" {
			writeJSON(w, http.StatusOK, map[string]string{
				"provider": "unknown",
				"mode":     comp.Type,
			})
			return
		}

		body, status, err := proxyGET(client, comp.Addr, path)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{
				"error":     err.Error(),
				"component": name,
			})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Source-Component", name)
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}
}

// handleComponentReload proxies a reload to a component.
func handleComponentReload(reg *Registry, client *http.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		comp, ok := reg.Get(name)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "component not found"})
			return
		}

		path := reloadEndpoint(comp.Type)
		if path == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "reload not supported for " + comp.Type,
			})
			return
		}

		body, status, err := proxyPOST(client, comp.Addr, path)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{
				"error":     err.Error(),
				"component": name,
			})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Source-Component", name)
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}
}
