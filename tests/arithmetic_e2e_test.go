package tests

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

// TestArithmeticExecution verifies real computation through the Docker API
// using shell arithmetic (works on all backends including WASM sandbox).
func TestArithmeticExecution(t *testing.T) {
	for name, c := range availableRunnerClients(t) {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			testID := generateTestID(name, "arith-exec")

			resp, err := c.ContainerCreate(ctx, &container.Config{
				Image: "alpine:latest",
				Cmd:   []string{"sh", "-c", "echo $((3 + 4 * 2))"},
			}, nil, nil, nil, "arith-exec-"+testID)
			if err != nil {
				t.Fatalf("create failed: %v", err)
			}
			defer c.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})

			if err := c.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
				t.Fatalf("start failed: %v", err)
			}

			waitCh, errCh := c.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
			select {
			case result := <-waitCh:
				if result.StatusCode != 0 {
					t.Errorf("expected exit code 0, got %d", result.StatusCode)
				}
			case err := <-errCh:
				t.Fatalf("wait error: %v", err)
			case <-time.After(5 * time.Minute):
				t.Fatal("timeout waiting for container")
			}

			logs := readLogs(t, c, resp.ID)
			if !strings.Contains(logs, "11") {
				t.Errorf("expected logs to contain %q, got %q", "11", logs)
			}
		})
	}
}

// TestArithmeticNonZeroExit verifies non-zero exit codes propagate through Docker API.
func TestArithmeticNonZeroExit(t *testing.T) {
	for name, c := range availableRunnerClients(t) {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			testID := generateTestID(name, "arith-nz")

			resp, err := c.ContainerCreate(ctx, &container.Config{
				Image: "alpine:latest",
				Cmd:   []string{"sh", "-c", "echo ERROR: bad input >&2; exit 1"},
			}, nil, nil, nil, "arith-nz-"+testID)
			if err != nil {
				t.Fatalf("create failed: %v", err)
			}
			defer c.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})

			if err := c.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
				t.Fatalf("start failed: %v", err)
			}

			waitCh, errCh := c.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
			select {
			case result := <-waitCh:
				if result.StatusCode != 1 {
					t.Errorf("expected exit code 1, got %d", result.StatusCode)
				}
			case err := <-errCh:
				t.Fatalf("wait error: %v", err)
			case <-time.After(5 * time.Minute):
				t.Fatal("timeout waiting for container")
			}
		})
	}
}

// TestArithmeticExecInContainer verifies exec can run real computation in a container.
func TestArithmeticExecInContainer(t *testing.T) {
	for name, c := range availableRunnerClients(t) {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			testID := generateTestID(name, "arith-exec-in")

			resp, err := c.ContainerCreate(ctx, &container.Config{
				Image:     "alpine:latest",
				Cmd:       []string{"tail", "-f", "/dev/null"},
				OpenStdin: true,
				Tty:       true,
			}, nil, nil, nil, "arith-exec-in-"+testID)
			if err != nil {
				t.Fatalf("create failed: %v", err)
			}
			defer c.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})

			if err := c.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
				t.Fatalf("start failed: %v", err)
			}

			execResp, err := c.ContainerExecCreate(ctx, resp.ID, container.ExecOptions{
				Cmd:          []string{"sh", "-c", "echo $((7 * 6))"},
				AttachStdout: true,
				AttachStderr: true,
			})
			if err != nil {
				t.Fatalf("exec create failed: %v", err)
			}

			hijacked, err := c.ContainerExecAttach(ctx, execResp.ID, container.ExecStartOptions{})
			if err != nil {
				t.Fatalf("exec attach failed: %v", err)
			}
			output, _ := io.ReadAll(hijacked.Reader)
			hijacked.Close()

			if !strings.Contains(string(output), "42") {
				t.Errorf("expected exec output to contain %q, got %q", "42", string(output))
			}
		})
	}
}

// TestArithmeticEvalBinary verifies the eval-arithmetic Go binary runs
// through the Docker API. This uses a real compiled binary, not shell
// arithmetic, proving that process execution is fully functional.
// The memory backend (WASM sandbox) cannot run native binaries.
func TestArithmeticEvalBinary(t *testing.T) {
	if evalBinaryPath == "" {
		t.Skip("eval binary not built")
	}
	for name, c := range availableRunnerClients(t) {
		t.Run(name, func(t *testing.T) {
			if name == "memory" {
				t.Skip("WASM sandbox cannot execute native binaries")
			}
			ctx := context.Background()
			testID := generateTestID(name, "eval-bin")

			resp, err := c.ContainerCreate(ctx, &container.Config{
				Image: "alpine:latest",
				Cmd:   []string{evalBinaryPath, "(3 + 4) * 2"},
			}, nil, nil, nil, "eval-bin-"+testID)
			if err != nil {
				t.Fatalf("create failed: %v", err)
			}
			defer c.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})

			if err := c.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
				t.Fatalf("start failed: %v", err)
			}

			waitCh, errCh := c.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
			select {
			case result := <-waitCh:
				if result.StatusCode != 0 {
					t.Errorf("expected exit code 0, got %d", result.StatusCode)
				}
			case err := <-errCh:
				t.Fatalf("wait error: %v", err)
			case <-time.After(5 * time.Minute):
				t.Fatal("timeout waiting for container")
			}

			logs := readLogs(t, c, resp.ID)
			if !strings.Contains(logs, "14") {
				t.Errorf("expected logs to contain %q, got %q", "14", logs)
			}
		})
	}
}

// readLogs reads container logs with stdcopy demux and retry.
func readLogs(t *testing.T, c *client.Client, id string) string {
	t.Helper()
	ctx := context.Background()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		rc, err := c.ContainerLogs(ctx, id, container.LogsOptions{
			ShowStdout: true,
			ShowStderr: true,
		})
		if err != nil {
			t.Fatalf("container logs failed: %v", err)
		}
		var stdout, stderr bytes.Buffer
		stdcopy.StdCopy(&stdout, &stderr, rc)
		rc.Close()
		combined := stdout.String() + stderr.String()
		if len(combined) > 0 {
			return combined
		}
		time.Sleep(500 * time.Millisecond)
	}
	return ""
}
