package bleephub

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"
)

// --- GitHub Copilot coding agent repository secrets ---

func TestAgentsRepoSecrets_RoundTrip(t *testing.T) {
	repo := seedTestRepo(t, "agents-sec-repo", false)
	base := "/api/v3/repos/" + repo.FullName + "/agents/secrets"

	// Public key matches the shared Actions sealed-box keypair.
	resp := ghGet(t, base+"/public-key", defaultToken)
	if resp.StatusCode != 200 {
		t.Fatalf("public-key status %d, want 200", resp.StatusCode)
	}
	body := decodeJSON(t, resp)
	kp, err := testServer.store.ActionsKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	if body["key_id"] != kp.KeyID || body["key"] != kp.PublicKey {
		t.Fatalf("public-key = %v, want key_id=%s", body, kp.KeyID)
	}

	// Create.
	mustStatus(t, putSealedSecret(t, base+"/AGENT_TOKEN", "plain-1"), 201, "create secret")

	// List.
	resp = ghGet(t, base, defaultToken)
	if resp.StatusCode != 200 {
		t.Fatalf("list status %d, want 200", resp.StatusCode)
	}
	list := decodeJSON(t, resp)
	if list["total_count"] != float64(1) {
		t.Fatalf("total_count = %v, want 1", list["total_count"])
	}
	secrets := list["secrets"].([]interface{})
	if secrets[0].(map[string]interface{})["name"] != "AGENT_TOKEN" {
		t.Fatalf("secrets[0] = %v, want AGENT_TOKEN", secrets[0])
	}

	// Get.
	resp = ghGet(t, base+"/AGENT_TOKEN", defaultToken)
	if resp.StatusCode != 200 {
		t.Fatalf("get status %d, want 200", resp.StatusCode)
	}
	got := decodeJSON(t, resp)
	if got["name"] != "AGENT_TOKEN" {
		t.Fatalf("get name = %v, want AGENT_TOKEN", got["name"])
	}

	// Update.
	mustStatus(t, putSealedSecret(t, base+"/AGENT_TOKEN", "plain-2"), 204, "update secret")

	// Delete.
	mustStatus(t, ghDelete(t, base+"/AGENT_TOKEN", defaultToken), 204, "delete secret")
	mustStatus(t, ghGet(t, base+"/AGENT_TOKEN", defaultToken), 404, "get deleted secret")
	mustStatus(t, ghDelete(t, base+"/AGENT_TOKEN", defaultToken), 404, "delete deleted secret")
}

func TestAgentsRepoSecrets_Validation(t *testing.T) {
	repo := seedTestRepo(t, "agents-sec-valid", false)
	base := "/api/v3/repos/" + repo.FullName + "/agents/secrets"

	// Invalid name (starts with a digit).
	mustStatus(t, putSealedSecret(t, base+"/1BAD", "v"), 422, "invalid secret name")

	// Wrong key_id.
	resp := ghPut(t, base+"/GOOD_NAME", defaultToken, map[string]interface{}{
		"encrypted_value": "c2VjcmV0",
		"key_id":          "not-the-key",
	})
	mustStatus(t, resp, 422, "wrong key_id")

	// Unknown repository.
	mustStatus(t, ghGet(t, "/api/v3/repos/admin/no-such-repo/agents/secrets", defaultToken), 404, "unknown repo list")
}

// --- GitHub Copilot coding agent organization secrets ---

