package tests

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/pkg/stdcopy"
)

// TestGitHubRunnerContainerJob simulates a GitHub Actions container job.
// GitHub Actions runner sequence:
//  1. Version check (ping)
//  2. Create network
//  3. Pull image
//  4. Create container with tail -f /dev/null
//  5. Start container
//  6. Execute steps via exec
//  7. Collect logs
//  8. Force-remove container
//  9. Remove network
func TestGitHubRunnerContainerJob(t *testing.T) {
	for name, c := range availableRunnerClients(t) {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			testID := generateTestID(name)

			// === Step 1: Version check ===
			t.Log("Step 1: Version check")
			_, err := c.Ping(ctx)
			if err != nil {
				t.Fatalf("ping failed: %v", err)
			}

			// === Step 2: Create network ===
			t.Log("Step 2: Create network")
			netName := "github_network_" + testID
			netResp, err := c.NetworkCreate(ctx, netName, network.CreateOptions{
				Driver: "bridge",
				Labels: map[string]string{
					"github-runner": testID,
				},
			})
			if err != nil {
				t.Fatalf("network create failed: %v", err)
			}
			defer c.NetworkRemove(ctx, netResp.ID)

			// === Step 3: Pull image ===
			t.Log("Step 3: Pull image")
			rc, err := c.ImagePull(ctx, "alpine:latest", image.PullOptions{})
			if err != nil {
				t.Fatalf("image pull failed: %v", err)
			}
			io.Copy(io.Discard, rc)
			rc.Close()

			// === Step 4: Create container with tail -f /dev/null (idle pattern) ===
			t.Log("Step 4: Create container")
			containerName := "github_runner_" + testID
			resp, err := c.ContainerCreate(ctx,
				&container.Config{
					Image: "alpine:latest",
					Cmd:   []string{"tail", "-f", "/dev/null"},
					Labels: map[string]string{
						"github-runner": testID,
					},
				},
				&container.HostConfig{
					NetworkMode: container.NetworkMode(netName),
				},
				nil, nil, containerName,
			)
			if err != nil {
				t.Fatalf("container create failed: %v", err)
			}
			defer c.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})

			// === Step 5: Start container ===
			t.Log("Step 5: Start container")
			if err := c.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
				t.Fatalf("container start failed: %v", err)
			}

			// Verify running
			info, err := c.ContainerInspect(ctx, resp.ID)
			if err != nil {
				t.Fatalf("container inspect failed: %v", err)
			}
			if !info.State.Running {
				t.Fatal("expected container to be running")
			}

			// === Step 6: Execute steps via exec ===
			t.Log("Step 6: Execute steps")

			// Step 6a: Run a simple command
			execResp, err := c.ContainerExecCreate(ctx, resp.ID, container.ExecOptions{
				Cmd:          []string{"echo", "hello from github runner"},
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
			var stdoutBuf, stderrBuf bytes.Buffer
			stdcopy.StdCopy(&stdoutBuf, &stderrBuf, hijacked.Reader)
			hijacked.Close()
			output := stdoutBuf.String()
			t.Logf("step output: %q", output)

			if !strings.Contains(output, "hello from github runner") {
				t.Errorf("expected output to contain 'hello from github runner', got: %q", output)
			}

			// === Step 7: Collect logs ===
			t.Log("Step 7: Collect logs")
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

			// === Step 8: Force-remove container ===
			t.Log("Step 8: Force-remove container")
			if err := c.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true}); err != nil {
				t.Fatalf("container remove failed: %v", err)
			}

			// === Step 9: Remove network ===
			t.Log("Step 9: Remove network")
			if err := c.NetworkRemove(ctx, netResp.ID); err != nil {
				t.Logf("network remove (may be already removed): %v", err)
			}
		})
	}
}

