package lambda

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
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

func pullImage(t *testing.T) {
	t.Helper()
	rc, _ := dockerClient.ImagePull(context.Background(), "alpine:latest", image.PullOptions{})
	if rc != nil {
		io.Copy(io.Discard, rc)
		rc.Close()
	}
}

func TestLambdaArithmeticSuccess(t *testing.T) {
	pullImage(t)
	ctx := context.Background()

	resp, err := dockerClient.ContainerCreate(ctx, &container.Config{
		Image: "alpine:latest",
		Cmd:   []string{evalBinaryPath, "3 + 4 * 2"},
	}, nil, nil, nil, "lambda-arith-success")
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

	logs := readContainerLogs(t, resp.ID)
	if !strings.Contains(logs, "11") {
		t.Errorf("expected logs to contain %q, got %q", "11", logs)
	}
}

func TestLambdaArithmeticParentheses(t *testing.T) {
	pullImage(t)
	ctx := context.Background()

	resp, err := dockerClient.ContainerCreate(ctx, &container.Config{
		Image: "alpine:latest",
		Cmd:   []string{evalBinaryPath, "(3 + 4) * 2"},
	}, nil, nil, nil, "lambda-arith-parens")
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

	logs := readContainerLogs(t, resp.ID)
	if !strings.Contains(logs, "14") {
		t.Errorf("expected logs to contain %q, got %q", "14", logs)
	}
}

func TestLambdaArithmeticInvalid(t *testing.T) {
	pullImage(t)
	ctx := context.Background()

	resp, err := dockerClient.ContainerCreate(ctx, &container.Config{
		Image: "alpine:latest",
		Cmd:   []string{evalBinaryPath, "3 +"},
	}, nil, nil, nil, "lambda-arith-invalid")
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

	logs := readContainerLogs(t, resp.ID)
	if !strings.Contains(logs, "ERROR") {
		t.Errorf("expected logs to contain %q, got %q", "ERROR", logs)
	}
}

func TestLambdaArithmeticDivision(t *testing.T) {
	pullImage(t)
	ctx := context.Background()

	resp, err := dockerClient.ContainerCreate(ctx, &container.Config{
		Image: "alpine:latest",
		Cmd:   []string{evalBinaryPath, "10 / 3"},
	}, nil, nil, nil, "lambda-arith-div")
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

	logs := readContainerLogs(t, resp.ID)
	if !strings.Contains(logs, "3.333") {
		t.Errorf("expected logs to contain %q, got %q", "3.333", logs)
	}
}

func TestLambdaArithmeticWithLabels(t *testing.T) {
	pullImage(t)
	ctx := context.Background()

	resp, err := dockerClient.ContainerCreate(ctx, &container.Config{
		Image:  "alpine:latest",
		Cmd:    []string{evalBinaryPath, "100 - 42"},
		Labels: map[string]string{"arith-test": "lambda"},
	}, nil, nil, nil, "lambda-arith-labels")
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

	logs := readContainerLogs(t, resp.ID)
	if !strings.Contains(logs, "58") {
		t.Errorf("expected logs to contain %q, got %q", "58", logs)
	}

	// Verify label filter finds the container
	containers, err := dockerClient.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filters.NewArgs(filters.Arg("label", "arith-test=lambda")),
	})
	if err != nil {
		t.Fatalf("list with filter failed: %v", err)
	}
	found := false
	for _, c := range containers {
		if c.ID == resp.ID {
			found = true
			break
		}
	}
	if !found {
		t.Error("container not found via label filter")
	}
}

func TestLambdaArithmeticEnvVar(t *testing.T) {
	pullImage(t)
	ctx := context.Background()

	resp, err := dockerClient.ContainerCreate(ctx, &container.Config{
		Image: "alpine:latest",
		Cmd:   []string{evalBinaryPath, "(3 + 4) * 2"},
		Env:   []string{"EXPR=(3 + 4) * 2"},
	}, nil, nil, nil, "lambda-arith-env")
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

	logs := readContainerLogs(t, resp.ID)
	if !strings.Contains(logs, "14") {
		t.Errorf("expected logs to contain %q, got %q", "14", logs)
	}
}
