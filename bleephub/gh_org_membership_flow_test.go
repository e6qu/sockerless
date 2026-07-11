package bleephub

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// waitFor polls cond until it holds or a 15s deadline expires (webhook
// deliveries are asynchronous).
func waitFor(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal(msg)
}

// Integration flows over the shared test server for the Organizations
// surface: the invitation lifecycle (PUT membership → pending → user-side
// accept → active), member/public-member endpoints, team hierarchy + roles
// + repos, the global org list, org profile fields, and org webhooks.

// newSharedServerUser registers a user + PAT directly in the shared
// server's store (bleephub has no REST user-creation endpoint — real
// GitHub provisions users out of band too).
func newSharedServerUser(t *testing.T, login string) (*User, string) {
	t.Helper()
	st := testServer.store
	st.mu.Lock()
	if existing := st.UsersByLogin[login]; existing != nil {
		st.mu.Unlock()
		t.Fatalf("user %q already exists", login)
	}
	u := &User{ID: st.NextUser, Login: login, Type: "User"}
	st.NextUser++
	st.Users[u.ID] = u
	st.UsersByLogin[login] = u
	tok := &Token{Value: "ghp_" + login + "0000000000000000000000000000000000", UserID: u.ID, Scopes: "repo,read:org"}
	st.Tokens[tok.Value] = tok
	st.mu.Unlock()
	return u, tok.Value
}

func TestOrgInvitationLifecycle(t *testing.T) {
	createOrgViaAdminAPI(t, "invite-org")
	_, inviteeToken := newSharedServerUser(t, "invitee")

	// Admin PUTs a membership for a non-member → pending invitation.
	put := ghPut(t, "/api/v3/orgs/invite-org/memberships/invitee", defaultToken,
		map[string]interface{}{"role": "member"})
	if put.StatusCode != http.StatusOK {
		put.Body.Close()
		t.Fatalf("PUT membership: got %d, want 200", put.StatusCode)
	}
	m := decodeJSON(t, put)
	if m["state"] != "pending" {
		t.Fatalf("new membership state = %v, want pending", m["state"])
	}

	// Pending members are NOT in the members list…
	members := ghGet(t, "/api/v3/orgs/invite-org/members", defaultToken)
	var memberList []map[string]interface{}
	if err := json.NewDecoder(members.Body).Decode(&memberList); err != nil {
		t.Fatal(err)
	}
	members.Body.Close()
	for _, u := range memberList {
		if u["login"] == "invitee" {
			t.Fatal("pending member leaked into the members list")
		}
	}
	// …and the membership check 404s.
	check := ghGet(t, "/api/v3/orgs/invite-org/members/invitee", defaultToken)
	check.Body.Close()
	if check.StatusCode != http.StatusNotFound {
		t.Fatalf("member check while pending: got %d, want 404", check.StatusCode)
	}

	// The invitee sees the pending membership on the user side.
	mine := ghGet(t, "/api/v3/user/memberships/orgs?state=pending", inviteeToken)
	var myMemberships []map[string]interface{}
	if err := json.NewDecoder(mine.Body).Decode(&myMemberships); err != nil {
		t.Fatal(err)
	}
	mine.Body.Close()
	if len(myMemberships) != 1 {
		t.Fatalf("user pending memberships = %d, want 1", len(myMemberships))
	}

	single := ghGet(t, "/api/v3/user/memberships/orgs/invite-org", inviteeToken)
	if single.StatusCode != http.StatusOK {
		single.Body.Close()
		t.Fatalf("GET own membership: got %d, want 200", single.StatusCode)
	}
	if got := decodeJSON(t, single); got["state"] != "pending" {
		t.Fatalf("own membership state = %v, want pending", got["state"])
	}

	// Accept: PATCH {"state":"active"}. Any other state value is invalid.
	bad := ghPatch(t, "/api/v3/user/memberships/orgs/invite-org", inviteeToken,
		map[string]interface{}{"state": "rejected"})
	bad.Body.Close()
	if bad.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("PATCH bad state: got %d, want 422", bad.StatusCode)
	}
	accept := ghPatch(t, "/api/v3/user/memberships/orgs/invite-org", inviteeToken,
		map[string]interface{}{"state": "active"})
	if accept.StatusCode != http.StatusOK {
		accept.Body.Close()
		t.Fatalf("accept: got %d, want 200", accept.StatusCode)
	}
	if got := decodeJSON(t, accept); got["state"] != "active" {
		t.Fatalf("accepted state = %v, want active", got["state"])
	}

	// Now the member check answers 204 and removal works.
	check2 := ghGet(t, "/api/v3/orgs/invite-org/members/invitee", defaultToken)
	check2.Body.Close()
	if check2.StatusCode != http.StatusNoContent {
		t.Fatalf("member check after accept: got %d, want 204", check2.StatusCode)
	}
	rm := ghDelete(t, "/api/v3/orgs/invite-org/members/invitee", defaultToken)
	rm.Body.Close()
	if rm.StatusCode != http.StatusNoContent {
		t.Fatalf("remove member: got %d, want 204", rm.StatusCode)
	}
	check3 := ghGet(t, "/api/v3/orgs/invite-org/members/invitee", defaultToken)
	check3.Body.Close()
	if check3.StatusCode != http.StatusNotFound {
		t.Fatalf("member check after removal: got %d, want 404", check3.StatusCode)
	}
}