func TestAgentsOrgSecrets_VisibilityAndSelectedRepos(t *testing.T) {
	org := seedTestOrg(t, "agents-sec-org")
	repo1 := seedOrgRepo(t, org, "sel-one", true)
	repo2 := seedOrgRepo(t, org, "sel-two", true)
	base := "/api/v3/orgs/" + org.Login + "/agents/secrets"

	mustStatus(t, ghGet(t, base+"/public-key", defaultToken), 200, "org public-key")

	// Create with visibility selected.
	enc, keyID := sealForServer(t, "org-plain")
	resp := ghPut(t, base+"/ORG_AGENT_SECRET", defaultToken, map[string]interface{}{
		"encrypted_value":         enc,
		"key_id":                  keyID,
		"visibility":              "selected",
		"selected_repository_ids": []int{repo1.ID},
	})
	mustStatus(t, resp, 201, "create org secret")

	// Get carries visibility + the /agents/ selected_repositories_url.
	resp = ghGet(t, base+"/ORG_AGENT_SECRET", defaultToken)
	if resp.StatusCode != 200 {
		t.Fatalf("get org secret status %d, want 200", resp.StatusCode)
	}
	got := decodeJSON(t, resp)
	if got["visibility"] != "selected" {
		t.Fatalf("visibility = %v, want selected", got["visibility"])
	}
	wantURL := fmt.Sprintf("/api/v3/orgs/%s/agents/secrets/ORG_AGENT_SECRET/repositories", org.Login)
	if url, _ := got["selected_repositories_url"].(string); url == "" || url[len(url)-len(wantURL):] != wantURL {
		t.Fatalf("selected_repositories_url = %v, want suffix %s", got["selected_repositories_url"], wantURL)
	}

	// List selected repositories.
	resp = ghGet(t, base+"/ORG_AGENT_SECRET/repositories", defaultToken)
	if resp.StatusCode != 200 {
		t.Fatalf("list selected repos status %d, want 200", resp.StatusCode)
	}
	repos := decodeJSON(t, resp)
	if repos["total_count"] != float64(1) {
		t.Fatalf("selected total_count = %v, want 1", repos["total_count"])
	}

	// Set the full list.
	resp = ghPut(t, base+"/ORG_AGENT_SECRET/repositories", defaultToken, map[string]interface{}{
		"selected_repository_ids": []int{repo1.ID, repo2.ID},
	})
	mustStatus(t, resp, 204, "set selected repos")

	// Remove one, then add it back.
	mustStatus(t, ghDelete(t, fmt.Sprintf("%s/ORG_AGENT_SECRET/repositories/%d", base, repo2.ID), defaultToken), 204, "remove selected repo")
	mustStatus(t, ghPut(t, fmt.Sprintf("%s/ORG_AGENT_SECRET/repositories/%d", base, repo2.ID), defaultToken, nil), 204, "add selected repo")

	// Repo-visible org secrets list.
	resp = ghGet(t, "/api/v3/repos/"+repo1.FullName+"/agents/organization-secrets", defaultToken)
	if resp.StatusCode != 200 {
		t.Fatalf("repo org-secrets status %d, want 200", resp.StatusCode)
	}
	visible := decodeJSON(t, resp)
	if visible["total_count"] != float64(1) {
		t.Fatalf("repo-visible org secrets = %v, want 1", visible["total_count"])
	}

	// A repo outside the selection sees nothing.
	repo3 := seedOrgRepo(t, org, "sel-three", true)
	resp = ghGet(t, "/api/v3/repos/"+repo3.FullName+"/agents/organization-secrets", defaultToken)
	notVisible := decodeJSON(t, resp)
	if notVisible["total_count"] != float64(0) {
		t.Fatalf("unselected repo sees %v org secrets, want 0", notVisible["total_count"])
	}

	// Per-repo add is a 409 when visibility is not "selected".
	enc, keyID = sealForServer(t, "all-plain")
	mustStatus(t, ghPut(t, base+"/ORG_ALL_SECRET", defaultToken, map[string]interface{}{
		"encrypted_value": enc, "key_id": keyID, "visibility": "all",
	}), 201, "create all-visibility secret")
	mustStatus(t, ghPut(t, fmt.Sprintf("%s/ORG_ALL_SECRET/repositories/%d", base, repo1.ID), defaultToken, nil), 409, "add repo to all-visibility secret")

	// Missing visibility is a 422.
	enc, keyID = sealForServer(t, "no-vis")
	mustStatus(t, ghPut(t, base+"/ORG_NO_VIS", defaultToken, map[string]interface{}{
		"encrypted_value": enc, "key_id": keyID,
	}), 422, "create without visibility")

	// Delete.
	mustStatus(t, ghDelete(t, base+"/ORG_AGENT_SECRET", defaultToken), 204, "delete org secret")
	mustStatus(t, ghGet(t, base+"/ORG_AGENT_SECRET", defaultToken), 404, "get deleted org secret")
}

// --- GitHub Copilot coding agent repository variables ---

