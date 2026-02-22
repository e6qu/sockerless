package tests

import (
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
)

func TestContainerCreateAndInspect(t *testing.T) {
	pullImage(t, "alpine")

	id := createContainer(t, "test-inspect", &container.Config{
		Image:  "alpine",
		Cmd:    []string{"echo", "hello"},
		Labels: map[string]string{"test": "true"},
	}, nil)
	defer removeContainer(t, id)

	if len(id) != 64 {
		t.Errorf("expected 64-char ID, got %d chars: %s", len(id), id)
	}

	// Inspect
	info, err := dockerClient.ContainerInspect(ctx, id)
	if err != nil {
		t.Fatalf("inspect failed: %v", err)
	}

	if info.ID != id {
		t.Errorf("expected ID %s, got %s", id, info.ID)
	}
	if info.Name != "/test-inspect" {
		t.Errorf("expected name /test-inspect, got %s", info.Name)
	}
	if info.Config.Image != "alpine" {
		t.Errorf("expected image alpine, got %s", info.Config.Image)
	}
	if info.State.Status != "created" {
		t.Errorf("expected status created, got %s", info.State.Status)
	}
}

func TestContainerStartStop(t *testing.T) {
	pullImage(t, "alpine")

	id := createContainer(t, "test-startstop", &container.Config{
		Image:     "alpine",
		Cmd:       []string{"sh"},
		Tty:       true,
		OpenStdin: true,
	}, nil)
	defer removeContainer(t, id)

	// Start
	if err := dockerClient.ContainerStart(ctx, id, container.StartOptions{}); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// Inspect — should be running
	info, err := dockerClient.ContainerInspect(ctx, id)
	if err != nil {
		t.Fatalf("inspect failed: %v", err)
	}
	if !info.State.Running {
		t.Error("expected container to be running")
	}

	// Stop
	timeout := 0
	if err := dockerClient.ContainerStop(ctx, id, container.StopOptions{Timeout: &timeout}); err != nil {
		t.Fatalf("stop failed: %v", err)
	}

	// Inspect — should be exited
	info, err = dockerClient.ContainerInspect(ctx, id)
	if err != nil {
		t.Fatalf("inspect failed: %v", err)
	}
	if info.State.Running {
		t.Error("expected container to not be running")
	}
	if info.State.Status != "exited" {
		t.Errorf("expected status exited, got %s", info.State.Status)
	}
}

func TestContainerList(t *testing.T) {
	pullImage(t, "alpine")

	id := createContainer(t, "test-list", &container.Config{
		Image:  "alpine",
		Cmd:    []string{"echo", "hello"},
		Labels: map[string]string{"listtest": "yes"},
	}, nil)
	defer removeContainer(t, id)

	// List all (including non-running)
	containers, err := dockerClient.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}

	found := false
	for _, c := range containers {
		if c.ID == id {
			found = true
			if c.State != "created" {
				t.Errorf("expected state created, got %s", c.State)
			}
			break
		}
	}
	if !found {
		t.Error("container not found in list")
	}
}

func TestContainerKill(t *testing.T) {
	pullImage(t, "alpine")

	id := createContainer(t, "test-kill", &container.Config{
		Image:     "alpine",
		Cmd:       []string{"sh"},
		Tty:       true,
		OpenStdin: true,
	}, nil)
	defer removeContainer(t, id)

	dockerClient.ContainerStart(ctx, id, container.StartOptions{})

	if err := dockerClient.ContainerKill(ctx, id, "SIGKILL"); err != nil {
		t.Fatalf("kill failed: %v", err)
	}

	info, err := dockerClient.ContainerInspect(ctx, id)
	if err != nil {
		t.Fatalf("inspect failed: %v", err)
	}
	if info.State.Running {
		t.Error("expected container to not be running after kill")
	}
}

func TestContainerRemove(t *testing.T) {
	pullImage(t, "alpine")

	id := createContainer(t, "test-remove", &container.Config{
		Image: "alpine",
		Cmd:   []string{"echo", "hello"},
	}, nil)

	if err := dockerClient.ContainerRemove(ctx, id, container.RemoveOptions{}); err != nil {
		t.Fatalf("remove failed: %v", err)
	}

	// Inspect should fail
	_, err := dockerClient.ContainerInspect(ctx, id)
	if err == nil {
		t.Error("expected error inspecting removed container")
	}
}

func TestContainerRemoveForce(t *testing.T) {
	pullImage(t, "alpine")

	id := createContainer(t, "test-remove-force", &container.Config{
		Image:     "alpine",
		Cmd:       []string{"sh"},
		Tty:       true,
		OpenStdin: true,
	}, nil)

	dockerClient.ContainerStart(ctx, id, container.StartOptions{})

	// Force remove running container
	if err := dockerClient.ContainerRemove(ctx, id, container.RemoveOptions{Force: true}); err != nil {
		t.Fatalf("force remove failed: %v", err)
	}
}

func TestContainerWait(t *testing.T) {
	pullImage(t, "alpine")

	// Non-interactive container exits immediately
	id := createContainer(t, "test-wait", &container.Config{
		Image: "alpine",
		Cmd:   []string{"echo", "hello"},
	}, nil)
	defer removeContainer(t, id)

	dockerClient.ContainerStart(ctx, id, container.StartOptions{})

	waitCh, errCh := dockerClient.ContainerWait(ctx, id, container.WaitConditionNotRunning)
	select {
	case result := <-waitCh:
		if result.StatusCode != 0 {
			t.Errorf("expected exit code 0, got %d", result.StatusCode)
		}
	case err := <-errCh:
		t.Fatalf("wait failed: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for container")
	}
}

func TestContainerNameConflict(t *testing.T) {
	pullImage(t, "alpine")

	id := createContainer(t, "test-conflict", &container.Config{
		Image: "alpine",
		Cmd:   []string{"echo", "hello"},
	}, nil)
	defer removeContainer(t, id)

	// Creating another container with the same name should fail
	_, err := dockerClient.ContainerCreate(ctx, &container.Config{
		Image: "alpine",
		Cmd:   []string{"echo", "hello"},
	}, nil, nil, nil, "test-conflict")
	if err == nil {
		t.Error("expected error for duplicate name")
	}
}

func TestContainerStartAlreadyStarted(t *testing.T) {
	pullImage(t, "alpine")

	id := createContainer(t, "test-double-start", &container.Config{
		Image:     "alpine",
		Cmd:       []string{"sh"},
		Tty:       true,
		OpenStdin: true,
	}, nil)
	defer removeContainer(t, id)

	dockerClient.ContainerStart(ctx, id, container.StartOptions{})

	// Starting again should return 304 (not modified)
	err := dockerClient.ContainerStart(ctx, id, container.StartOptions{})
	// Docker SDK treats 304 as success (no error)
	if err != nil {
		t.Logf("second start returned: %v (may be expected)", err)
	}
}

func TestContainerWithLabels(t *testing.T) {
	pullImage(t, "alpine")

	labels := map[string]string{
		"com.example.app":     "myapp",
		"com.example.version": "1.0",
	}

	id := createContainer(t, "test-labels", &container.Config{
		Image:  "alpine",
		Cmd:    []string{"echo", "hello"},
		Labels: labels,
	}, nil)
	defer removeContainer(t, id)

	info, err := dockerClient.ContainerInspect(ctx, id)
	if err != nil {
		t.Fatalf("inspect failed: %v", err)
	}

	for k, v := range labels {
		if info.Config.Labels[k] != v {
			t.Errorf("expected label %s=%s, got %s", k, v, info.Config.Labels[k])
		}
	}
}
