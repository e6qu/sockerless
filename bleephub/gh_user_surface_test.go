package bleephub

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

// userSurfaceUser creates a fresh user plus a classic personal access
// token so the /user endpoints operate on an account isolated from the
// shared admin fixture.
func userSurfaceUser(t *testing.T, login string) (*User, string) {
	t.Helper()
	u := createTestUser(t, login)
	tok := "ghp_" + login + "0000000000token"
	testServer.store.mu.Lock()
	testServer.store.Tokens[tok] = &Token{Value: tok, UserID: u.ID, Scopes: "repo, workflow, read:org, admin:org, gist, user", CreatedAt: time.Now()}
	testServer.store.mu.Unlock()
	return u, tok
}

// ─── PATCH /user + GET /user/{account_id} ───────────────────────────────

func TestUserProfile_UpdateAuthenticatedUser(t *testing.T) {
	u, tok := userSurfaceUser(t, "profileuser")

	resp := ghPatch(t, "/api/v3/user", tok, map[string]interface{}{
		"name":             "Profile User",
		"blog":             "https://blog.example.com",
		"company":          "Example Corp",
		"location":         "Berlin",
		"bio":              "I write Go",
		"hireable":         true,
		"twitter_username": "profileuser",
	})
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("PATCH /user: status = %d, want 200", resp.StatusCode)
	}
	data := decodeJSON(t, resp)
	if data["name"] != "Profile User" || data["blog"] != "https://blog.example.com" ||
		data["company"] != "Example Corp" || data["location"] != "Berlin" ||
		data["bio"] != "I write Go" || data["hireable"] != true ||
		data["twitter_username"] != "profileuser" {
		t.Fatalf("PATCH /user response missing updated profile fields: %v", data)
	}

	// GET /user reflects the stored profile.
	got := decodeJSON(t, ghGet(t, "/api/v3/user", tok))
	if got["company"] != "Example Corp" || got["hireable"] != true {
		t.Fatalf("GET /user does not reflect PATCHed profile: %v", got)
	}

	// GET /user/{account_id} resolves the same account by numeric ID.
	byID := decodeJSON(t, ghGet(t, "/api/v3/user/"+strconv.Itoa(u.ID), tok))
	if byID["login"] != "profileuser" || byID["location"] != "Berlin" {
		t.Fatalf("GET /user/{account_id}: got %v", byID)
	}

	// Unknown account IDs are 404.
	mustStatus(t, ghGet(t, "/api/v3/user/99999999", tok), http.StatusNotFound, "GET /user/{account_id} unknown")
	mustStatus(t, ghGet(t, "/api/v3/user/not-a-number", tok), http.StatusNotFound, "GET /user/{account_id} non-numeric")
}

// ─── Emails ──────────────────────────────────────────────────────────────