func TestOrgPublicMembers(t *testing.T) {
	createOrgViaAdminAPI(t, "pub-org")
	_, memberToken := newSharedServerUser(t, "pubmember")

	ghPut(t, "/api/v3/orgs/pub-org/memberships/pubmember", defaultToken,
		map[string]interface{}{"role": "member"}).Body.Close()
	ghPatch(t, "/api/v3/user/memberships/orgs/pub-org", memberToken,
		map[string]interface{}{"state": "active"}).Body.Close()

	// Membership starts concealed.
	hidden := ghGet(t, "/api/v3/orgs/pub-org/public_members/pubmember", defaultToken)
	hidden.Body.Close()
	if hidden.StatusCode != http.StatusNotFound {
		t.Fatalf("concealed check: got %d, want 404", hidden.StatusCode)
	}

	// Only the member themselves can publicize — the admin gets 403.
	asAdmin := ghPut(t, "/api/v3/orgs/pub-org/public_members/pubmember", defaultToken, nil)
	asAdmin.Body.Close()
	if asAdmin.StatusCode != http.StatusForbidden {
		t.Fatalf("publicize by other user: got %d, want 403", asAdmin.StatusCode)
	}
	pub := ghPut(t, "/api/v3/orgs/pub-org/public_members/pubmember", memberToken, nil)
	pub.Body.Close()
	if pub.StatusCode != http.StatusNoContent {
		t.Fatalf("publicize: got %d, want 204", pub.StatusCode)
	}

	// Listed (anonymously) + check 204.
	list := ghGet(t, "/api/v3/orgs/pub-org/public_members", "")
	var publicMembers []map[string]interface{}
	if err := json.NewDecoder(list.Body).Decode(&publicMembers); err != nil {
		t.Fatal(err)
	}
	list.Body.Close()
	if len(publicMembers) != 1 || publicMembers[0]["login"] != "pubmember" {
		t.Fatalf("public members = %v, want [pubmember]", publicMembers)
	}

	// Conceal again.
	conceal := ghDelete(t, "/api/v3/orgs/pub-org/public_members/pubmember", memberToken)
	conceal.Body.Close()
	if conceal.StatusCode != http.StatusNoContent {
		t.Fatalf("conceal: got %d, want 204", conceal.StatusCode)
	}
	gone := ghGet(t, "/api/v3/orgs/pub-org/public_members/pubmember", defaultToken)
	gone.Body.Close()
	if gone.StatusCode != http.StatusNotFound {
		t.Fatalf("check after conceal: got %d, want 404", gone.StatusCode)
	}
}

func TestOrganizationsGlobalList(t *testing.T) {
	createOrgViaAdminAPI(t, "global-list-org")

	// Walk the since cursor the way a real client enumerates
	// /organizations: the shared test server accumulates organizations
	// from every test, so the created one may sit past the first page.
	var orgID float64
	since := 0
	for orgID == 0 {
		resp := ghGet(t, "/api/v3/organizations?per_page=100&since="+jsonNumber(float64(since)), defaultToken)
		var orgs []map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&orgs); err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if len(orgs) == 0 {
			break
		}
		for _, o := range orgs {
			if o["login"] == "global-list-org" {
				orgID = o["id"].(float64)
			}
		}
		since = int(orgs[len(orgs)-1]["id"].(float64))
	}
	if orgID == 0 {
		t.Fatal("created org missing from /organizations")
	}

	// The since cursor excludes orgs up to and including the id.
	resp2 := ghGet(t, "/api/v3/organizations?since="+jsonNumber(orgID), defaultToken)
	var after []map[string]interface{}
	if err := json.NewDecoder(resp2.Body).Decode(&after); err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()
	for _, o := range after {
		if o["id"].(float64) <= orgID {
			t.Fatalf("since=%v returned org id %v", orgID, o["id"])
		}
	}
}

