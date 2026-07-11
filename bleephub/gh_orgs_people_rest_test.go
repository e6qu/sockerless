package bleephub

import (
	"net/http"
	"testing"
	"time"
)

// expectStatus drains and closes the response, failing unless the status
// matches.
func expectStatus(t *testing.T, resp *http.Response, want int, context string) {
	t.Helper()
	resp.Body.Close()
	if resp.StatusCode != want {
		t.Fatalf("%s: got %d, want %d", context, resp.StatusCode, want)
	}
}

// activateOrgMember runs the real invitation-acceptance flow: the org
// owner PUTs a pending membership and the member accepts it.
func activateOrgMember(t *testing.T, orgLogin, login, memberToken string) {
	t.Helper()
	expectStatus(t, ghPut(t, "/api/v3/orgs/"+orgLogin+"/memberships/"+login, defaultToken,
		map[string]interface{}{"role": "member"}), http.StatusOK, "PUT membership "+login)
	expectStatus(t, ghPatch(t, "/api/v3/user/memberships/orgs/"+orgLogin, memberToken,
		map[string]interface{}{"state": "active"}), http.StatusOK, "accept membership "+login)
}

func TestOrgInvitationsLifecycle(t *testing.T) {
	createOrgViaAdminAPI(t, "people-inv-org")

	teamResp := ghPost(t, "/api/v3/orgs/people-inv-org/teams", defaultToken, map[string]interface{}{"name": "inv team"})
	if teamResp.StatusCode != http.StatusCreated {
		teamResp.Body.Close()
		t.Fatalf("create team: %d", teamResp.StatusCode)
	}
	team := decodeJSON(t, teamResp)
	teamID := int(team["id"].(float64))

	invitee, inviteeToken := newSharedServerUser(t, "people-invitee")

	created := ghPost(t, "/api/v3/orgs/people-inv-org/invitations", defaultToken, map[string]interface{}{
		"invitee_id": invitee.ID,
		"role":       "direct_member",
		"team_ids":   []int{teamID},
	})
	if created.StatusCode != http.StatusCreated {
		created.Body.Close()
		t.Fatalf("create invitation: %d", created.StatusCode)
	}
	inv := decodeJSON(t, created)
	if inv["login"] != "people-invitee" {
		t.Fatalf("invitation login = %v, want people-invitee", inv["login"])
	}
	if inv["role"] != "direct_member" {
		t.Fatalf("invitation role = %v", inv["role"])
	}
	if inv["team_count"] != float64(1) {
		t.Fatalf("team_count = %v, want 1", inv["team_count"])
	}
	if inviter, ok := inv["inviter"].(map[string]interface{}); !ok || inviter["login"] != "admin" {
		t.Fatalf("inviter = %v, want admin", inv["inviter"])
	}
	if inv["failed_at"] != nil {
		t.Fatalf("fresh invitation has failed_at = %v", inv["failed_at"])
	}
	invID := int(inv["id"].(float64))

	// The invitation creates the pending membership the invitee accepts.
	pendingMembership := ghGet(t, "/api/v3/orgs/people-inv-org/memberships/people-invitee", defaultToken)
	if pendingMembership.StatusCode != http.StatusOK {
		pendingMembership.Body.Close()
		t.Fatalf("GET membership: %d", pendingMembership.StatusCode)
	}
	if m := decodeJSON(t, pendingMembership); m["state"] != "pending" {
		t.Fatalf("membership state = %v, want pending", m["state"])
	}

	// Pending list carries the invitation; the team-scoped views agree.
	list := decodeJSONArray(t, ghGet(t, "/api/v3/orgs/people-inv-org/invitations", defaultToken))
	if len(list) != 1 || int(list[0]["id"].(float64)) != invID {
		t.Fatalf("pending invitations = %v", list)
	}
	teamsOfInv := decodeJSONArray(t, ghGet(t, "/api/v3/orgs/people-inv-org/invitations/"+itoa(invID)+"/teams", defaultToken))
	if len(teamsOfInv) != 1 || teamsOfInv[0]["slug"] != "inv-team" {
		t.Fatalf("invitation teams = %v", teamsOfInv)
	}
	teamInvs := decodeJSONArray(t, ghGet(t, "/api/v3/orgs/people-inv-org/teams/inv-team/invitations", defaultToken))
	if len(teamInvs) != 1 || int(teamInvs[0]["id"].(float64)) != invID {
		t.Fatalf("team invitations = %v", teamInvs)
	}
	legacyTeamInvs := decodeJSONArray(t, ghGet(t, "/api/v3/teams/"+itoa(teamID)+"/invitations", defaultToken))
	if len(legacyTeamInvs) != 1 || int(legacyTeamInvs[0]["id"].(float64)) != invID {
		t.Fatalf("legacy team invitations = %v", legacyTeamInvs)
	}

	// Accepting the membership consumes the invitation and joins the
	// invited team.
	expectStatus(t, ghPatch(t, "/api/v3/user/memberships/orgs/people-inv-org", inviteeToken,
		map[string]interface{}{"state": "active"}), http.StatusOK, "accept invitation")
	if remaining := decodeJSONArray(t, ghGet(t, "/api/v3/orgs/people-inv-org/invitations", defaultToken)); len(remaining) != 0 {
		t.Fatalf("invitations after accept = %v", remaining)
	}
	members := decodeJSONArray(t, ghGet(t, "/api/v3/orgs/people-inv-org/teams/inv-team/members", defaultToken))
	found := false
	for _, m := range members {
		if m["login"] == "people-invitee" {
			found = true
		}
	}
	if !found {
		t.Fatalf("invitee did not join the invited team: %v", members)
	}

	// Validation surface.
	for name, body := range map[string]map[string]interface{}{
		"already a member":  {"invitee_id": invitee.ID},
		"neither id/email":  {"role": "direct_member"},
		"both id and email": {"invitee_id": invitee.ID, "email": "x@example.com"},
		"bad role":          {"email": "y@example.com", "role": "emperor"},
		"reinstate":         {"email": "y@example.com", "role": "reinstate"},
		"unknown team":      {"email": "y@example.com", "team_ids": []int{999999}},
		"unknown invitee":   {"invitee_id": 9999999},
	} {
		expectStatus(t, ghPost(t, "/api/v3/orgs/people-inv-org/invitations", defaultToken, body),
			http.StatusUnprocessableEntity, "create invitation ("+name+")")
	}

	// A plain member (not an owner) cannot create invitations.
	expectStatus(t, ghPost(t, "/api/v3/orgs/people-inv-org/invitations", inviteeToken,
		map[string]interface{}{"email": "z@example.com"}), http.StatusForbidden, "member creates invitation")
}

