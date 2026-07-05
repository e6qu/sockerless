package bleephub

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// FuzzSearchQueryParser drives the qualifier tokenizer (repo:/user:/org:/is:/
// in:/label:/language:/author:/hash:, quoted phrases, bare terms) with raw
// attacker-controlled `q` strings. The parser must never panic or hang and
// must always yield a searchQuery whose slice fields are non-nil-safe to
// iterate.
func FuzzSearchQueryParser(f *testing.F) {
	seeds := []string{
		`repo:octo/cat is:issue is:open label:bug "needs triage" author:alice`,
		`user:me org:acme language:go in:title in:body hash:deadbeef`,
		`:`, `::`, `repo:`, `is:`, `is::`, `"unterminated`, `""`, `"" ""`,
		`state:CLOSED type:USER extension:.go filename:main.go path:a/b`,
		`label:"multi word label" repo:a/b`,
		"\t\t  \t", "a\tb\tc", `+1 -1`,
		`repo:a:b:c`, `is:pull-request`, `is:pr`, `ext:go file:x`,
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, q string) {
		req := httptest.NewRequest(http.MethodGet, "/search/issues?"+url.Values{"q": {q}}.Encode(), nil)
		parsed := parseSearchQuery(req)
		// Every field must be safe to consume: iterate the term slice, read the
		// pointer bools. A panic here fails the fuzz.
		for range parsed.Terms {
		}
		_ = parsed.IsIssue
		_ = parsed.IsPR
		_ = parsed.matchesText("any text to match")
		if parsed.PerPage < 0 || parsed.Page < 0 {
			t.Fatalf("negative pagination from q=%q: page=%d perPage=%d", q, parsed.Page, parsed.PerPage)
		}
	})
}

// FuzzSearchEndpoints drives every /search/* handler with fuzzed q + page +
// per_page against a small seeded corpus. Invariant: no 5xx/panic; a 200
// response is always a well-formed search envelope (integer total_count + an
// items array), never a truncated or wrong-typed body.
func FuzzSearchEndpoints(f *testing.F) {
	s := fuzzRoutedServer(f)
	admin := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(admin, "search-fuzz", "searchable repo", false)
	s.store.CreateIssue(repo.ID, admin.ID, "findable bug", "body with keyword", nil, nil, 0)
	stor := s.store.GetGitStorage(admin.Login, "search-fuzz")
	_, _ = initRepoWithFiles(stor, "main", "init", map[string]string{"main.go": "package main"}, repoSignature(admin.Login, "a@b.c"))

	endpoints := []string{"issues", "repositories", "code", "users", "commits", "labels", "topics"}

	f.Add(`repo:admin/search-fuzz bug`, "1", "30")
	f.Add(`is:issue is:open`, "0", "0")
	f.Add(``, "-5", "999999999")
	f.Add(`label:x`, "2147483647", "100")
	f.Add(`"quoted phrase`, "1", "1")
	f.Add(`user:admin`, "1", "101")
	f.Add(`repo:admin/search-fuzz`, "9223372036854775807", "9223372036854775807")

	f.Fuzz(func(t *testing.T, q, page, perPage string) {
		for _, ep := range endpoints {
			vals := url.Values{"q": {q}, "page": {page}, "per_page": {perPage}}
			w := fuzzServe(s, http.MethodGet, "/api/v3/search/"+ep+"?"+vals.Encode(), nil)
			if w.Code >= 500 {
				t.Fatalf("search/%s q=%q page=%q per_page=%q -> %d: %s", ep, q, page, perPage, w.Code, w.Body.String())
			}
			if w.Code == http.StatusOK {
				var env map[string]interface{}
				if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
					t.Fatalf("search/%s 200 body not JSON: %v (%s)", ep, err, w.Body.String())
				}
				if _, ok := env["total_count"]; !ok {
					t.Fatalf("search/%s envelope missing total_count: %v", ep, env)
				}
				if _, ok := env["items"].([]interface{}); !ok {
					t.Fatalf("search/%s envelope items not an array: %v", ep, env["items"])
				}
			}
		}
	})
}
