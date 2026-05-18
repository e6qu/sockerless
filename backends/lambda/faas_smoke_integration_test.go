package lambda

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
)

func TestLambdaFaaSE2ESmoke(t *testing.T) {
	ctx := context.Background()

	if agentTestImageName == "" {
		t.Fatal("agentTestImageName unset — TestMain should have built it")
	}

	testID := generateTestID()
	resp, err := dockerClient.ContainerCreate(ctx,
		&container.Config{
			Image: "alpine:latest",
			Cmd:   []string{"sh", "-c", "while [ ! -f /tmp/sockerless-done ]; do sleep 1; done"},
		},
		nil, nil, nil, "lambda_faas_smoke_"+testID,
	)
	if err != nil {
		t.Fatalf("container create failed: %v", err)
	}
	t.Cleanup(func() { _ = dockerClient.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true}) })

	if err := dockerClient.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		t.Fatalf("container start failed: %v", err)
	}

	runLambdaSmokeExec(t, ctx, resp.ID, []string{"sh", "-c", "printf lambda-step-1"}, "lambda-step-1")
	runLambdaSmokeExec(t, ctx, resp.ID, []string{"sh", "-c", "printf lambda-step-2 && touch /tmp/sockerless-done"}, "lambda-step-2")

	waitCh, errCh := dockerClient.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	select {
	case result := <-waitCh:
		if result.StatusCode != 0 {
			t.Fatalf("wait status = %d, want 0", result.StatusCode)
		}
	case err := <-errCh:
		t.Fatalf("container wait error: %v", err)
	case <-time.After(5 * time.Minute):
		t.Fatal("timeout waiting for container exit")
	}

	if err := dockerClient.ContainerRemove(ctx, resp.ID, container.RemoveOptions{}); err != nil {
		t.Fatalf("container remove failed: %v", err)
	}
}

func runLambdaSmokeExec(t *testing.T, ctx context.Context, containerID string, cmd []string, wantStdout string) {
	t.Helper()

	deadline := time.Now().Add(60 * time.Second)
	var gotStdout []byte
	var lastExitCode int
	var lastErr error

	for time.Now().Before(deadline) {
		execResp, err := dockerClient.ContainerExecCreate(ctx, containerID, container.ExecOptions{
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

		hijacked, err := dockerClient.ContainerExecAttach(ctx, execResp.ID, container.ExecAttachOptions{})
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

		inspect, err := dockerClient.ContainerExecInspect(ctx, execResp.ID)
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
