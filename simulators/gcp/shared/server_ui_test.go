package simulator

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestRegisterUIExposesDeploymentNeutralIdentityCoordinates(t *testing.T) {
	t.Setenv("SIM_RUNTIME", "process")
	srv, err := NewServer(Config{Provider: "gcp", LogLevel: "disabled", UIIdentityEndpoint: "/identity", UILogoutEndpoint: "/logout"})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	srv.RegisterUI(fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("ui")}})
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ui/config.json", nil))
	var got map[string]string
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &got) != nil {
		t.Fatalf("UI config response: %d %q", rec.Code, rec.Body.String())
	}
	if got["identityEndpoint"] != "/identity" || got["logoutEndpoint"] != "/logout" {
		t.Fatalf("UI config coordinates: got %#v", got)
	}
}
