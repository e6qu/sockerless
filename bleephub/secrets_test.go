package bleephub

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

func TestSecretsCreateAndList(t *testing.T) {
	token := "bph_0000000000000000000000000000000000000000"

	// PUT a secret
	req, _ := http.NewRequest("PUT", testBaseURL+"/api/v3/repos/owner/repo/actions/secrets/MY_SECRET",
		bytes.NewBufferString(`{"value":"s3cret"}`))
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("create: status %d, want 201", resp.StatusCode)
	}

	// GET list
	req2, _ := http.NewRequest("GET", testBaseURL+"/api/v3/repos/owner/repo/actions/secrets", nil)
	req2.Header.Set("Authorization", "token "+token)
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != 200 {
		t.Fatalf("list: status %d, want 200", resp2.StatusCode)
	}

	body, _ := io.ReadAll(resp2.Body)
	var result map[string]interface{}
	json.Unmarshal(body, &result)
	count := int(result["total_count"].(float64))
	if count < 1 {
		t.Errorf("total_count = %d, want >= 1", count)
	}
}

func TestSecretsUpdate(t *testing.T) {
	token := "bph_0000000000000000000000000000000000000000"

	// Create
	req, _ := http.NewRequest("PUT", testBaseURL+"/api/v3/repos/owner/repo/actions/secrets/UPDATE_ME",
		bytes.NewBufferString(`{"value":"v1"}`))
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("create: status %d, want 201", resp.StatusCode)
	}

	// Update (same name)
	req2, _ := http.NewRequest("PUT", testBaseURL+"/api/v3/repos/owner/repo/actions/secrets/UPDATE_ME",
		bytes.NewBufferString(`{"value":"v2"}`))
	req2.Header.Set("Authorization", "token "+token)
	req2.Header.Set("Content-Type", "application/json")
	resp2, _ := http.DefaultClient.Do(req2)
	resp2.Body.Close()
	if resp2.StatusCode != 204 {
		t.Fatalf("update: status %d, want 204", resp2.StatusCode)
	}
}

func TestSecretsValueNotExposed(t *testing.T) {
	token := "bph_0000000000000000000000000000000000000000"

	// Create
	req, _ := http.NewRequest("PUT", testBaseURL+"/api/v3/repos/owner/repo/actions/secrets/HIDDEN_VAL",
		bytes.NewBufferString(`{"value":"top-secret"}`))
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()

	// GET single secret — should not contain value
	req2, _ := http.NewRequest("GET", testBaseURL+"/api/v3/repos/owner/repo/actions/secrets/HIDDEN_VAL", nil)
	req2.Header.Set("Authorization", "token "+token)
	resp2, _ := http.DefaultClient.Do(req2)
	defer resp2.Body.Close()

	body, _ := io.ReadAll(resp2.Body)
	if bytes.Contains(body, []byte("top-secret")) {
		t.Error("GET response exposes secret value")
	}
}

func TestSecretsDelete(t *testing.T) {
	token := "bph_0000000000000000000000000000000000000000"

	// Create
	req, _ := http.NewRequest("PUT", testBaseURL+"/api/v3/repos/owner/repo/actions/secrets/DELETE_ME",
		bytes.NewBufferString(`{"value":"val"}`))
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()

	// Delete
	req2, _ := http.NewRequest("DELETE", testBaseURL+"/api/v3/repos/owner/repo/actions/secrets/DELETE_ME", nil)
	req2.Header.Set("Authorization", "token "+token)
	resp2, _ := http.DefaultClient.Do(req2)
	resp2.Body.Close()
	if resp2.StatusCode != 204 {
		t.Fatalf("delete: status %d, want 204", resp2.StatusCode)
	}

	// Verify gone
	req3, _ := http.NewRequest("GET", testBaseURL+"/api/v3/repos/owner/repo/actions/secrets/DELETE_ME", nil)
	req3.Header.Set("Authorization", "token "+token)
	resp3, _ := http.DefaultClient.Do(req3)
	resp3.Body.Close()
	if resp3.StatusCode != 404 {
		t.Fatalf("after delete: status %d, want 404", resp3.StatusCode)
	}
}

func TestSecretsNoAuth401(t *testing.T) {
	// No auth header
	resp, err := http.Get(testBaseURL + "/api/v3/repos/owner/repo/actions/secrets")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestSecretsMissingRepo404(t *testing.T) {
	token := "bph_0000000000000000000000000000000000000000"

	req, _ := http.NewRequest("GET", testBaseURL+"/api/v3/repos/nonexist/repo/actions/secrets/NOPE", nil)
	req.Header.Set("Authorization", "token "+token)
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestSecretsStoreUnit(t *testing.T) {
	s := newTestServer()

	// Initialize secrets for a repo
	s.store.mu.Lock()
	s.store.RepoSecrets = make(map[string]map[string]*Secret)
	s.store.RepoSecrets["owner/repo"] = map[string]*Secret{
		"DB_PASS": {Name: "DB_PASS", Value: "password123"},
		"API_KEY": {Name: "API_KEY", Value: "key456"},
	}
	s.store.mu.Unlock()

	// Verify lookup
	s.store.mu.RLock()
	secrets := s.store.RepoSecrets["owner/repo"]
	s.store.mu.RUnlock()

	if len(secrets) != 2 {
		t.Fatalf("secrets count = %d, want 2", len(secrets))
	}
	if secrets["DB_PASS"].Value != "password123" {
		t.Errorf("DB_PASS value = %q", secrets["DB_PASS"].Value)
	}
}

func TestSecretsCRUDUnit(t *testing.T) {
	s := newTestServer()

	s.store.mu.Lock()
	s.store.RepoSecrets = make(map[string]map[string]*Secret)
	s.store.RepoSecrets["test/repo"] = make(map[string]*Secret)
	s.store.RepoSecrets["test/repo"]["KEY"] = &Secret{Name: "KEY", Value: "val1"}
	s.store.mu.Unlock()

	// Update
	s.store.mu.Lock()
	s.store.RepoSecrets["test/repo"]["KEY"].Value = "val2"
	s.store.mu.Unlock()

	s.store.mu.RLock()
	v := s.store.RepoSecrets["test/repo"]["KEY"].Value
	s.store.mu.RUnlock()
	if v != "val2" {
		t.Errorf("updated value = %q, want val2", v)
	}

	// Delete
	s.store.mu.Lock()
	delete(s.store.RepoSecrets["test/repo"], "KEY")
	s.store.mu.Unlock()

	s.store.mu.RLock()
	_, exists := s.store.RepoSecrets["test/repo"]["KEY"]
	s.store.mu.RUnlock()
	if exists {
		t.Error("secret still exists after delete")
	}
}
