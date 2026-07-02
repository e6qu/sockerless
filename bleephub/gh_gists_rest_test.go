package bleephub

import (
	"fmt"
	"net/http"
	"testing"
)

func createTestGist(t *testing.T, token string, public bool) map[string]interface{} {
	t.Helper()
	resp := ghPost(t, "/api/v3/gists", token, map[string]interface{}{
		"description": "test gist",
		"public":      public,
		"files": map[string]interface{}{
			"hello.go": map[string]interface{}{
				"content": "package main\n\nfunc main() {}",
			},
		},
	})
	if resp.StatusCode != 201 {
		resp.Body.Close()
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	return decodeJSON(t, resp)
}

func TestCreateGist(t *testing.T) {
	data := createTestGist(t, defaultToken, true)
	if data["description"] != "test gist" {
		t.Fatalf("expected description='test gist', got %v", data["description"])
	}
	if data["public"] != true {
		t.Fatalf("expected public=true, got %v", data["public"])
	}
	files, ok := data["files"].(map[string]interface{})
	if !ok {
		t.Fatal("expected files object")
	}
	hello, ok := files["hello.go"].(map[string]interface{})
	if !ok {
		t.Fatal("expected hello.go file object")
	}
	if hello["content"] != "package main\n\nfunc main() {}" {
		t.Fatalf("unexpected content: %v", hello["content"])
	}
	if hello["filename"] != "hello.go" {
		t.Fatalf("unexpected filename: %v", hello["filename"])
	}
	if data["id"] == nil || data["id"] == "" {
		t.Fatal("expected gist id")
	}
}

func TestCreateGistNoFiles(t *testing.T) {
	resp := ghPost(t, "/api/v3/gists", defaultToken, map[string]interface{}{
		"description": "empty",
		"public":      true,
		"files":       map[string]interface{}{},
	})
	defer resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Fatalf("expected 422, got %d", resp.StatusCode)
	}
}

func TestGetGist(t *testing.T) {
	created := createTestGist(t, defaultToken, true)
	id := created["id"].(string)

	resp := ghGet(t, "/api/v3/gists/"+id, defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)
	if data["id"] != id {
		t.Fatalf("expected id=%s, got %v", id, data["id"])
	}
	if data["url"] == nil {
		t.Fatal("expected url")
	}
	if data["forks_url"] == nil {
		t.Fatal("expected forks_url")
	}
	files := data["files"].(map[string]interface{})
	hello := files["hello.go"].(map[string]interface{})
	if hello["content"] == nil {
		t.Fatal("expected content in single gist response")
	}
}

func TestGetGistNotFound(t *testing.T) {
	resp := ghGet(t, "/api/v3/gists/nosuchgist", defaultToken)
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestUpdateGist(t *testing.T) {
	created := createTestGist(t, defaultToken, true)
	id := created["id"].(string)

	resp := ghPatch(t, "/api/v3/gists/"+id, defaultToken, map[string]interface{}{
		"description": "updated description",
		"files": map[string]interface{}{
			"hello.go": map[string]interface{}{
				"content": "package main\n\nfunc main() { println(\"hi\") }",
			},
			"new.txt": map[string]interface{}{
				"content": "new file content",
			},
		},
	})
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)
	if data["description"] != "updated description" {
		t.Fatalf("expected updated description, got %v", data["description"])
	}
	files := data["files"].(map[string]interface{})
	if _, ok := files["new.txt"]; !ok {
		t.Fatal("expected new.txt file after update")
	}
	hello := files["hello.go"].(map[string]interface{})
	if hello["content"] != "package main\n\nfunc main() { println(\"hi\") }" {
		t.Fatalf("unexpected updated content: %v", hello["content"])
	}
	if len(data["history"].([]interface{})) == 0 {
		t.Fatal("expected history entry after update")
	}
}

