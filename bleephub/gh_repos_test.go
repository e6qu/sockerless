package bleephub

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strconv"
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
		"query": `mutation{createRepository(input:{name:"gql-created",visibility:"PUBLIC",hasIssuesEnabled:false,hasWikiEnabled:true}){repository{name,owner{login},isPrivate,hasIssuesEnabled,hasWikiEnabled}}}`,
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
	if repo["hasIssuesEnabled"] != false {
		t.Fatalf("expected hasIssuesEnabled=false, got %v", repo["hasIssuesEnabled"])
	}
	if repo["hasWikiEnabled"] != true {
		t.Fatalf("expected hasWikiEnabled=true, got %v", repo["hasWikiEnabled"])
	}
}

func TestGraphQLCreateRepoAcceptsAuthenticatedOwnerID(t *testing.T) {
	admin := testServer.store.LookupUserByLogin("admin")
	if admin == nil {
		t.Fatal("default admin was not seeded")
	}
	resp := ghPost(t, "/api/graphql", defaultToken, map[string]interface{}{
		"query":     `mutation($input:CreateRepositoryInput!){createRepository(input:$input){repository{name,owner{login}}}}`,
		"variables": map[string]interface{}{"input": map[string]interface{}{"name": "gql-owner-id", "ownerId": admin.NodeID, "visibility": "PRIVATE"}},
	})
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)
	payload := data["data"].(map[string]interface{})["createRepository"].(map[string]interface{})
	repo := payload["repository"].(map[string]interface{})
	if repo["name"] != "gql-owner-id" || repo["owner"].(map[string]interface{})["login"] != admin.Login {
		t.Fatalf("ownerId repository = %#v", repo)
	}
}

func TestGraphQLCreateRepoAcceptsActiveOrganizationOwnerID(t *testing.T) {
	admin := testServer.store.LookupUserByLogin("admin")
	if admin == nil {
		t.Fatal("default admin was not seeded")
	}
	org := testServer.store.CreateOrg(admin, "gql-owner-org", "GraphQL owner organization", "")
	if org == nil {
		t.Fatal("create organization")
	}
	resp := ghPost(t, "/api/graphql", defaultToken, map[string]interface{}{
		"query":     `mutation($input:CreateRepositoryInput!){createRepository(input:$input){repository{name,owner{login}}}}`,
		"variables": map[string]interface{}{"input": map[string]interface{}{"name": "gql-org-owner-id", "ownerId": org.NodeID, "visibility": "PRIVATE"}},
	})
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)
	payload := data["data"].(map[string]interface{})["createRepository"].(map[string]interface{})
	repo := payload["repository"].(map[string]interface{})
	if repo["name"] != "gql-org-owner-id" || repo["owner"].(map[string]interface{})["login"] != org.Login {
		t.Fatalf("organization ownerId repository = %#v", repo)
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

func TestGitStorageInitFailure(t *testing.T) {
	tmpDir := t.TempDir()
	readOnlyDir := tmpDir + "/readonly"
	if err := os.Mkdir(readOnlyDir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BLEEPHUB_GIT_DIR", readOnlyDir)

	repo := testServer.store.CreateRepo(&User{Login: "admin", ID: 1}, "git-init-fail", "", false)
	if repo != nil {
		t.Fatal("expected nil repo when git storage init fails")
	}
}

func TestGitDeleteCleanup(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("BLEEPHUB_GIT_DIR", tmpDir)

	repo := testServer.store.CreateRepo(&User{Login: "admin", ID: 1}, "git-cleanup", "", false)
	if repo == nil {
		t.Fatal("expected repo to be created")
	}

	repoDir := tmpDir + "/admin/git-cleanup"
	if _, err := os.Stat(repoDir); err != nil {
		t.Fatalf("expected git dir to exist: %v", err)
	}

	deleted, err := testServer.store.DeleteRepo("admin", "git-cleanup")
	if err != nil {
		t.Fatalf("DeleteRepo: %v", err)
	}
	if !deleted {
		t.Fatal("expected repo to be deleted")
	}

	if _, err := os.Stat(repoDir); !os.IsNotExist(err) {
		t.Fatalf("expected git dir to be removed after delete: %v", err)
	}
}

func TestDeleteRepoS3GitCleanupFailurePreservesRepo(t *testing.T) {
	resetS3FSCacheForTest(t)
	t.Setenv("BLEEPHUB_GIT_DIR", "")
	t.Setenv("BLEEPHUB_S3_BUCKET", "")

	st := NewStore()
	st.SeedDefaultUser()
	admin := st.UsersByLogin["admin"]
	repo := st.CreateRepo(admin, "s3-cleanup-fail", "", false)
	if repo == nil {
		t.Fatal("expected repo to be created before S3 git storage is enabled")
	}

	resetS3FSCacheForTest(t)
	t.Setenv("BLEEPHUB_S3_BUCKET", "bucket")
	t.Setenv("BLEEPHUB_S3_ENDPOINT", "http://127.0.0.1:1")

	deleted, err := st.DeleteRepo(admin.Login, repo.Name)
	if err == nil {
		t.Fatal("DeleteRepo returned nil error, want S3 git cleanup failure")
	}
	if !deleted {
		t.Fatal("DeleteRepo returned false for an existing repo")
	}
	if st.ReposByName[repo.FullName] == nil {
		t.Fatal("repo metadata was deleted even though required S3 git cleanup failed")
	}
	if st.GitStorages[repo.FullName] == nil {
		t.Fatal("git storage index was deleted even though required S3 git cleanup failed")
	}
}

func TestGitFetchNoAuthPublicRepo(t *testing.T) {
	ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name":    "public-clone",
		"private": false,
	})

	resp, err := http.Get(testBaseURL + "/admin/public-clone.git/info/refs?service=git-upload-pack")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 for unauthenticated fetch on public repo, got %d", resp.StatusCode)
	}
}

