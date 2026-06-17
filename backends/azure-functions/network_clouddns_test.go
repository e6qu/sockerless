package azf

import (
	"testing"

	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
)

// A `docker network connect --network-alias X` after create (before start) must
// stamp the network + alias onto the PendingCreate, so ContainerStart's
// cloud-dns path VNet-integrates and registers it — exactly as the
// create-with-network path does. The synthetic network driver records the
// endpoint in Store.Containers, which this stateless backend doesn't use, so
// without cloudDNSNetworkConnect the alias would be lost.
func TestCloudDNSNetworkConnect_StampsPendingCreate(t *testing.T) {
	s := &Server{
		BaseServer: &core.BaseServer{
			Store:          core.NewStore(),
			PendingCreates: core.NewStateStore[api.Container](),
		},
		AZF: core.NewStateStore[AZFState](),
	}

	const netID = "netabc123"
	s.Store.Networks.Put(netID, api.Network{ID: netID, Name: "buildnet"})

	c := api.Container{
		ID:     "c1redis",
		Name:   "/redis-svc",
		Config: api.ContainerConfig{Labels: map[string]string{}},
		State:  api.ContainerState{Running: false},
	}
	s.PendingCreates.Put(c.ID, c)

	s.cloudDNSNetworkConnect(netID, &api.NetworkConnectRequest{
		Container:      "c1redis",
		EndpointConfig: &api.EndpointSettings{Aliases: []string{"redis"}},
	})

	got, ok := s.PendingCreates.Get("c1redis")
	if !ok {
		t.Fatal("container vanished from PendingCreates")
	}
	ep := got.NetworkSettings.Networks["buildnet"]
	if ep == nil {
		t.Fatal("network not stamped onto the PendingCreate")
	}
	if ep.NetworkID != netID {
		t.Errorf("NetworkID = %q, want %q", ep.NetworkID, netID)
	}
	if len(ep.Aliases) != 1 || ep.Aliases[0] != "redis" {
		t.Errorf("Aliases = %v, want [redis]", ep.Aliases)
	}
}