func TestUpdateGistDeleteFile(t *testing.T) {
	created := createTestGist(t, defaultToken, true)
	id := created["id"].(string)

	resp := ghPatch(t, "/api/v3/gists/"+id, defaultToken, map[string]interface{}{
		"files": map[string]interface{}{
			"hello.go": nil,
		},
	})
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)
	files := data["files"].(map[string]interface{})
	if _, ok := files["hello.go"]; ok {
		t.Fatal("expected hello.go to be deleted")
	}
}

func TestDeleteGist(t *testing.T) {
	created := createTestGist(t, defaultToken, true)
	id := created["id"].(string)

	resp := ghDelete(t, "/api/v3/gists/"+id, defaultToken)
	defer resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}

	resp2 := ghGet(t, "/api/v3/gists/"+id, defaultToken)
	defer resp2.Body.Close()
	if resp2.StatusCode != 404 {
		t.Fatalf("expected 404 after delete, got %d", resp2.StatusCode)
	}
}

func TestStarGist(t *testing.T) {
	created := createTestGist(t, defaultToken, true)
	id := created["id"].(string)

	star := ghPut(t, "/api/v3/gists/"+id+"/star", defaultToken, nil)
	defer star.Body.Close()
	if star.StatusCode != 204 {
		t.Fatalf("expected 204 on star, got %d", star.StatusCode)
	}

	check := ghGet(t, "/api/v3/gists/"+id+"/star", defaultToken)
	defer check.Body.Close()
	if check.StatusCode != 204 {
		t.Fatalf("expected 204 on check starred, got %d", check.StatusCode)
	}

	unstar := ghDelete(t, "/api/v3/gists/"+id+"/star", defaultToken)
	defer unstar.Body.Close()
	if unstar.StatusCode != 204 {
		t.Fatalf("expected 204 on unstar, got %d", unstar.StatusCode)
	}

	check2 := ghGet(t, "/api/v3/gists/"+id+"/star", defaultToken)
	defer check2.Body.Close()
	if check2.StatusCode != 404 {
		t.Fatalf("expected 404 after unstar, got %d", check2.StatusCode)
	}
}

func TestListStarredGists(t *testing.T) {
	created := createTestGist(t, defaultToken, true)
	id := created["id"].(string)
	ghPut(t, "/api/v3/gists/"+id+"/star", defaultToken, nil).Body.Close()

	resp := ghGet(t, "/api/v3/gists/starred", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	data := decodeJSONArray(t, resp)
	found := false
	for _, g := range data {
		if g["id"] == id {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected starred gist in list")
	}
}

func TestForkGist(t *testing.T) {
	created := createTestGist(t, defaultToken, true)
	id := created["id"].(string)

	resp := ghPost(t, "/api/v3/gists/"+id+"/forks", defaultToken, nil)
	if resp.StatusCode != 201 {
		resp.Body.Close()
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)
	if data["id"] == id {
		t.Fatal("fork id should differ from original")
	}

	listResp := ghGet(t, "/api/v3/gists/"+id+"/forks", defaultToken)
	if listResp.StatusCode != 200 {
		listResp.Body.Close()
		t.Fatalf("expected 200, got %d", listResp.StatusCode)
	}
	forks := decodeJSONArray(t, listResp)
	if len(forks) != 1 {
		t.Fatalf("expected 1 fork, got %d", len(forks))
	}
}

func TestGistComments(t *testing.T) {
	created := createTestGist(t, defaultToken, true)
	id := created["id"].(string)

	createResp := ghPost(t, "/api/v3/gists/"+id+"/comments", defaultToken, map[string]interface{}{
		"body": "first comment",
	})
	if createResp.StatusCode != 201 {
		createResp.Body.Close()
		t.Fatalf("expected 201, got %d", createResp.StatusCode)
	}
	comment := decodeJSON(t, createResp)
	commentID := fmt.Sprintf("%v", comment["id"])
	if comment["body"] != "first comment" {
		t.Fatalf("unexpected comment body: %v", comment["body"])
	}

	getResp := ghGet(t, "/api/v3/gists/"+id+"/comments/"+commentID, defaultToken)
	if getResp.StatusCode != 200 {
		getResp.Body.Close()
		t.Fatalf("expected 200, got %d", getResp.StatusCode)
	}
	got := decodeJSON(t, getResp)
	if got["body"] != "first comment" {
		t.Fatalf("unexpected body on get: %v", got["body"])
	}

	patchResp := ghPatch(t, "/api/v3/gists/"+id+"/comments/"+commentID, defaultToken, map[string]interface{}{
		"body": "updated comment",
	})
	if patchResp.StatusCode != 200 {
		patchResp.Body.Close()
		t.Fatalf("expected 200, got %d", patchResp.StatusCode)
	}
	updated := decodeJSON(t, patchResp)
	if updated["body"] != "updated comment" {
		t.Fatalf("unexpected updated body: %v", updated["body"])
	}

	listResp := ghGet(t, "/api/v3/gists/"+id+"/comments", defaultToken)
	if listResp.StatusCode != 200 {
		listResp.Body.Close()
		t.Fatalf("expected 200, got %d", listResp.StatusCode)
	}
	comments := decodeJSONArray(t, listResp)
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(comments))
	}

	delResp := ghDelete(t, "/api/v3/gists/"+id+"/comments/"+commentID, defaultToken)
	defer delResp.Body.Close()
	if delResp.StatusCode != 204 {
		t.Fatalf("expected 204, got %d", delResp.StatusCode)
	}

	getResp2 := ghGet(t, "/api/v3/gists/"+id+"/comments/"+commentID, defaultToken)
	defer getResp2.Body.Close()
	if getResp2.StatusCode != 404 {
		t.Fatalf("expected 404 after delete, got %d", getResp2.StatusCode)
	}
}