func jsonNumber(f float64) string {
	b, _ := json.Marshal(int(f))
	return string(b)
}

func TestOrgProfileFieldsPatch(t *testing.T) {
	createOrgViaAdminAPI(t, "profile-org")

	patch := ghPatch(t, "/api/v3/orgs/profile-org", defaultToken, map[string]interface{}{
		"company":                         "ACME Holdings",
		"location":                        "Rotterdam",
		"blog":                            "https://blog.example.test",
		"twitter_username":                "acme",
		"billing_email":                   "billing@example.test",
		"default_repository_permission":   "write",
		"members_can_create_repositories": false,
		"web_commit_signoff_required":     true,
	})
	if patch.StatusCode != http.StatusOK {
		patch.Body.Close()
		t.Fatalf("PATCH org: got %d, want 200", patch.StatusCode)
	}
	got := decodeJSON(t, patch)
	for field, want := range map[string]interface{}{
		"company":                         "ACME Holdings",
		"location":                        "Rotterdam",
		"blog":                            "https://blog.example.test",
		"twitter_username":                "acme",
		"billing_email":                   "billing@example.test",
		"default_repository_permission":   "write",
		"members_can_create_repositories": false,
		"web_commit_signoff_required":     true,
	} {
		if got[field] != want {
			t.Errorf("%s = %v, want %v", field, got[field], want)
		}
	}

	// Enum validation on default_repository_permission.
	bad := ghPatch(t, "/api/v3/orgs/profile-org", defaultToken, map[string]interface{}{
		"default_repository_permission": "superuser",
	})
	bad.Body.Close()
	if bad.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("invalid enum: got %d, want 422", bad.StatusCode)
	}

	// An untouched org serves GitHub's defaults.
	createOrgViaAdminAPI(t, "default-org")
	fresh := ghGet(t, "/api/v3/orgs/default-org", defaultToken)
	fGot := decodeJSON(t, fresh)
	if fGot["default_repository_permission"] != "read" {
		t.Errorf("default default_repository_permission = %v, want read", fGot["default_repository_permission"])
	}
	if fGot["members_can_create_repositories"] != true {
		t.Errorf("default members_can_create_repositories = %v, want true", fGot["members_can_create_repositories"])
	}
}

