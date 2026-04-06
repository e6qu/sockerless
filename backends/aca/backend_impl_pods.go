package aca

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/sockerless/api"
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

// ExecCreate creates an exec instance. Now supports both agent-based exec
// and cloud-based exec via the ACA management API, so the agent check is
// no longer a hard requirement.
func (s *Server) ExecCreate(containerID string, req *api.ExecCreateRequest) (*api.ExecCreateResponse, error) {
	c, ok := s.ResolveContainerAuto(context.Background(), containerID)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: containerID}
	}

	if !c.State.Running {
		return nil, &api.ConflictError{Message: "Container " + containerID + " is not running"}
	}

	// Delegate to BaseServer for the actual exec creation.
	// ExecStart will route to agent or cloud exec as appropriate.
	return s.BaseServer.ExecCreate(containerID, req)
}

// ExecStart starts an exec instance. If an agent is connected, delegates
// to BaseServer (agent driver). Otherwise, falls back to cloudExecStart
// which uses the ACA management API WebSocket exec endpoint.
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

	// Use cloud exec via ACA management API
	return s.cloudExecStart(&exec, &c)
}

// ContainerAttach attaches to a container's streams.
// If an agent is connected, delegates to BaseServer (which uses the driver chain).
// Otherwise, falls back to cloud exec via the ACA management API, creating a
// shell session that serves as an attach-like experience.
func (s *Server) ContainerAttach(id string, opts api.ContainerAttachOptions) (io.ReadWriteCloser, error) {
	c, ok := s.ResolveContainerAuto(context.Background(), id)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}
	cid := c.ID

	// Fall back to cloud exec, creating a shell session as attach.
	// Build a synthetic exec instance for the container's entrypoint.
	exec := &api.ExecInstance{
		ContainerID: cid,
		ProcessConfig: api.ExecProcessConfig{
			Entrypoint: "/bin/sh",
			Tty:        opts.Stream,
		},
	}
	return s.cloudExecStart(exec, &c)
}

// ContainerExport is not supported by the ACA backend.
// ACA Jobs do not provide filesystem access for container export.
func (s *Server) ContainerExport(ref string) (io.ReadCloser, error) {
	if _, ok := s.ResolveContainerIDAuto(context.Background(), ref); !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: ref}
	}
	return nil, &api.NotImplementedError{Message: "container export is not supported by ACA backend: no container filesystem access"}
}

// ContainerCommit is not supported by the ACA backend.
// ACA containers cannot be snapshotted into images.
func (s *Server) ContainerCommit(req *api.ContainerCommitRequest) (*api.ContainerCommitResponse, error) {
	if _, ok := s.ResolveContainerIDAuto(context.Background(), req.Container); !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: req.Container}
	}
	return nil, &api.NotImplementedError{Message: "container commit is not supported by ACA backend: cannot snapshot ACA containers into images"}
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
