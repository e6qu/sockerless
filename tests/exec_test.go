package tests

import (
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
)

func TestExecCreateAndInspect(t *testing.T) {
	pullImage(t, "alpine")

	id := createContainer(t, "test-exec", &container.Config{
		Image:     "alpine",
		Cmd:       []string{"sh"},
		Tty:       true,
		OpenStdin: true,
	}, nil)
	defer removeContainer(t, id)

	dockerClient.ContainerStart(ctx, id, container.StartOptions{})

	// Create exec
	execResp, err := dockerClient.ContainerExecCreate(ctx, id, container.ExecOptions{
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          []string{"echo", "exec-output"},
	})
	if err != nil {
		t.Fatalf("exec create failed: %v", err)
	}

	if execResp.ID == "" {
		t.Error("expected non-empty exec ID")
	}

	// Inspect exec
	execInfo, err := dockerClient.ContainerExecInspect(ctx, execResp.ID)
	if err != nil {
		t.Fatalf("exec inspect failed: %v", err)
	}

	if execInfo.ContainerID != id {
		t.Errorf("expected container ID %s, got %s", id, execInfo.ContainerID)
	}
}

func TestExecStart(t *testing.T) {
	pullImage(t, "alpine")

	id := createContainer(t, "test-exec-start", &container.Config{
		Image:     "alpine",
		Cmd:       []string{"sh"},
		Tty:       true,
		OpenStdin: true,
	}, nil)
	defer removeContainer(t, id)

	dockerClient.ContainerStart(ctx, id, container.StartOptions{})

	execResp, err := dockerClient.ContainerExecCreate(ctx, id, container.ExecOptions{
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          []string{"echo", "hello-exec"},
	})
	if err != nil {
		t.Fatalf("exec create failed: %v", err)
	}

	// Start exec
	resp, err := dockerClient.ContainerExecAttach(ctx, execResp.ID, types.ExecStartCheck{})
	if err != nil {
		t.Fatalf("exec start failed: %v", err)
	}
	defer resp.Close()

	// Read output
	buf := make([]byte, 4096)
	n, _ := resp.Reader.Read(buf)
	t.Logf("exec output: %q", string(buf[:n]))
}
