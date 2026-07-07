package bleephub

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-billy/v5/memfs"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// fuzzFixture is a fully wired bleephub server plus the identifiers of a small,
// realistic seed (an org, a repo with git content, an issue, a PR, a team, and a
// scoped token). The HTTP fuzzers build requests against fixture.handler — the
// same wrapped middleware chain server.go assembles in ListenAndServe — so a
// fuzzed byte string exercises the production request path end to end.
type fuzzFixture struct {
	s           *Server
	handler     http.Handler
	adminToken  string
	scopedToken string
	// vocabulary of real identifiers the decoder can splice into a path template
	// so mutations frequently land on existing resources.
	segVocab []string
}

// newFuzzFixture builds and seeds the fuzz server exactly once per fuzz target.
func newFuzzFixture(tb testing.TB) *fuzzFixture {
	tb.Helper()
	s := newTestServer()
	s.registerRoutes()

	admin := s.store.UsersByLogin["admin"]
	if admin == nil {
		tb.Fatalf("seed: no admin user")
		return nil
	}

	// Org + team.
	if s.store.CreateOrg(admin, "fuzz-org", "Fuzz Org", "") == nil {
		tb.Fatalf("seed: create org")
		return nil
	}
	team := s.store.CreateTeam("fuzz-org", "Fuzz Team", TeamOptions{Description: "t"})
	if team == nil {
		tb.Fatalf("seed: create team")
		return nil
	}

	// Repo + a couple of committed files so contents/git-data handlers see real
	// objects, not an empty repository.
	repo := s.store.CreateRepo(admin, "fuzz-repo", "Fuzz repo", false)
	if repo == nil {
		tb.Fatalf("seed: create repo")
		return nil
	}
	seedGitContent(tb, s, "admin/fuzz-repo", map[string]string{
		"README.md":         "# fuzz\n",
		".github/wf/ci.yml": "name: ci\non: push\n",
	})
	seedPullRequestBranches(tb, s, repo, "feature")

	// Issue + PR.
	issue := s.store.CreateIssue(repo.ID, admin.ID, "Fuzz issue", "body", nil, nil, 0)
	if issue == nil {
		tb.Fatalf("seed: create issue")
		return nil
	}
	pr := s.store.CreatePullRequest(repo.ID, admin.ID, "Fuzz PR", "body", "feature", "main", false, nil, nil, 0)
	if pr == nil {
		tb.Fatalf("seed: create pull request")
		return nil
	}

	// A scoped (non-admin) token so the fuzzer can drive authz-sensitive paths
	// with a credential that is valid but limited.
	scoped := "fuzz-scoped-token-000000000000000000000000"
	s.store.mu.Lock()
	s.store.Tokens[scoped] = &Token{Value: scoped, UserID: admin.ID, Scopes: "public_repo", CreatedAt: time.Now().UTC()}
	s.store.mu.Unlock()

	handler := s.ghHeadersMiddleware(s.prefixStripMiddleware(s.internalAuthMiddleware(s.mux)))

	return &fuzzFixture{
		s:           s,
		handler:     handler,
		adminToken:  AdminToken(),
		scopedToken: scoped,
		segVocab: []string{
			"admin", "fuzz-org", "fuzz-repo", "fuzz-team",
			strconv.Itoa(issue.Number), strconv.Itoa(pr.Number),
			strconv.Itoa(repo.ID), strconv.Itoa(admin.ID),
			"main", "refs/heads/main", "heads/main", "feature",
			"1", "0", "2", "v4", "README.md", ".github/wf/ci.yml",
			"missing", "does-not-exist",
			// adversarial segment values
			"", "..", "../..", "%2e%2e", "..%2f..", "0x10", "-1",
			"99999999999999999999999999", "3.14", "null", "true",
			"a b", "a/b/c", "🔥", strings.Repeat("A", 4096),
			"'", "\"", "<script>", "%00", "\t",
		},
	}
}

