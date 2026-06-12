package bleephub

import (
	"fmt"
	"testing"
	"time"
)

// --- repository variables ---

func TestRepoVariablesCRUD(t *testing.T) {
	repo := seedTestRepo(t, "var-crud", false)
	base := "/api/v3/repos/" + repo.FullName + "/actions/variables"

	// Create (lowercase input → stored/returned uppercase, real-API rule).
	mustStatus(t, ghPost(t, base, defaultToken, map[string]interface{}{
		"name": "deploy_env", "value": "staging",
	}), 201, "create")

	// Duplicate create conflicts — gh CLI's `gh variable set` POSTs first
	// and PATCHes on exactly this 409.
	mustStatus(t, ghPost(t, base, defaultToken, map[string]interface{}{
		"name": "DEPLOY_ENV", "value": "other",
	}), 409, "duplicate create")

	// Get.
	v := decodeJSON(t, ghGet(t, base+"/deploy_env", defaultToken))
	if v["name"] != "DEPLOY_ENV" || v["value"] != "staging" {
		t.Fatalf("get = %v", v)
	}
	if _, err := time.Parse(time.RFC3339, v["created_at"].(string)); err != nil {
		t.Errorf("created_at not RFC3339: %v", v["created_at"])
	}

	// List.
	list := decodeJSON(t, ghGet(t, base, defaultToken))
	if int(list["total_count"].(float64)) != 1 {
		t.Fatalf("total_count = %v, want 1", list["total_count"])
	}

	// Patch value.
	mustStatus(t, ghPatch(t, base+"/DEPLOY_ENV", defaultToken, map[string]interface{}{
		"value": "production",
	}), 204, "patch value")
	v = decodeJSON(t, ghGet(t, base+"/DEPLOY_ENV", defaultToken))
	if v["value"] != "production" {
		t.Fatalf("after patch value = %v", v["value"])
	}

	// Patch rename.
	mustStatus(t, ghPatch(t, base+"/DEPLOY_ENV", defaultToken, map[string]interface{}{
		"name": "TARGET_ENV",
	}), 204, "patch rename")
	mustStatus(t, ghGet(t, base+"/DEPLOY_ENV", defaultToken), 404, "old name gone")
	v = decodeJSON(t, ghGet(t, base+"/TARGET_ENV", defaultToken))
	if v["value"] != "production" {
		t.Fatalf("renamed variable lost value: %v", v)
	}

	// Rename collision.
	mustStatus(t, ghPost(t, base, defaultToken, map[string]interface{}{
		"name": "OTHER_VAR", "value": "x",
	}), 201, "create other")
	mustStatus(t, ghPatch(t, base+"/OTHER_VAR", defaultToken, map[string]interface{}{
		"name": "TARGET_ENV",
	}), 409, "rename collision")

	// Delete.
	mustStatus(t, ghDelete(t, base+"/TARGET_ENV", defaultToken), 204, "delete")
	mustStatus(t, ghGet(t, base+"/TARGET_ENV", defaultToken), 404, "get after delete")
	mustStatus(t, ghDelete(t, base+"/TARGET_ENV", defaultToken), 404, "delete again")
	mustStatus(t, ghPatch(t, base+"/TARGET_ENV", defaultToken, map[string]interface{}{
		"value": "x",
	}), 404, "patch missing")
}

func TestRepoVariablesBadNames422(t *testing.T) {
	repo := seedTestRepo(t, "var-names", false)
	base := "/api/v3/repos/" + repo.FullName + "/actions/variables"
	for _, name := range []string{"", "9LIVES", "WITH-DASH", "GITHUB_VAR", "github_var"} {
		resp := ghPost(t, base, defaultToken, map[string]interface{}{"name": name, "value": "v"})
		mustStatus(t, resp, 422, "bad name "+name)
	}
}

func TestRepoVariablesMissingRepo404(t *testing.T) {
	mustStatus(t, ghGet(t, "/api/v3/repos/admin/no-such-repo/actions/variables", defaultToken), 404, "list")
	mustStatus(t, ghPost(t, "/api/v3/repos/admin/no-such-repo/actions/variables", defaultToken,
		map[string]interface{}{"name": "X", "value": "v"}), 404, "create")
}

