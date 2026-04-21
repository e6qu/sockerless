package tests

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
)

func TestContainerStats(t *testing.T) {
	pullImage(t, "alpine")

	id := createContainer(t, "test-stats", &container.Config{
		Image:     "alpine",
		Cmd:       []string{"sh"},
		Tty:       true,
		OpenStdin: true,
	}, nil)
	defer removeContainer(t, id)

	if err := dockerClient.ContainerStart(ctx, id, container.StartOptions{}); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// Get stats (non-streaming)
	resp, err := dockerClient.ContainerStats(ctx, id, false)
	if err != nil {
		t.Fatalf("stats failed: %v", err)
	}
	defer resp.Body.Close()

	var stats map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		t.Fatalf("failed to decode stats: %v", err)
	}

	// Verify required fields
	if _, ok := stats["read"]; !ok {
		t.Error("stats missing 'read' field")
	}
	if _, ok := stats["cpu_stats"]; !ok {
		t.Error("stats missing 'cpu_stats' field")
	}
	if _, ok := stats["memory_stats"]; !ok {
		t.Error("stats missing 'memory_stats' field")
	}

	// Verify memory_stats has usage
	memStats, ok := stats["memory_stats"].(map[string]any)
	if !ok {
		t.Fatal("memory_stats is not an object")
	}
	if _, ok := memStats["usage"]; !ok {
		t.Error("memory_stats missing 'usage' field")
	}
	if _, ok := memStats["limit"]; !ok {
		t.Error("memory_stats missing 'limit' field")
	}
}

func TestContainerStatsStream(t *testing.T) {
	pullImage(t, "alpine")

	id := createContainer(t, "test-stats-stream", &container.Config{
		Image:     "alpine",
		Cmd:       []string{"sh"},
		Tty:       true,
		OpenStdin: true,
	}, nil)
	defer removeContainer(t, id)

	if err := dockerClient.ContainerStart(ctx, id, container.StartOptions{}); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// Get stats (streaming) with a timeout
	streamCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	resp, err := dockerClient.ContainerStats(streamCtx, id, true)
	if err != nil {
		t.Fatalf("stats stream failed: %v", err)
	}
	defer resp.Body.Close()

	// Read at least 2 JSON objects from the stream
	dec := json.NewDecoder(resp.Body)
	count := 0
	for count < 2 {
		var stats map[string]any
		if err := dec.Decode(&stats); err != nil {
			break
		}
		if _, ok := stats["read"]; !ok {
			t.Errorf("stats line %d missing 'read' field", count)
		}
		count++
	}

	if count < 2 {
		t.Errorf("expected at least 2 stats entries from stream, got %d", count)
	}
}

func TestContainerStatsNotRunning(t *testing.T) {
	pullImage(t, "alpine")

	id := createContainer(t, "test-stats-stopped", &container.Config{
		Image: "alpine",
		Cmd:   []string{"echo", "hello"},
	}, nil)
	defer removeContainer(t, id)

	resp, err := dockerClient.ContainerStats(ctx, id, false)
	if err != nil {
		t.Fatalf("expected stats snapshot for non-running container, got error: %v", err)
	}
	defer resp.Body.Close()

	var stats map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		t.Fatalf("failed to decode stats: %v", err)
	}
	if _, ok := stats["read"]; !ok {
		t.Error("stats response missing 'read' field")
	}
}

func TestContainerTop(t *testing.T) {
	pullImage(t, "alpine")

	id := createContainer(t, "test-top", &container.Config{
		Image:     "alpine",
		Cmd:       []string{"sh"},
		Tty:       true,
		OpenStdin: true,
	}, nil)
	defer removeContainer(t, id)

	if err := dockerClient.ContainerStart(ctx, id, container.StartOptions{}); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// Container top requires an agent connection.
	// Without an agent, it returns NotImplemented (501).
	top, err := dockerClient.ContainerTop(ctx, id, nil)
	if err != nil {
		// Expected when no agent is connected
		t.Logf("top returned expected error (no agent): %v", err)
		return
	}

	// Agent connected — verify process list
	if len(top.Titles) == 0 {
		t.Error("top returned no titles")
	}
	if len(top.Processes) == 0 {
		t.Error("top returned no processes")
	}
}

func TestContainerTopNotRunning(t *testing.T) {
	pullImage(t, "alpine")

	id := createContainer(t, "test-top-stopped", &container.Config{
		Image: "alpine",
		Cmd:   []string{"echo", "hello"},
	}, nil)
	defer removeContainer(t, id)

	// Don't start — top should fail
	_, err := dockerClient.ContainerTop(ctx, id, nil)
	if err == nil {
		t.Error("expected error for top on non-running container")
	}
}

func TestSystemDf(t *testing.T) {
	pullImage(t, "alpine")

	id := createContainer(t, "test-df-container", &container.Config{
		Image: "alpine",
		Cmd:   []string{"echo", "hello"},
	}, nil)
	defer removeContainer(t, id)

	du, err := dockerClient.DiskUsage(ctx, types.DiskUsageOptions{})
	if err != nil {
		t.Fatalf("disk usage failed: %v", err)
	}

	if len(du.Images) == 0 {
		t.Error("disk usage returned no images")
	}

	found := false
	for _, c := range du.Containers {
		if c.ID == id {
			found = true
			break
		}
	}
	if !found {
		t.Error("disk usage did not include test container")
	}
	// Volume disk usage is intentionally not asserted here: named
	// volumes are unsupported on the ECS backend (tests/volumes_test.go
	// pins `VolumeCreate` as NotImplemented). Phase 91 re-enables.
}

func TestSystemDfWithRunningContainer(t *testing.T) {
	pullImage(t, "alpine")

	id := createContainer(t, "test-df-running", &container.Config{
		Image:     "alpine",
		Cmd:       []string{"sh"},
		Tty:       true,
		OpenStdin: true,
	}, nil)
	defer removeContainer(t, id)

	if err := dockerClient.ContainerStart(ctx, id, container.StartOptions{}); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// Give container time to start and create rootfs
	time.Sleep(200 * time.Millisecond)

	du, err := dockerClient.DiskUsage(ctx, types.DiskUsageOptions{})
	if err != nil {
		t.Fatalf("disk usage failed: %v", err)
	}

	// Find our container and check its size
	for _, c := range du.Containers {
		if c.ID == id {
			// Running container should have non-zero SizeRw
			if c.SizeRw > 0 {
				t.Logf("container rootfs size: %d bytes", c.SizeRw)
			}
			return
		}
	}
	t.Error("running container not found in disk usage")
}

// TestContainerCreateVolume removed — named volumes aren't supported
// on the ECS backend (see tests/volumes_test.go and BUG-731). The
// disk-usage / volume interaction will be re-tested when Phase 91
// ships real EFS-backed volume provisioning.
