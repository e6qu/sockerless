package bleephub

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/storage/memory"
)

func ghPatch(t *testing.T, path string, token string, body interface{}) *http.Response {
	t.Helper()
	var bodyReader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(b)
	}
	req, err := http.NewRequest("PATCH", testBaseURL+path, bodyReader)
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

func ghDelete(t *testing.T, path string, token string) *http.Response {
	t.Helper()
	req, err := http.NewRequest("DELETE", testBaseURL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "token "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// TestCreateRepo verifies POST /api/v3/user/repos → 201.
func TestCreateRepo(t *testing.T) {
	resp := ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name":        "test-create",
		"description": "A test repo",
		"private":     false,
	})
	if resp.StatusCode != 201 {
		resp.Body.Close()
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)

	if data["name"] != "test-create" {
		t.Fatalf("expected name=test-create, got %v", data["name"])
	}
	if data["full_name"] != "admin/test-create" {
		t.Fatalf("expected full_name=admin/test-create, got %v", data["full_name"])
	}
	if data["description"] != "A test repo" {
		t.Fatalf("expected description='A test repo', got %v", data["description"])
	}
	if data["private"] != false {
		t.Fatalf("expected private=false, got %v", data["private"])
	}
	if data["clone_url"] == nil {
		t.Fatal("missing clone_url")
	}
	if data["default_branch"] != "main" {
		t.Fatalf("expected default_branch=main, got %v", data["default_branch"])
	}
}

// TestCreateRepoNoAuth verifies POST /api/v3/user/repos without token → 401.
func TestCreateRepoNoAuth(t *testing.T) {
	resp := ghPost(t, "/api/v3/user/repos", "", map[string]interface{}{
		"name": "should-fail",
	})
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

// TestGetRepo verifies GET /api/v3/repos/admin/test-create → 200.
func TestGetRepo(t *testing.T) {
	// Ensure repo exists
	ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name": "test-get",
	})

	resp := ghGet(t, "/api/v3/repos/admin/test-get", "")
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)

	if data["name"] != "test-get" {
		t.Fatalf("expected name=test-get, got %v", data["name"])
	}
	if data["owner"] == nil {
		t.Fatal("missing owner")
	}
}

// TestGetRepoNotFound verifies GET for nonexistent repo → 404.
func TestGetRepoNotFound(t *testing.T) {
	resp := ghGet(t, "/api/v3/repos/admin/nonexistent", "")
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

// TestUpdateRepo verifies PATCH → description changed.
func TestUpdateRepo(t *testing.T) {
	ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name": "test-update",
	})

	resp := ghPatch(t, "/api/v3/repos/admin/test-update", defaultToken, map[string]interface{}{
		"description": "Updated description",
	})
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)

	if data["description"] != "Updated description" {
		t.Fatalf("expected updated description, got %v", data["description"])
	}
}

// TestDeleteRepo verifies DELETE → 204, subsequent GET → 404.
func TestDeleteRepo(t *testing.T) {
	ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name": "test-delete",
	})

	resp := ghDelete(t, "/api/v3/repos/admin/test-delete", defaultToken)
	defer resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}

	resp2 := ghGet(t, "/api/v3/repos/admin/test-delete", "")
	defer resp2.Body.Close()
	if resp2.StatusCode != 404 {
		t.Fatalf("expected 404 after delete, got %d", resp2.StatusCode)
	}
}

// TestListUserRepos verifies GET /api/v3/user/repos → array.
func TestListUserRepos(t *testing.T) {
	ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name": "test-list",
	})

	resp := ghGet(t, "/api/v3/user/repos", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	defer resp.Body.Close()

	var repos []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&repos); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if len(repos) == 0 {
		t.Fatal("expected at least 1 repo")
	}
}

// TestListBranches verifies GET branches → list (empty for new repo).
func TestListBranches(t *testing.T) {
	ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name": "test-branches",
	})

	resp := ghGet(t, "/api/v3/repos/admin/test-branches/branches", "")
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	defer resp.Body.Close()

	var branches []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&branches); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	// Empty repo has no branches
	if len(branches) != 0 {
		t.Fatalf("expected 0 branches for empty repo, got %d", len(branches))
	}
}

