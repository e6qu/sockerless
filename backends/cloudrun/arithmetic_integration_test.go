// Integration tests for the cloudrun backend. TestMain (in
// integration_test.go) brings up the sockerless backend, GCP simulator,
// and the docker client pointed at the backend. SOCKERLESS_TEST_TARGET
// (sim or cloud) is required; harness fails loud on missing config.

package cloudrun

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

func TestCloudRunArithmeticSuccess(t *testing.T) {
	ctx := context.Background()

	resp, err := dockerClient.ContainerCreate(ctx, &container.Config{
		Image:     evalImageName,
		Cmd:       []string{"3 + 4 * 2"},
		OpenStdin: true,
	}, nil, nil, nil, "cr-arith-success")
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
		t.Errorf("expected logs to contain '11', got %q", logs)
	}
}

func TestCloudRunArithmeticParentheses(t *testing.T) {
	ctx := context.Background()

	resp, err := dockerClient.ContainerCreate(ctx, &container.Config{
		Image:     evalImageName,
		Cmd:       []string{"(3 + 4) * 2"},
		OpenStdin: true,
	}, nil, nil, nil, "cr-arith-parens")
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
		t.Errorf("expected logs to contain '14', got %q", logs)
	}
}

func TestCloudRunArithmeticInvalid(t *testing.T) {
	ctx := context.Background()

	resp, err := dockerClient.ContainerCreate(ctx, &container.Config{
		Image:     evalImageName,
		Cmd:       []string{"3 +"},
		OpenStdin: true,
	}, nil, nil, nil, "cr-arith-invalid")
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
		t.Errorf("expected logs to contain 'ERROR', got %q", logs)
	}
}

func TestCloudRunArithmeticDivision(t *testing.T) {
	ctx := context.Background()

	resp, err := dockerClient.ContainerCreate(ctx, &container.Config{
		Image:     evalImageName,
		Cmd:       []string{"10 / 3"},
		OpenStdin: true,
	}, nil, nil, nil, "cr-arith-div")
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
		t.Errorf("expected logs to contain '3.333', got %q", logs)
	}
}

func TestCloudRunArithmeticWithLabels(t *testing.T) {
	ctx := context.Background()

	resp, err := dockerClient.ContainerCreate(ctx, &container.Config{
		Image:     evalImageName,
		Cmd:       []string{"100 - 42"},
		OpenStdin: true,
		Labels:    map[string]string{"arith-test": "cloudrun"},
	}, nil, nil, nil, "cr-arith-labels")
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
		t.Errorf("expected logs to contain '58', got %q", logs)
	}

	// Labels round-trip via GCP annotations since their JSON
	// representation fails the label-value charset.
	info, err := dockerClient.ContainerInspect(ctx, resp.ID)
	if err != nil {
		t.Fatalf("inspect failed: %v", err)
	}
	if info.Config == nil || info.Config.Labels["arith-test"] != "cloudrun" {
		t.Errorf("expected Labels[arith-test]=cloudrun to round-trip via annotations; got %+v", info.Config.Labels)
	}
}

func TestCloudRunArithmeticEnvVar(t *testing.T) {
	ctx := context.Background()

	resp, err := dockerClient.ContainerCreate(ctx, &container.Config{
		Image:     evalImageName,
		Cmd:       []string{"(3 + 4) * 2"},
		OpenStdin: true,
		Env:       []string{"EXPR=(3 + 4) * 2"},
	}, nil, nil, nil, "cr-arith-env")
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
		t.Errorf("expected logs to contain '14', got %q", logs)
	}
}

// Runner job-timeout enforcement is exhaustively unit-tested in the
// bootstrap binary itself — see
// agent/cmd/sockerless-cloudrun-bootstrap/main_test.go ::
// TestRunWithTimeout_FiresOnHang / _FinishesEarly / _ZeroExit /
// _DisabledByZero + TestJobTimeoutFromEnv. End-to-end exercise
// against a deployed bootstrap (overlay path) requires
// SOCKERLESS_TEST_TARGET=cloud where the runner image carries the
// bootstrap binary. The sim path here intentionally does NOT activate
// the overlay (see TestMain comment) because the bootstrap defaults
// to long-lived HTTP-server mode and would never let the test
// container exit. Re-add a sim-side end-to-end timer test once the
// bootstrap learns a one-shot mode the sim can drive.
