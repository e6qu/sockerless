package bleephub

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

const codespaceTestImage = "alpine:latest"

func cleanupCodespaceContainer(t *testing.T, name string) {
	t.Helper()
	ctx, cancel := contextWithTimeout(30 * time.Second)
	defer cancel()
	_ = dockerRemoveContainer(ctx, codespaceContainerName(name))
}

func contextWithTimeout(d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), d)
}

func createTestCodespaceRepo(t *testing.T, name string) *Repo {
	t.Helper()
	admin := testServer.store.UsersByLogin["admin"]
	repo := testServer.store.CreateRepo(admin, name, "codespace test repo", false)
	if repo == nil {
		t.Fatalf("failed to create repo %s", name)
	}
	// Seed a devcontainer.json pointing at the fast test image.
	stor := testServer.store.GitStorages[repo.FullName]
	if _, err := initRepoWithFiles(stor, repo.DefaultBranch, "init", map[string]string{
		".devcontainer/devcontainer.json": fmt.Sprintf(`{"image":"%s"}`, codespaceTestImage),
	}, repoSignature(admin.Login, "bleephub@local")); err != nil {
		t.Fatalf("init repo files: %v", err)
	}
	return repo
}

func TestCodespaces_UserCreateListGetDelete(t *testing.T) {
	repo := createTestCodespaceRepo(t, "cs-user-repo")

	// Create via user endpoint.
	resp := ghPost(t, "/api/v3/user/codespaces", defaultToken, map[string]any{
		"repository_id": repo.ID,
		"machine":       "basicLinux32",
		"display_name":  "User Codespace",
	})
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("create user codespace: %d %s", resp.StatusCode, b)
	}
	created := decodeJSON(t, resp)
	name := created["name"].(string)
	t.Cleanup(func() {
		if cs := testServer.store.GetCodespaceByName(name); cs != nil {
			testServer.store.DeleteCodespace(cs.ID)
		}
		cleanupCodespaceContainer(t, name)
	})
	if created["state"] != "Available" && created["state"] != "Unavailable" {
		t.Fatalf("unexpected state: %v", created["state"])
	}
	if created["display_name"] != "User Codespace" {
		t.Fatalf("unexpected display_name: %v", created["display_name"])
	}

	// List user codespaces.
	resp = ghGet(t, "/api/v3/user/codespaces", defaultToken)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("list user codespaces: %d %s", resp.StatusCode, b)
	}
	var listResp struct {
		Codespaces []map[string]any `json:"codespaces"`
		TotalCount int              `json:"total_count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	resp.Body.Close()
	found := false
	for _, cs := range listResp.Codespaces {
		if cs["name"].(string) == name {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("created codespace not in list: %v", listResp.Codespaces)
	}

	// Get user codespace.
	resp = ghGet(t, "/api/v3/user/codespaces/"+name, defaultToken)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("get user codespace: %d %s", resp.StatusCode, b)
	}
	got := decodeJSON(t, resp)
	if got["name"].(string) != name {
		t.Fatalf("unexpected name: %v", got["name"])
	}

	// Patch.
	resp = ghPatch(t, "/api/v3/user/codespaces/"+name, defaultToken, map[string]any{
		"display_name": "Renamed",
	})
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("patch user codespace: %d %s", resp.StatusCode, b)
	}
	patched := decodeJSON(t, resp)
	if patched["display_name"] != "Renamed" {
		t.Fatalf("patch did not update display_name: %v", patched["display_name"])
	}

	// Delete.
	resp = ghDelete(t, "/api/v3/user/codespaces/"+name, defaultToken)
	if resp.StatusCode != http.StatusAccepted {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("delete user codespace: %d %s", resp.StatusCode, b)
	}
	resp.Body.Close()

	// Ensure container removed.
	ctx, cancel := contextWithTimeout(10 * time.Second)
	defer cancel()
	if out, _ := runDockerCLI(ctx, "ps", "-a", "--filter", "name="+codespaceContainerName(name), "--format", "{{.Names}}"); strings.TrimSpace(string(out)) != "" {
		t.Fatalf("container still exists after delete")
	}
}

func TestCodespaces_RepoCreateStartStopDelete(t *testing.T) {
	repo := createTestCodespaceRepo(t, "cs-repo-repo")

	resp := ghPost(t, fmt.Sprintf("/api/v3/repos/%s/codespaces", repo.FullName), defaultToken, map[string]any{
		"machine":      "basicLinux32",
		"display_name": "Repo Codespace",
	})
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("create repo codespace: %d %s", resp.StatusCode, b)
	}
	created := decodeJSON(t, resp)
	name := created["name"].(string)
	t.Cleanup(func() {
		if cs := testServer.store.GetCodespaceByName(name); cs != nil {
			testServer.store.DeleteCodespace(cs.ID)
		}
		cleanupCodespaceContainer(t, name)
	})

	// Start then stop.
	resp = ghPost(t, fmt.Sprintf("/api/v3/repos/%s/codespaces/%s/start", repo.FullName, name), defaultToken, nil)
	if resp.StatusCode != http.StatusAccepted {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("start repo codespace: %d %s", resp.StatusCode, b)
	}
	started := decodeJSON(t, resp)
	resp.Body.Close()
	if started["state"] != "Available" && started["state"] != "Unavailable" {
		t.Fatalf("unexpected start state: %v", started["state"])
	}

	resp = ghPost(t, fmt.Sprintf("/api/v3/repos/%s/codespaces/%s/stop", repo.FullName, name), defaultToken, nil)
	if resp.StatusCode != http.StatusAccepted {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("stop repo codespace: %d %s", resp.StatusCode, b)
	}
	stopped := decodeJSON(t, resp)
	resp.Body.Close()
	if stopped["state"] != "Shutdown" && stopped["state"] != "Unavailable" {
		t.Fatalf("unexpected stop state: %v", stopped["state"])
	}

	// Delete.
	resp = ghDelete(t, fmt.Sprintf("/api/v3/repos/%s/codespaces/%s", repo.FullName, name), defaultToken)
	if resp.StatusCode != http.StatusAccepted {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("delete repo codespace: %d %s", resp.StatusCode, b)
	}
	resp.Body.Close()
}

func TestCodespaces_MachinesList(t *testing.T) {
	repo := createTestCodespaceRepo(t, "cs-machines-repo")
	resp := ghGet(t, fmt.Sprintf("/api/v3/repos/%s/codespaces/machines", repo.FullName), defaultToken)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("list machines: %d %s", resp.StatusCode, b)
	}
	var m struct {
		Machines   []map[string]any `json:"machines"`
		TotalCount int              `json:"total_count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		t.Fatalf("decode machines: %v", err)
	}
	resp.Body.Close()
	if m.TotalCount == 0 {
		t.Fatal("expected machines")
	}
}

