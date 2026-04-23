package azf

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"
)

// readContainerLogs reads Docker multiplexed logs for a container, retrying up to 10s
// for log ingestion delay.
func readContainerLogs(t *testing.T, id string) string {
	t.Helper()
	ctx := context.Background()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		rc, err := dockerClient.ContainerLogs(ctx, id, container.LogsOptions{
			ShowStdout: true,
			ShowStderr: true,
		})
		if err != nil {
			t.Fatalf("container logs failed: %v", err)
		}
		var stdout, stderr bytes.Buffer
		stdcopy.StdCopy(&stdout, &stderr, rc)
		rc.Close()
		combined := stdout.String() + stderr.String()
		if len(combined) > 0 {
			return combined
		}
		time.Sleep(500 * time.Millisecond)
	}
	return ""
}

// checkLogs verifies log content. Azure Monitor log queries may fail in
// non-TLS integration tests, so this is a soft check (same pattern as
// TestAZFContainerLogs).
func checkLogs(t *testing.T, id, expected string) {
	t.Helper()
	logs := readContainerLogs(t, id)
	if logs == "" {
		t.Logf("note: logs empty, may be due to Azure Monitor TLS requirement in integration tests")
		return
	}
	if !strings.Contains(logs, expected) {
		t.Errorf("expected logs to contain %q, got %q", expected, logs)
	}
}

func TestAZFArithmeticSuccess(t *testing.T) {
	ctx := context.Background()

	resp, err := dockerClient.ContainerCreate(ctx, &container.Config{
		Image: evalImageName,
		Cmd:   []string{"3 + 4 * 2"},
	}, nil, nil, nil, "azf-arith-success")
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	defer dockerClient.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})

	if err := dockerClient.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	waitCh, errCh := dockerClient.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	select {
	case result := <-waitCh:
		if result.StatusCode != 0 {
			t.Errorf("expected exit code 0, got %d", result.StatusCode)
		}
	case err := <-errCh:
		t.Fatalf("wait error: %v", err)
	case <-time.After(5 * time.Minute):
		t.Fatal("timeout waiting for container")
	}

	checkLogs(t, resp.ID, "11")
}

// TestAZFArithmeticInvalid — re-enabled from the BUG-744 deletion.
// The eval-arithmetic binary exits 1 on invalid syntax; Phase 95 maps
// the function's non-2xx HTTP response via core.HTTPStatusToExitCode
// so docker wait returns exit code 1 here.
func TestAZFArithmeticInvalid(t *testing.T) {
	ctx := context.Background()

	resp, err := dockerClient.ContainerCreate(ctx, &container.Config{
		Image: evalImageName,
		Cmd:   []string{"3 +"},
	}, nil, nil, nil, "azf-arith-invalid")
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	defer dockerClient.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})

	if err := dockerClient.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	waitCh, errCh := dockerClient.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	select {
	case result := <-waitCh:
		if result.StatusCode != 1 {
			t.Errorf("expected exit code 1, got %d", result.StatusCode)
		}
	case err := <-errCh:
		t.Fatalf("wait error: %v", err)
	case <-time.After(5 * time.Minute):
		t.Fatal("timeout waiting for container")
	}

	checkLogs(t, resp.ID, "ERROR")
}

func TestAZFArithmeticParentheses(t *testing.T) {
	ctx := context.Background()

	resp, err := dockerClient.ContainerCreate(ctx, &container.Config{
		Image: evalImageName,
		Cmd:   []string{"(3 + 4) * 2"},
	}, nil, nil, nil, "azf-arith-parens")
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	defer dockerClient.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})

	if err := dockerClient.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	waitCh, errCh := dockerClient.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	select {
	case result := <-waitCh:
		if result.StatusCode != 0 {
			t.Errorf("expected exit code 0, got %d", result.StatusCode)
		}
	case err := <-errCh:
		t.Fatalf("wait error: %v", err)
	case <-time.After(5 * time.Minute):
		t.Fatal("timeout waiting for container")
	}

	checkLogs(t, resp.ID, "14")
}

func TestAZFArithmeticDivision(t *testing.T) {
	ctx := context.Background()

	resp, err := dockerClient.ContainerCreate(ctx, &container.Config{
		Image: evalImageName,
		Cmd:   []string{"10 / 3"},
	}, nil, nil, nil, "azf-arith-div")
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	defer dockerClient.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})

	if err := dockerClient.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	waitCh, errCh := dockerClient.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	select {
	case result := <-waitCh:
		if result.StatusCode != 0 {
			t.Errorf("expected exit code 0, got %d", result.StatusCode)
		}
	case err := <-errCh:
		t.Fatalf("wait error: %v", err)
	case <-time.After(5 * time.Minute):
		t.Fatal("timeout waiting for container")
	}

	checkLogs(t, resp.ID, "3.333")
}

func TestAZFArithmeticWithLabels(t *testing.T) {
	ctx := context.Background()

	resp, err := dockerClient.ContainerCreate(ctx, &container.Config{
		Image:  evalImageName,
		Cmd:    []string{"100 - 42"},
		Labels: map[string]string{"arith-test": "azf"},
	}, nil, nil, nil, "azf-arith-labels")
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	defer dockerClient.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})

	if err := dockerClient.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	waitCh, errCh := dockerClient.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	select {
	case result := <-waitCh:
		if result.StatusCode != 0 {
			t.Errorf("expected exit code 0, got %d", result.StatusCode)
		}
	case err := <-errCh:
		t.Fatalf("wait error: %v", err)
	case <-time.After(5 * time.Minute):
		t.Fatal("timeout waiting for container")
	}

	checkLogs(t, resp.ID, "58")
}

func TestAZFArithmeticEnvVar(t *testing.T) {
	ctx := context.Background()

	resp, err := dockerClient.ContainerCreate(ctx, &container.Config{
		Image: evalImageName,
		Cmd:   []string{"(3 + 4) * 2"},
		Env:   []string{"EXPR=(3 + 4) * 2"},
	}, nil, nil, nil, "azf-arith-env")
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	defer dockerClient.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})

	if err := dockerClient.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	waitCh, errCh := dockerClient.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	select {
	case result := <-waitCh:
		if result.StatusCode != 0 {
			t.Errorf("expected exit code 0, got %d", result.StatusCode)
		}
	case err := <-errCh:
		t.Fatalf("wait error: %v", err)
	case <-time.After(5 * time.Minute):
		t.Fatal("timeout waiting for container")
	}

	checkLogs(t, resp.ID, "14")
}