func TestGitFetchNoAuthPrivateRepo(t *testing.T) {
	ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name":    "private-clone",
		"private": true,
	})

	resp, err := http.Get(testBaseURL + "/admin/private-clone.git/info/refs?service=git-upload-pack")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("expected 401 for unauthenticated fetch on private repo, got %d", resp.StatusCode)
	}
}

func TestGitPushNoAuth(t *testing.T) {
	ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name": "no-auth-push",
	})

	resp, err := http.Get(testBaseURL + "/admin/no-auth-push.git/info/refs?service=git-receive-pack")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("expected 401 for unauthenticated push, got %d", resp.StatusCode)
	}
}

func TestGitPushWithAuth(t *testing.T) {
	repo := testServer.store.CreateRepo(&User{Login: "admin", ID: 1}, "auth-push", "", false)
	if repo == nil {
		t.Fatal("expected repo to be created")
	}

	cloneStorage := memory.NewStorage()
	gitRepo, err := git.Init(cloneStorage, nil)
	if err != nil {
		t.Fatal(err)
	}

	blobHash, _ := storeBlob(cloneStorage, []byte("auth test\n"))
	treeHash, _ := storeTree(cloneStorage, []object.TreeEntry{
		{Name: "test.txt", Mode: 0100644, Hash: blobHash},
	})
	commitHash, _ := storeCommit(cloneStorage, treeHash, plumbing.ZeroHash, "auth commit")
	cloneStorage.SetReference(plumbing.NewHashReference(plumbing.NewBranchReferenceName("main"), commitHash))
	cloneStorage.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.NewBranchReferenceName("main")))

	_, err = gitRepo.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{testBaseURL + "/admin/auth-push.git"},
	})
	if err != nil {
		t.Fatal(err)
	}

	err = gitRepo.Push(&git.PushOptions{
		RemoteName: "origin",
		Auth:       &githttp.BasicAuth{Username: "x", Password: defaultToken},
	})
	if err != nil {
		t.Fatalf("expected push to succeed with auth, got: %v", err)
	}
}

