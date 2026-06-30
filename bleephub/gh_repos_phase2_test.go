package bleephub

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

// TestRepoTopicsREST verifies GET and PUT /repos/{owner}/{repo}/topics.
func TestRepoTopicsREST(t *testing.T) {
	ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name": "topics-rest",
	})

	// GET returns empty topics.
	getResp := ghGet(t, "/api/v3/repos/admin/topics-rest/topics", defaultToken)
	defer getResp.Body.Close()
	if getResp.StatusCode != 200 {
		t.Fatalf("expected 200 for get topics, got %d", getResp.StatusCode)
	}
	var got map[string]interface{}
	if err := json.NewDecoder(getResp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	names, _ := got["names"].([]interface{})
	if len(names) != 0 {
		t.Fatalf("expected empty topics, got %v", names)
	}

	// PUT topics.
	putResp := ghPut(t, "/api/v3/repos/admin/topics-rest/topics", defaultToken, map[string]interface{}{
		"names": []string{"go", "ci", "bleephub"},
	})
	if putResp.StatusCode != 200 {
		putResp.Body.Close()
		t.Fatalf("expected 200 for put topics, got %d", putResp.StatusCode)
	}
	var putOut map[string]interface{}
	if err := json.NewDecoder(putResp.Body).Decode(&putOut); err != nil {
		t.Fatal(err)
	}
	putResp.Body.Close()
	putNames, _ := putOut["names"].([]interface{})
	if len(putNames) != 3 {
		t.Fatalf("expected 3 topics, got %v", putNames)
	}

	// GET reflects the update.
	getResp2 := ghGet(t, "/api/v3/repos/admin/topics-rest/topics", defaultToken)
	defer getResp2.Body.Close()
	var got2 map[string]interface{}
	if err := json.NewDecoder(getResp2.Body).Decode(&got2); err != nil {
		t.Fatal(err)
	}
	names2, _ := got2["names"].([]interface{})
	if len(names2) != 3 {
		t.Fatalf("expected 3 topics after put, got %v", names2)
	}

	// Repo JSON also exposes topics.
	repoResp := ghGet(t, "/api/v3/repos/admin/topics-rest", defaultToken)
	defer repoResp.Body.Close()
	var repo map[string]interface{}
	if err := json.NewDecoder(repoResp.Body).Decode(&repo); err != nil {
		t.Fatal(err)
	}
	repoTopics, _ := repo["topics"].([]interface{})
	if len(repoTopics) != 3 {
		t.Fatalf("expected 3 topics in repo json, got %v", repoTopics)
	}
}

// TestRepoTopicsREST_Validation verifies topic name validation.
func TestRepoTopicsREST_Validation(t *testing.T) {
	ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name": "topics-validation",
	})

	cases := []struct {
		name   string
		topics []string
	}{
		{"empty topic", []string{"go", ""}},
		{"too long", []string{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}, // 51 chars
		{"invalid char space", []string{"go lang"}},
		{"invalid char slash", []string{"go/lang"}},
		{"too many", make([]string, 21)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := ghPut(t, "/api/v3/repos/admin/topics-validation/topics", defaultToken, map[string]interface{}{
				"names": tc.topics,
			})
			defer resp.Body.Close()
			if resp.StatusCode != 422 {
				t.Fatalf("expected 422, got %d", resp.StatusCode)
			}
		})
	}
}

// TestRepoTopicsREST_PrivateRequiresRead verifies private repo access control.
func TestRepoTopicsREST_PrivateRequiresRead(t *testing.T) {
	ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name":    "topics-private",
		"private": true,
	})

	testServer.store.mu.Lock()
	other := &User{ID: testServer.store.NextUser, Login: "other", Type: "User"}
	testServer.store.NextUser++
	testServer.store.Users[other.ID] = other
	testServer.store.UsersByLogin[other.Login] = other
	otherTok := &Token{Value: "ghp_otherusertoken000000000000000000000000", UserID: other.ID, Scopes: "repo"}
	testServer.store.Tokens[otherTok.Value] = otherTok
	testServer.store.mu.Unlock()

	resp := ghGet(t, "/api/v3/repos/admin/topics-private/topics", otherTok.Value)
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("expected 404 for unreadable private repo, got %d", resp.StatusCode)
	}
}

