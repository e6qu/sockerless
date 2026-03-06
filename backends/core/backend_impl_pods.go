package core

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/sockerless/api"
)

// PodCreate creates a new pod.
func (s *BaseServer) PodCreate(req *api.PodCreateRequest) (*api.PodCreateResponse, error) {
	if req.Name == "" {
		return nil, &api.InvalidParameterError{Message: "pod name is required"}
	}

	if s.Store.Pods.Exists(req.Name) {
		return nil, &api.ConflictError{
			Message: fmt.Sprintf("pod with name %s already exists", req.Name),
		}
	}

	hostname := req.Hostname
	var sharedNS []string
	if req.Share != "" {
		sharedNS = strings.Split(req.Share, ",")
	}
	pod := s.Store.Pods.CreatePodWithOpts(req.Name, req.Labels, hostname, sharedNS)

	return &api.PodCreateResponse{ID: pod.ID}, nil
}

// PodList returns all pods matching the given filters.
func (s *BaseServer) PodList(opts api.PodListOptions) ([]*api.PodListEntry, error) {
	pods := s.Store.Pods.ListPods()
	result := make([]*api.PodListEntry, 0, len(pods))
	for _, pod := range pods {
		if !matchPodFilters(pod, opts.Filters) {
			continue
		}
		containers := s.buildPodContainerInfos(pod)
		result = append(result, &api.PodListEntry{
			ID:         pod.ID,
			Name:       pod.Name,
			Status:     pod.Status,
			Created:    pod.Created,
			Labels:     pod.Labels,
			Containers: containers,
		})
	}
	return result, nil
}

// PodInspect returns detailed information about a pod.
func (s *BaseServer) PodInspect(name string) (*api.PodInspectResponse, error) {
	pod, ok := s.Store.Pods.GetPod(name)
	if !ok {
		return nil, &api.NotFoundError{Resource: "pod", ID: name}
	}

	containers := s.buildPodContainerInfos(pod)
	return &api.PodInspectResponse{
		ID:               pod.ID,
		Name:             pod.Name,
		Created:          pod.Created,
		State:            pod.Status,
		Hostname:         pod.Hostname,
		Labels:           pod.Labels,
		NumContainers:    len(pod.ContainerIDs),
		Containers:       containers,
		SharedNamespaces: pod.SharedNS,
	}, nil
}

// PodExists checks if a pod exists.
func (s *BaseServer) PodExists(name string) (bool, error) {
	return s.Store.Pods.Exists(name), nil
}

// PodStart starts all containers in a pod.
func (s *BaseServer) PodStart(name string) (*api.PodActionResponse, error) {
	pod, ok := s.Store.Pods.GetPod(name)
	if !ok {
		return nil, &api.NotFoundError{Resource: "pod", ID: name}
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, cid := range pod.ContainerIDs {
		c, ok := s.Store.Containers.Get(cid)
		if !ok || c.State.Running {
			continue
		}
		exitCh := make(chan struct{})
		s.Store.WaitChs.Store(cid, exitCh)
		pid := s.Store.NextPID()
		s.Store.Containers.Update(cid, func(c *api.Container) {
			c.State.Status = "running"
			c.State.Running = true
			c.State.Pid = pid
			c.State.StartedAt = now
			c.State.FinishedAt = "0001-01-01T00:00:00Z"
			c.State.ExitCode = 0
		})

		if c.Config.Healthcheck != nil && len(c.Config.Healthcheck.Test) > 0 &&
			(len(c.Config.Healthcheck.Test) != 1 || !strings.EqualFold(c.Config.Healthcheck.Test[0], "NONE")) {
			s.StartHealthCheck(cid)
		}

		s.emitEvent("container", "start", cid, map[string]string{
			"name": strings.TrimPrefix(c.Name, "/"),
		})
	}
	s.Store.Pods.SetStatus(pod.ID, "running")
	return &api.PodActionResponse{ID: pod.ID, Errs: []string{}}, nil
}

// PodStop stops all containers in a pod.
func (s *BaseServer) PodStop(name string, timeout *int) (*api.PodActionResponse, error) {
	pod, ok := s.Store.Pods.GetPod(name)
	if !ok {
		return nil, &api.NotFoundError{Resource: "pod", ID: name}
	}

	for _, cid := range pod.ContainerIDs {
		c, ok := s.Store.Containers.Get(cid)
		if !ok || !c.State.Running {
			continue
		}
		s.StopHealthCheck(cid)
		s.Store.ForceStopContainer(cid, 0)
		s.emitEvent("container", "die", cid, map[string]string{
			"exitCode": "0",
			"name":     strings.TrimPrefix(c.Name, "/"),
		})
		s.emitEvent("container", "stop", cid, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})
	}
	s.Store.Pods.SetStatus(pod.ID, "stopped")
	return &api.PodActionResponse{ID: pod.ID, Errs: []string{}}, nil
}