func TestTeamHierarchyAndRoles(t *testing.T) {
	createOrgViaAdminAPI(t, "team-org")
	ghPost(t, "/api/v3/orgs/team-org/repos", defaultToken, map[string]interface{}{"name": "team-repo"}).Body.Close()

	// Parent team, seeding a maintainer + a repo from the create body.
	parentResp := ghPost(t, "/api/v3/orgs/team-org/teams", defaultToken, map[string]interface{}{
		"name":        "Platform",
		"privacy":     "closed",
		"permission":  "push",
		"maintainers": []string{"admin"},
		"repo_names":  []string{"team-org/team-repo"},
	})
	if parentResp.StatusCode != http.StatusCreated {
		parentResp.Body.Close()
		t.Fatalf("create parent team: got %d, want 201", parentResp.StatusCode)
	}
	parent := decodeJSON(t, parentResp)
	parentID := int(parent["id"].(float64))
	if parent["notification_setting"] != "notifications_enabled" {
		t.Errorf("default notification_setting = %v", parent["notification_setting"])
	}

	// Child team referencing the parent.
	childResp := ghPost(t, "/api/v3/orgs/team-org/teams", defaultToken, map[string]interface{}{
		"name":                 "Platform Infra",
		"parent_team_id":       parentID,
		"notification_setting": "notifications_disabled",
	})
	if childResp.StatusCode != http.StatusCreated {
		childResp.Body.Close()
		t.Fatalf("create child team: got %d, want 201", childResp.StatusCode)
	}
	child := decodeJSON(t, childResp)
	childParent, _ := child["parent"].(map[string]interface{})
	if childParent == nil || int(childParent["id"].(float64)) != parentID {
		t.Fatalf("child team parent = %v, want id %d", child["parent"], parentID)
	}
	if child["notification_setting"] != "notifications_disabled" {
		t.Errorf("notification_setting = %v", child["notification_setting"])
	}

	// Child teams listing.
	kids := ghGet(t, "/api/v3/orgs/team-org/teams/platform/teams", defaultToken)
	var childTeams []map[string]interface{}
	if err := json.NewDecoder(kids.Body).Decode(&childTeams); err != nil {
		t.Fatal(err)
	}
	kids.Body.Close()
	if len(childTeams) != 1 || childTeams[0]["slug"] != "platform-infra" {
		t.Fatalf("child teams = %v", childTeams)
	}

	// Cycle prevention: re-parenting the parent under its child 422s.
	cyc := ghPatch(t, "/api/v3/orgs/team-org/teams/platform", defaultToken, map[string]interface{}{
		"parent_team_id": int(child["id"].(float64)),
	})
	cyc.Body.Close()
	if cyc.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("cycle re-parent: got %d, want 422", cyc.StatusCode)
	}

	// Maintainer seeded at create reads back with role=maintainer.
	tm := ghGet(t, "/api/v3/orgs/team-org/teams/platform/memberships/admin", defaultToken)
	if tm.StatusCode != http.StatusOK {
		tm.Body.Close()
		t.Fatalf("GET team membership: got %d, want 200", tm.StatusCode)
	}
	if got := decodeJSON(t, tm); got["role"] != "maintainer" || got["state"] != "active" {
		t.Fatalf("team membership = %v, want maintainer/active", got)
	}

	// Role validation + role update via PUT.
	badRole := ghPut(t, "/api/v3/orgs/team-org/teams/platform/memberships/admin", defaultToken,
		map[string]interface{}{"role": "overlord"})
	badRole.Body.Close()
	if badRole.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("bad team role: got %d, want 422", badRole.StatusCode)
	}
	demote := ghPut(t, "/api/v3/orgs/team-org/teams/platform/memberships/admin", defaultToken,
		map[string]interface{}{"role": "member"})
	if demote.StatusCode != http.StatusOK {
		demote.Body.Close()
		t.Fatalf("demote: got %d, want 200", demote.StatusCode)
	}
	if got := decodeJSON(t, demote); got["role"] != "member" {
		t.Fatalf("demoted role = %v, want member", got["role"])
	}

	// Adding a non-org-member to a team auto-invites them (pending).
	newSharedServerUser(t, "teamling")
	invited := ghPut(t, "/api/v3/orgs/team-org/teams/platform/memberships/teamling", defaultToken,
		map[string]interface{}{})
	if invited.StatusCode != http.StatusOK {
		invited.Body.Close()
		t.Fatalf("add non-member: got %d, want 200", invited.StatusCode)
	}
	if got := decodeJSON(t, invited); got["state"] != "pending" {
		t.Fatalf("auto-invited team membership state = %v, want pending", got["state"])
	}

	// Team repos: list carries permissions + role_name; check answers 204
	// or the repository body under the repository media type.
	repos := ghGet(t, "/api/v3/orgs/team-org/teams/platform/repos", defaultToken)
	var teamRepos []map[string]interface{}
	if err := json.NewDecoder(repos.Body).Decode(&teamRepos); err != nil {
		t.Fatal(err)
	}
	repos.Body.Close()
	if len(teamRepos) != 1 || teamRepos[0]["name"] != "team-repo" {
		t.Fatalf("team repos = %v", teamRepos)
	}
	if teamRepos[0]["role_name"] != "write" {
		t.Errorf("role_name = %v, want write (team permission push)", teamRepos[0]["role_name"])
	}
	perms, _ := teamRepos[0]["permissions"].(map[string]interface{})
	if perms == nil || perms["push"] != true || perms["admin"] != false {
		t.Errorf("permissions = %v", perms)
	}

	checkResp := ghGet(t, "/api/v3/orgs/team-org/teams/platform/repos/team-org/team-repo", defaultToken)
	checkResp.Body.Close()
	if checkResp.StatusCode != http.StatusNoContent {
		t.Fatalf("team repo check: got %d, want 204", checkResp.StatusCode)
	}
	mediaReq, _ := http.NewRequest("GET", testBaseURL+"/api/v3/orgs/team-org/teams/platform/repos/team-org/team-repo", nil)
	mediaReq.Header.Set("Authorization", "token "+defaultToken)
	mediaReq.Header.Set("Accept", "application/vnd.github.v3.repository+json")
	mediaResp, err := http.DefaultClient.Do(mediaReq)
	if err != nil {
		t.Fatal(err)
	}
	if mediaResp.StatusCode != http.StatusOK {
		mediaResp.Body.Close()
		t.Fatalf("team repo check (repository media type): got %d, want 200", mediaResp.StatusCode)
	}
	var repoBody map[string]interface{}
	if err := json.NewDecoder(mediaResp.Body).Decode(&repoBody); err != nil {
		t.Fatal(err)
	}
	mediaResp.Body.Close()
	if repoBody["full_name"] != "team-org/team-repo" {
		t.Errorf("repo body = %v", repoBody["full_name"])
	}

	// Team rename re-keys the slug: the new slug resolves, the old 404s.
	rename := ghPatch(t, "/api/v3/orgs/team-org/teams/platform-infra", defaultToken, map[string]interface{}{
		"name": "Infra Core",
	})
	rename.Body.Close()
	if rename.StatusCode != http.StatusOK {
		t.Fatalf("rename: got %d, want 200", rename.StatusCode)
	}
	newSlug := ghGet(t, "/api/v3/orgs/team-org/teams/infra-core", defaultToken)
	newSlug.Body.Close()
	if newSlug.StatusCode != http.StatusOK {
		t.Fatalf("renamed slug lookup: got %d, want 200", newSlug.StatusCode)
	}
	oldSlug := ghGet(t, "/api/v3/orgs/team-org/teams/platform-infra", defaultToken)
	oldSlug.Body.Close()
	if oldSlug.StatusCode != http.StatusNotFound {
		t.Fatalf("stale slug lookup: got %d, want 404", oldSlug.StatusCode)
	}
}

