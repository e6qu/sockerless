package core

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"

	"github.com/sockerless/api"
)

// stubNarrowStream is a minimal narrow-StreamDriver impl for the
// adapter tests. The "stub" prefix mirrors the convention from
// `driver_adapt_exec_test.go` — explicit test-only naming so the
// scaffolding is never confused for a production fallback.
type stubNarrowStream struct {
	gotContainerID string
	gotTTY         bool
	returnErr      error
}

func (s *stubNarrowStream) Attach(_ context.Context, containerID string, tty bool, _ net.Conn) error {
	s.gotContainerID = containerID
	s.gotTTY = tty
	return s.returnErr
}

func (s *stubNarrowStream) LogBytes(_ string) []byte             { return nil }
func (s *stubNarrowStream) LogSubscribe(_, _ string) chan []byte { return nil }
func (s *stubNarrowStream) LogUnsubscribe(_, _ string)           {}

func TestWrapLegacyAttach_DelegatesAndPropagatesError(t *testing.T) {
	narrow := &stubNarrowStream{}
	wrapped := WrapLegacyAttach(narrow, "ecs", "ECSAttach")

	dctx := DriverContext{Container: api.Container{ID: "abc"}, Ctx: context.Background()}
	if err := wrapped.Attach(dctx, true, &fakeNetConn{}); err != nil {
		t.Fatalf("Attach: unexpected error %v", err)
	}
	if narrow.gotContainerID != "abc" {
		t.Errorf("containerID: got %q, want abc", narrow.gotContainerID)
	}
	if !narrow.gotTTY {
		t.Errorf("tty: not propagated")
	}

	// Error from narrow driver should propagate.
	narrow.returnErr = errors.New("attach failed")
	if err := wrapped.Attach(dctx, false, &fakeNetConn{}); err == nil {
		t.Fatal("expected error to propagate from narrow driver")
	}
}

func TestWrapLegacyAttach_Describe(t *testing.T) {
	got := WrapLegacyAttach(&stubNarrowStream{}, "lambda", "ReverseAgentAttach").Describe()
	if !strings.Contains(got, "lambda") || !strings.Contains(got, "ReverseAgentAttach") {
		t.Errorf("Describe should name backend + impl, got %q", got)
	}
	if WrapLegacyAttach(&stubNarrowStream{}, "", "").Describe() == "" {
		t.Errorf("Describe should never return empty")
	}
}

func TestWrapLegacyAttach_NilNarrow_ReturnsError(t *testing.T) {
	wrapped := WrapLegacyAttach(nil, "ecs", "")
	err := wrapped.Attach(DriverContext{}, false, &fakeNetConn{})
	if err == nil || !strings.Contains(err.Error(), "narrow driver is nil") {
		t.Errorf("expected nil-narrow error, got %v", err)
	}
}

func TestWrapLegacyAttach_NonNetConn_ReturnsError(t *testing.T) {
	wrapped := WrapLegacyAttach(&stubNarrowStream{}, "docker", "DockerAttach")
	type rw struct {
		fakeReader
		fakeWriter
	}
	err := wrapped.Attach(DriverContext{}, false, &rw{})
	if err == nil || !strings.Contains(err.Error(), "net.Conn") {
		t.Errorf("expected net.Conn-required error, got %v", err)
	}
}

func TestNewCloudLogsAttachDriver_Describe(t *testing.T) {
	d := NewCloudLogsAttachDriver(nil, nil, "lambda", "CloudLogsReadOnlyAttach")
	got := d.Describe()
	if !strings.Contains(got, "lambda") || !strings.Contains(got, "CloudLogsReadOnlyAttach") {
		t.Errorf("Describe: got %q", got)
	}
}

func TestNewCloudLogsAttachDriver_NilServer_ReturnsError(t *testing.T) {
	d := NewCloudLogsAttachDriver(nil, nil, "lambda", "")
	err := d.Attach(DriverContext{}, false, &fakeNetConn{})
	if err == nil || !strings.Contains(err.Error(), "server / fetch is nil") {
		t.Errorf("expected nil-server/fetch error, got %v", err)
	}
}
