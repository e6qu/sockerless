package bleephub

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// drainGoroutines waits for the live goroutine count to settle, giving
// short-lived background work (webhook delivery HTTP round-trips, the actions
// event drain) a window to finish. It returns the settled count.
func drainGoroutines() int {
	prev := runtime.NumGoroutine()
	stable := 0
	for i := 0; i < 100; i++ {
		time.Sleep(20 * time.Millisecond)
		runtime.GC()
		n := runtime.NumGoroutine()
		if n >= prev {
			stable++
		} else {
			stable = 0
		}
		prev = n
		if stable >= 5 {
			break
		}
	}
	return prev
}

// TestStressGoroutineLeak runs repeated batches of full lifecycle operations
// that each spin up background goroutines — per-event webhook deliveries and
// the actions/checks event loop — and asserts the settled goroutine count does
// not climb monotonically across batches. A steady climb is a leak (an
// un-cancelled context, an unclosed channel, an orphaned delivery goroutine
// that never returns). The delivery target is a live 200 sink so every
// delivery goroutine terminates on its first attempt.
func TestStressGoroutineLeak(t *testing.T) {
	var delivered atomic.Int64
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		delivered.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer sink.Close()

	s := newTestServer()
	admin := s.store.LookupUserByLogin("admin")
	if admin == nil {
		t.Fatal("admin missing")
	}

	batch := func(round int) {
		var wg sync.WaitGroup
		for i := 0; i < 12; i++ {
			wg.Add(1)
			go func(n int) {
				defer wg.Done()
				repoKey := fmt.Sprintf("admin/leak-%d-%d", round, n)
				name := fmt.Sprintf("leak-%d-%d", round, n)
				if s.store.CreateRepo(admin, name, "", false) == nil {
					return
				}
				hook := s.store.CreateHook(repoKey, sink.URL, "sekret", "json", "0", []string{"push", "issues"}, true)
				if hook == nil {
					return
				}
				// Each emit spawns one delivery goroutine per matching hook;
				// they must all return after hitting the 200 sink.
				for e := 0; e < 3; e++ {
					s.emitWebhookEvent(repoKey, "push", "", map[string]interface{}{"ref": "refs/heads/main", "n": e})
				}
			}(i)
		}
		wg.Wait()
	}

	// Warm-up round: spin up the singleton background machinery (actions event
	// loop, http transport pools) so it is part of the baseline, not counted
	// as a leak.
	batch(0)
	baseline := drainGoroutines()

	const rounds = 6
	for r := 1; r <= rounds; r++ {
		batch(r)
	}
	settled := drainGoroutines()

	if delivered.Load() == 0 {
		t.Fatal("no webhook deliveries reached the sink — the leak probe exercised nothing")
	}

	// Allow a small constant slack for pooled connections / GC-pending
	// stacks, but a per-round climb (rounds * work) is a real leak.
	slack := 16
	if settled > baseline+slack {
		t.Errorf("goroutine count climbed from %d to %d over %d batches (slack %d) — likely a leak; deliveries=%d",
			baseline, settled, rounds, slack, delivered.Load())
	}
}
