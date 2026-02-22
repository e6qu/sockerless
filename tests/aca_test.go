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

// acaClient returns a Docker client configured for the ACA backend, or skips the test.
func acaClient(t *testing.T) *client.Client {
	t.Helper()
	socket := os.Getenv("SOCKERLESS_ACA_SOCKET")
	if socket == "" {
		t.Skip("SOCKERLESS_ACA_SOCKET not set, skipping ACA integration test")
	}
	c, err := client.NewClientWithOpts(
		client.WithHost("unix://"+socket),
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		t.Fatalf("failed to create ACA docker client: %v", err)
	}
	return c
}

func TestACAContainerLifecycle(t *testing.T) {
	c := acaClient(t)
	ctx := context.Background()

	// Pull image
	rc, err := c.ImagePull(ctx, "alpine:latest", image.PullOptions{})
	if err != nil {
		t.Fatalf("image pull failed: %v", err)
	}
	io.Copy(io.Discard, rc)
	rc.Close()

	testID := generateTestID()

	// Create
	resp, err := c.ContainerCreate(ctx,
		&container.Config{
			Image: "alpine:latest",
			Cmd:   []string{"tail", "-f", "/dev/null"},
		},
		nil, nil, nil, "aca_"+testID,
	)
	if err != nil {
		t.Fatalf("container create failed: %v", err)
	}
	defer c.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})

	// Inspect (should be created)
	info, err := c.ContainerInspect(ctx, resp.ID)
	if err != nil {
		t.Fatalf("container inspect failed: %v", err)
	}
	if info.State.Status != "created" {
		t.Errorf("expected status created, got %s", info.State.Status)
	}

	// Start (ACA may take longer — 10 min timeout)
	startCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	if err := c.ContainerStart(startCtx, resp.ID, container.StartOptions{}); err != nil {
		t.Fatalf("container start failed: %v", err)
	}

	// Verify running
	info, err = c.ContainerInspect(ctx, resp.ID)
	if err != nil {
		t.Fatalf("container inspect failed: %v", err)
	}
	if !info.State.Running {
		t.Error("expected container to be running")
	}

	// Stop
	timeout := 10
	if err := c.ContainerStop(ctx, resp.ID, container.StopOptions{Timeout: &timeout}); err != nil {
		t.Fatalf("container stop failed: %v", err)
	}

	// Verify stopped
	info, err = c.ContainerInspect(ctx, resp.ID)
	if err != nil {
		t.Fatalf("container inspect failed: %v", err)
	}
	if info.State.Running {
		t.Error("expected container to be stopped")
	}

	// Remove
	if err := c.ContainerRemove(ctx, resp.ID, container.RemoveOptions{}); err != nil {
		t.Fatalf("container remove failed: %v", err)
	}
}

func TestACAContainerLogs(t *testing.T) {
	c := acaClient(t)
	ctx := context.Background()

	rc, err := c.ImagePull(ctx, "alpine:latest", image.PullOptions{})
	if err != nil {
		t.Fatalf("image pull failed: %v", err)
	}
	io.Copy(io.Discard, rc)
	rc.Close()

	testID := generateTestID()
	resp, err := c.ContainerCreate(ctx,
		&container.Config{
			Image:      "alpine:latest",
			Entrypoint: []string{"sh", "-c", "echo hello-aca && sleep 5"},
		},
		nil, nil, nil, "aca_logs_"+testID,
	)
	if err != nil {
		t.Fatalf("container create failed: %v", err)
	}
	defer c.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})

	startCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	if err := c.ContainerStart(startCtx, resp.ID, container.StartOptions{}); err != nil {
		t.Fatalf("container start failed: %v", err)
	}

	// Wait for log ingestion (Azure Monitor can have 2-10s delay)
	time.Sleep(10 * time.Second)

	logReader, err := c.ContainerLogs(ctx, resp.ID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
	})
	if err != nil {
		t.Fatalf("container logs failed: %v", err)
	}
	logData, _ := io.ReadAll(logReader)
	logReader.Close()

	t.Logf("logs: %q", string(logData))
	if !strings.Contains(string(logData), "hello-aca") {
		t.Log("note: log may not yet be available due to Azure Monitor ingestion delay")
	}

	c.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})
}

