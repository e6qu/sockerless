package tests

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
)

// ecsClient returns a Docker client configured for the ECS backend, or skips the test.
func ecsClient(t *testing.T) *client.Client {
	t.Helper()
	socket := os.Getenv("SOCKERLESS_ECS_SOCKET")
	if socket == "" {
		t.Skip("SOCKERLESS_ECS_SOCKET not set, skipping ECS integration test")
	}
	c, err := client.NewClientWithOpts(
		client.WithHost("unix://"+socket),
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		t.Fatalf("failed to create ECS docker client: %v", err)
	}
	return c
}

func TestECSContainerLifecycle(t *testing.T) {
	c := ecsClient(t)
	ctx := context.Background()

	// Pull image
	rc, err := c.ImagePull(ctx, "alpine:latest", image.PullOptions{})
	if err != nil {
		t.Fatalf("image pull failed: %v", err)
	}
	defer rc.Close()
	buf := make([]byte, 4096)
	for {
		if _, err := rc.Read(buf); err != nil {
			break
		}
	}

	// Create container
	resp, err := c.ContainerCreate(ctx, &container.Config{
		Image: "alpine:latest",
		Cmd:   []string{"echo", "hello from ecs"},
		Tty:   false,
	}, nil, nil, nil, "ecs-lifecycle-test")
	if err != nil {
		t.Fatalf("container create failed: %v", err)
	}
	defer c.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})

	// Start
	if err := c.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		t.Fatalf("container start failed: %v", err)
	}

	// Wait
	waitCh, errCh := c.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	select {
	case result := <-waitCh:
		if result.StatusCode != 0 {
			t.Errorf("expected exit code 0, got %d", result.StatusCode)
		}
	case err := <-errCh:
		t.Fatalf("container wait error: %v", err)
	case <-time.After(5 * time.Minute):
		t.Fatal("timeout waiting for container")
	}

	// Inspect
	info, err := c.ContainerInspect(ctx, resp.ID)
	if err != nil {
		t.Fatalf("container inspect failed: %v", err)
	}
	if info.State.Status != "exited" {
		t.Errorf("expected status 'exited', got %q", info.State.Status)
	}
}

func TestECSContainerLogs(t *testing.T) {
	c := ecsClient(t)
	ctx := context.Background()

	pullRC, _ := c.ImagePull(ctx, "alpine:latest", image.PullOptions{})
	if pullRC != nil {
		buf := make([]byte, 4096)
		for {
			if _, err := pullRC.Read(buf); err != nil {
				break
			}
		}
		pullRC.Close()
	}

	resp, err := c.ContainerCreate(ctx, &container.Config{
		Image: "alpine:latest",
		Cmd:   []string{"echo", "log-test-output"},
	}, nil, nil, nil, "ecs-logs-test")
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	defer c.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})

	c.ContainerStart(ctx, resp.ID, container.StartOptions{})

	// Wait for exit
	waitCh, _ := c.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	select {
	case <-waitCh:
	case <-time.After(5 * time.Minute):
		t.Fatal("timeout")
	}

	// Get logs
	logRC, err := c.ContainerLogs(ctx, resp.ID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
	})
	if err != nil {
		t.Fatalf("logs failed: %v", err)
	}
	defer logRC.Close()

	logBuf := make([]byte, 4096)
	n, _ := logRC.Read(logBuf)
	logOutput := string(logBuf[:n])
	if !strings.Contains(logOutput, "log-test-output") {
		t.Errorf("expected logs to contain 'log-test-output', got %q", logOutput)
	}
}

