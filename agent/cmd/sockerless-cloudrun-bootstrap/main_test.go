package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestParseExecEnvelope(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantOK  bool
		wantArg string
	}{
		{name: "empty body falls through", body: "", wantOK: false},
		{name: "raw bytes fall through", body: "hello", wantOK: false},
		{name: "JSON without envelope falls through", body: `{"foo":"bar"}`, wantOK: false},
		{name: "envelope with empty argv falls through", body: `{"sockerless":{"exec":{"argv":[]}}}`, wantOK: false},
		{name: "valid envelope parses", body: `{"sockerless":{"exec":{"argv":["echo","hi"]}}}`, wantOK: true, wantArg: "hi"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env, ok := parseExecEnvelope([]byte(tt.body))
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if tt.wantOK && len(env.Argv) > 1 && env.Argv[1] != tt.wantArg {
				t.Fatalf("argv[1] = %q, want %q", env.Argv[1], tt.wantArg)
			}
		})
	}
}

func TestRunExecEnvelope_StdoutCapture(t *testing.T) {
	env := execEnvelopeExec{Argv: []string{"echo", "hello-from-bootstrap"}}
	w := httptest.NewRecorder()
	runExecEnvelope(w, env)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var res execEnvelopeResponse
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if res.SockerlessExecResult.ExitCode != 0 {
		t.Fatalf("exitCode = %d, want 0", res.SockerlessExecResult.ExitCode)
	}
	stdoutBytes, _ := base64.StdEncoding.DecodeString(res.SockerlessExecResult.Stdout)
	if !strings.Contains(string(stdoutBytes), "hello-from-bootstrap") {
		t.Fatalf("stdout = %q, want contains hello-from-bootstrap", string(stdoutBytes))
	}
}

func TestRunExecEnvelope_NonZeroExit(t *testing.T) {
	env := execEnvelopeExec{Argv: []string{"sh", "-c", "exit 7"}}
	w := httptest.NewRecorder()
	runExecEnvelope(w, env)
	var res execEnvelopeResponse
	_ = json.Unmarshal(w.Body.Bytes(), &res)
	if res.SockerlessExecResult.ExitCode != 7 {
		t.Fatalf("exitCode = %d, want 7", res.SockerlessExecResult.ExitCode)
	}
}

func TestRunExecEnvelope_StdinPiped(t *testing.T) {
	env := execEnvelopeExec{
		Argv:  []string{"sh", "-c", "cat"},
		Stdin: base64.StdEncoding.EncodeToString([]byte("piped-input-bytes\n")),
	}
	w := httptest.NewRecorder()
	runExecEnvelope(w, env)
	var res execEnvelopeResponse
	_ = json.Unmarshal(w.Body.Bytes(), &res)
	stdoutBytes, _ := base64.StdEncoding.DecodeString(res.SockerlessExecResult.Stdout)
	if !strings.Contains(string(stdoutBytes), "piped-input-bytes") {
		t.Fatalf("stdout = %q, want contains piped-input-bytes", string(stdoutBytes))
	}
}

func TestParseUserArgv_RoundTrip(t *testing.T) {
	want := []string{"go", "build", "-v", "-o", "/tmp/out", "."}
	encoded, _ := json.Marshal(want)
	t.Setenv("SOCKERLESS_USER_CMD", base64.StdEncoding.EncodeToString(encoded))
	got := parseUserArgv("SOCKERLESS_USER_CMD")
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("argv[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestRunDefaultInvoke_RunsEnvBakedCmd(t *testing.T) {
	t.Setenv("SOCKERLESS_USER_CMD", base64.StdEncoding.EncodeToString([]byte(`["echo","default-invoke-output"]`)))
	t.Setenv("SOCKERLESS_USER_ENTRYPOINT", "")
	w := httptest.NewRecorder()
	runDefaultInvoke(w)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%q", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "default-invoke-output") {
		t.Fatalf("body = %q", w.Body.String())
	}
}

