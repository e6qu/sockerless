package aca

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
)

// PodStart starts all containers in a pod by calling ContainerStart for each.
// This triggers ACA Job creation via the deferred-start mechanism.
func (s *Server) PodStart(name string) (*api.PodActionResponse, error) {
	pod, ok := s.Store.Pods.GetPod(name)
	if !ok {
		return nil, &api.NotFoundError{Resource: "pod", ID: name}
	}

	var errs []string
	for _, cid := range pod.ContainerIDs {
		// Check PendingCreates (containers between create and start)
		if c, ok := s.PendingCreates.Get(cid); ok {
			if c.State.Running {
				continue
			}
		} else {
			// Check CloudState for already-running containers
			if c, ok := s.ResolveContainerAuto(context.Background(), cid); ok && c.State.Running {
				continue
			}
		}
		if err := s.ContainerStart(cid); err != nil {
			errs = append(errs, fmt.Sprintf("container %s: %v", cid[:12], err))
		}
	}
	if len(errs) == 0 {
		s.Store.Pods.SetStatus(pod.ID, "running")
		errs = []string{}
	}
	return &api.PodActionResponse{ID: pod.ID, Errs: errs}, nil
}

// PodStop stops all containers in a pod by calling ContainerStop for each.
// This stops ACA Job executions for each container.
func (s *Server) PodStop(name string, timeout *int) (*api.PodActionResponse, error) {
	pod, ok := s.Store.Pods.GetPod(name)
	if !ok {
		return nil, &api.NotFoundError{Resource: "pod", ID: name}
	}

	var errs []string
	for _, cid := range pod.ContainerIDs {
		c, ok := s.ResolveContainerAuto(context.Background(), cid)
		if !ok || !c.State.Running {
			continue
		}
		if err := s.ContainerStop(cid, timeout); err != nil {
			errs = append(errs, fmt.Sprintf("container %s: %v", cid[:12], err))
		}
	}
	s.Store.Pods.SetStatus(pod.ID, "stopped")
	if errs == nil {
		errs = []string{}
	}
	return &api.PodActionResponse{ID: pod.ID, Errs: errs}, nil
}

// PodKill sends a signal to all containers in a pod by calling ContainerKill for each.
func (s *Server) PodKill(name string, signal string) (*api.PodActionResponse, error) {
	pod, ok := s.Store.Pods.GetPod(name)
	if !ok {
		return nil, &api.NotFoundError{Resource: "pod", ID: name}
	}

	if signal == "" {
		signal = "SIGKILL"
	}

	var errs []string
	for _, cid := range pod.ContainerIDs {
		c, ok := s.ResolveContainerAuto(context.Background(), cid)
		if !ok || !c.State.Running {
			continue
		}
		if err := s.ContainerKill(cid, signal); err != nil {
			errs = append(errs, fmt.Sprintf("container %s: %v", cid[:12], err))
		}
	}
	s.Store.Pods.SetStatus(pod.ID, "exited")
	if errs == nil {
		errs = []string{}
	}
	return &api.PodActionResponse{ID: pod.ID, Errs: errs}, nil
}

// PodRemove removes a pod and all its containers by calling ContainerRemove for each.
// This deletes ACA Jobs and cleans up Azure resources.
func (s *Server) PodRemove(name string, force bool) error {
	pod, ok := s.Store.Pods.GetPod(name)
	if !ok {
		return &api.NotFoundError{Resource: "pod", ID: name}
	}

	// Without force, reject if any containers are running
	if !force {
		for _, cid := range pod.ContainerIDs {
			if c, ok := s.ResolveContainerAuto(context.Background(), cid); ok && c.State.Running {
				return &api.ConflictError{
					Message: fmt.Sprintf("pod %s has running containers, cannot remove without force", name),
				}
			}
		}
	}

	// Remove each container through the ACA-aware ContainerRemove method.
	// Take a copy of ContainerIDs since ContainerRemove modifies the pod.
	cids := make([]string, len(pod.ContainerIDs))
	copy(cids, pod.ContainerIDs)
	for _, cid := range cids {
		if _, ok := s.ResolveContainerAuto(context.Background(), cid); !ok {
			continue
		}
		if err := s.ContainerRemove(cid, force); err != nil {
			s.Logger.Warn().Err(err).Str("container", cid[:12]).Msg("failed to remove pod container")
		}
	}

	s.Store.Pods.DeletePod(pod.ID)
	return nil
}

// ExecCreate creates an exec instance. ExecStart requires the
// in-container reverse-agent (no fallback to ACA management-API
// WebSocket exec — that path silently swaps execution semantics).
func (s *Server) ExecCreate(containerID string, req *api.ExecCreateRequest) (*api.ExecCreateResponse, error) {
	c, ok := s.ResolveContainerAuto(context.Background(), containerID)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: containerID}
	}

	if !c.State.Running {
		return nil, &api.ConflictError{Message: "Container " + containerID + " is not running"}
	}

	return s.BaseServer.ExecCreate(containerID, req)
}