func TestECSContainerExec(t *testing.T) {
	c := ecsClient(t)
	ctx := context.Background()

	pullRC, _ := c.ImagePull(ctx, "alpine:latest", image.PullOptions{})
	if pullRC != nil {
		buf := make([]byte, 4096)
		for {
			if _, err := pullRC.Read(buf); err != nil {
				break
			}
		}
		pullRC.Close()
	}

	resp, err := c.ContainerCreate(ctx, &container.Config{
		Image:     "alpine:latest",
		Cmd:       []string{"tail", "-f", "/dev/null"},
		OpenStdin: true,
		Tty:       true,
	}, nil, nil, nil, "ecs-exec-test")
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	defer c.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})

	c.ContainerStart(ctx, resp.ID, container.StartOptions{})

	// Create exec
	execResp, err := c.ContainerExecCreate(ctx, resp.ID, container.ExecOptions{
		Cmd:          []string{"echo", "exec-output"},
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		t.Fatalf("exec create failed: %v", err)
	}

	// Start exec
	hijacked, err := c.ContainerExecAttach(ctx, execResp.ID, container.ExecStartOptions{})
	if err != nil {
		t.Fatalf("exec start failed: %v", err)
	}
	output, _ := io.ReadAll(hijacked.Reader)
	hijacked.Close()

	if !strings.Contains(string(output), "exec-output") {
		t.Errorf("expected exec output to contain 'exec-output', got %q", string(output))
	}

	// Stop container
	timeout := 5
	c.ContainerStop(ctx, resp.ID, container.StopOptions{Timeout: &timeout})
}

func TestECSContainerList(t *testing.T) {
	c := ecsClient(t)
	ctx := context.Background()

	pullRC, _ := c.ImagePull(ctx, "alpine:latest", image.PullOptions{})
	if pullRC != nil {
		buf := make([]byte, 4096)
		for {
			if _, err := pullRC.Read(buf); err != nil {
				break
			}
		}
		pullRC.Close()
	}

	resp, err := c.ContainerCreate(ctx, &container.Config{
		Image:  "alpine:latest",
		Cmd:    []string{"sleep", "30"},
		Labels: map[string]string{"test": "ecs-list"},
	}, nil, nil, nil, "ecs-list-test")
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	defer c.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})

	c.ContainerStart(ctx, resp.ID, container.StartOptions{})

	// List running containers
	containers, err := c.ContainerList(ctx, container.ListOptions{})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}

	found := false
	for _, ctr := range containers {
		if ctr.ID == resp.ID {
			found = true
			if ctr.Labels["test"] != "ecs-list" {
				t.Errorf("expected label test=ecs-list")
			}
			break
		}
	}
	if !found {
		t.Error("container not found in list")
	}

	timeout := 5
	c.ContainerStop(ctx, resp.ID, container.StopOptions{Timeout: &timeout})
}

func TestECSNetworkOperations(t *testing.T) {
	c := ecsClient(t)
	ctx := context.Background()

	// Create network
	netResp, err := c.NetworkCreate(ctx, "ecs-test-net", network.CreateOptions{
		Driver: "bridge",
	})
	if err != nil {
		t.Fatalf("network create failed: %v", err)
	}
	defer c.NetworkRemove(ctx, netResp.ID)

	// Inspect
	netInfo, err := c.NetworkInspect(ctx, netResp.ID, network.InspectOptions{})
	if err != nil {
		t.Fatalf("network inspect failed: %v", err)
	}
	if netInfo.Name != "ecs-test-net" {
		t.Errorf("expected name 'ecs-test-net', got %q", netInfo.Name)
	}

	// List
	networks, err := c.NetworkList(ctx, network.ListOptions{})
	if err != nil {
		t.Fatalf("network list failed: %v", err)
	}
	found := false
	for _, n := range networks {
		if n.ID == netResp.ID {
			found = true
			break
		}
	}
	if !found {
		t.Error("network not found in list")
	}
}

func TestECSVolumeOperations(t *testing.T) {
	c := ecsClient(t)
	ctx := context.Background()

	// Create volume
	vol, err := c.VolumeCreate(ctx, volume.CreateOptions{Name: "ecs-test-vol"})
	if err != nil {
		t.Fatalf("volume create failed: %v", err)
	}
	defer c.VolumeRemove(ctx, vol.Name, true)

	// Inspect
	volInfo, err := c.VolumeInspect(ctx, vol.Name)
	if err != nil {
		t.Fatalf("volume inspect failed: %v", err)
	}
	if volInfo.Name != "ecs-test-vol" {
		t.Errorf("expected name 'ecs-test-vol', got %q", volInfo.Name)
	}

	// List
	volList, err := c.VolumeList(ctx, volume.ListOptions{})
	if err != nil {
		t.Fatalf("volume list failed: %v", err)
	}
	found := false
	for _, v := range volList.Volumes {
		if v.Name == "ecs-test-vol" {
			found = true
			break
		}
	}
	if !found {
		t.Error("volume not found in list")
	}
}
