package cloudrun

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/sockerless/api"
)

// memberOnNetwork builds a container joined to a user-defined network with the
// given network ID + alias.
func memberOnNetwork(id, name, netID, alias string, openStdin bool) *api.Container {
	c := &api.Container{ID: id, Name: name}
	c.Config.Image = "img-" + name
	c.Config.OpenStdin = openStdin
	c.NetworkSettings.Networks = map[string]*api.EndpointSettings{
		"mynet": {NetworkID: netID, Aliases: []string{alias}},
	}
	return c
}

// TestNetworkServiceMembers_PersistAndRebuild proves a network's service-style
// member (e.g. a `services:` redis) round-trips through the Service revision
// annotations and is rebuilt into networkServices after a simulated restart —
// the script-runner (OpenStdin) is NOT persisted (it is a per-stage transient).
func TestNetworkServiceMembers_PersistAndRebuild(t *testing.T) {
	const netID = "net-deadbeef0001"
	runner := memberOnNetwork("runner000001stage1", "/runner", netID, "runner", true)
	redis := memberOnNetwork("redis0000000000001", "/redis", netID, "redis", false)

	s := newServerForState(t)
	svc, err := s.buildServiceSpec(s.ctx(), []containerInput{
		{ID: runner.ID, Container: runner, IsMain: true},
		{ID: redis.ID, Container: redis, IsMain: false},
	})
	if err != nil {
		t.Fatalf("buildServiceSpec: %v", err)
	}

	if got := svc.Annotations[networkIDAnnotation]; got != netID {
		t.Fatalf("network id annotation = %q, want %q", got, netID)
	}
	blob := svc.Annotations[networkServiceMembersAnnotation]
	if blob == "" {
		t.Fatal("service members annotation missing")
	}

	// Decode and confirm only the service-style member (redis) is persisted.
	decoded, err := base64.StdEncoding.DecodeString(blob)
	if err != nil {
		t.Fatalf("decode members: %v", err)
	}
	var persisted []api.Container
	if err := json.Unmarshal(decoded, &persisted); err != nil {
		t.Fatalf("unmarshal members: %v", err)
	}
	if len(persisted) != 1 || persisted[0].ID != redis.ID {
		t.Fatalf("persisted members = %+v, want only redis (%s)", persisted, redis.ID)
	}

	// Simulate a backend restart: a fresh server with an empty networkServices
	// map applies the persisted blob, exactly as rebuildNetworkServicesFromCloud
	// does after listing the Service.
	fresh := newServerForState(t)
	if n := fresh.applyNetworkServiceMembers(netID, blob); n != 1 {
		t.Fatalf("applyNetworkServiceMembers = %d, want 1", n)
	}

	members := fresh.serviceMembersOfNetwork(netID)
	if len(members) != 1 || members[0].ID != redis.ID {
		t.Fatalf("serviceMembersOfNetwork after rebuild = %+v, want redis (%s)", members, redis.ID)
	}
	if members[0].Config.Image != redis.Config.Image {
		t.Fatalf("rebuilt member lost config: image = %q, want %q", members[0].Config.Image, redis.Config.Image)
	}
}

// TestNetworkServiceMembers_NoNetworkNoAnnotation confirms a single-container
// (no user-defined network) Service carries no member annotations.
func TestNetworkServiceMembers_NoNetworkNoAnnotation(t *testing.T) {
	c := &api.Container{ID: "solo000000000000001", Name: "/solo"}
	c.Config.Image = "img-solo"

	s := newServerForState(t)
	svc, err := s.buildServiceSpec(s.ctx(), []containerInput{
		{ID: c.ID, Container: c, IsMain: true},
	})
	if err != nil {
		t.Fatalf("buildServiceSpec: %v", err)
	}
	if _, ok := svc.Annotations[networkServiceMembersAnnotation]; ok {
		t.Fatal("solo container must not carry service-member annotation")
	}
}