// TestGraphQLRepository verifies the repository query.
func TestGraphQLRepository(t *testing.T) {
	ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name":    "test-gql",
		"private": true,
	})

	resp := ghPost(t, "/api/graphql", defaultToken, map[string]string{
		"query": `{repository(owner:"admin",name:"test-gql"){name,isPrivate,visibility}}`,
	})
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)

	d, _ := data["data"].(map[string]interface{})
	repo, _ := d["repository"].(map[string]interface{})
	if repo == nil {
		t.Fatalf("expected repository in data: %v", data)
	}
	if repo["name"] != "test-gql" {
		t.Fatalf("expected name=test-gql, got %v", repo["name"])
	}
	if repo["isPrivate"] != true {
		t.Fatalf("expected isPrivate=true, got %v", repo["isPrivate"])
	}
	if repo["visibility"] != "PRIVATE" {
		t.Fatalf("expected visibility=PRIVATE, got %v", repo["visibility"])
	}
}

// TestGraphQLViewerRepos verifies viewer { repositories } query.
func TestGraphQLViewerRepos(t *testing.T) {
	ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name": "test-viewer-repos",
	})

	resp := ghPost(t, "/api/graphql", defaultToken, map[string]string{
		"query": `{viewer{repositories(first:10){nodes{nameWithOwner},totalCount,pageInfo{hasNextPage}}}}`,
	})
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)

	d, _ := data["data"].(map[string]interface{})
	viewer, _ := d["viewer"].(map[string]interface{})
	repos, _ := viewer["repositories"].(map[string]interface{})
	if repos == nil {
		t.Fatalf("expected repositories in viewer: %v", data)
	}

	totalCount, _ := repos["totalCount"].(float64)
	if totalCount < 1 {
		t.Fatalf("expected totalCount >= 1, got %v", totalCount)
	}

	nodes, _ := repos["nodes"].([]interface{})
	if len(nodes) == 0 {
		t.Fatal("expected at least 1 node")
	}

	pageInfo, _ := repos["pageInfo"].(map[string]interface{})
	if pageInfo == nil {
		t.Fatal("missing pageInfo")
	}
}

// TestGraphQLCreateRepo verifies the createRepository mutation.
func TestGraphQLCreateRepo(t *testing.T) {
	resp := ghPost(t, "/api/graphql", defaultToken, map[string]string{
		"query": `mutation{createRepository(input:{name:"gql-created",visibility:"PUBLIC"}){repository{name,owner{login},isPrivate}}}`,
	})
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)

	d, _ := data["data"].(map[string]interface{})
	payload, _ := d["createRepository"].(map[string]interface{})
	repo, _ := payload["repository"].(map[string]interface{})
	if repo == nil {
		t.Fatalf("expected repository in createRepository payload: %v", data)
	}
	if repo["name"] != "gql-created" {
		t.Fatalf("expected name=gql-created, got %v", repo["name"])
	}

	owner, _ := repo["owner"].(map[string]interface{})
	if owner == nil || owner["login"] != "admin" {
		t.Fatalf("expected owner.login=admin, got %v", owner)
	}
	if repo["isPrivate"] != false {
		t.Fatalf("expected isPrivate=false for PUBLIC repo, got %v", repo["isPrivate"])
	}
}

// TestGraphQLRepoNotFound verifies null result for nonexistent repo.
func TestGraphQLRepoNotFound(t *testing.T) {
	resp := ghPost(t, "/api/graphql", defaultToken, map[string]string{
		"query": `{repository(owner:"admin",name:"nonexistent"){name}}`,
	})
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)

	d, _ := data["data"].(map[string]interface{})
	if d["repository"] != nil {
		t.Fatalf("expected null repository, got %v", d["repository"])
	}
}

// TestGitInfoRefs verifies correct content-type and pkt-line response.
func TestGitInfoRefs(t *testing.T) {
	ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name": "test-git-info",
	})

	resp, err := http.Get(testBaseURL + "/admin/test-git-info.git/info/refs?service=git-upload-pack")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if ct != "application/x-git-upload-pack-advertisement" {
		t.Fatalf("expected git content-type, got %s", ct)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "# service=git-upload-pack") {
		t.Fatalf("expected pkt-line service header in response")
	}
}