func TestOrgInvitationsCancelAndEmail(t *testing.T) {
	createOrgViaAdminAPI(t, "people-inv2-org")

	emailUser, _ := newSharedServerUser(t, "people-emailuser")
	testServer.store.mu.Lock()
	emailUser.Email = "people-emailuser@example.com"
	testServer.store.mu.Unlock()

	// An email invitation addressed to an existing account resolves it.
	created := ghPost(t, "/api/v3/orgs/people-inv2-org/invitations", defaultToken,
		map[string]interface{}{"email": "people-emailuser@example.com"})
	if created.StatusCode != http.StatusCreated {
		created.Body.Close()
		t.Fatalf("create email invitation: %d", created.StatusCode)
	}
	inv := decodeJSON(t, created)
	if inv["login"] != "people-emailuser" {
		t.Fatalf("resolved email invitation login = %v", inv["login"])
	}
	invID := int(inv["id"].(float64))

	// Cancelling removes both the invitation and the pending membership.
	expectStatus(t, ghDelete(t, "/api/v3/orgs/people-inv2-org/invitations/"+itoa(invID), defaultToken),
		http.StatusNoContent, "cancel invitation")
	expectStatus(t, ghGet(t, "/api/v3/orgs/people-inv2-org/memberships/people-emailuser", defaultToken),
		http.StatusNotFound, "membership after cancel")
	if list := decodeJSONArray(t, ghGet(t, "/api/v3/orgs/people-inv2-org/invitations", defaultToken)); len(list) != 0 {
		t.Fatalf("invitations after cancel = %v", list)
	}
	expectStatus(t, ghDelete(t, "/api/v3/orgs/people-inv2-org/invitations/"+itoa(invID), defaultToken),
		http.StatusNotFound, "cancel twice")

	// An email invitation with no matching account stays login-less.
	created2 := ghPost(t, "/api/v3/orgs/people-inv2-org/invitations", defaultToken,
		map[string]interface{}{"email": "ghost@example.com", "role": "admin"})
	if created2.StatusCode != http.StatusCreated {
		created2.Body.Close()
		t.Fatalf("create ghost invitation: %d", created2.StatusCode)
	}
	ghost := decodeJSON(t, created2)
	if ghost["login"] != nil || ghost["email"] != "ghost@example.com" {
		t.Fatalf("ghost invitation login/email = %v/%v", ghost["login"], ghost["email"])
	}

	// The role filter matches admin invitations only.
	admins := decodeJSONArray(t, ghGet(t, "/api/v3/orgs/people-inv2-org/invitations?role=admin", defaultToken))
	if len(admins) != 1 || admins[0]["email"] != "ghost@example.com" {
		t.Fatalf("role=admin filter = %v", admins)
	}
	if direct := decodeJSONArray(t, ghGet(t, "/api/v3/orgs/people-inv2-org/invitations?role=direct_member", defaultToken)); len(direct) != 0 {
		t.Fatalf("role=direct_member filter = %v", direct)
	}
	expectStatus(t, ghGet(t, "/api/v3/orgs/people-inv2-org/invitations?role=bogus", defaultToken),
		http.StatusUnprocessableEntity, "bad role filter")
}

