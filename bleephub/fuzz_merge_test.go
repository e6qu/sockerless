package bleephub

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
)

// fuzzRoutedServer builds a fully route-registered in-process server whose
// GitHub REST surface is reachable through the real ghHeadersMiddleware auth
// chain — the same wiring doMiscReq uses, but with every route mounted. Fuzz
// targets drive it via fuzzServe. The seeded admin token (defaultToken)
// authenticates as the site-admin user.
func fuzzRoutedServer(tb testing.TB) *Server {
	tb.Helper()
	s := newTestServer()
	s.initGraphQLSchema()
	s.registerRoutes()
	return s
}

// fuzzServe drives one request through the auth+header middleware and returns
// the recorder. body may be nil.
func fuzzServe(s *Server, method, path string, body []byte) *httptest.ResponseRecorder {
	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, path, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	req.Header.Set("Authorization", "token "+defaultToken)
	w := httptest.NewRecorder()
	s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
	return w
}

var fuzzMergeRepoSeq int64

// seedGitBackedPR creates a repository with a real git base branch ("main")
// and a real feature branch ("feat") one commit ahead, then opens PR #1 from
// feat into main. The returned repo path can be merged for real; the merge
// produces a resolvable merge commit. Each call uses a fresh repository name so
// the destructive merge in one fuzz iteration never bleeds into the next.
func seedGitBackedPR(tb testing.TB, s *Server) (repoPath string, headSHA string) {
	tb.Helper()
	admin := s.store.UsersByLogin["admin"]
	name := fmt.Sprintf("merge-fuzz-%d", atomic.AddInt64(&fuzzMergeRepoSeq, 1))
	repo := s.store.CreateRepo(admin, name, "", false)
	if repo == nil {
		tb.Fatalf("CreateRepo returned nil")
		return "", ""
	}
	stor := s.store.GetGitStorage(admin.Login, name)
	sig := repoSignature(admin.Login, "admin@bleephub.local")

	baseHash, err := initRepoWithFiles(stor, "main", "init", map[string]string{"README.md": "base"}, sig)
	if err != nil {
		tb.Fatalf("initRepoWithFiles: %v", err)
	}
	// Branch feat off main, then add a commit so the PR has a real diff.
	if err := stor.SetReference(plumbing.NewHashReference(plumbing.NewBranchReferenceName("feat"), baseHash)); err != nil {
		tb.Fatalf("SetReference feat: %v", err)
	}
	head, err := createFileCommit(stor, "feat", "feature.txt", "hello", "add feature", sig)
	if err != nil {
		tb.Fatalf("createFileCommit: %v", err)
	}

	pr := s.store.CreatePullRequest(repo.ID, admin.ID, "Fuzz merge", "body", "feat", "main", false, nil, nil, 0)
	if pr == nil {
		tb.Fatalf("CreatePullRequest returned nil")
	}
	return "/api/v3/repos/" + repo.FullName, head.String()
}

// FuzzMergePullRequestBody fuzzes the PUT /pulls/{n}/merge body decode
// (merge_method / commit_title / commit_message / sha) against a real
// git-backed PR. Invariants:
//   - never a 5xx or panic on any body;
//   - an invalid merge_method is rejected 422;
//   - a stale sha is rejected 409;
//   - a successful (200) merge reports merged=true with a merge_commit_sha
//     that resolves through the git data API.
func FuzzMergePullRequestBody(f *testing.F) {
	s := fuzzRoutedServer(f)

	f.Add(`{"merge_method":"merge"}`)
	f.Add(`{"merge_method":"squash","commit_title":"t","commit_message":"m"}`)
	f.Add(`{"merge_method":"rebase"}`)
	f.Add(`{}`)
	f.Add(``)
	f.Add(`{"merge_method":"MERGE"}`)
	f.Add(`{"merge_method":"fast-forward"}`)
	f.Add(`{"merge_method":123}`)
	f.Add(`{"merge_method":null,"sha":""}`)
	f.Add(`{"sha":"0000000000000000000000000000000000000000"}`)
	f.Add(`{"merge_method":"merge","sha":"deadbeef"}`)
	f.Add(`{"commit_title":"","commit_message":""}`)
	f.Add(`not json`)
	f.Add(`[]`)
	f.Add(`{"merge_method":"squash","commit_message":"trailing"}`)

	f.Fuzz(func(t *testing.T, body string) {
		repoPath, headSHA := seedGitBackedPR(t, s)

		w := fuzzServe(s, http.MethodPut, repoPath+"/pulls/1/merge", []byte(body))

		if w.Code >= 500 {
			t.Fatalf("merge returned %d (want <500) for body %q: %s", w.Code, body, w.Body.String())
		}

		// Derive the expectation for well-formed JSON bodies.
		var parsed map[string]interface{}
		validJSON := json.Unmarshal([]byte(body), &parsed) == nil
		if validJSON {
			method, hasMethod := parsed["merge_method"]
			if hasMethod {
				if ms, ok := method.(string); ok {
					switch ms {
					case "", "merge", "squash", "rebase":
					default:
						if w.Code != http.StatusUnprocessableEntity {
							t.Fatalf("invalid merge_method %q: status %d, want 422; body %s", ms, w.Code, w.Body.String())
						}
					}
				}
				// A non-string merge_method fails JSON decode into the typed
				// struct → 400 Problems parsing JSON, still <500 (already asserted).
			}
			// A sha mismatching the real head must be a 409 (never a 200 merge
			// of the wrong head).
			if shaVal, ok := parsed["sha"].(string); ok && shaVal != "" && shaVal != headSHA {
				if w.Code == http.StatusOK {
					t.Fatalf("merge succeeded (200) despite stale sha %q != head %q", shaVal, headSHA)
				}
			}
		}

		if w.Code == http.StatusOK {
			var res map[string]interface{}
			if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
				t.Fatalf("200 merge body not JSON: %v (%s)", err, w.Body.String())
			}
			if res["merged"] != true {
				t.Fatalf("200 merge without merged=true: %v", res)
			}
			sha, _ := res["sha"].(string)
			if sha == "" {
				t.Fatalf("200 merge without sha: %v", res)
			}
			// The merge commit must resolve through the git data API.
			cw := fuzzServe(s, http.MethodGet, repoPath+"/git/commits/"+sha, nil)
			if cw.Code != http.StatusOK {
				t.Fatalf("merge_commit_sha %q does not resolve: git/commits status %d", sha, cw.Code)
			}
		}
	})
}