// --- organization variables ---

func TestOrgVariablesLifecycle(t *testing.T) {
	org := seedTestOrg(t, "varorg-life")
	selRepo := seedOrgRepo(t, org, "var-sel-target", false)
	base := "/api/v3/orgs/" + org.Login + "/actions/variables"

	// visibility is required at the org scope.
	mustStatus(t, ghPost(t, base, defaultToken, map[string]interface{}{
		"name": "NO_VIS", "value": "v",
	}), 422, "missing visibility")
	mustStatus(t, ghPost(t, base, defaultToken, map[string]interface{}{
		"name": "BAD_VIS", "value": "v", "visibility": "everyone",
	}), 422, "invalid visibility")

	mustStatus(t, ghPost(t, base, defaultToken, map[string]interface{}{
		"name": "ORG_VAR_ALL", "value": "all-v", "visibility": "all",
	}), 201, "create all")
	mustStatus(t, ghPost(t, base, defaultToken, map[string]interface{}{
		"name": "ORG_VAR_SEL", "value": "sel-v", "visibility": "selected",
		"selected_repository_ids": []int{selRepo.ID},
	}), 201, "create selected")
	mustStatus(t, ghPost(t, base, defaultToken, map[string]interface{}{
		"name": "ORG_VAR_ALL", "value": "again", "visibility": "all",
	}), 409, "duplicate create")

	// Get: org shape carries visibility; URL only when selected.
	v := decodeJSON(t, ghGet(t, base+"/ORG_VAR_ALL", defaultToken))
	if v["visibility"] != "all" {
		t.Errorf("visibility = %v", v["visibility"])
	}
	if _, has := v["selected_repositories_url"]; has {
		t.Error("visibility=all variable must not carry selected_repositories_url")
	}
	v = decodeJSON(t, ghGet(t, base+"/ORG_VAR_SEL", defaultToken))
	if v["visibility"] != "selected" {
		t.Errorf("visibility = %v", v["visibility"])
	}
	if _, has := v["selected_repositories_url"]; !has {
		t.Error("selected variable missing selected_repositories_url")
	}

	// Repositories endpoints conflict on a non-selected variable (the
	// variables spec documents 409 on list/set too).
	mustStatus(t, ghGet(t, base+"/ORG_VAR_ALL/repositories", defaultToken), 409, "list repos on all")
	mustStatus(t, ghPut(t, base+"/ORG_VAR_ALL/repositories", defaultToken,
		map[string]interface{}{"selected_repository_ids": []int{selRepo.ID}}), 409, "set repos on all")
	mustStatus(t, ghPut(t, fmt.Sprintf("%s/ORG_VAR_ALL/repositories/%d", base, selRepo.ID), defaultToken, nil), 409, "add repo on all")

	repos := decodeJSON(t, ghGet(t, base+"/ORG_VAR_SEL/repositories", defaultToken))
	if int(repos["total_count"].(float64)) != 1 {
		t.Fatalf("repositories total_count = %v, want 1", repos["total_count"])
	}

	other := seedOrgRepo(t, org, "var-sel-other", true)
	addPath := fmt.Sprintf("%s/ORG_VAR_SEL/repositories/%d", base, other.ID)
	mustStatus(t, ghPut(t, addPath, defaultToken, nil), 204, "add repo")
	mustStatus(t, ghDelete(t, addPath, defaultToken), 204, "remove repo")
	mustStatus(t, ghPut(t, base+"/ORG_VAR_SEL/repositories", defaultToken,
		map[string]interface{}{"selected_repository_ids": []int{other.ID}}), 204, "set repos")

	// Patch visibility selected → all clears the selection.
	mustStatus(t, ghPatch(t, base+"/ORG_VAR_SEL", defaultToken, map[string]interface{}{
		"visibility": "all",
	}), 204, "patch visibility")
	v = decodeJSON(t, ghGet(t, base+"/ORG_VAR_SEL", defaultToken))
	if v["visibility"] != "all" {
		t.Errorf("after patch visibility = %v", v["visibility"])
	}
	mustStatus(t, ghGet(t, base+"/ORG_VAR_SEL/repositories", defaultToken), 409, "repos after visibility change")

	// Delete.
	mustStatus(t, ghDelete(t, base+"/ORG_VAR_SEL", defaultToken), 204, "delete")
	mustStatus(t, ghGet(t, base+"/ORG_VAR_SEL", defaultToken), 404, "get after delete")

	mustStatus(t, ghGet(t, "/api/v3/orgs/no-such-org/actions/variables", defaultToken), 404, "missing org")
}

