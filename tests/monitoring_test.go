package tests

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/volume"
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

	// Don't start the container — stats should fail
	_, err := dockerClient.ContainerStats(ctx, id, false)
	if err == nil {
		t.Error("expected error for stats on non-running container")
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

	// Get top
	top, err := dockerClient.ContainerTop(ctx, id, nil)
	if err != nil {
		t.Fatalf("top failed: %v", err)
	}

	// Verify we have column titles
	if len(top.Titles) == 0 {
		t.Error("top returned no titles")
	}

	// Verify at least one process (the main entrypoint)
	if len(top.Processes) == 0 {
		t.Error("top returned no processes")
	}

	// Find the CMD column and verify the main process
	cmdCol := -1
	for i, title := range top.Titles {
		if title == "CMD" {
			cmdCol = i
			break
		}
	}
	if cmdCol == -1 {
		t.Fatal("top titles missing CMD column")
	}

	// First process should be PID 1 (the main process)
	pidCol := -1
	for i, title := range top.Titles {
		if title == "PID" {
			pidCol = i
			break
		}
	}
	if pidCol >= 0 && top.Processes[0][pidCol] != "1" {
		t.Errorf("expected first process PID to be 1, got %s", top.Processes[0][pidCol])
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

	// Create a container
	id := createContainer(t, "test-df-container", &container.Config{
		Image: "alpine",
		Cmd:   []string{"echo", "hello"},
	}, nil)
	defer removeContainer(t, id)

	// Create a volume
	volName := "test-df-volume-" + generateTestID()
	createVolume(t, volName)
	defer removeVolume(t, volName)

	// Get disk usage
	du, err := dockerClient.DiskUsage(ctx, types.DiskUsageOptions{})
	if err != nil {
		t.Fatalf("disk usage failed: %v", err)
	}

	// Verify images are listed
	if len(du.Images) == 0 {
		t.Error("disk usage returned no images")
	}

	// Verify our container appears
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

	// Verify our volume appears
	volFound := false
	for _, v := range du.Volumes {
		if v.Name == volName {
			volFound = true
			break
		}
	}
	if !volFound {
		t.Error("disk usage did not include test volume")
	}
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

	// Give WASM process time to start and create rootfs
	time.Sleep(200 * time.Millisecond)

	du, err := dockerClient.DiskUsage(ctx, types.DiskUsageOptions{})
	if err != nil {
		t.Fatalf("disk usage failed: %v", err)
	}

	// Find our container and check its size
	for _, c := range du.Containers {
		if c.ID == id {
			// Running WASM container should have non-zero SizeRw
			// (rootfs skeleton creates files)
			if c.SizeRw > 0 {
				t.Logf("container rootfs size: %d bytes", c.SizeRw)
			}
			return
		}
	}
	t.Error("running container not found in disk usage")
}

func TestContainerCreateVolume(t *testing.T) {
	// Skip this — it's not a monitoring test but helps ensure volume
	// temp dir tracking works for df
	volName := "test-vol-df-" + generateTestID()
	_, err := dockerClient.VolumeCreate(ctx, volume.CreateOptions{Name: volName})
	if err != nil {
		t.Fatalf("volume create failed: %v", err)
	}
	defer removeVolume(t, volName)

	du, err := dockerClient.DiskUsage(ctx, types.DiskUsageOptions{})
	if err != nil {
		t.Fatalf("disk usage failed: %v", err)
	}

	for _, v := range du.Volumes {
		if v.Name == volName {
			return
		}
	}
	t.Error("volume not found in disk usage")
}
