package tests

import (
	"context"
	"testing"
	"time"

	"github.com/moby/moby/client"

	"github.com/moby/moby/api/types/container"
)

// TestResourceTaggingIntegration validates that the tag additions don't break
// container lifecycle. Tags are verified to be correctly applied via unit tests
// in backends/core (TestTagSet*) and registry tests (TestRegistry*).
// The simulator tag storage is validated via the ECS/Lambda structs accepting tags.
func TestResourceTaggingIntegration(t *testing.T) {
	if dockerClient == nil {
		t.Fatal("dockerClient must be initialised by TestMain (no fallback)")
	}
	ctx := context.Background()

	// Pull image
	rc, err := dockerClient.ImagePull(ctx, "alpine:latest", client.ImagePullOptions{})
	if err != nil {
		t.Fatalf("image pull failed: %v", err)
	}
	buf := make([]byte, 4096)
	for {
		if _, err := rc.Read(buf); err != nil {
			break
		}
	}
	rc.Close()

	// Create container with labels
	resp, err := dockerClient.ContainerCreate(ctx, client.ContainerCreateOptions{Config: &container.Config{
		Image: "alpine:latest",
		Cmd:   []string{"echo", "resource-tracking-test"},
		Labels: map[string]string{
			"test": "resource-tracking",
		},
	}, Name: "resource-tracking-test"})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	defer dockerClient.ContainerRemove(ctx, resp.ID, client.ContainerRemoveOptions{Force: true})

	// Start
	if _, err := dockerClient.ContainerStart(ctx, resp.ID, client.ContainerStartOptions{}); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// Wait for container to finish
	waited := dockerClient.ContainerWait(ctx, resp.ID, client.ContainerWaitOptions{Condition: container.WaitConditionNotRunning})
	waitCh, errCh := waited.Result, waited.Error
	select {
	case result := <-waitCh:
		if result.StatusCode != 0 {
			t.Errorf("expected exit 0, got %d", result.StatusCode)
		}
	case err := <-errCh:
		t.Fatalf("wait error: %v", err)
	case <-time.After(30 * time.Second):
		t.Fatal("timeout")
	}

	// Inspect should still work (tags don't break anything)
	inspected, err := dockerClient.ContainerInspect(ctx, resp.ID, client.ContainerInspectOptions{})
	info := inspected.Container
	if err != nil {
		t.Fatalf("inspect failed: %v", err)
	}
	if info.Config.Labels["test"] != "resource-tracking" {
		t.Errorf("expected label test=resource-tracking, got %v", info.Config.Labels)
	}
}
