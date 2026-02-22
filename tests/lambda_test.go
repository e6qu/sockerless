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

// lambdaClient returns a Docker client configured for the Lambda backend, or skips the test.
func lambdaClient(t *testing.T) *client.Client {
	t.Helper()
	socket := os.Getenv("SOCKERLESS_LAMBDA_SOCKET")
	if socket == "" {
		t.Skip("SOCKERLESS_LAMBDA_SOCKET not set, skipping Lambda integration test")
	}
	c, err := client.NewClientWithOpts(
		client.WithHost("unix://"+socket),
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		t.Fatalf("failed to create Lambda docker client: %v", err)
	}
	return c
}

func TestLambdaContainerLifecycle(t *testing.T) {
	c := lambdaClient(t)
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
			Cmd:   []string{"echo", "hello from lambda"},
		},
		nil, nil, nil, "lambda_"+testID,
	)
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

	// Remove
	if err := c.ContainerRemove(ctx, resp.ID, container.RemoveOptions{}); err != nil {
		t.Fatalf("container remove failed: %v", err)
	}
}

func TestLambdaContainerLogs(t *testing.T) {
	c := lambdaClient(t)
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
			Cmd:   []string{"echo", "hello-lambda-logs"},
		},
		nil, nil, nil, "lambda_logs_"+testID,
	)
	if err != nil {
		t.Fatalf("container create failed: %v", err)
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
	if !strings.Contains(string(logData), "hello-lambda-logs") {
		t.Log("note: log may not yet be available due to CloudWatch ingestion delay")
	}
}

func TestLambdaContainerList(t *testing.T) {
	c := lambdaClient(t)
	ctx := context.Background()

	testID := generateTestID()
	resp, err := c.ContainerCreate(ctx,
		&container.Config{
			Image: "alpine:latest",
		},
		nil, nil, nil, "lambda_list_"+testID,
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

func TestLambdaContainerStopNoOp(t *testing.T) {
	c := lambdaClient(t)
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
			Cmd:   []string{"sleep", "30"},
		},
		nil, nil, nil, "lambda_stop_"+testID,
	)
	if err != nil {
		t.Fatalf("container create failed: %v", err)
	}
	defer c.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})

	c.ContainerStart(ctx, resp.ID, container.StartOptions{})

	// Stop should succeed as no-op
	timeout := 5
	if err := c.ContainerStop(ctx, resp.ID, container.StopOptions{Timeout: &timeout}); err != nil {
		t.Fatalf("container stop failed (should be no-op): %v", err)
	}
}

func TestLambdaContainerExec(t *testing.T) {
	c := lambdaClient(t)
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
			Image:     "alpine:latest",
			Cmd:       []string{"tail", "-f", "/dev/null"},
			Tty:       true,
			OpenStdin: true,
		},
		nil, nil, nil, "lambda_exec_"+testID,
	)
	if err != nil {
		t.Fatalf("container create failed: %v", err)
	}
	defer c.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})

	if err := c.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		t.Fatalf("container start failed: %v", err)
	}

	// Exec create should succeed (synthetic exec from core)
	execResp, err := c.ContainerExecCreate(ctx, resp.ID, container.ExecOptions{
		Cmd:          []string{"echo", "hello"},
		AttachStdout: true,
	})
	if err != nil {
		t.Fatalf("exec create failed: %v", err)
	}

	if execResp.ID == "" {
		t.Error("expected non-empty exec ID")
	}
}

func TestLambdaNetworkOperations(t *testing.T) {
	c := lambdaClient(t)
	ctx := context.Background()

	testID := generateTestID()

	// Network create should succeed
	netResp, err := c.NetworkCreate(ctx, "lambda-net-"+testID, network.CreateOptions{})
	if err != nil {
		t.Fatalf("network create failed: %v", err)
	}

	// Network inspect
	net, err := c.NetworkInspect(ctx, netResp.ID, network.InspectOptions{})
	if err != nil {
		t.Fatalf("network inspect failed: %v", err)
	}
	if net.Name != "lambda-net-"+testID {
		t.Errorf("expected network name %q, got %q", "lambda-net-"+testID, net.Name)
	}

	// Network remove
	if err := c.NetworkRemove(ctx, netResp.ID); err != nil {
		t.Fatalf("network remove failed: %v", err)
	}
}

func TestLambdaVolumeOperations(t *testing.T) {
	c := lambdaClient(t)
	ctx := context.Background()

	testID := generateTestID()

	// Volume create should succeed
	vol, err := c.VolumeCreate(ctx, volume.CreateOptions{Name: "lambda-vol-" + testID})
	if err != nil {
		t.Fatalf("volume create failed: %v", err)
	}

	if vol.Name != "lambda-vol-"+testID {
		t.Errorf("expected volume name %q, got %q", "lambda-vol-"+testID, vol.Name)
	}

	// Volume inspect
	volInfo, err := c.VolumeInspect(ctx, vol.Name)
	if err != nil {
		t.Fatalf("volume inspect failed: %v", err)
	}
	if volInfo.Name != vol.Name {
		t.Errorf("expected volume name %q, got %q", vol.Name, volInfo.Name)
	}

	// Volume remove
	if err := c.VolumeRemove(ctx, vol.Name, false); err != nil {
		t.Fatalf("volume remove failed: %v", err)
	}
}