func TestListGistsForAuthUser(t *testing.T) {
	created := createTestGist(t, defaultToken, true)
	id := created["id"].(string)

	resp := ghGet(t, "/api/v3/gists", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	data := decodeJSONArray(t, resp)
	found := false
	for _, g := range data {
		if g["id"] == id {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected gist in authenticated user list")
	}
}

func TestListPublicGists(t *testing.T) {
	created := createTestGist(t, defaultToken, true)
	id := created["id"].(string)

	resp := ghGet(t, "/api/v3/gists/public", "")
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	data := decodeJSONArray(t, resp)
	found := false
	for _, g := range data {
		if g["id"] == id {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected public gist in public list")
	}
}

func TestPrivateGistHiddenFromOthers(t *testing.T) {
	created := createTestGist(t, defaultToken, false)
	id := created["id"].(string)

	resp := ghGet(t, "/api/v3/gists/"+id, "")
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("expected 404 for private gist without auth, got %d", resp.StatusCode)
	}
}

func TestGistPagination(t *testing.T) {
	for i := 0; i < 3; i++ {
		createTestGist(t, defaultToken, true)
	}

	req, err := http.NewRequest("GET", testBaseURL+"/api/v3/gists?per_page=1&page=1", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "token "+defaultToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	link := resp.Header.Get("Link")
	if link == "" {
		t.Fatal("expected Link header for pagination")
	}
}

func TestListGistCommits(t *testing.T) {
	created := createTestGist(t, defaultToken, true)
	id := created["id"].(string)

	resp := ghGet(t, "/api/v3/gists/"+id+"/commits", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	data := decodeJSONArray(t, resp)
	if len(data) == 0 {
		t.Fatal("expected at least one commit")
	}
	if data[0]["version"] == nil {
		t.Fatal("expected version in commit")
	}
}

func TestGetGistAtRevision(t *testing.T) {
	created := createTestGist(t, defaultToken, true)
	id := created["id"].(string)

	commitsResp := ghGet(t, "/api/v3/gists/"+id+"/commits", defaultToken)
	commits := decodeJSONArray(t, commitsResp)
	sha := commits[0]["version"].(string)

	resp := ghGet(t, "/api/v3/gists/"+id+"/"+sha, defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)
	if data["id"] != id {
		t.Fatalf("expected id=%s, got %v", id, data["id"])
	}
}