// --- environment variables ---

func TestEnvVariablesMissingEnv404(t *testing.T) {
	repo := seedTestRepo(t, "envvar-missing", false)
	base := "/api/v3/repos/" + repo.FullName + "/environments/ghost/variables"
	mustStatus(t, ghGet(t, base, defaultToken), 404, "list")
	mustStatus(t, ghPost(t, base, defaultToken, map[string]interface{}{"name": "X", "value": "v"}), 404, "create")
}

func TestEnvVariablesCRUD(t *testing.T) {
	repo := seedTestRepo(t, "envvar-crud", false)
	testServer.store.Deployments.UpsertEnvironment(repo.ID, "staging")
	base := "/api/v3/repos/" + repo.FullName + "/environments/staging/variables"

	mustStatus(t, ghPost(t, base, defaultToken, map[string]interface{}{
		"name": "env_var", "value": "e1",
	}), 201, "create")
	mustStatus(t, ghPost(t, base, defaultToken, map[string]interface{}{
		"name": "ENV_VAR", "value": "e2",
	}), 409, "duplicate")

	v := decodeJSON(t, ghGet(t, base+"/ENV_VAR", defaultToken))
	if v["name"] != "ENV_VAR" || v["value"] != "e1" {
		t.Fatalf("get = %v", v)
	}

	mustStatus(t, ghPatch(t, base+"/ENV_VAR", defaultToken, map[string]interface{}{
		"value": "e3",
	}), 204, "patch")
	list := decodeJSON(t, ghGet(t, base, defaultToken))
	if int(list["total_count"].(float64)) != 1 {
		t.Fatalf("total_count = %v", list["total_count"])
	}

	mustStatus(t, ghDelete(t, base+"/ENV_VAR", defaultToken), 204, "delete")
	mustStatus(t, ghGet(t, base+"/ENV_VAR", defaultToken), 404, "get after delete")
}

// --- repo-visible organization variables ---

func TestRepoOrganizationVariablesList(t *testing.T) {
	org := seedTestOrg(t, "varorg-repovis")
	pubRepo := seedOrgRepo(t, org, "varvis-pub", false)
	privRepo := seedOrgRepo(t, org, "varvis-priv", true)
	base := "/api/v3/orgs/" + org.Login + "/actions/variables"

	mustStatus(t, ghPost(t, base, defaultToken, map[string]interface{}{
		"name": "VV_ALL", "value": "v", "visibility": "all",
	}), 201, "create all")
	mustStatus(t, ghPost(t, base, defaultToken, map[string]interface{}{
		"name": "VV_PRIV", "value": "v", "visibility": "private",
	}), 201, "create private")
	mustStatus(t, ghPost(t, base, defaultToken, map[string]interface{}{
		"name": "VV_SEL", "value": "v", "visibility": "selected",
		"selected_repository_ids": []int{pubRepo.ID},
	}), 201, "create selected")

	names := func(repo *Repo) map[string]bool {
		body := decodeJSON(t, ghGet(t, "/api/v3/repos/"+repo.FullName+"/actions/organization-variables", defaultToken))
		out := map[string]bool{}
		for _, raw := range body["variables"].([]interface{}) {
			item := raw.(map[string]interface{})
			out[item["name"].(string)] = true
			// Repo-level item shape is the plain actions-variable.
			if _, has := item["visibility"]; has {
				t.Errorf("repo-level org variable item leaks visibility: %v", item)
			}
		}
		return out
	}

	pub := names(pubRepo)
	if !pub["VV_ALL"] || pub["VV_PRIV"] || !pub["VV_SEL"] {
		t.Errorf("public repo sees %v, want VV_ALL+VV_SEL", pub)
	}
	priv := names(privRepo)
	if !priv["VV_ALL"] || !priv["VV_PRIV"] || priv["VV_SEL"] {
		t.Errorf("private repo sees %v, want VV_ALL+VV_PRIV", priv)
	}
}

