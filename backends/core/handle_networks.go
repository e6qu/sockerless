package core

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/sockerless/api"
)

func (s *BaseServer) handleNetworkCreate(w http.ResponseWriter, r *http.Request) {
	var req api.NetworkCreateRequest
	if err := ReadJSON(r, &req); err != nil {
		WriteError(w, &api.InvalidParameterError{Message: err.Error()})
		return
	}

	if req.Name == "" {
		WriteError(w, &api.InvalidParameterError{Message: "network name is required"})
		return
	}

	resp, err := s.Drivers.Network.Create(r.Context(), req.Name, &req)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			WriteError(w, &api.ConflictError{Message: err.Error()})
		} else {
			WriteError(w, &api.InvalidParameterError{Message: err.Error()})
		}
		return
	}

	s.emitEvent("network", "create", resp.ID, map[string]string{"name": req.Name})
	WriteJSON(w, http.StatusCreated, resp)
}

func (s *BaseServer) handleNetworkList(w http.ResponseWriter, r *http.Request) {
	filters := ParseFilters(r.URL.Query().Get("filters"))

	result, err := s.Drivers.Network.List(r.Context(), filters)
	if err != nil {
		WriteError(w, &api.ServerError{Message: err.Error()})
		return
	}
	WriteJSON(w, http.StatusOK, result)
}

func (s *BaseServer) handleNetworkInspect(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")
	n, err := s.Drivers.Network.Inspect(r.Context(), ref)
	if err != nil {
		WriteError(w, &api.NotFoundError{Resource: "network", ID: ref})
		return
	}
	WriteJSON(w, http.StatusOK, n)
}

func (s *BaseServer) handleNetworkDisconnect(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")
	net, ok := s.Store.ResolveNetwork(ref)
	if !ok {
		WriteError(w, &api.NotFoundError{Resource: "network", ID: ref})
		return
	}

	var req api.NetworkDisconnectRequest
	if err := ReadJSON(r, &req); err != nil {
		WriteError(w, &api.InvalidParameterError{Message: err.Error()})
		return
	}

	containerID, found := s.Store.ResolveContainerID(req.Container)
	if !found {
		if req.Force {
			w.WriteHeader(http.StatusOK)
			return
		}
		WriteError(w, &api.NotFoundError{Resource: "container", ID: req.Container})
		return
	}

	_ = s.Drivers.Network.Disconnect(r.Context(), net.ID, containerID)

	s.emitEvent("network", "disconnect", net.ID, map[string]string{"container": containerID})
	w.WriteHeader(http.StatusOK)
}

func (s *BaseServer) handleNetworkRemove(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")
	n, ok := s.Store.ResolveNetwork(ref)
	if !ok {
		WriteError(w, &api.NotFoundError{Resource: "network", ID: ref})
		return
	}

	if err := s.Drivers.Network.Remove(r.Context(), n.ID); err != nil {
		if strings.Contains(err.Error(), "pre-defined") {
			WriteError(w, &api.ConflictError{
				Message: fmt.Sprintf("%s is a pre-defined network and cannot be removed", n.Name),
			})
		} else {
			WriteError(w, &api.ServerError{Message: err.Error()})
		}
		return
	}

	s.emitEvent("network", "destroy", n.ID, map[string]string{"name": n.Name})
	w.WriteHeader(http.StatusNoContent)
}

func (s *BaseServer) handleNetworkPrune(w http.ResponseWriter, r *http.Request) {
	filters := ParseFilters(r.URL.Query().Get("filters"))

	resp, err := s.Drivers.Network.Prune(r.Context(), filters)
	if err != nil {
		WriteError(w, &api.ServerError{Message: err.Error()})
		return
	}
	WriteJSON(w, http.StatusOK, resp)
}

func (s *BaseServer) handleNetworkConnect(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")
	net, ok := s.Store.ResolveNetwork(ref)
	if !ok {
		WriteError(w, &api.NotFoundError{Resource: "network", ID: ref})
		return
	}

	var req api.NetworkConnectRequest
	if err := ReadJSON(r, &req); err != nil {
		WriteError(w, &api.InvalidParameterError{Message: err.Error()})
		return
	}

	containerID, ok := s.Store.ResolveContainerID(req.Container)
	if !ok {
		WriteError(w, &api.NotFoundError{Resource: "container", ID: req.Container})
		return
	}

	if err := s.Drivers.Network.Connect(r.Context(), net.ID, containerID, req.EndpointConfig); err != nil {
		WriteError(w, &api.ServerError{Message: err.Error()})
		return
	}

	s.emitEvent("network", "connect", net.ID, map[string]string{"container": containerID})

	// Implicit pod grouping: if this network already has a pod, join it
	if pod, exists := s.Store.Pods.GetPodForNetwork(net.Name); exists {
		if _, inPod := s.Store.Pods.GetPodForContainer(containerID); !inPod {
			_ = s.Store.Pods.AddContainer(pod.ID, containerID)
		}
	}

	w.WriteHeader(http.StatusOK)
}
