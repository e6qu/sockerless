package bleephub

import (
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestStoreLockReentrancyUnderWriterPressure hammers the read paths that
// gather rows under st.mu and render with the self-locking serializers
// (search, followers, notifications) while writers mutate the store and the
// follow graph for as long as the readers run. sync.RWMutex read locks are
// not reentrant: once a writer queues on Lock, new RLock calls block, so a
// handler that re-acquired st.mu while already holding it (the old
// repoToJSON-under-search-lock shape) deadlocks under exactly this pressure.
// The followers/user-search pair also exercises the Store.mu/Misc.mu ordering
// in both directions, which used to be an ABBA inversion. The watchdog fails
// the test instead of letting the whole binary hit the go test timeout.
func TestStoreLockReentrancyUnderWriterPressure(t *testing.T) {
	admin := testServer.store.LookupUserByLogin("admin")
	if admin == nil {
		t.Fatal("admin user missing")
	}
	repo := testServer.store.CreateRepo(admin, "lock-reentrancy", "", false)
	if repo == nil {
		t.Fatal("create repo failed")
	}
	if issue := testServer.store.CreateIssue(repo.ID, admin.ID, "deadlock probe", "probe body", nil, nil, 0); issue == nil {
		t.Fatal("create issue failed")
	}

	const (
		readersPerPath = 3
		readIters      = 25
	)
	readPaths := []string{
		"/api/v3/search/issues?q=deadlock",
		"/api/v3/search/repositories?q=lock-reentrancy",
		"/api/v3/search/users?q=admin",
		"/api/v3/users/admin/followers",
		"/api/v3/notifications?all=true",
	}

	errCh := make(chan error, readersPerPath*len(readPaths)+2)
	do := func(method, path string) error {
		req, err := http.NewRequest(method, testBaseURL+path, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "token "+defaultToken)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		resp.Body.Close()
		if resp.StatusCode >= 400 {
			return fmt.Errorf("%s %s: status %d", method, path, resp.StatusCode)
		}
		return nil
	}

	var readersDone atomic.Bool
	var readers sync.WaitGroup
	for _, path := range readPaths {
		for i := 0; i < readersPerPath; i++ {
			readers.Add(1)
			go func(p string) {
				defer readers.Done()
				for n := 0; n < readIters; n++ {
					if err := do("GET", p); err != nil {
						errCh <- err
						return
					}
				}
			}(path)
		}
	}

	var writers sync.WaitGroup

	// Writer A: store mutations that queue on st.mu.Lock between the read
	// paths' RLock acquisitions, for as long as the readers run. Issue
	// creation is capped so the notification/search scans stay bounded (they
	// are O(issues) per rendered row); after the cap the pressure continues
	// with a constant-size write that still takes st.mu.Lock.
	writers.Add(1)
	go func() {
		defer writers.Done()
		for n := 0; !readersDone.Load(); n++ {
			if n < 300 {
				if issue := testServer.store.CreateIssue(repo.ID, admin.ID,
					fmt.Sprintf("writer pressure %d", n), "generated during lock hammering", nil, nil, 0); issue == nil {
					errCh <- fmt.Errorf("writer: create issue %d failed", n)
					return
				}
			} else {
				testServer.store.MarkNotificationsRead(admin.ID, time.Now().UTC(), "")
			}
		}
	}()

	// Writer B: follow-graph mutations that queue on Misc.mu.Lock while the
	// followers/user-search readers cross Store.mu and Misc.mu.
	writers.Add(1)
	go func() {
		defer writers.Done()
		for n := 0; !readersDone.Load(); n++ {
			var err error
			if n%2 == 0 {
				err = do("PUT", "/api/v3/user/following/admin")
			} else {
				err = do("DELETE", "/api/v3/user/following/admin")
			}
			if err != nil {
				errCh <- fmt.Errorf("writer follow: %w", err)
				return
			}
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
	case <-time.After(90 * time.Second):
		t.Fatal("deadlock: readers/writers did not finish — a read path re-acquired st.mu while holding it, or Store.mu/Misc.mu inverted")
	}
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}
}