// --- persistence round-trip for every new bucket ---

// TestSecretsVariablesPersistenceReload follows the
// persistence_reload_test.go session pattern: write through the same
// MustPut calls the handlers make, reopen, and assert the five new
// buckets (plus the sealed-box keypair) come back.
func TestSecretsVariablesPersistenceReload(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BLEEPHUB_PERSIST", "true")
	t.Setenv("BLEEPHUB_DATA_DIR", dir)

	now := time.Now().UTC().Truncate(time.Second)
	repoKey := "o/r"
	envKey := envScopeKey(repoKey, "production")

	// --- session 1 ---
	p1, err := NewPersistence()
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	st1 := NewStore()
	if err := st1.SetPersistence(p1); err != nil {
		t.Fatalf("SetPersistence: %v", err)
	}

	kp1, err := st1.ActionsKeyPair()
	if err != nil {
		t.Fatalf("keypair: %v", err)
	}

	st1.RepoVariables[repoKey] = map[string]*ActionsVariable{
		"RV": {Name: "RV", Value: "rv", CreatedAt: now, UpdatedAt: now},
	}
	p1.MustPut("repo_variables", repoKey, st1.RepoVariables[repoKey])

	st1.OrgSecrets["acme"] = map[string]*OrgSecret{
		"OS": {Secret: Secret{Name: "OS", Value: "os", CreatedAt: now, UpdatedAt: now}, Visibility: "selected", SelectedRepoIDs: []int{7}},
	}
	p1.MustPut("org_secrets", "acme", st1.OrgSecrets["acme"])

	st1.OrgVariables["acme"] = map[string]*ActionsVariable{
		"OV": {Name: "OV", Value: "ov", Visibility: "all", CreatedAt: now, UpdatedAt: now},
	}
	p1.MustPut("org_variables", "acme", st1.OrgVariables["acme"])

	st1.EnvSecrets[envKey] = map[string]*Secret{
		"ES": {Name: "ES", Value: "es", CreatedAt: now, UpdatedAt: now},
	}
	p1.MustPut("env_secrets", envKey, st1.EnvSecrets[envKey])

	st1.EnvVariables[envKey] = map[string]*ActionsVariable{
		"EV": {Name: "EV", Value: "ev", CreatedAt: now, UpdatedAt: now},
	}
	p1.MustPut("env_variables", envKey, st1.EnvVariables[envKey])

	if err := p1.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// --- session 2 ---
	p2, err := NewPersistence()
	if err != nil {
		t.Fatalf("re-open: %v", err)
	}
	st2 := NewStore()
	if err := st2.SetPersistence(p2); err != nil {
		t.Fatalf("re-load SetPersistence: %v", err)
	}
	defer p2.Close()

	kp2, err := st2.ActionsKeyPair()
	if err != nil {
		t.Fatalf("keypair after reload: %v", err)
	}
	if kp2.KeyID != kp1.KeyID || kp2.PublicKey != kp1.PublicKey {
		t.Error("sealed-box keypair did not survive reload (clients caching the public key would go stale)")
	}

	if v := st2.RepoVariables[repoKey]["RV"]; v == nil || v.Value != "rv" {
		t.Errorf("repo variable did not persist: %+v", v)
	}
	os := st2.OrgSecrets["acme"]["OS"]
	if os == nil || os.Value != "os" || os.Visibility != "selected" || len(os.SelectedRepoIDs) != 1 || os.SelectedRepoIDs[0] != 7 {
		t.Errorf("org secret did not persist faithfully: %+v", os)
	}
	if v := st2.OrgVariables["acme"]["OV"]; v == nil || v.Value != "ov" || v.Visibility != "all" {
		t.Errorf("org variable did not persist: %+v", v)
	}
	if sec := st2.EnvSecrets[envKey]["ES"]; sec == nil || sec.Value != "es" {
		t.Errorf("env secret did not persist: %+v", sec)
	}
	if v := st2.EnvVariables[envKey]["EV"]; v == nil || v.Value != "ev" {
		t.Errorf("env variable did not persist: %+v", v)
	}
}
