package aca

import (
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
		c, ok := s.Store.Containers.Get(cid)
		if !ok || c.State.Running {
			continue
		}
		if err := s.ContainerStart(cid); err != nil {
			errs = append(errs, fmt.Sprintf("container %s: %v", cid[:12], err))
		}
	}
	s.Store.Pods.SetStatus(pod.ID, "running")
	if errs == nil {
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
		c, ok := s.Store.Containers.Get(cid)
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
		c, ok := s.Store.Containers.Get(cid)
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
			c, ok := s.Store.Containers.Get(cid)
			if ok && c.State.Running {
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
		if _, ok := s.Store.Containers.Get(cid); !ok {
			continue
		}
		if err := s.ContainerRemove(cid, force); err != nil {
			s.Logger.Warn().Err(err).Str("container", cid[:12]).Msg("failed to remove pod container")
		}
	}

	s.Store.Pods.DeletePod(pod.ID)
	return nil
}

// ExecCreate creates an exec instance, adding an agent-connectivity check for ACA.
// If the container has no agent connected, exec cannot run against the remote
// ACA Job, so we return an error early.
func (s *Server) ExecCreate(containerID string, req *api.ExecCreateRequest) (*api.ExecCreateResponse, error) {
	id, ok := s.Store.ResolveContainerID(containerID)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: containerID}
	}

	c, _ := s.Store.Containers.Get(id)
	if !c.State.Running {
		return nil, &api.ConflictError{Message: "Container " + containerID + " is not running"}
	}

	// ACA-specific: check that an agent is available for exec
	if c.AgentAddress == "" {
		return nil, &api.NotImplementedError{
			Message: fmt.Sprintf("exec requires an agent connection, but container %s has no agent attached (ACA backend)", strings.TrimPrefix(c.Name, "/")),
		}
	}

	// Delegate to BaseServer for the actual exec creation
	return s.BaseServer.ExecCreate(containerID, req)
}

// ExecStart starts an exec instance with an agent-connectivity check.
// If the container has no agent, returns NotImplementedError.
func (s *Server) ExecStart(id string, opts api.ExecStartRequest) (io.ReadWriteCloser, error) {
	exec, ok := s.Store.Execs.Get(id)
	if !ok {
		return nil, &api.NotFoundError{Resource: "exec instance", ID: id}
	}

	c, ok := s.Store.Containers.Get(exec.ContainerID)
	if !ok {
		return nil, &api.ConflictError{
			Message: fmt.Sprintf("Container %s has been removed", exec.ContainerID),
		}
	}

	// ACA-specific: check that an agent is available
	if c.AgentAddress == "" {
		return nil, &api.NotImplementedError{
			Message: fmt.Sprintf("exec requires an agent connection, but container %s has no agent attached (ACA backend)", strings.TrimPrefix(c.Name, "/")),
		}
	}

	// Delegate to BaseServer which handles agent-based exec via the driver chain
	return s.BaseServer.ExecStart(id, opts)
}

// ContainerAttach attaches to a container's streams.
// If an agent is connected, delegates to BaseServer (which uses the driver chain).
// Otherwise returns NotImplementedError since ACA Jobs have no direct attach support.
func (s *Server) ContainerAttach(id string, opts api.ContainerAttachOptions) (io.ReadWriteCloser, error) {
	cid, ok := s.Store.ResolveContainerID(id)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}
	c, _ := s.Store.Containers.Get(cid)
	if c.AgentAddress != "" {
		return s.BaseServer.ContainerAttach(id, opts)
	}
	return nil, &api.NotImplementedError{
		Message: "ACA backend does not support attach without a connected agent",
	}
}

// ContainerExport is not supported by the ACA backend.
// ACA Jobs do not provide filesystem access for container export.
func (s *Server) ContainerExport(ref string) (io.ReadCloser, error) {
	if _, ok := s.Store.ResolveContainerID(ref); !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: ref}
	}
	return nil, &api.NotImplementedError{Message: "container export is not supported by ACA backend: no container filesystem access"}
}

// ContainerCommit is not supported by the ACA backend.
// ACA containers cannot be snapshotted into images.
func (s *Server) ContainerCommit(req *api.ContainerCommitRequest) (*api.ContainerCommitResponse, error) {
	if _, ok := s.Store.ResolveContainerID(req.Container); !ok {
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
