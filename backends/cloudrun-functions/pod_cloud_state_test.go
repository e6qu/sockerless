package gcf

import (
	"testing"

	functionspb "cloud.google.com/go/functions/apiv2/functionspb"
)

// TestPodMembersFromFunctionRoundtrip verifies that a pod manifest
// encoded into a Function's SOCKERLESS_POD_CONTAINERS env var decodes
// back into the same per-member specs cloud_state needs for `docker ps`.
func TestPodMembersFromFunctionRoundtrip(t *testing.T) {
	members := []PodMemberSpec{
		{
			Name:         "postgres",
			ContainerID:  "11111111111111111111111111111111aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			BaseImageRef: "postgres:16",
			Cmd:          []string{"postgres"},
			Env:          []string{"POSTGRES_PASSWORD=x"},
		},
		{
			Name:         "main",
			ContainerID:  "22222222222222222222222222222222bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			BaseImageRef: "alpine:latest",
			Entrypoint:   []string{"sh", "-c"},
			Cmd:          []string{"echo hi"},
		},
	}
	enc, err := EncodePodManifest(members)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	fn := &functionspb.Function{
		ServiceConfig: &functionspb.ServiceConfig{
			EnvironmentVariables: map[string]string{
				"SOCKERLESS_POD_CONTAINERS": enc,
			},
		},
	}
	got := podMembersFromFunction(fn)
	if len(got) != 2 {
		t.Fatalf("len: got %d want 2", len(got))
	}
	if got[0].Name != "postgres" || got[0].ContainerID != members[0].ContainerID {
		t.Errorf("members[0]: %+v", got[0])
	}
	if got[1].Image != "alpine:latest" {
		t.Errorf("expected image to round-trip, got %q", got[1].Image)
	}
	if got[0].Root != "/containers/postgres" {
		t.Errorf("root: %q", got[0].Root)
	}
}

func TestPodMembersFromFunctionEmpty(t *testing.T) {
	if got := podMembersFromFunction(&functionspb.Function{}); got != nil {
		t.Errorf("expected nil from empty function, got %+v", got)
	}
	fn := &functionspb.Function{
		ServiceConfig: &functionspb.ServiceConfig{
			EnvironmentVariables: map[string]string{
				"SOCKERLESS_POD_CONTAINERS": "not-base64",
			},
		},
	}
	if got := podMembersFromFunction(fn); got != nil {
		t.Errorf("expected nil from bad encoding, got %+v", got)
	}
}

func TestPodMemberToContainerSurfacesDegradation(t *testing.T) {
	fn := &functionspb.Function{
		State: functionspb.Function_ACTIVE,
	}
	labels := map[string]string{
		"sockerless_pod":        "ci-pod",
		"sockerless_created_at": "2026-05-02T10:00:00Z",
	}
	m := PodMemberJSON{
		Name:        "main",
		ContainerID: "11111111111111111111111111111111aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Image:       "alpine:latest",
		Cmd:         []string{"echo", "hi"},
	}
	c := podMemberToContainer(fn, labels, m)
	if c.ID != m.ContainerID {
		t.Errorf("ID: %q", c.ID)
	}
	if c.Name != "/main" {
		t.Errorf("Name: %q", c.Name)
	}
	if c.Image != "alpine:latest" {
		t.Errorf("Image: %q", c.Image)
	}
	if c.Config.Labels["sockerless.pod"] != "ci-pod" {
		t.Errorf("missing pod label: %v", c.Config.Labels)
	}
	if c.Config.Labels["sockerless.namespace.mount"] != "shared-degraded" {
		t.Errorf("expected mount-ns degradation label, got %v", c.Config.Labels)
	}
	if c.HostConfig.PidMode != "shared-degraded" {
		t.Errorf("expected PidMode shared-degraded, got %q", c.HostConfig.PidMode)
	}
}
