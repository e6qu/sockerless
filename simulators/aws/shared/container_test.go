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
	// ECR pull-through cache hit for a Docker Hub library image:
	// `docker-hub/` and `library/` both get stripped so the resolved
	// ref matches the plain Docker Hub name the local daemon can pull.
	got := ResolveLocalImage("123456789012.dkr.ecr.us-east-1.amazonaws.com/docker-hub/library/nginx:1.25")
	if got != "nginx:1.25" {
		t.Errorf("expected nginx:1.25, got %q", got)
	}
}

func TestResolveLocalImage_ECR_DockerHubNonLibrary(t *testing.T) {
	// Non-library docker-hub image: strip docker-hub/ but leave the
	// user/repo path intact so e.g. `user/myimg:tag` round-trips.
	got := ResolveLocalImage("123456789012.dkr.ecr.us-east-1.amazonaws.com/docker-hub/user/myimg:tag")
	if got != "user/myimg:tag" {
		t.Errorf("expected user/myimg:tag, got %q", got)
	}
}

func TestResolveLocalImage_ECR_LibraryOnly(t *testing.T) {
	got := ResolveLocalImage("123456789012.dkr.ecr.us-east-1.amazonaws.com/library/nginx:1.25")
	if got != "nginx:1.25" {
		t.Errorf("expected nginx:1.25, got %q", got)
	}
}
