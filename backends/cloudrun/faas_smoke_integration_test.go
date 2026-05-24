package cloudrun

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/pkg/stdcopy"
)

func TestCloudRunFaaSE2ESmoke(t *testing.T) {
	if os.Getenv(cloudRunExecE2EEnv) != "1" {
		cmd := exec.Command(os.Args[0], "-test.run", "^TestCloudRunFaaSE2ESmoke$", "-test.v")
		cmd.Env = append(os.Environ(), cloudRunExecE2EEnv+"=1")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Cloud Run FaaS smoke subprocess failed: %v\n%s", err, string(out))
		}
		return
	}

	ctx := context.Background()

	rc, err := dockerClient.ImagePull(ctx, "alpine:latest", image.PullOptions{})
	if err != nil {
		t.Fatalf("image pull failed: %v", err)
	}
	io.Copy(io.Discard, rc)
	rc.Close()

	testID := generateTestID()
	// External-signal termination (ContainerStop → SIGTERM) instead
	// of filesystem polling. The exec helpers verify their own
	// stdout/exit code; the test code then issues ContainerStop, the
	// container's shell receives SIGTERM, the `trap 'exit 0' TERM`
	// handler fires and exits cleanly. This avoids the previous
	// `kill 1` from inside the exec, which killed PID 1 before the
	// exec process could report its exit status (Docker then reported
	// the exec as exit code -1).
	resp, err := dockerClient.ContainerCreate(ctx,
		&container.Config{
			Image: "alpine:latest",
			Cmd:   []string{"sh", "-c", "trap 'exit 0' TERM; sleep 600 & wait"},
		},
		nil, nil, nil, "cloudrun_faas_smoke_"+testID,
	)
	if err != nil {
		t.Fatalf("container create failed: %v", err)
	}
	t.Cleanup(func() { _ = dockerClient.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true}) })

	startCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	if err := dockerClient.ContainerStart(startCtx, resp.ID, container.StartOptions{}); err != nil {
		t.Fatalf("container start failed: %v", err)
	}

	runCloudRunSmokeExec(t, ctx, resp.ID, []string{"sh", "-c", "printf cloudrun-step-1"}, "cloudrun-step-1")
	runCloudRunSmokeExec(t, ctx, resp.ID, []string{"sh", "-c", "printf cloudrun-step-2"}, "cloudrun-step-2")

	waitCh, errCh := dockerClient.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	stopTimeout := 2
	if err := dockerClient.ContainerStop(ctx, resp.ID, container.StopOptions{Timeout: &stopTimeout}); err != nil {
		t.Fatalf("container stop failed: %v", err)
	}
	select {
	case result := <-waitCh:
		if result.StatusCode != 0 {
			t.Fatalf("wait status = %d, want 0 (trap caught SIGTERM)", result.StatusCode)
		}
	case err := <-errCh:
		t.Fatalf("container wait error: %v", err)
	case <-time.After(30 * time.Second):
		t.Fatal("timeout waiting for container exit (30s after ContainerStop SIGTERM)")
	}

	// Cloud Run's backend deletes the underlying service when the
	// container stops, so the container ID is already gone by the
	// time the test's t.Cleanup runs (with Force: true, which is
	// idempotent on 404). No explicit ContainerRemove here.
}

func runCloudRunSmokeExec(t *testing.T, ctx context.Context, containerID string, cmd []string, wantStdout string) {
	t.Helper()

	execResp, err := dockerClient.ContainerExecCreate(ctx, containerID, container.ExecOptions{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		t.Fatalf("exec create failed: %v", err)
	}
	if execResp.ID == "" {
		t.Fatal("expected non-empty exec ID")
	}

	hijacked, err := dockerClient.ContainerExecAttach(ctx, execResp.ID, container.ExecAttachOptions{})
	if err != nil {
		t.Fatalf("exec attach failed: %v", err)
	}
	defer hijacked.Close()

	var stdout, stderr bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdout, &stderr, hijacked.Reader); err != nil {
		t.Fatalf("exec stream copy failed: %v", err)
	}
	if got := stdout.String(); got != wantStdout {
		t.Fatalf("exec stdout = %q, want %q, stderr = %q", got, wantStdout, stderr.String())
	}

	inspect, err := dockerClient.ContainerExecInspect(ctx, execResp.ID)
	if err != nil {
		t.Fatalf("exec inspect failed: %v", err)
	}
	if inspect.ExitCode != 0 {
		t.Fatalf("exec exit code = %d", inspect.ExitCode)
	}
}