func TestGitFetchPrivateRepoWithAuth(t *testing.T) {
	repo := testServer.store.CreateRepo(&User{Login: "admin", ID: 1}, "private-auth-fetch", "", true)
	if repo == nil {
		t.Fatal("expected repo to be created")
	}

	resp, err := http.NewRequest("GET", testBaseURL+"/admin/private-auth-fetch.git/info/refs?service=git-upload-pack", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Header.Set("Authorization", "token "+defaultToken)
	httpResp, err := http.DefaultClient.Do(resp)
	if err != nil {
		t.Fatal(err)
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode != 200 {
		t.Fatalf("expected 200 for authenticated fetch on private repo, got %d", httpResp.StatusCode)
	}
}

// TestCreateRepoAutoInit verifies auto_init creates a README commit.
func TestCreateRepoAutoInit(t *testing.T) {
	resp := ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name":        "auto-init",
		"description": "A self-initialized repo",
		"auto_init":   true,
	})
	if resp.StatusCode != 201 {
		resp.Body.Close()
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)
	if data["default_branch"] != "main" {
		t.Fatalf("expected default_branch=main, got %v", data["default_branch"])
	}

	// README should be reachable
	readmeResp := ghGet(t, "/api/v3/repos/admin/auto-init/readme", defaultToken)
	if readmeResp.StatusCode != 200 {
		readmeResp.Body.Close()
		t.Fatalf("expected 200 for readme, got %d", readmeResp.StatusCode)
	}
	readme := decodeJSON(t, readmeResp)
	if readme["name"] != "README.md" {
		t.Fatalf("expected readme name README.md, got %v", readme["name"])
	}
	content, _ := base64.StdEncoding.DecodeString(readme["content"].(string))
	if !strings.Contains(string(content), "# auto-init") {
		t.Fatalf("unexpected readme content: %s", string(content))
	}
	if !strings.Contains(string(content), "A self-initialized repo") {
		t.Fatalf("expected description in readme, got: %s", string(content))
	}

	// Branch should exist now
	branchesResp := ghGet(t, "/api/v3/repos/admin/auto-init/branches", defaultToken)
	defer branchesResp.Body.Close()
	if branchesResp.StatusCode != 200 {
		t.Fatalf("expected 200 for branches, got %d", branchesResp.StatusCode)
	}
	var branches []map[string]interface{}
	if err := json.NewDecoder(branchesResp.Body).Decode(&branches); err != nil {
		t.Fatal(err)
	}
	if len(branches) != 1 {
		t.Fatalf("expected 1 branch, got %d", len(branches))
	}
}

// TestCreateRepoWithTemplates verifies gitignore and license templates are committed.
func TestCreateRepoWithTemplates(t *testing.T) {
	resp := ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name":               "templated",
		"gitignore_template": "Go",
		"license_template":   "mit",
	})
	if resp.StatusCode != 201 {
		resp.Body.Close()
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	defer resp.Body.Close()

	for _, path := range []string{".gitignore", "LICENSE"} {
		contentResp := ghGet(t, "/api/v3/repos/admin/templated/contents/"+path, defaultToken)
		if contentResp.StatusCode != 200 {
			contentResp.Body.Close()
			t.Fatalf("expected 200 for %s, got %d", path, contentResp.StatusCode)
		}
		contentResp.Body.Close()
	}
}

