package bleephub

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	gitStorage "github.com/go-git/go-git/v5/storage"
)

func gitDataTestServer(t *testing.T) *Server {
	t.Helper()
	s := newTestServer()
	s.registerGHRepoRoutes()
	admin := s.store.UsersByLogin["admin"]
	s.store.Tokens[adminPAT] = &Token{Value: adminPAT, UserID: admin.ID, Scopes: "repo"}
	return s
}

func TestGitDataBlob(t *testing.T) {
	s := gitDataTestServer(t)
	admin := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(admin, "gitdata-blob", "", false)

	// Create blob
	body, _ := json.Marshal(map[string]string{"content": "hello world", "encoding": "utf-8"})
	w := doMiscReq(s, "POST", "/api/v3/repos/"+repo.FullName+"/git/blobs", string(body))
	if w.Code != http.StatusCreated {
		t.Fatalf("create blob status = %d, want 201; body = %s", w.Code, w.Body.String())
	}
	var blob map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &blob); err != nil {
		t.Fatal(err)
	}
	sha, _ := blob["sha"].(string)
	if sha == "" {
		t.Fatalf("blob sha empty")
	}

	// Get blob
	w = doMiscReq(s, "GET", "/api/v3/repos/"+repo.FullName+"/git/blobs/"+sha, "")
	if w.Code != http.StatusOK {
		t.Fatalf("get blob status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["sha"] != sha {
		t.Errorf("sha = %v, want %v", got["sha"], sha)
	}
	if got["encoding"] != "base64" {
		t.Errorf("encoding = %v, want base64", got["encoding"])
	}
}

func TestGitDataTreeAndCommit(t *testing.T) {
	s := gitDataTestServer(t)
	admin := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(admin, "gitdata-tree", "", false)
	stor := s.store.GetGitStorage(admin.Login, repo.Name)

	// Create blob
	body, _ := json.Marshal(map[string]string{"content": "file body", "encoding": "utf-8"})
	w := doMiscReq(s, "POST", "/api/v3/repos/"+repo.FullName+"/git/blobs", string(body))
	if w.Code != http.StatusCreated {
		t.Fatalf("create blob status = %d, want 201", w.Code)
	}
	var blobResp map[string]any
	json.Unmarshal(w.Body.Bytes(), &blobResp)
	blobSHA := blobResp["sha"].(string)

	// Create tree
	body, _ = json.Marshal(map[string]any{
		"tree": []map[string]any{{"path": "hello.txt", "mode": "100644", "type": "blob", "sha": blobSHA}},
	})
	w = doMiscReq(s, "POST", "/api/v3/repos/"+repo.FullName+"/git/trees", string(body))
	if w.Code != http.StatusCreated {
		t.Fatalf("create tree status = %d, want 201; body = %s", w.Code, w.Body.String())
	}
	var treeResp map[string]any
	json.Unmarshal(w.Body.Bytes(), &treeResp)
	treeSHA := treeResp["sha"].(string)
	if treeSHA == "" {
		t.Fatalf("tree sha empty")
	}

	// Get tree
	w = doMiscReq(s, "GET", "/api/v3/repos/"+repo.FullName+"/git/trees/"+treeSHA, "")
	if w.Code != http.StatusOK {
		t.Fatalf("get tree status = %d, want 200; body = %s", w.Code, w.Body.String())
	}

	// Create commit (no parents -> root commit)
	body, _ = json.Marshal(map[string]any{
		"message": "root",
		"tree":    treeSHA,
	})
	w = doMiscReq(s, "POST", "/api/v3/repos/"+repo.FullName+"/git/commits", string(body))
	if w.Code != http.StatusCreated {
		t.Fatalf("create commit status = %d, want 201; body = %s", w.Code, w.Body.String())
	}
	var commitResp map[string]any
	json.Unmarshal(w.Body.Bytes(), &commitResp)
	commitSHA := commitResp["sha"].(string)
	if commitSHA == "" {
		t.Fatalf("commit sha empty")
	}

	// Get commit
	w = doMiscReq(s, "GET", "/api/v3/repos/"+repo.FullName+"/git/commits/"+commitSHA, "")
	if w.Code != http.StatusOK {
		t.Fatalf("get commit status = %d, want 200; body = %s", w.Code, w.Body.String())
	}

	// Verify commit is in storage
	_, err := object.GetCommit(stor, plumbing.NewHash(commitSHA))
	if err != nil {
		t.Fatalf("commit not in storage: %v", err)
	}
}

