package bleephub

import (
	"fmt"
	"net/http"
	"testing"
)

func TestEnterpriseCodeSecurityConfigurations_CRUD(t *testing.T) {
	base := enterpriseAPI + "/code-security/configurations"

	// Missing name/description → 400 (the documented bad-request status for
	// the enterprise create endpoint).
	resp := ghPost(t, base, defaultToken, map[string]interface{}{"name": "no-description"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("create without description: got %d, want 400", resp.StatusCode)
	}

	// Create with defaults.
	resp = ghPost(t, base, defaultToken, map[string]interface{}{
		"name":              "baseline",
		"description":       "Enterprise baseline security posture",
		"dependabot_alerts": "enabled",
	})
	if resp.StatusCode != http.StatusCreated {
		resp.Body.Close()
		t.Fatalf("create: got %d, want 201", resp.StatusCode)
	}
	cfg := decodeJSON(t, resp)
	if cfg["target_type"] != "enterprise" {
		t.Fatalf("target_type = %v, want enterprise", cfg["target_type"])
	}
	if cfg["dependency_graph"] != "enabled" {
		t.Fatalf("dependency_graph default = %v, want enabled", cfg["dependency_graph"])
	}
	if cfg["dependabot_alerts"] != "enabled" {
		t.Fatalf("dependabot_alerts = %v, want enabled", cfg["dependabot_alerts"])
	}
	if cfg["enforcement"] != "enforced" {
		t.Fatalf("enforcement default = %v, want enforced", cfg["enforcement"])
	}
	id := int(cfg["id"].(float64))
	one := fmt.Sprintf("%s/%d", base, id)

	// Get.
	resp = ghGet(t, one, defaultToken)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("get: got %d, want 200", resp.StatusCode)
	}
	got := decodeJSON(t, resp)
	if got["name"] != "baseline" {
		t.Fatalf("get name = %v", got["name"])
	}

	// List contains it.
	resp = ghGet(t, base, defaultToken)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("list: got %d, want 200", resp.StatusCode)
	}
	found := false
	for _, item := range decodeJSONArray(t, resp) {
		if int(item["id"].(float64)) == id {
			found = true
		}
	}
	if !found {
		t.Fatal("list does not contain the created configuration")
	}

	// Patch: change a feature + name.
	resp = ghPatch(t, one, defaultToken, map[string]interface{}{
		"name":            "baseline-v2",
		"secret_scanning": "enabled",
	})
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("patch: got %d, want 200", resp.StatusCode)
	}
	patched := decodeJSON(t, resp)
	if patched["name"] != "baseline-v2" || patched["secret_scanning"] != "enabled" {
		t.Fatalf("patch result: name=%v secret_scanning=%v", patched["name"], patched["secret_scanning"])
	}
	// Untouched member kept its value.
	if patched["dependabot_alerts"] != "enabled" {
		t.Fatalf("patch clobbered dependabot_alerts: %v", patched["dependabot_alerts"])
	}

	// Invalid enum → 422 on update.
	resp = ghPatch(t, one, defaultToken, map[string]interface{}{"secret_scanning": "sideways"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("patch invalid enum: got %d, want 422", resp.StatusCode)
	}

	// Delete.
	resp = ghDelete(t, one, defaultToken)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: got %d, want 204", resp.StatusCode)
	}
	resp = ghGet(t, one, defaultToken)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("get after delete: got %d, want 404", resp.StatusCode)
	}
}

func TestEnterpriseCodeSecurityConfiguration_AdvancedSecurityAggregates(t *testing.T) {
	base := enterpriseAPI + "/code-security/configurations"
	resp := ghPost(t, base, defaultToken, map[string]interface{}{
		"name":          "ghas-code-only",
		"description":   "code security product only",
		"code_security": "enabled",
	})
	if resp.StatusCode != http.StatusCreated {
		resp.Body.Close()
		t.Fatalf("create: got %d, want 201", resp.StatusCode)
	}
	cfg := decodeJSON(t, resp)
	if cfg["advanced_security"] != "code_security" {
		t.Fatalf("advanced_security = %v, want code_security (folded from the code_security toggle)", cfg["advanced_security"])
	}
	id := int(cfg["id"].(float64))
	resp = ghDelete(t, fmt.Sprintf("%s/%d", base, id), defaultToken)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("cleanup delete: got %d", resp.StatusCode)
	}
}

