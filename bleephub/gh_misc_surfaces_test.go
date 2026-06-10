package bleephub

import (
	"encoding/json"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestGPGKeyCRUD(t *testing.T) {
	s := newTestServer()
	s.registerGHMiscEndpoints()

	list := func() []interface{} {
		w := doMiscReq(s, "GET", "/api/v3/user/gpg_keys", "")
		if w.Code != 200 {
			t.Fatalf("list status = %d", w.Code)
		}
		var arr []interface{}
		json.Unmarshal(w.Body.Bytes(), &arr)
		return arr
	}

	if got := len(list()); got != 0 {
		t.Fatalf("initial count = %d, want 0", got)
	}

	w := doMiscReq(s, "POST", "/api/v3/user/gpg_keys", `{
		"armored_public_key": "-----BEGIN PGP PUBLIC KEY BLOCK-----\ntest-key-data\n-----END PGP PUBLIC KEY BLOCK-----",
		"name": "test-key"
	}`)
	if w.Code != 201 {
		t.Fatalf("create status = %d, body = %s", w.Code, w.Body.String())
	}
	var created map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &created)
	keyID := int(created["id"].(float64))
	if created["name"] != "test-key" {
		t.Fatalf("name = %v, want test-key", created["name"])
	}
	if created["can_sign"] != true {
		t.Fatalf("can_sign = %v, want true", created["can_sign"])
	}

	if got := len(list()); got != 1 {
		t.Fatalf("after create count = %d, want 1", got)
	}

	w = doMiscReq(s, "GET", "/api/v3/user/gpg_keys/"+strconv.Itoa(keyID), "")
	if w.Code != 200 {
		t.Fatalf("get status = %d", w.Code)
	}

	w = doMiscReq(s, "GET", "/api/v3/users/admin/gpg_keys", "")
	if w.Code != 200 {
		t.Fatalf("list by login status = %d", w.Code)
	}
	var byLogin []interface{}
	json.Unmarshal(w.Body.Bytes(), &byLogin)
	if len(byLogin) != 1 {
		t.Fatalf("list by login count = %d, want 1", len(byLogin))
	}

	w = doMiscReq(s, "DELETE", "/api/v3/user/gpg_keys/"+strconv.Itoa(keyID), "")
	if w.Code != 204 {
		t.Fatalf("delete status = %d", w.Code)
	}

	if got := len(list()); got != 0 {
		t.Fatalf("after delete count = %d, want 0", got)
	}

	w = doMiscReq(s, "GET", "/api/v3/user/gpg_keys/99999", "")
	if w.Code != 404 {
		t.Fatalf("get missing status = %d, want 404", w.Code)
	}

	w = doMiscReq(s, "POST", "/api/v3/user/gpg_keys", `{}`)
	if w.Code != 422 {
		t.Fatalf("create empty status = %d, want 422", w.Code)
	}
}

func TestGPGKeyDeleteOwnership(t *testing.T) {
	s := newTestServer()
	s.registerGHMiscEndpoints()

	s.store.mu.Lock()
	other := &User{ID: s.store.NextUser, Login: "other-user", Type: "User", SiteAdmin: false}
	s.store.NextUser++
	s.store.Users[other.ID] = other
	s.store.UsersByLogin[other.Login] = other
	s.store.Tokens["ghp_other"] = &Token{Value: "ghp_other", UserID: other.ID}
	s.store.mu.Unlock()

	w := doMiscReq(s, "POST", "/api/v3/user/gpg_keys", `{
		"armored_public_key": "-----BEGIN PGP PUBLIC KEY BLOCK-----\ntest\n-----END PGP PUBLIC KEY BLOCK-----"
	}`)
	if w.Code != 201 {
		t.Fatalf("admin create status = %d", w.Code)
	}
	var created map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &created)
	keyID := int(created["id"].(float64))

	req := httptest.NewRequest("DELETE", "/api/v3/user/gpg_keys/"+strconv.Itoa(keyID), nil)
	req.Header.Set("Authorization", "token ghp_other")
	req.Header.Set("Content-Type", "application/json")
	rw := httptest.NewRecorder()
	s.ghHeadersMiddleware(s.mux).ServeHTTP(rw, req)
	if rw.Code != 404 {
		t.Fatalf("other user delete status = %d, want 404", rw.Code)
	}
}

