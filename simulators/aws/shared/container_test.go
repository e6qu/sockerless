package simulator

import "testing"

func TestResolveLocalImage_AR(t *testing.T) {
	got := ResolveLocalImage("us-central1-docker.pkg.dev/proj/docker-hub/library/alpine:latest")
	if got != "alpine:latest" {
		t.Errorf("expected alpine:latest, got %q", got)
	}
}

func TestResolveLocalImage_ECR(t *testing.T) {
	got := ResolveLocalImage("123456789012.dkr.ecr.us-east-1.amazonaws.com/alpine:latest")
	if got != "alpine:latest" {
		t.Errorf("expected alpine:latest, got %q", got)
	}
}

func TestResolveLocalImage_ACR(t *testing.T) {
	got := ResolveLocalImage("myacr.azurecr.io/library/nginx:latest")
	if got != "nginx:latest" {
		t.Errorf("expected nginx:latest, got %q", got)
	}
}

func TestResolveLocalImage_Passthrough(t *testing.T) {
	got := ResolveLocalImage("alpine:latest")
	if got != "alpine:latest" {
		t.Errorf("expected alpine:latest, got %q", got)
	}
}

func TestResolveLocalImage_ECR_DockerHub(t *testing.T) {
	// ECR strips library/ first (no-op here), then docker-hub/.
	// Remaining "library/nginx:1.25" is returned as-is because library/ was already attempted.
	got := ResolveLocalImage("123456789012.dkr.ecr.us-east-1.amazonaws.com/docker-hub/library/nginx:1.25")
	if got != "library/nginx:1.25" {
		t.Errorf("expected library/nginx:1.25, got %q", got)
	}
}

func TestResolveLocalImage_ECR_LibraryOnly(t *testing.T) {
	got := ResolveLocalImage("123456789012.dkr.ecr.us-east-1.amazonaws.com/library/nginx:1.25")
	if got != "nginx:1.25" {
		t.Errorf("expected nginx:1.25, got %q", got)
	}
}
