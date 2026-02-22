package tests

import (
	"archive/tar"
	"bytes"
	"strings"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"
)

// TestExecShellScript simulates what act does: upload a script file via
// PUT archive, then exec sh -e /path/to/script.sh
func TestExecShellScript(t *testing.T) {
	pullImage(t, "alpine")

	id := createContainer(t, "test-script-exec", &container.Config{
		Image:      "alpine",
		Entrypoint: []string{"tail", "-f", "/dev/null"},
		Tty:        false,
		OpenStdin:  true,
	}, nil)
	defer removeContainer(t, id)

	if err := dockerClient.ContainerStart(ctx, id, container.StartOptions{}); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// Create a tar archive with the script
	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	scriptContent := "#!/bin/sh\necho 'Hello from script'\n"
	tw.WriteHeader(&tar.Header{
		Name: "0.sh",
		Size: int64(len(scriptContent)),
		Mode: 0755,
	})
	tw.Write([]byte(scriptContent))
	tw.Close()

	// Upload via PUT archive (like act does)
	err := dockerClient.CopyToContainer(ctx, id, "/var/run/act/workflow", &tarBuf, container.CopyToContainerOptions{})
	if err != nil {
		t.Fatalf("copy to container failed: %v", err)
	}

	// Exec sh -e /var/run/act/workflow/0.sh (exactly like act does)
	execResp, err := dockerClient.ContainerExecCreate(ctx, id, container.ExecOptions{
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          []string{"sh", "-e", "/var/run/act/workflow/0.sh"},
	})
	if err != nil {
		t.Fatalf("exec create failed: %v", err)
	}

	resp, err := dockerClient.ContainerExecAttach(ctx, execResp.ID, types.ExecStartCheck{})
	if err != nil {
		t.Fatalf("exec attach failed: %v", err)
	}
	defer resp.Close()

	// Demux stdout/stderr using Docker's stdcopy (validates multiplexed stream format)
	var stdout, stderr bytes.Buffer
	_, err = stdcopy.StdCopy(&stdout, &stderr, resp.Reader)
	if err != nil {
		t.Fatalf("stdcopy failed: %v", err)
	}
	t.Logf("stdout: %q, stderr: %q", stdout.String(), stderr.String())

	// Check exit code
	execInfo, err := dockerClient.ContainerExecInspect(ctx, execResp.ID)
	if err != nil {
		t.Fatalf("exec inspect failed: %v", err)
	}
	t.Logf("exit code: %d", execInfo.ExitCode)

	if execInfo.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", execInfo.ExitCode)
	}

	if got := strings.TrimSpace(stdout.String()); got != "Hello from script" {
		t.Errorf("expected stdout 'Hello from script', got: %q", got)
	}
}