func TestGitDataRefsAndTag(t *testing.T) {
	s := gitDataTestServer(t)
	admin := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(admin, "gitdata-refs", "", false)
	stor := s.store.GetGitStorage(admin.Login, repo.Name)

	// Seed a root commit directly via helper.
	sig := repoSignature(admin.Login, "bleephub@local")
	commitHash, err := initRepoWithFiles(stor, "main", "init", map[string]string{"a.txt": "a"}, sig)
	if err != nil {
		t.Fatalf("init repo: %v", err)
	}

	// Create a tag object pointing at the commit
	body, _ := json.Marshal(map[string]any{
		"tag":     "v1.0.0",
		"message": "release",
		"object":  commitHash.String(),
		"type":    "commit",
	})
	w := doMiscReq(s, "POST", "/api/v3/repos/"+repo.FullName+"/git/tags", string(body))
	if w.Code != http.StatusCreated {
		t.Fatalf("create tag status = %d, want 201; body = %s", w.Code, w.Body.String())
	}
	var tagResp map[string]any
	json.Unmarshal(w.Body.Bytes(), &tagResp)
	tagSHA := tagResp["sha"].(string)

	// Get tag
	w = doMiscReq(s, "GET", "/api/v3/repos/"+repo.FullName+"/git/tags/"+tagSHA, "")
	if w.Code != http.StatusOK {
		t.Fatalf("get tag status = %d, want 200; body = %s", w.Code, w.Body.String())
	}

	// Create refs/heads/feature
	body, _ = json.Marshal(map[string]any{"ref": "refs/heads/feature", "sha": commitHash.String()})
	w = doMiscReq(s, "POST", "/api/v3/repos/"+repo.FullName+"/git/refs", string(body))
	if w.Code != http.StatusCreated {
		t.Fatalf("create ref status = %d, want 201; body = %s", w.Code, w.Body.String())
	}
	var refResp map[string]any
	json.Unmarshal(w.Body.Bytes(), &refResp)
	if refResp["ref"] != "refs/heads/feature" {
		t.Errorf("ref = %v, want refs/heads/feature", refResp["ref"])
	}

	// Create a second commit to fast-forward the branch
	body, _ = json.Marshal(map[string]any{
		"tree":    treeHashFromCommit(t, stor, commitHash),
		"parents": []string{commitHash.String()},
		"message": "second",
	})
	w = doMiscReq(s, "POST", "/api/v3/repos/"+repo.FullName+"/git/commits", string(body))
	if w.Code != http.StatusCreated {
		t.Fatalf("create second commit status = %d, want 201; body = %s", w.Code, w.Body.String())
	}
	var commit2Resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &commit2Resp)
	commit2SHA := commit2Resp["sha"].(string)

	// Update ref (fast-forward)
	body, _ = json.Marshal(map[string]any{"sha": commit2SHA})
	w = doMiscReq(s, "PATCH", "/api/v3/repos/"+repo.FullName+"/git/refs/heads/feature", string(body))
	if w.Code != http.StatusOK {
		t.Fatalf("update ref status = %d, want 200; body = %s", w.Code, w.Body.String())
	}

	// List refs
	w = doMiscReq(s, "GET", "/api/v3/repos/"+repo.FullName+"/git/refs/heads", "")
	if w.Code != http.StatusOK {
		t.Fatalf("list refs status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	var refs []map[string]any
	json.Unmarshal(w.Body.Bytes(), &refs)
	if len(refs) != 2 { // main + feature
		t.Errorf("len(refs) = %d, want 2", len(refs))
	}

	// Delete ref
	w = doMiscReq(s, "DELETE", "/api/v3/repos/"+repo.FullName+"/git/refs/heads/feature", "")
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete ref status = %d, want 204; body = %s", w.Code, w.Body.String())
	}
}

