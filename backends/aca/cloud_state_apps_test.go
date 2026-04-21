package aca

import (
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appcontainers/armappcontainers/v2"
	"github.com/rs/zerolog"
	core "github.com/sockerless/backend-core"
)

func newServerForAppState(t *testing.T) *Server {
	t.Helper()
	s := &Server{
		BaseServer: core.NewBaseServer(core.NewStore(), core.BackendDescriptor{
			ID: "t", Name: "test", InstanceID: "inst-1",
		}, zerolog.Nop()),
		ACA:          core.NewStateStore[ACAState](),
		NetworkState: core.NewStateStore[NetworkState](),
		config:       Config{SubscriptionID: "sub", ResourceGroup: "rg", Environment: "env"},
	}
	s.SetSelf(s)
	return s
}

// TestResolveAppACAState_CacheHit returns cached ACAState when AppName
// is populated — no cloud call.
func TestResolveAppACAState_CacheHit(t *testing.T) {
	s := newServerForAppState(t)
	s.ACA.Put("abc123", ACAState{AppName: "sockerless-app-abc"})
	got, ok := s.resolveAppACAState(s.ctx(), "abc123")
	if !ok || got.AppName == "" {
		t.Fatalf("expected cache hit, got ok=%v app=%q", ok, got.AppName)
	}
}

// TestResolveAppACAState_MissAndNoCloud returns (zero, false) when the
// ContainerApps client isn't wired.
func TestResolveAppACAState_MissAndNoCloud(t *testing.T) {
	s := newServerForAppState(t)
	got, ok := s.resolveAppACAState(s.ctx(), "nonexistent")
	if ok {
		t.Fatalf("expected miss + no cloud = false, got ok=%v app=%q", ok, got.AppName)
	}
}

// TestAppContainerState_Running — Succeeded + LatestReadyRevisionName
// means the App is serving.
func TestAppContainerState_Running(t *testing.T) {
	ps := armappcontainers.ContainerAppProvisioningStateSucceeded
	rev := "myapp--rev1"
	app := &armappcontainers.ContainerApp{
		Properties: &armappcontainers.ContainerAppProperties{
			ProvisioningState:       &ps,
			LatestReadyRevisionName: &rev,
		},
	}
	st := appContainerState(app)
	if st.Status != "running" || !st.Running {
		t.Fatalf("expected running, got %+v", st)
	}
}

// TestAppContainerState_Failed — Failed state maps to "exited" code 1.
func TestAppContainerState_Failed(t *testing.T) {
	ps := armappcontainers.ContainerAppProvisioningStateFailed
	app := &armappcontainers.ContainerApp{
		Properties: &armappcontainers.ContainerAppProperties{
			ProvisioningState: &ps,
		},
	}
	st := appContainerState(app)
	if st.Status != "exited" || st.ExitCode != 1 {
		t.Fatalf("expected exited/1, got %+v", st)
	}
}

// TestAppContainerState_InProgress — InProgress provisioning = "created".
func TestAppContainerState_InProgress(t *testing.T) {
	ps := armappcontainers.ContainerAppProvisioningStateInProgress
	app := &armappcontainers.ContainerApp{
		Properties: &armappcontainers.ContainerAppProperties{
			ProvisioningState: &ps,
		},
	}
	st := appContainerState(app)
	if st.Status != "created" {
		t.Fatalf("expected created, got %+v", st)
	}
}

// TestAppToContainer_Shape — key fields round-trip from ContainerApp
// into api.Container.
func TestAppToContainer_Shape(t *testing.T) {
	p := &acaCloudState{server: newServerForAppState(t)}
	ps := armappcontainers.ContainerAppProvisioningStateSucceeded
	rev := "app1--rev1"
	main := "main"
	img := "myreg.azurecr.io/app:v1"
	tags := map[string]*string{
		"sockerless-managed":      ptr("true"),
		"sockerless-container-id": ptr("abcdef012345"),
		"sockerless-name":         ptr("/webapp"),
		"sockerless-network":      ptr("mynet"),
		"sockerless-backend":      ptr("aca"),
	}
	app := &armappcontainers.ContainerApp{
		Tags: tags,
		Properties: &armappcontainers.ContainerAppProperties{
			ProvisioningState:       &ps,
			LatestReadyRevisionName: &rev,
			Template: &armappcontainers.Template{
				Containers: []*armappcontainers.Container{{
					Name:    &main,
					Image:   &img,
					Command: []*string{ptr("/bin/app")},
					Args:    []*string{ptr("--serve")},
				}},
			},
		},
	}
	mapped := azureTagsToMap(app.Tags)
	got := p.appToContainer(app, mapped)
	if got.ID != "abcdef012345" {
		t.Errorf("ID = %q", got.ID)
	}
	if got.Name != "/webapp" {
		t.Errorf("Name = %q", got.Name)
	}
	if got.Image != img {
		t.Errorf("Image = %q", got.Image)
	}
	if got.State.Status != "running" || !got.State.Running {
		t.Errorf("State = %+v", got.State)
	}
	if got.HostConfig.NetworkMode != "mynet" {
		t.Errorf("NetworkMode = %q", got.HostConfig.NetworkMode)
	}
	if got.Driver != "aca-apps" {
		t.Errorf("Driver = %q, want aca-apps", got.Driver)
	}
}
