package bleephub

import (
	"testing"
)

func TestCodeSecurityConfigurations_CRUD(t *testing.T) {
	org := createTestOrg(t)
	base := "/api/v3/orgs/" + org + "/code-security/configurations"

	// Create.
	resp := ghPost(t, base, defaultToken, map[string]interface{}{
		"name":                        "octo-defaults",
		"description":                 "Baseline security posture",
		"dependabot_alerts":           "enabled",
		"secret_scanning":             "enabled",
		"code_scanning_default_setup": "enabled",
		"code_scanning_default_setup_options": map[string]interface{}{
			"runner_type": "labeled", "runner_label": "sec-runner",
		},
	})
	if resp.StatusCode != 201 {
		t.Fatalf("create configuration: %d", resp.StatusCode)
	}
	created := decodeJSON(t, resp)
	if created["name"] != "octo-defaults" || created["target_type"] != "organization" {
		t.Fatalf("created = %v", created)
	}
	if created["dependabot_alerts"] != "enabled" || created["secret_scanning"] != "enabled" {
		t.Fatalf("created = %v", created)
	}
	// Unspecified members hold their documented creation defaults.
	if created["dependency_graph"] != "enabled" || created["enforcement"] != "enforced" || created["advanced_security"] != "disabled" {
		t.Fatalf("creation defaults = %v", created)
	}
	opts := created["code_scanning_default_setup_options"].(map[string]interface{})
	if opts["runner_type"] != "labeled" || opts["runner_label"] != "sec-runner" {
		t.Fatalf("setup options = %v", opts)
	}
	id := itoa(int(created["id"].(float64)))

	// Duplicate name is rejected.
	resp = ghPost(t, base, defaultToken, map[string]interface{}{
		"name": "octo-defaults", "description": "dup",
	})
	resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Fatalf("duplicate name: %d", resp.StatusCode)
	}

	// List.
	resp = ghGet(t, base, defaultToken)
	if resp.StatusCode != 200 {
		t.Fatalf("list configurations: %d", resp.StatusCode)
	}
	if list := decodeJSONArray(t, resp); len(list) != 1 {
		t.Fatalf("list = %v", list)
	}

	// GET one.
	resp = ghGet(t, base+"/"+id, defaultToken)
	if resp.StatusCode != 200 {
		t.Fatalf("get configuration: %d", resp.StatusCode)
	}
	if got := decodeJSON(t, resp); got["name"] != "octo-defaults" {
		t.Fatalf("get = %v", got)
	}

	// PATCH with a real change returns the updated configuration.
	resp = ghPatch(t, base+"/"+id, defaultToken, map[string]interface{}{
		"secret_scanning_push_protection": "enabled",
	})
	if resp.StatusCode != 200 {
		t.Fatalf("patch configuration: %d", resp.StatusCode)
	}
	if got := decodeJSON(t, resp); got["secret_scanning_push_protection"] != "enabled" {
		t.Fatalf("patched = %v", got)
	}

	// PATCH that changes nothing is a 204.
	resp = ghPatch(t, base+"/"+id, defaultToken, map[string]interface{}{
		"secret_scanning_push_protection": "enabled",
	})
	resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("no-op patch: %d", resp.StatusCode)
	}

	// DELETE.
	resp = ghDelete(t, base+"/"+id, defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("delete configuration: %d", resp.StatusCode)
	}
	resp = ghGet(t, base+"/"+id, defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("get deleted configuration: %d", resp.StatusCode)
	}
}