func TestOrgFailedInvitations(t *testing.T) {
	createOrgViaAdminAPI(t, "people-fail-org")
	stale, _ := newSharedServerUser(t, "people-staleinvitee")

	created := ghPost(t, "/api/v3/orgs/people-fail-org/invitations", defaultToken,
		map[string]interface{}{"invitee_id": stale.ID})
	if created.StatusCode != http.StatusCreated {
		created.Body.Close()
		t.Fatalf("create invitation: %d", created.StatusCode)
	}
	invID := int(decodeJSON(t, created)["id"].(float64))

	// Age the invitation past the 7-day TTL.
	testServer.store.mu.Lock()
	testServer.store.OrgInvitations[invID].CreatedAt = time.Now().UTC().Add(-8 * 24 * time.Hour)
	testServer.store.mu.Unlock()

	if pending := decodeJSONArray(t, ghGet(t, "/api/v3/orgs/people-fail-org/invitations", defaultToken)); len(pending) != 0 {
		t.Fatalf("expired invitation still pending: %v", pending)
	}
	failed := decodeJSONArray(t, ghGet(t, "/api/v3/orgs/people-fail-org/failed_invitations", defaultToken))
	if len(failed) != 1 {
		t.Fatalf("failed invitations = %v", failed)
	}
	if failed[0]["failed_at"] == nil || failed[0]["failed_reason"] != "Invitation expired." {
		t.Fatalf("failed invitation shape = %v", failed[0])
	}
	// The expired invitation's pending membership is gone too.
	expectStatus(t, ghGet(t, "/api/v3/orgs/people-fail-org/memberships/people-staleinvitee", defaultToken),
		http.StatusNotFound, "membership after expiry")
}

