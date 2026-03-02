package main

import (
	"encoding/json"
	"net/http"
	"sync"
)

// containerEntry is a container with its source backend label.
type containerEntry struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Image   string `json:"image"`
	State   string `json:"state"`
	Created string `json:"created"`
	PodName string `json:"pod_name,omitempty"`
	Backend string `json:"backend"`
}

// handleContainers returns containers merged from all backends.
func handleContainers(reg *Registry, client *http.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		backends := reg.ListByType("backend")

		var mu sync.Mutex
		var all []containerEntry
		var wg sync.WaitGroup

		for _, b := range backends {
			wg.Add(1)
			go func(name, addr string) {
				defer wg.Done()
				body, status, err := proxyGET(client, addr, "/internal/v1/containers/summary")
				if err != nil || status != http.StatusOK {
					return
				}
				var containers []struct {
					ID      string `json:"id"`
					Name    string `json:"name"`
					Image   string `json:"image"`
					State   string `json:"state"`
					Created string `json:"created"`
					PodName string `json:"pod_name,omitempty"`
				}
				if json.Unmarshal(body, &containers) == nil {
					mu.Lock()
					for _, c := range containers {
						all = append(all, containerEntry{
							ID:      c.ID,
							Name:    c.Name,
							Image:   c.Image,
							State:   c.State,
							Created: c.Created,
							PodName: c.PodName,
							Backend: name,
						})
					}
					mu.Unlock()
				}
			}(b.Name, b.Addr)
		}
		wg.Wait()

		if all == nil {
			all = []containerEntry{}
		}
		writeJSON(w, http.StatusOK, all)
	}
}