func TestAgentsRepoVariables_CRUD(t *testing.T) {
	repo := seedTestRepo(t, "agents-var-repo", false)
	base := "/api/v3/repos/" + repo.FullName + "/agents/variables"

	// Create.
	mustStatus(t, ghPost(t, base, defaultToken, map[string]interface{}{
		"name": "agent_mode", "value": "fast",
	}), 201, "create variable")

	// Duplicate create conflicts.
	mustStatus(t, ghPost(t, base, defaultToken, map[string]interface{}{
		"name": "AGENT_MODE", "value": "again",
	}), 409, "duplicate create")

	// Invalid name.
	mustStatus(t, ghPost(t, base, defaultToken, map[string]interface{}{
		"name": "GITHUB_RESERVED", "value": "x",
	}), 422, "reserved name")

	// List.
	resp := ghGet(t, base, defaultToken)
	if resp.StatusCode != 200 {
		t.Fatalf("list status %d, want 200", resp.StatusCode)
	}
	list := decodeJSON(t, resp)
	if list["total_count"] != float64(1) {
		t.Fatalf("total_count = %v, want 1", list["total_count"])
	}

	// Get (names are upper-cased).
	resp = ghGet(t, base+"/AGENT_MODE", defaultToken)
	if resp.StatusCode != 200 {
		t.Fatalf("get status %d, want 200", resp.StatusCode)
	}
	got := decodeJSON(t, resp)
	if got["value"] != "fast" {
		t.Fatalf("value = %v, want fast", got["value"])
	}

	// Patch value + rename.
	mustStatus(t, ghPatch(t, base+"/AGENT_MODE", defaultToken, map[string]interface{}{
		"name": "AGENT_SPEED", "value": "slow",
	}), 204, "patch variable")
	resp = ghGet(t, base+"/AGENT_SPEED", defaultToken)
	if resp.StatusCode != 200 {
		t.Fatalf("get renamed status %d, want 200", resp.StatusCode)
	}
	got = decodeJSON(t, resp)
	if got["value"] != "slow" {
		t.Fatalf("patched value = %v, want slow", got["value"])
	}
	mustStatus(t, ghGet(t, base+"/AGENT_MODE", defaultToken), 404, "old name gone")

	// Delete.
	mustStatus(t, ghDelete(t, base+"/AGENT_SPEED", defaultToken), 204, "delete variable")
	mustStatus(t, ghGet(t, base+"/AGENT_SPEED", defaultToken), 404, "get deleted variable")
}

// --- GitHub Copilot coding agent organization variables ---

func TestAgentsOrgVariables_CRUDAndSelectedRepos(t *testing.T) {
	org := seedTestOrg(t, "agents-var-org")
	repo1 := seedOrgRepo(t, org, "var-one", true)
	repo2 := seedOrgRepo(t, org, "var-two", true)
	base := "/api/v3/orgs/" + org.Login + "/agents/variables"

	// Create with visibility selected.
	mustStatus(t, ghPost(t, base, defaultToken, map[string]interface{}{
		"name": "ORG_AGENT_VAR", "value": "v1",
		"visibility":              "selected",
		"selected_repository_ids": []int{repo1.ID},
	}), 201, "create org variable")

	// Missing visibility is a 422.
	mustStatus(t, ghPost(t, base, defaultToken, map[string]interface{}{
		"name": "NO_VIS", "value": "v",
	}), 422, "create without visibility")

	// Get.
	resp := ghGet(t, base+"/ORG_AGENT_VAR", defaultToken)
	if resp.StatusCode != 200 {
		t.Fatalf("get org variable status %d, want 200", resp.StatusCode)
	}
	got := decodeJSON(t, resp)
	if got["visibility"] != "selected" || got["value"] != "v1" {
		t.Fatalf("org variable = %v, want selected/v1", got)
	}

	// Selected repositories list + set + per-repo add/remove.
	resp = ghGet(t, base+"/ORG_AGENT_VAR/repositories", defaultToken)
	if resp.StatusCode != 200 {
		t.Fatalf("list selected repos status %d, want 200", resp.StatusCode)
	}
	repos := decodeJSON(t, resp)
	if repos["total_count"] != float64(1) {
		t.Fatalf("selected total_count = %v, want 1", repos["total_count"])
	}
	mustStatus(t, ghPut(t, base+"/ORG_AGENT_VAR/repositories", defaultToken, map[string]interface{}{
		"selected_repository_ids": []int{repo1.ID, repo2.ID},
	}), 204, "set selected repos")
	mustStatus(t, ghDelete(t, fmt.Sprintf("%s/ORG_AGENT_VAR/repositories/%d", base, repo1.ID), defaultToken), 204, "remove selected repo")
	mustStatus(t, ghPut(t, fmt.Sprintf("%s/ORG_AGENT_VAR/repositories/%d", base, repo1.ID), defaultToken, nil), 204, "add selected repo")

	// Patch visibility to all: selection endpoints now conflict.
	mustStatus(t, ghPatch(t, base+"/ORG_AGENT_VAR", defaultToken, map[string]interface{}{
		"visibility": "all",
	}), 204, "patch visibility")
	mustStatus(t, ghGet(t, base+"/ORG_AGENT_VAR/repositories", defaultToken), 409, "list repos on all-visibility variable")
	mustStatus(t, ghPut(t, base+"/ORG_AGENT_VAR/repositories", defaultToken, map[string]interface{}{
		"selected_repository_ids": []int{repo1.ID},
	}), 409, "set repos on all-visibility variable")

	// Repo-visible org variables list (visibility all → visible everywhere).
	resp = ghGet(t, "/api/v3/repos/"+repo2.FullName+"/agents/organization-variables", defaultToken)
	if resp.StatusCode != 200 {
		t.Fatalf("repo org-variables status %d, want 200", resp.StatusCode)
	}
	visible := decodeJSON(t, resp)
	if visible["total_count"] != float64(1) {
		t.Fatalf("repo-visible org variables = %v, want 1", visible["total_count"])
	}

	// List org variables.
	resp = ghGet(t, base, defaultToken)
	if resp.StatusCode != 200 {
		t.Fatalf("list org variables status %d, want 200", resp.StatusCode)
	}
	list := decodeJSON(t, resp)
	if list["total_count"] != float64(1) {
		t.Fatalf("org variables total_count = %v, want 1", list["total_count"])
	}

	// Delete.
	mustStatus(t, ghDelete(t, base+"/ORG_AGENT_VAR", defaultToken), 204, "delete org variable")
	mustStatus(t, ghGet(t, base+"/ORG_AGENT_VAR", defaultToken), 404, "get deleted org variable")
}

