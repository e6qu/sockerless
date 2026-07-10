package bleephub

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
)

func TestLiveCodespaces_UserAndRepo(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	repo := testServer.store.CreateRepo(admin, "live-codespace-repo", "live", false)
	stor := testServer.store.GitStorages[repo.FullName]
	if _, err := initRepoWithFiles(stor, repo.DefaultBranch, "init", map[string]string{
		".devcontainer/devcontainer.json": fmt.Sprintf(`{"image":"%s"}`, codespaceTestImage),
	}, repoSignature(admin.Login, "bleephub@local")); err != nil {
		t.Fatalf("init repo: %v", err)
	}

	// Create via user endpoint.
	body, _ := json.Marshal(map[string]any{
		"repository_id": repo.ID,
		"machine":       "basicLinux32",
	})
	resp, err := authedPost("/api/v3/user/codespaces", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("create user codespace: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("create user codespace: %d %s", resp.StatusCode, b)
	}
	var userCs map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&userCs); err != nil {
		t.Fatalf("decode: %v", err)
	}
	resp.Body.Close()
	userName := userCs["name"].(string)
	t.Cleanup(func() {
		if cs := testServer.store.GetCodespaceByName(userName); cs != nil {
			_, _ = testServer.store.DeleteCodespace(cs.ID)
		}
		cleanupCodespaceContainer(t, userName)
	})

	// List user codespaces.
	resp = authedGet(t, "/api/v3/user/codespaces")
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("list user codespaces: %d %s", resp.StatusCode, b)
	}
	json.NewDecoder(resp.Body).Decode(&map[string]any{})
	resp.Body.Close()

	// Get user codespace.
	resp = authedGet(t, "/api/v3/user/codespaces/"+userName)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("get user codespace: %d %s", resp.StatusCode, b)
	}
	resp.Body.Close()

	// Create via repo endpoint.
	body, _ = json.Marshal(map[string]any{"machine": "basicLinux32"})
	resp, err = authedPost(fmt.Sprintf("/api/v3/repos/%s/codespaces", repo.FullName), "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("create repo codespace: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("create repo codespace: %d %s", resp.StatusCode, b)
	}
	var repoCs map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&repoCs); err != nil {
		t.Fatalf("decode: %v", err)
	}
	resp.Body.Close()
	repoName := repoCs["name"].(string)
	t.Cleanup(func() {
		if cs := testServer.store.GetCodespaceByName(repoName); cs != nil {
			_, _ = testServer.store.DeleteCodespace(cs.ID)
		}
		cleanupCodespaceContainer(t, repoName)
	})

	// List repo codespaces.
	resp = authedGet(t, fmt.Sprintf("/api/v3/repos/%s/codespaces", repo.FullName))
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("list repo codespaces: %d %s", resp.StatusCode, b)
	}
	json.NewDecoder(resp.Body).Decode(&map[string]any{})
	resp.Body.Close()

	// Machines list.
	resp = authedGet(t, fmt.Sprintf("/api/v3/repos/%s/codespaces/machines", repo.FullName))
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("list machines: %d %s", resp.StatusCode, b)
	}
	resp.Body.Close()
}

func TestLiveCodespaces_Secrets(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	repo := testServer.store.CreateRepo(admin, "live-cs-secrets-repo", "live", false)
	stor := testServer.store.GitStorages[repo.FullName]
	if _, err := initRepoWithFiles(stor, repo.DefaultBranch, "init", map[string]string{"README.md": "# hi"}, repoSignature(admin.Login, "bleephub@local")); err != nil {
		t.Fatalf("init repo: %v", err)
	}

	// Public key.
	resp := authedGet(t, "/api/v3/user/codespaces/secrets/public-key")
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("public key: %d %s", resp.StatusCode, b)
	}
	var pk struct {
		KeyID string `json:"key_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&pk); err != nil {
		t.Fatalf("decode pk: %v", err)
	}
	resp.Body.Close()

	enc, _, _ := testServer.store.SealSecretValue("live-secret")
	body, _ := json.Marshal(map[string]any{"encrypted_value": enc, "key_id": pk.KeyID})

	// User secret (PUT).
	resp, err := authedPut("/api/v3/user/codespaces/secrets/LIVE_SECRET", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("put user secret: %v", err)
	}
	if resp.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("put user secret: %d %s", resp.StatusCode, b)
	}
	resp.Body.Close()

	// Repo secret (PUT).
	resp, err = authedPut(fmt.Sprintf("/api/v3/repos/%s/codespaces/secrets/LIVE_REPO_SECRET", repo.FullName), "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("put repo secret: %v", err)
	}
	if resp.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("put repo secret: %d %s", resp.StatusCode, b)
	}
	resp.Body.Close()
}

func authedPut(path, contentType string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest("PUT", testBaseURL+path, body)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("Authorization", "Bearer "+defaultToken)
	return http.DefaultClient.Do(req)
}
