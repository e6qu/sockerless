package bleephub

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

// protectBranchForWriteTests puts a base protection rule (status checks +
// restrictions) on main so the sub-resource endpoints have state to act on.
func protectBranchForWriteTests(t *testing.T, repo string) string {
	t.Helper()
	base := "/api/v3/repos/admin/" + repo + "/branches/main/protection"
	resp := ghPut(t, base, defaultToken, map[string]interface{}{
		"required_status_checks": map[string]interface{}{"strict": true, "contexts": []string{"ci"}},
		"restrictions": map[string]interface{}{
			"users": []map[string]interface{}{{"login": "admin", "id": 1, "type": "User"}},
		},
	})
	requireStatus(t, resp, 200)
	return base
}

func TestBranchProtectionRequiredSignatures(t *testing.T) {
	repo := createRepoWriteRepo(t, true)

	// Unprotected branch → 404 on every method.
	resp := ghGet(t, "/api/v3/repos/admin/"+repo+"/branches/main/protection/required_signatures", defaultToken)
	requireStatus(t, resp, 404)

	base := protectBranchForWriteTests(t, repo)

	resp = ghGet(t, base+"/required_signatures", defaultToken)
	data := decodeJSONWithStatus(t, resp, 200)
	if data["enabled"] != false {
		t.Fatalf("enabled = %v, want false before POST", data["enabled"])
	}
	if data["url"] == "" {
		t.Fatal("missing url")
	}

	resp = ghPost(t, base+"/required_signatures", defaultToken, nil)
	data = decodeJSONWithStatus(t, resp, 200)
	if data["enabled"] != true {
		t.Fatalf("enabled = %v, want true after POST", data["enabled"])
	}

	// The top-level protection object now carries required_signatures.
	resp = ghGet(t, base, defaultToken)
	protection := decodeJSONWithStatus(t, resp, 200)
	rs, _ := protection["required_signatures"].(map[string]interface{})
	if rs == nil || rs["enabled"] != true {
		t.Fatalf("protection.required_signatures = %v, want enabled true", protection["required_signatures"])
	}

	delResp := ghDelete(t, base+"/required_signatures", defaultToken)
	requireStatus(t, delResp, 204)
	resp = ghGet(t, base+"/required_signatures", defaultToken)
	data = decodeJSONWithStatus(t, resp, 200)
	if data["enabled"] != false {
		t.Fatalf("enabled = %v, want false after DELETE", data["enabled"])
	}
}

func TestBranchProtectionRestrictionsApps(t *testing.T) {
	repo := createRepoWriteRepo(t, true)
	base := protectBranchForWriteTests(t, repo)

	admin := testServer.store.LookupUserByLogin("admin")
	appA := testServer.store.CreateApp(admin.ID, "Push App A", "app with push access",
		map[string]string{"contents": "write"}, []string{"push"})
	appB := testServer.store.CreateApp(admin.ID, "Push App B", "second app with push access",
		map[string]string{"contents": "write"}, []string{"push"})

	resp := ghGet(t, base+"/restrictions/apps", defaultToken)
	list := decodeJSONWithStatus2xxArray(t, resp, 200)
	if len(list) != 0 {
		t.Fatalf("initial apps = %v, want empty", list)
	}

	// Unknown slugs are rejected.
	resp = ghPost(t, base+"/restrictions/apps", defaultToken, map[string]interface{}{"apps": []string{"no-such-app"}})
	requireStatus(t, resp, 422)

	resp = ghPost(t, base+"/restrictions/apps", defaultToken, map[string]interface{}{"apps": []string{appA.Slug}})
	list = decodeJSONWithStatus2xxArray(t, resp, 200)
	if len(list) != 1 || list[0]["slug"] != appA.Slug {
		t.Fatalf("apps after POST = %v, want [%s]", list, appA.Slug)
	}

	// PUT replaces the whole list.
	resp = ghPut(t, base+"/restrictions/apps", defaultToken, map[string]interface{}{"apps": []string{appB.Slug}})
	list = decodeJSONWithStatus2xxArray(t, resp, 200)
	if len(list) != 1 || list[0]["slug"] != appB.Slug {
		t.Fatalf("apps after PUT = %v, want [%s]", list, appB.Slug)
	}

	// POST appends without duplicating.
	resp = ghPost(t, base+"/restrictions/apps", defaultToken, map[string]interface{}{"apps": []string{appA.Slug, appB.Slug}})
	list = decodeJSONWithStatus2xxArray(t, resp, 200)
	if len(list) != 2 {
		t.Fatalf("apps after append = %v, want 2 entries", list)
	}

	resp = ghDeleteWithBody(t, base+"/restrictions/apps", defaultToken, map[string]interface{}{"apps": []interface{}{appB.Slug}})
	list = decodeJSONWithStatus2xxArray(t, resp, 200)
	if len(list) != 1 || list[0]["slug"] != appA.Slug {
		t.Fatalf("apps after DELETE = %v, want [%s]", list, appA.Slug)
	}

	resp = ghGet(t, base+"/restrictions/apps", defaultToken)
	list = decodeJSONWithStatus2xxArray(t, resp, 200)
	if len(list) != 1 {
		t.Fatalf("apps round-trip = %v, want 1 entry", list)
	}
}