func TestOutsideCollaborators(t *testing.T) {
	createOrgViaAdminAPI(t, "people-oc-org")
	expectStatus(t, ghPost(t, "/api/v3/orgs/people-oc-org/repos", defaultToken,
		map[string]interface{}{"name": "oc-repo"}), http.StatusCreated, "create org repo")

	// A direct collaborator who is not a member is an outside collaborator.
	external, _ := newSharedServerUser(t, "people-oc-ext")
	testServer.store.AddRepoCollaborator("people-oc-org", "oc-repo", external.Login, "push")
	outside := decodeJSONArray(t, ghGet(t, "/api/v3/orgs/people-oc-org/outside_collaborators", defaultToken))
	if len(outside) != 1 || outside[0]["login"] != "people-oc-ext" {
		t.Fatalf("outside collaborators = %v", outside)
	}

	// An active member with team-granted repo access is NOT an outside
	// collaborator…
	_, memberToken := newSharedServerUser(t, "people-oc-member")
	activateOrgMember(t, "people-oc-org", "people-oc-member", memberToken)
	teamResp := ghPost(t, "/api/v3/orgs/people-oc-org/teams", defaultToken, map[string]interface{}{"name": "oc team", "permission": "push"})
	if teamResp.StatusCode != http.StatusCreated {
		teamResp.Body.Close()
		t.Fatalf("create team: %d", teamResp.StatusCode)
	}
	teamResp.Body.Close()
	expectStatus(t, ghPut(t, "/api/v3/orgs/people-oc-org/teams/oc-team/memberships/people-oc-member", defaultToken, nil),
		http.StatusOK, "add team member")
	expectStatus(t, ghPut(t, "/api/v3/orgs/people-oc-org/teams/oc-team/repos/people-oc-org/oc-repo", defaultToken,
		map[string]interface{}{"permission": "push"}), http.StatusNoContent, "grant team repo")
	for _, u := range decodeJSONArray(t, ghGet(t, "/api/v3/orgs/people-oc-org/outside_collaborators", defaultToken)) {
		if u["login"] == "people-oc-member" {
			t.Fatal("active member listed as outside collaborator")
		}
	}

	// …until converted: the conversion keeps the team-derived repo access
	// as a direct grant and drops the membership.
	expectStatus(t, ghPut(t, "/api/v3/orgs/people-oc-org/outside_collaborators/people-oc-member", defaultToken, nil),
		http.StatusNoContent, "convert member")
	expectStatus(t, ghGet(t, "/api/v3/orgs/people-oc-org/memberships/people-oc-member", defaultToken),
		http.StatusNotFound, "membership after conversion")
	if perm := testServer.store.GetRepoCollaboratorPermission("people-oc-org", "oc-repo", "people-oc-member"); perm != "push" {
		t.Fatalf("converted member repo permission = %q, want push", perm)
	}
	converted := decodeJSONArray(t, ghGet(t, "/api/v3/orgs/people-oc-org/outside_collaborators", defaultToken))
	foundConverted := false
	for _, u := range converted {
		if u["login"] == "people-oc-member" {
			foundConverted = true
		}
	}
	if !foundConverted {
		t.Fatalf("converted member missing from outside collaborators: %v", converted)
	}

	// Owners cannot be converted; unknown users and non-members 404.
	expectStatus(t, ghPut(t, "/api/v3/orgs/people-oc-org/outside_collaborators/admin", defaultToken, nil),
		http.StatusForbidden, "convert owner")
	expectStatus(t, ghPut(t, "/api/v3/orgs/people-oc-org/outside_collaborators/nobody-here", defaultToken, nil),
		http.StatusNotFound, "convert unknown user")
	expectStatus(t, ghPut(t, "/api/v3/orgs/people-oc-org/outside_collaborators/people-oc-ext", defaultToken, nil),
		http.StatusNotFound, "convert non-member")

	// Removing an outside collaborator strips the grants; members 422.
	expectStatus(t, ghDelete(t, "/api/v3/orgs/people-oc-org/outside_collaborators/people-oc-ext", defaultToken),
		http.StatusNoContent, "remove outside collaborator")
	if perm := testServer.store.GetRepoCollaboratorPermission("people-oc-org", "oc-repo", "people-oc-ext"); perm != "" {
		t.Fatalf("removed collaborator still has permission %q", perm)
	}
	_, member2Token := newSharedServerUser(t, "people-oc-member2")
	activateOrgMember(t, "people-oc-org", "people-oc-member2", member2Token)
	expectStatus(t, ghDelete(t, "/api/v3/orgs/people-oc-org/outside_collaborators/people-oc-member2", defaultToken),
		http.StatusUnprocessableEntity, "remove member as outside collaborator")

	// Filters: bleephub models no 2FA, so 2fa_insecure matches no one and
	// unknown filters are validation errors.
	if insecure := decodeJSONArray(t, ghGet(t, "/api/v3/orgs/people-oc-org/outside_collaborators?filter=2fa_insecure", defaultToken)); len(insecure) != 0 {
		t.Fatalf("2fa_insecure filter = %v", insecure)
	}
	expectStatus(t, ghGet(t, "/api/v3/orgs/people-oc-org/outside_collaborators?filter=bogus", defaultToken),
		http.StatusUnprocessableEntity, "bad filter")
}

func TestOrgBlocks(t *testing.T) {
	createOrgViaAdminAPI(t, "people-blk-org")
	target, targetToken := newSharedServerUser(t, "people-blk-user")
	_ = target

	if blocked := decodeJSONArray(t, ghGet(t, "/api/v3/orgs/people-blk-org/blocks", defaultToken)); len(blocked) != 0 {
		t.Fatalf("initial block list = %v", blocked)
	}
	expectStatus(t, ghGet(t, "/api/v3/orgs/people-blk-org/blocks/people-blk-user", defaultToken),
		http.StatusNotFound, "check unblocked")

	expectStatus(t, ghPut(t, "/api/v3/orgs/people-blk-org/blocks/people-blk-user", defaultToken, nil),
		http.StatusNoContent, "block user")
	expectStatus(t, ghGet(t, "/api/v3/orgs/people-blk-org/blocks/people-blk-user", defaultToken),
		http.StatusNoContent, "check blocked")
	blocked := decodeJSONArray(t, ghGet(t, "/api/v3/orgs/people-blk-org/blocks", defaultToken))
	if len(blocked) != 1 || blocked[0]["login"] != "people-blk-user" {
		t.Fatalf("block list = %v", blocked)
	}
	// Blocking is idempotent.
	expectStatus(t, ghPut(t, "/api/v3/orgs/people-blk-org/blocks/people-blk-user", defaultToken, nil),
		http.StatusNoContent, "block again")

	// A blocked user cannot be invited.
	expectStatus(t, ghPost(t, "/api/v3/orgs/people-blk-org/invitations", defaultToken,
		map[string]interface{}{"invitee_id": target.ID}), http.StatusUnprocessableEntity, "invite blocked user")

	expectStatus(t, ghDelete(t, "/api/v3/orgs/people-blk-org/blocks/people-blk-user", defaultToken),
		http.StatusNoContent, "unblock user")
	expectStatus(t, ghGet(t, "/api/v3/orgs/people-blk-org/blocks/people-blk-user", defaultToken),
		http.StatusNotFound, "check after unblock")

	// Validation: yourself, members, unknown users, non-owner callers.
	expectStatus(t, ghPut(t, "/api/v3/orgs/people-blk-org/blocks/admin", defaultToken, nil),
		http.StatusUnprocessableEntity, "block self")
	_, memberToken := newSharedServerUser(t, "people-blk-member")
	activateOrgMember(t, "people-blk-org", "people-blk-member", memberToken)
	expectStatus(t, ghPut(t, "/api/v3/orgs/people-blk-org/blocks/people-blk-member", defaultToken, nil),
		http.StatusUnprocessableEntity, "block member")
	expectStatus(t, ghPut(t, "/api/v3/orgs/people-blk-org/blocks/nobody-here", defaultToken, nil),
		http.StatusNotFound, "block unknown")
	expectStatus(t, ghGet(t, "/api/v3/orgs/people-blk-org/blocks", targetToken),
		http.StatusForbidden, "non-owner lists blocks")
}

