//go:build integration

package lambda

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/moby/moby/client"

	"github.com/moby/moby/api/types/container"
)

func TestLambdaFaaSE2ESmoke(t *testing.T) {
	ctx := context.Background()

	if agentTestImageName == "" {
		t.Fatal("agentTestImageName unset — TestMain should have built it")
	}

	testID := generateTestID()
	resp, err := dockerClient.ContainerCreate(ctx, client.ContainerCreateOptions{Config: &container.Config{
		Image: "alpine:latest",
		// Shell traps SIGTERM → exit 0. `sleep 600 & wait`
		// keeps the shell alive but lets the trap fire while
		// idle. Termination is driven by the test's
		// ContainerStop call below; signalling externally
		// keeps the exec processes alive long enough to
		// report their exit status.
		Cmd: []string{"sh", "-c", "trap 'exit 0' TERM; sleep 600 & wait"},
	}, Name: "lambda_faas_smoke_" + testID},
	)
	if err != nil {
		t.Fatalf("container create failed: %v", err)
	}
	t.Cleanup(func() { _, _ = dockerClient.ContainerRemove(ctx, resp.ID, client.ContainerRemoveOptions{Force: true}) })

	if _, err := dockerClient.ContainerStart(ctx, resp.ID, client.ContainerStartOptions{}); err != nil {
		t.Fatalf("container start failed: %v", err)
	}

	runLambdaSmokeExec(t, ctx, resp.ID, []string{"sh", "-c", "printf lambda-step-1"}, "lambda-step-1")
	runLambdaSmokeExec(t, ctx, resp.ID, []string{"sh", "-c", "printf lambda-step-2"}, "lambda-step-2")

	waited := dockerClient.ContainerWait(ctx, resp.ID, client.ContainerWaitOptions{Condition: container.WaitConditionNotRunning})
	waitCh, errCh := waited.Result, waited.Error
	stopTimeout := 2
	if _, err := dockerClient.ContainerStop(ctx, resp.ID, client.ContainerStopOptions{Timeout: &stopTimeout}); err != nil {
		t.Fatalf("container stop failed: %v", err)
	}
	select {
	case result := <-waitCh:
		// Lambda's ContainerStop reports ExitCode=137 (the FaaS
		// "force-terminated" semantic — AWS Lambda has no signal API
		// to deliver SIGTERM to the underlying container; the backend
		// records 137 directly on stop and lets the in-flight
		// invocation drain in the background).
		if result.StatusCode != 137 {
			t.Fatalf("wait status = %d, want 137 (Lambda stop semantic)", result.StatusCode)
		}
	case err := <-errCh:
		t.Fatalf("container wait error: %v", err)
	case <-time.After(30 * time.Second):
		t.Fatal("timeout waiting for container exit (30s after ContainerStop)")
	}

	// The Lambda backend deletes the underlying function when the
	// container stops, so the container ID is already gone by the
	// time the test's t.Cleanup runs (Force: true is idempotent
	// on 404).
}

func runLambdaSmokeExec(t *testing.T, ctx context.Context, containerID string, cmd []string, wantStdout string) {
	t.Helper()

	deadline := time.Now().Add(60 * time.Second)
	var gotStdout []byte
	var lastExitCode int
	var lastErr error

	for time.Now().Before(deadline) {
		execResp, err := dockerClient.ExecCreate(ctx, containerID, client.ExecCreateOptions{
			Cmd:          cmd,
			AttachStdout: true,
			AttachStderr: true,
		})
		if err != nil {
			lastErr = err
			time.Sleep(500 * time.Millisecond)
			continue
		}
		if execResp.ID == "" {
			t.Fatal("expected non-empty exec ID")
		}

		hijacked, err := dockerClient.ExecAttach(ctx, execResp.ID, client.ExecAttachOptions{})
		if err != nil {
			lastErr = err
			time.Sleep(500 * time.Millisecond)
			continue
		}
		raw, readErr := io.ReadAll(hijacked.Reader)
		hijacked.Close()
		if readErr != nil {
			lastErr = readErr
			time.Sleep(500 * time.Millisecond)
			continue
		}
		gotStdout = demuxDockerStream(raw)

		inspect, err := dockerClient.ExecInspect(ctx, execResp.ID, client.ExecInspectOptions{})
		if err != nil {
			lastErr = err
			time.Sleep(500 * time.Millisecond)
			continue
		}
		lastExitCode = inspect.ExitCode
		if lastExitCode == 0 && bytes.Equal(gotStdout, []byte(wantStdout)) {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}

	t.Fatalf("exec did not reach reverse-agent path: want stdout %q, got %q, last exit %d, last error %v", wantStdout, string(gotStdout), lastExitCode, lastErr)
}
