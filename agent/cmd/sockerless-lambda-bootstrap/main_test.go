package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// encodeArgv matches the backend's encoding: base64(JSON(argv)).
func encodeArgv(argv []string) string {
	b, _ := json.Marshal(argv)
	return base64.StdEncoding.EncodeToString(b)
}

// TestHandleOneInvocation_RoundTrip verifies the Runtime-API loop
// (single iteration) polls /next, runs the user entrypoint with the
// payload on stdin, and posts /response with the stdout.
func TestHandleOneInvocation_RoundTrip(t *testing.T) {
	var posted atomic.Value
	var gotBody atomic.Value
	var nextCount int32

	mux := http.NewServeMux()
	mux.HandleFunc("/2018-06-01/runtime/invocation/next", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&nextCount, 1)
		w.Header().Set("Lambda-Runtime-Aws-Request-Id", "req-1")
		w.Header().Set("Lambda-Runtime-Invoked-Function-Arn", "arn:aws:lambda:us-east-1:000000000000:function:test")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"echo":"hello"}`))
	})
	mux.HandleFunc("/2018-06-01/runtime/invocation/req-1/response", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		posted.Store("response")
		gotBody.Store(string(body))
		w.WriteHeader(http.StatusAccepted)
	})
	mux.HandleFunc("/2018-06-01/runtime/invocation/req-1/error", func(w http.ResponseWriter, r *http.Request) {
		posted.Store("error")
		w.WriteHeader(http.StatusAccepted)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Configure a user entrypoint that echoes stdin to stdout.
	t.Setenv(envUserEntrypoint, encodeArgv([]string{"/bin/cat"}))
	t.Setenv(envUserCmd, "")

	if err := handleOneInvocation(srv.URL); err != nil {
		t.Fatalf("handleOneInvocation: %v", err)
	}

	if nextCount != 1 {
		t.Errorf("/next should be called once, got %d", nextCount)
	}
	if p := posted.Load(); p != "response" {
		t.Errorf("want /response, got %v", p)
	}
	if b := gotBody.Load(); b == nil || !strings.Contains(b.(string), `"echo":"hello"`) {
		t.Errorf("want echoed payload, got %v", b)
	}
}

// TestHandleOneInvocation_UserError verifies that a non-zero user exit
// code posts to /error with an errorMessage envelope.
func TestHandleOneInvocation_UserError(t *testing.T) {
	var posted atomic.Value
	var gotBody atomic.Value

	mux := http.NewServeMux()
	mux.HandleFunc("/2018-06-01/runtime/invocation/next", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Lambda-Runtime-Aws-Request-Id", "req-err")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	})
	mux.HandleFunc("/2018-06-01/runtime/invocation/req-err/response", func(w http.ResponseWriter, r *http.Request) {
		posted.Store("response")
		w.WriteHeader(http.StatusAccepted)
	})
	mux.HandleFunc("/2018-06-01/runtime/invocation/req-err/error", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		posted.Store("error")
		gotBody.Store(string(body))
		w.WriteHeader(http.StatusAccepted)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	t.Setenv(envUserEntrypoint, encodeArgv([]string{"/bin/sh"}))
	t.Setenv(envUserCmd, encodeArgv([]string{"-c", "exit 7"}))

	if err := handleOneInvocation(srv.URL); err != nil {
		t.Fatalf("handleOneInvocation: %v", err)
	}

	if p := posted.Load(); p != "error" {
		t.Errorf("want /error, got %v", p)
	}
	b, _ := gotBody.Load().(string)
	if !strings.Contains(b, "errorMessage") {
		t.Errorf("want errorMessage in body, got %q", b)
	}
}

// TestRunUserInvocation_NoEntrypoint verifies the "echo payload"
// fallback when no user entrypoint is configured (matches the
// testdata handler's semantics).
func TestRunUserInvocation_NoEntrypoint(t *testing.T) {
	t.Setenv(envUserEntrypoint, "")
	t.Setenv(envUserCmd, "")

	stdout, _, exit := runUserInvocation(context.Background(), []byte(`{"k":"v"}`))
	if exit != 0 {
		t.Errorf("want exit 0, got %d", exit)
	}
	if !bytes.Equal(stdout, []byte(`{"k":"v"}`)) {
		t.Errorf("want payload echoed, got %q", string(stdout))
	}
}

// TestBuildErrorPayload verifies the error envelope shape.
func TestBuildErrorPayload(t *testing.T) {
	body := buildErrorPayload([]byte("boom\n"), 5)
	s := string(body)
	if !strings.Contains(s, `"errorMessage":"boom"`) {
		t.Errorf("missing stderr message: %s", s)
	}
	if !strings.Contains(s, `"errorType":"HandlerError"`) {
		t.Errorf("missing errorType: %s", s)
	}
}

// TestBuildErrorPayload_EmptyStderr verifies the fallback message when
// the user process exits non-zero without writing to stderr.
func TestBuildErrorPayload_EmptyStderr(t *testing.T) {
	body := buildErrorPayload(nil, 3)
	s := string(body)
	if !strings.Contains(s, `user process exited 3`) {
		t.Errorf("want fallback message, got %s", s)
	}
}

// TestPostInitError verifies the init-error envelope hits the right
// Runtime-API endpoint.
func TestPostInitError(t *testing.T) {
	var called atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/2018-06-01/runtime/init/error", func(w http.ResponseWriter, r *http.Request) {
		called.Add(1)
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"errorType":"InitError"`) {
			t.Errorf("want InitError type, got %s", string(body))
		}
		w.WriteHeader(http.StatusAccepted)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	postInitError(srv.URL, "dial failed: boom")

	if called.Load() != 1 {
		t.Errorf("init/error should be hit once, got %d", called.Load())
	}
}
