package core

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/sockerless/api"
)

func TestStreamCloudLogs_NotFound(t *testing.T) {
	s := newSpecTestServer()
	s.CloudState = &mockCloudState{} // empty

	fetch := func(_ context.Context, _ CloudLogParams, _ any) ([]CloudLogEntry, any, error) {
		t.Fatal("fetch should not be called when container not found")
		return nil, nil, nil
	}

	_, err := StreamCloudLogs(s, "nonexistent", api.ContainerLogsOptions{ShowStdout: true}, fetch, StreamCloudLogsOptions{})
	if err == nil {
		t.Fatal("expected error for missing container")
	}
	if _, ok := err.(*api.NotFoundError); !ok {
		t.Errorf("expected NotFoundError, got %T: %v", err, err)
	}
}

func TestStreamCloudLogs_CreatedState(t *testing.T) {
	s := newSpecTestServer()

	id, err := createContainerInState(s, "created")
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	fetch := func(_ context.Context, _ CloudLogParams, _ any) ([]CloudLogEntry, any, error) {
		t.Fatal("fetch should not be called for created container")
		return nil, nil, nil
	}

	_, err = StreamCloudLogs(s, id, api.ContainerLogsOptions{ShowStdout: true}, fetch, StreamCloudLogsOptions{})
	if err == nil {
		t.Fatal("expected error for created container")
	}
	if _, ok := err.(*api.InvalidParameterError); !ok {
		t.Errorf("expected InvalidParameterError, got %T: %v", err, err)
	}
}

func TestStreamCloudLogs_ReturnsEntries(t *testing.T) {
	s := newSpecTestServer()

	id, err := createContainerInState(s, "exited")
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	now := time.Now().UTC()
	fetch := func(_ context.Context, _ CloudLogParams, _ any) ([]CloudLogEntry, any, error) {
		return []CloudLogEntry{
			{Timestamp: now, Message: "hello"},
			{Timestamp: now.Add(time.Second), Message: "world"},
		}, nil, nil
	}

	rc, err := StreamCloudLogs(s, id, api.ContainerLogsOptions{ShowStdout: true}, fetch, StreamCloudLogsOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read error: %v", err)
	}

	output := string(data)
	if output == "" {
		t.Fatal("expected non-empty log output")
	}
	if !logContains(output, "hello") || !logContains(output, "world") {
		t.Errorf("expected both 'hello' and 'world' in output, got %q", output)
	}
}

func TestStreamCloudLogs_TailFilter(t *testing.T) {
	s := newSpecTestServer()

	id, err := createContainerInState(s, "exited")
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	now := time.Now().UTC()
	fetch := func(_ context.Context, _ CloudLogParams, _ any) ([]CloudLogEntry, any, error) {
		return []CloudLogEntry{
			{Timestamp: now, Message: "line1"},
			{Timestamp: now.Add(1 * time.Second), Message: "line2"},
			{Timestamp: now.Add(2 * time.Second), Message: "line3"},
			{Timestamp: now.Add(3 * time.Second), Message: "line4"},
			{Timestamp: now.Add(4 * time.Second), Message: "line5"},
		}, nil, nil
	}

	rc, err := StreamCloudLogs(s, id, api.ContainerLogsOptions{
		ShowStdout: true,
		Tail:       "2",
	}, fetch, StreamCloudLogsOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read error: %v", err)
	}

	output := string(data)
	// Should only have the last 2 lines
	if logContains(output, "line1") || logContains(output, "line2") || logContains(output, "line3") {
		t.Errorf("expected only last 2 lines, but found earlier lines in output: %q", output)
	}
	if !logContains(output, "line4") || !logContains(output, "line5") {
		t.Errorf("expected line4 and line5 in output, got %q", output)
	}
}

func logContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
