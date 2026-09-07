package tests

import (
	"testing"

	"github.com/moby/moby/client"

	"github.com/moby/moby/api/types/container"
)

func TestContainerMemoryLimit(t *testing.T) {
	pullImage(t, "alpine")

	for name, c := range availableRunnerClients(t) {
		t.Run(name, func(t *testing.T) {
			containerName := "mem-test-" + generateTestID(name)
			resp, err := c.ContainerCreate(ctx, client.ContainerCreateOptions{Config: &container.Config{
				Image: "alpine",
				Cmd:   []string{"tail", "-f", "/dev/null"},
			}, HostConfig: &container.HostConfig{
				Resources: container.Resources{
					Memory: 512 * 1024 * 1024, // 512MB
				},
			}, Name: containerName})
			if err != nil {
				t.Fatalf("container create failed: %v", err)
			}
			defer c.ContainerRemove(ctx, resp.ID, client.ContainerRemoveOptions{Force: true})

			inspected, err := c.ContainerInspect(ctx, resp.ID, client.ContainerInspectOptions{})
			inspect := inspected.Container
			if err != nil {
				t.Fatalf("inspect failed: %v", err)
			}

			if inspect.HostConfig.Memory != 512*1024*1024 {
				t.Errorf("expected memory limit 512MB (%d), got %d",
					int64(512*1024*1024), inspect.HostConfig.Memory)
			}
		})
	}
}

func TestContainerCPUShares(t *testing.T) {
	pullImage(t, "alpine")

	for name, c := range availableRunnerClients(t) {
		t.Run(name, func(t *testing.T) {
			containerName := "cpu-test-" + generateTestID(name)
			resp, err := c.ContainerCreate(ctx, client.ContainerCreateOptions{Config: &container.Config{
				Image: "alpine",
				Cmd:   []string{"tail", "-f", "/dev/null"},
			}, HostConfig: &container.HostConfig{
				Resources: container.Resources{
					CPUShares: 512,
				},
			}, Name: containerName})
			if err != nil {
				t.Fatalf("container create failed: %v", err)
			}
			defer c.ContainerRemove(ctx, resp.ID, client.ContainerRemoveOptions{Force: true})

			inspected2, err := c.ContainerInspect(ctx, resp.ID, client.ContainerInspectOptions{})
			inspect := inspected2.Container
			if err != nil {
				t.Fatalf("inspect failed: %v", err)
			}

			if inspect.HostConfig.CPUShares != 512 {
				t.Errorf("expected CPU shares 512, got %d", inspect.HostConfig.CPUShares)
			}
		})
	}
}

func TestContainerMemoryAndCPU_Combined(t *testing.T) {
	pullImage(t, "alpine")

	for name, c := range availableRunnerClients(t) {
		t.Run(name, func(t *testing.T) {
			containerName := "resources-test-" + generateTestID(name)
			resp, err := c.ContainerCreate(ctx, client.ContainerCreateOptions{Config: &container.Config{
				Image: "alpine",
				Cmd:   []string{"tail", "-f", "/dev/null"},
			}, HostConfig: &container.HostConfig{
				Resources: container.Resources{
					Memory:    256 * 1024 * 1024, // 256MB
					CPUShares: 1024,
				},
			}, Name: containerName})
			if err != nil {
				t.Fatalf("container create failed: %v", err)
			}
			defer c.ContainerRemove(ctx, resp.ID, client.ContainerRemoveOptions{Force: true})

			inspected3, err := c.ContainerInspect(ctx, resp.ID, client.ContainerInspectOptions{})
			inspect := inspected3.Container
			if err != nil {
				t.Fatalf("inspect failed: %v", err)
			}

			if inspect.HostConfig.Memory != 256*1024*1024 {
				t.Errorf("expected memory limit 256MB (%d), got %d",
					int64(256*1024*1024), inspect.HostConfig.Memory)
			}
			if inspect.HostConfig.CPUShares != 1024 {
				t.Errorf("expected CPU shares 1024, got %d", inspect.HostConfig.CPUShares)
			}
		})
	}
}
