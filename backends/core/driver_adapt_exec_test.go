package core

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"

	"github.com/sockerless/api"
)

// stubNarrowExec is a minimal narrow-ExecDriver impl for the adapter
// tests. The "stub" prefix is intentional and explicit — this is
// test scaffolding only; production code paths never wrap a stub.
type stubNarrowExec struct {
	gotContainerID string
	gotExecID      string
	gotCmd         []string
	gotEnv         []string
	gotWorkDir     string
	gotTTY         bool
	exitCode       int
}

func (s *stubNarrowExec) Exec(_ context.Context, containerID, execID string,
	cmd []string, env []string, workDir string, tty bool, _ net.Conn) int {
	s.gotContainerID = containerID
	s.gotExecID = execID
	s.gotCmd = cmd
	s.gotEnv = env
	s.gotWorkDir = workDir
	s.gotTTY = tty
	return s.exitCode
}

// fakeNetConn satisfies net.Conn enough for the adapter's type
// assertion. None of the methods are exercised in the adapter path —
// the narrow driver's stub above ignores the conn — but the
// interface check itself must pass.
type fakeNetConn struct{ net.Conn }

func TestWrapLegacyExec_DelegatesAndReturnsExit(t *testing.T) {
	narrow := &stubNarrowExec{exitCode: 7}
	wrapped := WrapLegacyExec(narrow, "ecs", "SSMExec")

	dctx := DriverContext{
		Ctx:       context.Background(),
		Container: api.Container{ID: "abcdef"},
	}
	opts := ExecOptions{
		ExecID:  "exec-1",
		Cmd:     []string{"echo", "hi"},
		Env:     []string{"FOO=bar"},
		WorkDir: "/work",
		TTY:     true,
	}

	exit, err := wrapped.Exec(dctx, opts, &fakeNetConn{})
	if err != nil {
		t.Fatalf("Exec: unexpected error %v", err)
	}
	if exit != 7 {
		t.Errorf("exit code: got %d, want 7", exit)
	}
	if narrow.gotContainerID != "abcdef" {
		t.Errorf("containerID: got %q", narrow.gotContainerID)
	}
	if narrow.gotExecID != "exec-1" {
		t.Errorf("execID: got %q", narrow.gotExecID)
	}
	if len(narrow.gotCmd) != 2 || narrow.gotCmd[1] != "hi" {
		t.Errorf("cmd: got %v", narrow.gotCmd)
	}
	if !narrow.gotTTY {
		t.Errorf("tty: not propagated")
	}
}

func TestWrapLegacyExec_Describe(t *testing.T) {
	got := WrapLegacyExec(&stubNarrowExec{}, "ecs", "SSMExec").Describe()
	if !strings.Contains(got, "ecs") || !strings.Contains(got, "SSMExec") {
		t.Errorf("Describe should name backend + impl, got %q", got)
	}

	// Empty backend/impl falls through to a generic descriptor —
	// still honest, just less specific.
	got = WrapLegacyExec(&stubNarrowExec{}, "", "").Describe()
	if got == "" {
		t.Errorf("Describe should never return empty")
	}
}

func TestWrapLegacyExec_NilNarrow_ReturnsError(t *testing.T) {
	wrapped := WrapLegacyExec(nil, "ecs", "")
	exit, err := wrapped.Exec(DriverContext{}, ExecOptions{}, &fakeNetConn{})
	if err == nil {
		t.Fatal("expected error when narrow driver is nil")
	}
	if exit != -1 {
		t.Errorf("nil-narrow exit: got %d, want -1", exit)
	}
	if !strings.Contains(err.Error(), "narrow driver is nil") {
		t.Errorf("error should mention nil narrow driver, got %q", err.Error())
	}
}

func TestWrapLegacyExec_NonNetConn_ReturnsError(t *testing.T) {
	wrapped := WrapLegacyExec(&stubNarrowExec{}, "docker", "DockerExec")
	// Use a plain pipe end (io.ReadWriter, not net.Conn).
	r, w := errClosingPipe{}, errClosingPipe{}
	_ = r
	_ = w
	type rw struct {
		fakeReader
		fakeWriter
	}
	exit, err := wrapped.Exec(DriverContext{}, ExecOptions{}, &rw{})
	if err == nil {
		t.Fatal("expected error when caller passes non-Conn ReadWriter")
	}
	if exit != -1 {
		t.Errorf("non-Conn exit: got %d, want -1", exit)
	}
	if !strings.Contains(err.Error(), "net.Conn") {
		t.Errorf("error should mention net.Conn requirement, got %q", err.Error())
	}
}

// fakeReader / fakeWriter / errClosingPipe satisfy io.Reader /
// io.Writer / io.Closer for the non-Conn-ReadWriter test above. Naming
// `fakeXxx` mirrors the in-tree convention from `aca/azure.go`'s
// `fakeCredential` (test-only — this never appears in a production
// path).
type fakeReader struct{}

func (fakeReader) Read(_ []byte) (int, error) { return 0, errors.New("fakeReader") }

type fakeWriter struct{}

func (fakeWriter) Write(p []byte) (int, error) { return len(p), nil }

type errClosingPipe struct{}

func (errClosingPipe) Read(_ []byte) (int, error)  { return 0, errors.New("closed") }
func (errClosingPipe) Write(p []byte) (int, error) { return len(p), nil }
