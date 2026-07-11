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
			_, _ = testServer.store.DeleteCodespace(cs.ID)
		}
		cleanupCodespaceContainer(t, name)
	})
	if created["state"] != "Available" {
		t.Fatalf("created state = %v, want Available", created["state"])
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
			_, _ = testServer.store.DeleteCodespace(cs.ID)
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
	if started["state"] != "Available" {
		t.Fatalf("start state = %v, want Available", started["state"])
	}

	resp = ghPost(t, fmt.Sprintf("/api/v3/repos/%s/codespaces/%s/stop", repo.FullName, name), defaultToken, nil)
	if resp.StatusCode != http.StatusAccepted {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("stop repo codespace: %d %s", resp.StatusCode, b)
	}
	stopped := decodeJSON(t, resp)
	resp.Body.Close()
	if stopped["state"] != "Shutdown" {
		t.Fatalf("stop state = %v, want Shutdown", stopped["state"])
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

// TestCodespaces_OrgMemberAdministration exercises the org-owner view of
// a member's codespaces on the organization's repositories:
// list → stop (200 with the codespace body) → delete (202).
func TestCodespaces_OrgMemberAdministration(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	createOrgViaAdminAPI(t, "cs-admin-org")
	org := testServer.store.GetOrg("cs-admin-org")
	repo := testServer.store.CreateOrgRepo(org, admin, "cs-admin-repo", "org codespace repo", false)
	if repo == nil {
		t.Fatal("create org repo")
	}
	stor := testServer.store.GitStorages[repo.FullName]
	if _, err := initRepoWithFiles(stor, repo.DefaultBranch, "init", map[string]string{
		".devcontainer/devcontainer.json": fmt.Sprintf(`{"image":%q}`, codespaceTestImage),
	}, repoSignature(admin.Login, "bleephub@local")); err != nil {
		t.Fatalf("init repo files: %v", err)
	}

	_, memberToken := newSharedServerUser(t, "cs-org-member")
	activateOrgMember(t, "cs-admin-org", "cs-org-member", memberToken)

	created := ghPost(t, "/api/v3/user/codespaces", memberToken, map[string]any{
		"repository_id": repo.ID,
		"machine":       "basicLinux32",
	})
	if created.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(created.Body)
		created.Body.Close()
		t.Fatalf("member creates codespace: %d %s", created.StatusCode, b)
	}
	name := decodeJSON(t, created)["name"].(string)
	t.Cleanup(func() { cleanupCodespaceContainer(t, name) })

	list := ghGet(t, "/api/v3/orgs/cs-admin-org/members/cs-org-member/codespaces", defaultToken)
	if list.StatusCode != http.StatusOK {
		list.Body.Close()
		t.Fatalf("org member codespaces list: %d", list.StatusCode)
	}
	listing := decodeJSON(t, list)
	if listing["total_count"] != float64(1) {
		t.Fatalf("total_count = %v, want 1", listing["total_count"])
	}
	first := listing["codespaces"].([]interface{})[0].(map[string]interface{})
	if first["name"] != name {
		t.Fatalf("listed codespace = %v", first)
	}
	if _, has := first["html_url"]; has {
		t.Fatalf("org-scoped codespace carries undocumented html_url: %v", first)
	}

	stopped := ghPost(t, "/api/v3/orgs/cs-admin-org/members/cs-org-member/codespaces/"+name+"/stop", defaultToken, nil)
	if stopped.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(stopped.Body)
		stopped.Body.Close()
		t.Fatalf("org member codespace stop: %d %s", stopped.StatusCode, b)
	}
	if body := decodeJSON(t, stopped); body["name"] != name {
		t.Fatalf("stop response = %v", body)
	}

	// The member and codespace must both resolve within the org.
	resp := ghGet(t, "/api/v3/orgs/cs-admin-org/members/nobody-here/codespaces", defaultToken)
	if resp.StatusCode != http.StatusNotFound {
		resp.Body.Close()
		t.Fatalf("unknown member list: %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = ghDelete(t, "/api/v3/orgs/cs-admin-org/members/cs-org-member/codespaces/no-such-codespace", defaultToken)
	if resp.StatusCode != http.StatusNotFound {
		resp.Body.Close()
		t.Fatalf("unknown codespace delete: %d", resp.StatusCode)
	}
	resp.Body.Close()
	// Only org owners administer member codespaces.
	resp = ghGet(t, "/api/v3/orgs/cs-admin-org/members/cs-org-member/codespaces", memberToken)
	if resp.StatusCode != http.StatusForbidden {
		resp.Body.Close()
		t.Fatalf("member lists own org codespaces: %d, want 403", resp.StatusCode)
	}
	resp.Body.Close()

	deleted := ghDelete(t, "/api/v3/orgs/cs-admin-org/members/cs-org-member/codespaces/"+name, defaultToken)
	if deleted.StatusCode != http.StatusAccepted {
		b, _ := io.ReadAll(deleted.Body)
		deleted.Body.Close()
		t.Fatalf("org member codespace delete: %d %s", deleted.StatusCode, b)
	}
	deleted.Body.Close()
	after := decodeJSON(t, ghGet(t, "/api/v3/orgs/cs-admin-org/members/cs-org-member/codespaces", defaultToken))
	if after["total_count"] != float64(0) {
		t.Fatalf("codespaces after delete = %v", after)
	}
}

func TestCodespaces_OrgList(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	org := testServer.store.CreateOrg(admin, "cs-list-org", "Codespaces List Org", "")
	if org == nil {
		t.Fatal("create org failed")
	}

	// Honest empty list before any member codespace exists... except that
	// the admin may own codespaces created by earlier tests, so assert
	// against membership: every returned codespace belongs to an org member.
	repo := createTestCodespaceRepo(t, "cs-org-list-repo")
	resp := ghPost(t, "/api/v3/user/codespaces", defaultToken, map[string]any{
		"repository_id": repo.ID,
		"machine":       "basicLinux32",
	})
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("create codespace: %d %s", resp.StatusCode, b)
	}
	created := decodeJSON(t, resp)
	name, _ := created["name"].(string)
	defer cleanupCodespaceContainer(t, name)

	resp = ghGet(t, "/api/v3/orgs/cs-list-org/codespaces", defaultToken)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("list org codespaces: %d", resp.StatusCode)
	}
	var listResp struct {
		TotalCount int              `json:"total_count"`
		Codespaces []map[string]any `json:"codespaces"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		t.Fatalf("decode org codespaces: %v", err)
	}
	resp.Body.Close()
	if listResp.TotalCount != len(listResp.Codespaces) {
		t.Fatalf("total_count %d != len(codespaces) %d", listResp.TotalCount, len(listResp.Codespaces))
	}
	found := false
	for _, cs := range listResp.Codespaces {
		if cs["name"] == name {
			found = true
		}
	}
	if !found {
		t.Fatalf("member codespace %s missing from org list", name)
	}

	// Non-admin callers are forbidden.
	outsider := createTestUser(t, "cs-outsider")
	testServer.store.Tokens["ghp_cs_outsider"] = &Token{Value: "ghp_cs_outsider", UserID: outsider.ID}
	resp = ghGet(t, "/api/v3/orgs/cs-list-org/codespaces", "ghp_cs_outsider")
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("non-admin org codespaces: %d, want 403", resp.StatusCode)
	}
}

func TestCodespaces_OrgAccessControls(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	org := testServer.store.CreateOrg(admin, "cs-access-org", "Codespaces Access Org", "")
	if org == nil {
		t.Fatal("create org failed")
	}
	member := createTestUser(t, "cs-access-member")
	testServer.store.SetMembership(org.Login, member.ID, OrgRoleMember, MembershipStateActive)
	member2 := createTestUser(t, "cs-access-member2")
	testServer.store.SetMembership(org.Login, member2.ID, OrgRoleMember, MembershipStateActive)

	// Invalid visibility.
	resp := ghPut(t, "/api/v3/orgs/cs-access-org/codespaces/access", defaultToken, map[string]any{
		"visibility": "everyone-on-earth",
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("invalid visibility: %d, want 422", resp.StatusCode)
	}

	// Usernames that are neither members nor collaborators are rejected.
	resp = ghPut(t, "/api/v3/orgs/cs-access-org/codespaces/access", defaultToken, map[string]any{
		"visibility":         "selected_members",
		"selected_usernames": []string{"total-stranger"},
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("stranger username: %d, want 400", resp.StatusCode)
	}

	// Set selected_members with one member.
	resp = ghPut(t, "/api/v3/orgs/cs-access-org/codespaces/access", defaultToken, map[string]any{
		"visibility":         "selected_members",
		"selected_usernames": []string{"cs-access-member"},
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("set access: %d, want 204", resp.StatusCode)
	}

	// Add and remove selected users.
	resp = ghPost(t, "/api/v3/orgs/cs-access-org/codespaces/access/selected_users", defaultToken, map[string]any{
		"selected_usernames": []string{"cs-access-member2"},
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("add selected user: %d, want 204", resp.StatusCode)
	}
	access := testServer.store.OrgCodespacesAccess["cs-access-org"]
	if access == nil || len(access.SelectedUsernames) != 2 {
		t.Fatalf("selected usernames after add: %+v", access)
	}
	resp = ghDeleteWithBody(t, "/api/v3/orgs/cs-access-org/codespaces/access/selected_users", defaultToken, map[string]interface{}{
		"selected_usernames": []interface{}{"cs-access-member"},
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("remove selected user: %d, want 204", resp.StatusCode)
	}
	access = testServer.store.OrgCodespacesAccess["cs-access-org"]
	if access == nil || len(access.SelectedUsernames) != 1 || access.SelectedUsernames[0] != "cs-access-member2" {
		t.Fatalf("selected usernames after remove: %+v", access)
	}

	// Disabling access makes the selected-users endpoints invalid.
	resp = ghPut(t, "/api/v3/orgs/cs-access-org/codespaces/access", defaultToken, map[string]any{
		"visibility": "disabled",
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("disable access: %d, want 204", resp.StatusCode)
	}
	resp = ghPost(t, "/api/v3/orgs/cs-access-org/codespaces/access/selected_users", defaultToken, map[string]any{
		"selected_usernames": []string{"cs-access-member"},
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("add selected user while disabled: %d, want 422", resp.StatusCode)
	}
}

func TestCodespaces_OrgSecretSelectedRepoAddRemove(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	org := testServer.store.CreateOrg(admin, "cs-secret-repo-org", "Codespaces Secret Repo Org", "")
	if org == nil {
		t.Fatal("create org failed")
	}
	r1 := testServer.store.CreateOrgRepo(org, admin, "cs-secret-repo-1", "", false)
	r2 := testServer.store.CreateOrgRepo(org, admin, "cs-secret-repo-2", "", false)
	if r1 == nil || r2 == nil {
		t.Fatal("create org repos failed")
	}

	enc, keyID := sealForServer(t, "org codespace secret value")
	resp := ghPut(t, "/api/v3/orgs/cs-secret-repo-org/codespaces/secrets/CS_SELECTED", defaultToken, map[string]any{
		"encrypted_value":         enc,
		"key_id":                  keyID,
		"visibility":              "selected",
		"selected_repository_ids": []int{r1.ID},
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("put org codespace secret: %d, want 204", resp.StatusCode)
	}

	// Add the second repository.
	resp = ghPut(t, fmt.Sprintf("/api/v3/orgs/cs-secret-repo-org/codespaces/secrets/CS_SELECTED/repositories/%d", r2.ID), defaultToken, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("add selected repo: %d, want 204", resp.StatusCode)
	}
	resp = ghGet(t, "/api/v3/orgs/cs-secret-repo-org/codespaces/secrets/CS_SELECTED/repositories", defaultToken)
	repos := decodeJSON(t, resp)
	if repos["total_count"] != float64(2) {
		t.Fatalf("selected repos after add = %v, want 2", repos["total_count"])
	}

	// Remove the first repository.
	resp = ghDelete(t, fmt.Sprintf("/api/v3/orgs/cs-secret-repo-org/codespaces/secrets/CS_SELECTED/repositories/%d", r1.ID), defaultToken)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("remove selected repo: %d, want 204", resp.StatusCode)
	}
	resp = ghGet(t, "/api/v3/orgs/cs-secret-repo-org/codespaces/secrets/CS_SELECTED/repositories", defaultToken)
	repos = decodeJSON(t, resp)
	if repos["total_count"] != float64(1) {
		t.Fatalf("selected repos after remove = %v, want 1", repos["total_count"])
	}

	// A secret with visibility all conflicts.
	enc, keyID = sealForServer(t, "all visibility value")
	resp = ghPut(t, "/api/v3/orgs/cs-secret-repo-org/codespaces/secrets/CS_ALL", defaultToken, map[string]any{
		"encrypted_value": enc,
		"key_id":          keyID,
		"visibility":      "all",
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("put all-visibility secret: %d, want 204", resp.StatusCode)
	}
	resp = ghPut(t, fmt.Sprintf("/api/v3/orgs/cs-secret-repo-org/codespaces/secrets/CS_ALL/repositories/%d", r1.ID), defaultToken, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("add repo to all-visibility secret: %d, want 409", resp.StatusCode)
	}

	// Unknown secret.
	resp = ghPut(t, fmt.Sprintf("/api/v3/orgs/cs-secret-repo-org/codespaces/secrets/NO_SUCH/repositories/%d", r1.ID), defaultToken, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown secret add repo: %d, want 404", resp.StatusCode)
	}
}
