//go:build integration

package aca

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/moby/moby/client"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
)

func TestACAFaaSE2ESmoke(t *testing.T) {
	if os.Getenv(acaAppsE2EEnv) != "1" {
		cmd := exec.Command(os.Args[0], "-test.run", "^TestACAFaaSE2ESmoke$", "-test.v")
		cmd.Env = append(os.Environ(), acaAppsE2EEnv+"=1")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("ACA FaaS smoke subprocess failed: %v\n%s", err, string(out))
		}
		return
	}

	if acaOverlayImageName == "" {
		t.Fatal("ACA overlay image was not built by TestMain")
	}

	ctx := context.Background()
	testID := generateTestID()
	resp, err := dockerClient.ContainerCreate(ctx, client.ContainerCreateOptions{Config: &container.Config{
		Image: acaOverlayImageName,
		Cmd:   []string{"/opt/sockerless/container-command", "hold"},
	}, Name: "aca_faas_smoke_" + testID},
	)
	if err != nil {
		t.Fatalf("container create failed: %v", err)
	}
	t.Cleanup(func() { _, _ = dockerClient.ContainerRemove(ctx, resp.ID, client.ContainerRemoveOptions{Force: true}) })

	startCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	if _, err := dockerClient.ContainerStart(startCtx, resp.ID, client.ContainerStartOptions{}); err != nil {
		t.Fatalf("container start failed: %v", err)
	}

	runACASmokeExec(t, ctx, resp.ID, []string{"/opt/sockerless/container-command", "print", "aca-step-1"}, "aca-step-1")
	runACASmokeExec(t, ctx, resp.ID, []string{"/opt/sockerless/container-command", "print", "aca-step-2"}, "aca-step-2")

	waited := dockerClient.ContainerWait(ctx, resp.ID, client.ContainerWaitOptions{Condition: container.WaitConditionNotRunning})
	waitCh, errCh := waited.Result, waited.Error
	timeout := 1
	if _, err := dockerClient.ContainerStop(ctx, resp.ID, client.ContainerStopOptions{Timeout: &timeout}); err != nil {
		t.Fatalf("container stop failed: %v", err)
	}
	select {
	case result := <-waitCh:
		if result.StatusCode != 143 {
			t.Fatalf("wait status = %d, want 143", result.StatusCode)
		}
	case err := <-errCh:
		t.Fatalf("container wait error: %v", err)
	case <-time.After(5 * time.Minute):
		t.Fatal("timeout waiting for container exit")
	}

	if _, err := dockerClient.ContainerRemove(ctx, resp.ID, client.ContainerRemoveOptions{}); err != nil {
		t.Fatalf("container remove failed: %v", err)
	}
}

func runACASmokeExec(t *testing.T, ctx context.Context, containerID string, cmd []string, wantStdout string) {
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
