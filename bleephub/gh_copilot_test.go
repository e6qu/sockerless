package bleephub

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

// copilotTestOrg creates an org owned by the seeded admin plus active
// member users, returning the org and the members.
func copilotTestOrg(t *testing.T, login string, memberLogins ...string) (*Org, []*User) {
	t.Helper()
	admin := testServer.store.LookupUserByLogin("admin")
	org := testServer.store.CreateOrg(admin, login, login, "")
	if org == nil {
		t.Fatalf("CreateOrg(%q) returned nil", login)
	}
	members := make([]*User, 0, len(memberLogins))
	for _, ml := range memberLogins {
		u := seedTestUser(testServer, ml)
		testServer.store.SetMembership(org.Login, u.ID, OrgRoleMember, MembershipStateActive)
		members = append(members, u)
	}
	return org, members
}

func TestOrgCopilotBillingSeats_AddListCancelReinstate(t *testing.T) {
	org, members := copilotTestOrg(t, "copilot-billing-org", "copilot-alice", "copilot-bob")
	base := "/api/v3/orgs/" + org.Login

	// Assign seats to both members.
	resp := ghPost(t, base+"/copilot/billing/selected_users", defaultToken,
		map[string]interface{}{"selected_usernames": []string{"copilot-alice", "copilot-bob"}})
	created := decodeJSONWithStatus(t, resp, 201)
	if got := created["seats_created"]; got != float64(2) {
		t.Fatalf("seats_created = %v, want 2", got)
	}

	// Billing details reflect the real seat state.
	billing := decodeJSONWithStatus(t, ghGet(t, base+"/copilot/billing", defaultToken), 200)
	breakdown, ok := billing["seat_breakdown"].(map[string]interface{})
	if !ok {
		t.Fatalf("seat_breakdown missing: %v", billing)
	}
	if breakdown["total"] != float64(2) || breakdown["added_this_cycle"] != float64(2) || breakdown["pending_cancellation"] != float64(0) {
		t.Fatalf("seat_breakdown = %v, want total=2 added_this_cycle=2 pending_cancellation=0", breakdown)
	}
	if billing["seat_management_setting"] != "assign_selected" || billing["plan_type"] != "business" {
		t.Fatalf("billing policies = %v", billing)
	}
	if billing["public_code_suggestions"] == nil {
		t.Fatalf("public_code_suggestions missing: %v", billing)
	}

	// Seat listing carries full seat objects.
	seatsBody := decodeJSONWithStatus(t, ghGet(t, base+"/copilot/billing/seats", defaultToken), 200)
	if seatsBody["total_seats"] != float64(2) {
		t.Fatalf("total_seats = %v, want 2", seatsBody["total_seats"])
	}
	seats := seatsBody["seats"].([]interface{})
	if len(seats) != 2 {
		t.Fatalf("len(seats) = %d, want 2", len(seats))
	}
	first := seats[0].(map[string]interface{})
	if first["assignee"].(map[string]interface{})["login"] != "copilot-alice" {
		t.Fatalf("first seat assignee = %v", first["assignee"])
	}
	if first["created_at"] == nil || first["pending_cancellation_date"] != nil {
		t.Fatalf("seat = %v, want created_at set and no pending cancellation", first)
	}
	if first["organization"].(map[string]interface{})["login"] != org.Login {
		t.Fatalf("seat organization = %v", first["organization"])
	}

	// Seat listing pagination.
	pageTwo := decodeJSONWithStatus(t, ghGet(t, base+"/copilot/billing/seats?per_page=1&page=2", defaultToken), 200)
	if pageTwo["total_seats"] != float64(2) {
		t.Fatalf("paged total_seats = %v, want 2", pageTwo["total_seats"])
	}
	if page := pageTwo["seats"].([]interface{}); len(page) != 1 {
		t.Fatalf("page 2 has %d seats, want 1", len(page))
	}

	// Member seat detail is consistent with the billing state.
	seat := decodeJSONWithStatus(t, ghGet(t, base+"/members/copilot-alice/copilot", defaultToken), 200)
	if seat["assignee"].(map[string]interface{})["login"] != "copilot-alice" || seat["plan_type"] != "business" {
		t.Fatalf("member seat = %v", seat)
	}

	// A member views their own seat.
	aliceTok := testServer.store.CreateToken(members[0].ID, "read:org").Value
	requireStatus(t, ghGet(t, base+"/members/copilot-alice/copilot", aliceTok), 200)

	// Cancel one seat: it goes pending-cancellation at the next cycle,
	// still counted in the total.
	resp = ghDeleteWithBody(t, base+"/copilot/billing/selected_users", defaultToken,
		map[string]interface{}{"selected_usernames": []string{"copilot-alice"}})
	cancelled := decodeJSONWithStatus(t, resp, 200)
	if cancelled["seats_cancelled"] != float64(1) {
		t.Fatalf("seats_cancelled = %v, want 1", cancelled["seats_cancelled"])
	}
	billing = decodeJSONWithStatus(t, ghGet(t, base+"/copilot/billing", defaultToken), 200)
	breakdown = billing["seat_breakdown"].(map[string]interface{})
	if breakdown["total"] != float64(2) || breakdown["pending_cancellation"] != float64(1) {
		t.Fatalf("post-cancel breakdown = %v, want total=2 pending_cancellation=1", breakdown)
	}
	seat = decodeJSONWithStatus(t, ghGet(t, base+"/members/copilot-alice/copilot", defaultToken), 200)
	date, _ := seat["pending_cancellation_date"].(string)
	if _, err := time.Parse("2006-01-02", date); err != nil {
		t.Fatalf("pending_cancellation_date = %v, want YYYY-MM-DD", seat["pending_cancellation_date"])
	}

	// Re-adding reinstates the pending seat.
	resp = ghPost(t, base+"/copilot/billing/selected_users", defaultToken,
		map[string]interface{}{"selected_usernames": []string{"copilot-alice"}})
	if got := decodeJSONWithStatus(t, resp, 201)["seats_created"]; got != float64(1) {
		t.Fatalf("reinstate seats_created = %v, want 1", got)
	}
	seat = decodeJSONWithStatus(t, ghGet(t, base+"/members/copilot-alice/copilot", defaultToken), 200)
	if seat["pending_cancellation_date"] != nil {
		t.Fatalf("reinstated seat still pending: %v", seat)
	}

	// Re-adding a fully active seat creates nothing.
	resp = ghPost(t, base+"/copilot/billing/selected_users", defaultToken,
		map[string]interface{}{"selected_usernames": []string{"copilot-bob"}})
	if got := decodeJSONWithStatus(t, resp, 201)["seats_created"]; got != float64(0) {
		t.Fatalf("re-add seats_created = %v, want 0", got)
	}
}

