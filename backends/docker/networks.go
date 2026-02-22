package docker

import (
	"net/http"

	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	"github.com/sockerless/api"
)

func (s *Server) handleNetworkCreate(w http.ResponseWriter, r *http.Request) {
	var req api.NetworkCreateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, &api.InvalidParameterError{Message: err.Error()})
		return
	}

	opts := network.CreateOptions{
		Driver:     req.Driver,
		Internal:   req.Internal,
		Attachable: req.Attachable,
		Ingress:    req.Ingress,
		EnableIPv6: &req.EnableIPv6,
		Options:    req.Options,
		Labels:     req.Labels,
	}

	if req.IPAM != nil {
		ipamConfigs := make([]network.IPAMConfig, len(req.IPAM.Config))
		for i, c := range req.IPAM.Config {
			ipamConfigs[i] = network.IPAMConfig{
				Subnet:  c.Subnet,
				IPRange: c.IPRange,
				Gateway: c.Gateway,
			}
		}
		opts.IPAM = &network.IPAM{
			Driver: req.IPAM.Driver,
			Config: ipamConfigs,
		}
	}

	resp, err := s.docker.NetworkCreate(r.Context(), req.Name, opts)
	if err != nil {
		writeError(w, mapDockerError(err))
		return
	}

	writeJSON(w, http.StatusCreated, api.NetworkCreateResponse{
		ID:      resp.ID,
		Warning: resp.Warning,
	})
}

func (s *Server) handleNetworkList(w http.ResponseWriter, r *http.Request) {
	networks, err := s.docker.NetworkList(r.Context(), network.ListOptions{})
	if err != nil {
		writeError(w, mapDockerError(err))
		return
	}

	result := make([]*api.Network, 0, len(networks))
	for _, n := range networks {
		net := &api.Network{
			Name:       n.Name,
			ID:         n.ID,
			Created:    n.Created.Format("2006-01-02T15:04:05.999999999Z07:00"),
			Scope:      n.Scope,
			Driver:     n.Driver,
			EnableIPv6: n.EnableIPv6,
			Internal:   n.Internal,
			Attachable: n.Attachable,
			Ingress:    n.Ingress,
			Labels:     n.Labels,
			Options:    n.Options,
		}
		result = append(result, net)
	}

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleNetworkInspect(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	n, err := s.docker.NetworkInspect(r.Context(), id, network.InspectOptions{})
	if err != nil {
		writeError(w, mapDockerError(err))
		return
	}

	net := api.Network{
		Name:       n.Name,
		ID:         n.ID,
		Created:    n.Created.Format("2006-01-02T15:04:05.999999999Z07:00"),
		Scope:      n.Scope,
		Driver:     n.Driver,
		EnableIPv6: n.EnableIPv6,
		Internal:   n.Internal,
		Attachable: n.Attachable,
		Ingress:    n.Ingress,
		Labels:     n.Labels,
		Options:    n.Options,
	}
	writeJSON(w, http.StatusOK, net)
}

func (s *Server) handleNetworkDisconnect(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req api.NetworkDisconnectRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, &api.InvalidParameterError{Message: err.Error()})
		return
	}

	err := s.docker.NetworkDisconnect(r.Context(), id, req.Container, req.Force)
	if err != nil {
		writeError(w, mapDockerError(err))
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleNetworkRemove(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.docker.NetworkRemove(r.Context(), id); err != nil {
		writeError(w, mapDockerError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleNetworkPrune(w http.ResponseWriter, r *http.Request) {
	report, err := s.docker.NetworksPrune(r.Context(), filters.Args{})
	if err != nil {
		writeError(w, mapDockerError(err))
		return
	}

	deleted := report.NetworksDeleted
	if deleted == nil {
		deleted = []string{}
	}

	writeJSON(w, http.StatusOK, api.NetworkPruneResponse{
		NetworksDeleted: deleted,
	})
}