func TestCodespaces_UserSecretsCRUD(t *testing.T) {
	// Fetch public key.
	resp := ghGet(t, "/api/v3/user/codespaces/secrets/public-key", defaultToken)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("get public key: %d %s", resp.StatusCode, b)
	}
	pk := decodeJSON(t, resp)
	keyID := pk["key_id"].(string)

	// Encrypt a dummy value.
	plain := "secret-value"
	enc, _, err := testServer.store.SealSecretValue(plain)
	if err != nil {
		t.Fatalf("seal secret: %v", err)
	}

	// Put secret.
	resp = ghPut(t, "/api/v3/user/codespaces/secrets/MY_SECRET", defaultToken, map[string]any{
		"encrypted_value": enc,
		"key_id":          keyID,
	})
	if resp.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("put user secret: %d %s", resp.StatusCode, b)
	}
	resp.Body.Close()

	// List.
	resp = ghGet(t, "/api/v3/user/codespaces/secrets", defaultToken)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("list user secrets: %d %s", resp.StatusCode, b)
	}
	var listResp struct {
		Secrets    []map[string]any `json:"secrets"`
		TotalCount int              `json:"total_count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		t.Fatalf("decode secrets: %v", err)
	}
	resp.Body.Close()
	found := false
	for _, s := range listResp.Secrets {
		if s["name"].(string) == "MY_SECRET" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("secret not in list")
	}

	// Get.
	resp = ghGet(t, "/api/v3/user/codespaces/secrets/MY_SECRET", defaultToken)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("get user secret: %d %s", resp.StatusCode, b)
	}
	resp.Body.Close()

	// Delete.
	resp = ghDelete(t, "/api/v3/user/codespaces/secrets/MY_SECRET", defaultToken)
	if resp.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("delete user secret: %d %s", resp.StatusCode, b)
	}
	resp.Body.Close()
}

func TestCodespaces_RepoSecretsCRUD(t *testing.T) {
	repo := createTestCodespaceRepo(t, "cs-repo-secrets")
	resp := ghGet(t, fmt.Sprintf("/api/v3/repos/%s/codespaces/secrets/public-key", repo.FullName), defaultToken)
	pk := decodeJSON(t, resp)
	keyID := pk["key_id"].(string)
	enc, _, _ := testServer.store.SealSecretValue("repo-secret")

	resp = ghPut(t, fmt.Sprintf("/api/v3/repos/%s/codespaces/secrets/REPO_SECRET", repo.FullName), defaultToken, map[string]any{
		"encrypted_value": enc,
		"key_id":          keyID,
	})
	if resp.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("put repo secret: %d %s", resp.StatusCode, b)
	}
	resp.Body.Close()

	resp = ghGet(t, fmt.Sprintf("/api/v3/repos/%s/codespaces/secrets/REPO_SECRET", repo.FullName), defaultToken)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("get repo secret: %d %s", resp.StatusCode, b)
	}
	resp.Body.Close()

	resp = ghDelete(t, fmt.Sprintf("/api/v3/repos/%s/codespaces/secrets/REPO_SECRET", repo.FullName), defaultToken)
	if resp.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("delete repo secret: %d %s", resp.StatusCode, b)
	}
	resp.Body.Close()
}

func TestCodespaces_OrgSecretsCRUD(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	org := testServer.store.CreateOrg(admin, "cs-secrets-org", "Codespaces Secrets Org", "")

	resp := ghGet(t, fmt.Sprintf("/api/v3/orgs/%s/codespaces/secrets/public-key", org.Login), defaultToken)
	pk := decodeJSON(t, resp)
	keyID := pk["key_id"].(string)
	enc, _, _ := testServer.store.SealSecretValue("org-secret")

	resp = ghPut(t, fmt.Sprintf("/api/v3/orgs/%s/codespaces/secrets/ORG_SECRET", org.Login), defaultToken, map[string]any{
		"encrypted_value": enc,
		"key_id":          keyID,
		"visibility":      "all",
	})
	if resp.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("put org secret: %d %s", resp.StatusCode, b)
	}
	resp.Body.Close()

	resp = ghGet(t, fmt.Sprintf("/api/v3/orgs/%s/codespaces/secrets/ORG_SECRET", org.Login), defaultToken)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("get org secret: %d %s", resp.StatusCode, b)
	}
	resp.Body.Close()

	resp = ghGet(t, fmt.Sprintf("/api/v3/orgs/%s/codespaces/secrets/ORG_SECRET/repositories", org.Login), defaultToken)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("list org secret repos: %d %s", resp.StatusCode, b)
	}
	resp.Body.Close()

	resp = ghDelete(t, fmt.Sprintf("/api/v3/orgs/%s/codespaces/secrets/ORG_SECRET", org.Login), defaultToken)
	if resp.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("delete org secret: %d %s", resp.StatusCode, b)
	}
	resp.Body.Close()
}

func TestCodespaces_404Cases(t *testing.T) {
	repo := createTestCodespaceRepo(t, "cs-404-repo")

	resp := ghGet(t, fmt.Sprintf("/api/v3/repos/%s/codespaces/no-such-codespace", repo.FullName), defaultToken)
	if resp.StatusCode != http.StatusNotFound {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("expected 404, got %d %s", resp.StatusCode, b)
	}
	resp.Body.Close()

	resp = ghGet(t, "/api/v3/repos/no-owner/no-repo/codespaces", defaultToken)
	if resp.StatusCode != http.StatusNotFound {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("expected 404 repo, got %d %s", resp.StatusCode, b)
	}
	resp.Body.Close()
}

// runDockerCLI is already defined in store_codespaces.go.
