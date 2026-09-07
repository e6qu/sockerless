//go:build integration

package ecs

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

// startBackendWithEnv spawns an additional sockerless-backend-ecs
// process with the harness's base config plus extraEnv, and returns a
// docker client wired to it. Used for process-level config that can't
// be set per-request (SOCKERLESS_ECS_SHARED_VOLUMES).
func startBackendWithEnv(t *testing.T, extraEnv ...string) *client.Client {
	t.Helper()
	port := findFreePort()
	cmd := exec.Command(backendBinaryPath, "--addr", fmt.Sprintf(":%d", port), "--log-level", "debug")
	cmd.Env = append(append(os.Environ(), backendBaseEnv...), extraEnv...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start backend with env %v: %v", extraEnv, err)
	}
	t.Cleanup(func() {
		cmd.Process.Kill()
		cmd.Wait()
	})
	if err := waitForReady(fmt.Sprintf("http://localhost:%d/internal/v1/info", port), 15*time.Second); err != nil {
		t.Fatalf("backend with env %v not ready: %v", extraEnv, err)
	}
	cli, err := client.New(
		client.WithHost(fmt.Sprintf("tcp://localhost:%d", port)),
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		t.Fatalf("docker client: %v", err)
	}
	return cli
}

func pullAlpine(t *testing.T, cli *client.Client) {
	t.Helper()
	rc, err := cli.ImagePull(context.Background(), "alpine:latest", client.ImagePullOptions{})
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
}

func runToCompletion(t *testing.T, cli *client.Client, name string, cmd []string, binds []string) string {
	t.Helper()
	ctx := context.Background()
	resp, err := cli.ContainerCreate(ctx, client.ContainerCreateOptions{Config: &container.Config{
		Image: "alpine:latest",
		Cmd:   cmd,
	}, HostConfig: &container.HostConfig{Binds: binds}, Name: name})
	if err != nil {
		t.Fatalf("container create (%s) failed: %v", name, err)
	}
	t.Cleanup(func() {
		_, _ = cli.ContainerRemove(ctx, resp.ID, client.ContainerRemoveOptions{Force: true})
	})
	if _, err := cli.ContainerStart(ctx, resp.ID, client.ContainerStartOptions{}); err != nil {
		t.Fatalf("container start (%s) failed: %v", name, err)
	}
	waited := cli.ContainerWait(ctx, resp.ID, client.ContainerWaitOptions{Condition: container.WaitConditionNotRunning})
	waitCh, errCh := waited.Result, waited.Error
	select {
	case result := <-waitCh:
		if result.StatusCode != 0 {
			t.Fatalf("container %s exited with %d, want 0", name, result.StatusCode)
		}
	case err := <-errCh:
		t.Fatalf("container wait (%s) error: %v", name, err)
	case <-time.After(5 * time.Minute):
		t.Fatalf("timeout waiting for container %s", name)
	}
	logRC, err := cli.ContainerLogs(ctx, resp.ID, client.ContainerLogsOptions{ShowStdout: true, ShowStderr: true})
	if err != nil {
		t.Fatalf("logs (%s) failed: %v", name, err)
	}
	defer logRC.Close()
	// Read the FULL demuxed stream: a single Read() can return only the sim's
	// first frame and miss the script's output in a later frame (CI-flaky),
	// which read as a shared-workspace failure even when the volume was shared.
	var out bytes.Buffer
	_, _ = stdcopy.StdCopy(&out, &out, logRC)
	return out.String()
}

// TestECSSharedVolumeWorkspaceSharing proves the runner-workspace
// sharing contract end-to-end: a writer task using a named volume and
// a reader task using a HOST BIND whose source path is configured in
// SOCKERLESS_ECS_SHARED_VOLUMES (pointing at the same EFS access
// point) observe the same files. This is exactly the GitHub Actions
// runner shape — the runner task writes the workspace, the job
// container reads it through a `docker create -v /home/runner/_work:/__w`
// bind that sockerless translates to the shared named volume.
func TestECSSharedVolumeWorkspaceSharing(t *testing.T) {
	ctx := context.Background()
	const runnerWork = "/home/runner/_work"

	// Provision the shared workspace volume (EFS access point) through
	// the primary backend's real volume path — no fixture shortcuts.
	volName := "shared-ws-" + generateTestID()
	volCreated, err := dockerClient.VolumeCreate(ctx, client.VolumeCreateOptions{Name: volName})
	vol := volCreated.Volume
	if err != nil {
		t.Fatalf("volume create failed: %v", err)
	}
	t.Cleanup(func() {
		_, _ = dockerClient.VolumeRemove(ctx, volName, client.VolumeRemoveOptions{Force: true})
	})
	apID := vol.Options["accessPointId"]
	fsID := vol.Options["fileSystemId"]
	if apID == "" || fsID == "" {
		t.Fatalf("volume %s missing accessPointId/fileSystemId in Options: %+v", volName, vol.Options)
	}

	// Spawn a backend configured the way a runner task's sockerless
	// sidecar would be: the runner's workspace path maps onto the
	// shared volume's access point.
	sharedSpec := fmt.Sprintf("%s=%s=%s=%s", volName, runnerWork, apID, fsID)
	cli := startBackendWithEnv(t, "SOCKERLESS_ECS_SHARED_VOLUMES="+sharedSpec)
	pullAlpine(t, cli)

	// Writer (the "runner" side): named-volume bind, writes the
	// workspace marker file.
	payload := "runner-workspace-payload-" + generateTestID()
	runToCompletion(t, cli, "shared-vol-writer-"+generateTestID(),
		[]string{"sh", "-c", fmt.Sprintf("echo %s > /work/from-runner.txt", payload)},
		[]string{volName + ":/work"},
	)

	// Reader (the "job container" side): HOST BIND on the configured
	// workspace path + a redundant sub-path bind + docker.sock — the
	// exact bind set the GitHub runner issues. The sub-path and
	// docker.sock binds must drop; the workspace bind must translate
	// to the shared named volume.
	logs := runToCompletion(t, cli, "shared-vol-reader-"+generateTestID(),
		[]string{"sh", "-c", "cat /data/from-runner.txt"},
		[]string{
			runnerWork + ":/data",
			runnerWork + "/_temp:/scratch",
			"/var/run/docker.sock:/var/run/docker.sock",
		},
	)
	if !strings.Contains(logs, payload) {
		t.Fatalf("reader logs %q do not contain writer payload %q — workspace not shared", logs, payload)
	}

	// Unmapped host bind must reject with the configure-this hint, not
	// silently convert.
	_, err = cli.ContainerCreate(ctx, client.ContainerCreateOptions{Config: &container.Config{
		Image: "alpine:latest",
		Cmd:   []string{"true"},
	}, HostConfig: &container.HostConfig{Binds: []string{"/not/mapped:/x"}}, Name: "shared-vol-reject-" + generateTestID()})
	if err == nil {
		t.Fatal("container create with unmapped host bind succeeded, want rejection")
	}
	if !strings.Contains(err.Error(), "SOCKERLESS_ECS_SHARED_VOLUMES") {
		t.Fatalf("rejection %q does not mention SOCKERLESS_ECS_SHARED_VOLUMES", err)
	}
}
