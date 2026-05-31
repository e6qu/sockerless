package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestHTTPSGatewayInfoDefaultsWhenStopped(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "make", "components.mk"), "")
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	registerHTTPSGatewayAPI(mux)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/https-gateway", nil)
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	var got HTTPSGatewayInfo
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Running {
		t.Fatalf("gateway should not be running: %+v", got)
	}
	if got.Port != 8443 || got.AdminPort != 28443 {
		t.Fatalf("ports = %d/%d, want 8443/28443", got.Port, got.AdminPort)
	}
	if got.Endpoints["azure_blob"] != "https://{account}.blob.azure.sockerless.localhost:8443" {
		t.Fatalf("azure blob endpoint = %q", got.Endpoints["azure_blob"])
	}
}

func TestHTTPSGatewayInfoReadsMakeEnv(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "make", "components.mk"), "")
	ca := filepath.Join(dir, "ca.pem")
	mustWrite(t, ca, "cert")
	mustWrite(t, filepath.Join(dir, ".stack-pids", "https-gateway.env"), ""+
		"SOCKERLESS_HTTPS_GATEWAY_PORT=9443\n"+
		"SOCKERLESS_HTTPS_GATEWAY_ADMIN_PORT=29443\n"+
		"SOCKERLESS_HTTPS_GATEWAY_CA_CERT="+ca+"\n")
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	got := readHTTPSGatewayInfo()
	if got.Port != 9443 || got.AdminPort != 29443 {
		t.Fatalf("ports = %d/%d, want 9443/29443", got.Port, got.AdminPort)
	}
	if got.CAPath != ca || !got.CAPresent {
		t.Fatalf("CA = %q present=%v, want %q true", got.CAPath, got.CAPresent, ca)
	}
	if got.Endpoints["aws"] != "https://aws.sockerless.localhost:9443" {
		t.Fatalf("aws endpoint = %q", got.Endpoints["aws"])
	}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
