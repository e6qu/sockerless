package bleephub

import (
	"bytes"
	"io"
	"testing"
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
	testServer.store.CreateIssue(repo.ID, admin.ID, "Issue title", "body", nil, nil, 0)

	resp := ghGet(t, "/api/v3/users/admin/events", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	events := decodeJSONArray(t, resp)
	if len(events) == 0 {
		t.Fatal("expected events")
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