func TestOrgInteractionLimits(t *testing.T) {
	createOrgViaAdminAPI(t, "people-il-org")

	unset := ghGet(t, "/api/v3/orgs/people-il-org/interaction-limits", defaultToken)
	if unset.StatusCode != http.StatusOK {
		unset.Body.Close()
		t.Fatalf("GET unset limits: %d", unset.StatusCode)
	}
	if body := decodeJSON(t, unset); len(body) != 0 {
		t.Fatalf("unset interaction limits = %v, want empty object", body)
	}

	put := ghPut(t, "/api/v3/orgs/people-il-org/interaction-limits", defaultToken,
		map[string]interface{}{"limit": "collaborators_only", "expiry": "one_week"})
	if put.StatusCode != http.StatusOK {
		put.Body.Close()
		t.Fatalf("PUT limits: %d", put.StatusCode)
	}
	set := decodeJSON(t, put)
	if set["limit"] != "collaborators_only" || set["origin"] != "organization" {
		t.Fatalf("set limits = %v", set)
	}
	expires, err := time.Parse(time.RFC3339, set["expires_at"].(string))
	if err != nil {
		t.Fatalf("expires_at parse: %v", err)
	}
	if d := time.Until(expires); d < 6*24*time.Hour || d > 8*24*time.Hour {
		t.Fatalf("one_week expiry lands %v away", d)
	}

	got := decodeJSON(t, ghGet(t, "/api/v3/orgs/people-il-org/interaction-limits", defaultToken))
	if got["limit"] != "collaborators_only" {
		t.Fatalf("read-back limits = %v", got)
	}

	expectStatus(t, ghPut(t, "/api/v3/orgs/people-il-org/interaction-limits", defaultToken,
		map[string]interface{}{"limit": "everyone"}), http.StatusUnprocessableEntity, "bad limit")
	expectStatus(t, ghPut(t, "/api/v3/orgs/people-il-org/interaction-limits", defaultToken,
		map[string]interface{}{"limit": "existing_users", "expiry": "two_centuries"}), http.StatusUnprocessableEntity, "bad expiry")

	expectStatus(t, ghDelete(t, "/api/v3/orgs/people-il-org/interaction-limits", defaultToken),
		http.StatusNoContent, "delete limits")
	if body := decodeJSON(t, ghGet(t, "/api/v3/orgs/people-il-org/interaction-limits", defaultToken)); len(body) != 0 {
		t.Fatalf("limits after delete = %v", body)
	}
}

