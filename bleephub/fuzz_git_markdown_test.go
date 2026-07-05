package bleephub

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

var fuzzGitRepoSeq int64

// FuzzGitRefUpdate fuzzes the PATCH .../git/refs/{ref...} body (sha/force)
// and the {ref...} path against a git-backed repo. A fresh repo per iteration
// keeps the run deterministic (a force-update mutates the ref target, so a
// shared repo would couple iterations). Invariant: never a 5xx or panic; a
// 200 response is a well-formed ref object with an object.sha.
func FuzzGitRefUpdate(f *testing.F) {
	s := fuzzRoutedServer(f)
	admin := s.store.UsersByLogin["admin"]

	// A representative real base SHA for the seeds; each iteration re-seeds.
	seedRepo := s.store.CreateRepo(admin, "ref-fuzz-seed", "", false)
	_ = seedRepo
	seedStor := s.store.GetGitStorage(admin.Login, "ref-fuzz-seed")
	seedBase, err := initRepoWithFiles(seedStor, "main", "init", map[string]string{"f": "x"}, repoSignature(admin.Login, "a@b.c"))
	if err != nil {
		f.Fatalf("seed init: %v", err)
	}
	realSHA := seedBase.String()

	f.Add("heads/main", realSHA, false)
	f.Add("heads/main", realSHA, true)
	f.Add("heads/main", "deadbeef", false)
	f.Add("heads/main", "", false)
	f.Add("heads/nonexistent", realSHA, false)
	f.Add("", realSHA, false)
	f.Add("heads/main", strings.Repeat("f", 200), true)
	f.Add("tags/../../etc", realSHA, false)
	f.Add("heads/main", "0000000000000000000000000000000000000000", true)

	f.Fuzz(func(t *testing.T, ref, sha string, force bool) {
		name := fmt.Sprintf("ref-fuzz-%d", atomic.AddInt64(&fuzzGitRepoSeq, 1))
		s.store.CreateRepo(admin, name, "", false)
		stor := s.store.GetGitStorage(admin.Login, name)
		if _, err := initRepoWithFiles(stor, "main", "init", map[string]string{"f": "x"}, repoSignature(admin.Login, "a@b.c")); err != nil {
			t.Fatalf("init: %v", err)
		}

		body, _ := json.Marshal(map[string]interface{}{"sha": sha, "force": force})
		req, err := http.NewRequest(http.MethodPatch, "http://x/api/v3/repos/admin/"+name+"/git/refs/"+ref, bytes.NewReader(body))
		if err != nil {
			return
		}
		req.Header.Set("Authorization", "token "+defaultToken)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)

		if w.Code >= 500 {
			t.Fatalf("ref update ref=%q sha=%q force=%v -> %d: %s", ref, sha, force, w.Code, w.Body.String())
		}
		if w.Code == http.StatusOK {
			var res map[string]interface{}
			if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
				t.Fatalf("200 ref body not JSON: %v", err)
			}
			obj, _ := res["object"].(map[string]interface{})
			if obj == nil || obj["sha"] == nil {
				t.Fatalf("200 ref update missing object.sha: %v", res)
			}
		}
	})
}

