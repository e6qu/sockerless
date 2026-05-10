package main

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// chdirTo makes path the current working directory for the test and
// restores it on cleanup. Used to scope `.stack-pids/<name>.log` reads
// to a temp dir.
func chdirTo(t *testing.T, dir string) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
}

func writeLogFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	pidsDir := filepath.Join(dir, ".stack-pids")
	if err := os.MkdirAll(pidsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(pidsDir, name+".log")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}
	return path
}

func setupLogsServer(t *testing.T) (*TopologyManager, *http.ServeMux, string) {
	t.Helper()
	tmp := t.TempDir()
	chdirTo(t, tmp)
	mgr := NewTopologyManager(filepath.Join(tmp, "sockerless.yaml"), "")
	if err := mgr.LoadOrMigrate(); err != nil {
		t.Fatalf("load: %v", err)
	}
	if err := mgr.Replace(Topology{Projects: []ProjectConfig{{
		Name: "p",
		Instances: []Instance{
			{Name: "sim-aws", Kind: InstanceKindSim, Cloud: CloudAWS, Port: 4500},
		},
	}}}); err != nil {
		t.Fatalf("replace: %v", err)
	}
	mux := http.NewServeMux()
	registerTopologyAPI(mux, mgr, nil)
	return mgr, mux, tmp
}

func TestInstanceLogsTailMissingFile(t *testing.T) {
	_, mux, _ := setupLogsServer(t)
	req := httptest.NewRequest("GET", "/api/v1/topology/projects/p/instances/sim-aws/logs", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var got struct {
		Lines []string `json:"lines"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v; body=%s", err, w.Body.String())
	}
	if got.Lines == nil {
		t.Errorf("missing log file should yield empty array, got nil")
	}
	if len(got.Lines) != 0 {
		t.Errorf("missing log file should be empty, got %v", got.Lines)
	}
}

func TestInstanceLogsTailLastN(t *testing.T) {
	_, mux, dir := setupLogsServer(t)
	writeLogFile(t, dir, "sim-aws", "a\nb\nc\nd\ne\n")

	req := httptest.NewRequest("GET", "/api/v1/topology/projects/p/instances/sim-aws/logs?lines=2", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var got struct {
		Lines []string `json:"lines"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if len(got.Lines) != 2 || got.Lines[0] != "d" || got.Lines[1] != "e" {
		t.Errorf("lines=2: got %v, want [d e]", got.Lines)
	}
}

func TestInstanceLogsTailDefaultCap(t *testing.T) {
	_, mux, dir := setupLogsServer(t)
	var b strings.Builder
	for i := 0; i < 250; i++ {
		b.WriteString("line\n")
	}
	writeLogFile(t, dir, "sim-aws", b.String())

	req := httptest.NewRequest("GET", "/api/v1/topology/projects/p/instances/sim-aws/logs", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	var got struct {
		Lines []string `json:"lines"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	// Default cap is 200; the 250-line file should clamp to 200.
	if len(got.Lines) != 200 {
		t.Errorf("default cap = 200; got %d", len(got.Lines))
	}
}

func TestInstanceLogsNotFound(t *testing.T) {
	_, mux, _ := setupLogsServer(t)
	req := httptest.NewRequest("GET", "/api/v1/topology/projects/p/instances/missing/logs", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestInstanceLogsStreamSeedAndAppend(t *testing.T) {
	_, mux, dir := setupLogsServer(t)
	logPath := writeLogFile(t, dir, "sim-aws", "seed-1\nseed-2\n")

	srv := httptest.NewServer(mux)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET",
		srv.URL+"/api/v1/topology/projects/p/instances/sim-aws/logs?follow=1&lines=10", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	if got := resp.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", got)
	}

	gotLines := make(chan string, 32)
	go func() {
		defer close(gotLines)
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for sc.Scan() {
			text := sc.Text()
			if strings.HasPrefix(text, "data: ") {
				gotLines <- strings.TrimPrefix(text, "data: ")
			}
		}
	}()

	// Verify seed lines arrive first.
	expectLine(t, gotLines, "seed-1")
	expectLine(t, gotLines, "seed-2")

	// Append new content; verify it streams.
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open append: %v", err)
	}
	if _, err := f.WriteString("appended-1\nappended-2\n"); err != nil {
		t.Fatalf("write append: %v", err)
	}
	_ = f.Close()

	expectLine(t, gotLines, "appended-1")
	expectLine(t, gotLines, "appended-2")

	cancel() // close stream
}

func expectLine(t *testing.T, ch <-chan string, want string) {
	t.Helper()
	select {
	case got, ok := <-ch:
		if !ok {
			t.Fatalf("stream closed before line %q arrived", want)
		}
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("timeout waiting for line %q", want)
	}
}

func TestInstanceLogsStreamWaitsForFile(t *testing.T) {
	_, mux, dir := setupLogsServer(t)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET",
		srv.URL+"/api/v1/topology/projects/p/instances/sim-aws/logs?follow=1&lines=10", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()

	gotLines := make(chan string, 16)
	go func() {
		defer close(gotLines)
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			text := sc.Text()
			if strings.HasPrefix(text, "data: ") {
				gotLines <- strings.TrimPrefix(text, "data: ")
			}
		}
	}()

	// Create the log file after the stream has been waiting.
	time.Sleep(400 * time.Millisecond)
	writeLogFile(t, dir, "sim-aws", "first-line\n")
	expectLine(t, gotLines, "first-line")
	cancel()
}

func TestParseLinesParam(t *testing.T) {
	cases := []struct {
		raw  string
		def  int
		want int
	}{
		{"", 200, 200},
		{"50", 200, 50},
		{"0", 200, 0},
		{"-5", 200, 200},
		{"abc", 200, 200},
		{"99999", 200, 10000},
	}
	for _, tc := range cases {
		if got := parseLinesParam(tc.raw, tc.def); got != tc.want {
			t.Errorf("parseLinesParam(%q, %d) = %d, want %d", tc.raw, tc.def, got, tc.want)
		}
	}
}

func TestTailLines(t *testing.T) {
	if got := tailLines(nil, 5); len(got) != 0 {
		t.Errorf("nil → %v", got)
	}
	if got := tailLines([]byte("a\nb\nc\n"), 2); len(got) != 2 || got[0] != "b" || got[1] != "c" {
		t.Errorf("trailing newline: %v", got)
	}
	if got := tailLines([]byte("a\nb\nc"), 5); len(got) != 3 {
		t.Errorf("no trailing newline: %v", got)
	}
}

func TestEscapeSSELine(t *testing.T) {
	if got := escapeSSELine("plain"); got != "plain" {
		t.Errorf("plain unchanged: %q", got)
	}
	if got := escapeSSELine("a\nb\rc"); got != "a b c" {
		t.Errorf("newlines collapsed: %q", got)
	}
}