// TestCreateRepoVisibility verifies visibility field overrides private.
func TestCreateRepoVisibility(t *testing.T) {
	resp := ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name":       "vis-public",
		"private":    true,
		"visibility": "public",
	})
	if resp.StatusCode != 201 {
		resp.Body.Close()
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)
	if data["private"] != false {
		t.Fatalf("expected private=false, got %v", data["private"])
	}
	if data["visibility"] != "public" {
		t.Fatalf("expected visibility=public, got %v", data["visibility"])
	}

	resp2 := ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name":       "vis-private",
		"visibility": "private",
	})
	if resp2.StatusCode != 201 {
		resp2.Body.Close()
		t.Fatalf("expected 201, got %d", resp2.StatusCode)
	}
	data2 := decodeJSON(t, resp2)
	if data2["private"] != true {
		t.Fatalf("expected private=true, got %v", data2["private"])
	}
}

// TestCreateRepoDefaultBranch verifies a non-main default branch is honored.
func TestCreateRepoDefaultBranch(t *testing.T) {
	resp := ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name":           "custom-branch",
		"auto_init":      true,
		"default_branch": "develop",
	})
	if resp.StatusCode != 201 {
		resp.Body.Close()
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)
	if data["default_branch"] != "develop" {
		t.Fatalf("expected default_branch=develop, got %v", data["default_branch"])
	}

	branchesResp := ghGet(t, "/api/v3/repos/admin/custom-branch/branches", defaultToken)
	defer branchesResp.Body.Close()
	var branches []map[string]interface{}
	if err := json.NewDecoder(branchesResp.Body).Decode(&branches); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, b := range branches {
		if b["name"] == "develop" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected develop branch to exist, got %v", branches)
	}
}

// TestCreateOrgRepoExtended verifies org repo creation supports the new fields.
func TestCreateOrgRepoExtended(t *testing.T) {
	createOrgViaAdminAPI(t, "create-org", "Create Org")

	resp := ghPost(t, "/api/v3/orgs/create-org/repos", defaultToken, map[string]interface{}{
		"name":           "org-repo",
		"auto_init":      true,
		"private":        true,
		"default_branch": "trunk",
	})
	if resp.StatusCode != 201 {
		resp.Body.Close()
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)
	if data["full_name"] != "create-org/org-repo" {
		t.Fatalf("expected full_name=create-org/org-repo, got %v", data["full_name"])
	}
	owner, _ := data["owner"].(map[string]interface{})
	if owner["login"] != "create-org" {
		t.Fatalf("expected owner.login=create-org, got %v", owner["login"])
	}
	if owner["type"] != "Organization" {
		t.Fatalf("expected owner.type=Organization, got %v", owner["type"])
	}
	if data["default_branch"] != "trunk" {
		t.Fatalf("expected default_branch=trunk, got %v", data["default_branch"])
	}

	readmeResp := ghGet(t, "/api/v3/repos/create-org/org-repo/readme", defaultToken)
	defer readmeResp.Body.Close()
	if readmeResp.StatusCode != 200 {
		t.Fatalf("expected 200 for readme, got %d", readmeResp.StatusCode)
	}
}

// TestPutContentsCreateFile verifies PUT contents creates a new file and commit.
func TestPutContentsCreateFile(t *testing.T) {
	ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name": "put-create",
	})

	encoded := base64.StdEncoding.EncodeToString([]byte("hello world"))
	resp := ghPut(t, "/api/v3/repos/admin/put-create/contents/foo/bar.txt", defaultToken, map[string]interface{}{
		"message": "add file",
		"content": encoded,
		"branch":  "main",
	})
	if resp.StatusCode != 201 {
		resp.Body.Close()
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)
	content, _ := data["content"].(map[string]interface{})
	if content["name"] != "bar.txt" {
		t.Fatalf("expected name=bar.txt, got %v", content["name"])
	}
	commit, _ := data["commit"].(map[string]interface{})
	if commit["message"] != "add file" {
		t.Fatalf("expected commit message 'add file', got %v", commit["message"])
	}

	// Verify file is readable
	getResp := ghGet(t, "/api/v3/repos/admin/put-create/contents/foo/bar.txt", defaultToken)
	defer getResp.Body.Close()
	if getResp.StatusCode != 200 {
		t.Fatalf("expected 200 for get, got %d", getResp.StatusCode)
	}
}

