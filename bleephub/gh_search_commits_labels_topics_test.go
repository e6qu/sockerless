package bleephub

import (
	"encoding/base64"
	"strconv"
	"testing"
)

// putTestFile creates or updates a file through the real contents API,
// producing a real git commit with the given message.
func putTestFile(t *testing.T, repoKey, path, message, content string) {
	t.Helper()
	resp := ghPut(t, "/api/v3/repos/"+repoKey+"/contents/"+path, defaultToken, map[string]interface{}{
		"message": message,
		"content": base64.StdEncoding.EncodeToString([]byte(content)),
	})
	decodeJSONWithStatus(t, resp, 201)
}

func TestSearchCommits(t *testing.T) {
	repoKey := createTestRepo(t)
	putTestFile(t, repoKey, "a.txt", "add alpha searchable-commit-marker", "alpha")
	putTestFile(t, repoKey, "b.txt", "add beta unrelated", "beta")

	resp := ghGet(t, "/api/v3/search/commits?q=searchable-commit-marker+repo:"+repoKey, defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("search commits status = %d", resp.StatusCode)
	}
	env := decodeJSON(t, resp)
	if env["total_count"] != float64(1) {
		t.Fatalf("total_count = %v, want 1", env["total_count"])
	}
	items, _ := env["items"].([]interface{})
	if len(items) != 1 {
		t.Fatalf("items = %d", len(items))
	}
	item, _ := items[0].(map[string]interface{})
	commit, _ := item["commit"].(map[string]interface{})
	if commit == nil {
		t.Fatalf("item missing commit: %v", item)
	}
	if commit["message"] != "add alpha searchable-commit-marker" {
		t.Fatalf("commit message = %v", commit["message"])
	}
	sha, _ := item["sha"].(string)
	if len(sha) != 40 {
		t.Fatalf("sha = %q", sha)
	}
	repoJSON, _ := item["repository"].(map[string]interface{})
	if repoJSON == nil || repoJSON["full_name"] != repoKey {
		t.Fatalf("repository = %v", item["repository"])
	}
	author, _ := commit["author"].(map[string]interface{})
	if author == nil || author["name"] == "" || author["date"] == nil {
		t.Fatalf("commit author = %v", commit["author"])
	}

	// hash: qualifier finds the same commit by its real SHA.
	resp = ghGet(t, "/api/v3/search/commits?q=hash:"+sha+"+repo:"+repoKey, defaultToken)
	env = decodeJSON(t, resp)
	if env["total_count"] != float64(1) {
		t.Fatalf("hash search total_count = %v", env["total_count"])
	}

	// Empty query → 422.
	resp = ghGet(t, "/api/v3/search/commits", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Fatalf("empty q status = %d, want 422", resp.StatusCode)
	}
}

func TestSearchLabels(t *testing.T) {
	repoKey := createTestRepo(t)
	resp := ghPost(t, "/api/v3/repos/"+repoKey+"/labels", defaultToken, map[string]interface{}{
		"name": "searchable-bug", "color": "d73a4a", "description": "Something is broken",
	})
	decodeJSONWithStatus(t, resp, 201)
	resp = ghPost(t, "/api/v3/repos/"+repoKey+"/labels", defaultToken, map[string]interface{}{
		"name": "enhancement", "color": "a2eeef",
	})
	decodeJSONWithStatus(t, resp, 201)

	repoResp := ghGet(t, "/api/v3/repos/"+repoKey, defaultToken)
	repoData := decodeJSONWithStatus(t, repoResp, 200)
	repoID := strconv.Itoa(int(repoData["id"].(float64)))

	resp = ghGet(t, "/api/v3/search/labels?repository_id="+repoID+"&q=searchable", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("search labels status = %d", resp.StatusCode)
	}
	env := decodeJSON(t, resp)
	if env["total_count"] != float64(1) {
		t.Fatalf("total_count = %v", env["total_count"])
	}
	items, _ := env["items"].([]interface{})
	label, _ := items[0].(map[string]interface{})
	if label["name"] != "searchable-bug" || label["color"] != "d73a4a" {
		t.Fatalf("label item = %v", label)
	}
	if label["description"] != "Something is broken" {
		t.Fatalf("label description = %v", label["description"])
	}

	// Missing repository_id → 422; unknown repository → 404.
	resp = ghGet(t, "/api/v3/search/labels?q=x", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Fatalf("missing repository_id status = %d, want 422", resp.StatusCode)
	}
	resp = ghGet(t, "/api/v3/search/labels?repository_id=999999&q=x", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("unknown repository status = %d, want 404", resp.StatusCode)
	}
}

func TestSearchTopics(t *testing.T) {
	repoKey := createTestRepo(t)
	resp := ghPut(t, "/api/v3/repos/"+repoKey+"/topics", defaultToken, map[string]interface{}{
		"names": []string{"searchable-topic-golang", "other-subject"},
	})
	decodeJSONWithStatus(t, resp, 200)

	resp = ghGet(t, "/api/v3/search/topics?q=searchable-topic", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("search topics status = %d", resp.StatusCode)
	}
	env := decodeJSON(t, resp)
	if env["total_count"] != float64(1) {
		t.Fatalf("total_count = %v", env["total_count"])
	}
	items, _ := env["items"].([]interface{})
	topic, _ := items[0].(map[string]interface{})
	if topic["name"] != "searchable-topic-golang" {
		t.Fatalf("topic item = %v", topic)
	}
	if topic["repository_count"] != float64(1) || topic["created_at"] == nil {
		t.Fatalf("topic aggregation = %v", topic)
	}

	// Missing q → 422.
	resp = ghGet(t, "/api/v3/search/topics", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Fatalf("missing q status = %d, want 422", resp.StatusCode)
	}
}
