package bleephub

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRulesets_FullLifecycle(t *testing.T) {
	s := newTestServer()
	s.registerGHRulesetRoutes()

	admin := s.store.UsersByLogin["admin"]
	s.store.CreateRepo(admin, "rules-repo", "", false)

	do := func(method, path string, body []byte) *httptest.ResponseRecorder {
		var req *http.Request
		if body != nil {
			req = httptest.NewRequest(method, path, bytes.NewReader(body))
		} else {
			req = httptest.NewRequest(method, path, nil)
		}
		req.Header.Set("Authorization", "Bearer bleephub-admin-token-00000000000000000000")
		w := httptest.NewRecorder()
		s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
		return w
	}

	create, _ := json.Marshal(map[string]any{
		"name":        "protect-main",
		"target":      "branch",
		"enforcement": "active",
		"conditions": map[string]any{
			"ref_name": map[string]any{
				"include": []string{"~DEFAULT_BRANCH"},
			},
		},
		"rules": []map[string]any{
			{"type": "creation"},
			{"type": "deletion"},
			{"type": "required_linear_history"},
		},
	})
	w := do("POST", "/api/v3/repos/admin/rules-repo/rulesets", create)
	if w.Code != http.StatusCreated {
		t.Fatalf("create ruleset: %d body=%s", w.Code, w.Body.String())
	}
	var created map[string]any
	json.Unmarshal(w.Body.Bytes(), &created)
	rsID := int(created["id"].(float64))
	if created["name"] != "protect-main" {
		t.Errorf("name = %v", created["name"])
	}
	if created["source_type"] != "Repository" {
		t.Errorf("source_type = %v", created["source_type"])
	}
	if rules, ok := created["rules"].([]any); !ok || len(rules) != 3 {
		t.Errorf("expected 3 rules, got %v", created["rules"])
	}

	// List
	w = do("GET", "/api/v3/repos/admin/rules-repo/rulesets", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list: %d body=%s", w.Code, w.Body.String())
	}
	var list []map[string]any
	json.Unmarshal(w.Body.Bytes(), &list)
	if len(list) != 1 {
		t.Fatalf("expected 1 ruleset, got %d", len(list))
	}
	if _, ok := list[0]["rules"]; ok {
		t.Errorf("list should not include rules by default")
	}

	// Get
	w = do("GET", "/api/v3/repos/admin/rules-repo/rulesets/"+itoa(rsID), nil)
	if w.Code != http.StatusOK {
		t.Fatalf("get: %d body=%s", w.Code, w.Body.String())
	}
	var got map[string]any
	json.Unmarshal(w.Body.Bytes(), &got)
	if got["name"] != "protect-main" {
		t.Errorf("get name = %v", got["name"])
	}

	// Update
	update, _ := json.Marshal(map[string]any{
		"enforcement": "evaluate",
		"rules": []map[string]any{
			{"type": "creation"},
		},
	})
	w = do("PUT", "/api/v3/repos/admin/rules-repo/rulesets/"+itoa(rsID), update)
	if w.Code != http.StatusOK {
		t.Fatalf("update: %d body=%s", w.Code, w.Body.String())
	}
	var updated map[string]any
	json.Unmarshal(w.Body.Bytes(), &updated)
	if updated["enforcement"] != "evaluate" {
		t.Errorf("enforcement = %v", updated["enforcement"])
	}
	if rules, ok := updated["rules"].([]any); !ok || len(rules) != 1 {
		t.Errorf("expected 1 rule after update, got %v", updated["rules"])
	}

	// History exists after update.
	w = do("GET", "/api/v3/repos/admin/rules-repo/rulesets/"+itoa(rsID)+"/history", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("history: %d body=%s", w.Code, w.Body.String())
	}
	var hist []map[string]any
	json.Unmarshal(w.Body.Bytes(), &hist)
	if len(hist) != 1 {
		t.Errorf("expected 1 history version, got %d", len(hist))
	}

	// List branch rules
	w = do("GET", "/api/v3/repos/admin/rules-repo/rules/branches/main", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("branch rules: %d body=%s", w.Code, w.Body.String())
	}
	var brules []map[string]any
	json.Unmarshal(w.Body.Bytes(), &brules)
	if len(brules) != 1 || brules[0]["type"] != "creation" {
		t.Errorf("expected active creation rule on main, got %+v", brules)
	}

	// Delete
	w = do("DELETE", "/api/v3/repos/admin/rules-repo/rulesets/"+itoa(rsID), nil)
	if w.Code != http.StatusNoContent {
		t.Errorf("delete: %d", w.Code)
	}
	w = do("GET", "/api/v3/repos/admin/rules-repo/rulesets/"+itoa(rsID), nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("get after delete: %d", w.Code)
	}
}