func TestOrgWebhooks(t *testing.T) {
	createOrgViaAdminAPI(t, "hook-org")
	ghPost(t, "/api/v3/orgs/hook-org/repos", defaultToken, map[string]interface{}{"name": "hooked-repo"}).Body.Close()

	// Capture deliveries.
	var mu sync.Mutex
	var events []string
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		events = append(events, r.Header.Get("X-GitHub-Event")+":"+r.Header.Get("X-GitHub-Hook-Installation-Target-Type"))
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer sink.Close()

	// name=web is mandatory on org hooks.
	noName := ghPost(t, "/api/v3/orgs/hook-org/hooks", defaultToken, map[string]interface{}{
		"config": map[string]interface{}{"url": sink.URL},
	})
	noName.Body.Close()
	if noName.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("org hook without name: got %d, want 422", noName.StatusCode)
	}

	create := ghPost(t, "/api/v3/orgs/hook-org/hooks", defaultToken, map[string]interface{}{
		"name":   "web",
		"config": map[string]interface{}{"url": sink.URL, "content_type": "json"},
		"events": []string{"*"},
	})
	if create.StatusCode != http.StatusCreated {
		create.Body.Close()
		t.Fatalf("create org hook: got %d, want 201", create.StatusCode)
	}
	hook := decodeJSON(t, create)
	hookID := int(hook["id"].(float64))
	if hook["type"] != "Organization" {
		t.Errorf("hook type = %v, want Organization", hook["type"])
	}

	// CRUD: get, list, patch.
	get := ghGet(t, jsonHookPath("hook-org", hookID), defaultToken)
	get.Body.Close()
	if get.StatusCode != http.StatusOK {
		t.Fatalf("get org hook: got %d", get.StatusCode)
	}
	patch := ghPatch(t, jsonHookPath("hook-org", hookID), defaultToken, map[string]interface{}{
		"events": []string{"push", "issues", "organization"},
	})
	if patch.StatusCode != http.StatusOK {
		patch.Body.Close()
		t.Fatalf("patch org hook: got %d", patch.StatusCode)
	}
	patched := decodeJSON(t, patch)
	if evs := patched["events"].([]interface{}); len(evs) != 3 {
		t.Errorf("patched events = %v", evs)
	}

	// Ping delivers with the organization target type.
	ping := ghPost(t, jsonHookPath("hook-org", hookID)+"/pings", defaultToken, nil)
	ping.Body.Close()
	if ping.StatusCode != http.StatusNoContent {
		t.Fatalf("ping: got %d, want 204", ping.StatusCode)
	}

	// A repo event on an org-owned repo fans out to the org hook.
	issue := ghPost(t, "/api/v3/repos/hook-org/hooked-repo/issues", defaultToken,
		map[string]interface{}{"title": "fan-out"})
	issue.Body.Close()
	if issue.StatusCode != http.StatusCreated {
		t.Fatalf("create issue: got %d", issue.StatusCode)
	}

	waitFor(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		hasPing, hasIssues := false, false
		for _, e := range events {
			if e == "ping:organization" {
				hasPing = true
			}
			if e == "issues:organization" {
				hasIssues = true
			}
		}
		return hasPing && hasIssues
	}, "org hook did not receive ping + fanned-out issues event")

	// Deliveries are introspectable.
	var deliveryCount int
	waitFor(t, func() bool {
		dl := ghGet(t, jsonHookPath("hook-org", hookID)+"/deliveries", defaultToken)
		var deliveries []map[string]interface{}
		if err := json.NewDecoder(dl.Body).Decode(&deliveries); err != nil {
			t.Fatal(err)
		}
		dl.Body.Close()
		deliveryCount = len(deliveries)
		return deliveryCount >= 2
	}, "org hook deliveries not recorded")

	// Org membership changes emit the organization event to org hooks.
	newSharedServerUser(t, "hookling")
	ghPut(t, "/api/v3/orgs/hook-org/memberships/hookling", defaultToken,
		map[string]interface{}{"role": "member"}).Body.Close()
	waitFor(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		for _, e := range events {
			if e == "organization:organization" {
				return true
			}
		}
		return false
	}, "organization event for member_invited not delivered")

	// Delete; the hook stops resolving.
	del := ghDelete(t, jsonHookPath("hook-org", hookID), defaultToken)
	del.Body.Close()
	if del.StatusCode != http.StatusNoContent {
		t.Fatalf("delete org hook: got %d, want 204", del.StatusCode)
	}
	gone := ghGet(t, jsonHookPath("hook-org", hookID), defaultToken)
	gone.Body.Close()
	if gone.StatusCode != http.StatusNotFound {
		t.Fatalf("get after delete: got %d, want 404", gone.StatusCode)
	}
}

