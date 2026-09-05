package core

import (
	"reflect"
	"testing"

	"github.com/sockerless/api"
)

func TestIsBuiltinNetwork(t *testing.T) {
	for name, want := range map[string]bool{"": true, "default": true, "Bridge": true, "host": true, "none": true, "ci-net": false} {
		if got := IsBuiltinNetwork(name); got != want {
			t.Errorf("IsBuiltinNetwork(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestIsLikelyAlias(t *testing.T) {
	for name, want := range map[string]bool{
		"postgres":                         true,
		"redis-cache":                      true,
		"abcdef012345":                     true, // 12 hex chars is a legal short alias
		"abcdef0123456789abcdef":           false,
		"":                                 false,
		"a/b":                              false,
		"host:80":                          false,
		"runner-job-container-name-longer": true,
	} {
		if got := IsLikelyAlias(name); got != want {
			t.Errorf("IsLikelyAlias(%q) = %v, want %v", name, got, want)
		}
	}
}

func member(id, name, hostname string, aliases ...string) api.Container {
	c := api.Container{ID: id, Name: name}
	c.Config.Hostname = hostname
	c.NetworkSettings.Networks = map[string]*api.EndpointSettings{
		"net-1": {NetworkID: "net-1", Aliases: aliases},
		"other": {NetworkID: "other", Aliases: []string{"ignored"}},
	}
	return c
}

func TestHostAliasesForMembers(t *testing.T) {
	members := []api.Container{
		member("c1", "/postgres", "pghost", "db", "postgres"),
		member("c2", "/0123456789abcdef0123", "", "cache", "db"),
		{ID: "c3", Name: "/nil-endpoint", NetworkSettings: api.NetworkSettings{Networks: map[string]*api.EndpointSettings{"net-1": nil}}},
	}
	got := HostAliasesForMembers(members, "net-1")
	want := []string{"db", "postgres", "pghost", "cache", "nil-endpoint"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("HostAliasesForMembers = %v, want %v", got, want)
	}
}

func TestDeferOrMaterializeNetworkPod(t *testing.T) {
	service := api.Container{ID: "svc"}
	job := api.Container{ID: "job"}
	runner := api.Container{ID: "runner"}
	runner.Config.OpenStdin = true
	pinned := api.Container{ID: "pinned"}

	t.Run("service alone defers", func(t *testing.T) {
		defer_, members := DeferOrMaterializeNetworkPod(service, nil, nil)
		if !defer_ || members != nil {
			t.Fatalf("= %v, %v; want defer", defer_, members)
		}
	})
	t.Run("service joining a job container: first sibling is main", func(t *testing.T) {
		defer_, members := DeferOrMaterializeNetworkPod(service, []api.Container{job}, nil)
		if defer_ || len(members) != 2 || members[0].ID != "job" || members[1].ID != "svc" {
			t.Fatalf("= %v, %v", defer_, members)
		}
	})
	t.Run("script-runner alone falls through", func(t *testing.T) {
		defer_, members := DeferOrMaterializeNetworkPod(runner, nil, nil)
		if defer_ || members != nil {
			t.Fatalf("= %v, %v; want single-container path", defer_, members)
		}
	})
	t.Run("script-runner is main, siblings and pinned services join once", func(t *testing.T) {
		defer_, members := DeferOrMaterializeNetworkPod(runner, []api.Container{service}, []api.Container{pinned, service})
		if defer_ {
			t.Fatal("must not defer")
		}
		var ids []string
		for _, m := range members {
			ids = append(ids, m.ID)
		}
		if want := []string{"runner", "svc", "pinned"}; !reflect.DeepEqual(ids, want) {
			t.Fatalf("members = %v, want %v", ids, want)
		}
	})
	t.Run("script-runner with only pinned services still materializes", func(t *testing.T) {
		defer_, members := DeferOrMaterializeNetworkPod(runner, nil, []api.Container{pinned})
		if defer_ || len(members) != 2 {
			t.Fatalf("= %v, %v", defer_, members)
		}
	})
}