func TestOrganizationRoles(t *testing.T) {
	createOrgViaAdminAPI(t, "people-roles-org")

	listing := decodeJSON(t, ghGet(t, "/api/v3/orgs/people-roles-org/organization-roles", defaultToken))
	if listing["total_count"] != float64(6) {
		t.Fatalf("total_count = %v, want 6", listing["total_count"])
	}
	roles := listing["roles"].([]interface{})
	byName := map[string]map[string]interface{}{}
	for _, raw := range roles {
		role := raw.(map[string]interface{})
		byName[role["name"].(string)] = role
	}
	read := byName["all_repo_read"]
	if read == nil || read["base_role"] != "read" || read["source"] != "Predefined" {
		t.Fatalf("all_repo_read = %v", read)
	}
	if perms := read["permissions"].([]interface{}); len(perms) != 0 {
		t.Fatalf("all_repo_read permissions = %v, want empty", perms)
	}
	sm := byName["security_manager"]
	if sm == nil || sm["base_role"] != "read" {
		t.Fatalf("security_manager = %v", sm)
	}
	if perms := sm["permissions"].([]interface{}); len(perms) != 1 || perms[0] != "manage_security_products" {
		t.Fatalf("security_manager permissions = %v", perms)
	}

	single := decodeJSON(t, ghGet(t, "/api/v3/orgs/people-roles-org/organization-roles/138", defaultToken))
	if single["name"] != "all_repo_read" {
		t.Fatalf("GET role 138 = %v", single)
	}
	expectStatus(t, ghGet(t, "/api/v3/orgs/people-roles-org/organization-roles/999999", defaultToken),
		http.StatusNotFound, "unknown role")

	teamResp := ghPost(t, "/api/v3/orgs/people-roles-org/teams", defaultToken, map[string]interface{}{"name": "roles team"})
	teamResp.Body.Close()
	_, memberToken := newSharedServerUser(t, "people-roles-user")
	activateOrgMember(t, "people-roles-org", "people-roles-user", memberToken)

	// Team assignment.
	expectStatus(t, ghPut(t, "/api/v3/orgs/people-roles-org/organization-roles/teams/roles-team/140", defaultToken, nil),
		http.StatusNoContent, "assign role to team")
	teams := decodeJSONArray(t, ghGet(t, "/api/v3/orgs/people-roles-org/organization-roles/140/teams", defaultToken))
	if len(teams) != 1 || teams[0]["slug"] != "roles-team" || teams[0]["assignment"] != "direct" {
		t.Fatalf("role teams = %v", teams)
	}

	// Team members hold the role indirectly; a direct grant on top makes
	// it mixed; revoking the direct grant reverts to indirect.
	expectStatus(t, ghPut(t, "/api/v3/orgs/people-roles-org/teams/roles-team/memberships/people-roles-user", defaultToken, nil),
		http.StatusOK, "add team member")
	assignmentOf := func() string {
		t.Helper()
		users := decodeJSONArray(t, ghGet(t, "/api/v3/orgs/people-roles-org/organization-roles/140/users", defaultToken))
		for _, u := range users {
			if u["login"] == "people-roles-user" {
				return u["assignment"].(string)
			}
		}
		return ""
	}
	if a := assignmentOf(); a != "indirect" {
		t.Fatalf("team-derived assignment = %q, want indirect", a)
	}
	expectStatus(t, ghPut(t, "/api/v3/orgs/people-roles-org/organization-roles/users/people-roles-user/140", defaultToken, nil),
		http.StatusNoContent, "assign role to user")
	if a := assignmentOf(); a != "mixed" {
		t.Fatalf("direct+team assignment = %q, want mixed", a)
	}
	expectStatus(t, ghDelete(t, "/api/v3/orgs/people-roles-org/organization-roles/users/people-roles-user/140", defaultToken),
		http.StatusNoContent, "revoke user role")
	if a := assignmentOf(); a != "indirect" {
		t.Fatalf("post-revoke assignment = %q, want indirect", a)
	}

	// Non-members cannot be assigned roles.
	newSharedServerUser(t, "people-roles-outsider")
	expectStatus(t, ghPut(t, "/api/v3/orgs/people-roles-org/organization-roles/users/people-roles-outsider/140", defaultToken, nil),
		http.StatusUnprocessableEntity, "assign role to non-member")
	expectStatus(t, ghPut(t, "/api/v3/orgs/people-roles-org/organization-roles/teams/roles-team/999999", defaultToken, nil),
		http.StatusNotFound, "assign unknown role")

	// Revoking all of a team's roles clears its assignment.
	expectStatus(t, ghDelete(t, "/api/v3/orgs/people-roles-org/organization-roles/teams/roles-team", defaultToken),
		http.StatusNoContent, "revoke all team roles")
	if teams := decodeJSONArray(t, ghGet(t, "/api/v3/orgs/people-roles-org/organization-roles/140/teams", defaultToken)); len(teams) != 0 {
		t.Fatalf("role teams after revoke-all = %v", teams)
	}
}