func TestUserEmails_AddListDeleteVisibility(t *testing.T) {
	_, tok := userSurfaceUser(t, "emailuser")

	// The account starts with its primary email.
	list := decodeJSONArray(t, ghGet(t, "/api/v3/user/emails", tok))
	if len(list) != 1 || list[0]["email"] != "emailuser@example.com" || list[0]["primary"] != true {
		t.Fatalf("initial email list = %v", list)
	}

	// Add a secondary address.
	resp := ghPost(t, "/api/v3/user/emails", tok, map[string]interface{}{
		"emails": []string{"second@example.com"},
	})
	if resp.StatusCode != http.StatusCreated {
		resp.Body.Close()
		t.Fatalf("POST /user/emails: status = %d, want 201", resp.StatusCode)
	}
	added := decodeJSONArray(t, resp)
	if len(added) != 1 || added[0]["email"] != "second@example.com" || added[0]["primary"] != false {
		t.Fatalf("added emails = %v", added)
	}
	if vis, present := added[0]["visibility"]; !present || vis != nil {
		t.Fatalf("new email visibility should be null, got %v (present=%v)", vis, present)
	}

	list = decodeJSONArray(t, ghGet(t, "/api/v3/user/emails", tok))
	if len(list) != 2 {
		t.Fatalf("email list after add = %v", list)
	}

	// Duplicates are rejected with a validation error.
	mustStatus(t, ghPost(t, "/api/v3/user/emails", tok, map[string]interface{}{
		"emails": []string{"second@example.com"},
	}), http.StatusUnprocessableEntity, "POST duplicate email")

	// No public emails yet.
	pub := decodeJSONArray(t, ghGet(t, "/api/v3/user/public_emails", tok))
	if len(pub) != 0 {
		t.Fatalf("public emails before visibility change = %v", pub)
	}

	// Make the primary email public.
	resp = ghPatch(t, "/api/v3/user/email/visibility", tok, map[string]interface{}{"visibility": "public"})
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("PATCH /user/email/visibility: status = %d, want 200", resp.StatusCode)
	}
	updated := decodeJSONArray(t, resp)
	if len(updated) != 1 || updated[0]["visibility"] != "public" || updated[0]["primary"] != true {
		t.Fatalf("visibility update response = %v", updated)
	}
	pub = decodeJSONArray(t, ghGet(t, "/api/v3/user/public_emails", tok))
	if len(pub) != 1 || pub[0]["email"] != "emailuser@example.com" {
		t.Fatalf("public emails after visibility change = %v", pub)
	}

	// Invalid visibility value is a validation error.
	mustStatus(t, ghPatch(t, "/api/v3/user/email/visibility", tok, map[string]interface{}{"visibility": "internal"}),
		http.StatusUnprocessableEntity, "PATCH visibility invalid")

	// Deleting a secondary address works; the primary is protected.
	mustStatus(t, ghDeleteWithBody(t, "/api/v3/user/emails", tok, map[string]interface{}{
		"emails": []interface{}{"second@example.com"},
	}), http.StatusNoContent, "DELETE secondary email")
	list = decodeJSONArray(t, ghGet(t, "/api/v3/user/emails", tok))
	if len(list) != 1 {
		t.Fatalf("email list after delete = %v", list)
	}
	mustStatus(t, ghDeleteWithBody(t, "/api/v3/user/emails", tok, map[string]interface{}{
		"emails": []interface{}{"emailuser@example.com"},
	}), http.StatusUnprocessableEntity, "DELETE primary email")
	mustStatus(t, ghDeleteWithBody(t, "/api/v3/user/emails", tok, map[string]interface{}{
		"emails": []interface{}{"never-added@example.com"},
	}), http.StatusNotFound, "DELETE unknown email")

	// The bare-array request body form GitHub documents is accepted too.
	resp = ghPost(t, "/api/v3/user/emails", tok, []string{"third@example.com"})
	mustStatus(t, resp, http.StatusCreated, "POST bare-array body")
}

// ─── SSH signing key single read ─────────────────────────────────────────

func TestUserSSHSigningKey_GetByID(t *testing.T) {
	_, tok := userSurfaceUser(t, "signkeyuser")

	resp := ghPost(t, "/api/v3/user/ssh_signing_keys", tok, map[string]interface{}{
		"key":   "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAISignKeyUserTest signkeyuser@example.com",
		"title": "signing key",
	})
	if resp.StatusCode != http.StatusCreated {
		resp.Body.Close()
		t.Fatalf("create ssh signing key: status = %d, want 201", resp.StatusCode)
	}
	created := decodeJSON(t, resp)
	id := int(created["id"].(float64))

	got := decodeJSON(t, ghGet(t, "/api/v3/user/ssh_signing_keys/"+strconv.Itoa(id), tok))
	if int(got["id"].(float64)) != id || got["key"] != created["key"] {
		t.Fatalf("GET ssh signing key = %v, created = %v", got, created)
	}

	mustStatus(t, ghGet(t, "/api/v3/user/ssh_signing_keys/999999", tok), http.StatusNotFound, "GET unknown ssh signing key")
}

// ─── Interaction limits ──────────────────────────────────────────────────

func TestUserInteractionLimits_RoundTrip(t *testing.T) {
	_, tok := userSurfaceUser(t, "limituser")

	// No limit set: 204.
	mustStatus(t, ghGet(t, "/api/v3/user/interaction-limits", tok), http.StatusNoContent, "GET before set")

	// Invalid group is a validation error.
	mustStatus(t, ghPut(t, "/api/v3/user/interaction-limits", tok, map[string]interface{}{
		"limit": "everyone",
	}), http.StatusUnprocessableEntity, "PUT invalid limit")

	// Set a limit.
	resp := ghPut(t, "/api/v3/user/interaction-limits", tok, map[string]interface{}{
		"limit":  "contributors_only",
		"expiry": "one_week",
	})
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("PUT /user/interaction-limits: status = %d, want 200", resp.StatusCode)
	}
	set := decodeJSON(t, resp)
	if set["limit"] != "contributors_only" || set["origin"] != "user" {
		t.Fatalf("PUT response = %v", set)
	}
	expires, err := time.Parse(time.RFC3339, set["expires_at"].(string))
	if err != nil || time.Until(expires) < 6*24*time.Hour {
		t.Fatalf("expires_at %v not ~one week out (err %v)", set["expires_at"], err)
	}

	got := decodeJSON(t, ghGet(t, "/api/v3/user/interaction-limits", tok))
	if got["limit"] != "contributors_only" || got["origin"] != "user" {
		t.Fatalf("GET after set = %v", got)
	}

	mustStatus(t, ghDelete(t, "/api/v3/user/interaction-limits", tok), http.StatusNoContent, "DELETE limits")
	mustStatus(t, ghGet(t, "/api/v3/user/interaction-limits", tok), http.StatusNoContent, "GET after delete")
}

