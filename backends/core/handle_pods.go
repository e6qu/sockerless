package core

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/sockerless/api"
)

// PodCreateRequest is the Podman-compatible pod creation request body.
type PodCreateRequest struct {
	Name     string            `json:"name"`
	Labels   map[string]string `json:"labels,omitempty"`
	Hostname string            `json:"hostname,omitempty"`
	Share    string            `json:"share,omitempty"` // comma-separated: "ipc,net,uts"
	NoInfra  bool              `json:"no_infra,omitempty"`
}

// PodCreateResponse is returned after successful pod creation.
type PodCreateResponse struct {
	ID string `json:"Id"`
}

// PodInspectResponse is the Podman-compatible pod inspect response.
type PodInspectResponse struct {
	ID               string             `json:"Id"`
	Name             string             `json:"Name"`
	Created          string             `json:"Created"`
	State            string             `json:"State"`
	Hostname         string             `json:"Hostname"`
	Labels           map[string]string  `json:"Labels"`
	NumContainers    int                `json:"NumContainers"`
	Containers       []PodContainerInfo `json:"Containers"`
	SharedNamespaces []string           `json:"SharedNamespaces"`
}

// PodContainerInfo describes a container within a pod.
type PodContainerInfo struct {
	ID    string `json:"Id"`
	Name  string `json:"Name"`
	State string `json:"State"`
}

// PodListEntry is a single entry in the pod list response.
type PodListEntry struct {
	ID         string            `json:"Id"`
	Name       string            `json:"Name"`
	Status     string            `json:"Status"`
	Created    string            `json:"Created"`
	Labels     map[string]string `json:"Labels"`
	Containers []PodContainerInfo `json:"Containers"`
}

func (s *BaseServer) handlePodCreate(w http.ResponseWriter, r *http.Request) {
	var req PodCreateRequest
	if err := ReadJSON(r, &req); err != nil {
		WriteError(w, &api.InvalidParameterError{Message: err.Error()})
		return
	}

	if req.Name == "" {
		WriteError(w, &api.InvalidParameterError{Message: "pod name is required"})
		return
	}

	// Check for duplicate name
	if s.Store.Pods.Exists(req.Name) {
		WriteError(w, &api.ConflictError{
			Message: fmt.Sprintf("pod with name %s already exists", req.Name),
		})
		return
	}

	hostname := req.Hostname
	var sharedNS []string
	if req.Share != "" {
		sharedNS = strings.Split(req.Share, ",")
	}
	pod := s.Store.Pods.CreatePodWithOpts(req.Name, req.Labels, hostname, sharedNS)

	WriteJSON(w, http.StatusCreated, PodCreateResponse{ID: pod.ID})
}

func (s *BaseServer) handlePodList(w http.ResponseWriter, r *http.Request) {
	filters := ParseFilters(r.URL.Query().Get("filters"))
	pods := s.Store.Pods.ListPods()
	result := make([]PodListEntry, 0, len(pods))
	for _, pod := range pods {
		if !matchPodFilters(pod, filters) {
			continue
		}
		containers := s.buildPodContainerInfos(pod)
		result = append(result, PodListEntry{
			ID:         pod.ID,
			Name:       pod.Name,
			Status:     pod.Status,
			Created:    pod.Created,
			Labels:     pod.Labels,
			Containers: containers,
		})
	}
	WriteJSON(w, http.StatusOK, result)
}

func matchPodFilters(pod *PodContext, filters map[string][]string) bool {
	for key, values := range filters {
		switch key {
		case "name":
			matched := false
			for _, v := range values {
				if strings.Contains(pod.Name, v) {
					matched = true
					break
				}
			}
			if !matched {
				return false
			}
		case "id":
			matched := false
			for _, v := range values {
				if strings.HasPrefix(pod.ID, v) {
					matched = true
					break
				}
			}
			if !matched {
				return false
			}
		case "label":
			if !MatchLabels(pod.Labels, values) {
				return false
			}
		case "status":
			matched := false
			for _, v := range values {
				if pod.Status == v {
					matched = true
					break
				}
			}
			if !matched {
				return false
			}
		}
	}
	return true
}

