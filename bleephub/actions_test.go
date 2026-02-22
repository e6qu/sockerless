package bleephub

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rs/zerolog"
)

func TestActionDownloadInfoReturnsFormat(t *testing.T) {
	s := newTestServer()

	body := `{"actions":[{"nameWithOwner":"actions/checkout","ref":"v4"}]}`
	req := httptest.NewRequest("POST", "/_apis/v1/ActionDownloadInfo/scope/hub/plan", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	s.handleActionDownloadInfo(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	actions, ok := resp["actions"].(map[string]interface{})
	if !ok {
		t.Fatal("response missing actions map")
	}

	entry, ok := actions["actions/checkout@v4"]
	if !ok {
		t.Fatal("missing key 'actions/checkout@v4'")
	}

	info := entry.(map[string]interface{})
	if info["nameWithOwner"] != "actions/checkout" {
		t.Errorf("nameWithOwner = %v", info["nameWithOwner"])
	}
	if info["ref"] != "v4" {
		t.Errorf("ref = %v", info["ref"])
	}

	tarURL, _ := info["tarballUrl"].(string)
	if tarURL == "" {
		t.Error("tarballUrl is empty")
	}
}

func TestActionDownloadInfoEmptyBody(t *testing.T) {
	s := newTestServer()

	req := httptest.NewRequest("POST", "/_apis/v1/ActionDownloadInfo/scope/hub/plan", bytes.NewBufferString("{}"))
	w := httptest.NewRecorder()
	s.handleActionDownloadInfo(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	actions := resp["actions"].(map[string]interface{})
	if len(actions) != 0 {
		t.Errorf("expected empty actions, got %d", len(actions))
	}
}

func TestActionDownloadInfoMultipleActions(t *testing.T) {
	s := newTestServer()

	body := `{"actions":[
		{"nameWithOwner":"actions/checkout","ref":"v4"},
		{"nameWithOwner":"actions/setup-go","ref":"v5"}
	]}`
	req := httptest.NewRequest("POST", "/_apis/v1/ActionDownloadInfo/scope/hub/plan", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	s.handleActionDownloadInfo(w, req)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	actions := resp["actions"].(map[string]interface{})
	if len(actions) != 2 {
		t.Errorf("expected 2 actions, got %d", len(actions))
	}
	if _, ok := actions["actions/checkout@v4"]; !ok {
		t.Error("missing actions/checkout@v4")
	}
	if _, ok := actions["actions/setup-go@v5"]; !ok {
		t.Error("missing actions/setup-go@v5")
	}
}

func TestActionTarball404ForMissing(t *testing.T) {
	s := newTestServer()

	req := httptest.NewRequest("GET", "/_apis/v1/actions/tarball/actions/checkout/v4", nil)
	req.SetPathValue("owner", "actions")
	req.SetPathValue("repo", "checkout")
	req.SetPathValue("ref", "v4")
	w := httptest.NewRecorder()
	s.handleActionTarball(w, req)

	// Without actual GitHub connectivity, this should fail with 502
	// (or succeed if cached). In test, it will be 502 since no internet.
	// The important thing is it doesn't panic or return 500.
	if w.Code != http.StatusOK && w.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 200 or 502", w.Code)
	}
}

func TestActionTarballServesFromCache(t *testing.T) {
	s := newTestServer()

	// Pre-populate cache
	s.actionCache.Put("actions/checkout@v4", &ActionCacheEntry{
		Data:        []byte("fake-tarball-data"),
		ResolvedSha: "abc123",
	})

	req := httptest.NewRequest("GET", "/_apis/v1/actions/tarball/actions/checkout/v4", nil)
	req.SetPathValue("owner", "actions")
	req.SetPathValue("repo", "checkout")
	req.SetPathValue("ref", "v4")
	w := httptest.NewRecorder()
	s.handleActionTarball(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if w.Body.String() != "fake-tarball-data" {
		t.Errorf("body = %q", w.Body.String())
	}
}

func TestActionCacheGetPut(t *testing.T) {
	ac := NewActionCache()

	if ac.Get("foo@v1") != nil {
		t.Error("expected nil for missing key")
	}

	ac.Put("foo@v1", &ActionCacheEntry{Data: []byte("data"), ResolvedSha: "sha"})
	entry := ac.Get("foo@v1")
	if entry == nil {
		t.Fatal("expected entry after Put")
	}
	if string(entry.Data) != "data" {
		t.Errorf("data = %q", string(entry.Data))
	}
}

// newTestServer creates a minimal server for unit testing.
func newTestServer() *Server {
	logger := zerolog.Nop()
	s := &Server{
		addr:          "127.0.0.1:0",
		mux:           http.NewServeMux(),
		logger:        logger,
		store:         NewStore(),
		actionCache:   NewActionCache(),
		artifactStore: NewArtifactStore(),
	}
	s.store.SeedDefaultUser()
	return s
}
