//go:build integration

package gcf

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

func TestGCFFaaSE2ESmoke(t *testing.T) {
	if os.Getenv(gcfExecE2EEnv) != "1" {
		cmd := exec.Command(os.Args[0], "-test.run", "^TestGCFFaaSE2ESmoke$", "-test.v")
		cmd.Env = append(os.Environ(), gcfExecE2EEnv+"=1")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("GCF FaaS smoke subprocess failed: %v\n%s", err, string(out))
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
	resp, err := dockerClient.ContainerCreate(ctx,
		&container.Config{
			Image: "alpine:latest",
			// Shell traps SIGTERM → exit 0. `sleep 600 & wait`
			// keeps the shell alive but lets the trap fire while
			// idle. Termination is driven by the test's
			// ContainerStop call below; signalling externally
			// keeps the exec processes alive long enough to
			// report their exit status.
			Cmd: []string{"sh", "-c", "trap 'exit 0' TERM; sleep 600 & wait"},
		},
		nil, nil, nil, "gcf_faas_smoke_"+testID,
	)
	if err != nil {
		t.Fatalf("container create failed: %v", err)
	}
	t.Cleanup(func() { _ = dockerClient.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true}) })

	if err := dockerClient.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		t.Fatalf("container start failed: %v", err)
	}

	runGCFSmokeExec(t, ctx, resp.ID, []string{"sh", "-c", "printf gcf-step-1"}, "gcf-step-1")
	runGCFSmokeExec(t, ctx, resp.ID, []string{"sh", "-c", "printf gcf-step-2"}, "gcf-step-2")

	waitCh, errCh := dockerClient.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	stopTimeout := 2
	if err := dockerClient.ContainerStop(ctx, resp.ID, container.StopOptions{Timeout: &stopTimeout}); err != nil {
		t.Fatalf("container stop failed: %v", err)
	}
	select {
	case result := <-waitCh:
		// GCF's ContainerStop reports ExitCode=137 (the FaaS
		// "force-terminated" semantic — Cloud Functions has no
		// signal API to deliver SIGTERM to the underlying container).
		if result.StatusCode != 137 {
			t.Fatalf("wait status = %d, want 137 (GCF stop semantic)", result.StatusCode)
		}
	case err := <-errCh:
		t.Fatalf("container wait error: %v", err)
	case <-time.After(30 * time.Second):
		t.Fatal("timeout waiting for container exit (30s after ContainerStop)")
	}

	// The GCF backend deletes the underlying function when the
	// container stops, so the container ID is already gone by
	// the time the test's t.Cleanup runs (Force: true is
	// idempotent on 404).
}

func runGCFSmokeExec(t *testing.T, ctx context.Context, containerID string, cmd []string, wantStdout string) {
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
