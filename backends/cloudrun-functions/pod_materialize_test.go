package gcf

import (
	"strings"
	"testing"

	"github.com/sockerless/api"
)

func TestContainersToPodOverlaySpec(t *testing.T) {
	containers := []api.Container{
		{
			ID:   "111111111111aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			Name: "/postgres",
			Config: api.ContainerConfig{
				Image: "us-central1-docker.pkg.dev/p/r/postgres:16",
				Cmd:   []string{"postgres"},
				Env:   []string{"POSTGRES_PASSWORD=x"},
			},
		},
		{
			ID:   "222222222222bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			Name: "/main-step",
			Config: api.ContainerConfig{
				Image:      "us-central1-docker.pkg.dev/p/r/alpine:latest",
				Entrypoint: []string{"sh", "-c"},
				Cmd:        []string{"echo hello && pg_isready -h localhost"},
				WorkingDir: "/work",
			},
		},
	}
	mainID := containers[1].ID
	spec := containersToPodOverlaySpec("/tmp/bootstrap", "ci-pod", mainID, containers)

	if spec.PodName != "ci-pod" {
		t.Errorf("PodName: %q", spec.PodName)
	}
	if spec.MainName != "main-step" {
		t.Errorf("MainName: %q want main-step", spec.MainName)
	}
	if spec.BootstrapBinaryPath != "/tmp/bootstrap" {
		t.Errorf("BootstrapBinaryPath: %q", spec.BootstrapBinaryPath)
	}
	if len(spec.Members) != 2 {
		t.Fatalf("Members len: %d", len(spec.Members))
	}
	if spec.Members[0].Name != "postgres" {
		t.Errorf("members[0].Name: %q", spec.Members[0].Name)
	}
	if spec.Members[1].Name != "main-step" {
		t.Errorf("members[1].Name: %q", spec.Members[1].Name)
	}
	if spec.Members[1].Workdir != "/work" {
		t.Errorf("members[1].Workdir: %q", spec.Members[1].Workdir)
	}
}

func TestContainersToPodOverlaySpecUnnamed(t *testing.T) {
	c := api.Container{
		ID:     "abcdef012345xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
		Config: api.ContainerConfig{Image: "alpine", Cmd: []string{"true"}},
	}
	spec := containersToPodOverlaySpec("/x", "p", c.ID, []api.Container{c})
	if spec.Members[0].Name != "abcdef012345" {
		t.Errorf("expected fallback to short ID, got %q", spec.Members[0].Name)
	}
}

func TestSanitizePodMemberName(t *testing.T) {
	cases := map[string]string{
		"":            "x",
		"OK-Name":     "ok-name",
		"main_step":   "main-step",
		"my.svc":      "my-svc",
		"weird!@#$":   "weird",
		"!!!@@@":      "x",
		"abc.123_xyz": "abc-123-xyz",
	}
	for in, want := range cases {
		if got := sanitizePodMemberName(in); got != want {
			t.Errorf("sanitizePodMemberName(%q) = %q want %q", in, got, want)
		}
	}
}

func TestSanitizePodLabelValue(t *testing.T) {
	cases := map[string]string{
		"OK-pod_1": "ok-pod_1",
		"my-pod":   "my-pod",
		"weird!":   "weird",
		"":         "",
		"UPPER":    "upper",
	}
	for in, want := range cases {
		if got := sanitizePodLabelValue(in); got != want {
			t.Errorf("sanitizePodLabelValue(%q) = %q want %q", in, got, want)
		}
	}
}

func TestContainersToPodOverlaySpecMainAtZero(t *testing.T) {
	containers := []api.Container{
		{
			ID:     "111111111111aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			Name:   "/main",
			Config: api.ContainerConfig{Image: "alpine", Cmd: []string{"echo", "hi"}},
		},
		{
			ID:     "222222222222bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			Name:   "/sidecar",
			Config: api.ContainerConfig{Image: "redis:7", Cmd: []string{"redis-server"}},
		},
	}
	spec := containersToPodOverlaySpec("/x", "p", containers[0].ID, containers)
	if spec.MainName != "main" {
		t.Errorf("expected main at index 0 to be picked, got %q", spec.MainName)
	}
	// Member ordering is preserved — important for content tag stability
	// across StartedIDs orderings on the same pod manifest.
	if !strings.HasPrefix(spec.Members[0].Name, "main") {
		t.Errorf("members[0].Name: %q", spec.Members[0].Name)
	}
}