// seedGitContent commits files into a repo's git storage in one commit at HEAD.
func seedGitContent(tb testing.TB, s *Server, repoFullName string, files map[string]string) {
	tb.Helper()
	parts := strings.SplitN(repoFullName, "/", 2)
	storer := s.store.GetGitStorage(parts[0], parts[1])
	if storer == nil {
		tb.Fatalf("seed git: no storage for %s", repoFullName)
	}
	fs := memfs.New()
	repo, err := git.Init(storer, fs)
	if err != nil {
		repo, err = git.Open(storer, fs)
		if err != nil {
			tb.Fatalf("seed git init/open: %v", err)
		}
	}
	wt, err := repo.Worktree()
	if err != nil {
		tb.Fatalf("seed git worktree: %v", err)
	}
	for path, body := range files {
		if idx := strings.LastIndex(path, "/"); idx > 0 {
			_ = fs.MkdirAll(path[:idx], 0o755)
		}
		f, err := fs.Create(path)
		if err != nil {
			tb.Fatalf("seed git create %s: %v", path, err)
		}
		_, _ = f.Write([]byte(body))
		_ = f.Close()
		if _, err := wt.Add(path); err != nil {
			tb.Fatalf("seed git add %s: %v", path, err)
		}
	}
	sig := &object.Signature{Name: "admin", Email: "admin@bleephub.local", When: time.Now()}
	if _, err := wt.Commit("seed", &git.CommitOptions{Author: sig, Committer: sig}); err != nil {
		tb.Fatalf("seed git commit: %v", err)
	}
}

// --- deterministic byte reader ------------------------------------------------

type fuzzReader struct {
	b []byte
	i int
}

func (r *fuzzReader) u8() int {
	if r.i >= len(r.b) {
		return 0
	}
	v := r.b[r.i]
	r.i++
	return int(v)
}

// pick returns a stable index in [0,n).
func (r *fuzzReader) pick(n int) int {
	if n <= 0 {
		return 0
	}
	return r.u8() % n
}

func (r *fuzzReader) rest() []byte {
	if r.i >= len(r.b) {
		return nil
	}
	v := r.b[r.i:]
	r.i = len(r.b)
	return v
}

var fuzzMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD"}

var fuzzQueryParams = []string{
	"per_page=100", "per_page=-1", "per_page=99999999999999999999",
	"page=0", "page=-5", "page=999999999", "q=test", "q=", "sort=created",
	"direction=asc", "since=2020-01-01T00:00:00Z", "since=notadate",
	"state=all", "state=bogus", "first=100", "first=-1", "after=Y3Vyc29yOjA=",
	"after=!!!!", "ref=main", "path=..", "affiliation=x", "type=all",
	"labels=a,b,c", "milestone=*", "assignee=none",
}

var fuzzBodies = []string{
	"{}", "[]", "null", "true", "123", `""`,
	`{"name":"x"}`, `{"title":"t","body":"b"}`, `{"name":123,"privacy":true}`,
	`{"content":"aGVsbG8=","message":"m"}`, `{"content":"not-base64!!"}`,
	`{"sha":"deadbeef","message":"m","tree":"x"}`,
	`{"id":99999999999999999999999999,"n":1e309}`,
	`{"scopes":[1,2,3]}`, `{"labels":[{"name":1}]}`,
	`{"a":{"b":{"c":{"d":{"e":{}}}}}}`,
	`{"ref":"refs/heads/x","sha":"y"}`,
	"not json at all", "{unterminated", `{"x":`,
}

var placeholderRe = regexp.MustCompile(`\{[^}]*\}`)

