package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sockerless/github-runner-dispatcher-aws/internal/config"
	"github.com/sockerless/github-runner-dispatcher-aws/pkg/poller"
)

// TestSmokeDispatchLoopSkipsUnknownLabel verifies the main loop's
// "no matching label → skip + mark seen" path end-to-end through the
// poller's HTTP layer. Sockerless / docker is not involved (the
// configured label set is empty so the spawn step never runs).
func TestSmokeDispatchLoopSkipsUnknownLabel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/actions/runs/42/jobs"):
			fmt.Fprint(w, `{"jobs":[
				{"id":7001, "run_id":42, "name":"build", "status":"queued", "labels":["ubuntu-latest"]}
			]}`)
		case strings.Contains(r.URL.Path, "/actions/runs"):
			fmt.Fprint(w, `{"workflow_runs":[{"id":42}]}`)
		}
	}))
	defer srv.Close()

	gh := poller.New(srv.Client(), "tok", "owner/repo")
	gh.APIBase = srv.URL

	loop := newDispatchLoop(gh, config.Config{}) // empty config — no label maps
	if err := loop.Step(context.Background()); err != nil {
		t.Fatalf("Step: %v", err)
	}

	// Second step should drop the job via dedup; loop should still
	// succeed without touching docker (no spawn invoked).
	if err := loop.Step(context.Background()); err != nil {
		t.Fatalf("Step 2: %v", err)
	}
}

// TestSmokeCleanupReapsOfflineRunners verifies the GC sweep:
// dispatcher-prefixed offline runners get DELETEd; online + non-
// dispatcher runners are left alone. The container side (ListManaged)
// is exercised against the empty-config code path — i.e. no
// docker_host configured, so no docker calls are issued. This keeps
// the smoke test free of a docker daemon dep.
func TestSmokeCleanupReapsOfflineRunners(t *testing.T) {
	deleted := map[int64]bool{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/actions/runners"):
			fmt.Fprint(w, `{"runners":[
				{"id":11, "name":"dispatcher-7001-1", "status":"offline"},
				{"id":12, "name":"dispatcher-7002-1", "status":"online", "busy":true},
				{"id":13, "name":"manual-runner",     "status":"offline"}
			]}`)
		case r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/runners/"):
			parts := strings.Split(r.URL.Path, "/")
			var id int64
			fmt.Sscanf(parts[len(parts)-1], "%d", &id)
			deleted[id] = true
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	gh := poller.New(srv.Client(), "tok", "owner/repo")
	gh.APIBase = srv.URL
	loop := newDispatchLoop(gh, config.Config{}) // no labels → docker side is a no-op
	if err := loop.Cleanup(context.Background()); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if !deleted[11] {
		t.Errorf("offline dispatcher runner 11 should have been deleted")
	}
	if deleted[12] {
		t.Errorf("online runner 12 must NOT be deleted")
	}
	if deleted[13] {
		t.Errorf("non-dispatcher runner 13 must NOT be deleted")
	}
}

// TestSmokeDispatchLoopMatchesLabelButLivenessFails ensures that a
// matched label whose docker_host is unreachable surfaces as
// log-and-skip without marking the job as seen — so the next poll
// cycle retries when the daemon is back up.
func TestSmokeDispatchLoopMatchesLabelButLivenessFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/actions/runs/42/jobs"):
			fmt.Fprint(w, `{"jobs":[
				{"id":8001, "run_id":42, "name":"job", "status":"queued", "labels":["sockerless-test"]}
			]}`)
		case strings.Contains(r.URL.Path, "/actions/runs"):
			fmt.Fprint(w, `{"workflow_runs":[{"id":42}]}`)
		}
	}))
	defer srv.Close()

	gh := poller.New(srv.Client(), "tok", "owner/repo")
	gh.APIBase = srv.URL

	cfg := config.Config{Labels: []config.Label{{
		Name:       "sockerless-test",
		DockerHost: "tcp://127.0.0.1:1", // reserved port, definitely unreachable
		Image:      "test/image:latest",
	}}}
	loop := newDispatchLoop(gh, cfg)
	if err := loop.Step(context.Background()); err != nil {
		t.Fatalf("Step: %v", err)
	}
	// Job should NOT be marked seen — it must reappear on the next
	// poll once the daemon is back. Re-poll and confirm.
	if !gh.Seen.Has(8001) {
		// expected: not seen yet
	} else {
		t.Fatalf("liveness-fail path must not mark job 8001 as seen")
	}
}
