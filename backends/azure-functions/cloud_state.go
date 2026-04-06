package azf

import (
	"context"
	"strings"
	"time"

	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
)

// azfCloudState implements core.CloudStateProvider for Azure Functions.
// Queries AZF state store and PendingCreates for container state.
type azfCloudState struct {
	server *Server
}

func (p *azfCloudState) GetContainer(ctx context.Context, ref string) (api.Container, bool, error) {
	containers := p.allContainers()

	for _, c := range containers {
		if c.ID == ref {
			return c, true, nil
		}
		if c.Name == ref || c.Name == "/"+ref || strings.TrimPrefix(c.Name, "/") == ref {
			return c, true, nil
		}
		if len(ref) >= 3 && strings.HasPrefix(c.ID, ref) {
			return c, true, nil
		}
	}
	return api.Container{}, false, nil
}

func (p *azfCloudState) ListContainers(ctx context.Context, all bool, filters map[string][]string) ([]api.Container, error) {
	containers := p.allContainers()

	var result []api.Container
	for _, c := range containers {
		if !all && !c.State.Running {
			continue
		}
		if !core.MatchContainerFilters(c, filters) {
			continue
		}
		result = append(result, c)
	}
	return result, nil
}

func (p *azfCloudState) CheckNameAvailable(ctx context.Context, name string) (bool, error) {
	containers := p.allContainers()
	for _, c := range containers {
		if c.Name == name || c.Name == "/"+name {
			return false, nil
		}
	}
	return true, nil
}

func (p *azfCloudState) WaitForExit(ctx context.Context, containerID string) (int, error) {
	// Check WaitChs first — FaaS containers use exit channels
	if ch, ok := p.server.Store.WaitChs.Load(containerID); ok {
		select {
		case <-ctx.Done():
			return -1, ctx.Err()
		case <-ch.(chan struct{}):
			// Channel closed — container exited.
			// Re-query to get exit code.
			containers := p.allContainers()
			for _, c := range containers {
				if c.ID == containerID {
					return c.State.ExitCode, nil
				}
			}
			return 0, nil
		}
	}

	// Fallback: poll state store
	interval := p.server.config.PollInterval
	if interval == 0 {
		interval = 2 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return -1, ctx.Err()
		case <-ticker.C:
			containers := p.allContainers()
			for _, c := range containers {
				if c.ID == containerID && !c.State.Running && c.State.Status == "exited" {
					return c.State.ExitCode, nil
				}
			}
		}
	}
}

// allContainers merges PendingCreates and AZF state store entries
// to produce the full set of known containers.
func (p *azfCloudState) allContainers() []api.Container {
	seen := make(map[string]bool)
	var result []api.Container

	// PendingCreates (containers between create and start)
	for _, c := range p.server.PendingCreates.List() {
		seen[c.ID] = true
		result = append(result, c)
	}

	// Containers from Store that have matching AZF state
	for _, id := range p.server.AZF.Keys() {
		if seen[id] {
			continue
		}
		if c, ok := p.server.Store.Containers.Get(id); ok {
			seen[id] = true
			result = append(result, c)
		}
	}

	return result
}
