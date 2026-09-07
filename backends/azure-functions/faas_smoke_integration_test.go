//go:build integration

package azf

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/moby/moby/client"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
)

func TestAZFFaaSE2ESmoke(t *testing.T) {
	ctx := context.Background()

	testID := generateTestID()
	resp, err := dockerClient.ContainerCreate(ctx, client.ContainerCreateOptions{Config: &container.Config{
		Image: alpineImageName,
		// Shell traps SIGTERM → exit 0. `sleep 600 & wait`
		// keeps the shell alive but lets the trap fire while
		// idle. Termination is driven by the test's
		// ContainerStop call below; signalling externally
		// keeps the exec processes alive long enough to
		// report their exit status.
		Cmd: []string{"sh", "-c", "trap 'exit 0' TERM; sleep 600 & wait"},
	}, Name: "azf_faas_smoke_" + testID},
	)
	if err != nil {
		t.Fatalf("container create failed: %v", err)
	}
	t.Cleanup(func() { _, _ = dockerClient.ContainerRemove(ctx, resp.ID, client.ContainerRemoveOptions{Force: true}) })

	if _, err := dockerClient.ContainerStart(ctx, resp.ID, client.ContainerStartOptions{}); err != nil {
		t.Fatalf("container start failed: %v", err)
	}

	runAZFSmokeExec(t, ctx, resp.ID, []string{"sh", "-c", "printf azf-step-1"}, "azf-step-1")
	runAZFSmokeExec(t, ctx, resp.ID, []string{"sh", "-c", "printf azf-step-2"}, "azf-step-2")

	waited := dockerClient.ContainerWait(ctx, resp.ID, client.ContainerWaitOptions{Condition: container.WaitConditionNotRunning})
	waitCh, errCh := waited.Result, waited.Error
	stopTimeout := 2
	if _, err := dockerClient.ContainerStop(ctx, resp.ID, client.ContainerStopOptions{Timeout: &stopTimeout}); err != nil {
		t.Fatalf("container stop failed: %v", err)
	}
	select {
	case result := <-waitCh:
		// AZF's ContainerStop reports ExitCode=137 (the FaaS
		// "force-terminated" semantic — Azure Functions has no
		// signal API to deliver SIGTERM to the underlying container,
		// so the backend records 137 directly on stop).
		if result.StatusCode != 137 {
			t.Fatalf("wait status = %d, want 137 (AZF stop semantic)", result.StatusCode)
		}
	case err := <-errCh:
		t.Fatalf("container wait error: %v", err)
	case <-time.After(30 * time.Second):
		t.Fatal("timeout waiting for container exit (30s after ContainerStop)")
	}

	// The AZF backend deletes the underlying function app when the
	// container stops, so the container ID is already gone by the
	// time the test's t.Cleanup runs (Force: true is idempotent
	// on 404).
}

func runAZFSmokeExec(t *testing.T, ctx context.Context, containerID string, cmd []string, wantStdout string) {
	t.Helper()

	execResp, err := dockerClient.ExecCreate(ctx, containerID, client.ExecCreateOptions{
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

	hijacked, err := dockerClient.ExecAttach(ctx, execResp.ID, client.ExecAttachOptions{})
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

	inspect, err := dockerClient.ExecInspect(ctx, execResp.ID, client.ExecInspectOptions{})
	if err != nil {
		t.Fatalf("exec inspect failed: %v", err)
	}
	if inspect.ExitCode != 0 {
		t.Fatalf("exec exit code = %d", inspect.ExitCode)
	}
}