func TestOrgCopilotBillingSeats_ValidationAndPermissions(t *testing.T) {
	org, members := copilotTestOrg(t, "copilot-validation-org", "copilot-val-member")
	base := "/api/v3/orgs/" + org.Login
	outsider := seedTestUser(testServer, "copilot-outsider")

	// Unknown username → 422, nothing assigned.
	requireStatus(t, ghPost(t, base+"/copilot/billing/selected_users", defaultToken,
		map[string]interface{}{"selected_usernames": []string{"copilot-no-such-user"}}), 422)

	// Existing user who is not an org member → 422.
	requireStatus(t, ghPost(t, base+"/copilot/billing/selected_users", defaultToken,
		map[string]interface{}{"selected_usernames": []string{outsider.Login}}), 422)

	// Empty list → 422.
	requireStatus(t, ghPost(t, base+"/copilot/billing/selected_users", defaultToken,
		map[string]interface{}{"selected_usernames": []string{}}), 422)

	// Non-owner member → 403 on both reads and writes.
	memberTok := testServer.store.CreateToken(members[0].ID, "admin:org").Value
	requireStatus(t, ghGet(t, base+"/copilot/billing", memberTok), 403)
	requireStatus(t, ghPost(t, base+"/copilot/billing/selected_users", memberTok,
		map[string]interface{}{"selected_usernames": []string{members[0].Login}}), 403)

	// Unknown org → 404.
	requireStatus(t, ghGet(t, "/api/v3/orgs/copilot-no-such-org/copilot/billing", defaultToken), 404)

	// Member without a seat → 404; unknown member → 404.
	requireStatus(t, ghGet(t, base+"/members/"+members[0].Login+"/copilot", defaultToken), 404)
	requireStatus(t, ghGet(t, base+"/members/copilot-no-such-user/copilot", defaultToken), 404)

	// Member with a pending invitation → 422.
	invited := seedTestUser(testServer, "copilot-invited")
	testServer.store.SetMembership(org.Login, invited.ID, OrgRoleMember, MembershipStatePending)
	requireStatus(t, ghGet(t, base+"/members/"+invited.Login+"/copilot", defaultToken), 422)
}

