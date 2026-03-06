package main

import (
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// Component represents a registered sockerless component.
type Component struct {
	Name   string `json:"name"`
	Type   string `json:"type"`   // backend, frontend, simulator, coordinator
	Addr   string `json:"addr"`   // e.g. http://localhost:9100
	Health string `json:"health"` // up, down, unknown
	Uptime int    `json:"uptime"` // seconds, from last health check
}

// Registry holds all known components and their health state.
type Registry struct {
	mu         sync.RWMutex
	components map[string]*Component // keyed by name
}

// NewRegistry creates an empty component registry.
func NewRegistry() *Registry {
	return &Registry{components: make(map[string]*Component)}
}

// Add registers or updates a component.
func (r *Registry) Add(c Component) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c.Health == "" {
		c.Health = "unknown"
	}
	r.components[c.Name] = &c
}

// Get returns a component by name.
func (r *Registry) Get(name string) (Component, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.components[name]
	if !ok {
		return Component{}, false
	}
	return *c, true
}

// List returns all components sorted by name.
func (r *Registry) List() []Component {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]Component, 0, len(r.components))
	for _, c := range r.components {
		list = append(list, *c)
	}
	return list
}

// Len returns the number of registered components.
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.components)
}

// ListByType returns components of the given type.
func (r *Registry) ListByType(typ string) []Component {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var list []Component
	for _, c := range r.components {
		if c.Type == typ {
			list = append(list, *c)
		}
	}
	return list
}

// healthEndpoint returns the health path for a component type.
func healthEndpoint(typ string) string {
	switch typ {
	case "backend":
		return "/internal/v1/healthz"
	case "simulator", "coordinator":
		return "/health"
	default:
		return "/health"
	}
}

// PollLoop polls all components' health endpoints at the given interval.
// It stops when the done channel is closed.
func (r *Registry) PollLoop(interval time.Duration, done <-chan struct{}) {
	client := &http.Client{Timeout: 3 * time.Second}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		r.pollOnce(client)
		select {
		case <-done:
			return
		case <-ticker.C:
		}
	}
}

// pollOnce checks health of all components.
func (r *Registry) pollOnce(client *http.Client) {
	r.mu.RLock()
	names := make([]string, 0, len(r.components))
	for name := range r.components {
		names = append(names, name)
	}
	r.mu.RUnlock()

	type result struct {
		name   string
		health string
		uptime int
	}

	results := make(chan result, len(names))
	var wg sync.WaitGroup
	for _, name := range names {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			r.mu.RLock()
			c := r.components[name]
			if c == nil {
				r.mu.RUnlock()
				return
			}
			addr := c.Addr
			typ := c.Type
			r.mu.RUnlock()

			health, uptime := checkHealth(client, addr, typ)
			results <- result{name: name, health: health, uptime: uptime}
		}(name)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	for res := range results {
		r.mu.Lock()
		if c, ok := r.components[res.name]; ok {
			c.Health = res.health
			c.Uptime = res.uptime
		}
		r.mu.Unlock()
	}
}

// checkHealth probes a component's health endpoint and returns status + uptime.
func checkHealth(client *http.Client, addr, typ string) (string, int) {
	url := addr + healthEndpoint(typ)
	resp, err := client.Get(url)
	if err != nil {
		return "down", 0
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "down", 0
	}

	// Try to parse uptime from JSON response
	var body struct {
		UptimeSeconds int `json:"uptime_seconds"`
	}
	if err := decodeJSON(resp.Body, &body); err == nil && body.UptimeSeconds > 0 {
		return "up", body.UptimeSeconds
	}
	return "up", 0
}

// proxyGET performs a GET to a component endpoint and returns the raw body.
func proxyGET(client *http.Client, addr, path string) ([]byte, int, error) {
	resp, err := client.Get(addr + path)
	if err != nil {
		return nil, 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read body: %w", err)
	}
	return body, resp.StatusCode, nil
}

// proxyPOST performs a POST to a component endpoint and returns the raw body.
func proxyPOST(client *http.Client, addr, path string) ([]byte, int, error) {
	resp, err := client.Post(addr+path, "application/json", nil)
	if err != nil {
		return nil, 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read body: %w", err)
	}
	return body, resp.StatusCode, nil
}
