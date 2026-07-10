package bleephub

import (
	"bytes"
	"io"
	"testing"
	"time"
)

func createTestUser(t *testing.T, login string) *User {
	t.Helper()
	resp, err := authedPost("/internal/users", "application/json", bytes.NewReader(mustJSON(map[string]interface{}{
		"login": login,
		"email": login + "@example.com",
	})))
	if err != nil {
		t.Fatalf("create user %s: %v", login, err)
	}
	if resp.StatusCode != 201 {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("create user %s: %d %s", login, resp.StatusCode, b)
	}
	resp.Body.Close()
	return testServer.store.UsersByLogin[login]
}

func TestUserExtras_ListUsers(t *testing.T) {
	resp := ghGet(t, "/api/v3/users", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	users := decodeJSONArray(t, resp)
	if len(users) == 0 {
		t.Fatal("expected users")
	}
}

func TestEnterpriseAdminUsersCRUDSiteAdminAndSuspension(t *testing.T) {
	login := "admin-api-user"
	create := ghPost(t, "/api/v3/admin/users", defaultToken, map[string]interface{}{
		"login": login,
		"email": "admin-api-user@example.com",
	})
	if create.StatusCode != 201 {
		body, _ := io.ReadAll(create.Body)
		create.Body.Close()
		t.Fatalf("create admin user: got %d, want 201: %s", create.StatusCode, body)
	}
	created := decodeJSON(t, create)
	if created["login"] != login {
		t.Fatalf("create login = %v, want %s", created["login"], login)
	}

	promote := ghPut(t, "/api/v3/users/"+login+"/site_admin", defaultToken, nil)
	promote.Body.Close()
	if promote.StatusCode != 204 {
		t.Fatalf("promote status = %d, want 204", promote.StatusCode)
	}
	got := decodeJSONWithStatus(t, ghGet(t, "/api/v3/users/"+login, defaultToken), 200)
	if got["site_admin"] != true {
		t.Fatalf("site_admin after promote = %v, want true", got["site_admin"])
	}

	demote := ghDelete(t, "/api/v3/users/"+login+"/site_admin", defaultToken)
	demote.Body.Close()
	if demote.StatusCode != 204 {
		t.Fatalf("demote status = %d, want 204", demote.StatusCode)
	}
	got = decodeJSONWithStatus(t, ghGet(t, "/api/v3/users/"+login, defaultToken), 200)
	if got["site_admin"] != false {
		t.Fatalf("site_admin after demote = %v, want false", got["site_admin"])
	}

	suspend := ghPut(t, "/api/v3/users/"+login+"/suspended", defaultToken, map[string]string{"reason": "test"})
	suspend.Body.Close()
	if suspend.StatusCode != 204 {
		t.Fatalf("suspend status = %d, want 204", suspend.StatusCode)
	}

	u := testServer.store.LookupUserByLogin(login)
	if u == nil {
		t.Fatalf("created user %q missing from store", login)
	}
	userToken := "tok-" + login
	testServer.store.mu.Lock()
	testServer.store.Tokens[userToken] = &Token{Value: userToken, UserID: u.ID, Scopes: "repo", CreatedAt: time.Now().UTC()}
	testServer.store.mu.Unlock()
	asSuspended := ghGet(t, "/api/v3/user", userToken)
	asSuspended.Body.Close()
	if asSuspended.StatusCode != 403 {
		t.Fatalf("suspended user token status = %d, want 403", asSuspended.StatusCode)
	}

	unsuspend := ghDelete(t, "/api/v3/users/"+login+"/suspended", defaultToken)
	unsuspend.Body.Close()
	if unsuspend.StatusCode != 204 {
		t.Fatalf("unsuspend status = %d, want 204", unsuspend.StatusCode)
	}
	asActive := ghGet(t, "/api/v3/user", userToken)
	asActive.Body.Close()
	if asActive.StatusCode != 200 {
		t.Fatalf("unsuspended user token status = %d, want 200", asActive.StatusCode)
	}

	del := ghDelete(t, "/api/v3/admin/users/"+login, defaultToken)
	del.Body.Close()
	if del.StatusCode != 204 {
		t.Fatalf("delete admin user status = %d, want 204", del.StatusCode)
	}
	getDeleted := ghGet(t, "/api/v3/users/"+login, defaultToken)
	getDeleted.Body.Close()
	if getDeleted.StatusCode != 404 {
		t.Fatalf("get deleted user status = %d, want 404", getDeleted.StatusCode)
	}
}

func TestUserExtras_Blocks(t *testing.T) {
	u := createTestUser(t, "blocktarget")
	_ = u

	putResp := ghPut(t, "/api/v3/user/blocks/blocktarget", defaultToken, nil)
	putResp.Body.Close()
	if putResp.StatusCode != 204 {
		t.Fatalf("expected 204, got %d", putResp.StatusCode)
	}

	checkResp := ghGet(t, "/api/v3/user/blocks/blocktarget", defaultToken)
	checkResp.Body.Close()
	if checkResp.StatusCode != 204 {
		t.Fatalf("expected 204, got %d", checkResp.StatusCode)
	}

	listResp := ghGet(t, "/api/v3/user/blocks", defaultToken)
	if listResp.StatusCode != 200 {
		listResp.Body.Close()
		t.Fatalf("expected 200, got %d", listResp.StatusCode)
	}
	blocks := decodeJSONArray(t, listResp)
	if len(blocks) == 0 {
		t.Fatal("expected blocked users")
	}

	delResp := ghDelete(t, "/api/v3/user/blocks/blocktarget", defaultToken)
	delResp.Body.Close()
	if delResp.StatusCode != 204 {
		t.Fatalf("expected 204, got %d", delResp.StatusCode)
	}
}

func TestUserExtras_SocialAccounts(t *testing.T) {
	resp := ghPost(t, "/api/v3/user/social_accounts", defaultToken, []map[string]interface{}{
		{"url": "https://example.com/me"},
	})
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	accounts := decodeJSONArray(t, resp)
	if len(accounts) == 0 {
		t.Fatal("expected social accounts")
	}

	listResp := ghGet(t, "/api/v3/users/admin/social_accounts", defaultToken)
	if listResp.StatusCode != 200 {
		listResp.Body.Close()
		t.Fatalf("expected 200, got %d", listResp.StatusCode)
	}
	got := decodeJSONArray(t, listResp)
	if len(got) == 0 {
		t.Fatal("expected public social accounts")
	}

	delResp := ghDelete(t, "/api/v3/user/social_accounts", defaultToken)
	delResp.Body.Close()
	if delResp.StatusCode != 204 {
		t.Fatalf("expected 204, got %d", delResp.StatusCode)
	}
}

func TestUserExtras_SSHSigningKeys(t *testing.T) {
	resp := ghPost(t, "/api/v3/user/ssh_signing_keys", defaultToken, map[string]interface{}{
		"key": "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIDIhz2GK/XCUj4i6Q5yQJNL1MXMY0RxzPV2QrBqfHrDq",
	})
	if resp.StatusCode != 201 {
		resp.Body.Close()
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	key := decodeJSON(t, resp)
	keyID := int(key["id"].(float64))

	listResp := ghGet(t, "/api/v3/user/ssh_signing_keys", defaultToken)
	if listResp.StatusCode != 200 {
		listResp.Body.Close()
		t.Fatalf("expected 200, got %d", listResp.StatusCode)
	}
	keys := decodeJSONArray(t, listResp)
	if len(keys) == 0 {
		t.Fatal("expected keys")
	}

	delResp := ghDelete(t, "/api/v3/user/ssh_signing_keys/"+itoa(keyID), defaultToken)
	delResp.Body.Close()
	if delResp.StatusCode != 204 {
		t.Fatalf("expected 204, got %d", delResp.StatusCode)
	}
}

func TestUserExtras_Following(t *testing.T) {
	createTestUser(t, "followtarget")

	putResp := ghPut(t, "/api/v3/user/following/followtarget", defaultToken, nil)
	putResp.Body.Close()
	if putResp.StatusCode != 204 {
		t.Fatalf("expected 204, got %d", putResp.StatusCode)
	}

	checkResp := ghGet(t, "/api/v3/user/following/followtarget", defaultToken)
	checkResp.Body.Close()
	if checkResp.StatusCode != 204 {
		t.Fatalf("expected 204, got %d", checkResp.StatusCode)
	}

	publicCheck := ghGet(t, "/api/v3/users/admin/following/followtarget", defaultToken)
	publicCheck.Body.Close()
	if publicCheck.StatusCode != 204 {
		t.Fatalf("expected 204, got %d", publicCheck.StatusCode)
	}
}

func TestUserExtras_Events(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	repo := testServer.store.CreateRepo(admin, "event-repo", "", false)
	if repo == nil {
		t.Fatal("create repo failed")
	}
	issue := testServer.store.CreateIssue(repo.ID, admin.ID, "Issue title", "body", nil, nil, 0)

	fetchIssueEvent := func() map[string]interface{} {
		t.Helper()
		resp := ghGet(t, "/api/v3/users/admin/events?per_page=100", defaultToken)
		if resp.StatusCode != 200 {
			resp.Body.Close()
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
		for _, ev := range decodeJSONArray(t, resp) {
			if ev["type"] != "IssuesEvent" {
				continue
			}
			payload, _ := ev["payload"].(map[string]interface{})
			embedded, _ := payload["issue"].(map[string]interface{})
			if embedded != nil && embedded["title"] == "Issue title" && payload["action"] == "opened" {
				return ev
			}
		}
		t.Fatal("expected an IssuesEvent for the created issue")
		return nil
	}

	first := fetchIssueEvent()
	second := fetchIssueEvent()

	// The event derives from the stored issue: its ID and timestamp are
	// stable across requests, and created_at is the issue's recorded
	// creation time — not the render time.
	if id, _ := first["id"].(string); id == "" {
		t.Fatalf("event id missing: %v", first)
	}
	if second["id"] != first["id"] {
		t.Fatalf("event id not stable across requests: %v vs %v", first["id"], second["id"])
	}
	if second["created_at"] != first["created_at"] {
		t.Fatalf("event created_at not stable across requests: %v vs %v", first["created_at"], second["created_at"])
	}
	if want := issue.CreatedAt.UTC().Format(time.RFC3339); first["created_at"] != want {
		t.Fatalf("event created_at = %v, want the issue's recorded creation time %s", first["created_at"], want)
	}
	actor, _ := first["actor"].(map[string]interface{})
	if actor == nil || actor["login"] != "admin" {
		t.Fatalf("event actor = %v", first["actor"])
	}
	repoJSON, _ := first["repo"].(map[string]interface{})
	if repoJSON == nil || repoJSON["name"] != "admin/event-repo" {
		t.Fatalf("event repo = %v", first["repo"])
	}
}

func TestUserExtras_UserGists(t *testing.T) {
	created := createTestGist(t, defaultToken, true)
	id := created["id"].(string)

	resp := ghGet(t, "/api/v3/users/admin/gists", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	gists := decodeJSONArray(t, resp)
	found := false
	for _, g := range gists {
		if g["id"] == id {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected user gist")
	}
}

func TestUserExtras_StarredRepo(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	repo := testServer.store.CreateRepo(admin, "star-repo", "", false)
	if repo == nil {
		t.Fatal("create repo failed")
	}
	testServer.store.StarRepo(admin.ID, "admin", "star-repo")

	resp := ghGet(t, "/api/v3/user/starred/admin/star-repo", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}
}

func TestUserExtras_Subscriptions(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	repo := testServer.store.CreateRepo(admin, "sub-repo", "", false)
	if repo == nil {
		t.Fatal("create repo failed")
	}
	testServer.store.SetRepoSubscription(admin.ID, repo.ID, true)

	resp := ghGet(t, "/api/v3/user/subscriptions", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	subs := decodeJSONArray(t, resp)
	if len(subs) == 0 {
		t.Fatal("expected subscriptions")
	}
}