// TestAgentsSecrets_IsolatedFromActions verifies the /agents/ tables are
// distinct from the Actions ones: an Actions secret does not appear on
// the agents surface and vice versa.
func TestAgentsSecrets_IsolatedFromActions(t *testing.T) {
	repo := seedTestRepo(t, "agents-isolation", false)

	mustStatus(t, putSealedSecret(t, "/api/v3/repos/"+repo.FullName+"/actions/secrets/ACTIONS_ONLY", "a"), 201, "create actions secret")
	mustStatus(t, putSealedSecret(t, "/api/v3/repos/"+repo.FullName+"/agents/secrets/AGENTS_ONLY", "b"), 201, "create agents secret")

	resp := ghGet(t, "/api/v3/repos/"+repo.FullName+"/agents/secrets", defaultToken)
	list := decodeJSON(t, resp)
	secrets := list["secrets"].([]interface{})
	if len(secrets) != 1 || secrets[0].(map[string]interface{})["name"] != "AGENTS_ONLY" {
		t.Fatalf("agents secrets = %v, want only AGENTS_ONLY", secrets)
	}

	mustStatus(t, ghGet(t, "/api/v3/repos/"+repo.FullName+"/actions/secrets/AGENTS_ONLY", defaultToken), 404, "agents secret invisible to actions surface")
	mustStatus(t, ghGet(t, "/api/v3/repos/"+repo.FullName+"/agents/secrets/ACTIONS_ONLY", defaultToken), 404, "actions secret invisible to agents surface")
}