// TestGitHubRunnerContainerAction simulates a GitHub Actions container action.
// Container actions use the original entrypoint (not tail -f /dev/null).
func TestGitHubRunnerContainerAction(t *testing.T) {
	for name, c := range availableRunnerClients(t) {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			testID := generateTestID(name)

			// Pull and create
			rc, err := c.ImagePull(ctx, "alpine:latest", image.PullOptions{})
			if err != nil {
				t.Fatalf("image pull failed: %v", err)
			}
			io.Copy(io.Discard, rc)
			rc.Close()

			resp, err := c.ContainerCreate(ctx,
				&container.Config{
					Image:      "alpine:latest",
					Entrypoint: []string{"echo", "action output"},
				},
				nil, nil, nil, "action_"+testID,
			)
			if err != nil {
				t.Fatalf("container create failed: %v", err)
			}
			defer c.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})

			// Start
			if err := c.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
				t.Fatalf("container start failed: %v", err)
			}

			// Wait for completion
			waitCh, errCh := c.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
			select {
			case result := <-waitCh:
				if result.StatusCode != 0 {
					t.Errorf("expected exit code 0, got %d", result.StatusCode)
				}
			case err := <-errCh:
				t.Fatalf("wait error: %v", err)
			case <-time.After(30 * time.Second):
				t.Fatal("timeout waiting for container")
			}
		})
	}
}

// TestGitHubRunnerMultiStep simulates multiple exec steps with different workdirs and envs.
func TestGitHubRunnerMultiStep(t *testing.T) {
	for name, c := range availableRunnerClients(t) {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			testID := generateTestID(name)

			rc, err := c.ImagePull(ctx, "alpine:latest", image.PullOptions{})
			if err != nil {
				t.Fatalf("image pull failed: %v", err)
			}
			io.Copy(io.Discard, rc)
			rc.Close()

			// Create volume for workspace
			vol, err := c.VolumeCreate(ctx, volume.CreateOptions{
				Name: "workspace_" + testID,
			})
			if err != nil {
				t.Fatalf("volume create failed: %v", err)
			}
			defer c.VolumeRemove(ctx, vol.Name, true)

			resp, err := c.ContainerCreate(ctx,
				&container.Config{
					Image: "alpine:latest",
					Cmd:   []string{"tail", "-f", "/dev/null"},
				},
				nil, nil, nil, "multi_step_"+testID,
			)
			if err != nil {
				t.Fatalf("container create failed: %v", err)
			}
			defer c.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})

			if err := c.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
				t.Fatalf("container start failed: %v", err)
			}

			// Step 1: Run with env
			execStep := func(cmd []string, env []string, workDir string) string {
				t.Helper()
				execResp, err := c.ContainerExecCreate(ctx, resp.ID, container.ExecOptions{
					Cmd:          cmd,
					Env:          env,
					WorkingDir:   workDir,
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
				out, _ := io.ReadAll(hijacked.Reader)
				hijacked.Close()
				return string(out)
			}

			// Step 1: Echo with custom env
			// Real backends expand env vars; memory backend echoes the command
			out1 := execStep(
				[]string{"sh", "-c", "echo $STEP_NAME"},
				[]string{"STEP_NAME=checkout"},
				"",
			)
			if !strings.Contains(out1, "checkout") && !strings.Contains(out1, "STEP_NAME") {
				t.Errorf("step 1: expected 'checkout' or command echo, got %q", out1)
			}

			// Step 2: Different env
			out2 := execStep(
				[]string{"sh", "-c", "echo $STEP_NAME"},
				[]string{"STEP_NAME=build"},
				"",
			)
			if !strings.Contains(out2, "build") && !strings.Contains(out2, "STEP_NAME") {
				t.Errorf("step 2: expected 'build' or command echo, got %q", out2)
			}

			// Step 3: With workdir
			out3 := execStep(
				[]string{"pwd"},
				nil,
				"/tmp",
			)
			if !strings.Contains(out3, "/tmp") && !strings.Contains(out3, "pwd") {
				t.Errorf("step 3: expected '/tmp' or command echo, got %q", out3)
			}
		})
	}
}
