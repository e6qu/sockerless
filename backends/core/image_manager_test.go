package core

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/sockerless/api"
)

// mockBuildService is a test CloudBuildService that records calls.
type mockBuildService struct {
	available bool
	called    bool
	opts      CloudBuildOptions
	err       error
}

func (m *mockBuildService) Available() bool { return m.available }

func (m *mockBuildService) AssembleMultiArchManifest(ctx context.Context, opts MultiArchManifestOptions) error {
	return nil
}

func (m *mockBuildService) Build(ctx context.Context, opts CloudBuildOptions) (*CloudBuildResult, error) {
	m.called = true
	m.opts = opts
	if m.err != nil {
		return nil, m.err
	}
	return &CloudBuildResult{
		ImageRef: "registry/repo:tag",
		ImageID:  "sha256:abc123def456",
		Duration: 5 * time.Second,
	}, nil
}

func TestImageManagerBuild_DelegatesToCloudBuild(t *testing.T) {
	s := newPodTestServer()
	mock := &mockBuildService{available: true}

	mgr := &ImageManager{
		Base:         s,
		BuildService: mock,
		Logger:       s.Logger,
	}

	// Build with a tar body
	body := strings.NewReader("")
	rc, err := mgr.Build(buildOpts("myapp:latest", "Dockerfile"), body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer rc.Close()
	io.Copy(io.Discard, rc)

	if !mock.called {
		t.Error("CloudBuildService.Build should have been called")
	}
	if mock.opts.Dockerfile != "Dockerfile" {
		t.Errorf("expected Dockerfile=%q, got %q", "Dockerfile", mock.opts.Dockerfile)
	}
}

func TestImageManagerBuild_FallsBackWhenUnavailable(t *testing.T) {
	s := newPodTestServer()
	mock := &mockBuildService{available: false}

	mgr := &ImageManager{
		Base:         s,
		BuildService: mock,
		Logger:       s.Logger,
	}

	body := strings.NewReader("")
	_, _ = mgr.Build(buildOpts("myapp:latest", "Dockerfile"), body)

	if mock.called {
		t.Error("CloudBuildService.Build should NOT be called when unavailable")
	}
}

func TestImageManagerBuild_FallsBackWhenNil(t *testing.T) {
	s := newPodTestServer()

	mgr := &ImageManager{
		Base:   s,
		Logger: s.Logger,
		// BuildService is nil
	}

	body := strings.NewReader("")
	_, err := mgr.Build(buildOpts("myapp:latest", "Dockerfile"), body)
	// Should not panic, falls back to BaseServer.ImageBuild
	_ = err
}

func buildOpts(tag, dockerfile string) api.ImageBuildOptions {
	return api.ImageBuildOptions{
		Tags:       []string{tag},
		Dockerfile: dockerfile,
	}
}
