package tests

import (
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"
	"io"
	"bytes"
)

func TestContainerLogs(t *testing.T) {
	pullImage(t, "alpine")

	id := createContainer(t, "test-logs", &container.Config{
		Image: "alpine",
		Cmd:   []string{"echo", "hello world"},
	}, nil)
	defer removeContainer(t, id)

	dockerClient.ContainerStart(ctx, id, container.StartOptions{})

	// Wait for container to exit
	waitCh, errCh := dockerClient.ContainerWait(ctx, id, container.WaitConditionNotRunning)
	select {
	case <-waitCh:
	case err := <-errCh:
		t.Fatalf("wait failed: %v", err)
	}

	rc, err := dockerClient.ContainerLogs(ctx, id, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
	})
	if err != nil {
		t.Fatalf("logs failed: %v", err)
	}
	defer rc.Close()

	var stdout, stderr bytes.Buffer
	_, err = stdcopy.StdCopy(&stdout, &stderr, rc)
	if err != nil {
		t.Fatalf("stdcopy failed: %v", err)
	}

	if stdout.Len() == 0 && stderr.Len() == 0 {
		t.Error("expected some log output")
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

	resp, err := dockerClient.ContainerAttach(ctx, id, container.AttachOptions{
		Stream: true,
		Stdout: true,
		Stderr: true,
	})
	if err != nil {
		t.Fatalf("attach failed: %v", err)
	}
	defer resp.Close()

	// Start the container after attach
	dockerClient.ContainerStart(ctx, id, container.StartOptions{})

	// Read output
	var stdout, stderr bytes.Buffer
	stdcopy.StdCopy(&stdout, &stderr, resp.Reader)

	output := stdout.String() + stderr.String()
	if output == "" {
		// With memory backend, attach may return data before start completes
		// Read from the connection directly
		buf := make([]byte, 4096)
		n, _ := resp.Reader.Read(buf)
		if n > 0 {
			output = string(buf[:n])
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

	dockerClient.ContainerStart(ctx, id, container.StartOptions{})

	waitCh, errCh := dockerClient.ContainerWait(ctx, id, container.WaitConditionNotRunning)
	select {
	case <-waitCh:
	case err := <-errCh:
		t.Fatalf("wait failed: %v", err)
	}

	rc, err := dockerClient.ContainerLogs(ctx, id, container.LogsOptions{
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
