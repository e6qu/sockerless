package cloudrun

import (
	"testing"

	runpb "cloud.google.com/go/run/apiv2/runpb"
	"github.com/rs/zerolog"
	core "github.com/sockerless/backend-core"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func newServerForState(t *testing.T) *Server {
	t.Helper()
	s := &Server{
		BaseServer: core.NewBaseServer(core.NewStore(), core.BackendDescriptor{
			ID: "t", Name: "test", InstanceID: "inst-1",
		}, zerolog.Nop()),
		CloudRun:     core.NewStateStore[CloudRunState](),
		NetworkState: core.NewStateStore[NetworkState](),
		config:       Config{Project: "proj", Region: "us-central1"},
	}
	s.SetSelf(s)
	return s
}

// TestResolveServiceCloudRunState_CacheHit returns the cached state
// when ServiceName is populated — no cloud call.
func TestResolveServiceCloudRunState_CacheHit(t *testing.T) {
	s := newServerForState(t)
	s.CloudRun.Put("abc123", CloudRunState{
		ServiceName: "projects/p/locations/us-central1/services/sockerless-svc-abc",
	})
	got, ok := s.resolveServiceCloudRunState(s.ctx(), "abc123")
	if !ok || got.ServiceName == "" {
		t.Fatalf("expected cache hit, got ok=%v svc=%q", ok, got.ServiceName)
	}
}

// TestResolveServiceCloudRunState_MissAndNoCloud returns (zero, false)
// when the Services client isn't wired. Mirrors the Jobs resolver so
// lifecycle callers can treat "no result" uniformly.
func TestResolveServiceCloudRunState_MissAndNoCloud(t *testing.T) {
	s := newServerForState(t)
	got, ok := s.resolveServiceCloudRunState(s.ctx(), "nonexistent")
	if ok {
		t.Fatalf("expected miss + no cloud = false, got ok=%v svc=%q", ok, got.ServiceName)
	}
}

// TestServiceContainerState_Running — TerminalCondition Ready +
// SUCCEEDED + LatestReadyRevision non-empty maps to running.
func TestServiceContainerState_Running(t *testing.T) {
	svc := &runpb.Service{
		CreateTime: timestamppb.Now(),
		TerminalCondition: &runpb.Condition{
			Type:  "Ready",
			State: runpb.Condition_CONDITION_SUCCEEDED,
		},
		LatestReadyRevision: "projects/p/locations/us/services/s/revisions/r1",
	}
	st := serviceContainerState(svc)
	if st.Status != "running" || !st.Running {
		t.Fatalf("expected running, got %+v", st)
	}
	if st.StartedAt == "" {
		t.Error("expected StartedAt to be populated from CreateTime")
	}
}

// TestServiceContainerState_Failed — FAILED state maps to "exited"
// with exit code 1 and propagates the condition message.
func TestServiceContainerState_Failed(t *testing.T) {
	svc := &runpb.Service{
		TerminalCondition: &runpb.Condition{
			Type:    "Ready",
			State:   runpb.Condition_CONDITION_FAILED,
			Message: "revision deploy failed",
		},
	}
	st := serviceContainerState(svc)
	if st.Status != "exited" || st.ExitCode != 1 {
		t.Fatalf("expected exited/1, got %+v", st)
	}
	if st.Error != "revision deploy failed" {
		t.Errorf("expected Error to propagate condition message, got %q", st.Error)
	}
}

// TestServiceContainerState_Pending — no TerminalCondition yet means
// the Service is still reconciling; container state is "created".
func TestServiceContainerState_Pending(t *testing.T) {
	svc := &runpb.Service{
		TerminalCondition: &runpb.Condition{
			Type:  "Ready",
			State: runpb.Condition_CONDITION_PENDING,
		},
	}
	st := serviceContainerState(svc)
	if st.Status != "created" {
		t.Fatalf("expected created, got %+v", st)
	}
}

// TestServiceToContainer_Shape — all key Service fields round-trip
// into an api.Container equivalent.
func TestServiceToContainer_Shape(t *testing.T) {
	p := &cloudRunCloudState{server: newServerForState(t)}
	svc := &runpb.Service{
		Name:       "projects/proj/locations/us-central1/services/sockerless-svc-abc",
		CreateTime: timestamppb.Now(),
		Labels: map[string]string{
			"sockerless_managed":      "true",
			"sockerless_container_id": "abcdef0123456789abcdef",
			"sockerless_backend":      "cloudrun",
			"sockerless_name":         "/webapp",
			"sockerless_network":      "mynet",
		},
		Annotations: map[string]string{
			// Full container ID lives here when it exceeds 63 chars in
			// real workloads; the 22-char test fixture stays in labels.
		},
		Template: &runpb.RevisionTemplate{
			Containers: []*runpb.Container{{
				Image:   "us-central1-docker.pkg.dev/p/r/app:v1",
				Command: []string{"/bin/app"},
				Args:    []string{"--serve"},
				Env: []*runpb.EnvVar{
					{Name: "FOO", Values: &runpb.EnvVar_Value{Value: "bar"}},
				},
			}},
		},
		TerminalCondition: &runpb.Condition{
			Type:  "Ready",
			State: runpb.Condition_CONDITION_SUCCEEDED,
		},
		LatestReadyRevision: "projects/p/locations/us-central1/services/sockerless-svc-abc/revisions/r1",
	}
	got, err := p.serviceToContainer(svc)
	if err != nil {
		t.Fatalf("serviceToContainer: %v", err)
	}
	if got.ID != "abcdef0123456789abcdef" {
		t.Errorf("ID = %q, want abcdef0123456789abcdef", got.ID)
	}
	if got.Name != "/webapp" {
		t.Errorf("Name = %q, want /webapp", got.Name)
	}
	if got.Image != "us-central1-docker.pkg.dev/p/r/app:v1" {
		t.Errorf("Image = %q", got.Image)
	}
	if got.State.Status != "running" || !got.State.Running {
		t.Errorf("State = %+v, want running", got.State)
	}
	if got.HostConfig.NetworkMode != "mynet" {
		t.Errorf("NetworkMode = %q, want mynet", got.HostConfig.NetworkMode)
	}
	if len(got.Config.Env) != 1 || got.Config.Env[0] != "FOO=bar" {
		t.Errorf("Env = %v, want [FOO=bar]", got.Config.Env)
	}
}