// TestPutContentsUpdateFile verifies PUT contents updates an existing file.
func TestPutContentsUpdateFile(t *testing.T) {
	ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name":      "put-update",
		"auto_init": true,
	})

	encoded := base64.StdEncoding.EncodeToString([]byte("first"))
	resp1 := ghPut(t, "/api/v3/repos/admin/put-update/contents/x.txt", defaultToken, map[string]interface{}{
		"message": "first",
		"content": encoded,
	})
	if resp1.StatusCode != 201 {
		resp1.Body.Close()
		t.Fatalf("expected 201, got %d", resp1.StatusCode)
	}
	data1 := decodeJSON(t, resp1)
	content1, _ := data1["content"].(map[string]interface{})
	sha1 := content1["sha"].(string)

	encoded2 := base64.StdEncoding.EncodeToString([]byte("second"))
	resp2 := ghPut(t, "/api/v3/repos/admin/put-update/contents/x.txt", defaultToken, map[string]interface{}{
		"message": "second",
		"content": encoded2,
		"sha":     sha1,
	})
	if resp2.StatusCode != 201 {
		resp2.Body.Close()
		t.Fatalf("expected 201, got %d", resp2.StatusCode)
	}
	data2 := decodeJSON(t, resp2)
	content2, _ := data2["content"].(map[string]interface{})
	if content2["sha"] == sha1 {
		t.Fatal("expected sha to change after update")
	}
}

// TestGetContentsRootListing verifies GET contents with an empty path lists root files.
func TestGetContentsRootListing(t *testing.T) {
	ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name":      "root-listing",
		"auto_init": true,
	})

	resp := ghGet(t, "/api/v3/repos/admin/root-listing/contents/", defaultToken)
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var items []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range items {
		if item["name"] == "README.md" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected README.md in root listing, got %v", items)
	}
}

// TestGitignoreTemplates verifies the gitignore template endpoints.
func TestGitignoreTemplates(t *testing.T) {
	listResp := ghGet(t, "/api/v3/gitignore/templates", defaultToken)
	if listResp.StatusCode != 200 {
		listResp.Body.Close()
		t.Fatalf("expected 200, got %d", listResp.StatusCode)
	}
	var names []string
	if err := json.NewDecoder(listResp.Body).Decode(&names); err != nil {
		t.Fatal(err)
	}
	listResp.Body.Close()
	if len(names) == 0 {
		t.Fatal("expected at least one gitignore template")
	}

	getResp := ghGet(t, "/api/v3/gitignore/templates/Go", defaultToken)
	if getResp.StatusCode != 200 {
		getResp.Body.Close()
		t.Fatalf("expected 200, got %d", getResp.StatusCode)
	}
	data := decodeJSON(t, getResp)
	if data["name"] != "Go" {
		t.Fatalf("expected name=Go, got %v", data["name"])
	}

	missingResp := ghGet(t, "/api/v3/gitignore/templates/NonExistent", defaultToken)
	defer missingResp.Body.Close()
	if missingResp.StatusCode != 404 {
		t.Fatalf("expected 404, got %d", missingResp.StatusCode)
	}
}

// TestLicenseTemplates verifies the license template endpoints.
func TestLicenseTemplates(t *testing.T) {
	listResp := ghGet(t, "/api/v3/licenses", defaultToken)
	if listResp.StatusCode != 200 {
		listResp.Body.Close()
		t.Fatalf("expected 200, got %d", listResp.StatusCode)
	}
	var items []map[string]interface{}
	if err := json.NewDecoder(listResp.Body).Decode(&items); err != nil {
		t.Fatal(err)
	}
	listResp.Body.Close()
	if len(items) == 0 {
		t.Fatal("expected at least one license")
	}

	getResp := ghGet(t, "/api/v3/licenses/mit", defaultToken)
	if getResp.StatusCode != 200 {
		getResp.Body.Close()
		t.Fatalf("expected 200, got %d", getResp.StatusCode)
	}
	data := decodeJSON(t, getResp)
	if data["key"] != "mit" {
		t.Fatalf("expected key=mit, got %v", data["key"])
	}

	missingResp := ghGet(t, "/api/v3/licenses/notreal", defaultToken)
	defer missingResp.Body.Close()
	if missingResp.StatusCode != 404 {
		t.Fatalf("expected 404, got %d", missingResp.StatusCode)
	}
}