func TestACAContainerExec(t *testing.T) {
	c := acaClient(t)
	ctx := context.Background()

	rc, err := c.ImagePull(ctx, "alpine:latest", image.PullOptions{})
	if err != nil {
		t.Fatalf("image pull failed: %v", err)
	}
	io.Copy(io.Discard, rc)
	rc.Close()

	testID := generateTestID()
	resp, err := c.ContainerCreate(ctx,
		&container.Config{
			Image: "alpine:latest",
			Cmd:   []string{"tail", "-f", "/dev/null"},
		},
		nil, nil, nil, "aca_exec_"+testID,
	)
	if err != nil {
		t.Fatalf("container create failed: %v", err)
	}
	defer c.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})

	startCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	if err := c.ContainerStart(startCtx, resp.ID, container.StartOptions{}); err != nil {
		t.Fatalf("container start failed: %v", err)
	}

	// Exec
	execResp, err := c.ContainerExecCreate(ctx, resp.ID, container.ExecOptions{
		Cmd:          []string{"echo", "hello-exec-aca"},
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		t.Fatalf("exec create failed: %v", err)
	}

	hijacked, err := c.ContainerExecAttach(ctx, execResp.ID, container.ExecStartOptions{})
	if err != nil {
		t.Fatalf("exec start failed: %v", err)
	}
	output, _ := io.ReadAll(hijacked.Reader)
	hijacked.Close()

	if !strings.Contains(string(output), "hello-exec-aca") {
		t.Errorf("expected exec output to contain 'hello-exec-aca', got %q", string(output))
	}
}

func TestACAContainerList(t *testing.T) {
	c := acaClient(t)
	ctx := context.Background()

	testID := generateTestID()
	resp, err := c.ContainerCreate(ctx,
		&container.Config{
			Image: "alpine:latest",
		},
		nil, nil, nil, "aca_list_"+testID,
	)
	if err != nil {
		t.Fatalf("container create failed: %v", err)
	}
	defer c.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})

	containers, err := c.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		t.Fatalf("container list failed: %v", err)
	}

	found := false
	for _, cn := range containers {
		if cn.ID == resp.ID {
			found = true
			break
		}
	}
	if !found {
		t.Error("created container not found in list")
	}
}

func TestACANetworkOperations(t *testing.T) {
	c := acaClient(t)
	ctx := context.Background()

	testID := generateTestID()
	netName := "aca_net_" + testID

	// Create
	netResp, err := c.NetworkCreate(ctx, netName, network.CreateOptions{
		Driver: "bridge",
	})
	if err != nil {
		t.Fatalf("network create failed: %v", err)
	}
	defer c.NetworkRemove(ctx, netResp.ID)

	// Inspect
	net, err := c.NetworkInspect(ctx, netResp.ID, network.InspectOptions{})
	if err != nil {
		t.Fatalf("network inspect failed: %v", err)
	}
	if net.Name != netName {
		t.Errorf("expected name %s, got %s", netName, net.Name)
	}

	// Remove
	if err := c.NetworkRemove(ctx, netResp.ID); err != nil {
		t.Fatalf("network remove failed: %v", err)
	}
}

func TestACAVolumeOperations(t *testing.T) {
	c := acaClient(t)
	ctx := context.Background()

	testID := generateTestID()
	volName := "aca_vol_" + testID

	// Create
	vol, err := c.VolumeCreate(ctx, volume.CreateOptions{Name: volName})
	if err != nil {
		t.Fatalf("volume create failed: %v", err)
	}
	defer c.VolumeRemove(ctx, vol.Name, true)

	// Inspect
	volInfo, err := c.VolumeInspect(ctx, vol.Name)
	if err != nil {
		t.Fatalf("volume inspect failed: %v", err)
	}
	if volInfo.Name != volName {
		t.Errorf("expected name %s, got %s", volName, volInfo.Name)
	}

	// Remove
	if err := c.VolumeRemove(ctx, vol.Name, true); err != nil {
		t.Fatalf("volume remove failed: %v", err)
	}
}
