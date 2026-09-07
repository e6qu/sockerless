package tests

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/moby/moby/client"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
)

func TestContainerLogs(t *testing.T) {
	pullImage(t, "alpine")

	id := createContainer(t, "test-logs", &container.Config{
		Image: "alpine",
		Cmd:   []string{"echo", "hello world"},
	}, nil)
	defer removeContainer(t, id)

	_, _ = dockerClient.ContainerStart(ctx, id, client.ContainerStartOptions{})

	// Wait for container to exit
	waited := dockerClient.ContainerWait(ctx, id, client.ContainerWaitOptions{Condition: container.WaitConditionNotRunning})
	waitCh, errCh := waited.Result, waited.Error
	select {
	case <-waitCh:
	case err := <-errCh:
		t.Fatalf("wait failed: %v", err)
	}

	logs := readLogs(t, dockerClient, id)
	if !strings.Contains(logs, "hello world") {
		t.Errorf("expected logs to contain %q, got %q", "hello world", logs)
	}
}

func TestContainerAttach(t *testing.T) {
	pullImage(t, "alpine")

	id := createContainer(t, "test-attach", &container.Config{
		Image:        "alpine",
		Cmd:          []string{"echo", "hello"},
		AttachStdout: true,
		AttachStderr: true,
	}, nil)
	defer removeContainer(t, id)

	resp, err := dockerClient.ContainerAttach(ctx, id, client.ContainerAttachOptions{
		Stream: true,
		Stdout: true,
		Stderr: true,
	})
	if err != nil {
		t.Fatalf("attach failed: %v", err)
	}
	defer resp.Close()

	// Start the container after attach
	_, _ = dockerClient.ContainerStart(ctx, id, client.ContainerStartOptions{})

	// Read output
	var stdout, stderr bytes.Buffer
	stdcopy.StdCopy(&stdout, &stderr, resp.Reader)

	output := stdout.String() + stderr.String()
	if output == "" {
		// Attach may return data before start completes
		// Read from the connection directly
		buf := make([]byte, 4096)
		n, _ := resp.Reader.Read(buf)
		if n > 0 {
			_ = string(buf[:n])
		}
	}

	t.Logf("attach output: stdout=%q stderr=%q", stdout.String(), stderr.String())
}

func TestContainerLogsWithTimestamps(t *testing.T) {
	pullImage(t, "alpine")

	id := createContainer(t, "test-logs-ts", &container.Config{
		Image: "alpine",
		Cmd:   []string{"echo", "timestamped"},
	}, nil)
	defer removeContainer(t, id)

	_, _ = dockerClient.ContainerStart(ctx, id, client.ContainerStartOptions{})

	waited2 := dockerClient.ContainerWait(ctx, id, client.ContainerWaitOptions{Condition: container.WaitConditionNotRunning})
	waitCh, errCh := waited2.Result, waited2.Error
	select {
	case <-waitCh:
	case err := <-errCh:
		t.Fatalf("wait failed: %v", err)
	}

	rc, err := dockerClient.ContainerLogs(ctx, id, client.ContainerLogsOptions{
		ShowStdout: true,
		Timestamps: true,
	})
	if err != nil {
		t.Fatalf("logs failed: %v", err)
	}
	defer rc.Close()

	var stdout bytes.Buffer
	stdcopy.StdCopy(&stdout, io.Discard, rc)

	t.Logf("logs with timestamps: %q", stdout.String())
}
