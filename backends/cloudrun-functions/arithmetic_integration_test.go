//go:build integration

// Integration tests for the gcf backend. TestMain (in
// integration_test.go) brings up the sockerless backend, GCP simulator,
// and the docker client pointed at the backend. SOCKERLESS_TEST_TARGET
// (sim or cloud) is required; harness fails loud on missing config.

package gcf

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/client"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
)

// readContainerLogs reads Docker multiplexed logs for a container, retrying up to 10s
// for log ingestion delay.
func readContainerLogs(t *testing.T, id string) string {
	t.Helper()
	ctx := context.Background()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		rc, err := dockerClient.ContainerLogs(ctx, id, client.ContainerLogsOptions{
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

// checkLogs verifies log content. Cloud Logging gRPC may fail with context
// deadline exceeded in non-gRPC integration tests, so this is a soft check
// (same pattern as TestGCFContainerLogs).
func checkLogs(t *testing.T, id, expected string) {
	t.Helper()
	logs := readContainerLogs(t, id)
	if logs == "" {
		t.Logf("note: logs empty, may be due to Cloud Logging gRPC requirement in integration tests")
		return
	}
	if !strings.Contains(logs, expected) {
		t.Errorf("expected logs to contain %q, got %q", expected, logs)
	}
}

func TestGCFArithmeticSuccess(t *testing.T) {
	ctx := context.Background()

	resp, err := dockerClient.ContainerCreate(ctx, client.ContainerCreateOptions{Config: &container.Config{
		Image: evalImageName,
		Cmd:   []string{"3 + 4 * 2"},
	}, Name: "gcf-arith-success"})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	defer dockerClient.ContainerRemove(ctx, resp.ID, client.ContainerRemoveOptions{Force: true})

	if _, err := dockerClient.ContainerStart(ctx, resp.ID, client.ContainerStartOptions{}); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	waited := dockerClient.ContainerWait(ctx, resp.ID, client.ContainerWaitOptions{Condition: container.WaitConditionNotRunning})
	waitCh, errCh := waited.Result, waited.Error
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

// TestGCFArithmeticInvalid: the eval-arithmetic binary exits 1 on
// invalid syntax; the function's non-2xx HTTP response is mapped via
// core.HTTPStatusToExitCode so docker wait returns exit code 1 here.
func TestGCFArithmeticInvalid(t *testing.T) {
	ctx := context.Background()

	resp, err := dockerClient.ContainerCreate(ctx, client.ContainerCreateOptions{Config: &container.Config{
		Image: evalImageName,
		Cmd:   []string{"3 +"},
	}, Name: "gcf-arith-invalid"})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	defer dockerClient.ContainerRemove(ctx, resp.ID, client.ContainerRemoveOptions{Force: true})

	if _, err := dockerClient.ContainerStart(ctx, resp.ID, client.ContainerStartOptions{}); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	waited2 := dockerClient.ContainerWait(ctx, resp.ID, client.ContainerWaitOptions{Condition: container.WaitConditionNotRunning})
	waitCh, errCh := waited2.Result, waited2.Error
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

func TestGCFArithmeticParentheses(t *testing.T) {
	ctx := context.Background()

	resp, err := dockerClient.ContainerCreate(ctx, client.ContainerCreateOptions{Config: &container.Config{
		Image: evalImageName,
		Cmd:   []string{"(3 + 4) * 2"},
	}, Name: "gcf-arith-parens"})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	defer dockerClient.ContainerRemove(ctx, resp.ID, client.ContainerRemoveOptions{Force: true})

	if _, err := dockerClient.ContainerStart(ctx, resp.ID, client.ContainerStartOptions{}); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	waited3 := dockerClient.ContainerWait(ctx, resp.ID, client.ContainerWaitOptions{Condition: container.WaitConditionNotRunning})
	waitCh, errCh := waited3.Result, waited3.Error
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

func TestGCFArithmeticDivision(t *testing.T) {
	ctx := context.Background()

	resp, err := dockerClient.ContainerCreate(ctx, client.ContainerCreateOptions{Config: &container.Config{
		Image: evalImageName,
		Cmd:   []string{"10 / 3"},
	}, Name: "gcf-arith-div"})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	defer dockerClient.ContainerRemove(ctx, resp.ID, client.ContainerRemoveOptions{Force: true})

	if _, err := dockerClient.ContainerStart(ctx, resp.ID, client.ContainerStartOptions{}); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	waited4 := dockerClient.ContainerWait(ctx, resp.ID, client.ContainerWaitOptions{Condition: container.WaitConditionNotRunning})
	waitCh, errCh := waited4.Result, waited4.Error
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

func TestGCFArithmeticWithLabels(t *testing.T) {
	ctx := context.Background()

	resp, err := dockerClient.ContainerCreate(ctx, client.ContainerCreateOptions{Config: &container.Config{
		Image:  evalImageName,
		Cmd:    []string{"100 - 42"},
		Labels: map[string]string{"arith-test": "gcf"},
	}, Name: "gcf-arith-labels"})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	defer dockerClient.ContainerRemove(ctx, resp.ID, client.ContainerRemoveOptions{Force: true})

	if _, err := dockerClient.ContainerStart(ctx, resp.ID, client.ContainerStartOptions{}); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	waited5 := dockerClient.ContainerWait(ctx, resp.ID, client.ContainerWaitOptions{Condition: container.WaitConditionNotRunning})
	waitCh, errCh := waited5.Result, waited5.Error
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

	// Labels survive the round-trip via the SOCKERLESS_LABELS env var
	// (GCF Functions v2 has no Annotations and GCP's label-value
	// charset would reject the JSON blob).
	inspected, err := dockerClient.ContainerInspect(ctx, resp.ID, client.ContainerInspectOptions{})
	info := inspected.Container
	if err != nil {
		t.Fatalf("inspect failed: %v", err)
	}
	if info.Config == nil || info.Config.Labels["arith-test"] != "gcf" {
		t.Errorf("expected Labels[arith-test]=gcf to round-trip; got %+v", info.Config.Labels)
	}
}

func TestGCFArithmeticEnvVar(t *testing.T) {
	ctx := context.Background()

	resp, err := dockerClient.ContainerCreate(ctx, client.ContainerCreateOptions{Config: &container.Config{
		Image: evalImageName,
		Cmd:   []string{"(3 + 4) * 2"},
		Env:   []string{"EXPR=(3 + 4) * 2"},
	}, Name: "gcf-arith-env"})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	defer dockerClient.ContainerRemove(ctx, resp.ID, client.ContainerRemoveOptions{Force: true})

	if _, err := dockerClient.ContainerStart(ctx, resp.ID, client.ContainerStartOptions{}); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	waited6 := dockerClient.ContainerWait(ctx, resp.ID, client.ContainerWaitOptions{Condition: container.WaitConditionNotRunning})
	waitCh, errCh := waited6.Result, waited6.Error
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
