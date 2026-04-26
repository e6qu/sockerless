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

// PodList returns all pods matching the given filters./
// when the backend implements CloudPodLister, merge pods
// derived from cloud actuals (multi-container task/app grouped by the
// sockerless-pod tag) with the in-memory registry so that
// `docker pod ps` after a restart still reflects the cloud truth.
func (s *BaseServer) PodList(opts api.PodListOptions) ([]*api.PodListEntry, error) {
	pods := s.Store.Pods.ListPods()
	seen := make(map[string]bool)
	result := make([]*api.PodListEntry, 0, len(pods))
	for _, pod := range pods {
		if !matchPodFilters(pod, opts.Filters) {
			continue
		}
		containers := s.buildPodContainerInfos(pod)
		entry := &api.PodListEntry{
			ID:         pod.ID,
			Name:       pod.Name,
			Status:     pod.Status,
			Created:    pod.Created,
			Labels:     pod.Labels,
			Containers: containers,
		}
		seen[entry.ID] = true
		result = append(result, entry)
	}
	if lister, ok := s.CloudState.(CloudPodLister); ok {
		cloudPods, err := lister.ListPods(context.Background())
		if err != nil {
			s.Logger.Debug().Err(err).Msg("cloud pod listing failed, returning cache-only result")
		}
		for _, p := range cloudPods {
			if p == nil || seen[p.ID] {
				continue
			}
			seen[p.ID] = true
			result = append(result, p)
		}
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

		// Libpod-shape optional fields (BUG-804). Sockerless pods are a
		// thin labelling wrapper over containers — we don't run a real
		// infra container and don't enforce per-pod cgroup / IPC /
		// blkio limits — so emit zero-valued shapes rather than
		// omitting. Empty slices/maps preferred over nil so the JSON
		// is always present in the expected type.
		Namespace:           "",
		CreateCommand:       []string{},
		ExitPolicy:          "continue",
		InfraContainerID:    "",
		InfraConfig:         api.PodInfraConfig{PortBindings: map[string][]api.PortBinding{}, DNSServer: []string{}, DNSSearch: []string{}, DNSOption: []string{}, HostAdd: []string{}, Networks: []string{}, NetworkOptions: map[string][]string{}},
		CgroupParent:        "",
		CgroupPath:          "",
		LockNumber:          0,
		RestartPolicy:       "",
		BlkioWeight:         0,
		CPUPeriod:           0,
		CPUQuota:            0,
		CPUShares:           0,
		CPUSetCPUs:          "",
		MemoryLimit:         0,
		MemorySwap:          0,
		BlkioDeviceReadBps:  []api.PodBlkioDeviceRate{},
		BlkioDeviceWriteBps: []api.PodBlkioDeviceRate{},
		VolumesFrom:         []string{},
		SecurityOpts:        []string{},
		Mounts:              []api.PodInspectMount{},
		Devices:             []api.PodInspectDevice{},
		Device_read_bps:     []api.PodBlkioDeviceRate{},
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

	stopExitCode := SignalToExitCode("SIGTERM") // 128+15 = 143 (BUG-826)
	for _, cid := range pod.ContainerIDs {
		c, ok := s.Store.Containers.Get(cid)
		if !ok || !c.State.Running {
			continue
		}
		s.StopHealthCheck(cid)
		s.Store.ForceStopContainer(cid, stopExitCode)
		s.emitEvent("container", "die", cid, map[string]string{
			"exitCode": fmt.Sprintf("%d", stopExitCode),
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
	exitCode := SignalToExitCode(signal)

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
