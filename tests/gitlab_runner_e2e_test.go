package tests

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/pkg/stdcopy"
)

// TestGitLabRunnerDockerExecutorFlow simulates a complete GitLab Runner docker-executor job.
// This follows the exact sequence of Docker API calls that GitLab Runner makes:
//
//  1. Pull images (service + build)
//  2. Create network
//  3. Create + start service containers
//  4. Create build container
//  5. Attach to build container (before start!)
//  6. Start build container
//  7. Exec commands in build container (clone, scripts)
//  8. Wait for build container
//  9. Cleanup: stop/remove containers, remove network, remove volumes
func TestGitLabRunnerDockerExecutorFlow(t *testing.T) {
	for name, c := range availableRunnerClients(t) {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			testID := generateTestID(name)

			// === Step 1: Pull images ===
			t.Log("Step 1: Pulling images")
			pullImg := func(ref string) {
				rc, err := c.ImagePull(ctx, ref, image.PullOptions{})
				if err != nil {
					t.Fatalf("failed to pull %s: %v", ref, err)
				}
				defer rc.Close()
				io.Copy(io.Discard, rc)
			}
			pullImg("alpine:latest")

			// === Step 2: Create network ===
			t.Log("Step 2: Creating network")
			netResp, err := c.NetworkCreate(ctx, "runner-net-"+testID, network.CreateOptions{
				Driver: "bridge",
			})
			if err != nil {
				t.Fatalf("network create failed: %v", err)
			}
			defer c.NetworkRemove(ctx, netResp.ID)

			// === Step 3: Create build container (with attach-before-start pattern) ===
			t.Log("Step 3: Creating build container")
			buildResp, err := c.ContainerCreate(ctx, &container.Config{
				Image:     "alpine:latest",
				Cmd:       []string{"tail", "-f", "/dev/null"},
				OpenStdin: true,
				Tty:       false,
				Labels: map[string]string{
					"com.gitlab.runner.job":  "test-job-1",
					"com.gitlab.runner.type": "build",
				},
			}, nil, nil, nil, "runner-build-"+testID)
			if err != nil {
				t.Fatalf("build container create failed: %v", err)
			}
			defer c.ContainerRemove(ctx, buildResp.ID, container.RemoveOptions{Force: true})

			// === Step 4: Attach to build container BEFORE start (GitLab Runner pattern) ===
			t.Log("Step 4: Attaching to build container (before start)")
			attachDone := make(chan error, 1)
			var attachConn types.HijackedResponse
			go func() {
				conn, err := c.ContainerAttach(ctx, buildResp.ID, container.AttachOptions{
					Stream: true,
					Stdin:  true,
					Stdout: true,
					Stderr: true,
				})
				attachConn = conn
				attachDone <- err
			}()

			// Synchronize on the attach result instead of a bare sleep: the
			// attach-before-start contract is the point of this test, so a
			// failure here must fail the test, not be silently slept past.
			select {
			case err := <-attachDone:
				if err != nil {
					t.Fatalf("attach-before-start failed: %v", err)
				}
				defer attachConn.Close()
			case <-time.After(30 * time.Second):
				t.Fatal("timeout waiting for attach-before-start to return")
			}

			// === Step 5: Start build container ===
			t.Log("Step 5: Starting build container")
			if err := c.ContainerStart(ctx, buildResp.ID, container.StartOptions{}); err != nil {
				t.Fatalf("build container start failed: %v", err)
			}

			// Verify container is running
			info, err := c.ContainerInspect(ctx, buildResp.ID)
			if err != nil {
				t.Fatalf("inspect failed: %v", err)
			}
			if !info.State.Running {
				t.Fatalf("expected container to be running, status: %s", info.State.Status)
			}

			// === Step 6: Exec commands (simulating git clone + script execution) ===
			t.Log("Step 6: Executing commands")

			// Exec 1: Setup script
			execAndWait := func(execName string, cmd []string) string {
				execResp, err := c.ContainerExecCreate(ctx, buildResp.ID, container.ExecOptions{
					Cmd:          cmd,
					AttachStdout: true,
					AttachStderr: true,
				})
				if err != nil {
					t.Fatalf("exec create (%s) failed: %v", execName, err)
				}

				hijacked, err := c.ContainerExecAttach(ctx, execResp.ID, container.ExecStartOptions{})
				if err != nil {
					t.Fatalf("exec attach (%s) failed: %v", execName, err)
				}
				defer hijacked.Close()

				var stdout bytes.Buffer
				stdcopy.StdCopy(&stdout, io.Discard, hijacked.Reader)
				return stdout.String()
			}

			// Simulate: git clone
			output := execAndWait("clone", []string{"echo", "Cloning repository..."})
			t.Logf("Clone output: %q", output)

			// Simulate: run script
			output = execAndWait("script", []string{"sh", "-c", "echo 'Running CI script' && echo 'Tests passed!'"})
			t.Logf("Script output: %q", output)
			if !strings.Contains(output, "Tests passed") {
				t.Errorf("expected script output to contain 'Tests passed', got %q", output)
			}

			// Simulate: upload artifacts
			output = execAndWait("artifacts", []string{"echo", "Uploading artifacts..."})
			t.Logf("Artifacts output: %q", output)

			// === Step 7: Stop build container ===
			t.Log("Step 7: Stopping build container")
			timeout := 10
			if err := c.ContainerStop(ctx, buildResp.ID, container.StopOptions{Timeout: &timeout}); err != nil {
				t.Fatalf("container stop failed: %v", err)
			}

			// === Step 8: Wait for container ===
			// The stateless docker wait path is exactly what this test exists to
			// exercise: a real StatusCode must come back, and a timeout means the
			// backend never reported the container stopped — a failure, not a pass.
			t.Log("Step 8: Waiting for container exit")
			waitCh, errCh := c.ContainerWait(ctx, buildResp.ID, container.WaitConditionNotRunning)
			select {
			case result := <-waitCh:
				t.Logf("Container exited with code: %d", result.StatusCode)
			case err := <-errCh:
				t.Fatalf("container wait error: %v", err)
			case <-time.After(30 * time.Second):
				t.Fatal("timeout waiting for container to stop")
			}

			// === Step 9: Cleanup ===
			t.Log("Step 9: Cleanup")

			// The build container must still be tracked by the cloud-backed
			// docker ps — its absence is a stateless-listing failure, not a
			// benign auto-removal (this container has no AutoRemove set).
			containers, err := c.ContainerList(ctx, container.ListOptions{All: true})
			if err != nil {
				t.Fatalf("container list failed: %v", err)
			}
			found := false
			for _, ctr := range containers {
				if ctr.ID == buildResp.ID {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("build container %s missing from ContainerList", buildResp.ID)
			}

			// Remove build container
			if err := c.ContainerRemove(ctx, buildResp.ID, container.RemoveOptions{Force: true}); err != nil {
				t.Errorf("container remove failed: %v", err)
			}

			// Remove network
			if err := c.NetworkRemove(ctx, netResp.ID); err != nil {
				t.Errorf("network remove failed: %v", err)
			}

			t.Log("GitLab Runner E2E flow completed successfully")
		})
	}
}

// TestGitLabRunnerMultiStageJob is intentionally absent: the
// realistic multi-stage GitLab CI flow depends on a shared cache
// volume across stages. Once real EFS-backed shared cache support
// lands across the runner backends, re-add this test.