func TestSecurityManagers(t *testing.T) {
	createOrgViaAdminAPI(t, "people-sm-org")
	teamResp := ghPost(t, "/api/v3/orgs/people-sm-org/teams", defaultToken, map[string]interface{}{"name": "sm team"})
	teamResp.Body.Close()

	expectStatus(t, ghPut(t, "/api/v3/orgs/people-sm-org/security-managers/teams/sm-team", defaultToken, nil),
		http.StatusNoContent, "add security manager team")
	managers := decodeJSONArray(t, ghGet(t, "/api/v3/orgs/people-sm-org/security-managers", defaultToken))
	if len(managers) != 1 || managers[0]["slug"] != "sm-team" {
		t.Fatalf("security managers = %v", managers)
	}

	// The deprecated surface is an alias of the security_manager
	// organization role, so the role's team list agrees.
	roleTeams := decodeJSONArray(t, ghGet(t, "/api/v3/orgs/people-sm-org/organization-roles/143/teams", defaultToken))
	if len(roleTeams) != 1 || roleTeams[0]["slug"] != "sm-team" {
		t.Fatalf("security_manager role teams = %v", roleTeams)
	}

	expectStatus(t, ghDelete(t, "/api/v3/orgs/people-sm-org/security-managers/teams/sm-team", defaultToken),
		http.StatusNoContent, "remove security manager team")
	if managers := decodeJSONArray(t, ghGet(t, "/api/v3/orgs/people-sm-org/security-managers", defaultToken)); len(managers) != 0 {
		t.Fatalf("security managers after remove = %v", managers)
	}
	expectStatus(t, ghPut(t, "/api/v3/orgs/people-sm-org/security-managers/teams/no-such-team", defaultToken, nil),
		http.StatusNotFound, "add unknown team")
}

func TestOrgMemberCopilotSeat(t *testing.T) {
	createOrgViaAdminAPI(t, "people-cop-org")
	_, memberToken := newSharedServerUser(t, "people-cop-member")
	activateOrgMember(t, "people-cop-org", "people-cop-member", memberToken)

	// A member without a GitHub Copilot seat assignment answers the
	// documented 404; assigning a seat through the billing API makes the
	// seat detail readable.
	expectStatus(t, ghGet(t, "/api/v3/orgs/people-cop-org/members/people-cop-member/copilot", defaultToken),
		http.StatusNotFound, "copilot seat before assignment")
	expectStatus(t, ghGet(t, "/api/v3/orgs/people-cop-org/members/nobody-here/copilot", defaultToken),
		http.StatusNotFound, "copilot seat unknown user")
	expectStatus(t, ghGet(t, "/api/v3/orgs/people-cop-org/members/admin/copilot", memberToken),
		http.StatusForbidden, "copilot seat of another member as non-owner")

	expectStatus(t, ghPost(t, "/api/v3/orgs/people-cop-org/copilot/billing/selected_users", defaultToken,
		map[string]interface{}{"selected_usernames": []string{"people-cop-member"}}),
		http.StatusCreated, "assign copilot seat")
	seat := decodeJSON(t, ghGet(t, "/api/v3/orgs/people-cop-org/members/people-cop-member/copilot", defaultToken))
	assignee, _ := seat["assignee"].(map[string]interface{})
	if assignee["login"] != "people-cop-member" {
		t.Fatalf("copilot seat assignee = %v", seat["assignee"])
	}
}

func TestOrgSecurityProductEnablement(t *testing.T) {
	createOrgViaAdminAPI(t, "people-sec-org")
	for _, name := range []string{"sec-repo-a", "sec-repo-b"} {
		expectStatus(t, ghPost(t, "/api/v3/orgs/people-sec-org/repos", defaultToken,
			map[string]interface{}{"name": name}), http.StatusCreated, "create "+name)
	}

	repoFlag := func(name string, pick func(*Repo) bool) bool {
		t.Helper()
		repo := testServer.store.GetRepo("people-sec-org", name)
		if repo == nil {
			t.Fatalf("repo %s missing", name)
		}
		return pick(repo)
	}
	vulnAlerts := func(r *Repo) bool { return r.VulnerabilityAlertsEnabled }
	autoFixes := func(r *Repo) bool { return r.AutomatedSecurityFixesEnabled }

	expectStatus(t, ghPost(t, "/api/v3/orgs/people-sec-org/dependabot_alerts/enable_all", defaultToken, nil),
		http.StatusNoContent, "enable dependabot alerts")
	if !repoFlag("sec-repo-a", vulnAlerts) || !repoFlag("sec-repo-b", vulnAlerts) {
		t.Fatal("dependabot_alerts enable_all did not flip vulnerability alerts on every org repo")
	}
	expectStatus(t, ghPost(t, "/api/v3/orgs/people-sec-org/dependabot_security_updates/enable_all", defaultToken, nil),
		http.StatusNoContent, "enable dependabot security updates")
	if !repoFlag("sec-repo-a", autoFixes) || !repoFlag("sec-repo-b", autoFixes) {
		t.Fatal("dependabot_security_updates enable_all did not flip automated security fixes")
	}
	expectStatus(t, ghPost(t, "/api/v3/orgs/people-sec-org/dependabot_alerts/disable_all", defaultToken, nil),
		http.StatusNoContent, "disable dependabot alerts")
	if repoFlag("sec-repo-a", vulnAlerts) || repoFlag("sec-repo-b", vulnAlerts) {
		t.Fatal("dependabot_alerts disable_all did not clear vulnerability alerts")
	}

	// Products bleephub has no per-repository setting for fail like an
	// instance without the feature's licensing.
	expectStatus(t, ghPost(t, "/api/v3/orgs/people-sec-org/advanced_security/enable_all", defaultToken, nil),
		http.StatusUnprocessableEntity, "advanced security")
	// Values outside the documented enums are not routes at all.
	expectStatus(t, ghPost(t, "/api/v3/orgs/people-sec-org/bogus_product/enable_all", defaultToken, nil),
		http.StatusNotFound, "unknown product")
	expectStatus(t, ghPost(t, "/api/v3/orgs/people-sec-org/dependabot_alerts/enable_some", defaultToken, nil),
		http.StatusNotFound, "unknown enablement")

	_, memberToken := newSharedServerUser(t, "people-sec-member")
	activateOrgMember(t, "people-sec-org", "people-sec-member", memberToken)
	expectStatus(t, ghPost(t, "/api/v3/orgs/people-sec-org/dependabot_alerts/enable_all", memberToken, nil),
		http.StatusForbidden, "non-owner flips security product")
}

