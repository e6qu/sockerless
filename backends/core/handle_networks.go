package core

import (
	"fmt"
	"net/http"
	"strings"
	"time"

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

	// Check for duplicate name
	for _, n := range s.Store.Networks.List() {
		if n.Name == req.Name {
			WriteError(w, &api.ConflictError{
				Message: fmt.Sprintf("network with name %s already exists", req.Name),
			})
			return
		}
	}

	id := GenerateID()
	driver := req.Driver
	if driver == "" {
		driver = "bridge"
	}

	ipam := api.IPAM{Driver: "default"}
	if req.IPAM != nil {
		ipam = *req.IPAM
	}
	if len(ipam.Config) == 0 {
		subnet := fmt.Sprintf("172.%d.0.0/16", 18+s.Store.Networks.Len())
		gateway := fmt.Sprintf("172.%d.0.1", 18+s.Store.Networks.Len())
		ipam.Config = []api.IPAMConfig{
			{Subnet: subnet, Gateway: gateway},
		}
	}

	labels := req.Labels
	if labels == nil {
		labels = make(map[string]string)
	}
	options := req.Options
	if options == nil {
		options = make(map[string]string)
	}

	network := api.Network{
		Name:       req.Name,
		ID:         id,
		Created:    time.Now().UTC().Format(time.RFC3339Nano),
		Scope:      "local",
		Driver:     driver,
		EnableIPv6: req.EnableIPv6,
		IPAM:       ipam,
		Internal:   req.Internal,
		Attachable: req.Attachable,
		Ingress:    req.Ingress,
		Containers: make(map[string]api.EndpointResource),
		Options:    options,
		Labels:     labels,
	}

	s.Store.Networks.Put(id, network)

	WriteJSON(w, http.StatusCreated, api.NetworkCreateResponse{ID: id})
}

func (s *BaseServer) handleNetworkList(w http.ResponseWriter, r *http.Request) {
	filters := ParseFilters(r.URL.Query().Get("filters"))

	var result []*api.Network
	for _, n := range s.Store.Networks.List() {
		if !MatchNetworkFilters(n, filters) {
			continue
		}
		n := n
		result = append(result, &n)
	}
	if result == nil {
		result = []*api.Network{}
	}
	WriteJSON(w, http.StatusOK, result)
}

func (s *BaseServer) handleNetworkInspect(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")
	n, ok := s.Store.ResolveNetwork(ref)
	if !ok {
		WriteError(w, &api.NotFoundError{Resource: "network", ID: ref})
		return
	}
	WriteJSON(w, http.StatusOK, n)
}

func (s *BaseServer) handleNetworkDisconnect(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")
	_, ok := s.Store.ResolveNetwork(ref)
	if !ok {
		WriteError(w, &api.NotFoundError{Resource: "network", ID: ref})
		return
	}

	var req api.NetworkDisconnectRequest
	if err := ReadJSON(r, &req); err != nil {
		WriteError(w, &api.InvalidParameterError{Message: err.Error()})
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *BaseServer) handleNetworkRemove(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")
	n, ok := s.Store.ResolveNetwork(ref)
	if !ok {
		WriteError(w, &api.NotFoundError{Resource: "network", ID: ref})
		return
	}

	if n.Name == "bridge" || n.Name == "host" || n.Name == "none" {
		WriteError(w, &api.ConflictError{
			Message: fmt.Sprintf("%s is a pre-defined network and cannot be removed", n.Name),
		})
		return
	}

	s.Store.Networks.Delete(n.ID)
	w.WriteHeader(http.StatusNoContent)
}

func (s *BaseServer) handleNetworkPrune(w http.ResponseWriter, r *http.Request) {
	filters := ParseFilters(r.URL.Query().Get("filters"))

	var deleted []string
	for _, n := range s.Store.Networks.List() {
		if n.Name == "bridge" || n.Name == "host" || n.Name == "none" {
			continue
		}
		if len(n.Containers) == 0 {
			if !MatchNetworkPruneFilters(n, filters) {
				continue
			}
			s.Store.Networks.Delete(n.ID)
			deleted = append(deleted, n.ID)
		}
	}
	if deleted == nil {
		deleted = []string{}
	}
	WriteJSON(w, http.StatusOK, api.NetworkPruneResponse{NetworksDeleted: deleted})
}

func (s *BaseServer) handleNetworkConnect(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")
	net, ok := s.Store.ResolveNetwork(ref)
	if !ok {
		WriteError(w, &api.NotFoundError{Resource: "network", ID: ref})
		return
	}
	networkID := net.ID

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

	// Add container to network, resolving IPAM from the actual network
	gateway := "172.18.0.1"
	ipBase := "172.18.0."
	if len(net.IPAM.Config) > 0 {
		gateway = net.IPAM.Config[0].Gateway
		ipBase = gateway[:strings.LastIndex(gateway, ".")+1]
	}
	endpoint := api.EndpointSettings{
		NetworkID:   networkID,
		EndpointID:  GenerateID()[:16],
		Gateway:     gateway,
		IPAddress:   fmt.Sprintf("%s%d", ipBase, len(net.Containers)+2),
		IPPrefixLen: 16,
		MacAddress:  "02:42:ac:12:00:02",
	}
	if req.EndpointConfig != nil {
		if req.EndpointConfig.IPAddress != "" {
			endpoint.IPAddress = req.EndpointConfig.IPAddress
		}
		if len(req.EndpointConfig.Aliases) > 0 {
			endpoint.Aliases = req.EndpointConfig.Aliases
		}
	}

	s.Store.Networks.Update(networkID, func(n *api.Network) {
		c, _ := s.Store.Containers.Get(containerID)
		n.Containers[containerID] = api.EndpointResource{
			Name:        strings.TrimPrefix(c.Name, "/"),
			EndpointID:  endpoint.EndpointID,
			MacAddress:  endpoint.MacAddress,
			IPv4Address: endpoint.IPAddress + "/16",
		}
	})

	s.Store.Containers.Update(containerID, func(c *api.Container) {
		c.NetworkSettings.Networks[net.Name] = &endpoint
	})

	w.WriteHeader(http.StatusOK)
}
