package bleephub

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
)

// seedCodespaceRecord inserts a codespace store entity directly — the
// state of a codespace whose backing container is gone (State Shutdown,
// no container) — so codespace sub-resource endpoints that never touch
// the container can be exercised without a Docker round-trip.
func seedCodespaceRecord(t *testing.T, ownerLogin, repoKey string) *Codespace {
	t.Helper()
	name, err := generateCodespaceName(repoKey)
	if err != nil {
		t.Fatalf("generate codespace name: %v", err)
	}
	st := testServer.store
	st.mu.Lock()
	m := codespaceDefaultMachine()
	now := time.Now().UTC()
	cs := &Codespace{
		ID:                 st.NextCodespaceID,
		Name:               name,
		OwnerLogin:         ownerLogin,
		RepoKey:            repoKey,
		MachineName:        m.Name,
		MachineDisplayName: m.DisplayName,
		MachineType:        m.Type,
		CreatedAt:          now,
		UpdatedAt:          now,
		LastUsedAt:         now,
		State:              "Shutdown",
	}
	cs.DisplayName = cs.Name
	st.Codespaces[cs.ID] = cs
	st.CodespacesByName[cs.Name] = cs
	st.NextCodespaceID++
	st.mu.Unlock()
	t.Cleanup(func() {
		st.mu.Lock()
		delete(st.Codespaces, cs.ID)
		delete(st.CodespacesByName, cs.Name)
		st.mu.Unlock()
	})
	return cs
}

type errReader struct {
	err error
}

func (r errReader) Read([]byte) (int, error) {
	return 0, r.err
}

func TestGenerateCodespaceNameRequiresRandomBytes(t *testing.T) {
	name, err := generateCodespaceNameWithReader("octo/repo", bytes.NewReader([]byte{0xab, 0xcd, 0xef, 0x12}))
	if err != nil {
		t.Fatalf("generate codespace name: %v", err)
	}
	if name != "github-repo-abcdef1" {
		t.Fatalf("name = %q, want github-repo-abcdef1", name)
	}

	wantErr := errors.New("entropy unavailable")
	if _, err := generateCodespaceNameWithReader("octo/repo", errReader{err: wantErr}); !errors.Is(err, wantErr) {
		t.Fatalf("generate with failing reader error = %v, want %v", err, wantErr)
	}
}

