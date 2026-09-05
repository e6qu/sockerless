package azf

import (
	"encoding/base64"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
)

func networkedContainer(id, name, netID string, openStdin bool, aliases ...string) api.Container {
	return api.Container{
		ID:   id,
		Name: "/" + name,
		Config: api.ContainerConfig{
			OpenStdin: openStdin,
			Labels:    map[string]string{},
		},
		NetworkSettings: api.NetworkSettings{
			Networks: map[string]*api.EndpointSettings{
				"buildnet": {NetworkID: netID, Aliases: aliases},
			},
		},
	}
}

func testServer() *Server {
	return &Server{
		BaseServer: &core.BaseServer{
			PendingCreates: core.NewStateStore[api.Container](),
		},
		AZF: core.NewStateStore[AZFState](),
	}
}

func TestIsBuiltinNetwork(t *testing.T) {
	for _, n := range []string{"", "default", "bridge", "host", "none", "BRIDGE"} {
		if !core.IsBuiltinNetwork(n) {
			t.Errorf("%q should be builtin", n)
		}
	}
	for _, n := range []string{"buildnet", "github_network_x", "skls-foo"} {
		if core.IsBuiltinNetwork(n) {
			t.Errorf("%q should NOT be builtin", n)
		}
	}
}

func TestShouldDeferOrMaterialize_GitLabPattern(t *testing.T) {
	s := testServer()
	// Service (postgres) created+started first with no siblings → defer.
	svc := networkedContainer("svc1", "redis", "net1", false, "redis")
	s.PendingCreates.Put(svc.ID, svc)
	if defer1, members := s.shouldDeferOrMaterializeNetworkPod(svc); !defer1 || members != nil {
		t.Fatalf("lone service should defer: defer=%v members=%v", defer1, members)
	}

	// Script-runner (OpenStdin) starts → materialize with itself as main +
	// the service as sidecar.
	runner := networkedContainer("run1", "build", "net1", true)
	s.PendingCreates.Put(runner.ID, runner)
	defer2, members := s.shouldDeferOrMaterializeNetworkPod(runner)
	if defer2 || len(members) != 2 {
		t.Fatalf("script-runner with sibling should materialize 2: defer=%v members=%d", defer2, len(members))
	}
	if members[0].ID != "run1" {
		t.Errorf("main (members[0]) must be the script-runner, got %s", members[0].ID)
	}
}

func TestShouldDeferOrMaterialize_GitHubPattern(t *testing.T) {
	s := testServer()
	// Job container (OpenStdin=false) created first → defer (no siblings).
	job := networkedContainer("job1", "job", "net1", false)
	s.PendingCreates.Put(job.ID, job)
	if d, m := s.shouldDeferOrMaterializeNetworkPod(job); !d || m != nil {
		t.Fatalf("lone job container should defer: defer=%v members=%v", d, m)
	}
	// Service arrives → materialize with the job (siblings[0]) as main.
	svc := networkedContainer("svc1", "postgres", "net1", false, "postgres")
	s.PendingCreates.Put(svc.ID, svc)
	d, m := s.shouldDeferOrMaterializeNetworkPod(svc)
	if d || len(m) != 2 {
		t.Fatalf("service completing a pod should materialize 2: defer=%v members=%d", d, len(m))
	}
	if m[0].ID != "job1" {
		t.Errorf("main must be the first-created job container, got %s", m[0].ID)
	}
}

func TestShouldDeferOrMaterialize_SingleFallThrough(t *testing.T) {
	s := testServer()
	lone := networkedContainer("c1", "build", "net1", true)
	s.PendingCreates.Put(lone.ID, lone)
	if d, m := s.shouldDeferOrMaterializeNetworkPod(lone); d || m != nil {
		t.Fatalf("lone OpenStdin container should fall through to single: defer=%v members=%v", d, m)
	}
}

func TestPendingMembersFiltersMaterializedMain(t *testing.T) {
	s := testServer()
	oldMain := networkedContainer("old", "build", "net1", true)
	s.PendingCreates.Put(oldMain.ID, oldMain)
	s.AZF.Put(oldMain.ID, AZFState{FunctionURL: "http://x"}) // already materialized
	svc := networkedContainer("svc", "redis", "net1", false)
	s.PendingCreates.Put(svc.ID, svc)

	got := s.pendingMembersOfNetwork("net1", "other")
	for _, c := range got {
		if c.ID == "old" {
			t.Error("already-materialized main must be filtered out of pending members")
		}
	}
}

func TestEncodeDecodePodMembers(t *testing.T) {
	in := []podMember{
		{ID: "a", Name: "build", Image: "overlay", IsMain: true},
		{ID: "b", Name: "redis", Image: "redis:7", IsMain: false},
	}
	out := decodePodMembers(encodePodMembers(in))
	if !reflect.DeepEqual(in, out) {
		t.Fatalf("round-trip mismatch:\n got %+v\nwant %+v", out, in)
	}
	if decodePodMembers("") != nil || decodePodMembers("!!!notb64") != nil {
		t.Error("invalid manifest should decode to nil")
	}
}

