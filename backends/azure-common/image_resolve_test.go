package azurecommon

import "testing"

func TestResolveAzureImageURI_AlreadyACR(t *testing.T) {
	ref := "myacr.azurecr.io/myapp:v1"
	got := ResolveAzureImageURI(ref, "myacr")
	if got != ref {
		t.Errorf("ACR URI should pass through, got %q", got)
	}
}

func TestResolveAzureImageURI_WithACR(t *testing.T) {
	got := ResolveAzureImageURI("alpine:latest", "myacr")
	want := "myacr.azurecr.io/library/alpine:latest"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveAzureImageURI_WithACRAndOrg(t *testing.T) {
	got := ResolveAzureImageURI("myorg/myapp:v2", "myacr")
	want := "myacr.azurecr.io/myorg/myapp:v2"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveAzureImageURI_NoACR(t *testing.T) {
	got := ResolveAzureImageURI("alpine:latest", "")
	// Should normalize to docker.io/library/alpine:latest or similar
	if got == "" {
		t.Error("should not return empty")
	}
}

func TestResolveAzureImageURI_NoTag(t *testing.T) {
	got := ResolveAzureImageURI("nginx", "myacr")
	if got == "" {
		t.Error("should not return empty")
	}
}
