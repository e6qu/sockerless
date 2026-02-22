package tests

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
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

			// === Phase 1: Pull images ===
			t.Log("Phase 1: Pulling images")
			pullImg := func(ref string) {
				rc, err := c.ImagePull(ctx, ref, image.PullOptions{})
				if err != nil {
					t.Fatalf("failed to pull %s: %v", ref, err)
				}
				defer rc.Close()
				io.Copy(io.Discard, rc)
			}
			pullImg("alpine:latest")

			// === Phase 2: Create network ===
			t.Log("Phase 2: Creating network")
			netResp, err := c.NetworkCreate(ctx, "runner-net-"+testID, network.CreateOptions{
				Driver: "bridge",
			})
			if err != nil {
				t.Fatalf("network create failed: %v", err)
			}
			defer c.NetworkRemove(ctx, netResp.ID)

			// === Phase 3: Create build container (with attach-before-start pattern) ===
			t.Log("Phase 3: Creating build container")
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

			// === Phase 4: Attach to build container BEFORE start (GitLab Runner pattern) ===
			t.Log("Phase 4: Attaching to build container (before start)")
			attachDone := make(chan struct{})
			go func() {
				defer close(attachDone)
				_, _ = c.ContainerAttach(ctx, buildResp.ID, container.AttachOptions{
					Stream: true,
					Stdin:  true,
					Stdout: true,
					Stderr: true,
				})
			}()

			// Give attach time to register
			time.Sleep(200 * time.Millisecond)

			// === Phase 5: Start build container ===
			t.Log("Phase 5: Starting build container")
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

			// === Phase 6: Exec commands (simulating git clone + script execution) ===
			t.Log("Phase 6: Executing commands")

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

				output, _ := io.ReadAll(hijacked.Reader)
				return string(output)
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

			// === Phase 7: Stop build container ===
			t.Log("Phase 7: Stopping build container")
			timeout := 10
			if err := c.ContainerStop(ctx, buildResp.ID, container.StopOptions{Timeout: &timeout}); err != nil {
				t.Logf("container stop error (may be expected): %v", err)
			}

			// === Phase 8: Wait for container ===
			t.Log("Phase 8: Waiting for container exit")
			waitCh, errCh := c.ContainerWait(ctx, buildResp.ID, container.WaitConditionNotRunning)
			select {
			case result := <-waitCh:
				t.Logf("Container exited with code: %d", result.StatusCode)
			case err := <-errCh:
				t.Logf("Container wait error: %v", err)
			case <-time.After(30 * time.Second):
				t.Log("Timeout waiting for container — proceeding with cleanup")
			}

			// === Phase 9: Cleanup ===
			t.Log("Phase 9: Cleanup")

			// List containers with label filter to verify our container is tracked
			containers, err := c.ContainerList(ctx, container.ListOptions{All: true})
			if err != nil {
				t.Logf("container list error: %v", err)
			} else {
				found := false
				for _, ctr := range containers {
					if ctr.ID == buildResp.ID {
						found = true
						break
					}
				}
				if !found {
					t.Log("WARNING: build container not found in list (may have been auto-removed)")
				}
			}

			// Remove build container
			c.ContainerRemove(ctx, buildResp.ID, container.RemoveOptions{Force: true})

			// Remove network
			c.NetworkRemove(ctx, netResp.ID)

			t.Log("GitLab Runner E2E flow completed successfully")
		})
	}
}

// TestGitLabRunnerMultiStageJob simulates a multi-stage CI job.
func TestGitLabRunnerMultiStageJob(t *testing.T) {
	for name, c := range availableRunnerClients(t) {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			testID := generateTestID(name)

			pullRC, _ := c.ImagePull(ctx, "alpine:latest", image.PullOptions{})
			if pullRC != nil {
				io.Copy(io.Discard, pullRC)
				pullRC.Close()
			}

			// Create a shared volume
			vol, err := c.VolumeCreate(ctx, volume.CreateOptions{Name: "runner-cache-" + testID})
			if err != nil {
				t.Fatalf("volume create failed: %v", err)
			}
			defer c.VolumeRemove(ctx, vol.Name, true)

			// Stage 1: Build
			t.Log("Stage 1: Build")
			buildResp, err := c.ContainerCreate(ctx, &container.Config{
				Image: "alpine:latest",
				Cmd:   []string{"sh", "-c", "echo 'build artifacts' > /cache/artifacts.txt && cat /cache/artifacts.txt"},
			}, &container.HostConfig{
				Binds: []string{vol.Name + ":/cache"},
			}, nil, nil, "stage-build-"+testID)
			if err != nil {
				t.Fatalf("stage 1 create failed: %v", err)
			}
			defer c.ContainerRemove(ctx, buildResp.ID, container.RemoveOptions{Force: true})

			c.ContainerStart(ctx, buildResp.ID, container.StartOptions{})
			waitCh, _ := c.ContainerWait(ctx, buildResp.ID, container.WaitConditionNotRunning)
			select {
			case result := <-waitCh:
				if result.StatusCode != 0 {
					t.Errorf("stage 1 exit code: %d", result.StatusCode)
				}
			case <-time.After(5 * time.Minute):
				t.Fatal("stage 1 timeout")
			}

			// Stage 2: Test (uses artifacts from stage 1)
			t.Log("Stage 2: Test")
			testResp, err := c.ContainerCreate(ctx, &container.Config{
				Image: "alpine:latest",
				Cmd:   []string{"sh", "-c", "cat /cache/artifacts.txt"},
			}, &container.HostConfig{
				Binds: []string{vol.Name + ":/cache"},
			}, nil, nil, "stage-test-"+testID)
			if err != nil {
				t.Fatalf("stage 2 create failed: %v", err)
			}
			defer c.ContainerRemove(ctx, testResp.ID, container.RemoveOptions{Force: true})

			c.ContainerStart(ctx, testResp.ID, container.StartOptions{})
			waitCh, _ = c.ContainerWait(ctx, testResp.ID, container.WaitConditionNotRunning)
			select {
			case result := <-waitCh:
				if result.StatusCode != 0 {
					t.Errorf("stage 2 exit code: %d", result.StatusCode)
				}
			case <-time.After(5 * time.Minute):
				t.Fatal("stage 2 timeout")
			}

			t.Log("Multi-stage job completed successfully")
		})
	}
}
