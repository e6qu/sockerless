package cloudrun

import (
	"strings"
	"testing"
	"time"

	runpb "cloud.google.com/go/run/apiv2/runpb"
	"github.com/rs/zerolog"
	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
)

func newServerForSpec(t *testing.T, vpcConnector string) *Server {
	t.Helper()
	s := &Server{
		BaseServer: core.NewBaseServer(core.NewStore(), core.BackendDescriptor{
			ID: "t", Name: "test", InstanceID: "inst-1",
		}, zerolog.Nop()),
		CloudRun:     core.NewStateStore[CloudRunState](),
		NetworkState: core.NewStateStore[NetworkState](),
		config: Config{
			Project:      "proj",
			Region:       "us-central1",
			VPCConnector: vpcConnector,
		},
	}
	s.SetSelf(s)
	return s
}

func demoContainer(id, name, image string) containerInput {
	return containerInput{
		ID:     id,
		IsMain: true,
		Container: &api.Container{
			ID:   id,
			Name: name,
			Config: api.ContainerConfig{
				Image:      image,
				Env:        []string{"FOO=bar", "EMPTY="},
				Entrypoint: []string{"/bin/app"},
				Cmd:        []string{"--serve"},
				WorkingDir: "/srv",
			},
			HostConfig: api.HostConfig{NetworkMode: "net-xyz"},
		},
	}
}

// TestBuildServiceName_PrefixAndLength — naming uses the sockerless-svc
// prefix and first 12 chars of the ID. Distinct prefix from buildJobName
// so Jobs + Services never collide.
func TestBuildServiceName_PrefixAndLength(t *testing.T) {
	got := buildServiceName("abcdef0123456789abcdef")
	want := "sockerless-svc-abcdef012345"
	if got != want {
		t.Fatalf("buildServiceName = %q, want %q", got, want)
	}
	if buildJobName("abcdef0123456789abcdef") == got {
		t.Fatal("buildServiceName must not equal buildJobName — they'd collide in the same project")
	}
}

// TestBuildServiceParent — parent path matches projects/{p}/locations/{l}.
func TestBuildServiceParent(t *testing.T) {
	s := newServerForSpec(t, "")
	if got, want := s.buildServiceParent(), "projects/proj/locations/us-central1"; got != want {
		t.Fatalf("buildServiceParent = %q, want %q", got, want)
	}
}

// TestBuildServiceSpec_Shape — Service has internal-only ingress,
// default URI disabled, MinInstanceCount=MaxInstanceCount=1 (long-running),
// and labels carry the sockerless tag set.
func TestBuildServiceSpec_Shape(t *testing.T) {
	s := newServerForSpec(t, "projects/p/locations/us-central1/connectors/c1")
	ci := demoContainer("abc123456789fffffffff", "/webapp", "gcr.io/proj/app:v1")
	svc := s.buildServiceSpec([]containerInput{ci})

	if svc.Ingress != runpb.IngressTraffic_INGRESS_TRAFFIC_INTERNAL_ONLY {
		t.Errorf("ingress = %v, want INGRESS_TRAFFIC_INTERNAL_ONLY", svc.Ingress)
	}
	if !svc.DefaultUriDisabled {
		t.Error("DefaultUriDisabled should be true (Services are peer-reachable only)")
	}
	if svc.Template == nil || len(svc.Template.Containers) != 1 {
		t.Fatalf("expected 1 container in template, got %+v", svc.Template)
	}
	if svc.Template.Containers[0].Image != "gcr.io/proj/app:v1" {
		t.Errorf("container image = %q", svc.Template.Containers[0].Image)
	}
	if svc.Template.Scaling == nil || svc.Template.Scaling.MinInstanceCount != 1 || svc.Template.Scaling.MaxInstanceCount != 1 {
		t.Errorf("scaling = %+v, want min=1 max=1", svc.Template.Scaling)
	}
	if svc.Template.VpcAccess == nil || svc.Template.VpcAccess.Connector != s.config.VPCConnector {
		t.Errorf("vpc access = %+v, want connector=%q", svc.Template.VpcAccess, s.config.VPCConnector)
	}
	if svc.Template.VpcAccess.Egress != runpb.VpcAccess_ALL_TRAFFIC {
		t.Errorf("vpc egress = %v, want ALL_TRAFFIC", svc.Template.VpcAccess.Egress)
	}
	if svc.Template.Timeout == nil || svc.Template.Timeout.AsDuration() != time.Hour {
		t.Errorf("timeout = %v, want 1h", svc.Template.Timeout)
	}
	// Labels carry the sockerless identity tags (keys use underscores per
	// AsGCPLabels, values are truncated at 63 chars).
	gotID := svc.Labels["sockerless_container_id"]
	if gotID == "" || !strings.HasPrefix(ci.ID, gotID) {
		t.Errorf("labels sockerless_container_id = %q, want prefix of %q", gotID, ci.ID)
	}
	if svc.Labels["sockerless_backend"] != "cloudrun" {
		t.Errorf("labels sockerless_backend = %q, want cloudrun", svc.Labels["sockerless_backend"])
	}
}

// TestBuildServiceSpec_NoVPCConnector — when VPCConnector is empty,
// VpcAccess is nil. Callers that need the internal DNS path will refuse
// to run in this config; the builder itself stays permissive.
func TestBuildServiceSpec_NoVPCConnector(t *testing.T) {
	s := newServerForSpec(t, "")
	ci := demoContainer("idididididid0000000000", "/c", "img:1")
	svc := s.buildServiceSpec([]containerInput{ci})
	if svc.Template.VpcAccess != nil {
		t.Fatalf("expected nil VpcAccess when connector unset, got %+v", svc.Template.VpcAccess)
	}
}