func TestOrgCopilotBillingSeats_Teams(t *testing.T) {
	org, members := copilotTestOrg(t, "copilot-teams-org", "copilot-team-user1", "copilot-team-user2")
	base := "/api/v3/orgs/" + org.Login
	team := testServer.store.CreateTeam(org.Login, "copilot engineers", TeamOptions{})
	if team == nil {
		t.Fatal("CreateTeam returned nil")
	}
	for _, m := range members {
		testServer.store.SetTeamMembership(org.Login, team.Slug, m.ID, TeamRoleMember)
	}

	// Unknown team → 422.
	requireStatus(t, ghPost(t, base+"/copilot/billing/selected_teams", defaultToken,
		map[string]interface{}{"selected_teams": []string{"no-such-team"}}), 422)

	// Assign by team name: every member gets a seat.
	resp := ghPost(t, base+"/copilot/billing/selected_teams", defaultToken,
		map[string]interface{}{"selected_teams": []string{"copilot engineers"}})
	if got := decodeJSONWithStatus(t, resp, 201)["seats_created"]; got != float64(2) {
		t.Fatalf("team seats_created = %v, want 2", got)
	}

	// The seat records its assigning team.
	seat := decodeJSONWithStatus(t, ghGet(t, base+"/members/"+members[0].Login+"/copilot", defaultToken), 200)
	at, ok := seat["assigning_team"].(map[string]interface{})
	if !ok || at["slug"] != team.Slug {
		t.Fatalf("assigning_team = %v, want slug %q", seat["assigning_team"], team.Slug)
	}

	// Team-assigned seats cannot be cancelled per user.
	requireStatus(t, ghDeleteWithBody(t, base+"/copilot/billing/selected_users", defaultToken,
		map[string]interface{}{"selected_usernames": []string{members[0].Login}}), 422)

	// Cancelling the team cancels every seat it assigned.
	resp = ghDeleteWithBody(t, base+"/copilot/billing/selected_teams", defaultToken,
		map[string]interface{}{"selected_teams": []string{team.Slug}})
	if got := decodeJSONWithStatus(t, resp, 200)["seats_cancelled"]; got != float64(2) {
		t.Fatalf("team seats_cancelled = %v, want 2", got)
	}
	billing := decodeJSONWithStatus(t, ghGet(t, base+"/copilot/billing", defaultToken), 200)
	breakdown := billing["seat_breakdown"].(map[string]interface{})
	if breakdown["pending_cancellation"] != float64(2) {
		t.Fatalf("breakdown = %v, want pending_cancellation=2", breakdown)
	}
}

func TestOrgCopilotMetrics_HonestlyEmptyWithoutActivity(t *testing.T) {
	org, _ := copilotTestOrg(t, "copilot-metrics-org", "copilot-metrics-member")
	base := "/api/v3/orgs/" + org.Login
	team := testServer.store.CreateTeam(org.Login, "metrics team", TeamOptions{})

	// No Copilot telemetry is recorded → empty metrics arrays.
	for _, path := range []string{base + "/copilot/metrics", base + "/team/" + team.Slug + "/copilot/metrics"} {
		resp := ghGet(t, path, defaultToken)
		if resp.StatusCode != 200 {
			t.Fatalf("GET %s = %d, want 200", path, resp.StatusCode)
		}
		var days []interface{}
		if err := json.NewDecoder(resp.Body).Decode(&days); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
		resp.Body.Close()
		if len(days) != 0 {
			t.Fatalf("%s returned %d fabricated day(s)", path, len(days))
		}
	}

	// Unknown team → 404; malformed since → 422.
	requireStatus(t, ghGet(t, base+"/team/no-such-team/copilot/metrics", defaultToken), 404)
	requireStatus(t, ghGet(t, base+"/copilot/metrics?since=yesterday", defaultToken), 422)

	// Single-day reports: the day parameter is required and validated;
	// with no activity there is no report for any day → 204.
	for _, report := range []string{"organization-1-day", "users-1-day", "user-teams-1-day"} {
		path := base + "/copilot/metrics/reports/" + report
		requireStatus(t, ghGet(t, path, defaultToken), 422)
		requireStatus(t, ghGet(t, path+"?day=not-a-day", defaultToken), 422)
		requireStatus(t, ghGet(t, path+"?day=2026-07-01", defaultToken), 204)
	}

	// Latest 28-day reports: a real 28-day window with nothing to download.
	for _, report := range []string{"organization-28-day", "users-28-day"} {
		body := decodeJSONWithStatus(t, ghGet(t, base+"/copilot/metrics/reports/"+report+"/latest", defaultToken), 200)
		links, ok := body["download_links"].([]interface{})
		if !ok || len(links) != 0 {
			t.Fatalf("download_links = %v, want empty array", body["download_links"])
		}
		start, err1 := time.Parse("2006-01-02", body["report_start_day"].(string))
		end, err2 := time.Parse("2006-01-02", body["report_end_day"].(string))
		if err1 != nil || err2 != nil || end.Sub(start) != 27*24*time.Hour {
			t.Fatalf("report window = %v..%v, want a 28-day window", body["report_start_day"], body["report_end_day"])
		}
	}
}