func TestBranchProtectionRequiredStatusChecksPatch(t *testing.T) {
	repo := createRepoWriteRepo(t, true)
	base := protectBranchForWriteTests(t, repo)

	resp := ghPatch(t, base+"/required_status_checks", defaultToken, map[string]interface{}{
		"strict": false,
		"checks": []map[string]interface{}{{"context": "build", "app_id": -1}},
	})
	data := decodeJSONWithStatus(t, resp, 200)
	if data["strict"] != false {
		t.Fatalf("strict = %v, want false", data["strict"])
	}
	checks, _ := data["checks"].([]interface{})
	if len(checks) != 1 {
		t.Fatalf("checks = %v, want 1 entry", data["checks"])
	}
	check := checks[0].(map[string]interface{})
	if check["context"] != "build" || check["app_id"] != float64(-1) {
		t.Fatalf("check = %v, want context build app_id -1", check)
	}
	// Contexts were not part of the PATCH body — the stored value survives.
	contexts, _ := data["contexts"].([]interface{})
	if len(contexts) != 1 || contexts[0] != "ci" {
		t.Fatalf("contexts = %v, want [ci]", data["contexts"])
	}
	if data["contexts_url"] == "" || data["url"] == "" {
		t.Fatal("missing url/contexts_url")
	}

	// The merged rule reads back the same through GET.
	resp = ghGet(t, base+"/required_status_checks", defaultToken)
	got := decodeJSONWithStatus(t, resp, 200)
	if got["strict"] != false {
		t.Fatalf("GET strict = %v, want false", got["strict"])
	}
	gotChecks, _ := got["checks"].([]interface{})
	if len(gotChecks) != 1 {
		t.Fatalf("GET checks = %v, want 1 entry", got["checks"])
	}

	// PATCH on a branch without protection is a 404.
	resp = ghPatch(t, "/api/v3/repos/admin/"+repo+"/branches/other/protection/required_status_checks",
		defaultToken, map[string]interface{}{"strict": true})
	requireStatus(t, resp, 404)
}

// decodeJSONWithStatus2xxArray asserts the status and decodes a JSON-array
// response body into a slice of objects.
func decodeJSONWithStatus2xxArray(t *testing.T, resp *http.Response, want int) []map[string]interface{} {
	t.Helper()
	defer resp.Body.Close()
	if resp.StatusCode != want {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want %d: %s", resp.StatusCode, want, body)
	}
	var out []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode array: %v", err)
	}
	return out
}
