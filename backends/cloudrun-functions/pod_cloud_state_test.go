package gcf

import (
	"testing"

	functionspb "cloud.google.com/go/functions/apiv2/functionspb"
	runpb "cloud.google.com/go/run/apiv2/runpb"
	core "github.com/sockerless/backend-core"
)

// TestServiceToPodMemberContainer_LabelsFromEnv pins that a pod member's Docker
// labels reconstruct from the authoritative SOCKERLESS_LABELS env var on the
// Cloud Run Service's main container, and that a malformed value surfaces.
func TestServiceToPodMemberContainer_LabelsFromEnv(t *testing.T) {
	v, _ := core.EncodeLabelsEnvValue(map[string]string{"role": "db"})
	svc := &runpb.Service{
		Name: "projects/p/locations/r/services/sockerless-svc-y",
		Template: &runpb.RevisionTemplate{
			Containers: []*runpb.Container{{
				Image: "redis:7",
				Env:   []*runpb.EnvVar{{Name: core.LabelsEnvVar, Values: &runpb.EnvVar_Value{Value: v}}},
			}},
		},
	}
	got, err := serviceToPodMemberContainer(svc, "member-1")
	if err != nil {
		t.Fatalf("serviceToPodMemberContainer: %v", err)
	}
	if got.Config.Labels["role"] != "db" {
		t.Fatalf("labels not reconstructed: %+v", got.Config.Labels)
	}

	svc.Template.Containers[0].Env[0].Values = &runpb.EnvVar_Value{Value: "!!!bad"}
	if _, err := serviceToPodMemberContainer(svc, "member-1"); err == nil {
		t.Fatal("expected error on malformed SOCKERLESS_LABELS")
	}
}

// TestPodMembersFromFunctionRoundtrip verifies that a pod manifest
// encoded into a Function's SOCKERLESS_POD_CONTAINERS env var decodes
// back into the same per-member specs cloud_state needs for `docker ps`.
func TestPodMembersFromFunctionRoundtrip(t *testing.T) {
	members := []core.PodMemberSpec{
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
	enc, err := core.EncodePodManifest(members)
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
	got, err := podMembersFromFunction(fn)
	if err != nil {
		t.Fatalf("podMembersFromFunction: %v", err)
	}
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
	// Absent manifest → (nil, nil): the function is simply not a pod.
	if got, err := podMembersFromFunction(&functionspb.Function{}); got != nil || err != nil {
		t.Errorf("expected (nil,nil) from empty function, got %+v err=%v", got, err)
	}
	fn := &functionspb.Function{
		ServiceConfig: &functionspb.ServiceConfig{
			EnvironmentVariables: map[string]string{
				"SOCKERLESS_POD_CONTAINERS": "not-base64",
			},
		},
	}
	// Present-but-undecodable manifest → surface the error, not silently
	// drop every pod member from `docker ps`.
	if got, err := podMembersFromFunction(fn); err == nil {
		t.Errorf("expected error from bad encoding, got members=%+v nil err", got)
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
	m := core.PodMemberJSON{
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