func TestOrgCopilotContentExclusion_RoundTrip(t *testing.T) {
	org, _ := copilotTestOrg(t, "copilot-exclusion-org")
	base := "/api/v3/orgs/" + org.Login + "/copilot/content_exclusion"

	// Unconfigured organizations have no rules.
	if rules := decodeJSONWithStatus(t, ghGet(t, base, defaultToken), 200); len(rules) != 0 {
		t.Fatalf("unconfigured rules = %v, want empty", rules)
	}

	// Configure path rules and read them back verbatim.
	rules := map[string]interface{}{
		"copilot-exclusion-org/secrets-repo": []interface{}{"/src/some-dir/kernel.rs", "secrets.json"},
		"*":                                  []interface{}{"/scripts/**"},
	}
	if msg := decodeJSONWithStatus(t, ghPut(t, base, defaultToken, rules), 200); msg["message"] == nil {
		t.Fatalf("PUT response = %v, want a message", msg)
	}
	got := decodeJSONWithStatus(t, ghGet(t, base, defaultToken), 200)
	if len(got) != 2 {
		t.Fatalf("rules = %v, want 2 scopes", got)
	}
	if scope := got["copilot-exclusion-org/secrets-repo"].([]interface{}); len(scope) != 2 || scope[0] != "/src/some-dir/kernel.rs" {
		t.Fatalf("scope rules = %v", scope)
	}

	// ifAnyMatch / ifNoneMatch rule objects are accepted.
	requireStatus(t, ghPut(t, base, defaultToken, map[string]interface{}{
		"copilot-exclusion-org/secrets-repo": []interface{}{
			map[string]interface{}{"ifAnyMatch": []string{"gitlab.com/**"}},
		},
	}), 200)

	// A rule that is neither a path string nor a match object → 422.
	requireStatus(t, ghPut(t, base, defaultToken, map[string]interface{}{"*": []interface{}{42}}), 422)
	requireStatus(t, ghPut(t, base, defaultToken, map[string]interface{}{
		"*": []interface{}{map[string]interface{}{"ifSomethingElse": []string{"x"}}},
	}), 422)
}