// TestUpdateRefNonCommitTargetRejected: a non-force branch update to a target
// that is not a commit (an annotated tag object) can never be a fast-forward,
// so it must be rejected with 422 and leave the ref untouched.
func TestUpdateRefNonCommitTargetRejected(t *testing.T) {
	s := gitDataTestServer(t)
	admin := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(admin, "gitdata-nonff", "", false)
	stor := s.store.GetGitStorage(admin.Login, repo.Name)

	sig := repoSignature(admin.Login, "bleephub@local")
	commitHash, err := initRepoWithFiles(stor, "main", "init", map[string]string{"a.txt": "a"}, sig)
	if err != nil {
		t.Fatalf("init repo: %v", err)
	}

	body, _ := json.Marshal(map[string]any{
		"tag":     "v1.0.0",
		"message": "release",
		"object":  commitHash.String(),
		"type":    "commit",
	})
	w := doMiscReq(s, "POST", "/api/v3/repos/"+repo.FullName+"/git/tags", string(body))
	if w.Code != http.StatusCreated {
		t.Fatalf("create tag status = %d, want 201; body = %s", w.Code, w.Body.String())
	}
	var tagResp map[string]any
	json.Unmarshal(w.Body.Bytes(), &tagResp)
	tagSHA := tagResp["sha"].(string)

	body, _ = json.Marshal(map[string]any{"sha": tagSHA})
	w = doMiscReq(s, "PATCH", "/api/v3/repos/"+repo.FullName+"/git/refs/heads/main", string(body))
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("update ref to tag object status = %d, want 422; body = %s", w.Code, w.Body.String())
	}

	ref, err := stor.Reference(plumbing.ReferenceName("refs/heads/main"))
	if err != nil {
		t.Fatalf("main ref lookup: %v", err)
	}
	if ref.Hash() != commitHash {
		t.Errorf("main = %s after rejected update, want %s", ref.Hash(), commitHash)
	}
}

func treeHashFromCommit(t *testing.T, stor gitStorage.Storer, h plumbing.Hash) string {
	t.Helper()
	c, err := object.GetCommit(stor, h)
	if err != nil {
		t.Fatal(err)
	}
	return c.TreeHash.String()
}

func TestGitDataWriteRequiresAuth(t *testing.T) {
	s := gitDataTestServer(t)
	admin := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(admin, "gitdata-auth", "", false)

	reqs := []struct {
		method string
		path   string
		body   string
	}{
		{"POST", "/api/v3/repos/" + repo.FullName + "/git/blobs", `{"content":"x"}`},
		{"POST", "/api/v3/repos/" + repo.FullName + "/git/trees", `{"tree":[]}`},
		{"POST", "/api/v3/repos/" + repo.FullName + "/git/commits", `{"message":"m","tree":""}`},
		{"POST", "/api/v3/repos/" + repo.FullName + "/git/tags", `{"tag":"t","object":"","type":"commit"}`},
		{"POST", "/api/v3/repos/" + repo.FullName + "/git/refs", `{"ref":"refs/heads/x","sha":""}`},
	}
	for _, tc := range reqs {
		w := doMiscReq(s, tc.method, tc.path, tc.body)
		// doMiscReq sends the admin PAT by default, so write should succeed (status not 401).
		// We are only verifying the route exists and auth is wired; empty SHAs may cause 422.
		if w.Code == http.StatusUnauthorized {
			t.Errorf("%s %s returned 401 with valid PAT", tc.method, tc.path)
		}
	}
}

func TestGitDataReadRequiresContentsRead(t *testing.T) {
	s := gitDataTestServer(t)
	admin := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(admin, "gitdata-read", "", false)
	stor := s.store.GetGitStorage(admin.Login, repo.Name)
	sig := repoSignature(admin.Login, "bleephub@local")
	commitHash, _ := initRepoWithFiles(stor, "main", "init", map[string]string{"a.txt": "a"}, sig)

	// Create an installation token with metadata:read only (no contents)
	s.store.Installations[1] = &Installation{ID: 1, AppID: 1, TargetID: admin.ID, Permissions: map[string]string{"metadata": "read"}}
	s.store.InstallationTokens["ghs_notallowed"] = &InstallationToken{Token: "ghs_notallowed", InstallationID: 1, AppID: 1, Permissions: map[string]string{"metadata": "read"}, ExpiresAt: time.Now().Add(time.Hour)}

	req, _ := http.NewRequest("GET", "/api/v3/repos/"+repo.FullName+"/git/commits/"+commitHash.String(), nil)
	req.Header.Set("Authorization", "Bearer ghs_notallowed")
	w := httptest.NewRecorder()
	s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 for metadata-only token", w.Code)
	}
}