func TestRulesets_CreateMissingName(t *testing.T) {
	s := newTestServer()
	s.registerGHRulesetRoutes()

	admin := s.store.UsersByLogin["admin"]
	s.store.CreateRepo(admin, "rules-repo2", "", false)

	body, _ := json.Marshal(map[string]any{
		"target": "branch",
	})
	req := httptest.NewRequest("POST", "/api/v3/repos/admin/rules-repo2/rulesets", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer bleephub-admin-token-00000000000000000000")
	w := httptest.NewRecorder()
	s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestRulesets_OrgFullLifecycle(t *testing.T) {
	s := newTestServer()
	s.registerGHRulesetRoutes()

	admin := s.store.UsersByLogin["admin"]
	org := s.store.CreateOrg(admin, "rules-org", "Rules Org", "")
	if org == nil {
		t.Fatal("failed to create org")
	}

	do := func(method, path string, body []byte) *httptest.ResponseRecorder {
		var req *http.Request
		if body != nil {
			req = httptest.NewRequest(method, path, bytes.NewReader(body))
		} else {
			req = httptest.NewRequest(method, path, nil)
		}
		req.Header.Set("Authorization", "Bearer bleephub-admin-token-00000000000000000000")
		w := httptest.NewRecorder()
		s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
		return w
	}

	create, _ := json.Marshal(map[string]any{
		"name":        "protect-main",
		"target":      "branch",
		"enforcement": "active",
		"conditions": map[string]any{
			"ref_name": map[string]any{
				"include": []string{"~ALL"},
			},
		},
		"rules": []map[string]any{
			{"type": "creation"},
			{"type": "deletion"},
		},
	})
	w := do("POST", "/api/v3/orgs/rules-org/rulesets", create)
	if w.Code != http.StatusCreated {
		t.Fatalf("create org ruleset: %d body=%s", w.Code, w.Body.String())
	}
	var created map[string]any
	json.Unmarshal(w.Body.Bytes(), &created)
	rsID := int(created["id"].(float64))
	if created["name"] != "protect-main" {
		t.Errorf("name = %v", created["name"])
	}
	if created["source_type"] != "Organization" {
		t.Errorf("source_type = %v", created["source_type"])
	}
	if created["source"] != "rules-org" {
		t.Errorf("source = %v", created["source"])
	}
	if rules, ok := created["rules"].([]any); !ok || len(rules) != 2 {
		t.Errorf("expected 2 rules, got %v", created["rules"])
	}

	// List
	w = do("GET", "/api/v3/orgs/rules-org/rulesets", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list: %d body=%s", w.Code, w.Body.String())
	}
	var list []map[string]any
	json.Unmarshal(w.Body.Bytes(), &list)
	if len(list) != 1 {
		t.Fatalf("expected 1 ruleset, got %d", len(list))
	}
	if _, ok := list[0]["rules"]; ok {
		t.Errorf("list should not include rules by default")
	}

	// Get
	w = do("GET", "/api/v3/orgs/rules-org/rulesets/"+itoa(rsID), nil)
	if w.Code != http.StatusOK {
		t.Fatalf("get: %d body=%s", w.Code, w.Body.String())
	}
	var got map[string]any
	json.Unmarshal(w.Body.Bytes(), &got)
	if got["name"] != "protect-main" {
		t.Errorf("get name = %v", got["name"])
	}

	// Update
	update, _ := json.Marshal(map[string]any{
		"enforcement": "evaluate",
		"rules": []map[string]any{
			{"type": "creation"},
		},
	})
	w = do("PUT", "/api/v3/orgs/rules-org/rulesets/"+itoa(rsID), update)
	if w.Code != http.StatusOK {
		t.Fatalf("update: %d body=%s", w.Code, w.Body.String())
	}
	var updated map[string]any
	json.Unmarshal(w.Body.Bytes(), &updated)
	if updated["enforcement"] != "evaluate" {
		t.Errorf("enforcement = %v", updated["enforcement"])
	}
	if rules, ok := updated["rules"].([]any); !ok || len(rules) != 1 {
		t.Errorf("expected 1 rule after update, got %v", updated["rules"])
	}

	// History exists after update.
	w = do("GET", "/api/v3/orgs/rules-org/rulesets/"+itoa(rsID)+"/history", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("history: %d body=%s", w.Code, w.Body.String())
	}
	var hist []map[string]any
	json.Unmarshal(w.Body.Bytes(), &hist)
	if len(hist) != 1 {
		t.Errorf("expected 1 history version, got %d", len(hist))
	}

	// List rule suites returns empty list.
	w = do("GET", "/api/v3/orgs/rules-org/rulesets/rule-suites", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("rule suites: %d body=%s", w.Code, w.Body.String())
	}
	var suites []map[string]any
	json.Unmarshal(w.Body.Bytes(), &suites)
	if len(suites) != 0 {
		t.Errorf("expected empty rule suites, got %+v", suites)
	}

	// Delete
	w = do("DELETE", "/api/v3/orgs/rules-org/rulesets/"+itoa(rsID), nil)
	if w.Code != http.StatusNoContent {
		t.Errorf("delete: %d", w.Code)
	}
	w = do("GET", "/api/v3/orgs/rules-org/rulesets/"+itoa(rsID), nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("get after delete: %d", w.Code)
	}
}

func TestRulesets_OrgNonAdminCannotCreate(t *testing.T) {
	s := newTestServer()
	s.registerGHRulesetRoutes()

	admin := s.store.UsersByLogin["admin"]
	org := s.store.CreateOrg(admin, "rules-org2", "Rules Org 2", "")
	_ = org

	// Create a non-admin member.
	member := &User{ID: 999, Login: "member-user", Email: "member@example.com"}
	s.store.Users[member.ID] = member
	s.store.UsersByLogin[member.Login] = member
	s.store.SetMembership("rules-org2", member.ID, OrgRoleMember, MembershipStateActive)
	tok := s.store.CreateToken(member.ID, "repo,read:org")

	body, _ := json.Marshal(map[string]any{
		"name":        "protect-main",
		"target":      "branch",
		"enforcement": "active",
	})
	req := httptest.NewRequest("POST", "/api/v3/orgs/rules-org2/rulesets", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tok.Value)
	w := httptest.NewRecorder()
	s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestRulesets_BranchRuleExclusion(t *testing.T) {
	s := newTestServer()
	s.registerGHRulesetRoutes()

	admin := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(admin, "rules-repo3", "", false)
	_ = repo

	create, _ := json.Marshal(map[string]any{
		"name":        "skip-release",
		"target":      "branch",
		"enforcement": "active",
		"conditions": map[string]any{
			"ref_name": map[string]any{
				"include": []string{"~ALL"},
				"exclude": []string{"release/*"},
			},
		},
		"rules": []map[string]any{
			{"type": "deletion"},
		},
	})
	req := httptest.NewRequest("POST", "/api/v3/repos/admin/rules-repo3/rulesets", bytes.NewReader(create))
	req.Header.Set("Authorization", "Bearer bleephub-admin-token-00000000000000000000")
	w := httptest.NewRecorder()
	s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d body=%s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/api/v3/repos/admin/rules-repo3/rules/branches/release/v1", nil)
	req.Header.Set("Authorization", "Bearer bleephub-admin-token-00000000000000000000")
	s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
	var rules []map[string]any
	json.Unmarshal(w.Body.Bytes(), &rules)
	if len(rules) != 0 {
		t.Errorf("expected no rules on excluded branch, got %+v", rules)
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/api/v3/repos/admin/rules-repo3/rules/branches/main", nil)
	req.Header.Set("Authorization", "Bearer bleephub-admin-token-00000000000000000000")
	s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
	json.Unmarshal(w.Body.Bytes(), &rules)
	if len(rules) != 1 {
		t.Errorf("expected 1 rule on main, got %+v", rules)
	}
}
