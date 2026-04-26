package core

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/sockerless/api"
)

// stubLegacyLogs records what the adapter handed to the legacy fn so
// the assertions can verify the dctx→narrow-arg projection.
type stubLegacyLogs struct {
	gotRef    string
	gotOpts   api.ContainerLogsOptions
	returnRC  io.ReadCloser
	returnErr error
}

func (s *stubLegacyLogs) fn(ref string, opts api.ContainerLogsOptions) (io.ReadCloser, error) {
	s.gotRef = ref
	s.gotOpts = opts
	return s.returnRC, s.returnErr
}

func TestWrapLegacyLogs_DelegatesAndPropagatesError(t *testing.T) {
	stub := &stubLegacyLogs{returnRC: io.NopCloser(strings.NewReader("hello"))}
	wrapped := WrapLegacyLogs(stub.fn, "ecs", "CloudWatchLogs")

	dctx := DriverContext{Container: api.Container{ID: "abc"}}
	rc, err := wrapped.Logs(dctx, api.ContainerLogsOptions{ShowStdout: true, Tail: "10"})
	if err != nil {
		t.Fatalf("Logs: unexpected error %v", err)
	}
	defer rc.Close()
	if stub.gotRef != "abc" {
		t.Errorf("ref: got %q, want abc", stub.gotRef)
	}
	if !stub.gotOpts.ShowStdout || stub.gotOpts.Tail != "10" {
		t.Errorf("opts: not propagated, got %+v", stub.gotOpts)
	}

	stub.returnErr = errors.New("logs failed")
	stub.returnRC = nil
	if _, err := wrapped.Logs(dctx, api.ContainerLogsOptions{}); err == nil {
		t.Fatal("expected error to propagate from legacy fn")
	}
}

func TestWrapLegacyLogs_Describe(t *testing.T) {
	stub := &stubLegacyLogs{}
	got := WrapLegacyLogs(stub.fn, "lambda", "CloudWatchLogs").Describe()
	if !strings.Contains(got, "lambda") || !strings.Contains(got, "CloudWatchLogs") {
		t.Errorf("Describe should name backend + impl, got %q", got)
	}
	if WrapLegacyLogs(stub.fn, "", "").Describe() == "" {
		t.Errorf("Describe should never return empty")
	}
}

func TestWrapLegacyLogs_NilFn_ReturnsError(t *testing.T) {
	wrapped := WrapLegacyLogs(nil, "ecs", "")
	_, err := wrapped.Logs(DriverContext{}, api.ContainerLogsOptions{})
	if err == nil || !strings.Contains(err.Error(), "function is nil") {
		t.Errorf("expected nil-fn error, got %v", err)
	}
}

func TestNewCloudLogsLogsDriver_Describe(t *testing.T) {
	d := NewCloudLogsLogsDriver(nil, nil, StreamCloudLogsOptions{}, "lambda", "CloudWatchStream")
	got := d.Describe()
	if !strings.Contains(got, "lambda") || !strings.Contains(got, "CloudWatchStream") {
		t.Errorf("Describe: got %q", got)
	}
}

func TestNewCloudLogsLogsDriver_NilServer_ReturnsError(t *testing.T) {
	d := NewCloudLogsLogsDriver(nil, nil, StreamCloudLogsOptions{}, "lambda", "")
	_, err := d.Logs(DriverContext{}, api.ContainerLogsOptions{})
	if err == nil || !strings.Contains(err.Error(), "server / fetch is nil") {
		t.Errorf("expected nil-server/fetch error, got %v", err)
	}
}
