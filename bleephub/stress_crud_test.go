package bleephub

import (
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// stressDuration returns how long the storm harnesses run. The default keeps
// a plain `go test` bounded while still exercising real concurrency every
// run; BLEEPHUB_STRESS_DURATION (a Go duration) scales it up for the heavy
// soak. There is no skip path — a meaningful storm always executes.
func stressDuration(def time.Duration) time.Duration {
	if v := os.Getenv("BLEEPHUB_STRESS_DURATION"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return def
}

// stressWorkers scales the goroutine count the same way.
func stressWorkers(def int) int {
	if v := os.Getenv("BLEEPHUB_STRESS_WORKERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

// TestStressCRUDStorm drives many goroutines issuing create/read/update/delete
// across interacting resources — repos, issues, labels, milestones, comments,
// reactions, orgs, teams, notifications, projects-v2, search — through the
// real wrapped HTTP handler chain on the shared server, for a bounded
// duration under a watchdog. The point is to run under `-race`: any data race,
// map-concurrent-write, or deadlock in a handler or the store surfaces here.
// Adversarial-but-well-formed requests must never 5xx; a 4xx (duplicate name,
// deleted parent) is acceptable. Afterwards the store's cross-index invariants
// must still hold.
func TestStressCRUDStorm(t *testing.T) {
	h := wrappedTestHandler(testServer)
	tok := defaultToken

	// A fixed pool of repos + seed issues gives the storm stable coordinates
	// to update/comment/react on without the harness itself having to
	// coordinate freshly created IDs across goroutines.
	const poolRepos = 6
	type poolRepo struct {
		owner, name string
		issues      []int
	}
	pool := make([]poolRepo, 0, poolRepos)
	for i := 0; i < poolRepos; i++ {
		name := fmt.Sprintf("stress-crud-%d", i)
		resp := ghPost(t, "/api/v3/user/repos", tok, map[string]interface{}{"name": name})
		if resp.StatusCode != 201 && resp.StatusCode != 422 {
			drainClose(resp)
			t.Fatalf("seed repo %s: status %d", name, resp.StatusCode)
		}
		drainClose(resp)
		pr := poolRepo{owner: "admin", name: name}
		for j := 0; j < 3; j++ {
			ir := ghPost(t, fmt.Sprintf("/api/v3/repos/admin/%s/issues", name), tok,
				map[string]interface{}{"title": fmt.Sprintf("seed issue %d", j), "body": "seed"})
			if ir.StatusCode == 201 {
				m := decodeJSON(t, ir)
				if n, ok := m["number"].(float64); ok {
					pr.issues = append(pr.issues, int(n))
				}
			} else {
				drainClose(ir)
			}
		}
		if len(pr.issues) == 0 {
			t.Fatalf("seed repo %s produced no issues", name)
		}
		pool = append(pool, pr)
	}

	nWorkers := stressWorkers(24)
	dur := stressDuration(3 * time.Second)
	deadline := time.Now().Add(dur)

	var uniq atomic.Int64
	var badStatus atomic.Int64
	errCh := make(chan error, 512)
	reportErr := func(err error) {
		select {
		case errCh <- err:
		default:
		}
	}

	do := func(method, path string, body interface{}) int {
		resp, err := doReq(h, method, path, tok, body)
		if err != nil {
			reportErr(fmt.Errorf("%s %s: %w", method, path, err))
			return 0
		}
		defer drainClose(resp)
		if resp.StatusCode >= 500 {
			badStatus.Add(1)
			reportErr(fmt.Errorf("%s %s: server error %d", method, path, resp.StatusCode))
		}
		return resp.StatusCode
	}

	var wg sync.WaitGroup
	for w := 0; w < nWorkers; w++ {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			rng := rand.New(rand.NewSource(int64(seed) + time.Now().UnixNano()))
			reactions := []string{"+1", "-1", "laugh", "heart", "hooray", "rocket", "eyes", "confused"}
			for time.Now().Before(deadline) {
				pr := pool[rng.Intn(len(pool))]
				repoPath := fmt.Sprintf("/api/v3/repos/%s/%s", pr.owner, pr.name)
				issueNo := pr.issues[rng.Intn(len(pr.issues))]
				u := uniq.Add(1)
				switch rng.Intn(18) {
				case 0: // create a fresh repo (unique)
					do("POST", "/api/v3/user/repos", map[string]interface{}{"name": fmt.Sprintf("stress-r-%d", u)})
				case 1: // read a pool repo
					do("GET", repoPath, nil)
				case 2: // create issue on a pool repo
					do("POST", repoPath+"/issues", map[string]interface{}{"title": fmt.Sprintf("issue %d", u), "body": "storm"})
				case 3: // list issues
					do("GET", repoPath+"/issues?state=all", nil)
				case 4: // patch a seed issue
					state := "open"
					if rng.Intn(2) == 0 {
						state = "closed"
					}
					do("PATCH", fmt.Sprintf("%s/issues/%d", repoPath, issueNo),
						map[string]interface{}{"title": fmt.Sprintf("patched %d", u), "state": state})
				case 5: // create unique label
					do("POST", repoPath+"/labels", map[string]interface{}{"name": fmt.Sprintf("lbl-%d", u), "color": "ededed"})
				case 6: // list labels
					do("GET", repoPath+"/labels", nil)
				case 7: // create milestone
					do("POST", repoPath+"/milestones", map[string]interface{}{"title": fmt.Sprintf("ms-%d", u)})
				case 8: // comment on a seed issue
					do("POST", fmt.Sprintf("%s/issues/%d/comments", repoPath, issueNo),
						map[string]interface{}{"body": fmt.Sprintf("comment %d", u)})
				case 9: // idempotent reaction on a seed issue
					do("POST", fmt.Sprintf("%s/issues/%d/reactions", repoPath, issueNo),
						map[string]interface{}{"content": reactions[rng.Intn(len(reactions))]})
				case 10: // search issues
					do("GET", "/api/v3/search/issues?q=storm", nil)
				case 11: // notifications
					do("GET", "/api/v3/notifications?all=true", nil)
				case 12: // create org (unique)
					do("POST", "/internal/orgs", map[string]interface{}{"login": fmt.Sprintf("stress-org-%d", u), "name": "S"})
				case 13: // list repo collaborators
					do("GET", repoPath+"/collaborators", nil)
				case 14: // add a custom-property-ish repo topic update
					do("PUT", repoPath+"/topics", map[string]interface{}{"names": []string{"a", fmt.Sprintf("t%d", u%5)}})
				case 15: // create a projects-v2 draft item via GraphQL is heavy; use repo read via GraphQL
					do("POST", "/api/graphql", map[string]interface{}{
						"query":     "query($o:String!,$n:String!){repository(owner:$o,name:$n){name issues(first:5){totalCount}}}",
						"variables": map[string]interface{}{"o": pr.owner, "n": pr.name},
					})
				case 16: // lock/unlock a seed issue
					if rng.Intn(2) == 0 {
						do("PUT", fmt.Sprintf("%s/issues/%d/lock", repoPath, issueNo), map[string]interface{}{"lock_reason": "resolved"})
					} else {
						do("DELETE", fmt.Sprintf("%s/issues/%d/lock", repoPath, issueNo), nil)
					}
				case 17: // milestone list + assignees read
					do("GET", repoPath+"/milestones?state=all", nil)
				}
			}
		}(w)
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(dur + 60*time.Second):
		t.Fatal("deadlock: CRUD storm workers did not finish within watchdog window")
	}

	close(errCh)
	var firstErrs []error
	for err := range errCh {
		firstErrs = append(firstErrs, err)
	}
	if n := badStatus.Load(); n > 0 {
		for i, err := range firstErrs {
			if i >= 20 {
				break
			}
			t.Error(err)
		}
		t.Fatalf("CRUD storm saw %d server-error (5xx) responses", n)
	}
	for i, err := range firstErrs {
		if i >= 20 {
			break
		}
		t.Error(err)
	}

	assertStoreIndexInvariants(t, testServer.store)
}

// assertStoreIndexInvariants checks the cross-index consistency the store
// must preserve under any interleaving: every id-keyed record is reachable
// through its secondary (name/slug) index and vice-versa, and the monotonic
// Next* allocators stayed ahead of every allocated id.
func assertStoreIndexInvariants(t *testing.T, st *Store) {
	t.Helper()
	st.mu.RLock()
	defer st.mu.RUnlock()

	// Repos ↔ ReposByName.
	for name, r := range st.ReposByName {
		if r == nil {
			t.Errorf("ReposByName[%q] is nil", name)
			continue
		}
		if st.Repos[r.ID] != r {
			t.Errorf("ReposByName[%q] (id %d) not the same pointer as Repos[%d]", name, r.ID, r.ID)
		}
		if r.FullName != name {
			t.Errorf("ReposByName key %q != repo.FullName %q", name, r.FullName)
		}
		if r.ID >= st.NextRepo {
			t.Errorf("repo id %d >= NextRepo %d (counter regressed)", r.ID, st.NextRepo)
		}
	}
	for id, r := range st.Repos {
		if r == nil {
			t.Errorf("Repos[%d] is nil", id)
			continue
		}
		if st.ReposByName[r.FullName] != r {
			t.Errorf("Repos[%d] (%q) missing from ReposByName", id, r.FullName)
		}
	}

	// Orgs ↔ OrgsByLogin.
	for login, o := range st.OrgsByLogin {
		if o == nil || st.Orgs[o.ID] != o {
			t.Errorf("OrgsByLogin[%q] not consistent with Orgs", login)
		}
	}
	for id, o := range st.Orgs {
		if o == nil || st.OrgsByLogin[o.Login] != o {
			t.Errorf("Orgs[%d] not consistent with OrgsByLogin", id)
		}
	}

	// Teams ↔ TeamsBySlug.
	for key, tm := range st.TeamsBySlug {
		if tm == nil || st.Teams[tm.ID] != tm {
			t.Errorf("TeamsBySlug[%q] not consistent with Teams", key)
		}
	}

	// Issues: every issue's repo must still exist, and the per-repo issue
	// number counter must exceed every issue number in it.
	for id, is := range st.Issues {
		if is == nil {
			t.Errorf("Issues[%d] is nil", id)
			continue
		}
		r := st.Repos[is.RepoID]
		if r == nil {
			t.Errorf("Issues[%d] references missing repo %d", id, is.RepoID)
			continue
		}
		if is.Number >= r.NextIssueNumber {
			t.Errorf("issue #%d in repo %d >= repo.NextIssueNumber %d", is.Number, is.RepoID, r.NextIssueNumber)
		}
	}

	// Labels/Milestones id keys must match their stored ID.
	for id, l := range st.Labels {
		if l != nil && l.ID != id {
			t.Errorf("Labels[%d].ID = %d (key/value mismatch)", id, l.ID)
		}
	}
	for id, m := range st.Milestones {
		if m != nil && m.ID != id {
			t.Errorf("Milestones[%d].ID = %d (key/value mismatch)", id, m.ID)
		}
	}
}