// ─── GitHub Marketplace purchases ────────────────────────────────────────

func TestUserMarketplacePurchases_ListWithRealPlan(t *testing.T) {
	_, tok := userSurfaceUser(t, "marketuser")

	// No purchases yet.
	list := decodeJSONArray(t, ghGet(t, "/api/v3/user/marketplace_purchases", tok))
	if len(list) != 0 {
		t.Fatalf("purchases before seeding = %v", list)
	}

	listing := publishMarketplaceGitHubApp(t, "User Purchases App", testBaseURL+"/health")
	requireMarketplaceStatus(t, marketplaceRequest(t, http.MethodPost,
		"/ui-data/marketplace/listings/"+listing.slug+"/purchase", "token "+tok,
		map[string]interface{}{"plan_id": listing.freePlanID, "billing_cycle": "monthly"}), http.StatusCreated)

	for _, path := range []string{"/api/v3/user/marketplace_purchases", "/api/v3/user/marketplace_purchases/stubbed"} {
		list = decodeJSONArray(t, ghGet(t, path, tok))
		if len(list) != 1 {
			t.Fatalf("%s: purchases = %v", path, list)
		}
		p := list[0]
		if p["billing_cycle"] != "monthly" || p["on_free_trial"] != false || p["unit_count"] != 0.0 {
			t.Fatalf("%s: purchase = %v", path, p)
		}
		account, _ := p["account"].(map[string]interface{})
		if account["login"] != "marketuser" || account["type"] != "User" {
			t.Fatalf("%s: account = %v", path, account)
		}
		plan, _ := p["plan"].(map[string]interface{})
		if plan["name"] != "Community" || plan["price_model"] != "FREE" || plan["accounts_url"] == nil || plan["number"] == nil {
			t.Fatalf("%s: plan = %v", path, plan)
		}
	}
}

// ─── Hovercard ───────────────────────────────────────────────────────────

func TestUserHovercard_FromRealMembershipsAndRepos(t *testing.T) {
	u, tok := userSurfaceUser(t, "hoveruser")
	admin := testServer.store.UsersByLogin["admin"]
	org := testServer.store.CreateOrg(admin, "hover-org", "Hover Org", "")
	testServer.store.SetMembership(org.Login, u.ID, OrgRoleMember, MembershipStateActive)

	card := decodeJSON(t, ghGet(t, "/api/v3/users/hoveruser/hovercard", tok))
	contexts, _ := card["contexts"].([]interface{})
	found := false
	for _, c := range contexts {
		ctx, _ := c.(map[string]interface{})
		if ctx["message"] == "Member of hover-org" && ctx["octicon"] == "organization" {
			found = true
		}
	}
	if !found {
		t.Fatalf("hovercard missing organization membership context: %v", card)
	}

	// Repository subject adds an ownership context for the repo owner.
	repo := testServer.store.CreateRepo(u, "hover-repo", "", false)
	card = decodeJSON(t, ghGet(t, fmt.Sprintf("/api/v3/users/hoveruser/hovercard?subject_type=repository&subject_id=%d", repo.ID), tok))
	contexts, _ = card["contexts"].([]interface{})
	found = false
	for _, c := range contexts {
		ctx, _ := c.(map[string]interface{})
		if ctx["message"] == "Owns this repository" {
			found = true
		}
	}
	if !found {
		t.Fatalf("hovercard missing repository ownership context: %v", card)
	}

	// subject_type without subject_id, and invalid subject types, are 422.
	mustStatus(t, ghGet(t, "/api/v3/users/hoveruser/hovercard?subject_type=repository", tok),
		http.StatusUnprocessableEntity, "hovercard subject_type without subject_id")
	mustStatus(t, ghGet(t, "/api/v3/users/hoveruser/hovercard?subject_type=galaxy&subject_id=1", tok),
		http.StatusUnprocessableEntity, "hovercard invalid subject_type")
	mustStatus(t, ghGet(t, "/api/v3/users/no-such-user-xyz/hovercard", tok),
		http.StatusNotFound, "hovercard unknown user")
}