// TestGitClonePush verifies creating a repo, pushing a commit via go-git, and verifying content.
func TestGitClonePush(t *testing.T) {
	// Create repo via API
	resp := ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name": "test-git-push",
	})
	resp.Body.Close()

	// Init locally and push (empty repo can't be cloned)
	cloneStorage := memory.NewStorage()
	repo, err := git.Init(cloneStorage, nil)
	if err != nil {
		t.Fatalf("failed to init: %v", err)
	}

	// Create remote
	_, err = repo.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{testBaseURL + "/admin/test-git-push.git"},
	})
	if err != nil {
		t.Fatalf("failed to create remote: %v", err)
	}

	// Create a commit using the go-git worktree-free approach
	// We need to create objects directly in memory storage
	blobHash, err := storeBlob(cloneStorage, []byte("Hello, bleephub!\n"))
	if err != nil {
		t.Fatalf("failed to store blob: %v", err)
	}

	treeHash, err := storeTree(cloneStorage, []object.TreeEntry{
		{Name: "README.md", Mode: 0100644, Hash: blobHash},
	})
	if err != nil {
		t.Fatalf("failed to store tree: %v", err)
	}

	commitHash, err := storeCommit(cloneStorage, treeHash, plumbing.ZeroHash, "Initial commit")
	if err != nil {
		t.Fatalf("failed to store commit: %v", err)
	}

	// Update refs/heads/main
	ref := plumbing.NewHashReference(plumbing.NewBranchReferenceName("main"), commitHash)
	if err := cloneStorage.SetReference(ref); err != nil {
		t.Fatalf("failed to set ref: %v", err)
	}
	headRef := plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.NewBranchReferenceName("main"))
	if err := cloneStorage.SetReference(headRef); err != nil {
		t.Fatalf("failed to set HEAD: %v", err)
	}

	// Push to bleephub
	err = repo.Push(&git.PushOptions{
		RemoteName: "origin",
		Auth:       &githttp.BasicAuth{Username: "x", Password: defaultToken},
	})
	if err != nil {
		t.Fatalf("failed to push: %v", err)
	}

	// Verify: list branches should now show main
	resp2 := ghGet(t, "/api/v3/repos/admin/test-git-push/branches", "")
	if resp2.StatusCode != 200 {
		resp2.Body.Close()
		t.Fatalf("expected 200, got %d", resp2.StatusCode)
	}
	defer resp2.Body.Close()

	var branches []map[string]interface{}
	json.NewDecoder(resp2.Body).Decode(&branches)

	found := false
	for _, b := range branches {
		if b["name"] == "main" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected 'main' branch after push, got %v", branches)
	}

	// Verify: README endpoint should work
	resp3 := ghGet(t, "/api/v3/repos/admin/test-git-push/readme", "")
	if resp3.StatusCode != 200 {
		resp3.Body.Close()
		t.Fatalf("expected 200 for readme, got %d", resp3.StatusCode)
	}
	readmeData := decodeJSON(t, resp3)
	if readmeData["name"] != "README.md" {
		t.Fatalf("expected readme name=README.md, got %v", readmeData["name"])
	}
}

// Helper functions for creating git objects in memory storage.

func storeBlob(s *memory.Storage, content []byte) (plumbing.Hash, error) {
	obj := s.NewEncodedObject()
	obj.SetType(plumbing.BlobObject)
	obj.SetSize(int64(len(content)))

	w, err := obj.Writer()
	if err != nil {
		return plumbing.ZeroHash, err
	}
	if _, err := w.Write(content); err != nil {
		return plumbing.ZeroHash, err
	}
	if err := w.Close(); err != nil {
		return plumbing.ZeroHash, err
	}

	return s.SetEncodedObject(obj)
}

func storeTree(s *memory.Storage, entries []object.TreeEntry) (plumbing.Hash, error) {
	tree := &object.Tree{Entries: entries}
	obj := s.NewEncodedObject()
	if err := tree.Encode(obj); err != nil {
		return plumbing.ZeroHash, err
	}
	return s.SetEncodedObject(obj)
}

func storeCommit(s *memory.Storage, treeHash, parentHash plumbing.Hash, msg string) (plumbing.Hash, error) {
	now := time.Now()
	commit := &object.Commit{
		Author: object.Signature{
			Name:  "Test User",
			Email: "test@bleephub.local",
			When:  now,
		},
		Committer: object.Signature{
			Name:  "Test User",
			Email: "test@bleephub.local",
			When:  now,
		},
		Message:  msg,
		TreeHash: treeHash,
	}
	if parentHash != plumbing.ZeroHash {
		commit.ParentHashes = []plumbing.Hash{parentHash}
	}

	obj := s.NewEncodedObject()
	if err := commit.Encode(obj); err != nil {
		return plumbing.ZeroHash, err
	}
	return s.SetEncodedObject(obj)
}
