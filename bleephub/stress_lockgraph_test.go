package bleephub

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestStressPRSerializerRace drives the REST pull-request serializers
// (pullRequestToJSON / pullRequestSimpleJSON) concurrently with a writer that
// mutates the same PR's title, body, state, and merge fields. Under `-race`
// this fails if the serializer reads a mutable PR field after releasing the
// store lock (the early-unlock class). The serializers read a private snapshot
// taken under the lock, so this must stay clean.
func TestStressPRSerializerRace(t *testing.T) {
	s := newTestServer()
	st := s.store
	admin := st.LookupUserByLogin("admin")
	if admin == nil {
		t.Fatal("admin missing")
	}
	repo := st.CreateRepo(admin, "pr-serializer-race", "", false)
	if repo == nil {
		t.Fatal("CreateRepo nil")
	}
	seedStorePullRequestBranches(t, st, repo, "feature")
	pr := st.CreatePullRequest(repo.ID, admin.ID, "orig", "body", "feature", "main", false, nil, nil, 0)
	if pr == nil {
		t.Fatal("CreatePullRequest nil")
	}

	var stop atomic.Bool
	var wg sync.WaitGroup

	// Writer: churn the PR's mutable fields under the store write lock.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; !stop.Load(); i++ {
			st.UpdatePullRequest(pr.ID, func(p *PullRequest) {
				p.Title = fmt.Sprintf("t%d", i)
				p.Body = fmt.Sprintf("b%d", i)
				p.State = []string{"OPEN", "CLOSED", "MERGED"}[i%3]
				p.Mergeable = []string{"MERGEABLE", "CONFLICTING", "UNKNOWN"}[i%3]
				p.Additions = i
				p.MergeCommitSHA = fmt.Sprintf("%040x", i)
			})
		}
	}()

	// Readers: render the PR through both serializers for the whole window.
	for r := 0; r < 8; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for !stop.Load() {
				_ = pullRequestToJSON(pr, st, "http://x", repo.FullName)
				_ = pullRequestSimpleJSON(pr, st, "http://x", repo.FullName)
			}
		}()
	}

	time.Sleep(stressDuration(400 * time.Millisecond))
	stop.Store(true)
	wg.Wait()
}

