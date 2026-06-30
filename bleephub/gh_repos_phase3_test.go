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

	// Tag the default branch via the git/refs endpoint (not implemented yet,
	// so create a tag ref directly through a commit reference using the
	// existing delete-ref storage path is not available). Instead, use the
	// contents endpoint to make the repo distinct and rely on the fact that
	// there are no tags initially.
	resp := ghGet(t, "/api/v3/repos/admin/tags-repo/tags", defaultToken)
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var tags []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		t.Fatal(err)
	}
	if len(tags) != 0 {
		t.Fatalf("expected 0 tags, got %d", len(tags))
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
