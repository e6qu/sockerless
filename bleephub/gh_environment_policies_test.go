package bleephub

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"testing"
)

// jsonBody marshals a request body for a hand-built http.Request.
func jsonBody(t *testing.T, v interface{}) io.Reader {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return bytes.NewReader(b)
}

func TestEnvironmentDeploymentBranchPolicies_CRUD(t *testing.T) {
	repo := createRepoWriteRepo(t, false)
	base := "/api/v3/repos/admin/" + repo + "/environments/production"

	resp := ghPut(t, base, defaultToken, map[string]interface{}{
		"deployment_branch_policy": map[string]interface{}{
			"protected_branches":     false,
			"custom_branch_policies": true,
		},
	})
	env := decodeJSONWithStatus(t, resp, 200)
	policy, _ := env["deployment_branch_policy"].(map[string]interface{})
	if policy == nil || policy["custom_branch_policies"] != true {
		t.Fatalf("deployment_branch_policy = %v, want custom_branch_policies true", env["deployment_branch_policy"])
	}

	// Create a branch policy and a tag policy.
	resp = ghPost(t, base+"/deployment-branch-policies", defaultToken, map[string]interface{}{"name": "release/*"})
	created := decodeJSONWithStatus(t, resp, 200)
	if created["name"] != "release/*" || created["type"] != "branch" {
		t.Fatalf("created policy = %v", created)
	}
	policyID := strconv.Itoa(int(created["id"].(float64)))

	resp = ghPost(t, base+"/deployment-branch-policies", defaultToken, map[string]interface{}{"name": "v*", "type": "tag"})
	tagPolicy := decodeJSONWithStatus(t, resp, 200)
	if tagPolicy["type"] != "tag" {
		t.Fatalf("tag policy type = %v, want tag", tagPolicy["type"])
	}

	// A duplicate name+type answers 303 See Other pointing at the existing policy.
	req, _ := http.NewRequest("POST", testBaseURL+base+"/deployment-branch-policies", jsonBody(t, map[string]interface{}{"name": "release/*"}))
	req.Header.Set("Authorization", "token "+defaultToken)
	req.Header.Set("Content-Type", "application/json")
	noRedirect := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	dupResp, err := noRedirect.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	dupResp.Body.Close()
	if dupResp.StatusCode != 303 {
		t.Fatalf("duplicate POST status = %d, want 303", dupResp.StatusCode)
	}
	if loc := dupResp.Header.Get("Location"); loc == "" {
		t.Fatal("duplicate POST missing Location header")
	}

	resp = ghGet(t, base+"/deployment-branch-policies", defaultToken)
	list := decodeJSONWithStatus(t, resp, 200)
	if list["total_count"] != float64(2) {
		t.Fatalf("total_count = %v, want 2", list["total_count"])
	}

	resp = ghGet(t, base+"/deployment-branch-policies/"+policyID, defaultToken)
	got := decodeJSONWithStatus(t, resp, 200)
	if got["name"] != "release/*" {
		t.Fatalf("get policy name = %v", got["name"])
	}

	resp = ghPut(t, base+"/deployment-branch-policies/"+policyID, defaultToken, map[string]interface{}{"name": "release/*/*"})
	updated := decodeJSONWithStatus(t, resp, 200)
	if updated["name"] != "release/*/*" {
		t.Fatalf("updated name = %v", updated["name"])
	}

	delResp := ghDelete(t, base+"/deployment-branch-policies/"+policyID, defaultToken)
	requireStatus(t, delResp, 204)
	resp = ghGet(t, base+"/deployment-branch-policies/"+policyID, defaultToken)
	requireStatus(t, resp, 404)

	// An environment without custom branch policies rejects policy creation.
	resp = ghPut(t, "/api/v3/repos/admin/"+repo+"/environments/staging", defaultToken, nil)
	requireStatus(t, resp, 200)
	resp = ghPost(t, "/api/v3/repos/admin/"+repo+"/environments/staging/deployment-branch-policies", defaultToken,
		map[string]interface{}{"name": "main"})
	requireStatus(t, resp, 404)
}