func TestCodeSecurityConfigurations_AttachDetachAndRepoView(t *testing.T) {
	org := createTestOrg(t)
	base := "/api/v3/orgs/" + org + "/code-security/configurations"
	repoName, repoID := createOrgRepoForGovernance(t, org)
	repoPath := org + "/" + repoName

	resp := ghPost(t, base, defaultToken, map[string]interface{}{
		"name": "attach-target", "description": "attach test",
	})
	if resp.StatusCode != 201 {
		t.Fatalf("create configuration: %d", resp.StatusCode)
	}
	id := itoa(int(decodeJSON(t, resp)["id"].(float64)))

	// Attach to the selected repository.
	resp = ghPost(t, base+"/"+id+"/attach", defaultToken, map[string]interface{}{
		"scope":                   "selected",
		"selected_repository_ids": []int{repoID},
	})
	if resp.StatusCode != 202 {
		t.Fatalf("attach: %d", resp.StatusCode)
	}
	resp.Body.Close()

	// The configuration's repositories list carries the attachment.
	resp = ghGet(t, base+"/"+id+"/repositories", defaultToken)
	if resp.StatusCode != 200 {
		t.Fatalf("list attached repos: %d", resp.StatusCode)
	}
	attached := decodeJSONArray(t, resp)
	if len(attached) != 1 || attached[0]["status"] != "attached" {
		t.Fatalf("attached = %v", attached)
	}
	repoJSON := attached[0]["repository"].(map[string]interface{})
	if int(repoJSON["id"].(float64)) != repoID {
		t.Fatalf("attached repo = %v", repoJSON)
	}

	// A status filter that excludes "attached" returns nothing.
	resp = ghGet(t, base+"/"+id+"/repositories?status=detached,failed", defaultToken)
	if got := decodeJSONArray(t, resp); len(got) != 0 {
		t.Fatalf("filtered attached = %v", got)
	}

	// The repo-level endpoint reads the association back.
	resp = ghGet(t, "/api/v3/repos/"+repoPath+"/code-security-configuration", defaultToken)
	if resp.StatusCode != 200 {
		t.Fatalf("repo configuration: %d", resp.StatusCode)
	}
	repoView := decodeJSON(t, resp)
	if repoView["status"] != "attached" {
		t.Fatalf("repo view = %v", repoView)
	}
	if cfg := repoView["configuration"].(map[string]interface{}); cfg["name"] != "attach-target" {
		t.Fatalf("repo view configuration = %v", cfg)
	}

	// Detach via the repository-selection body.
	resp = ghDeleteWithBody(t, base+"/detach", defaultToken, map[string]interface{}{
		"selected_repository_ids": []int{repoID},
	})
	resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("detach: %d", resp.StatusCode)
	}

	// No association remains: repo-level GET is a 204.
	resp = ghGet(t, "/api/v3/repos/"+repoPath+"/code-security-configuration", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("repo configuration after detach: %d", resp.StatusCode)
	}

	// Attaching an out-of-org repository ID is rejected.
	resp = ghPost(t, base+"/"+id+"/attach", defaultToken, map[string]interface{}{
		"scope":                   "selected",
		"selected_repository_ids": []int{99999999},
	})
	resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Fatalf("attach unknown repo: %d", resp.StatusCode)
	}

	// Detach requires at least one repository ID.
	resp = ghDeleteWithBody(t, base+"/detach", defaultToken, map[string]interface{}{
		"selected_repository_ids": []int{},
	})
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("detach without ids: %d", resp.StatusCode)
	}
}

func TestCodeSecurityConfigurations_Defaults(t *testing.T) {
	org := createTestOrg(t)
	base := "/api/v3/orgs/" + org + "/code-security/configurations"

	resp := ghPost(t, base, defaultToken, map[string]interface{}{
		"name": "default-cfg", "description": "defaults test",
	})
	if resp.StatusCode != 201 {
		t.Fatalf("create configuration: %d", resp.StatusCode)
	}
	id := itoa(int(decodeJSON(t, resp)["id"].(float64)))

	// No defaults configured yet.
	resp = ghGet(t, base+"/defaults", defaultToken)
	if resp.StatusCode != 200 {
		t.Fatalf("get defaults: %d", resp.StatusCode)
	}
	if got := decodeJSONArray(t, resp); len(got) != 0 {
		t.Fatalf("defaults = %v", got)
	}

	// Set as default for all new repositories.
	resp = ghPut(t, base+"/"+id+"/defaults", defaultToken, map[string]interface{}{
		"default_for_new_repos": "all",
	})
	if resp.StatusCode != 200 {
		t.Fatalf("set default: %d", resp.StatusCode)
	}
	set := decodeJSON(t, resp)
	if set["default_for_new_repos"] != "all" {
		t.Fatalf("set default = %v", set)
	}
	if cfg := set["configuration"].(map[string]interface{}); cfg["name"] != "default-cfg" {
		t.Fatalf("set default configuration = %v", cfg)
	}

	// The defaults listing reflects it.
	resp = ghGet(t, base+"/defaults", defaultToken)
	defaults := decodeJSONArray(t, resp)
	if len(defaults) != 1 || defaults[0]["default_for_new_repos"] != "all" {
		t.Fatalf("defaults = %v", defaults)
	}

	// Clearing with "none" removes it from the listing.
	resp = ghPut(t, base+"/"+id+"/defaults", defaultToken, map[string]interface{}{
		"default_for_new_repos": "none",
	})
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("clear default: %d", resp.StatusCode)
	}
	resp = ghGet(t, base+"/defaults", defaultToken)
	if got := decodeJSONArray(t, resp); len(got) != 0 {
		t.Fatalf("defaults after clear = %v", got)
	}
}