func TestOrgCopilotCodingAgentPermissions_RepoScopedPolicy(t *testing.T) {
	org, _ := copilotTestOrg(t, "copilot-agent-org")
	admin := testServer.store.LookupUserByLogin("admin")
	repo1 := testServer.store.CreateOrgRepo(org, admin, "agent-repo-1", "", false)
	repo2 := testServer.store.CreateOrgRepo(org, admin, "agent-repo-2", "", false)
	foreign := testServer.store.CreateRepo(admin, "copilot-agent-foreign-repo", "", false)
	base := "/api/v3/orgs/" + org.Login + "/copilot/coding-agent/permissions"

	// Default policy: all repositories, no selected_repositories_url.
	perms := decodeJSONWithStatus(t, ghGet(t, base, defaultToken), 200)
	if perms["enabled_repositories"] != "all" {
		t.Fatalf("default enabled_repositories = %v, want all", perms["enabled_repositories"])
	}
	if _, present := perms["selected_repositories_url"]; present {
		t.Fatalf("selected_repositories_url present on non-selected policy: %v", perms)
	}

	// The selected-repositories sub-resource conflicts until the policy
	// is "selected".
	requireStatus(t, ghGet(t, base+"/repositories", defaultToken), 409)
	requireStatus(t, ghPut(t, base+"/repositories/"+fmt.Sprint(repo1.ID), defaultToken, nil), 409)

	// Invalid policy value → 422.
	requireStatus(t, ghPut(t, base, defaultToken, map[string]interface{}{"enabled_repositories": "some"}), 422)

	// Switch to selected.
	requireStatus(t, ghPut(t, base, defaultToken, map[string]interface{}{"enabled_repositories": "selected"}), 204)
	perms = decodeJSONWithStatus(t, ghGet(t, base, defaultToken), 200)
	if perms["enabled_repositories"] != "selected" || perms["selected_repositories_url"] == nil {
		t.Fatalf("selected policy = %v", perms)
	}

	// Replace the selected list; only org-owned repositories qualify.
	requireStatus(t, ghPut(t, base+"/repositories", defaultToken,
		map[string]interface{}{"selected_repository_ids": []int{repo1.ID}}), 204)
	requireStatus(t, ghPut(t, base+"/repositories", defaultToken,
		map[string]interface{}{"selected_repository_ids": []int{foreign.ID}}), 422)
	requireStatus(t, ghPut(t, base+"/repositories", defaultToken,
		map[string]interface{}{"selected_repository_ids": []int{999999}}), 422)

	// Enable a second repository individually; unknown ID → 404;
	// a repository outside the organization → 422.
	requireStatus(t, ghPut(t, base+"/repositories/"+fmt.Sprint(repo2.ID), defaultToken, nil), 204)
	requireStatus(t, ghPut(t, base+"/repositories/999999", defaultToken, nil), 404)
	requireStatus(t, ghPut(t, base+"/repositories/"+fmt.Sprint(foreign.ID), defaultToken, nil), 422)

	list := decodeJSONWithStatus(t, ghGet(t, base+"/repositories", defaultToken), 200)
	if list["total_count"] != float64(2) {
		t.Fatalf("total_count = %v, want 2", list["total_count"])
	}
	repos := list["repositories"].([]interface{})
	if repos[0].(map[string]interface{})["name"] != "agent-repo-1" {
		t.Fatalf("repositories[0] = %v", repos[0])
	}

	// Disable one.
	requireStatus(t, ghDelete(t, base+"/repositories/"+fmt.Sprint(repo1.ID), defaultToken), 204)
	list = decodeJSONWithStatus(t, ghGet(t, base+"/repositories", defaultToken), 200)
	if list["total_count"] != float64(1) {
		t.Fatalf("post-delete total_count = %v, want 1", list["total_count"])
	}
}

func TestRepoCopilotCloudAgentConfiguration(t *testing.T) {
	admin := testServer.store.LookupUserByLogin("admin")
	repo := testServer.store.CreateRepo(admin, "cloud-agent-config-repo", "", false)
	path := "/api/v3/repos/" + repo.FullName + "/copilot/cloud-agent/configuration"

	cfg := decodeJSONWithStatus(t, ghGet(t, path, defaultToken), 200)
	if v, present := cfg["mcp_configuration"]; !present || v != nil {
		t.Fatalf("mcp_configuration = %v, want explicit null", v)
	}
	tools, ok := cfg["enabled_tools"].(map[string]interface{})
	if !ok {
		t.Fatalf("enabled_tools missing: %v", cfg)
	}
	for _, tool := range []string{"codeql", "copilot_code_review", "secret_scanning", "dependency_vulnerability_checks"} {
		if _, ok := tools[tool].(bool); !ok {
			t.Fatalf("enabled_tools.%s missing or not boolean: %v", tool, tools)
		}
	}
	for _, field := range []string{"require_actions_workflow_approval", "is_firewall_enabled", "is_firewall_recommended_allowlist_enabled"} {
		if _, ok := cfg[field].(bool); !ok {
			t.Fatalf("%s missing or not boolean: %v", field, cfg)
		}
	}
	if allow, ok := cfg["custom_allowlist"].([]interface{}); !ok || len(allow) != 0 {
		t.Fatalf("custom_allowlist = %v, want empty array", cfg["custom_allowlist"])
	}

	requireStatus(t, ghGet(t, "/api/v3/repos/admin/no-such-repo/copilot/cloud-agent/configuration", defaultToken), 404)
	requireStatus(t, ghGet(t, path, ""), 401)
}
