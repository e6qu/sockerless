package bleephub

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestListTags verifies GET /repos/{owner}/{repo}/tags.
func TestListTags(t *testing.T) {
	ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name":      "tags-repo",
		"auto_init": true,
	})

	refResp := ghGet(t, "/api/v3/repos/admin/tags-repo/git/refs/heads/main", defaultToken)
	defer refResp.Body.Close()
	if refResp.StatusCode != 200 {
		t.Fatalf("main ref status = %d", refResp.StatusCode)
	}
	var mainRef map[string]interface{}
	if err := json.NewDecoder(refResp.Body).Decode(&mainRef); err != nil {
		t.Fatal(err)
	}
	mainObj, _ := mainRef["object"].(map[string]interface{})
	mainSHA, _ := mainObj["sha"].(string)
	if mainSHA == "" {
		t.Fatalf("main ref missing sha: %v", mainRef)
	}

	tagResp := ghPost(t, "/api/v3/repos/admin/tags-repo/git/tags", defaultToken, map[string]interface{}{
		"tag":     "v1.0.0",
		"message": "release",
		"object":  mainSHA,
		"type":    "commit",
	})
	defer tagResp.Body.Close()
	if tagResp.StatusCode != 201 {
		t.Fatalf("tag object status = %d", tagResp.StatusCode)
	}
	var tagObj map[string]interface{}
	if err := json.NewDecoder(tagResp.Body).Decode(&tagObj); err != nil {
		t.Fatal(err)
	}
	tagSHA, _ := tagObj["sha"].(string)
	if tagSHA == "" || tagSHA == mainSHA {
		t.Fatalf("tag object sha = %q, main sha = %q", tagSHA, mainSHA)
	}

	annotatedRefResp := ghPost(t, "/api/v3/repos/admin/tags-repo/git/refs", defaultToken, map[string]interface{}{
		"ref": "refs/tags/v1.0.0",
		"sha": tagSHA,
	})
	defer annotatedRefResp.Body.Close()
	if annotatedRefResp.StatusCode != 201 {
		t.Fatalf("annotated tag ref status = %d", annotatedRefResp.StatusCode)
	}

	lightweightRefResp := ghPost(t, "/api/v3/repos/admin/tags-repo/git/refs", defaultToken, map[string]interface{}{
		"ref": "refs/tags/v1.0.1",
		"sha": mainSHA,
	})
	defer lightweightRefResp.Body.Close()
	if lightweightRefResp.StatusCode != 201 {
		t.Fatalf("lightweight tag ref status = %d", lightweightRefResp.StatusCode)
	}

	resp := ghGet(t, "/api/v3/repos/admin/tags-repo/tags", defaultToken)
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var tags []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		t.Fatal(err)
	}
	if len(tags) != 2 {
		t.Fatalf("expected 2 tags, got %d: %v", len(tags), tags)
	}
	byName := map[string]map[string]interface{}{}
	for _, tag := range tags {
		name, _ := tag["name"].(string)
		byName[name] = tag
	}
	for _, name := range []string{"v1.0.0", "v1.0.1"} {
		tag := byName[name]
		if tag == nil {
			t.Fatalf("missing tag %s in %v", name, tags)
		}
		commit, _ := tag["commit"].(map[string]interface{})
		if commit["sha"] != mainSHA {
			t.Fatalf("tag %s commit sha = %v, want %s", name, commit["sha"], mainSHA)
		}
		if strings.Contains(commit["url"].(string), tagSHA) {
			t.Fatalf("tag %s commit URL pointed at annotated tag object: %v", name, commit["url"])
		}
	}
}

// TestListRefs_All verifies GET /repos/{owner}/{repo}/git/refs.
func TestListRefs_All(t *testing.T) {
	ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name":      "refs-repo",
		"auto_init": true,
	})

	resp := ghGet(t, "/api/v3/repos/admin/refs-repo/git/refs", defaultToken)
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var refs []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&refs); err != nil {
		t.Fatal(err)
	}
	if len(refs) == 0 {
		t.Fatalf("expected at least one ref, got %d", len(refs))
	}
	foundMain := false
	for _, ref := range refs {
		if ref["ref"] == "refs/heads/main" {
			foundMain = true
		}
		obj, _ := ref["object"].(map[string]interface{})
		if obj["type"] == "" {
			t.Fatalf("expected object type, got %v", obj)
		}
		if obj["sha"] == "" {
			t.Fatalf("expected object sha, got %v", obj)
		}
	}
	if !foundMain {
		t.Fatalf("expected refs/heads/main in %v", refs)
	}
}

// TestListRefs_HeadsNamespace verifies GET /repos/{owner}/{repo}/git/refs/heads.
func TestListRefs_HeadsNamespace(t *testing.T) {
	ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name":      "refs-heads-repo",
		"auto_init": true,
	})

	resp := ghGet(t, "/api/v3/repos/admin/refs-heads-repo/git/refs/heads", defaultToken)
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var refs []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&refs); err != nil {
		t.Fatal(err)
	}
	if len(refs) == 0 {
		t.Fatalf("expected at least one head, got %d", len(refs))
	}
	for _, ref := range refs {
		name, _ := ref["ref"].(string)
		if !strings.HasPrefix(name, "refs/heads/") {
			t.Fatalf("expected branch ref, got %s", name)
		}
	}
}

// TestGetRef_Single verifies GET /repos/{owner}/{repo}/git/refs/heads/main.
func TestGetRef_Single(t *testing.T) {
	ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name":      "ref-single-repo",
		"auto_init": true,
	})

	resp := ghGet(t, "/api/v3/repos/admin/ref-single-repo/git/refs/heads/main", defaultToken)
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var ref map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&ref); err != nil {
		t.Fatal(err)
	}
	if ref["ref"] != "refs/heads/main" {
		t.Fatalf("expected refs/heads/main, got %v", ref["ref"])
	}
	obj, _ := ref["object"].(map[string]interface{})
	if obj["type"] != "commit" {
		t.Fatalf("expected type commit, got %v", obj["type"])
	}
	if obj["sha"] == "" {
		t.Fatalf("expected sha, got %v", obj["sha"])
	}
}

// TestGetRef_NotFound verifies GET for a non-existent ref returns 404.
func TestGetRef_NotFound(t *testing.T) {
	ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name":      "ref-notfound-repo",
		"auto_init": true,
	})

	resp := ghGet(t, "/api/v3/repos/admin/ref-notfound-repo/git/refs/heads/nope", defaultToken)
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

// TestListRefs_TagsNamespaceEmpty verifies GET /repos/{owner}/{repo}/git/refs/tags returns empty array.
func TestListRefs_TagsNamespaceEmpty(t *testing.T) {
	ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name":      "refs-tags-empty-repo",
		"auto_init": true,
	})

	resp := ghGet(t, "/api/v3/repos/admin/refs-tags-empty-repo/git/refs/tags", defaultToken)
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var refs []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&refs); err != nil {
		t.Fatal(err)
	}
	if len(refs) != 0 {
		t.Fatalf("expected 0 tags, got %d", len(refs))
	}
}