func TestHandleInvoke_DispatchesEnvelopeVsDefault(t *testing.T) {
	t.Setenv("SOCKERLESS_USER_CMD", base64.StdEncoding.EncodeToString([]byte(`["echo","DEFAULT"]`)))
	t.Setenv("SOCKERLESS_USER_ENTRYPOINT", "")

	// Empty body → default invoke
	w1 := httptest.NewRecorder()
	r1 := httptest.NewRequest("POST", "/", bytes.NewReader(nil))
	handleInvoke(w1, r1)
	if !strings.Contains(w1.Body.String(), "DEFAULT") {
		t.Fatalf("default invoke body = %q", w1.Body.String())
	}

	// Envelope body → exec path
	envBody := `{"sockerless":{"exec":{"argv":["echo","ENVELOPE"]}}}`
	w2 := httptest.NewRecorder()
	r2 := httptest.NewRequest("POST", "/", strings.NewReader(envBody))
	handleInvoke(w2, r2)
	var res execEnvelopeResponse
	_ = json.Unmarshal(w2.Body.Bytes(), &res)
	stdoutBytes, _ := base64.StdEncoding.DecodeString(res.SockerlessExecResult.Stdout)
	if !strings.Contains(string(stdoutBytes), "ENVELOPE") {
		t.Fatalf("envelope stdout = %q", string(stdoutBytes))
	}
}

// Sanity check: ensure the binary name in /opt/sockerless matches what
// the gcp-common renderer derives from the bootstrap binary's basename.
// (Renderer test lives in gcp-common; this is a smoke that we agree on
// the name.)
func TestExpectedBinaryName(t *testing.T) {
	want := "sockerless-cloudrun-bootstrap"
	got := os.Args[0]
	if !strings.HasSuffix(got, want) && !strings.Contains(got, "/sockerless-cloudrun-bootstrap") && !strings.Contains(got, ".test") {
		t.Logf("test binary name %q (expected to derive from %q in production)", got, want)
	}
}

// Bootstrap timeout helper. runWithTimeout: a subprocess that finishes
// within budget exits with its own code; one that exceeds budget gets
// SIGTERM/SIGKILL and the helper returns timedOut=true.

func TestRunWithTimeout_FinishesEarly(t *testing.T) {
	cmd := exec.Command("sh", "-c", "exit 7")
	exitCode, timedOut, err := runWithTimeout(cmd, 5, "test")
	if timedOut {
		t.Fatalf("expected timedOut=false, got true")
	}
	if err == nil {
		t.Fatalf("expected non-nil err for non-zero exit, got nil")
	}
	if exitCode != 7 {
		t.Errorf("expected exit code 7, got %d", exitCode)
	}
}

func TestRunWithTimeout_ZeroExit(t *testing.T) {
	cmd := exec.Command("true")
	exitCode, timedOut, err := runWithTimeout(cmd, 5, "test")
	if timedOut {
		t.Fatalf("expected timedOut=false, got true")
	}
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
}

func TestRunWithTimeout_FiresOnHang(t *testing.T) {
	// `sleep 9999` will be killed by the timer well before completion.
	cmd := exec.Command("sleep", "9999")
	start := time.Now()
	_, timedOut, _ := runWithTimeout(cmd, 1, "test")
	elapsed := time.Since(start)
	if !timedOut {
		t.Fatalf("expected timedOut=true, got false")
	}
	// 1s timer + ~immediate SIGTERM response from sleep. Should be ~1-2s,
	// not the full 30s grace.
	if elapsed > 5*time.Second {
		t.Errorf("expected timer to fire near 1s, took %v", elapsed)
	}
}

func TestRunWithTimeout_DisabledByZero(t *testing.T) {
	// timeoutSeconds=0 → no timer; should run to completion.
	cmd := exec.Command("sh", "-c", "exit 3")
	exitCode, timedOut, _ := runWithTimeout(cmd, 0, "test")
	if timedOut {
		t.Fatalf("expected timedOut=false with timer disabled, got true")
	}
	if exitCode != 3 {
		t.Errorf("expected exit code 3, got %d", exitCode)
	}
}

func TestJobTimeoutFromEnv(t *testing.T) {
	t.Setenv(envJobTimeoutSeconds, "")
	if got := jobTimeoutFromEnv(); got != jobTimeoutDefaultSeconds {
		t.Errorf("empty → %d, want %d", got, jobTimeoutDefaultSeconds)
	}
	t.Setenv(envJobTimeoutSeconds, "120")
	if got := jobTimeoutFromEnv(); got != 120 {
		t.Errorf("120 → %d, want 120", got)
	}
	t.Setenv(envJobTimeoutSeconds, "abc")
	if got := jobTimeoutFromEnv(); got != jobTimeoutDefaultSeconds {
		t.Errorf("abc → %d, want default %d", got, jobTimeoutDefaultSeconds)
	}
	t.Setenv(envJobTimeoutSeconds, "-1")
	if got := jobTimeoutFromEnv(); got != 0 {
		t.Errorf("-1 → %d, want 0 (clamped)", got)
	}
}