// TestDeleteContentsFile verifies DELETE /repos/{owner}/{repo}/contents/{path}.
func TestDeleteContentsFile(t *testing.T) {
	ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name":      "delete-contents",
		"auto_init": true,
	})

	// Create a file to delete.
	encoded := base64.StdEncoding.EncodeToString([]byte("to be deleted"))
	putResp := ghPut(t, "/api/v3/repos/admin/delete-contents/contents/remove-me.txt", defaultToken, map[string]interface{}{
		"message": "add file",
		"content": encoded,
	})
	if putResp.StatusCode != 201 {
		putResp.Body.Close()
		t.Fatalf("expected 201, got %d", putResp.StatusCode)
	}
	putData := decodeJSON(t, putResp)
	content, _ := putData["content"].(map[string]interface{})
	sha := content["sha"].(string)

	// Delete the file.
	delResp := ghDeleteWithBody(t, "/api/v3/repos/admin/delete-contents/contents/remove-me.txt", defaultToken, map[string]interface{}{
		"message": "remove file",
		"sha":     sha,
	})
	if delResp.StatusCode != 200 {
		delResp.Body.Close()
		t.Fatalf("expected 200, got %d", delResp.StatusCode)
	}
	delData := decodeJSON(t, delResp)
	if delData["content"] != nil {
		t.Fatalf("expected nil content, got %v", delData["content"])
	}
	commit, _ := delData["commit"].(map[string]interface{})
	if commit["message"] != "remove file" {
		t.Fatalf("expected commit message 'remove file', got %v", commit["message"])
	}

	// File is gone.
	getResp := ghGet(t, "/api/v3/repos/admin/delete-contents/contents/remove-me.txt", defaultToken)
	defer getResp.Body.Close()
	if getResp.StatusCode != 404 {
		t.Fatalf("expected 404 after delete, got %d", getResp.StatusCode)
	}
}

// TestDeleteContentsFile_ShaMismatch verifies deletion is rejected when SHA does not match.
func TestDeleteContentsFile_ShaMismatch(t *testing.T) {
	ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name":      "delete-sha-mismatch",
		"auto_init": true,
	})

	encoded := base64.StdEncoding.EncodeToString([]byte("content"))
	putResp := ghPut(t, "/api/v3/repos/admin/delete-sha-mismatch/contents/x.txt", defaultToken, map[string]interface{}{
		"message": "add file",
		"content": encoded,
	})
	if putResp.StatusCode != 201 {
		putResp.Body.Close()
		t.Fatalf("expected 201, got %d", putResp.StatusCode)
	}
	putResp.Body.Close()

	delResp := ghDeleteWithBody(t, "/api/v3/repos/admin/delete-sha-mismatch/contents/x.txt", defaultToken, map[string]interface{}{
		"message": "remove file",
		"sha":     "0000000000000000000000000000000000000000",
	})
	defer delResp.Body.Close()
	if delResp.StatusCode != 422 {
		t.Fatalf("expected 422 for sha mismatch, got %d", delResp.StatusCode)
	}
}

// TestDeleteContentsFile_NonExistentPath verifies deletion of a missing path returns 422.
func TestDeleteContentsFile_NonExistentPath(t *testing.T) {
	ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name":      "delete-missing",
		"auto_init": true,
	})

	delResp := ghDeleteWithBody(t, "/api/v3/repos/admin/delete-missing/contents/nope.txt", defaultToken, map[string]interface{}{
		"message": "remove file",
		"sha":     "0000000000000000000000000000000000000000",
	})
	defer delResp.Body.Close()
	if delResp.StatusCode != 422 {
		t.Fatalf("expected 422 for missing path, got %d", delResp.StatusCode)
	}
}

// TestDeleteContentsFile_RequiresPush verifies write access is enforced.
func TestDeleteContentsFile_RequiresPush(t *testing.T) {
	ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name": "delete-perms",
	})

	testServer.store.mu.Lock()
	other := &User{ID: testServer.store.NextUser, Login: "other", Type: "User"}
	testServer.store.NextUser++
	testServer.store.Users[other.ID] = other
	testServer.store.UsersByLogin[other.Login] = other
	otherTok := &Token{Value: "ghp_otherusertoken000000000000000000000000", UserID: other.ID, Scopes: "repo"}
	testServer.store.Tokens[otherTok.Value] = otherTok
	testServer.store.mu.Unlock()

	delResp := ghDeleteWithBody(t, "/api/v3/repos/admin/delete-perms/contents/x.txt", otherTok.Value, map[string]interface{}{
		"message": "remove file",
		"sha":     "0000000000000000000000000000000000000000",
	})
	defer delResp.Body.Close()
	if delResp.StatusCode != 403 {
		t.Fatalf("expected 403, got %d", delResp.StatusCode)
	}
}

// helper for DELETE with JSON body.
func ghDeleteWithBody(t *testing.T, path, token string, body map[string]interface{}) *http.Response {
	t.Helper()
	var bodyReader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(b)
	}
	req, err := http.NewRequest("DELETE", testBaseURL+path, bodyReader)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "token "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}