// TestStressReaderWriterLockGraph is the regression guard for the store
// lock-graph audit (the *Locked serializer refactor and the Store.mu →
// sub-store-mutex lock order). Many readers hammer the self-locking
// serializers that gather rows under st.mu and render outside it — search,
// notifications, GraphQL PR/repo — while writers mutate the same repos,
// issues, PRs, and the follow graph. Under `-race` this fails if a serializer
// re-reads a mutable field off-lock or re-acquires st.mu while holding it; the
// watchdog fails it if a lock-order inversion deadlocks the whole set instead
// of letting the go-test timeout kill the binary.
func TestStressReaderWriterLockGraph(t *testing.T) {
	h := wrappedTestHandler(testServer)
	tok := defaultToken

	// A repo with issues + a PR gives the GraphQL PR serializer and the
	// search/notification scans something non-trivial to render.
	const repoName = "stress-lockgraph"
	if r := ghPost(t, "/api/v3/user/repos", tok, map[string]interface{}{"name": repoName}); r.StatusCode != 201 && r.StatusCode != 422 {
		drainClose(r)
		t.Fatalf("seed repo: %d", r.StatusCode)
	} else {
		drainClose(r)
	}
	repoPath := "/api/v3/repos/admin/" + repoName
	for j := 0; j < 4; j++ {
		if r := ghPost(t, repoPath+"/issues", tok, map[string]interface{}{"title": fmt.Sprintf("lg issue %d", j), "body": "b"}); r != nil {
			drainClose(r)
		}
	}

	dur := stressDuration(3 * time.Second)
	deadline := time.Now().Add(dur)

	var bad atomic.Int64
	errCh := make(chan error, 256)
	reportErr := func(err error) {
		select {
		case errCh <- err:
		default:
		}
	}
	call := func(method, path string, body interface{}) {
		resp, err := doReq(h, method, path, tok, body)
		if err != nil {
			reportErr(fmt.Errorf("%s %s: %w", method, path, err))
			return
		}
		defer drainClose(resp)
		if resp.StatusCode >= 500 {
			bad.Add(1)
			reportErr(fmt.Errorf("%s %s: server error %d", method, path, resp.StatusCode))
		}
	}

	readPaths := []string{
		"/api/v3/search/issues?q=lg",
		"/api/v3/search/repositories?q=stress-lockgraph",
		"/api/v3/search/users?q=admin",
		"/api/v3/users/admin/followers",
		"/api/v3/users/admin/following",
		"/api/v3/notifications?all=true",
		repoPath,
		repoPath + "/issues?state=all",
		repoPath + "/pulls?state=all",
	}
	prGraphQL := map[string]interface{}{
		"query": `query($o:String!,$n:String!){repository(owner:$o,name:$n){
			pullRequests(first:10){nodes{number title state}}
			issues(first:10){nodes{number title}}}}`,
		"variables": map[string]interface{}{"o": "admin", "n": repoName},
	}

	var readers sync.WaitGroup
	nReaders := stressWorkers(20)
	for i := 0; i < nReaders; i++ {
		readers.Add(1)
		go func(id int) {
			defer readers.Done()
			for time.Now().Before(deadline) {
				if id%4 == 0 {
					call("POST", "/api/graphql", prGraphQL)
					continue
				}
				call("GET", readPaths[id%len(readPaths)], nil)
			}
		}(i)
	}

	var readersDone atomic.Bool
	var writers sync.WaitGroup
	var seq atomic.Int64

	// Writer A: issue/PR churn — creates, edits, state flips, and merges feed
	// the search/notification/GraphQL scans while they run.
	writers.Add(1)
	go func() {
		defer writers.Done()
		for n := 0; !readersDone.Load(); n++ {
			s := seq.Add(1)
			if n%3 == 0 {
				call("POST", repoPath+"/issues", map[string]interface{}{"title": fmt.Sprintf("w issue %d", s), "body": "x"})
			} else {
				call("PATCH", fmt.Sprintf("%s/issues/%d", repoPath, 1+int(s)%4),
					map[string]interface{}{"title": fmt.Sprintf("edited %d", s), "state": []string{"open", "closed"}[n%2]})
			}
		}
	}()

	// Writer B: follow-graph churn — exercises the Store.mu / Misc.mu ordering
	// in both directions against the followers/following readers.
	writers.Add(1)
	go func() {
		defer writers.Done()
		for n := 0; !readersDone.Load(); n++ {
			if n%2 == 0 {
				call("PUT", "/api/v3/user/following/admin", nil)
			} else {
				call("DELETE", "/api/v3/user/following/admin", nil)
			}
		}
	}()

	// Writer C: repo metadata churn — description/topics mutate the repo the
	// serializers render.
	writers.Add(1)
	go func() {
		defer writers.Done()
		for n := 0; !readersDone.Load(); n++ {
			s := seq.Add(1)
			call("PATCH", repoPath, map[string]interface{}{"description": fmt.Sprintf("desc %d", s)})
			call("PUT", repoPath+"/topics", map[string]interface{}{"names": []string{fmt.Sprintf("t%d", s%7)}})
		}
	}()

	done := make(chan struct{})
	go func() {
		readers.Wait()
		readersDone.Store(true)
		writers.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(dur + 60*time.Second):
		t.Fatal("deadlock: reader/writer lock-graph storm did not finish — a serializer re-acquired st.mu, or Store.mu/sub-store lock order inverted")
	}

	close(errCh)
	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}
	if bad.Load() > 0 {
		for i, err := range errs {
			if i >= 20 {
				break
			}
			t.Error(err)
		}
		t.Fatalf("reader/writer storm saw %d server-error (5xx) responses", bad.Load())
	}
}
