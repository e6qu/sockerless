package bleephub

import (
	"testing"
)

// TestCollectOrgVisibilityMatrix drives the full org-item visibility
// matrix (all/private/selected × public/private/selected/unselected
// repo) through the runner-side collector, for secrets and variables.
func TestCollectOrgVisibilityMatrix(t *testing.T) {
	org := seedTestOrg(t, "injorg-matrix")
	pubRepo := seedOrgRepo(t, org, "inj-pub", false)
	privRepo := seedOrgRepo(t, org, "inj-priv", true)
	selRepo := seedOrgRepo(t, org, "inj-sel", false) // public but selected
	userRepo := seedTestRepo(t, "inj-user-owned", true)

	secBase := "/api/v3/orgs/" + org.Login + "/actions/secrets"
	varBase := "/api/v3/orgs/" + org.Login + "/actions/variables"

	put := func(name, visibility string, ids []int) {
		t.Helper()
		enc, keyID := sealForServer(t, "sec:"+name)
		body := map[string]interface{}{
			"encrypted_value": enc, "key_id": keyID, "visibility": visibility,
		}
		if ids != nil {
			body["selected_repository_ids"] = ids
		}
		mustStatus(t, ghPut(t, secBase+"/"+name, defaultToken, body), 201, "put secret "+name)

		varBody := map[string]interface{}{
			"name": name, "value": "var:" + name, "visibility": visibility,
		}
		if ids != nil {
			varBody["selected_repository_ids"] = ids
		}
		mustStatus(t, ghPost(t, varBase, defaultToken, varBody), 201, "post variable "+name)
	}
	put("M_ALL", "all", nil)
	put("M_PRIV", "private", nil)
	put("M_SEL", "selected", []int{selRepo.ID})

	cases := []struct {
		repo *Repo
		want map[string]bool // item name → visible
	}{
		{pubRepo, map[string]bool{"M_ALL": true, "M_PRIV": false, "M_SEL": false}},
		{privRepo, map[string]bool{"M_ALL": true, "M_PRIV": true, "M_SEL": false}},
		{selRepo, map[string]bool{"M_ALL": true, "M_PRIV": false, "M_SEL": true}},
		// A user-owned repo has no org scope at all, even though it is
		// private — the owner segment is not an organization.
		{userRepo, map[string]bool{"M_ALL": false, "M_PRIV": false, "M_SEL": false}},
	}
	for _, tc := range cases {
		t.Run(tc.repo.FullName, func(t *testing.T) {
			secrets, vars := testServer.CollectJobSecretsAndVars(tc.repo.FullName, "")
			for name, want := range tc.want {
				if _, got := secrets[name]; got != want {
					t.Errorf("secret %s visible=%v, want %v", name, got, want)
				}
				if want && secrets[name] != "sec:"+name {
					t.Errorf("secret %s = %q, want %q", name, secrets[name], "sec:"+name)
				}
				if _, got := vars[name]; got != want {
					t.Errorf("variable %s visible=%v, want %v", name, got, want)
				}
				if want && vars[name] != "var:"+name {
					t.Errorf("variable %s = %q, want %q", name, vars[name], "var:"+name)
				}
			}
		})
	}
}

// TestCollectPrecedence proves the merge order: organization < repository
// < environment, independently for secrets and variables.
func TestCollectPrecedence(t *testing.T) {
	org := seedTestOrg(t, "injorg-prec")
	repo := seedOrgRepo(t, org, "prec-repo", false)
	testServer.store.Deployments.UpsertEnvironment(repo.ID, "production")

	// One name defined at all three scopes.
	enc, keyID := sealForServer(t, "from-org")
	mustStatus(t, ghPut(t, "/api/v3/orgs/"+org.Login+"/actions/secrets/STACKED", defaultToken,
		map[string]interface{}{"encrypted_value": enc, "key_id": keyID, "visibility": "all"}), 201, "org secret")
	mustStatus(t, putSealedSecret(t, "/api/v3/repos/"+repo.FullName+"/actions/secrets/STACKED", "from-repo"), 201, "repo secret")
	mustStatus(t, putSealedSecret(t, "/api/v3/repos/"+repo.FullName+"/environments/production/secrets/STACKED", "from-env"), 201, "env secret")

	mustStatus(t, ghPost(t, "/api/v3/orgs/"+org.Login+"/actions/variables", defaultToken,
		map[string]interface{}{"name": "STACKED_VAR", "value": "from-org", "visibility": "all"}), 201, "org var")
	mustStatus(t, ghPost(t, "/api/v3/repos/"+repo.FullName+"/actions/variables", defaultToken,
		map[string]interface{}{"name": "STACKED_VAR", "value": "from-repo"}), 201, "repo var")
	mustStatus(t, ghPost(t, "/api/v3/repos/"+repo.FullName+"/environments/production/variables", defaultToken,
		map[string]interface{}{"name": "STACKED_VAR", "value": "from-env"}), 201, "env var")

	// And one name per scope to show lower scopes still contribute.
	enc, keyID = sealForServer(t, "only-org")
	mustStatus(t, ghPut(t, "/api/v3/orgs/"+org.Login+"/actions/secrets/ONLY_ORG", defaultToken,
		map[string]interface{}{"encrypted_value": enc, "key_id": keyID, "visibility": "all"}), 201, "only-org secret")
	mustStatus(t, putSealedSecret(t, "/api/v3/repos/"+repo.FullName+"/actions/secrets/ONLY_REPO", "only-repo"), 201, "only-repo secret")
	mustStatus(t, putSealedSecret(t, "/api/v3/repos/"+repo.FullName+"/environments/production/secrets/ONLY_ENV", "only-env"), 201, "only-env secret")

	// With the environment: env wins, all scopes contribute.
	secrets, vars := testServer.CollectJobSecretsAndVars(repo.FullName, "production")
	if secrets["STACKED"] != "from-env" {
		t.Errorf("STACKED = %q, want from-env", secrets["STACKED"])
	}
	if vars["STACKED_VAR"] != "from-env" {
		t.Errorf("STACKED_VAR = %q, want from-env", vars["STACKED_VAR"])
	}
	for name, want := range map[string]string{"ONLY_ORG": "only-org", "ONLY_REPO": "only-repo", "ONLY_ENV": "only-env"} {
		if secrets[name] != want {
			t.Errorf("%s = %q, want %q", name, secrets[name], want)
		}
	}

	// Without the environment: repo wins, env-only items absent.
	secrets, vars = testServer.CollectJobSecretsAndVars(repo.FullName, "")
	if secrets["STACKED"] != "from-repo" {
		t.Errorf("no-env STACKED = %q, want from-repo", secrets["STACKED"])
	}
	if vars["STACKED_VAR"] != "from-repo" {
		t.Errorf("no-env STACKED_VAR = %q, want from-repo", vars["STACKED_VAR"])
	}
	if _, present := secrets["ONLY_ENV"]; present {
		t.Error("ONLY_ENV leaked into a job without the environment")
	}
}

// TestCollectUnknownRepo confirms the collector degrades to empty maps
// (never nil) for a repo bleephub does not know.
func TestCollectUnknownRepo(t *testing.T) {
	secrets, vars := testServer.CollectJobSecretsAndVars("ghost/ghost", "")
	if secrets == nil || vars == nil {
		t.Fatal("collector must return non-nil maps")
	}
	if len(secrets) != 0 || len(vars) != 0 {
		t.Errorf("unexpected items: %v %v", secrets, vars)
	}
}