func (s *BaseServer) handlePodInspect(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("name")
	pod, ok := s.Store.Pods.GetPod(ref)
	if !ok {
		WriteError(w, &api.NotFoundError{Resource: "pod", ID: ref})
		return
	}

	containers := s.buildPodContainerInfos(pod)
	WriteJSON(w, http.StatusOK, PodInspectResponse{
		ID:               pod.ID,
		Name:             pod.Name,
		Created:          pod.Created,
		State:            pod.Status,
		Hostname:         pod.Hostname,
		Labels:           pod.Labels,
		NumContainers:    len(pod.ContainerIDs),
		Containers:       containers,
		SharedNamespaces: pod.SharedNS,
	})
}

func (s *BaseServer) handlePodExists(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("name")
	if s.Store.Pods.Exists(ref) {
		w.WriteHeader(http.StatusNoContent)
	} else {
		WriteError(w, &api.NotFoundError{Resource: "pod", ID: ref})
	}
}

func (s *BaseServer) handlePodStart(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("name")
	pod, ok := s.Store.Pods.GetPod(ref)
	if !ok {
		WriteError(w, &api.NotFoundError{Resource: "pod", ID: ref})
		return
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, cid := range pod.ContainerIDs {
		c, ok := s.Store.Containers.Get(cid)
		if !ok || c.State.Running {
			continue
		}
		exitCh := make(chan struct{})
		s.Store.WaitChs.Store(cid, exitCh)
		s.Store.Containers.Update(cid, func(c *api.Container) {
			c.State.Status = "running"
			c.State.Running = true
			c.State.Pid = 42
			c.State.StartedAt = now
			c.State.FinishedAt = "0001-01-01T00:00:00Z"
			c.State.ExitCode = 0
		})

		// BUG-378: Spawn container process via driver chain
		cmd := append([]string{c.Path}, c.Args...)
		binds := s.resolveBindMounts(c.HostConfig.Binds, c.HostConfig.Mounts)
		_, _ = s.Drivers.ProcessLifecycle.Start(cid, cmd, c.Config.Env, binds)

		// BUG-379: Start health check if configured
		if c.Config.Healthcheck != nil && len(c.Config.Healthcheck.Test) > 0 &&
			(len(c.Config.Healthcheck.Test) != 1 || !strings.EqualFold(c.Config.Healthcheck.Test[0], "NONE")) {
			s.StartHealthCheck(cid)
		}

		s.emitEvent("container", "start", cid, map[string]string{
			"name": strings.TrimPrefix(c.Name, "/"),
		})
	}
	s.Store.Pods.SetStatus(pod.ID, "running")
	WriteJSON(w, http.StatusOK, map[string]any{
		"Id":   pod.ID,
		"Errs": []string{},
	})
}

func (s *BaseServer) handlePodStop(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("name")
	pod, ok := s.Store.Pods.GetPod(ref)
	if !ok {
		WriteError(w, &api.NotFoundError{Resource: "pod", ID: ref})
		return
	}

	for _, cid := range pod.ContainerIDs {
		c, ok := s.Store.Containers.Get(cid)
		if !ok || !c.State.Running {
			continue
		}
		s.StopHealthCheck(cid)
		s.Drivers.ProcessLifecycle.Stop(cid)
		s.Drivers.ProcessLifecycle.Cleanup(cid)
		s.Store.ForceStopContainer(cid, 0)
		s.emitEvent("container", "die", cid, map[string]string{
			"exitCode": "0",
			"name":     strings.TrimPrefix(c.Name, "/"),
		})
		s.emitEvent("container", "stop", cid, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})
	}
	s.Store.Pods.SetStatus(pod.ID, "stopped")
	WriteJSON(w, http.StatusOK, map[string]any{
		"Id":   pod.ID,
		"Errs": []string{},
	})
}