func jsonHookPath(org string, id int) string {
	return "/api/v3/orgs/" + org + "/hooks/" + jsonNumber(float64(id))
}

// TestGraphQLUserOrganizations mirrors the exact query `gh org list` sends:
// a root user(login:) lookup with the organizations connection.
func TestGraphQLUserOrganizations(t *testing.T) {
	createOrgViaAdminAPI(t, "gql-org")

	query := `query OrganizationList($user: String!, $limit: Int!) {
		user(login: $user) {
			login
			organizations(first: $limit) {
				totalCount
				nodes { login }
				pageInfo { hasNextPage endCursor }
			}
		}
	}`
	resp := ghPost(t, "/api/graphql", defaultToken, map[string]interface{}{
		"query":     query,
		"variables": map[string]interface{}{"user": "admin", "limit": 100},
	})
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("graphql: got %d", resp.StatusCode)
	}
	var out struct {
		Data struct {
			User struct {
				Login         string `json:"login"`
				Organizations struct {
					TotalCount int `json:"totalCount"`
					Nodes      []struct {
						Login string `json:"login"`
					} `json:"nodes"`
				} `json:"organizations"`
			} `json:"user"`
		} `json:"data"`
		Errors []interface{} `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if len(out.Errors) > 0 {
		t.Fatalf("graphql errors: %v", out.Errors)
	}
	if out.Data.User.Login != "admin" {
		t.Fatalf("user login = %q", out.Data.User.Login)
	}
	found := false
	for _, n := range out.Data.User.Organizations.Nodes {
		if n.Login == "gql-org" {
			found = true
		}
	}
	if !found {
		t.Fatalf("organizations missing gql-org: %+v", out.Data.User.Organizations)
	}
}
