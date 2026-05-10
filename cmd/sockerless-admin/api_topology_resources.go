package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
)

// rollupEntry is one resource attributed to a topology backend
// instance. Fields mirror the backend's `/internal/v1/resources`
// schema with the source identity (project, instance, cloud, port)
// added so the UI can group by any of those dimensions.
type rollupEntry struct {
	Project      string `json:"project"`
	Instance     string `json:"instance"`
	Cloud        string `json:"cloud,omitempty"`
	Backend      string `json:"backend,omitempty"`
	Port         int    `json:"port"`
	ContainerID  string `json:"container_id,omitempty"`
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`
	InstanceID   string `json:"instance_id,omitempty"`
	CreatedAt    string `json:"created_at,omitempty"`
	CleanedUp    bool   `json:"cleaned_up"`
	Status       string `json:"status,omitempty"`
}

// rollupSource describes which backend the rollup tried to query and
// whether the call succeeded. UI surfaces this so operators can tell
// "I see no resources" from "I couldn't reach this backend".
type rollupSource struct {
	Project       string `json:"project"`
	Instance      string `json:"instance"`
	Cloud         string `json:"cloud,omitempty"`
	Backend       string `json:"backend,omitempty"`
	Port          int    `json:"port"`
	OK            bool   `json:"ok"`
	Error         string `json:"error,omitempty"`
	ResourceCount int    `json:"resource_count"`
}

// rollupResponse pairs the flat resource list with per-source status.
type rollupResponse struct {
	Sources   []rollupSource `json:"sources"`
	Resources []rollupEntry  `json:"resources"`
}

// handleTopologyResources aggregates `/internal/v1/resources` across
// every running backend instance in the topology. Each row is
// attributed to {project, instance, cloud, backend} so the UI can
// pivot by any dimension.
//
// Sims are not queried — they do not expose a uniform resource
// endpoint (they implement the underlying cloud APIs directly). The
// rollup is thus backend-only, which is the real shape of the data,
// not a fallback.
func handleTopologyResources(mgr *TopologyManager, client *http.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		active := r.URL.Query().Get("active") == "true"
		path := "/internal/v1/resources"
		if active {
			path += "?active=true"
		}

		backends := backendInstancesFor(mgr)
		var (
			mu        sync.Mutex
			sources   = make([]rollupSource, 0, len(backends))
			resources []rollupEntry
			wg        sync.WaitGroup
		)

		for _, b := range backends {
			wg.Add(1)
			go func(b InstanceRef) {
				defer wg.Done()
				addr := fmt.Sprintf("http://localhost:%d", b.Instance.Port)
				body, status, err := proxyGET(client, addr, path)
				src := rollupSource{
					Project:  b.Project,
					Instance: b.Instance.Name,
					Cloud:    string(b.Instance.Cloud),
					Backend:  string(b.Instance.Backend),
					Port:     b.Instance.Port,
				}
				if err != nil {
					src.Error = err.Error()
					mu.Lock()
					sources = append(sources, src)
					mu.Unlock()
					return
				}
				if status != http.StatusOK {
					src.Error = fmt.Sprintf("upstream HTTP %d", status)
					mu.Lock()
					sources = append(sources, src)
					mu.Unlock()
					return
				}
				var raw []struct {
					ContainerID  string `json:"containerId"`
					ResourceType string `json:"resourceType"`
					ResourceID   string `json:"resourceId"`
					InstanceID   string `json:"instanceId"`
					CreatedAt    string `json:"createdAt"`
					CleanedUp    bool   `json:"cleanedUp"`
					Status       string `json:"status"`
				}
				if err := json.Unmarshal(body, &raw); err != nil {
					src.Error = "decode: " + err.Error()
					mu.Lock()
					sources = append(sources, src)
					mu.Unlock()
					return
				}
				src.OK = true
				src.ResourceCount = len(raw)
				rows := make([]rollupEntry, 0, len(raw))
				for _, e := range raw {
					rows = append(rows, rollupEntry{
						Project:      b.Project,
						Instance:     b.Instance.Name,
						Cloud:        string(b.Instance.Cloud),
						Backend:      string(b.Instance.Backend),
						Port:         b.Instance.Port,
						ContainerID:  e.ContainerID,
						ResourceType: e.ResourceType,
						ResourceID:   e.ResourceID,
						InstanceID:   e.InstanceID,
						CreatedAt:    e.CreatedAt,
						CleanedUp:    e.CleanedUp,
						Status:       e.Status,
					})
				}
				mu.Lock()
				sources = append(sources, src)
				resources = append(resources, rows...)
				mu.Unlock()
			}(b)
		}
		wg.Wait()

		if resources == nil {
			resources = []rollupEntry{}
		}
		writeJSON(w, http.StatusOK, rollupResponse{
			Sources:   sources,
			Resources: resources,
		})
	}
}

// backendInstancesFor returns every backend instance in the topology.
// "Running" is determined by the same liveness probe `readPidStatus`
// uses elsewhere — the rollup will still query each entry, but a
// dead instance fails fast in the dial step and shows up as a
// rollupSource with OK=false.
func backendInstancesFor(mgr *TopologyManager) []InstanceRef {
	var out []InstanceRef
	for _, ref := range mgr.Instances() {
		if ref.Instance.Kind == InstanceKindBackend {
			out = append(out, ref)
		}
	}
	return out
}
