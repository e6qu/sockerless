package poller

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestListRunners(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/actions/runners") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"runners":[
			{"id":11, "name":"dispatcher-7001-1", "status":"offline", "busy":false},
			{"id":12, "name":"dispatcher-7002-1", "status":"online",  "busy":true},
			{"id":13, "name":"some-other-runner", "status":"offline", "busy":false}
		]}`)
	}))
	defer srv.Close()

	c := New(srv.Client(), "tok", "owner/repo")
	c.APIBase = srv.URL
	rs, err := c.ListRunners(context.Background())
	if err != nil {
		t.Fatalf("ListRunners: %v", err)
	}
	if len(rs) != 3 {
		t.Fatalf("want 3 runners, got %d", len(rs))
	}
	dispatcherCount := 0
	for _, r := range rs {
		if IsDispatcherRunner(r) {
			dispatcherCount++
		}
	}
	if dispatcherCount != 2 {
		t.Fatalf("IsDispatcherRunner should match 2 entries, got %d", dispatcherCount)
	}
}

func TestDeleteRunnerIdempotent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/runners/") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		// 404 for "already gone" — should be tolerated.
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	c := New(srv.Client(), "tok", "owner/repo")
	c.APIBase = srv.URL
	if err := c.DeleteRunner(context.Background(), 99); err != nil {
		t.Fatalf("DeleteRunner on 404 should be tolerated, got: %v", err)
	}
}

func TestDeleteRunnerSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	c := New(srv.Client(), "tok", "owner/repo")
	c.APIBase = srv.URL
	if err := c.DeleteRunner(context.Background(), 99); err != nil {
		t.Fatalf("DeleteRunner: %v", err)
	}
}

// TestListRunnersPaginates proves a pool past 100 registrations is
// walked page by page — silent truncation at page 1 left the GC sweep
// blind to the tail.
func TestListRunnersPaginates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		switch page {
		case "", "1":
			var sb strings.Builder
			sb.WriteString(`{"runners":[`)
			for i := 0; i < 100; i++ {
				if i > 0 {
					sb.WriteString(",")
				}
				fmt.Fprintf(&sb, `{"id":%d,"name":"dispatcher-%d-1","status":"offline"}`, i+1, i+1)
			}
			sb.WriteString(`]}`)
			fmt.Fprint(w, sb.String())
		case "2":
			fmt.Fprint(w, `{"runners":[{"id":101,"name":"dispatcher-101-1","status":"offline"}]}`)
		default:
			t.Errorf("unexpected page %q requested", page)
			fmt.Fprint(w, `{"runners":[]}`)
		}
	}))
	defer srv.Close()

	c := New(srv.Client(), "tok", "owner/repo")
	c.APIBase = srv.URL
	rs, err := c.ListRunners(context.Background())
	if err != nil {
		t.Fatalf("ListRunners: %v", err)
	}
	if len(rs) != 101 {
		t.Fatalf("want 101 runners across 2 pages, got %d", len(rs))
	}
}