func TestSanitizeSiteContainerName(t *testing.T) {
	cases := map[string]string{
		"/redis":         "redis",
		"/My_Service":    "my-service",
		"/postgres:5432": "postgres-5432",
		"/":              "sidecar-3",
	}
	for in, want := range cases {
		if got := sanitizeSiteContainerName(in, 3); got != want {
			t.Errorf("sanitize(%q)=%q want %q", in, got, want)
		}
	}
}

func TestPodMemberImageAndStartup(t *testing.T) {
	ep, _ := json.Marshal([]string{"redis-server"})
	cmd, _ := json.Marshal([]string{"--port", "6379"})
	side := api.Container{Config: api.ContainerConfig{
		Image: "sockerless-overlay/azf-redis:test", // overlay applied at create
		Labels: map[string]string{
			labelBaseImage:      "redis:7-alpine",
			labelBaseEntrypoint: base64.StdEncoding.EncodeToString(ep),
			labelBaseCmd:        base64.StdEncoding.EncodeToString(cmd),
		},
	}}
	// docker ps shows the ORIGINAL image, not the overlay that runs.
	if got := podMemberDisplayImage(side); got != "redis:7-alpine" {
		t.Errorf("display image = %q, want raw redis:7-alpine", got)
	}
	// The container is overlaid (run image differs from the base).
	if !isAZFOverlaid(side) {
		t.Error("expected sidecar to be detected as overlaid")
	}
	if got := podMemberRawArgv(side); !reflect.DeepEqual(got, []string{"redis-server", "--port", "6379"}) {
		t.Errorf("raw argv = %v", got)
	}
}

func TestSidecarRunSpec(t *testing.T) {
	s := &Server{}
	s.config.CallbackURL = "ws://host.docker.internal:3375/v1/azf/reverse"
	ep, _ := json.Marshal([]string{"redis-server"})
	// Overlaid sidecar: runs the overlay in sidecar mode + reverse-agent env.
	over := api.Container{ID: "sc1", Config: api.ContainerConfig{
		Image:  "sockerless-overlay/azf-redis:test",
		Env:    []string{"FOO=bar"},
		Labels: map[string]string{labelBaseImage: "redis:7-alpine", labelBaseEntrypoint: base64.StdEncoding.EncodeToString(ep)},
	}}
	img, startUp, env := s.sidecarRunSpec(over)
	if img != "sockerless-overlay/azf-redis:test" {
		t.Errorf("overlaid run image = %q, want overlay", img)
	}
	if startUp != nil {
		t.Errorf("overlaid startUp = %v, want nil (bootstrap runs baked argv)", startUp)
	}
	joined := strings.Join(env, " ")
	for _, want := range []string{"SOCKERLESS_SIDECAR=1", "SOCKERLESS_CONTAINER_ID=sc1", "SOCKERLESS_CALLBACK_URL="} {
		if !strings.Contains(joined, want) {
			t.Errorf("overlaid env missing %q: %v", want, env)
		}
	}
	// Raw sidecar (no overlay): runs the raw image with the original argv.
	raw := api.Container{ID: "sc2", Config: api.ContainerConfig{
		Image:  "redis:7-alpine",
		Labels: map[string]string{labelBaseImage: "redis:7-alpine", labelBaseEntrypoint: base64.StdEncoding.EncodeToString(ep)},
	}}
	img, startUp, _ = s.sidecarRunSpec(raw)
	if img != "redis:7-alpine" || !reflect.DeepEqual(startUp, []string{"redis-server"}) {
		t.Errorf("raw run spec = %q %v, want redis:7-alpine [redis-server]", img, startUp)
	}
}

func TestHostAliasesForMembers(t *testing.T) {
	members := []api.Container{
		networkedContainer("a", "build", "net1", true),
		networkedContainer("b", "redis", "net1", false, "redis", "cache"),
	}
	got := core.HostAliasesForMembers(members, "net1")
	want := map[string]bool{"redis": true, "cache": true}
	for _, a := range got {
		delete(want, a)
	}
	if len(want) != 0 {
		t.Errorf("missing aliases %v in %v", want, got)
	}
}

func TestResolvePodMembersOrdersMainFirst(t *testing.T) {
	s := testServer()
	svc := networkedContainer("svc", "redis", "net1", false)
	run := networkedContainer("run", "build", "net1", true)
	s.PendingCreates.Put(svc.ID, svc)
	s.PendingCreates.Put(run.ID, run)
	pod := &core.PodContext{ContainerIDs: []string{"svc", "run"}}
	members := s.resolvePodMembers(pod)
	if len(members) != 2 || members[0].ID != "run" {
		t.Fatalf("main (OpenStdin) must be first: %+v", members)
	}
}