// decodeFuzzRequest turns a fuzz input into a concrete *http.Request against the
// wrapped handler. It is a pure function of data (so the corpus replays
// deterministically). It never returns nil: a request is always produced.
func (fx *fuzzFixture) decodeFuzzRequest(data []byte) *http.Request {
	r := &fuzzReader{b: data}

	// Choose a route template; usually keep its method (lands on a real handler),
	// occasionally override the method to probe method mismatches.
	tmpl := fuzzRoutePatterns[r.pick(len(fuzzRoutePatterns))]
	sp := strings.IndexByte(tmpl, ' ')
	method := tmpl[:sp]
	pathTmpl := tmpl[sp+1:]
	if r.u8()%4 == 0 {
		method = fuzzMethods[r.pick(len(fuzzMethods))]
	}

	// Fill each {placeholder}. Trailing-wildcard segments ({x...}) take a
	// possibly multi-segment value.
	path := placeholderRe.ReplaceAllStringFunc(pathTmpl, func(ph string) string {
		v := fx.segVocab[r.pick(len(fx.segVocab))]
		return v
	})

	// Assemble an optional query string.
	var q strings.Builder
	nq := r.pick(4)
	for k := 0; k < nq; k++ {
		if q.Len() > 0 {
			q.WriteByte('&')
		}
		q.WriteString(fuzzQueryParams[r.pick(len(fuzzQueryParams))])
	}

	// Body: for mutating methods, sometimes attach a JSON-ish or raw body.
	var body []byte
	jsonBody := false
	if method != "GET" && method != "HEAD" {
		switch r.pick(3) {
		case 0:
			// no body
		case 1:
			body = []byte(fuzzBodies[r.pick(len(fuzzBodies))])
			jsonBody = true
		case 2:
			body = r.rest()
		}
	}

	req := fx.buildRequest(method, path, q.String(), body, jsonBody)

	// Auth variant.
	switch r.pick(6) {
	case 0:
		req.Header.Set("Authorization", "token "+fx.adminToken)
	case 1:
		req.Header.Set("Authorization", "Bearer "+fx.adminToken)
	case 2:
		req.Header.Set("Authorization", "token "+fx.scopedToken)
	case 3:
		req.Header.Set("Authorization", "token gho_invalidtokenvalue000000000000000")
	case 4:
		// no auth
	case 5:
		req.Header.Set("Authorization", "Bearer")
	}
	return req
}

// buildRequest constructs a request without ever panicking on an unparseable
// target: control-character or space-bearing paths are set on r.URL directly so
// they still reach the mux.
func (fx *fuzzFixture) buildRequest(method, path, rawQuery string, body []byte, jsonBody bool) *http.Request {
	var rdr *bytes.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	target := "http://bleep.local" + path
	if rawQuery != "" {
		target += "?" + rawQuery
	}
	var req *http.Request
	var err error
	if rdr != nil {
		req, err = http.NewRequest(method, target, rdr)
	} else {
		req, err = http.NewRequest(method, target, http.NoBody)
	}
	if err != nil || req == nil {
		// Fall back to a guaranteed-valid request, then override the raw path so
		// adversarial bytes still drive the mux.
		var b2 *bytes.Reader
		if body != nil {
			b2 = bytes.NewReader(body)
		}
		if b2 != nil {
			req = httptest.NewRequest(method, "http://bleep.local/", b2)
		} else {
			req = httptest.NewRequest(method, "http://bleep.local/", http.NoBody)
		}
		req.URL.Path = path
		req.URL.RawPath = ""
		req.URL.RawQuery = rawQuery
	}
	if jsonBody {
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}

// serveAndCheck drives one request through the wrapped handler and asserts the
// core HTTP invariants: no panic (the fuzzer catches those), no HTTP 500, and a
// body advertised as JSON must parse as JSON.
func serveAndCheck(t *testing.T, handler http.Handler, req *http.Request) {
	t.Helper()
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code == http.StatusInternalServerError {
		t.Fatalf("HTTP 500 for %s %s\nbody: %s", req.Method, req.URL.Path, truncate(w.Body.Bytes(), 512))
	}
	ct := w.Header().Get("Content-Type")
	if strings.Contains(ct, "application/json") && w.Body.Len() > 0 {
		var v interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
			t.Fatalf("Content-Type %q but body is not valid JSON for %s %s: %v\nbody: %s",
				ct, req.Method, req.URL.Path, err, truncate(w.Body.Bytes(), 512))
		}
	}
}

func truncate(b []byte, n int) string {
	if len(b) > n {
		return string(b[:n]) + "...(truncated)"
	}
	return string(b)
}
