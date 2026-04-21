package aca

import (
	"strings"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appcontainers/armappcontainers/v2"
	"github.com/rs/zerolog"
	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
)

func newServerForAppSpec(t *testing.T) *Server {
	t.Helper()
	s := &Server{
		BaseServer: core.NewBaseServer(core.NewStore(), core.BackendDescriptor{
			ID: "t", Name: "test", InstanceID: "inst-1",
		}, zerolog.Nop()),
		ACA:          core.NewStateStore[ACAState](),
		NetworkState: core.NewStateStore[NetworkState](),
		config: Config{
			SubscriptionID: "sub",
			ResourceGroup:  "rg",
			Environment:    "env",
			Location:       "eastus",
		},
	}
	s.SetSelf(s)
	return s
}

func demoAppContainer(id, name, image string) containerInput {
	return containerInput{
		ID:     id,
		IsMain: true,
		Container: &api.Container{
			ID:   id,
			Name: name,
			Config: api.ContainerConfig{
				Image:      image,
				Env:        []string{"FOO=bar"},
				Entrypoint: []string{"/bin/app"},
				Cmd:        []string{"--serve"},
			},
			HostConfig: api.HostConfig{NetworkMode: "mynet"},
		},
	}
}

// TestBuildAppName_PrefixAndLength — App names use the sockerless-app
// prefix and 12-char ID suffix, distinct from buildJobName.
func TestBuildAppName_PrefixAndLength(t *testing.T) {
	got := buildAppName("abcdef0123456789abcdef")
	want := "sockerless-app-abcdef012345"
	if got != want {
		t.Fatalf("buildAppName = %q, want %q", got, want)
	}
	if buildJobName("abcdef0123456789abcdef") == got {
		t.Fatal("buildAppName must not equal buildJobName — collisions in same resource group")
	}
}

// TestBuildAppSpec_Shape — ContainerApp has internal-only ingress,
// single active revision, min=max=1, and one container.
func TestBuildAppSpec_Shape(t *testing.T) {
	s := newServerForAppSpec(t)
	ci := demoAppContainer("abcdef012345abcdef", "/webapp", "myreg.azurecr.io/app:v1")
	app := s.buildAppSpec([]containerInput{ci})

	if app.Location == nil || *app.Location != "eastus" {
		t.Errorf("Location = %v, want eastus", app.Location)
	}
	if app.Properties == nil || app.Properties.EnvironmentID == nil ||
		!strings.Contains(*app.Properties.EnvironmentID, "managedEnvironments/env") {
		t.Errorf("EnvironmentID not wired: %+v", app.Properties)
	}
	cfg := app.Properties.Configuration
	if cfg == nil || cfg.Ingress == nil {
		t.Fatalf("expected Configuration.Ingress, got %+v", cfg)
	}
	if cfg.Ingress.External == nil || *cfg.Ingress.External {
		t.Errorf("Ingress.External = %v, want false (internal-only)", cfg.Ingress.External)
	}
	if cfg.ActiveRevisionsMode == nil || *cfg.ActiveRevisionsMode != armappcontainers.ActiveRevisionsModeSingle {
		t.Errorf("ActiveRevisionsMode = %v, want Single", cfg.ActiveRevisionsMode)
	}
	tpl := app.Properties.Template
	if tpl == nil || len(tpl.Containers) != 1 {
		t.Fatalf("expected 1 container in template, got %+v", tpl)
	}
	if tpl.Containers[0].Image == nil || *tpl.Containers[0].Image != "myreg.azurecr.io/app:v1" {
		t.Errorf("container image = %v", tpl.Containers[0].Image)
	}
	if tpl.Scale == nil || tpl.Scale.MinReplicas == nil || tpl.Scale.MaxReplicas == nil ||
		*tpl.Scale.MinReplicas != 1 || *tpl.Scale.MaxReplicas != 1 {
		t.Errorf("Scale min/max = %+v, want both 1", tpl.Scale)
	}
	// Tag plumbing: ContainerID lives in Azure tags.
	if v, ok := app.Tags["sockerless-container-id"]; !ok || v == nil || *v == "" {
		t.Errorf("sockerless-container-id tag missing or empty: %+v", app.Tags)
	}
}
