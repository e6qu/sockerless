package aca

import (
	"context"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appcontainers/armappcontainers/v2"
	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
)

// Phase 88 — App-oriented siblings of the Job helpers in cloud_state.go.
// When Config.UseApp is true, sockerless provisions ACA ContainerApps
// with internal-only ingress so peer containers are reachable via
// stable per-revision FQDNs (BUG-716).

// resolveAppName returns the ACA ContainerApp name for a given
// container ID, or "" if no matching sockerless-managed App is found.
// Parallel to resolveJobName.
func (p *acaCloudState) resolveAppName(ctx context.Context, containerID string) (string, error) {
	if p.server.azure == nil || p.server.azure.ContainerApps == nil {
		return "", nil
	}
	pager := p.server.azure.ContainerApps.NewListByResourceGroupPager(p.server.config.ResourceGroup, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return "", err
		}
		for _, app := range page.Value {
			if app.Tags == nil {
				continue
			}
			tags := azureTagsToMap(app.Tags)
			if tags["sockerless-managed"] != "true" {
				continue
			}
			if tags["sockerless-container-id"] == containerID {
				if app.Name != nil {
					return *app.Name, nil
				}
			}
		}
	}
	return "", nil
}

// resolveAppACAState returns ACAState (populated via the AppName field)
// for a container ID, deriving from cloud actuals when the in-memory
// cache is empty. Parallel to resolveACAState but for the Apps path.
// Returns (zero, false) when the ContainerApps client isn't configured
// so callers can fall through to the Jobs resolver without a type-assert.
func (s *Server) resolveAppACAState(ctx context.Context, containerID string) (ACAState, bool) {
	if state, ok := s.ACA.Get(containerID); ok && state.AppName != "" {
		return state, true
	}
	csp, ok := s.CloudState.(*acaCloudState)
	if !ok {
		return ACAState{}, false
	}
	name, err := csp.resolveAppName(ctx, containerID)
	if err != nil || name == "" {
		return ACAState{}, false
	}
	state := ACAState{AppName: name}
	s.ACA.Update(containerID, func(st *ACAState) {
		if st.AppName == "" {
			st.AppName = name
		}
	})
	return state, true
}

// queryApps fetches all sockerless-managed ACA ContainerApps and
// reconstructs containers. Parallel to queryJobs.
func (p *acaCloudState) queryApps(ctx context.Context) ([]api.Container, error) {
	if p.server.azure == nil || p.server.azure.ContainerApps == nil {
		return nil, nil
	}
	var containers []api.Container
	pager := p.server.azure.ContainerApps.NewListByResourceGroupPager(p.server.config.ResourceGroup, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, app := range page.Value {
			if app.Tags == nil {
				continue
			}
			tags := azureTagsToMap(app.Tags)
			if tags["sockerless-managed"] != "true" {
				continue
			}
			containers = append(containers, p.appToContainer(app, tags))
		}
	}
	return containers, nil
}

// appToContainer reconstructs an api.Container from an ACA ContainerApp.
// Parallel to jobToContainer.
func (p *acaCloudState) appToContainer(app *armappcontainers.ContainerApp, tags map[string]string) api.Container {
	containerID := tags["sockerless-container-id"]
	name := tags["sockerless-name"]
	if name == "" && containerID != "" {
		name = "/" + containerID[:12]
	}

	image := ""
	var cmd []string
	var entrypoint []string
	var env []string
	if app.Properties != nil && app.Properties.Template != nil {
		for _, tc := range app.Properties.Template.Containers {
			if (tc.Name != nil && *tc.Name == "main") || len(app.Properties.Template.Containers) == 1 {
				if tc.Image != nil {
					image = *tc.Image
				}
				for _, a := range tc.Command {
					if a != nil {
						entrypoint = append(entrypoint, *a)
					}
				}
				for _, a := range tc.Args {
					if a != nil {
						cmd = append(cmd, *a)
					}
				}
				for _, ev := range tc.Env {
					if ev.Name != nil && ev.Value != nil {
						env = append(env, *ev.Name+"="+*ev.Value)
					}
				}
				break
			}
		}
	}

	state := appContainerState(app)

	created := tags["sockerless-created-at"]
	if created == "" && app.SystemData != nil && app.SystemData.CreatedAt != nil {
		created = app.SystemData.CreatedAt.Format(time.RFC3339Nano)
	}

	labels := core.ParseLabelsFromTags(tags)
	if labels == nil {
		labels = make(map[string]string)
	}

	networkName := tags["sockerless-network"]
	if networkName == "" {
		networkName = "bridge"
	}

	path := ""
	var args []string
	if len(entrypoint) > 0 {
		path = entrypoint[0]
		args = append(entrypoint[1:], cmd...)
	} else if len(cmd) > 0 {
		path = cmd[0]
		args = cmd[1:]
	}

	return api.Container{
		ID:      containerID,
		Name:    name,
		Created: created,
		Image:   image,
		Path:    path,
		Args:    args,
		State:   state,
		Config: api.ContainerConfig{
			Image:      image,
			Cmd:        cmd,
			Entrypoint: entrypoint,
			Env:        env,
			Labels:     labels,
		},
		HostConfig: api.HostConfig{
			NetworkMode: networkName,
		},
		NetworkSettings: api.NetworkSettings{
			Networks: map[string]*api.EndpointSettings{
				networkName: {
					NetworkID: networkName,
					IPAddress: "",
				},
			},
		},
		Platform: "linux",
		Driver:   "aca-apps",
	}
}

// appContainerState derives api.ContainerState from the ContainerApp's
// provisioning state and LatestReadyRevisionName. Apps are long-running
// so there's no exit-code concept in the happy path:
//
//   - Succeeded + LatestReadyRevisionName set → "running"
//   - InProgress / unset → "created"
//   - Failed / Canceled → "exited" with code 1
func appContainerState(app *armappcontainers.ContainerApp) api.ContainerState {
	startedAt := ""
	if app.SystemData != nil && app.SystemData.CreatedAt != nil {
		startedAt = app.SystemData.CreatedAt.Format(time.RFC3339Nano)
	}

	ps := ""
	if app.Properties != nil && app.Properties.ProvisioningState != nil {
		ps = string(*app.Properties.ProvisioningState)
	}
	ready := app.Properties != nil && app.Properties.LatestReadyRevisionName != nil && *app.Properties.LatestReadyRevisionName != ""

	switch {
	case ps == string(armappcontainers.ContainerAppProvisioningStateSucceeded) && ready:
		return api.ContainerState{
			Status:    "running",
			Running:   true,
			StartedAt: startedAt,
		}
	case ps == string(armappcontainers.ContainerAppProvisioningStateFailed) ||
		ps == string(armappcontainers.ContainerAppProvisioningStateCanceled):
		return api.ContainerState{
			Status:   "exited",
			ExitCode: 1,
			Error:    ps,
		}
	default:
		return api.ContainerState{
			Status: "created",
		}
	}
}