// PodKill sends a signal to all containers in a pod.
func (s *BaseServer) PodKill(name string, signal string) (*api.PodActionResponse, error) {
	pod, ok := s.Store.Pods.GetPod(name)
	if !ok {
		return nil, &api.NotFoundError{Resource: "pod", ID: name}
	}

	if signal == "" {
		signal = "SIGKILL"
	}
	exitCode := signalToExitCode(signal)

	for _, cid := range pod.ContainerIDs {
		c, ok := s.Store.Containers.Get(cid)
		if !ok || !c.State.Running {
			continue
		}
		s.StopHealthCheck(cid)
		s.Store.ForceStopContainer(cid, exitCode)
		s.emitEvent("container", "kill", cid, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})
		s.emitEvent("container", "die", cid, map[string]string{
			"exitCode": fmt.Sprintf("%d", exitCode),
			"name":     strings.TrimPrefix(c.Name, "/"),
		})
	}
	s.Store.Pods.SetStatus(pod.ID, "exited")
	return &api.PodActionResponse{ID: pod.ID, Errs: []string{}}, nil
}

// PodRemove removes a pod and optionally its containers.
func (s *BaseServer) PodRemove(name string, force bool) error {
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

	ctx := context.Background()
	s.removePodContainers(ctx, pod, force)

	s.Store.Pods.DeletePod(pod.ID)
	return nil
}

// removePodContainers removes all containers in a pod.
func (s *BaseServer) removePodContainers(ctx context.Context, pod *PodContext, force bool) {
	for _, cid := range pod.ContainerIDs {
		c, ok := s.Store.Containers.Get(cid)
		if !ok {
			continue
		}
		if force && c.State.Running {
			s.Store.ForceStopContainer(cid, 0)
		}
		s.StopHealthCheck(cid)
		for _, ep := range c.NetworkSettings.Networks {
			if ep != nil && ep.NetworkID != "" {
				_ = s.Drivers.Network.Disconnect(ctx, ep.NetworkID, cid)
			}
		}
		s.Store.Containers.Delete(cid)
		s.Store.ContainerNames.Delete(c.Name)
		s.Store.LogBuffers.Delete(cid)
		if ch, ok := s.Store.WaitChs.LoadAndDelete(cid); ok {
			close(ch.(chan struct{}))
		}
		s.Store.StagingDirs.Delete(cid)
		if dirs, ok := s.Store.TmpfsDirs.LoadAndDelete(cid); ok {
			for _, d := range dirs.([]string) {
				os.RemoveAll(d)
			}
		}
		for _, eid := range c.ExecIDs {
			s.Store.Execs.Delete(eid)
		}
		s.emitEvent("container", "destroy", cid, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})
	}
}

// buildPodContainerInfos returns container info for a pod's containers.
func (s *BaseServer) buildPodContainerInfos(pod *PodContext) []api.PodContainerInfo {
	infos := make([]api.PodContainerInfo, 0, len(pod.ContainerIDs))
	for _, cid := range pod.ContainerIDs {
		c, ok := s.Store.Containers.Get(cid)
		if !ok {
			continue
		}
		infos = append(infos, api.PodContainerInfo{
			ID:    cid,
			Name:  strings.TrimPrefix(c.Name, "/"),
			State: c.State.Status,
		})
	}
	return infos
}