func TestCodespacesUserMachines_RealCatalogValues(t *testing.T) {
	repo := createTestCodespaceRepo(t, "cs-user-machines-repo")
	cs := seedCodespaceRecord(t, "admin", repo.FullName)

	resp := ghGet(t, "/api/v3/user/codespaces/"+cs.Name+"/machines", defaultToken)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("list machines: %d %s", resp.StatusCode, b)
	}
	var out struct {
		TotalCount int              `json:"total_count"`
		Machines   []map[string]any `json:"machines"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode machines: %v", err)
	}
	resp.Body.Close()
	if out.TotalCount != len(codespaceMachines) || len(out.Machines) != len(codespaceMachines) {
		t.Fatalf("machines count = %d/%d, want %d", out.TotalCount, len(out.Machines), len(codespaceMachines))
	}
	// Every catalog entry carries its own real resources, not one
	// repeated placeholder row.
	byName := map[string]map[string]any{}
	for _, m := range out.Machines {
		byName[m["name"].(string)] = m
	}
	if m := byName["basicLinux32"]; m == nil || m["cpus"] != 2.0 || m["memory_in_bytes"] != float64(4*codespaceGiB) {
		t.Fatalf("basicLinux32 machine = %v", byName["basicLinux32"])
	}
	if m := byName["largeLinux64"]; m == nil || m["cpus"] != 16.0 || m["storage_in_bytes"] != float64(64*codespaceGiB) {
		t.Fatalf("largeLinux64 machine = %v", byName["largeLinux64"])
	}

	// Unknown sub-resources under a codespace are 404.
	respBogus := ghGet(t, "/api/v3/user/codespaces/"+cs.Name+"/bogus", defaultToken)
	respBogus.Body.Close()
	if respBogus.StatusCode != http.StatusNotFound {
		t.Fatalf("bogus sub-resource: %d, want 404", respBogus.StatusCode)
	}
}

func TestCodespacesExport_CreatesRealBranch(t *testing.T) {
	repo := createTestCodespaceRepo(t, "cs-export-repo")
	cs := seedCodespaceRecord(t, "admin", repo.FullName)

	resp := ghPost(t, "/api/v3/user/codespaces/"+cs.Name+"/exports", defaultToken, nil)
	if resp.StatusCode != http.StatusAccepted {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("export codespace: %d %s", resp.StatusCode, b)
	}
	export := decodeJSON(t, resp)
	if export["state"] != "succeeded" || export["id"] != "latest" {
		t.Fatalf("export = %v", export)
	}
	branch := export["branch"].(string)
	if branch != "codespace-"+cs.Name {
		t.Fatalf("export branch = %q", branch)
	}
	sha, _ := export["sha"].(string)
	if len(sha) != 40 {
		t.Fatalf("export sha = %q", sha)
	}

	// The exported branch really exists in the repository's git storage
	// and points at the exported commit.
	stor := testServer.store.GitStorages[repo.FullName]
	ref, err := stor.Reference(plumbing.NewBranchReferenceName(branch))
	if err != nil {
		t.Fatalf("exported branch missing from git storage: %v", err)
	}
	if ref.Hash().String() != sha {
		t.Fatalf("exported branch hash = %s, export sha = %s", ref.Hash(), sha)
	}

	// Export details round-trip via GET with id "latest".
	got := decodeJSON(t, ghGet(t, "/api/v3/user/codespaces/"+cs.Name+"/exports/latest", defaultToken))
	if got["branch"] != branch || got["sha"] != sha {
		t.Fatalf("GET export = %v", got)
	}

	respMissing := ghGet(t, "/api/v3/user/codespaces/"+cs.Name+"/exports/nope", defaultToken)
	respMissing.Body.Close()
	if respMissing.StatusCode != http.StatusNotFound {
		t.Fatalf("GET unknown export: %d, want 404", respMissing.StatusCode)
	}

	// Exporting an unpublished codespace (no repository) is a 422.
	unpublished := seedCodespaceRecord(t, "admin", "")
	respNoRepo := ghPost(t, "/api/v3/user/codespaces/"+unpublished.Name+"/exports", defaultToken, nil)
	respNoRepo.Body.Close()
	if respNoRepo.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("export unpublished codespace: %d, want 422", respNoRepo.StatusCode)
	}
}

func TestCodespacesPublish_CreatesRepository(t *testing.T) {
	cs := seedCodespaceRecord(t, "admin", "")

	resp := ghPost(t, "/api/v3/user/codespaces/"+cs.Name+"/publish", defaultToken, map[string]any{
		"name":    "cs-published-repo",
		"private": true,
	})
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("publish codespace: %d %s", resp.StatusCode, b)
	}
	published := decodeJSON(t, resp)
	repoJSON, _ := published["repository"].(map[string]any)
	if repoJSON == nil || repoJSON["full_name"] != "admin/cs-published-repo" || repoJSON["private"] != true {
		t.Fatalf("published repository = %v", published["repository"])
	}

	// The repository is a real store entity reachable through the API.
	repoResp := ghGet(t, "/api/v3/repos/admin/cs-published-repo", defaultToken)
	if repoResp.StatusCode != http.StatusOK {
		repoResp.Body.Close()
		t.Fatalf("GET published repo: %d", repoResp.StatusCode)
	}
	repoResp.Body.Close()

	// Publishing an already-published codespace is a 422.
	again := ghPost(t, "/api/v3/user/codespaces/"+cs.Name+"/publish", defaultToken, map[string]any{"name": "other"})
	again.Body.Close()
	if again.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("re-publish: %d, want 422", again.StatusCode)
	}

	// A repository name collision is a 422.
	other := seedCodespaceRecord(t, "admin", "")
	conflict := ghPost(t, "/api/v3/user/codespaces/"+other.Name+"/publish", defaultToken, map[string]any{"name": "cs-published-repo"})
	conflict.Body.Close()
	if conflict.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("publish with taken name: %d, want 422", conflict.StatusCode)
	}
}

func TestCodespacesUserSecret_SelectedRepositories(t *testing.T) {
	repoA := seedTestRepo(t, "cs-secret-repo-a", false)
	repoB := seedTestRepo(t, "cs-secret-repo-b", false)

	put := putSealedSecret(t, "/api/v3/user/codespaces/secrets/CS_SEL_SECRET", "sekrit")
	put.Body.Close()
	if put.StatusCode != http.StatusNoContent {
		t.Fatalf("put secret: %d", put.StatusCode)
	}

	// Set the full selected list.
	resp := ghPut(t, "/api/v3/user/codespaces/secrets/CS_SEL_SECRET/repositories", defaultToken, map[string]any{
		"selected_repository_ids": []int{repoA.ID},
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("set selected repos: %d", resp.StatusCode)
	}

	listSelected := func() []map[string]any {
		t.Helper()
		resp := ghGet(t, "/api/v3/user/codespaces/secrets/CS_SEL_SECRET/repositories", defaultToken)
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			t.Fatalf("list selected repos: %d", resp.StatusCode)
		}
		var out struct {
			TotalCount   int              `json:"total_count"`
			Repositories []map[string]any `json:"repositories"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatalf("decode selected repos: %v", err)
		}
		resp.Body.Close()
		if out.TotalCount != len(out.Repositories) {
			t.Fatalf("total_count %d != len(repositories) %d", out.TotalCount, len(out.Repositories))
		}
		return out.Repositories
	}

	repos := listSelected()
	if len(repos) != 1 || int(repos[0]["id"].(float64)) != repoA.ID {
		t.Fatalf("selected repos = %v", repos)
	}

	// Add one repository, then remove it.
	resp = ghPut(t, fmt.Sprintf("/api/v3/user/codespaces/secrets/CS_SEL_SECRET/repositories/%d", repoB.ID), defaultToken, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("add selected repo: %d", resp.StatusCode)
	}
	if repos = listSelected(); len(repos) != 2 {
		t.Fatalf("selected repos after add = %v", repos)
	}
	resp = ghDelete(t, fmt.Sprintf("/api/v3/user/codespaces/secrets/CS_SEL_SECRET/repositories/%d", repoB.ID), defaultToken)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("remove selected repo: %d", resp.StatusCode)
	}
	if repos = listSelected(); len(repos) != 1 {
		t.Fatalf("selected repos after remove = %v", repos)
	}

	// Unknown repositories and unknown secrets are 404.
	resp = ghPut(t, "/api/v3/user/codespaces/secrets/CS_SEL_SECRET/repositories", defaultToken, map[string]any{
		"selected_repository_ids": []int{99999999},
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("set with unknown repo: %d, want 404", resp.StatusCode)
	}
	resp = ghGet(t, "/api/v3/user/codespaces/secrets/NO_SUCH_SECRET/repositories", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("list for unknown secret: %d, want 404", resp.StatusCode)
	}

	del := ghDelete(t, "/api/v3/user/codespaces/secrets/CS_SEL_SECRET", defaultToken)
	del.Body.Close()
}