// TestTeamsPeoplePersistenceReload verifies the teams-people buckets
// (org invitations, blocks, interaction limits, role assignments)
// survive a persistence reload.
func TestTeamsPeoplePersistenceReload(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BLEEPHUB_PERSIST", "true")
	t.Setenv("BLEEPHUB_DATA_DIR", dir)

	p1, err := NewPersistence()
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	st1 := NewStore()
	if err := st1.SetPersistence(p1); err != nil {
		t.Fatalf("SetPersistence: %v", err)
	}
	st1.SeedDefaultUser()
	admin := st1.UsersByLogin["admin"]
	org := st1.CreateOrg(admin, "reload-people-org", "Reload People", "")
	team := st1.CreateTeam(org.Login, "reload-team", TeamOptions{})

	invitee := &User{ID: st1.NextUser, Login: "reload-invitee", Type: "User"}
	st1.NextUser++
	st1.Users[invitee.ID] = invitee
	st1.UsersByLogin[invitee.Login] = invitee

	inv, reason := st1.CreateOrgInvitation(org, admin, invitee, "", "direct_member", []int{team.ID})
	if inv == nil {
		t.Fatalf("CreateOrgInvitation: %s", reason)
	}
	st1.BlockUserForOrg(org.Login, invitee.ID)
	// A blocked invitee is a handler-level rejection; the store-level
	// combination here just exercises both buckets.
	expiry := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)
	st1.SetOrgInteractionLimit(org.Login, "contributors_only", expiry)
	st1.AssignOrgRoleToTeam(org.Login, securityManagerOrgRoleID, team.ID)
	st1.AssignOrgRoleToUser(org.Login, 140, admin.ID)

	if err := p1.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	p2, err := NewPersistence()
	if err != nil {
		t.Fatalf("re-open: %v", err)
	}
	defer p2.Close()
	st2 := NewStore()
	if err := st2.SetPersistence(p2); err != nil {
		t.Fatalf("re-load: %v", err)
	}

	reloaded := st2.GetOrgInvitation(org.Login, inv.ID)
	if reloaded == nil || reloaded.Login != "reload-invitee" || len(reloaded.TeamIDs) != 1 {
		t.Fatalf("invitation did not reload: %+v", reloaded)
	}
	if st2.NextOrgInvitationID != inv.ID+1 {
		t.Fatalf("NextOrgInvitationID = %d, want %d", st2.NextOrgInvitationID, inv.ID+1)
	}
	if !st2.IsUserBlockedByOrg(org.Login, invitee.ID) {
		t.Fatal("org block did not reload")
	}
	lim := st2.GetOrgInteractionLimit(org.Login)
	if lim == nil || lim.Limit != "contributors_only" || !lim.ExpiresAt.Equal(expiry) {
		t.Fatalf("interaction limit did not reload: %+v", lim)
	}
	if teams := st2.ListTeamsWithOrgRole(org.Login, securityManagerOrgRoleID); len(teams) != 1 || teams[0].ID != team.ID {
		t.Fatalf("role team assignment did not reload: %v", teams)
	}
	if users := st2.ListUsersWithOrgRole(org.Login, 140); users[admin.ID] != "direct" {
		t.Fatalf("role user assignment did not reload: %v", users)
	}
}
