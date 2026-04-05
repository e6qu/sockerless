package aca

import (
	"context"
	"strings"
	"time"

	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
)

// acaCloudState implements core.CloudStateProvider for Azure Container Apps.
// Container state is derived from PendingCreates (pre-start) and the
// ACA state store (post-start). The real cloud queries happen in
// the simulator; this provider merges local bookkeeping state.
type acaCloudState struct {
	server *Server
}

func (p *acaCloudState) GetContainer(ctx context.Context, ref string) (api.Container, bool, error) {
	containers, err := p.ListContainers(ctx, true, nil)
	if err != nil {
		return api.Container{}, false, err
	}

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

func (p *acaCloudState) ListContainers(ctx context.Context, all bool, filters map[string][]string) ([]api.Container, error) {
	var result []api.Container

	// Include PendingCreates (containers between create and start)
	for _, c := range p.server.PendingCreates.List() {
		if !all && !c.State.Running {
			continue
		}
		if filters != nil && !core.MatchContainerFilters(c, filters) {
			continue
		}
		result = append(result, c)
	}

	// Include containers tracked in Store.Containers (post-start, actively running)
	for _, c := range p.server.Store.Containers.List() {
		if !all && !c.State.Running {
			continue
		}
		if filters != nil && !core.MatchContainerFilters(c, filters) {
			continue
		}
		result = append(result, c)
	}

	return result, nil
}

func (p *acaCloudState) CheckNameAvailable(ctx context.Context, name string) (bool, error) {
	containers, err := p.ListContainers(ctx, true, nil)
	if err != nil {
		return false, err
	}
	for _, c := range containers {
		if c.Name == name || c.Name == "/"+name {
			return false, nil
		}
	}
	return true, nil
}

func (p *acaCloudState) WaitForExit(ctx context.Context, containerID string) (int, error) {
	ticker := time.NewTicker(p.server.config.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return -1, ctx.Err()
		case <-ticker.C:
			containers, err := p.ListContainers(ctx, true, nil)
			if err != nil {
				continue
			}
			for _, c := range containers {
				if c.ID == containerID && !c.State.Running && c.State.Status == "exited" {
					return c.State.ExitCode, nil
				}
			}
		}
	}
}