func TestEnvironmentDeploymentProtectionRules_CRUD(t *testing.T) {
	repo := createRepoWriteRepo(t, false)
	base := "/api/v3/repos/admin/" + repo + "/environments/production"

	resp := ghPut(t, base, defaultToken, nil)
	requireStatus(t, resp, 200)

	admin := testServer.store.LookupUserByLogin("admin")
	app := testServer.store.CreateApp(admin.ID, "Deployment Gate", "custom deployment protection rule provider",
		map[string]string{"deployments": "write"}, []string{"deployment_protection_rule"})
	testServer.store.CreateInstallation(app.ID, "User", admin.ID, "admin", app.Permissions, app.Events)

	// The app is available for the environment (installed on the repo owner).
	resp = ghGet(t, base+"/deployment_protection_rules/apps", defaultToken)
	apps := decodeJSONWithStatus(t, resp, 200)
	available, _ := apps["available_custom_deployment_protection_rule_integrations"].([]interface{})
	foundApp := false
	for _, a := range available {
		if m, ok := a.(map[string]interface{}); ok && m["slug"] == app.Slug {
			foundApp = true
			if m["integration_url"] == "" || m["node_id"] == "" {
				t.Fatalf("app entry missing members: %v", m)
			}
		}
	}
	if !foundApp {
		t.Fatalf("app %s not in available integrations: %v", app.Slug, available)
	}

	resp = ghPost(t, base+"/deployment_protection_rules", defaultToken, map[string]interface{}{"integration_id": app.ID})
	rule := decodeJSONWithStatus(t, resp, 201)
	if rule["enabled"] != true {
		t.Fatalf("rule enabled = %v, want true", rule["enabled"])
	}
	ruleApp, _ := rule["app"].(map[string]interface{})
	if ruleApp == nil || ruleApp["slug"] != app.Slug {
		t.Fatalf("rule app = %v, want slug %s", rule["app"], app.Slug)
	}
	ruleID := strconv.Itoa(int(rule["id"].(float64)))

	// Enabling the same integration twice is rejected.
	resp = ghPost(t, base+"/deployment_protection_rules", defaultToken, map[string]interface{}{"integration_id": app.ID})
	requireStatus(t, resp, 422)
	// An unknown integration is rejected.
	resp = ghPost(t, base+"/deployment_protection_rules", defaultToken, map[string]interface{}{"integration_id": 999999})
	requireStatus(t, resp, 422)

	resp = ghGet(t, base+"/deployment_protection_rules", defaultToken)
	list := decodeJSONWithStatus(t, resp, 200)
	if list["total_count"] != float64(1) {
		t.Fatalf("total_count = %v, want 1", list["total_count"])
	}

	resp = ghGet(t, base+"/deployment_protection_rules/"+ruleID, defaultToken)
	got := decodeJSONWithStatus(t, resp, 200)
	if got["id"] != rule["id"] {
		t.Fatalf("get rule id = %v, want %v", got["id"], rule["id"])
	}

	delResp := ghDelete(t, base+"/deployment_protection_rules/"+ruleID, defaultToken)
	requireStatus(t, delResp, 204)
	resp = ghGet(t, base+"/deployment_protection_rules/"+ruleID, defaultToken)
	requireStatus(t, resp, 404)

	// Deleting the environment prunes its policies and rules.
	envID := envIDForTest(t, repo, "production")
	if rule2 := testServer.store.CreateEnvProtectionRule(envID, app.ID); rule2 == nil {
		t.Fatal("re-enable rule failed")
	}
	delResp = ghDelete(t, base, defaultToken)
	requireStatus(t, delResp, 204)
	if rules := testServer.store.ListEnvProtectionRules(envID); len(rules) != 0 {
		t.Fatalf("environment deletion left %d protection rules behind", len(rules))
	}
}

// envIDForTest resolves an environment's numeric ID through the store.
func envIDForTest(t *testing.T, repoName, envName string) int {
	t.Helper()
	repo := testServer.store.GetRepo("admin", repoName)
	if repo == nil {
		t.Fatalf("repo admin/%s not found", repoName)
	}
	env := testServer.store.Deployments.GetEnvironment(repo.ID, envName)
	if env == nil {
		t.Fatalf("environment %s not found", envName)
	}
	return env.ID
}
