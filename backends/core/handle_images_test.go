package core

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// handleImagePush should reject a malformed X-Registry-Auth-style ?auth=
// query param rather than silently degrading to "no auth". Real Docker
// daemon returns 400 on a body it can't parse; bleephub's "registry"
// driver below cannot recover auth from garbage.
func TestImagePush_RejectsMalformedAuthBase64(t *testing.T) {
	s := newExtendedTestServer()
	req := httptest.NewRequest(http.MethodPost, "/images/example/push?tag=v1&auth=not-valid-base64!!!", nil)
	req.SetPathValue("name", "example")
	w := httptest.NewRecorder()

	s.handleImagePush(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for malformed base64, got %d (body=%q)", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "invalid base64") {
		t.Fatalf("expected error message to mention base64, got %q", w.Body.String())
	}
}

func TestImagePush_RejectsMalformedAuthJSON(t *testing.T) {
	s := newExtendedTestServer()
	// Valid base64 of a non-JSON payload.
	badJSON := base64.StdEncoding.EncodeToString([]byte("this is not json"))
	req := httptest.NewRequest(http.MethodPost, "/images/example/push?tag=v1&auth="+badJSON, nil)
	req.SetPathValue("name", "example")
	w := httptest.NewRecorder()

	s.handleImagePush(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for non-JSON auth body, got %d (body=%q)", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "invalid JSON") {
		t.Fatalf("expected error message to mention JSON, got %q", w.Body.String())
	}
}
