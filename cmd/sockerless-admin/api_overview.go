package main

import (
	"encoding/json"
	"net/http"
	"sync"
)

// handleOverview returns an aggregated system overview.
func handleOverview(reg *Registry, client *http.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		components := reg.List()

		up, down := 0, 0
		for _, c := range components {
			if c.Health == "up" {
				up++
			} else {
				down++
			}
		}

		// Aggregate container counts from all backends
		totalContainers := 0
		var mu sync.Mutex
		var wg sync.WaitGroup

		backends := reg.ListByType("backend")
		for _, b := range backends {
			wg.Add(1)
			go func(addr string) {
				defer wg.Done()
				body, status, err := proxyGET(client, addr, "/internal/v1/status")
				if err != nil || status != http.StatusOK {
					return
				}
				var resp struct {
					Containers int `json:"containers"`
				}
				if json.Unmarshal(body, &resp) == nil {
					mu.Lock()
					totalContainers += resp.Containers
					mu.Unlock()
				}
			}(b.Addr)
		}
		wg.Wait()

		writeJSON(w, http.StatusOK, map[string]any{
			"components_up":    up,
			"components_down":  down,
			"components_total": len(components),
			"total_containers": totalContainers,
			"backends":         len(backends),
			"components":       components,
		})
	}
}
