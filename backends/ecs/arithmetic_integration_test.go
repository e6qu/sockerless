//go:build integration

package ecs

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

func TestECSArithmeticSuccess(t *testing.T) {
	ctx := context.Background()

	resp, err := dockerClient.ContainerCreate(ctx, client.ContainerCreateOptions{Config: &container.Config{
		Image: evalImageName,
		Cmd:   []string{"3 + 4 * 2"},
	}, Name: "ecs-arith-success"})
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

	logs := readContainerLogs(t, resp.ID)
	if !strings.Contains(logs, "11") {
		t.Errorf("expected logs to contain '11', got %q", logs)
	}
}

func TestECSArithmeticParentheses(t *testing.T) {
	ctx := context.Background()

	resp, err := dockerClient.ContainerCreate(ctx, client.ContainerCreateOptions{Config: &container.Config{
		Image: evalImageName,
		Cmd:   []string{"(3 + 4) * 2"},
	}, Name: "ecs-arith-parens"})
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

func TestECSArithmeticInvalid(t *testing.T) {
	ctx := context.Background()

	resp, err := dockerClient.ContainerCreate(ctx, client.ContainerCreateOptions{Config: &container.Config{
		Image: evalImageName,
		Cmd:   []string{"3 +"},
	}, Name: "ecs-arith-invalid"})
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

func TestECSArithmeticDivision(t *testing.T) {
	ctx := context.Background()

	resp, err := dockerClient.ContainerCreate(ctx, client.ContainerCreateOptions{Config: &container.Config{
		Image: evalImageName,
		Cmd:   []string{"10 / 3"},
	}, Name: "ecs-arith-div"})
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

	logs := readContainerLogs(t, resp.ID)
	if !strings.Contains(logs, "3.333") {
		t.Errorf("expected logs to contain '3.333', got %q", logs)
	}
}

func TestECSArithmeticWithLabels(t *testing.T) {
	ctx := context.Background()

	resp, err := dockerClient.ContainerCreate(ctx, client.ContainerCreateOptions{Config: &container.Config{
		Image:  evalImageName,
		Cmd:    []string{"100 - 42"},
		Labels: map[string]string{"arith-test": "ecs"},
	}, Name: "ecs-arith-labels"})
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

	logs := readContainerLogs(t, resp.ID)
	if !strings.Contains(logs, "58") {
		t.Errorf("expected logs to contain '58', got %q", logs)
	}

	// Verify label filter finds the container
	listed, err := dockerClient.ContainerList(ctx, client.ContainerListOptions{
		All:     true,
		Filters: client.Filters{}.Add("label", "arith-test=ecs"),
	})
	containers := listed.Items
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

func TestECSArithmeticEnvVar(t *testing.T) {
	ctx := context.Background()

	resp, err := dockerClient.ContainerCreate(ctx, client.ContainerCreateOptions{Config: &container.Config{
		Image: evalImageName,
		Cmd:   []string{"(3 + 4) * 2"},
		Env:   []string{"EXPR=(3 + 4) * 2"},
	}, Name: "ecs-arith-env"})
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

	logs := readContainerLogs(t, resp.ID)
	if !strings.Contains(logs, "14") {
		t.Errorf("expected logs to contain '14', got %q", logs)
	}
}