// ExecStart starts an exec instance. Requires a registered
// reverse-agent for the container; fails loud if missing. No
// fallback to the ACA management-API WebSocket exec — that path
// runs in a separate ad-hoc shell session with different env /
// stream encoding and would hide reverse-agent setup bugs.
func (s *Server) ExecStart(id string, opts api.ExecStartRequest) (io.ReadWriteCloser, error) {
	exec, ok := s.Store.Execs.Get(id)
	if !ok {
		return nil, &api.NotFoundError{Resource: "exec instance", ID: id}
	}

	c, ok := s.ResolveContainerAuto(context.Background(), exec.ContainerID)
	if !ok {
		return nil, &api.ConflictError{
			Message: fmt.Sprintf("Container %s has been removed", exec.ContainerID),
		}
	}

	if s.reverseAgents.IsLifetimeExpired(c.ID) {
		return nil, &api.ServerError{Message: fmt.Sprintf(
			"container %s exceeded ACA's max invocation lifetime. "+
				"FaaS pods are not extended transparently — for sustained workloads use ACA Apps (always-on replicas) "+
				"or switch to a longer-lived backend. (FaaSPodLifetimeExceeded)",
			c.ID[:12],
		)}
	}
	if _, hasAgent := s.reverseAgents.Resolve(c.ID); !hasAgent {
		return nil, &api.ServerError{Message: fmt.Sprintf(
			"reverse-agent WebSocket not registered for container %s. "+
				"ACA exec requires SOCKERLESS_CALLBACK_URL reachable from inside the App / Job "+
				"so the bootstrap can dial back. See backends/aca/README.md § reverse-agent prerequisites. "+
				"(Was the bootstrap able to start and reach the callback URL?)",
			c.ID[:12],
		)}
	}
	return s.BaseServer.ExecStart(id, opts)
}

// ContainerAttach attaches to a container's streams. Requires a
// registered reverse-agent (same reason as ExecStart — no
// management-API fallback).
func (s *Server) ContainerAttach(id string, opts api.ContainerAttachOptions) (io.ReadWriteCloser, error) {
	c, ok := s.ResolveContainerAuto(context.Background(), id)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}

	if _, hasAgent := s.reverseAgents.Resolve(c.ID); !hasAgent {
		if opts.Stdin && s.config.UseApp {
			p := newStdinPipe()
			actual, _ := s.stdinPipes.LoadOrStore(c.ID, p)
			pipe := actual.(*stdinPipe)
			pipe.Open()
			return s.newAttachStream(c.ID, pipe), nil
		}
		return nil, &api.ServerError{Message: fmt.Sprintf(
			"reverse-agent WebSocket not registered for container %s. "+
				"ACA attach requires SOCKERLESS_CALLBACK_URL reachable from inside the App / Job "+
				"so the bootstrap can dial back. See backends/aca/README.md § reverse-agent prerequisites.",
			c.ID[:12],
		)}
	}
	return s.BaseServer.ContainerAttach(id, opts)
}

// ContainerExport streams the container's rootfs as tar via the
// reverse-agent.
func (s *Server) ContainerExport(ref string) (io.ReadCloser, error) {
	cid, ok := s.ResolveContainerIDAuto(context.Background(), ref)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: ref}
	}
	rc, err := core.RunContainerExportViaAgent(s.reverseAgents, cid)
	if err == core.ErrNoReverseAgent {
		return nil, &api.NotImplementedError{Message: "docker export requires a reverse-agent bootstrap inside the container (SOCKERLESS_CALLBACK_URL); no session registered"}
	}
	if err != nil {
		return nil, &api.ServerError{Message: fmt.Sprintf("export via reverse-agent: %v", err)}
	}
	return rc, nil
}

// ContainerCommit is not supported by the ACA backend.
// ACA containers cannot be snapshotted into images.
func (s *Server) ContainerCommit(req *api.ContainerCommitRequest) (*api.ContainerCommitResponse, error) {
	if _, ok := s.ResolveContainerIDAuto(context.Background(), req.Container); !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: req.Container}
	}
	if !s.config.EnableCommit {
		return nil, &api.NotImplementedError{Message: "docker commit on ACA is gated — set SOCKERLESS_ENABLE_COMMIT=1 (agent-driven commit captures added/modified files since container boot as a new layer)"}
	}
	return core.CommitContainerRequestViaAgent(s.BaseServer, s.reverseAgents, req)
}

// AuthLogin handles registry authentication.
// For ACR registries (*.azurecr.io), logs a warning and delegates to BaseServer.
// For all other registries, delegates to BaseServer directly.
func (s *Server) AuthLogin(req *api.AuthRequest) (*api.AuthResponse, error) {
	if strings.HasSuffix(req.ServerAddress, ".azurecr.io") {
		s.Logger.Warn().
			Str("registry", req.ServerAddress).
			Msg("ACR login: credentials stored locally; use `az acr login` for production")
		return s.BaseServer.AuthLogin(req)
	}
	return s.BaseServer.AuthLogin(req)
}

// Info returns system information enriched with ACA-specific metadata.
func (s *Server) Info() (*api.BackendInfo, error) {
	info, err := s.BaseServer.Info()
	if err != nil {
		return nil, err
	}

	// Enrich the Name field with ACA environment details
	info.Name = fmt.Sprintf("%s (aca:%s/%s)", info.Name, s.config.ResourceGroup, s.config.Environment)

	return info, nil
}
