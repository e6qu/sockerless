package cloudrun

import (
	"context"
	"time"

	runpb "cloud.google.com/go/run/apiv2/runpb"
	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
	"google.golang.org/api/iterator"
)

// — Service-oriented siblings of the Job helpers in
// cloud_state.go. When Config.UseService is true, sockerless provisions
// Cloud Run Services (long-running, internal-ingress) instead of Jobs
// so peer containers are reachable via stable per-revision IPs.
// These helpers are wired into the lifecycle code in later
// slices — they only query and derive state; they never mutate.

// resolveServiceName returns the Cloud Run Service name for a given
// container ID, or "" if no matching sockerless-managed Service is
// found. Parallel to resolveJobName.
func (p *cloudRunCloudState) resolveServiceName(ctx context.Context, containerID string) (string, error) {
	if p.server.gcp == nil || p.server.gcp.Services == nil {
		return "", nil
	}
	it := p.server.gcp.Services.ListServices(ctx, &runpb.ListServicesRequest{
		Parent: p.server.buildServiceParent(),
	})
	for {
		svc, err := it.Next()
		if err == iterator.Done {
			return "", nil
		}
		if err != nil {
			return "", err
		}
		if svc.Labels["sockerless_managed"] != "true" {
			continue
		}
		svcContainerID := svc.Annotations["sockerless_container_id"]
		if svcContainerID == "" {
			svcContainerID = svc.Labels["sockerless_container_id"]
		}
		if svcContainerID == containerID {
			return svc.Name, nil
		}
	}
}

// resolveServiceCloudRunState returns CloudRunState (populated via the
// ServiceName field) for a container ID, deriving from cloud actuals
// when the in-memory cache is empty. Parallel to resolveCloudRunState
// but for the Services code path. Returns (zero, false) when the
// Services client isn't configured so callers can fall through to the
// Jobs resolver without a type-assert.
func (s *Server) resolveServiceCloudRunState(ctx context.Context, containerID string) (CloudRunState, bool) {
	if state, ok := s.CloudRun.Get(containerID); ok && state.ServiceName != "" {
		return state, true
	}
	csp, ok := s.CloudState.(*cloudRunCloudState)
	if !ok {
		return CloudRunState{}, false
	}
	name, err := csp.resolveServiceName(ctx, containerID)
	if err != nil || name == "" {
		return CloudRunState{}, false
	}
	state := CloudRunState{ServiceName: name}
	s.CloudRun.Update(containerID, func(st *CloudRunState) {
		if st.ServiceName == "" {
			st.ServiceName = name
		}
	})
	return state, true
}

// queryServices fetches all sockerless-managed Cloud Run Services and
// reconstructs containers. Parallel to queryJobs.
func (p *cloudRunCloudState) queryServices(ctx context.Context) ([]api.Container, error) {
	if p.server.gcp == nil || p.server.gcp.Services == nil {
		return nil, nil
	}
	var containers []api.Container

	it := p.server.gcp.Services.ListServices(ctx, &runpb.ListServicesRequest{
		Parent: p.server.buildServiceParent(),
	})

	for {
		svc, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		if svc.Labels["sockerless_managed"] != "true" {
			continue
		}
		c, err := p.serviceToContainer(svc)
		if err != nil {
			p.server.Logger.Debug().Err(err).Str("service", svc.Name).Msg("skipping service in cloud state query")
			continue
		}
		containers = append(containers, c)
	}

	return containers, nil
}

// serviceToContainer reconstructs an api.Container from a Cloud Run
// Service proto. Parallel to jobToContainer.
func (p *cloudRunCloudState) serviceToContainer(svc *runpb.Service) (api.Container, error) {
	labels := svc.Labels

	containerID := svc.Annotations["sockerless_container_id"]
	if containerID == "" {
		containerID = labels["sockerless_container_id"]
	}
	name := labels["sockerless_name"]
	if name == "" && containerID != "" {
		if len(containerID) >= 12 {
			name = "/" + containerID[:12]
		} else {
			name = "/" + containerID
		}
	}

	image := ""
	var cmd []string
	var entrypoint []string
	var env []string
	if svc.Template != nil && len(svc.Template.Containers) > 0 {
		main := svc.Template.Containers[0]
		image = main.Image
		entrypoint = main.Command
		cmd = main.Args
		for _, e := range main.Env {
			if v, ok := e.Values.(*runpb.EnvVar_Value); ok {
				env = append(env, e.Name+"="+v.Value)
			}
		}
	}

	state := serviceContainerState(svc)

	created := ""
	if svc.CreateTime != nil {
		created = svc.CreateTime.AsTime().Format(time.RFC3339Nano)
	}

	// Phase 97: merge annotations (which carry Docker-label JSON blob
	// since it fails GCP label-value charset) before parsing.
	merged := mergeLabelsAndAnnotations(labels, svc.Annotations)
	gcpTags := gcpLabelsToTags(merged)
	dockerLabels := core.ParseLabelsFromTags(gcpTags)
	if dockerLabels == nil {
		dockerLabels = make(map[string]string)
	}

	networkName := labels["sockerless_network"]
	if networkName == "" {
		networkName = "bridge"
	}

	return api.Container{
		ID:      containerID,
		Name:    name,
		Created: created,
		Image:   image,
		State:   state,
		Config: api.ContainerConfig{
			Image:      image,
			Cmd:        cmd,
			Entrypoint: entrypoint,
			Env:        env,
			Labels:     dockerLabels,
		},
		HostConfig: api.HostConfig{
			NetworkMode: networkName,
		},
		NetworkSettings: api.NetworkSettings{
			Networks: map[string]*api.EndpointSettings{
				networkName: {
					NetworkID: networkName,
				},
			},
		},
	}, nil
}

// serviceContainerState derives api.ContainerState from the Service's
// TerminalCondition and LatestReadyRevision. Unlike Jobs, Services are
// long-running so there's no exit-code concept in the happy path:
// - TerminalCondition Ready + LatestReadyRevision set → "running"
// - TerminalCondition still pending/reconciling → "created"
// - TerminalCondition failed → "exited" with code 1
// Reconciling=true (an active revision rollout) keeps the state as
// "running" if a previous revision is already ready, so callers don't
// see a blip when a new revision is deploying.
func serviceContainerState(svc *runpb.Service) api.ContainerState {
	startedAt := ""
	if svc.CreateTime != nil {
		startedAt = svc.CreateTime.AsTime().Format(time.RFC3339Nano)
	}

	cond := svc.TerminalCondition
	ready := cond != nil && cond.Type == "Ready" && cond.State == runpb.Condition_CONDITION_SUCCEEDED
	failed := cond != nil && cond.State == runpb.Condition_CONDITION_FAILED

	switch {
	case ready && svc.LatestReadyRevision != "":
		return api.ContainerState{
			Status:    "running",
			Running:   true,
			StartedAt: startedAt,
		}
	case failed:
		msg := ""
		if cond != nil {
			msg = cond.Message
		}
		return api.ContainerState{
			Status:   "exited",
			ExitCode: 1,
			Error:    msg,
		}
	default:
		return api.ContainerState{
			Status: "created",
		}
	}
}