func TestEnterpriseCodeSecurityConfiguration_DefaultsAndAttach(t *testing.T) {
	// An organization-owned repository the attachment can land on.
	createEnterpriseTestOrg(t, "ent-cs-org")
	resp := ghPost(t, "/api/v3/orgs/ent-cs-org/repos", defaultToken, map[string]interface{}{"name": "cs-repo"})
	if resp.StatusCode != http.StatusCreated {
		resp.Body.Close()
		t.Fatalf("create org repo: got %d, want 201", resp.StatusCode)
	}
	resp.Body.Close()

	base := enterpriseAPI + "/code-security/configurations"
	resp = ghPost(t, base, defaultToken, map[string]interface{}{
		"name":        "attach-me",
		"description": "configuration under attachment test",
	})
	if resp.StatusCode != http.StatusCreated {
		resp.Body.Close()
		t.Fatalf("create: got %d, want 201", resp.StatusCode)
	}
	cfg := decodeJSON(t, resp)
	id := int(cfg["id"].(float64))
	one := fmt.Sprintf("%s/%d", base, id)

	// Attach with an invalid scope → 422.
	resp = ghPost(t, one+"/attach", defaultToken, map[string]interface{}{"scope": "some"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("attach invalid scope: got %d, want 422", resp.StatusCode)
	}

	// Attach to all repositories → 202.
	resp = ghPost(t, one+"/attach", defaultToken, map[string]interface{}{"scope": "all"})
	if resp.StatusCode != http.StatusAccepted {
		resp.Body.Close()
		t.Fatalf("attach: got %d, want 202", resp.StatusCode)
	}
	resp.Body.Close()

	// Repositories list shows the attachment. Walk the pages like a real
	// client: scope=all attaches every organization repository in the
	// shared test server, so the one created here may sit past page 1.
	var sawRepo bool
	for page := 1; !sawRepo; page++ {
		resp = ghGet(t, fmt.Sprintf("%s/repositories?per_page=100&page=%d", one, page), defaultToken)
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			t.Fatalf("repositories page %d: got %d, want 200", page, resp.StatusCode)
		}
		items := decodeJSONArray(t, resp)
		if len(items) == 0 {
			break
		}
		for _, item := range items {
			if item["status"] != "attached" {
				t.Fatalf("attachment status = %v, want attached", item["status"])
			}
			repo, _ := item["repository"].(map[string]interface{})
			if repo != nil && repo["full_name"] == "ent-cs-org/cs-repo" {
				sawRepo = true
			}
		}
	}
	if !sawRepo {
		t.Fatal("attached repositories list does not contain ent-cs-org/cs-repo")
	}

	// Status filter for a state bleephub's synchronous attach never leaves
	// behind → empty list.
	resp = ghGet(t, one+"/repositories?status=failed", defaultToken)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("repositories status filter: got %d, want 200", resp.StatusCode)
	}
	if got := len(decodeJSONArray(t, resp)); got != 0 {
		t.Fatalf("repositories with status=failed = %d, want 0", got)
	}

	// Set as default for new repositories.
	resp = ghPut(t, one+"/defaults", defaultToken, map[string]interface{}{
		"default_for_new_repos": "all",
	})
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("set default: got %d, want 200", resp.StatusCode)
	}
	def := decodeJSON(t, resp)
	if def["default_for_new_repos"] != "all" {
		t.Fatalf("default_for_new_repos = %v, want all", def["default_for_new_repos"])
	}
	if cfgBody, _ := def["configuration"].(map[string]interface{}); cfgBody == nil || int(cfgBody["id"].(float64)) != id {
		t.Fatalf("set-default configuration member = %v", def["configuration"])
	}

	// The defaults listing includes it.
	resp = ghGet(t, base+"/defaults", defaultToken)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("defaults: got %d, want 200", resp.StatusCode)
	}
	foundDefault := false
	for _, item := range decodeJSONArray(t, resp) {
		cfgBody, _ := item["configuration"].(map[string]interface{})
		if cfgBody != nil && int(cfgBody["id"].(float64)) == id {
			foundDefault = true
			if item["default_for_new_repos"] != "all" {
				t.Fatalf("defaults entry visibility = %v, want all", item["default_for_new_repos"])
			}
		}
	}
	if !foundDefault {
		t.Fatal("defaults listing does not contain the configuration")
	}

	// Deleting a default configuration → 409.
	resp = ghDelete(t, one, defaultToken)
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("delete default configuration: got %d, want 409", resp.StatusCode)
	}

	// Clear the default, then delete cleanly.
	resp = ghPut(t, one+"/defaults", defaultToken, map[string]interface{}{
		"default_for_new_repos": "none",
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("clear default: got %d, want 200", resp.StatusCode)
	}
	resp = ghDelete(t, one, defaultToken)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete after clearing default: got %d, want 204", resp.StatusCode)
	}

	// Its attachments were removed with it.
	resp = ghGet(t, one+"/repositories", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("repositories after delete: got %d, want 404", resp.StatusCode)
	}
}
