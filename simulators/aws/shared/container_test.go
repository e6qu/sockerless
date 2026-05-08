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

// parsePlatform — workload arch is carried in the spec, never derived
// from the host. Empty input → nil (Docker uses image default).
func TestParsePlatform(t *testing.T) {
	cases := []struct {
		in   string
		want string // "" if expecting nil
	}{
		{"", ""},
		{"linux/arm64", "linux/arm64"},
		{"linux/amd64", "linux/amd64"},
		{"linux/arm/v7", "linux/arm/v7"},
		{"garbage", ""}, // unknown shape → nil; caller is expected to vet
	}
	for _, tc := range cases {
		got := parsePlatform(tc.in)
		if tc.want == "" {
			if got != nil {
				t.Errorf("parsePlatform(%q) = %+v, want nil", tc.in, got)
			}
			continue
		}
		if got == nil {
			t.Errorf("parsePlatform(%q) = nil, want %s", tc.in, tc.want)
			continue
		}
		flat := got.OS + "/" + got.Architecture
		if got.Variant != "" {
			flat += "/" + got.Variant
		}
		if flat != tc.want {
			t.Errorf("parsePlatform(%q) = %s, want %s", tc.in, flat, tc.want)
		}
	}
}
