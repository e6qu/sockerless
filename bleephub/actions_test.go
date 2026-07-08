package bleephub

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rs/zerolog"
)

func TestActionDownloadInfoReturnsFormat(t *testing.T) {
	s := newTestServer()
	commitFilesToStorage(t, s, "actions/checkout", map[string]string{
		"action.yml": "name: checkout\nruns:\n  using: composite\n  steps: []\n",
	})

	body := `{"actions":[{"nameWithOwner":"actions/checkout","ref":"master"}]}`
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

	entry, ok := actions["actions/checkout@master"]
	if !ok {
		t.Fatal("missing key 'actions/checkout@master'")
	}

	info := entry.(map[string]interface{})
	if info["nameWithOwner"] != "actions/checkout" {
		t.Errorf("nameWithOwner = %v", info["nameWithOwner"])
	}
	if info["ref"] != "master" {
		t.Errorf("ref = %v", info["ref"])
	}

	tarURL, _ := info["tarballUrl"].(string)
	if tarURL == "" {
		t.Error("tarballUrl is empty")
	}
}

func TestActionDownloadInfoResolvesLocalActionSha(t *testing.T) {
	s := newTestServer()
	commitFilesToStorage(t, s, "actions/local-checkout", map[string]string{
		"action.yml": "name: local checkout\nruns:\n  using: composite\n  steps: []\n",
	})
	stor := s.store.GetGitStorage("actions", "local-checkout")
	wantSha := resolveActionRefSha(stor, "master")
	if wantSha == "0000000000000000000000000000000000000000" {
		t.Fatal("test repository did not resolve master")
	}

	body := `{"actions":[{"nameWithOwner":"actions/local-checkout","ref":"master"}]}`
	req := httptest.NewRequest("POST", "/_apis/v1/ActionDownloadInfo/scope/hub/plan", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	s.handleActionDownloadInfo(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp struct {
		Actions map[string]struct {
			ResolvedSha string `json:"resolvedSha"`
		} `json:"actions"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	got := resp.Actions["actions/local-checkout@master"].ResolvedSha
	if got != wantSha {
		t.Fatalf("resolvedSha = %q, want local git commit %q", got, wantSha)
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
	commitFilesToStorage(t, s, "actions/checkout", map[string]string{
		"action.yml": "name: checkout\nruns:\n  using: composite\n  steps: []\n",
	})
	commitFilesToStorage(t, s, "actions/setup-go", map[string]string{
		"action.yml": "name: setup go\nruns:\n  using: composite\n  steps: []\n",
	})

	body := `{"actions":[
		{"nameWithOwner":"actions/checkout","ref":"master"},
		{"nameWithOwner":"actions/setup-go","ref":"master"}
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
	if _, ok := actions["actions/checkout@master"]; !ok {
		t.Error("missing actions/checkout@master")
	}
	if _, ok := actions["actions/setup-go@master"]; !ok {
		t.Error("missing actions/setup-go@master")
	}
}

func TestActionDownloadInfoFailsLoudForUnresolvedAction(t *testing.T) {
	s := newTestServer()

	body := `{"actions":[{"nameWithOwner":"actions/checkout","ref":"v4"}]}`
	req := httptest.NewRequest("POST", "/_apis/v1/ActionDownloadInfo/scope/hub/plan", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	s.handleActionDownloadInfo(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422 for an action absent from bleephub git storage", w.Code)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("not resolvable from bleephub git storage")) {
		t.Fatalf("body = %q", w.Body.String())
	}
}

func TestActionTarball404ForMissing(t *testing.T) {
	s := newTestServer()
	commitFilesToStorage(t, s, "actions/checkout", map[string]string{
		"action.yml": "name: checkout\nruns:\n  using: composite\n  steps: []\n",
	})

	req := httptest.NewRequest("GET", "/_apis/v1/actions/tarball/actions/checkout/v4", nil)
	req.SetPathValue("owner", "actions")
	req.SetPathValue("repo", "checkout")
	req.SetPathValue("ref", "v4")
	w := httptest.NewRecorder()
	s.handleActionTarball(w, req)

	if w.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502 for an unresolved local action ref", w.Code)
	}
}

func TestActionTarballFailsLoudForExternalAction(t *testing.T) {
	s := newTestServer()

	req := httptest.NewRequest("GET", "/_apis/v1/actions/tarball/actions/checkout/v4", nil)
	req.SetPathValue("owner", "actions")
	req.SetPathValue("repo", "checkout")
	req.SetPathValue("ref", "v4")
	w := httptest.NewRecorder()
	s.handleActionTarball(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for action repository absent from bleephub", w.Code)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("not hosted in bleephub")) {
		t.Fatalf("body = %q", w.Body.String())
	}
}

func TestActionTarballServesFromCache(t *testing.T) {
	s := newTestServer()
	cachedTarball := testActionTarball(t)

	s.actionCache.Put("actions/checkout@v4", &ActionCacheEntry{
		Data:        cachedTarball,
		ResolvedSha: "abcdef0123456789abcdef0123456789abcdef01",
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
	gz, err := gzip.NewReader(bytes.NewReader(w.Body.Bytes()))
	if err != nil {
		t.Fatalf("cached tarball is not gzip: %v", err)
	}
	tr := tar.NewReader(gz)
	hdr, err := tr.Next()
	if err != nil {
		t.Fatalf("read cached tarball: %v", err)
	}
	if hdr.Name != "actions-checkout-abcdef0/action.yml" {
		t.Fatalf("cached tarball first entry = %q", hdr.Name)
	}
	content, err := io.ReadAll(tr)
	if err != nil {
		t.Fatalf("read action.yml: %v", err)
	}
	if !bytes.Contains(content, []byte("using: composite")) {
		t.Fatalf("cached action.yml content = %q", string(content))
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

func testActionTarball(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	content := []byte("name: checkout\nruns:\n  using: composite\n  steps: []\n")
	if err := tw.WriteHeader(&tar.Header{
		Name: "actions-checkout-abcdef0/action.yml",
		Mode: 0o644,
		Size: int64(len(content)),
	}); err != nil {
		t.Fatalf("write tar header: %v", err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatalf("write tar content: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return buf.Bytes()
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
		artifactStore: NewArtifactStoreWithByteStore("", nil),
	}
	s.store.SeedDefaultUser()
	return s
}