// TestRepoDeployKeys exercises deploy key CRUD.
func TestRepoDeployKeys(t *testing.T) {
	repoName := "deploy-keys-repo"
	ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{"name": repoName})

	resp := ghPost(t, "/api/v3/repos/admin/"+repoName+"/keys", defaultToken, map[string]interface{}{
		"title":     "laptop",
		"key":       "ssh-rsa AAAA test",
		"read_only": true,
	})
	if resp.StatusCode != 201 {
		resp.Body.Close()
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	created := decodeJSON(t, resp)
	keyID := int(created["id"].(float64))
	if created["title"] != "laptop" || created["read_only"] != true {
		t.Fatalf("unexpected created key: %v", created)
	}

	listResp := ghGet(t, "/api/v3/repos/admin/"+repoName+"/keys", defaultToken)
	if listResp.StatusCode != 200 {
		listResp.Body.Close()
		t.Fatalf("expected 200, got %d", listResp.StatusCode)
	}
	var keys []map[string]interface{}
	if err := json.NewDecoder(listResp.Body).Decode(&keys); err != nil {
		t.Fatal(err)
	}
	listResp.Body.Close()
	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}

	getResp := ghGet(t, "/api/v3/repos/admin/"+repoName+"/keys/"+strconv.Itoa(keyID), defaultToken)
	if getResp.StatusCode != 200 {
		getResp.Body.Close()
		t.Fatalf("expected 200, got %d", getResp.StatusCode)
	}
	got := decodeJSON(t, getResp)
	if got["id"] != float64(keyID) {
		t.Fatalf("expected id %d, got %v", keyID, got["id"])
	}

	delResp := ghDelete(t, "/api/v3/repos/admin/"+repoName+"/keys/"+strconv.Itoa(keyID), defaultToken)
	defer delResp.Body.Close()
	if delResp.StatusCode != 204 {
		t.Fatalf("expected 204, got %d", delResp.StatusCode)
	}

	getAfter := ghGet(t, "/api/v3/repos/admin/"+repoName+"/keys/"+strconv.Itoa(keyID), defaultToken)
	defer getAfter.Body.Close()
	if getAfter.StatusCode != 404 {
		t.Fatalf("expected 404 after delete, got %d", getAfter.StatusCode)
	}
}

