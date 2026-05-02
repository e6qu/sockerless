package poller

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPollOnceDedup(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/actions/runs") && !strings.Contains(r.URL.Path, "/jobs"):
			fmt.Fprint(w, `{"workflow_runs":[{"id":42}]}`)
		case strings.Contains(r.URL.Path, "/actions/runs/42/jobs"):
			fmt.Fprint(w, `{"jobs":[
				{"id":7001, "run_id":42, "name":"build", "status":"queued", "labels":["sockerless-ecs"], "html_url":"https://gh/x/runs/42/job/7001"},
				{"id":7002, "run_id":42, "name":"test",  "status":"in_progress", "labels":["sockerless-ecs"]}
			]}`)
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	c := New(srv.Client(), "tok", "owner/repo")
	c.APIBase = srv.URL

	// First call — should yield the queued job.
	jobs, err := c.PollOnce(context.Background())
	if err != nil {
		t.Fatalf("PollOnce 1: %v", err)
	}
	if len(jobs) != 1 || jobs[0].JobID != 7001 || jobs[0].Name != "build" {
		t.Fatalf("PollOnce 1 jobs: %+v", jobs)
	}
	if jobs[0].Labels[0] != "sockerless-ecs" {
		t.Fatalf("PollOnce 1 labels: %+v", jobs[0].Labels)
	}

	// Mark and re-poll — dedup should drop the job.
	c.Mark(7001)
	jobs, err = c.PollOnce(context.Background())
	if err != nil {
		t.Fatalf("PollOnce 2: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("PollOnce 2 should have deduped, got: %+v", jobs)
	}
}

func TestPollOnceAuthHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer my-pat" {
			t.Errorf("missing/wrong Authorization: %q", got)
		}
		fmt.Fprint(w, `{"workflow_runs":[]}`)
	}))
	defer srv.Close()
	c := New(srv.Client(), "my-pat", "owner/repo")
	c.APIBase = srv.URL
	if _, err := c.PollOnce(context.Background()); err != nil {
		t.Fatalf("PollOnce: %v", err)
	}
}

func TestSeenSetTTL(t *testing.T) {
	s := newSeenSet(50 * time.Millisecond)
	s.Add(1, time.Now().Add(-1*time.Second))
	if s.Has(1) {
		t.Fatalf("entry past TTL should be purged")
	}
	now := time.Now()
	s.Add(2, now)
	if !s.Has(2) {
		t.Fatalf("fresh entry should be present")
	}
}