func TestPagesBuildsCRUD(t *testing.T) {
	s := newTestServer()
	s.registerGHMiscEndpoints()
	admin := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(admin, "pages-build-test", "", false)

	w := doMiscReq(s, "GET", "/api/v3/repos/"+repo.FullName+"/pages/builds", "")
	if w.Code != 200 {
		t.Fatalf("list builds status = %d", w.Code)
	}
	var builds []interface{}
	json.Unmarshal(w.Body.Bytes(), &builds)
	if len(builds) != 0 {
		t.Fatalf("initial builds = %d, want 0", len(builds))
	}

	w = doMiscReq(s, "POST", "/api/v3/repos/"+repo.FullName+"/pages/builds", "")
	if w.Code != 201 {
		t.Fatalf("trigger build status = %d, body = %s", w.Code, w.Body.String())
	}
	var triggered map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &triggered)
	if triggered["status"] != "queued" {
		t.Fatalf("status = %v, want queued", triggered["status"])
	}
	// GitHub's request-a-build response is exactly {status, url} with NO `id`.
	if _, hasID := triggered["id"]; hasID {
		t.Fatalf("trigger response must not carry top-level id; got %v", triggered)
	}
	buildURL, _ := triggered["url"].(string)
	if buildURL == "" {
		t.Fatalf("trigger response missing url; body = %s", w.Body.String())
	}
	// Builds are addressed by the trailing segment of their url.
	buildIDStr := buildURL[strings.LastIndex(buildURL, "/")+1:]
	buildID, err := strconv.ParseInt(buildIDStr, 10, 64)
	if err != nil {
		t.Fatalf("build url trailing segment %q not numeric: %v", buildIDStr, err)
	}

	w = doMiscReq(s, "GET", "/api/v3/repos/"+repo.FullName+"/pages/builds/latest", "")
	if w.Code != 200 {
		t.Fatalf("latest build status = %d", w.Code)
	}
	var latest map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &latest)
	// Build object carries no top-level id; it is addressed via url.
	if _, hasID := latest["id"]; hasID {
		t.Fatalf("build object must not carry top-level id; got %v", latest)
	}
	if latest["url"] != buildURL {
		t.Fatalf("latest url = %v, want %s", latest["url"], buildURL)
	}
	// GitHub always emits error:{"message":null} and a pusher/commit field.
	if _, ok := latest["error"]; !ok {
		t.Fatalf("build missing error object; got %v", latest)
	}
	if _, ok := latest["commit"]; !ok {
		t.Fatalf("build missing commit field; got %v", latest)
	}
	if _, ok := latest["pusher"]; !ok {
		t.Fatalf("build missing pusher field; got %v", latest)
	}

	w = doMiscReq(s, "GET", "/api/v3/repos/"+repo.FullName+"/pages/builds/"+strconv.FormatInt(buildID, 10), "")
	if w.Code != 200 {
		t.Fatalf("get build status = %d", w.Code)
	}

	w = doMiscReq(s, "GET", "/api/v3/repos/"+repo.FullName+"/pages/builds", "")
	if w.Code != 200 {
		t.Fatalf("list after trigger status = %d", w.Code)
	}
	json.Unmarshal(w.Body.Bytes(), &builds)
	if len(builds) != 1 {
		t.Fatalf("builds after trigger = %d, want 1", len(builds))
	}

	w = doMiscReq(s, "POST", "/api/v3/repos/"+repo.FullName+"/pages/builds", "")
	if w.Code != 201 {
		t.Fatalf("trigger second build status = %d", w.Code)
	}
	var second map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &second)
	secondURL, _ := second["url"].(string)
	if secondURL == "" || secondURL == buildURL {
		t.Fatalf("second build url = %q, should be set and differ from first %q", secondURL, buildURL)
	}

	w = doMiscReq(s, "GET", "/api/v3/repos/nonexist/pages/builds/latest", "")
	if w.Code != 404 {
		t.Fatalf("nonexist repo latest status = %d, want 404", w.Code)
	}
}

func TestPagesCreateUpdateShape(t *testing.T) {
	s := newTestServer()
	s.registerGHMiscEndpoints()
	admin := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(admin, "pages-shape", "", false)

	// Missing source.branch on a legacy build is a 422.
	w := doMiscReq(s, "POST", "/api/v3/repos/"+repo.FullName+"/pages", `{"source":{"path":"/"}}`)
	if w.Code != 422 {
		t.Fatalf("create without branch status = %d, want 422; body = %s", w.Code, w.Body.String())
	}

	// Valid create: status building, full field set, build_type persisted.
	w = doMiscReq(s, "POST", "/api/v3/repos/"+repo.FullName+"/pages",
		`{"source":{"branch":"gh-pages","path":"/docs"},"build_type":"legacy"}`)
	if w.Code != 201 {
		t.Fatalf("create status = %d, body = %s", w.Code, w.Body.String())
	}
	var site map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &site)
	if site["status"] != "building" {
		t.Fatalf("fresh site status = %v, want building", site["status"])
	}
	for _, k := range []string{"custom_404", "protected_domain_state", "build_type", "https_enforced"} {
		if _, ok := site[k]; !ok {
			t.Errorf("site missing field %q; got %v", k, site)
		}
	}
	if site["build_type"] != "legacy" {
		t.Errorf("build_type = %v, want legacy", site["build_type"])
	}

	// PUT update returns 204 No Content with empty body and persists params.
	w = doMiscReq(s, "PUT", "/api/v3/repos/"+repo.FullName+"/pages",
		`{"https_enforced":true,"build_type":"workflow","cname":"example.com","public":true}`)
	if w.Code != 204 {
		t.Fatalf("update status = %d, want 204; body = %s", w.Code, w.Body.String())
	}
	if w.Body.Len() != 0 {
		t.Fatalf("update body not empty: %s", w.Body.String())
	}

	w = doMiscReq(s, "GET", "/api/v3/repos/"+repo.FullName+"/pages", "")
	if w.Code != 200 {
		t.Fatalf("get after update status = %d", w.Code)
	}
	var updated map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &updated)
	if updated["https_enforced"] != true {
		t.Errorf("https_enforced = %v, want true", updated["https_enforced"])
	}
	if updated["build_type"] != "workflow" {
		t.Errorf("build_type = %v, want workflow", updated["build_type"])
	}
	if updated["cname"] != "example.com" {
		t.Errorf("cname = %v, want example.com", updated["cname"])
	}
}