// ─── GET /user/issues ────────────────────────────────────────────────────

func TestUserIssues_FiltersAcrossRepos(t *testing.T) {
	u, tok := userSurfaceUser(t, "issueuser")
	repo := testServer.store.CreateRepo(u, "issue-repo", "", false)
	if repo == nil {
		t.Fatal("create repo failed")
	}

	resp := ghPost(t, "/api/v3/repos/issueuser/issue-repo/issues", tok, map[string]interface{}{
		"title": "created by me",
	})
	if resp.StatusCode != http.StatusCreated {
		resp.Body.Close()
		t.Fatalf("create issue: status = %d", resp.StatusCode)
	}
	issue := decodeJSON(t, resp)
	num := int(issue["number"].(float64))

	// Default filter is "assigned": nothing assigned yet.
	list := decodeJSONArray(t, ghGet(t, "/api/v3/user/issues", tok))
	if len(list) != 0 {
		t.Fatalf("assigned issues before assignment = %v", list)
	}

	// filter=created finds the authored issue, with its repository attached.
	list = decodeJSONArray(t, ghGet(t, "/api/v3/user/issues?filter=created", tok))
	if len(list) != 1 || list[0]["title"] != "created by me" {
		t.Fatalf("created issues = %v", list)
	}
	repoJSON, _ := list[0]["repository"].(map[string]interface{})
	if repoJSON == nil || repoJSON["full_name"] != "issueuser/issue-repo" {
		t.Fatalf("issue repository member = %v", list[0]["repository"])
	}

	// Assign the user; the default filter now matches.
	mustStatus(t, ghPost(t, fmt.Sprintf("/api/v3/repos/issueuser/issue-repo/issues/%d/assignees", num), tok,
		map[string]interface{}{"assignees": []string{"issueuser"}}), http.StatusCreated, "add assignee")
	list = decodeJSONArray(t, ghGet(t, "/api/v3/user/issues", tok))
	if len(list) != 1 {
		t.Fatalf("assigned issues after assignment = %v", list)
	}

	// Closing the issue removes it from the default open-state listing.
	mustStatus(t, ghPatch(t, fmt.Sprintf("/api/v3/repos/issueuser/issue-repo/issues/%d", num), tok,
		map[string]interface{}{"state": "closed"}), http.StatusOK, "close issue")
	list = decodeJSONArray(t, ghGet(t, "/api/v3/user/issues?filter=created", tok))
	if len(list) != 0 {
		t.Fatalf("open created issues after close = %v", list)
	}
	list = decodeJSONArray(t, ghGet(t, "/api/v3/user/issues?filter=created&state=closed", tok))
	if len(list) != 1 {
		t.Fatalf("closed created issues = %v", list)
	}
}

// ─── Migration repositories ──────────────────────────────────────────────

func TestUserMigrationRepositories_List(t *testing.T) {
	u, tok := userSurfaceUser(t, "miguser")
	repo := testServer.store.CreateRepo(u, "mig-repo", "", false)
	if repo == nil {
		t.Fatal("create repo failed")
	}

	resp := ghPost(t, "/api/v3/user/migrations", tok, map[string]interface{}{
		"repositories": []string{repo.FullName},
	})
	if resp.StatusCode != http.StatusCreated {
		resp.Body.Close()
		t.Fatalf("start migration: status = %d", resp.StatusCode)
	}
	mig := decodeJSON(t, resp)
	id := int(mig["id"].(float64))

	repos := decodeJSONArray(t, ghGet(t, fmt.Sprintf("/api/v3/user/migrations/%d/repositories", id), tok))
	if len(repos) != 1 || repos[0]["full_name"] != "miguser/mig-repo" {
		t.Fatalf("migration repositories = %v", repos)
	}

	// Another user's migration is invisible.
	mustStatus(t, ghGet(t, fmt.Sprintf("/api/v3/user/migrations/%d/repositories", id), defaultToken),
		http.StatusNotFound, "migration repositories cross-user")
}

// ─── Billing usage reports ───────────────────────────────────────────────

