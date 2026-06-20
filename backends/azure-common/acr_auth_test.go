package azurecommon

import (
	"testing"

	"github.com/rs/zerolog"
)

func TestACRAuthProvider_IsCloudRegistry_AzureCRSuffix(t *testing.T) {
	a := NewACRAuthProvider(zerolog.Nop())
	if !a.IsCloudRegistry("myacr.azurecr.io") {
		t.Fatalf("expected *.azurecr.io to be recognized as a cloud registry")
	}
	if a.IsCloudRegistry("docker.io") {
		t.Fatalf("docker.io must not be recognized as an ACR cloud registry")
	}
}

func TestACRAuthProvider_RegistryEndpoint_Coordinate(t *testing.T) {
	// A relocated registry coordinate (e.g. the simulator's /v2/ address) is
	// passed explicitly; OCI HTTP must route to it instead of the published
	// host, and IsCloudRegistry must recognize the coordinate host.
	const ep = "http://127.0.0.1:5000"
	a := NewACRAuthProvider(zerolog.Nop(), ep)

	if !a.IsCloudRegistry("127.0.0.1:5000") {
		t.Fatalf("coordinate host must be recognized as a cloud registry")
	}
	if got := a.RegistryEndpoint("127.0.0.1:5000"); got != ep {
		t.Fatalf("RegistryEndpoint(coordinate) = %q, want %q", got, ep)
	}
	// A real ACR ref still routes through the override when set (the backend
	// reaches the same relocated endpoint for both base and coordinate refs).
	if got := a.RegistryEndpoint("myacr.azurecr.io"); got != ep {
		t.Fatalf("RegistryEndpoint(azurecr) = %q, want %q", got, ep)
	}
	// A non-cloud registry gets no endpoint override.
	if got := a.RegistryEndpoint("docker.io"); got != "" {
		t.Fatalf("RegistryEndpoint(docker.io) = %q, want empty", got)
	}
}

func TestACRAuthProvider_OCIBaseURL(t *testing.T) {
	// No coordinate: dial https://<registry>.
	plain := NewACRAuthProvider(zerolog.Nop())
	plain.endpointURL = ""
	if got := plain.ociBaseURL("myacr.azurecr.io"); got != "https://myacr.azurecr.io" {
		t.Fatalf("ociBaseURL without coordinate = %q, want https://myacr.azurecr.io", got)
	}

	// Coordinate set (http scheme, trailing slash): honor scheme + relocated host.
	relocated := NewACRAuthProvider(zerolog.Nop(), "http://127.0.0.1:5000/")
	if got := relocated.ociBaseURL("myacr.azurecr.io"); got != "http://127.0.0.1:5000" {
		t.Fatalf("ociBaseURL with coordinate = %q, want http://127.0.0.1:5000", got)
	}
}