func TestCodespacesCreateForPullRequest(t *testing.T) {
	repo := createTestCodespaceRepo(t, "cs-pr-repo")

	// A real head branch (with the devcontainer) backs the pull request.
	stor := testServer.store.GitStorages[repo.FullName]
	admin := testServer.store.UsersByLogin["admin"]
	if _, err := initRepoWithFiles(stor, "feature", "feature work", map[string]string{
		".devcontainer/devcontainer.json": fmt.Sprintf(`{"image":%q}`, codespaceTestImage),
		"feature.txt":                     "feature",
	}, repoSignature(admin.Login, "bleephub@local")); err != nil {
		t.Fatalf("init feature branch: %v", err)
	}
	prResp := ghPost(t, "/api/v3/repos/"+repo.FullName+"/pulls", defaultToken, map[string]any{
		"title": "PR for codespace",
		"head":  "feature",
		"base":  "main",
	})
	if prResp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(prResp.Body)
		prResp.Body.Close()
		t.Fatalf("create pull request: %d %s", prResp.StatusCode, b)
	}
	pr := decodeJSON(t, prResp)
	num := int(pr["number"].(float64))

	resp := ghPost(t, fmt.Sprintf("/api/v3/repos/%s/pulls/%d/codespaces", repo.FullName, num), defaultToken, map[string]any{
		"machine": "basicLinux32",
	})
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("create pull request codespace: %d %s", resp.StatusCode, b)
	}
	created := decodeJSON(t, resp)
	name := created["name"].(string)
	t.Cleanup(func() {
		if cs := testServer.store.GetCodespaceByName(name); cs != nil {
			_, _ = testServer.store.DeleteCodespace(cs.ID)
		}
		cleanupCodespaceContainer(t, name)
	})
	gitStatus, _ := created["git_status"].(map[string]any)
	if gitStatus["ref"] != "feature" {
		t.Fatalf("pull request codespace ref = %v, want feature", gitStatus["ref"])
	}
	repoJSON, _ := created["repository"].(map[string]any)
	if repoJSON == nil || repoJSON["full_name"] != repo.FullName {
		t.Fatalf("pull request codespace repository = %v", created["repository"])
	}

	// Unknown pull request numbers are 404.
	missing := ghPost(t, "/api/v3/repos/"+repo.FullName+"/pulls/9999/codespaces", defaultToken, nil)
	missing.Body.Close()
	if missing.StatusCode != http.StatusNotFound {
		t.Fatalf("codespace for unknown pull request: %d, want 404", missing.StatusCode)
	}
}