func (s *BaseServer) handlePodKill(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("name")
	pod, ok := s.Store.Pods.GetPod(ref)
	if !ok {
		WriteError(w, &api.NotFoundError{Resource: "pod", ID: ref})
		return
	}

	signal := r.URL.Query().Get("signal")
	if signal == "" {
		signal = "SIGKILL"
	}
	exitCode := signalToExitCode(signal)

	for _, cid := range pod.ContainerIDs {
		c, ok := s.Store.Containers.Get(cid)
		if !ok || !c.State.Running {
			continue
		}
		s.Drivers.ProcessLifecycle.Kill(cid)
		s.StopHealthCheck(cid)
		s.Drivers.ProcessLifecycle.Cleanup(cid)
		s.Store.ForceStopContainer(cid, exitCode)
		s.emitEvent("container", "kill", cid, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})
		s.emitEvent("container", "die", cid, map[string]string{
			"exitCode": fmt.Sprintf("%d", exitCode),
			"name":     strings.TrimPrefix(c.Name, "/"),
		})
	}
	s.Store.Pods.SetStatus(pod.ID, "exited")
	WriteJSON(w, http.StatusOK, map[string]any{
		"Id":   pod.ID,
		"Errs": []string{},
	})
}

func (s *BaseServer) handlePodRemove(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("name")
	pod, ok := s.Store.Pods.GetPod(ref)
	if !ok {
		WriteError(w, &api.NotFoundError{Resource: "pod", ID: ref})
		return
	}

	force := r.URL.Query().Get("force") == "true" || r.URL.Query().Get("force") == "1"

	// Without force, reject if any containers are running
	if !force {
		for _, cid := range pod.ContainerIDs {
			c, ok := s.Store.Containers.Get(cid)
			if ok && c.State.Running {
				WriteError(w, &api.ConflictError{
					Message: fmt.Sprintf("pod %s has running containers, cannot remove without force", ref),
				})
				return
			}
		}
	}

	// Remove containers if force is set
	if force {
		for _, cid := range pod.ContainerIDs {
			c, ok := s.Store.Containers.Get(cid)
			if !ok {
				continue
			}
			if c.State.Running {
				s.Store.ForceStopContainer(cid, 0)
			}
			s.StopHealthCheck(cid)
			s.Drivers.ProcessLifecycle.Cleanup(cid)
			for _, ep := range c.NetworkSettings.Networks {
				if ep != nil && ep.NetworkID != "" {
					_ = s.Drivers.Network.Disconnect(r.Context(), ep.NetworkID, cid)
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
	} else {
		// Clean up all (non-running) containers in the pod
		for _, cid := range pod.ContainerIDs {
			c, ok := s.Store.Containers.Get(cid)
			if !ok {
				continue
			}
			// BUG-380: Stop health check for non-force path
			s.StopHealthCheck(cid)
			// BUG-381: Clean up process resources for non-force path
			s.Drivers.ProcessLifecycle.Cleanup(cid)
			// BUG-383: Disconnect networks for non-force path
			for _, ep := range c.NetworkSettings.Networks {
				if ep != nil && ep.NetworkID != "" {
					_ = s.Drivers.Network.Disconnect(r.Context(), ep.NetworkID, cid)
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
			// BUG-382: Emit destroy event for non-force path
			s.emitEvent("container", "destroy", cid, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})
		}
	}

	s.Store.Pods.DeletePod(pod.ID)
	w.WriteHeader(http.StatusNoContent)
}

// buildPodContainerInfos returns container info for a pod's containers.
func (s *BaseServer) buildPodContainerInfos(pod *PodContext) []PodContainerInfo {
	infos := make([]PodContainerInfo, 0, len(pod.ContainerIDs))
	for _, cid := range pod.ContainerIDs {
		c, ok := s.Store.Containers.Get(cid)
		if !ok {
			continue
		}
		infos = append(infos, PodContainerInfo{
			ID:    cid,
			Name:  strings.TrimPrefix(c.Name, "/"),
			State: c.State.Status,
		})
	}
	return infos
}
