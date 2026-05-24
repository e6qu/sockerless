package azf

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"
)

func TestAZFFaaSE2ESmoke(t *testing.T) {
	ctx := context.Background()

	testID := generateTestID()
	resp, err := dockerClient.ContainerCreate(ctx,
		&container.Config{
			Image: alpineImageName,
			// Shell traps SIGTERM → exit 0. `sleep 600 & wait`
			// keeps the shell alive but lets the trap fire while
			// idle. Termination is driven by the test's
			// ContainerStop call below; signalling externally
			// keeps the exec processes alive long enough to
			// report their exit status.
			Cmd: []string{"sh", "-c", "trap 'exit 0' TERM; sleep 600 & wait"},
		},
		nil, nil, nil, "azf_faas_smoke_"+testID,
	)
	if err != nil {
		t.Fatalf("container create failed: %v", err)
	}
	t.Cleanup(func() { _ = dockerClient.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true}) })

	if err := dockerClient.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		t.Fatalf("container start failed: %v", err)
	}

	runAZFSmokeExec(t, ctx, resp.ID, []string{"sh", "-c", "printf azf-step-1"}, "azf-step-1")
	runAZFSmokeExec(t, ctx, resp.ID, []string{"sh", "-c", "printf azf-step-2"}, "azf-step-2")

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

	// The AZF backend deletes the underlying function app when the
	// container stops, so the container ID is already gone by the
	// time the test's t.Cleanup runs (Force: true is idempotent
	// on 404).
}

func runAZFSmokeExec(t *testing.T, ctx context.Context, containerID string, cmd []string, wantStdout string) {
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
