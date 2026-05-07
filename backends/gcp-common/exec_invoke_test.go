package gcpcommon

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPostExecEnvelope_RoundTrip(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("missing/wrong bearer header: %q", r.Header.Get("Authorization"))
		}
		var req ExecEnvelopeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode envelope: %v", err)
		}
		if len(req.Sockerless.Exec.Argv) != 2 || req.Sockerless.Exec.Argv[0] != "echo" {
			t.Errorf("unexpected argv: %v", req.Sockerless.Exec.Argv)
		}
		var resp ExecEnvelopeResponse
		resp.SockerlessExecResult.ExitCode = 0
		resp.SockerlessExecResult.Stdout = base64.StdEncoding.EncodeToString([]byte("envelope-roundtrip\n"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	res, err := PostExecEnvelope(context.Background(), nil, server.URL, "test-token", ExecEnvelopeExec{
		Argv: []string{"echo", "hi"},
	})
	if err != nil {
		t.Fatalf("PostExecEnvelope: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("exitCode = %d, want 0", res.ExitCode)
	}
	if !strings.Contains(string(res.Stdout), "envelope-roundtrip") {
		t.Fatalf("stdout = %q", string(res.Stdout))
	}
}

func TestPostExecEnvelope_StdinPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ExecEnvelopeRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		stdin, _ := base64.StdEncoding.DecodeString(req.Sockerless.Exec.Stdin)
		var resp ExecEnvelopeResponse
		resp.SockerlessExecResult.Stdout = base64.StdEncoding.EncodeToString(stdin) // echo back
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	res, err := PostExecEnvelope(context.Background(), nil, server.URL, "", ExecEnvelopeExec{
		Argv:  []string{"cat"},
		Stdin: EncodeStdin([]byte("input-bytes")),
	})
	if err != nil {
		t.Fatalf("PostExecEnvelope: %v", err)
	}
	if string(res.Stdout) != "input-bytes" {
		t.Fatalf("stdout = %q, want input-bytes", string(res.Stdout))
	}
}

func TestPostExecEnvelope_NonZeroExit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var resp ExecEnvelopeResponse
		resp.SockerlessExecResult.ExitCode = 17
		resp.SockerlessExecResult.Stderr = base64.StdEncoding.EncodeToString([]byte("boom"))
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	res, err := PostExecEnvelope(context.Background(), nil, server.URL, "", ExecEnvelopeExec{
		Argv: []string{"sh", "-c", "exit 17"},
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.ExitCode != 17 || string(res.Stderr) != "boom" {
		t.Fatalf("exit=%d stderr=%q", res.ExitCode, string(res.Stderr))
	}
}

func TestPostExecEnvelope_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("upstream connect failed"))
	}))
	defer server.Close()

	_, err := PostExecEnvelope(context.Background(), nil, server.URL, "", ExecEnvelopeExec{
		Argv: []string{"echo", "hi"},
	})
	if err == nil {
		t.Fatal("expected error on 502, got nil")
	}
	if !strings.Contains(err.Error(), "502") {
		t.Fatalf("err should mention status: %v", err)
	}
}

func TestPostExecEnvelope_InputValidation(t *testing.T) {
	if _, err := PostExecEnvelope(context.Background(), nil, "", "", ExecEnvelopeExec{Argv: []string{"x"}}); err == nil {
		t.Fatal("expected error on empty url")
	}
	if _, err := PostExecEnvelope(context.Background(), nil, "http://example.invalid", "", ExecEnvelopeExec{}); err == nil {
		t.Fatal("expected error on empty argv")
	}
}