// TestRepoTransfer verifies transferring a repo to another user.
func TestRepoTransfer(t *testing.T) {
	repoName := "transfer-repo"
	ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{"name": repoName})

	userResp, err := authedPost("/internal/users", "application/json", bytes.NewReader([]byte(`{"login":"alice","email":"alice@local"}`)))
	if err != nil {
		t.Fatal(err)
	}
	if userResp.StatusCode != 201 {
		userResp.Body.Close()
		t.Fatalf("expected 201 for user create, got %d", userResp.StatusCode)
	}
	userResp.Body.Close()

	resp := ghPost(t, "/api/v3/repos/admin/"+repoName+"/transfer", defaultToken, map[string]interface{}{
		"new_owner": "alice",
	})
	if resp.StatusCode != 202 {
		resp.Body.Close()
		t.Fatalf("expected 202, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)
	if data["full_name"] != "alice/"+repoName {
		t.Fatalf("expected full_name=alice/%s, got %v", repoName, data["full_name"])
	}

	getOld := ghGet(t, "/api/v3/repos/admin/"+repoName, defaultToken)
	defer getOld.Body.Close()
	if getOld.StatusCode != 404 {
		t.Fatalf("expected 404 for old owner, got %d", getOld.StatusCode)
	}

	getNew := ghGet(t, "/api/v3/repos/alice/"+repoName, defaultToken)
	if getNew.StatusCode != 200 {
		getNew.Body.Close()
		t.Fatalf("expected 200 for new owner, got %d", getNew.StatusCode)
	}
}

// TestRepoRenameBranch verifies branch rename.
func TestRepoRenameBranch(t *testing.T) {
	repoName := "rename-branch-repo"
	ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name":      repoName,
		"auto_init": true,
	})

	resp := ghPost(t, "/api/v3/repos/admin/"+repoName+"/branches/main/rename", defaultToken, map[string]interface{}{
		"new_name": "trunk",
	})
	if resp.StatusCode != 201 {
		resp.Body.Close()
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)
	if data["name"] != "trunk" {
		t.Fatalf("expected name=trunk, got %v", data["name"])
	}
	commit, _ := data["commit"].(map[string]interface{})
	if commit["sha"] == nil {
		t.Fatal("missing commit.sha")
	}

	branchesResp := ghGet(t, "/api/v3/repos/admin/"+repoName+"/branches", defaultToken)
	if branchesResp.StatusCode != 200 {
		branchesResp.Body.Close()
		t.Fatalf("expected 200, got %d", branchesResp.StatusCode)
	}
	var branches []map[string]interface{}
	if err := json.NewDecoder(branchesResp.Body).Decode(&branches); err != nil {
		t.Fatal(err)
	}
	branchesResp.Body.Close()
	found := false
	for _, b := range branches {
		if b["name"] == "trunk" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected trunk branch, got %v", branches)
	}
}

// TestRepoSubscription verifies subscribe/unsubscribe endpoints.
func TestRepoSubscription(t *testing.T) {
	repoName := "subscription-repo"
	ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{"name": repoName})

	resp := ghPut(t, "/api/v3/repos/admin/"+repoName+"/subscription", defaultToken, map[string]interface{}{
		"subscribed": true,
	})
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)
	if data["subscribed"] != true {
		t.Fatalf("expected subscribed=true, got %v", data["subscribed"])
	}
	if data["repository_url"] == nil {
		t.Fatal("missing repository_url")
	}

	delResp := ghDelete(t, "/api/v3/repos/admin/"+repoName+"/subscription", defaultToken)
	defer delResp.Body.Close()
	if delResp.StatusCode != 204 {
		t.Fatalf("expected 204, got %d", delResp.StatusCode)
	}
}

// TestRepoVulnerabilityAlerts verifies enabling/disabling vulnerability alerts.
func TestRepoVulnerabilityAlerts(t *testing.T) {
	repoName := "vuln-alerts-repo"
	ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{"name": repoName})

	putResp := ghPut(t, "/api/v3/repos/admin/"+repoName+"/vulnerability-alerts", defaultToken, nil)
	if putResp.StatusCode != 204 {
		putResp.Body.Close()
		t.Fatalf("expected 204, got %d", putResp.StatusCode)
	}
	putResp.Body.Close()
	repo := testServer.store.GetRepo("admin", repoName)
	if repo == nil || !repo.VulnerabilityAlertsEnabled {
		t.Fatal("expected vulnerability alerts enabled")
	}

	delResp := ghDelete(t, "/api/v3/repos/admin/"+repoName+"/vulnerability-alerts", defaultToken)
	defer delResp.Body.Close()
	if delResp.StatusCode != 204 {
		t.Fatalf("expected 204, got %d", delResp.StatusCode)
	}
	repo = testServer.store.GetRepo("admin", repoName)
	if repo == nil || repo.VulnerabilityAlertsEnabled {
		t.Fatal("expected vulnerability alerts disabled")
	}
}
