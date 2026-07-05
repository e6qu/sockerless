package bleephub

import (
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// loadDuration is the steady-state duration of the sustained-load test. It is
// bounded by default so the test is a permanent part of `go test ./...`, and
// extendable for soak runs via BLEEPHUB_LOAD_SECONDS (e.g. =120 for a 2-minute
// leak hunt).
func loadDuration(t *testing.T) time.Duration {
	if v := os.Getenv("BLEEPHUB_LOAD_SECONDS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			t.Fatalf("BLEEPHUB_LOAD_SECONDS=%q invalid", v)
		}
		return time.Duration(n) * time.Second
	}
	return 4 * time.Second
}

func loadWorkers() int {
	if v := os.Getenv("BLEEPHUB_LOAD_WORKERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 32
}

// readWorkload is the set of realistic read requests a mixed client population
// issues. It is deliberately READ-ONLY so that any growth in HeapInuse or
// goroutine count across a steady-state run is a leak, not legitimate data
// accumulation.
func readWorkload(org, gitRepo string) []struct{ method, target string } {
	reqs := []struct{ method, target string }{
		{"GET", "/api/v3/orgs/" + org + "/repos?per_page=30"},
		{"GET", "/api/v3/repos/" + org + "/repo-0010/issues?state=all&per_page=30"},
		{"GET", "/api/v3/repos/" + org + "/repo-0020/pulls?state=all&per_page=30"},
		{"GET", "/api/v3/repos/" + org + "/repo-0030/issues/5"},
		{"GET", "/api/v3/repos/" + org + "/repo-0040"},
		{"GET", "/api/v3/search/issues?q=widgets+throughput"},
		{"GET", "/api/v3/notifications?all=true&per_page=50"},
		{"GET", "/api/v3/repos/" + org + "/" + gitRepo + "/commits?per_page=30"},
		{"GET", "/api/v3/repos/" + org + "/" + gitRepo + "/contributors"},
	}
	return reqs
}

func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(p / 100 * float64(len(sorted)))
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// TestLoadSustained drives the fully-wrapped handler from many concurrent
// clients doing a realistic read-mostly workload for a bounded duration,
// reports p50/p95/p99 latency and throughput, and asserts that under steady
// state neither HeapInuse nor the goroutine count climbs without bound (a
// monotonic climb is a leak) and that tail latency stays bounded (no cliff).
func TestLoadSustained(t *testing.T) {
	if testing.Short() {
		t.Skip("sustained-load test skipped in -short")
	}
	cfg := defaultCorpus()
	cfg.repos = 120 // keep seed time modest; still large enough to expose scans
	_, h, org, gitRepo := benchServer(t, cfg)
	reqs := readWorkload(org, gitRepo)

	workers := loadWorkers()
	dur := loadDuration(t)

	var stop atomic.Bool
	var total atomic.Int64
	var fiveXX atomic.Int64
	latCh := make(chan []time.Duration, workers)

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			lats := make([]time.Duration, 0, 4096)
			i := seed
			for !stop.Load() {
				req := reqs[i%len(reqs)]
				i++
				r := httptest.NewRequest(req.method, req.target, nil)
				r.Header.Set("Authorization", "token "+defaultToken)
				rec := httptest.NewRecorder()
				start := time.Now()
				h.ServeHTTP(rec, r)
				lats = append(lats, time.Since(start))
				total.Add(1)
				if rec.Code >= 500 {
					fiveXX.Add(1)
				}
			}
			latCh <- lats
		}(w)
	}

	// Warm up, then take the steady-state memory/goroutine baseline after a GC.
	warmup := dur / 4
	if warmup > 2*time.Second {
		warmup = 2 * time.Second
	}
	time.Sleep(warmup)
	runtime.GC()
	var base runtime.MemStats
	runtime.ReadMemStats(&base)
	baseHeap := base.HeapInuse
	baseGoros := runtime.NumGoroutine()

	// Sample HeapInuse + goroutines across the steady-state window.
	type sample struct {
		heap  uint64
		goros int
	}
	var samples []sample
	sampleDone := make(chan struct{})
	go func() {
		tick := time.NewTicker(dur / 10)
		defer tick.Stop()
		for {
			select {
			case <-sampleDone:
				return
			case <-tick.C:
				var m runtime.MemStats
				runtime.ReadMemStats(&m)
				samples = append(samples, sample{m.HeapInuse, runtime.NumGoroutine()})
			}
		}
	}()

	time.Sleep(dur)
	stop.Store(true)
	close(sampleDone)
	wg.Wait()

	var all []time.Duration
	for w := 0; w < workers; w++ {
		all = append(all, <-latCh...)
	}
	sort.Slice(all, func(i, j int) bool { return all[i] < all[j] })

	p50 := percentile(all, 50)
	p95 := percentile(all, 95)
	p99 := percentile(all, 99)
	tot := total.Load()
	tput := float64(tot) / dur.Seconds()

	// Let webhook/workflow-trigger goroutines (if any) drain, then measure the
	// settled heap/goroutine state.
	time.Sleep(200 * time.Millisecond)
	runtime.GC()
	var final runtime.MemStats
	runtime.ReadMemStats(&final)
	finalGoros := runtime.NumGoroutine()

	t.Logf("sustained load: workers=%d duration=%s requests=%d throughput=%.0f req/s",
		workers, dur, tot, tput)
	t.Logf("latency p50=%s p95=%s p99=%s max=%s", p50, p95, p99, all[len(all)-1])
	t.Logf("heap: baseline=%dKiB final=%dKiB", baseHeap/1024, final.HeapInuse/1024)
	t.Logf("goroutines: baseline=%d final=%d", baseGoros, finalGoros)
	for i, s := range samples {
		t.Logf("  sample %d: heap=%dKiB goroutines=%d", i, s.heap/1024, s.goros)
	}

	if fiveXX.Load() > 0 {
		t.Errorf("observed %d 5xx responses under load", fiveXX.Load())
	}
	if tot == 0 {
		t.Fatal("no requests completed")
	}

	// Memory-leak assertion: compare the mean HeapInuse of the last third of
	// steady-state samples to the first third. A real leak climbs monotonically;
	// a healthy server oscillates around a plateau. Allow generous slack for GC
	// timing and transient allocation.
	if len(samples) >= 6 {
		third := len(samples) / 3
		var firstSum, lastSum uint64
		for i := 0; i < third; i++ {
			firstSum += samples[i].heap
		}
		for i := len(samples) - third; i < len(samples); i++ {
			lastSum += samples[i].heap
		}
		firstAvg := firstSum / uint64(third)
		lastAvg := lastSum / uint64(third)
		// 2× plateau + 16 MiB absolute slack; a genuine leak over thousands of
		// requests blows well past this.
		limit := firstAvg*2 + 16*1024*1024
		if lastAvg > limit {
			t.Errorf("HeapInuse climbed under steady-state read load: first-third avg=%dKiB last-third avg=%dKiB (limit %dKiB) — possible leak",
				firstAvg/1024, lastAvg/1024, limit/1024)
		}
	}

	// Goroutine-leak assertion: after draining, the count must return near the
	// baseline (read-only workload spawns no lasting goroutines).
	if finalGoros > baseGoros+workers+20 {
		t.Errorf("goroutine count climbed: baseline=%d final=%d (workload is read-only)", baseGoros, finalGoros)
	}

	// Latency-cliff guard: in-process handling of any read in this workload is
	// sub-second even with a large corpus; a p99 in the multi-second range means
	// a pathological scan or lock stall.
	if p99 > 2*time.Second {
		t.Errorf("p99 latency %s exceeds 2s — latency cliff", p99)
	}
}

