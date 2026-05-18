package main

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseExecEnvelope(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		wantOK bool
	}{
		{name: "empty", body: "", wantOK: false},
		{name: "raw", body: "hello", wantOK: false},
		{name: "missing argv", body: `{"sockerless":{"exec":{"argv":[]}}}`, wantOK: false},
		{name: "valid", body: `{"sockerless":{"exec":{"argv":["echo","hi"]}}}`, wantOK: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := parseExecEnvelope([]byte(tt.body))
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
		})
	}
}

func TestRunExecEnvelope_StdinPiped(t *testing.T) {
	env := execEnvelopeExec{
		Argv:  []string{"sh", "-c", "cat"},
		Stdin: base64.StdEncoding.EncodeToString([]byte("azf-stdin-ok\n")),
	}
	w := httptest.NewRecorder()
	runExecEnvelope(w, httptest.NewRequest("POST", "/", nil).Context(), env)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%q", w.Code, w.Body.String())
	}
	var res execEnvelopeResponse
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if res.SockerlessExecResult.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0", res.SockerlessExecResult.ExitCode)
	}
	stdout, err := base64.StdEncoding.DecodeString(res.SockerlessExecResult.Stdout)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(stdout), "azf-stdin-ok") {
		t.Fatalf("stdout = %q, want stdin echo", string(stdout))
	}
}

func TestRunExecEnvelope_EnvAndWorkdir(t *testing.T) {
	dir := canonicalTempDir(t)
	env := execEnvelopeExec{
		Argv:    []string{"sh", "-c", "printf '%s:%s' \"$AZF_TEST_VALUE\" \"$(pwd)\""},
		Workdir: dir,
		Env:     []string{"AZF_TEST_VALUE=from-env"},
	}
	w := httptest.NewRecorder()
	runExecEnvelope(w, httptest.NewRequest("POST", "/", nil).Context(), env)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%q", w.Code, w.Body.String())
	}
	var res execEnvelopeResponse
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if res.SockerlessExecResult.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0", res.SockerlessExecResult.ExitCode)
	}
	stdout, err := base64.StdEncoding.DecodeString(res.SockerlessExecResult.Stdout)
	if err != nil {
		t.Fatal(err)
	}
	want := "from-env:" + dir
	if string(stdout) != want {
		t.Fatalf("stdout = %q, want %q", string(stdout), want)
	}
}

func TestRunExecEnvelope_InvalidStdin(t *testing.T) {
	env := execEnvelopeExec{
		Argv:  []string{"sh", "-c", "cat"},
		Stdin: "not-base64",
	}
	w := httptest.NewRecorder()
	runExecEnvelope(w, httptest.NewRequest("POST", "/", nil).Context(), env)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "stdin base64 decode") {
		t.Fatalf("body = %q, want decode error", w.Body.String())
	}
}

func TestHandleInvoke_DefaultUserCommand(t *testing.T) {
	t.Setenv(envUserEntrypoint, "")
	t.Setenv(envUserCmd, encodeArgvForTest(t, []string{"sh", "-c", "printf azf-default"}))

	w := httptest.NewRecorder()
	handleInvoke(w, httptest.NewRequest("POST", "/api/function", strings.NewReader("{}")))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%q", w.Code, w.Body.String())
	}
	if got := w.Header().Get("X-Sockerless-Exit-Code"); got != "0" {
		t.Fatalf("exit header = %q, want 0", got)
	}
	if w.Body.String() != "azf-default" {
		t.Fatalf("body = %q, want azf-default", w.Body.String())
	}
}

func TestHandleInvoke_DefaultWorkdir(t *testing.T) {
	dir := canonicalTempDir(t)
	t.Setenv(envUserEntrypoint, "")
	t.Setenv(envUserCmd, encodeArgvForTest(t, []string{"sh", "-c", "pwd"}))
	t.Setenv(envUserWorkdir, dir)

	w := httptest.NewRecorder()
	handleInvoke(w, httptest.NewRequest("POST", "/", strings.NewReader("")))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%q", w.Code, w.Body.String())
	}
	if strings.TrimSpace(w.Body.String()) != filepath.Clean(dir) {
		t.Fatalf("body = %q, want %q", w.Body.String(), dir)
	}
}

func TestJobTimeout(t *testing.T) {
	t.Setenv(envJobTimeout, "")
	if got := jobTimeout(); got != defaultJobTimeoutSeconds*time.Second {
		t.Fatalf("default timeout = %v", got)
	}
	t.Setenv(envJobTimeout, "2")
	if got := jobTimeout(); got != 2*time.Second {
		t.Fatalf("timeout = %v, want 2s", got)
	}
	t.Setenv(envJobTimeout, "0")
	if got := jobTimeout(); got != 0 {
		t.Fatalf("timeout = %v, want disabled", got)
	}
	t.Setenv(envJobTimeout, "not-an-int")
	if got := jobTimeout(); got != 0 {
		t.Fatalf("timeout = %v, want disabled", got)
	}
}

func TestDecodeArgvEnvErrors(t *testing.T) {
	t.Setenv(envUserCmd, "not-base64")
	if _, err := decodeArgvEnv(envUserCmd); err == nil || !strings.Contains(err.Error(), "not valid base64") {
		t.Fatalf("decodeArgvEnv invalid base64 error = %v", err)
	}
	t.Setenv(envUserCmd, base64.StdEncoding.EncodeToString([]byte(`{"not":"argv"}`)))
	if _, err := decodeArgvEnv(envUserCmd); err == nil || !strings.Contains(err.Error(), "not JSON argv") {
		t.Fatalf("decodeArgvEnv invalid JSON error = %v", err)
	}
}

func encodeArgvForTest(t *testing.T, argv []string) string {
	t.Helper()
	raw, err := json.Marshal(argv)
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(raw)
}

func canonicalTempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}