func TestAuditLogRecords(t *testing.T) {
	s := newTestServer()
	s.registerGHMiscEndpoints()

	s.recordAuditEvent("test.action", "admin", "test-org", map[string]interface{}{"key": "val"})

	w := doMiscReq(s, "GET", "/api/v3/orgs/test-org/audit-log", "")
	if w.Code != 200 {
		t.Fatalf("audit log status = %d", w.Code)
	}
	var entries []interface{}
	json.Unmarshal(w.Body.Bytes(), &entries)
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	entry := entries[0].(map[string]interface{})
	if entry["action"] != "test.action" {
		t.Fatalf("action = %v, want test.action", entry["action"])
	}
	if entry["actor"] != "admin" {
		t.Fatalf("actor = %v, want admin", entry["actor"])
	}
	if entry["version"] != "1.1" {
		t.Fatalf("version = %v, want 1.1", entry["version"])
	}

	w = doMiscReq(s, "GET", "/api/v3/orgs/other-org/audit-log", "")
	if w.Code != 200 {
		t.Fatalf("other org audit log status = %d", w.Code)
	}
	var otherEntries []interface{}
	json.Unmarshal(w.Body.Bytes(), &otherEntries)
	if len(otherEntries) != 0 {
		t.Fatalf("other org entries = %d, want 0", len(otherEntries))
	}

	s.recordAuditEvent("test.action2", "user1", "test-org", nil)
	s.recordAuditEvent("test.action3", "user2", "test-org", nil)
	w = doMiscReq(s, "GET", "/api/v3/orgs/test-org/audit-log?phrase=test.action2", "")
	if w.Code != 200 {
		t.Fatalf("filtered audit log status = %d", w.Code)
	}
	var filtered []interface{}
	json.Unmarshal(w.Body.Bytes(), &filtered)
	if len(filtered) != 1 {
		t.Fatalf("filtered entries = %d, want 1", len(filtered))
	}
}

func TestAuditLogFromRepoCreate(t *testing.T) {
	s := newTestServer()
	s.registerGHMiscEndpoints()
	s.registerGHRepoRoutes()

	resp := doMiscReq(s, "POST", "/api/v3/user/repos", `{"name":"audit-test-repo"}`)
	if resp.Code != 201 {
		t.Fatalf("create repo status = %d", resp.Code)
	}

	w := doMiscReq(s, "GET", "/api/v3/orgs/default/audit-log", "")
	if w.Code != 200 {
		t.Fatalf("audit log status = %d", w.Code)
	}
	var entries []interface{}
	json.Unmarshal(w.Body.Bytes(), &entries)
	found := false
	for _, e := range entries {
		m := e.(map[string]interface{})
		if m["action"] == "repo.create" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("no repo.create audit event found")
	}
}

func TestMarketplacePlansFromStore(t *testing.T) {
	s := newTestServer()
	s.registerGHMiscEndpoints()
	s.seedDefaultMarketplacePlans()

	w := doMiscReq(s, "GET", "/api/v3/marketplace_listing/plans", "")
	if w.Code != 200 {
		t.Fatalf("plans status = %d", w.Code)
	}
	var plans []interface{}
	json.Unmarshal(w.Body.Bytes(), &plans)
	if len(plans) == 0 {
		t.Fatal("no plans returned")
	}
	plan := plans[0].(map[string]interface{})
	if plan["name"] != "Free" {
		t.Fatalf("plan name = %v, want Free", plan["name"])
	}
	if plan["price_model"] != "FREE" {
		t.Fatalf("price_model = %v, want FREE", plan["price_model"])
	}
}

func TestMarketplaceAccount(t *testing.T) {
	s := newTestServer()
	s.registerGHMiscEndpoints()
	s.seedDefaultMarketplacePlans()

	w := doMiscReq(s, "GET", "/api/v3/marketplace_listing/accounts/42", "")
	if w.Code != 200 {
		t.Fatalf("account status = %d", w.Code)
	}
	var acct map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &acct)
	if acct["id"] != float64(42) {
		t.Fatalf("id = %v, want 42", acct["id"])
	}
	purchase := acct["marketplace_purchase"].(map[string]interface{})
	plan := purchase["plan"].(map[string]interface{})
	if plan["name"] != "Free" {
		t.Fatalf("plan name = %v, want Free", plan["name"])
	}
}