func TestUserBillingUsage_FromRealWorkflowRuns(t *testing.T) {
	u, tok := userSurfaceUser(t, "billuser")
	repo := testServer.store.CreateRepo(u, "bill-repo", "", false)
	if repo == nil {
		t.Fatal("create repo failed")
	}

	// Record a completed workflow run with one 90-second job — the real
	// state the report is derived from (metered as 2 rounded-up minutes).
	started := time.Now().UTC().Add(-10 * time.Minute)
	testServer.store.mu.Lock()
	testServer.store.Workflows["bill-run-1"] = &Workflow{
		ID:           "bill-run-1",
		Name:         "bill",
		RunID:        999901,
		Status:       "completed",
		Result:       "success",
		RepoFullName: repo.FullName,
		CreatedAt:    started,
		Jobs: map[string]*WorkflowJob{
			"build": {Key: "build", DisplayName: "build", Status: "completed", Result: "success",
				StartedAt: started, CompletedAt: started.Add(90 * time.Second)},
		},
	}
	testServer.store.mu.Unlock()
	t.Cleanup(func() {
		testServer.store.mu.Lock()
		delete(testServer.store.Workflows, "bill-run-1")
		testServer.store.mu.Unlock()
	})

	report := decodeJSON(t, ghGet(t, "/api/v3/users/billuser/settings/billing/usage", tok))
	items, _ := report["usageItems"].([]interface{})
	if len(items) != 1 {
		t.Fatalf("usageItems = %v", report)
	}
	item, _ := items[0].(map[string]interface{})
	if item["product"] != "Actions" || item["sku"] != "Actions Linux" || item["unitType"] != "minutes" {
		t.Fatalf("usage item = %v", item)
	}
	if item["quantity"] != 2.0 {
		t.Fatalf("quantity = %v, want 2 (90s rounds up)", item["quantity"])
	}
	if item["repositoryName"] != "billuser/bill-repo" {
		t.Fatalf("repositoryName = %v", item["repositoryName"])
	}
	if !strings.HasPrefix(item["date"].(string), started.Format("2006-01-02")) {
		t.Fatalf("date = %v", item["date"])
	}

	// The summary aggregates the same real usage.
	summary := decodeJSON(t, ghGet(t, "/api/v3/users/billuser/settings/billing/usage/summary", tok))
	if summary["user"] != "billuser" {
		t.Fatalf("summary user = %v", summary["user"])
	}
	tp, _ := summary["timePeriod"].(map[string]interface{})
	if tp == nil || tp["year"] == nil {
		t.Fatalf("summary timePeriod = %v", summary["timePeriod"])
	}
	sItems, _ := summary["usageItems"].([]interface{})
	if len(sItems) != 1 {
		t.Fatalf("summary usageItems = %v", summary)
	}
	sItem, _ := sItems[0].(map[string]interface{})
	if sItem["grossQuantity"] != 2.0 || sItem["netQuantity"] != 2.0 || sItem["discountAmount"] != 0.0 {
		t.Fatalf("summary item = %v", sItem)
	}

	// bleephub meters no AI-credit or premium-request products, so those
	// reports carry zero usage items for every account.
	for _, path := range []string{
		"/api/v3/users/billuser/settings/billing/ai_credit/usage",
		"/api/v3/users/billuser/settings/billing/premium_request/usage",
	} {
		rep := decodeJSON(t, ghGet(t, path, tok))
		if rep["user"] != "billuser" {
			t.Fatalf("%s: user = %v", path, rep["user"])
		}
		if items, ok := rep["usageItems"].([]interface{}); !ok || len(items) != 0 {
			t.Fatalf("%s: usageItems = %v", path, rep["usageItems"])
		}
	}

	// Filters: an unmatched repository filter empties the report; a bad
	// year is a 400; another (non-admin) user is forbidden.
	filtered := decodeJSON(t, ghGet(t, "/api/v3/users/billuser/settings/billing/usage?repository=billuser/other", tok))
	if items, _ := filtered["usageItems"].([]interface{}); len(items) != 0 {
		t.Fatalf("filtered usageItems = %v", filtered)
	}
	mustStatus(t, ghGet(t, "/api/v3/users/billuser/settings/billing/usage?year=abc", tok),
		http.StatusBadRequest, "billing usage bad year")

	_, otherTok := userSurfaceUser(t, "billspy")
	mustStatus(t, ghGet(t, "/api/v3/users/billuser/settings/billing/usage", otherTok),
		http.StatusForbidden, "billing usage other user")

	// A site administrator can read any account's usage.
	adminView := decodeJSON(t, ghGet(t, "/api/v3/users/billuser/settings/billing/usage", defaultToken))
	if items, _ := adminView["usageItems"].([]interface{}); len(items) != 1 {
		t.Fatalf("admin view usageItems = %v", adminView)
	}
	_ = u
}