// TestLoadWebhookDeliveryBounded fires a large burst of webhook events at a
// fast local sink and asserts (a) the retained per-hook delivery history stays
// capped at maxHookDeliveries rather than growing without bound, and (b) the
// per-delivery goroutines drain back to baseline (no goroutine leak).
func TestLoadWebhookDeliveryBounded(t *testing.T) {
	if testing.Short() {
		t.Skip("webhook-delivery load test skipped in -short")
	}
	s := NewServer("127.0.0.1:0", zerolog.Nop())
	admin := s.store.LookupUserByLogin("admin")
	org := s.store.CreateOrg(admin, "hook-org", "Hook Org", "")
	repo := s.store.CreateOrgRepo(org, admin, "hooked", "", false)
	repoKey := repo.FullName

	var received atomic.Int64
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer sink.Close()

	hook := s.store.CreateHook(repoKey, sink.URL, "", "json", "0", []string{"push"}, true)
	if hook == nil {
		t.Fatal("failed to create hook")
	}

	runtime.GC()
	baseGoros := runtime.NumGoroutine()

	const events = 1500
	for i := 0; i < events; i++ {
		s.emitWebhookEvent(repoKey, "push", "", map[string]interface{}{"seq": i})
	}

	// Wait for deliveries to settle (sink returns 200 on the first attempt so
	// each goroutine is short-lived) and for the delivery goroutines to drain
	// back to baseline. A burst emits one goroutine per event; they serialize
	// on the store write lock, so draining is contention-bound, not leaked —
	// poll until the count settles rather than guessing a fixed sleep.
	deadline := time.Now().Add(30 * time.Second)
	finalGoros := runtime.NumGoroutine()
	for time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
		if int(received.Load()) < events {
			continue
		}
		runtime.GC()
		finalGoros = runtime.NumGoroutine()
		if finalGoros <= baseGoros+20 {
			break
		}
	}

	got := len(s.store.ListDeliveries(hook.ID))
	t.Logf("emitted %d events, sink received %d, retained deliveries=%d (cap %d)",
		events, received.Load(), got, maxHookDeliveries)
	t.Logf("goroutines: baseline=%d final=%d", baseGoros, finalGoros)

	if got > maxHookDeliveries {
		t.Errorf("retained %d deliveries, expected <= cap %d (unbounded growth)", got, maxHookDeliveries)
	}
	if got == 0 {
		t.Fatal("no deliveries recorded — webhook path not exercised")
	}
	if finalGoros > baseGoros+50 {
		t.Errorf("webhook delivery goroutines leaked: baseline=%d final=%d", baseGoros, finalGoros)
	}
}
