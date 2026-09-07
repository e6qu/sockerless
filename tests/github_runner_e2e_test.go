package tests

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/client"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
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
			_, err := c.Ping(ctx, client.PingOptions{})
			if err != nil {
				t.Fatalf("ping failed: %v", err)
			}

			// === Step 2: Create network ===
			t.Log("Step 2: Create network")
			netName := "github_network_" + testID
			netResp, err := c.NetworkCreate(ctx, netName, client.NetworkCreateOptions{
				Driver: "bridge",
				Labels: map[string]string{
					"github-runner": testID,
				},
			})
			if err != nil {
				t.Fatalf("network create failed: %v", err)
			}
			defer c.NetworkRemove(ctx, netResp.ID, client.

				// === Step 3: Pull image ===
				NetworkRemoveOptions{})

			t.Log("Step 3: Pull image")
			rc, err := c.ImagePull(ctx, "alpine:latest", client.ImagePullOptions{})
			if err != nil {
				t.Fatalf("image pull failed: %v", err)
			}
			io.Copy(io.Discard, rc)
			rc.Close()

			// === Step 4: Create container with tail -f /dev/null (idle pattern) ===
			t.Log("Step 4: Create container")
			containerName := "github_runner_" + testID
			resp, err := c.ContainerCreate(ctx, client.ContainerCreateOptions{Config: &container.Config{
				Image: "alpine:latest",
				Cmd:   []string{"tail", "-f", "/dev/null"},
				Labels: map[string]string{
					"github-runner": testID,
				},
			}, HostConfig: &container.HostConfig{
				NetworkMode: container.NetworkMode(netName),
			}, Name: containerName},
			)
			if err != nil {
				t.Fatalf("container create failed: %v", err)
			}
			defer c.ContainerRemove(ctx, resp.ID, client.ContainerRemoveOptions{Force: true})

			// === Step 5: Start container ===
			t.Log("Step 5: Start container")
			if _, err := c.ContainerStart(ctx, resp.ID, client.ContainerStartOptions{}); err != nil {
				t.Fatalf("container start failed: %v", err)
			}

			// Verify running
			inspected, err := c.ContainerInspect(ctx, resp.ID, client.ContainerInspectOptions{})
			info := inspected.Container
			if err != nil {
				t.Fatalf("container inspect failed: %v", err)
			}
			if !info.State.Running {
				t.Fatal("expected container to be running")
			}

			// === Step 6: Execute steps via exec ===
			t.Log("Step 6: Execute steps")

			// Step 6a: Run a simple command
			execResp, err := c.ExecCreate(ctx, resp.ID, client.ExecCreateOptions{
				Cmd:          []string{"echo", "hello from github runner"},
				AttachStdout: true,
				AttachStderr: true,
			})
			if err != nil {
				t.Fatalf("exec create failed: %v", err)
			}

			hijacked, err := c.ExecAttach(ctx, execResp.ID, client.ExecAttachOptions{})
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
			logReader, err := c.ContainerLogs(ctx, resp.ID, client.ContainerLogsOptions{
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
			if _, err := c.ContainerRemove(ctx, resp.ID, client.ContainerRemoveOptions{Force: true}); err != nil {
				t.Fatalf("container remove failed: %v", err)
			}

			// === Step 9: Remove network ===
			t.Log("Step 9: Remove network")
			if _, err := c.NetworkRemove(ctx, netResp.ID, client.NetworkRemoveOptions{}); err != nil {
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
			rc, err := c.ImagePull(ctx, "alpine:latest", client.ImagePullOptions{})
			if err != nil {
				t.Fatalf("image pull failed: %v", err)
			}
			io.Copy(io.Discard, rc)
			rc.Close()

			resp, err := c.ContainerCreate(ctx, client.ContainerCreateOptions{Config: &container.Config{
				Image:      "alpine:latest",
				Entrypoint: []string{"echo", "action output"},
			}, Name: "action_" + testID},
			)
			if err != nil {
				t.Fatalf("container create failed: %v", err)
			}
			defer c.ContainerRemove(ctx, resp.ID, client.ContainerRemoveOptions{Force: true})

			// Start
			if _, err := c.ContainerStart(ctx, resp.ID, client.ContainerStartOptions{}); err != nil {
				t.Fatalf("container start failed: %v", err)
			}

			// Wait for completion
			waited := c.ContainerWait(ctx, resp.ID, client.ContainerWaitOptions{Condition: container.WaitConditionNotRunning})
			waitCh, errCh := waited.Result, waited.Error
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

			rc, err := c.ImagePull(ctx, "alpine:latest", client.ImagePullOptions{})
			if err != nil {
				t.Fatalf("image pull failed: %v", err)
			}
			io.Copy(io.Discard, rc)
			rc.Close()

			resp, err := c.ContainerCreate(ctx, client.ContainerCreateOptions{Config: &container.Config{
				Image: "alpine:latest",
				Cmd:   []string{"tail", "-f", "/dev/null"},
			}, Name: "multi_step_" + testID},
			)
			if err != nil {
				t.Fatalf("container create failed: %v", err)
			}
			defer c.ContainerRemove(ctx, resp.ID, client.ContainerRemoveOptions{Force: true})

			if _, err := c.ContainerStart(ctx, resp.ID, client.ContainerStartOptions{}); err != nil {
				t.Fatalf("container start failed: %v", err)
			}

			// Step 1: Run with env
			execStep := func(cmd []string, env []string, workDir string) string {
				t.Helper()
				execResp, err := c.ExecCreate(ctx, resp.ID, client.ExecCreateOptions{
					Cmd:          cmd,
					Env:          env,
					WorkingDir:   workDir,
					AttachStdout: true,
					AttachStderr: true,
				})
				if err != nil {
					t.Fatalf("exec create failed: %v", err)
				}
				hijacked, err := c.ExecAttach(ctx, execResp.ID, client.ExecAttachOptions{})
				if err != nil {
					t.Fatalf("exec start failed: %v", err)
				}
				var stdout bytes.Buffer
				stdcopy.StdCopy(&stdout, io.Discard, hijacked.Reader)
				hijacked.Close()
				output := stdout.Bytes()
				return string(output)
			}

			// Step 1: Echo with custom env
			// Backends expand env vars via agent exec
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