// TestAgentsCodeScanPersistenceReload verifies every new bucket —
// Copilot coding agent secrets/variables/tasks, code scanning autofixes,
// CodeQL databases, and CodeQL variant analyses — survives a persistence
// reload with counters intact.
func TestAgentsCodeScanPersistenceReload(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BLEEPHUB_PERSIST", "true")
	t.Setenv("BLEEPHUB_DATA_DIR", dir)

	// --- session 1: create state, then close ---
	p1, err := NewPersistence()
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	st1 := NewStore()
	if err := st1.SetPersistence(p1); err != nil {
		t.Fatalf("SetPersistence: %v", err)
	}
	objectFS, objectStore := newObjectByteStoreForTest(t)
	st1.ObjectByteStore = objectStore
	st1.SeedDefaultUser()
	user := st1.UsersByLogin["admin"]
	repo := st1.CreateRepo(user, "agents-reload", "", false)
	if repo == nil {
		t.Fatal("CreateRepo returned nil")
	}

	srv1 := &Server{store: st1}
	srv1.upsertSecret(st1.AgentsRepoSecrets, "agents_repo_secrets", repo.FullName, "RELOAD_SECRET", "plain-value")
	now := time.Now().UTC()
	tbl := agentsVariableTable{srv1, "agents_repo_variables", repo.FullName}
	if !tbl.create(&ActionsVariable{Name: "RELOAD_VAR", Value: "vv", CreatedAt: now, UpdatedAt: now}) {
		t.Fatal("create agents variable failed")
	}

	task := st1.CreateAgentTask(repo, user, "reload prompt", "claude-sonnet-4.6", false, "", "")

	alert := st1.CreateCodeScanningAlert(repo.FullName, "reload-rule", "error", "d", "CodeQL", "open",
		[]CodeScanningAlertInstance{{Ref: "refs/heads/main", Path: "f.go", StartLine: 1, State: "open"}})
	if _, created := st1.CreateCodeScanningAutofix(alert); !created {
		t.Fatal("autofix not created")
	}

	db, err := st1.UpsertCodeQLDatabase(repo.FullName, "go", "database.zip", "application/zip", "reload-sha", []byte("db-bytes"), user.ID)
	if err != nil {
		t.Fatalf("UpsertCodeQLDatabase: %v", err)
	}
	if got := string(readS3TestFile(t, objectFS, db.StoragePath)); got != "db-bytes" {
		t.Fatalf("CodeQL database object bytes = %q, want db-bytes", got)
	}
	va, err := st1.CreateCodeQLVariantAnalysis(repo.FullName, user.ID, "go", []byte("pack"), []string{repo.FullName})
	if err != nil {
		t.Fatalf("CreateCodeQLVariantAnalysis: %v", err)
	}
	if got := string(readS3TestFile(t, objectFS, va.StoragePath)); got != "pack" {
		t.Fatalf("CodeQL variant-analysis query-pack object bytes = %q, want pack", got)
	}

	if err := p1.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// --- session 2: reload, assert everything came back ---
	p2, err := NewPersistence()
	if err != nil {
		t.Fatalf("re-open: %v", err)
	}
	st2 := NewStore()
	st2.ObjectByteStore = objectStore
	if err := st2.SetPersistence(p2); err != nil {
		t.Fatalf("re-load SetPersistence: %v", err)
	}
	defer p2.Close()

	if sec := st2.AgentsRepoSecrets[repo.FullName]["RELOAD_SECRET"]; sec == nil || sec.Value != "plain-value" {
		t.Fatalf("agents repo secret after reload = %v, want plain-value", sec)
	}
	if v := st2.AgentsRepoVariables[repo.FullName]["RELOAD_VAR"]; v == nil || v.Value != "vv" {
		t.Fatalf("agents repo variable after reload = %v, want vv", v)
	}
	gotTask := st2.GetAgentTask(task.ID)
	if gotTask == nil || gotTask.Prompt != "reload prompt" || len(gotTask.Sessions) != 1 {
		t.Fatalf("agent task after reload = %+v, want prompt + 1 session", gotTask)
	}
	if fix := st2.GetCodeScanningAutofix(repo.FullName, alert.Number); fix == nil || fix.Status != "success" {
		t.Fatalf("autofix after reload = %+v, want success", fix)
	}
	gotDB := st2.GetCodeQLDatabase(repo.FullName, "go")
	if gotDB == nil || gotDB.CommitOID != "reload-sha" {
		t.Fatalf("CodeQL database after reload = %+v", gotDB)
	}
	gotBytes, err := st2.ReadCodeQLDatabaseContent(context.Background(), gotDB)
	if err != nil {
		t.Fatalf("ReadCodeQLDatabaseContent: %v", err)
	}
	if !bytes.Equal(gotBytes, []byte("db-bytes")) {
		t.Fatalf("CodeQL database bytes after reload = %q, want db-bytes", gotBytes)
	}
	if st2.NextCodeQLDatabaseID != db.ID+1 {
		t.Fatalf("NextCodeQLDatabaseID = %d, want %d", st2.NextCodeQLDatabaseID, db.ID+1)
	}
	gotVA := st2.GetCodeQLVariantAnalysis(repo.FullName, va.ID)
	if gotVA == nil || gotVA.Status != "succeeded" || len(gotVA.ScannedRepositories) != 1 {
		t.Fatalf("variant analysis after reload = %+v, want succeeded with 1 scanned repo", gotVA)
	}
	if st2.NextCodeQLVariantAnalysisID != va.ID+1 {
		t.Fatalf("NextCodeQLVariantAnalysisID = %d, want %d", st2.NextCodeQLVariantAnalysisID, va.ID+1)
	}
}