// FuzzContentPut fuzzes the PUT .../contents/{path...} body (content base64,
// branch, message) and the {path...} segment against a git-backed repo. A
// fresh repo per iteration keeps the run deterministic (a commit mutates the
// tree, so a shared repo would couple iterations). Invariant: never a 5xx or
// panic; a 201 carries a resolvable commit sha.
func FuzzContentPut(f *testing.F) {
	s := fuzzRoutedServer(f)
	admin := s.store.UsersByLogin["admin"]

	b64 := func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
	f.Add("docs/readme.md", b64("hello"), "main", "add readme")
	f.Add("nested/deep/file.txt", b64("x"), "main", "msg")
	f.Add("file", "not-base64!!", "main", "msg")
	f.Add("", b64("x"), "main", "msg")
	f.Add("dir/", b64("x"), "main", "msg")
	f.Add("../escape", b64("x"), "main", "msg")
	f.Add("a//b", b64("x"), "main", "msg")
	f.Add("file", b64("x"), "nonexistent-branch", "msg")
	f.Add("file", b64("x"), "main", "")
	f.Add("file", "", "main", "msg")
	f.Add(".git/config", b64("x"), "main", "msg")
	f.Add("/abs", b64("x"), "main", "msg")

	f.Fuzz(func(t *testing.T, path, content, branch, message string) {
		name := fmt.Sprintf("content-fuzz-%d", atomic.AddInt64(&fuzzGitRepoSeq, 1))
		s.store.CreateRepo(admin, name, "", false)
		stor := s.store.GetGitStorage(admin.Login, name)
		if _, err := initRepoWithFiles(stor, "main", "init", map[string]string{"seed": "x"}, repoSignature(admin.Login, "a@b.c")); err != nil {
			t.Fatalf("init: %v", err)
		}

		body, _ := json.Marshal(map[string]interface{}{
			"message": message, "content": content, "branch": branch,
		})
		req, err := http.NewRequest(http.MethodPut, "http://x/api/v3/repos/admin/"+name+"/contents/"+path, bytes.NewReader(body))
		if err != nil {
			return
		}
		req.Header.Set("Authorization", "token "+defaultToken)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)

		if w.Code >= 500 {
			t.Fatalf("content PUT path=%q branch=%q -> %d: %s", path, branch, w.Code, w.Body.String())
		}
		if w.Code == http.StatusCreated {
			var res map[string]interface{}
			if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
				t.Fatalf("201 content body not JSON: %v", err)
			}
			commit, _ := res["commit"].(map[string]interface{})
			if commit == nil || commit["sha"] == nil {
				t.Fatalf("201 content PUT missing commit.sha: %v", res)
			}
		}
	})
}

// FuzzMarkdownRender fuzzes POST /markdown with fuzzed text, mode, and context
// through the goldmark + html-pipeline reference-linkifier. Invariant: no
// panic/hang; a 200 body is well-formed HTML (parses as a fragment); an
// invalid mode is 422.
func FuzzMarkdownRender(f *testing.F) {
	s := fuzzRoutedServer(f)
	admin := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(admin, "md-fuzz", "", false)
	s.store.CreateIssue(repo.ID, admin.ID, "an issue", "b", nil, nil, 0)

	f.Add("# Hello\n\n@admin see #1", "gfm", "admin/md-fuzz")
	f.Add("plain **text** with `code`", "markdown", "")
	f.Add("| a | b |\n|---|---|\n| 1 | 2 |", "gfm", "admin/md-fuzz")
	f.Add("@admin @nobody #1 #999 #notanumber", "gfm", "admin/md-fuzz")
	f.Add("<script>alert(1)</script>", "gfm", "admin/md-fuzz")
	f.Add(strings.Repeat("#", 500)+" heading", "markdown", "")
	f.Add("[link](http://x) <a href=x>@admin</a>", "gfm", "admin/md-fuzz")
	f.Add("text", "invalid-mode", "")
	f.Add("", "gfm", "no-slash-context")
	f.Add("@a@b@c #1#2#3", "gfm", "admin/md-fuzz")
	f.Add("- [ ] task\n- [x] done", "gfm", "admin/md-fuzz")
	f.Add("\x00\x01\x02 control", "gfm", "admin/md-fuzz")

	f.Fuzz(func(t *testing.T, text, mode, context string) {
		body, _ := json.Marshal(map[string]interface{}{"text": text, "mode": mode, "context": context})
		w := fuzzServe(s, http.MethodPost, "/api/v3/markdown", body)

		if w.Code >= 500 {
			t.Fatalf("markdown text=%q mode=%q -> %d: %s", text, mode, w.Code, w.Body.String())
		}
		switch mode {
		case "", "markdown", "gfm":
			if w.Code == http.StatusOK {
				// The rendered HTML must parse as a fragment (well-formed enough
				// for xhtml, which is what the linkifier itself re-parses).
				if _, err := xhtml.ParseFragment(bytes.NewReader(w.Body.Bytes()), &xhtml.Node{Type: xhtml.ElementNode, Data: "body", DataAtom: atom.Body}); err != nil {
					t.Fatalf("200 markdown body not parseable HTML: %v", err)
				}
			}
		default:
			if w.Code != http.StatusUnprocessableEntity {
				t.Fatalf("invalid mode %q -> %d, want 422", mode, w.Code)
			}
		}
	})
}
