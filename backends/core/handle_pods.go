package core

import (
	"fmt"
	"net/http"
	"strings"

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

	pod := s.Store.Pods.CreatePod(req.Name, req.Labels)

	if req.Hostname != "" {
		pod.Hostname = req.Hostname
	}
	if req.Share != "" {
		pod.SharedNS = strings.Split(req.Share, ",")
	}

	WriteJSON(w, http.StatusCreated, PodCreateResponse{ID: pod.ID})
}

func (s *BaseServer) handlePodList(w http.ResponseWriter, r *http.Request) {
	pods := s.Store.Pods.ListPods()
	result := make([]PodListEntry, 0, len(pods))
	for _, pod := range pods {
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

	s.Store.Pods.SetStatus(pod.ID, "running")
	WriteJSON(w, http.StatusOK, map[string]any{
		"Id":     pod.ID,
		"Errs":   []string{},
	})
}

func (s *BaseServer) handlePodStop(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("name")
	pod, ok := s.Store.Pods.GetPod(ref)
	if !ok {
		WriteError(w, &api.NotFoundError{Resource: "pod", ID: ref})
		return
	}

	s.Store.Pods.SetStatus(pod.ID, "stopped")
	WriteJSON(w, http.StatusOK, map[string]any{
		"Id":     pod.ID,
		"Errs":   []string{},
	})
}

func (s *BaseServer) handlePodKill(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("name")
	pod, ok := s.Store.Pods.GetPod(ref)
	if !ok {
		WriteError(w, &api.NotFoundError{Resource: "pod", ID: ref})
		return
	}

	s.Store.Pods.SetStatus(pod.ID, "exited")
	WriteJSON(w, http.StatusOK, map[string]any{
		"Id":     pod.ID,
		"Errs":   []string{},
	})
}

func (s *BaseServer) handlePodRemove(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("name")
	pod, ok := s.Store.Pods.GetPod(ref)
	if !ok {
		WriteError(w, &api.NotFoundError{Resource: "pod", ID: ref})
		return
	}

	// Remove containers if force is set
	force := r.URL.Query().Get("force") == "true" || r.URL.Query().Get("force") == "1"
	if force {
		for _, cid := range pod.ContainerIDs {
			c, ok := s.Store.Containers.Get(cid)
			if !ok {
				continue
			}
			if c.State.Running {
				s.Store.ForceStopContainer(cid, 0)
			}
			s.Store.Containers.Delete(cid)
			s.Store.ContainerNames.Delete(c.Name)
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
