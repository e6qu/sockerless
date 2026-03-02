package main

import (
	"encoding/json"
	"net/http"
	"sync"
)

// resourceEntry is a cloud resource with its source backend label.
type resourceEntry struct {
	ContainerID  string `json:"containerId"`
	Backend      string `json:"backend"`
	ResourceType string `json:"resourceType"`
	ResourceID   string `json:"resourceId"`
	InstanceID   string `json:"instanceId"`
	CreatedAt    string `json:"createdAt"`
	CleanedUp    bool   `json:"cleanedUp"`
	Status       string `json:"status"`
}

// handleResources returns cloud resources merged from all backends.
func handleResources(reg *Registry, client *http.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		backends := reg.ListByType("backend")

		active := r.URL.Query().Get("active")
		path := "/internal/v1/resources"
		if active == "true" {
			path += "?active=true"
		}

		var mu sync.Mutex
		var all []resourceEntry
		var wg sync.WaitGroup

		for _, b := range backends {
			wg.Add(1)
			go func(name, addr string) {
				defer wg.Done()
				body, status, err := proxyGET(client, addr, path)
				if err != nil || status != http.StatusOK {
					return
				}
				var resources []struct {
					ContainerID  string `json:"containerId"`
					Backend      string `json:"backend"`
					ResourceType string `json:"resourceType"`
					ResourceID   string `json:"resourceId"`
					InstanceID   string `json:"instanceId"`
					CreatedAt    string `json:"createdAt"`
					CleanedUp    bool   `json:"cleanedUp"`
					Status       string `json:"status"`
				}
				if json.Unmarshal(body, &resources) == nil {
					mu.Lock()
					for _, res := range resources {
						all = append(all, resourceEntry{
							ContainerID:  res.ContainerID,
							Backend:      name,
							ResourceType: res.ResourceType,
							ResourceID:   res.ResourceID,
							InstanceID:   res.InstanceID,
							CreatedAt:    res.CreatedAt,
							CleanedUp:    res.CleanedUp,
							Status:       res.Status,
						})
					}
					mu.Unlock()
				}
			}(b.Name, b.Addr)
		}
		wg.Wait()

		if all == nil {
			all = []resourceEntry{}
		}
		writeJSON(w, http.StatusOK, all)
	}
}

// handleResourceCleanup proxies a cleanup request to a specific backend.
func handleResourceCleanup(reg *Registry, client *http.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		component := r.URL.Query().Get("component")
		if component == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "component query parameter is required",
			})
			return
		}

		comp, ok := reg.Get(component)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "component not found"})
			return
		}
		if comp.Type != "backend" {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "cleanup only supported for backends",
			})
			return
		}

		body, status, err := proxyPOST(client, comp.Addr, "/internal/v1/resources/cleanup")
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{
				"error":     err.Error(),
				"component": component,
			})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Source-Component", component)
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}
}
