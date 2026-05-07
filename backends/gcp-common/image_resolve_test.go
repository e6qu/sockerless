package gcpcommon

import "testing"

func TestResolveGCPImageURI_AlreadyAR(t *testing.T) {
	ref := "us-docker.pkg.dev/myproject/myrepo/myimage:v1"
	got := ResolveGCPImageURI(ref, "myproject", "us-central1", "")
	if got != ref {
		t.Errorf("AR URI should pass through, got %q", got)
	}
}

func TestResolveGCPImageURI_AlreadyGCR(t *testing.T) {
	ref := "gcr.io/myproject/myimage:v1"
	got := ResolveGCPImageURI(ref, "myproject", "us-central1", "")
	if got != ref {
		t.Errorf("GCR URI should pass through, got %q", got)
	}
}

func TestResolveGCPImageURI_DockerHub(t *testing.T) {
	got := ResolveGCPImageURI("alpine:latest", "myproject", "us-central1", "")
	want := "us-central1-docker.pkg.dev/myproject/docker-hub/library/alpine:latest"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveGCPImageURI_DockerHubWithOrg(t *testing.T) {
	got := ResolveGCPImageURI("myorg/myapp:v2", "proj", "europe-west1", "")
	want := "europe-west1-docker.pkg.dev/proj/docker-hub/myorg/myapp:v2"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveGCPImageURI_NoTag(t *testing.T) {
	got := ResolveGCPImageURI("nginx", "proj", "us", "")
	if got == "" {
		t.Error("should not return empty")
	}
	// Should have :latest appended
	if got[len(got)-7:] != ":latest" {
		t.Errorf("should append :latest, got %q", got)
	}
}

func TestResolveGCPImageURI_SimulatorPassthrough(t *testing.T) {
	// When endpointURL is set the backend is talking to the simulator,
	// which has no AR remote-proxy provisioned — leave the ref alone.
	got := ResolveGCPImageURI("alpine:latest", "sim-project", "us-central1", "http://127.0.0.1:4567")
	if got != "alpine:latest" {
		t.Errorf("simulator mode should pass ref through, got %q", got)
	}
}
