package tests

import (
	"testing"

	"github.com/docker/docker/api/types/container"
)

func TestContainerMemoryLimit(t *testing.T) {
	pullImage(t, "alpine")

	for name, c := range availableRunnerClients(t) {
		t.Run(name, func(t *testing.T) {
			containerName := "mem-test-" + generateTestID(name)
			resp, err := c.ContainerCreate(ctx, &container.Config{
				Image: "alpine",
				Cmd:   []string{"tail", "-f", "/dev/null"},
			}, &container.HostConfig{
				Resources: container.Resources{
					Memory: 512 * 1024 * 1024, // 512MB
				},
			}, nil, nil, containerName)
			if err != nil {
				t.Fatalf("container create failed: %v", err)
			}
			defer c.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})

			inspect, err := c.ContainerInspect(ctx, resp.ID)
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
			resp, err := c.ContainerCreate(ctx, &container.Config{
				Image: "alpine",
				Cmd:   []string{"tail", "-f", "/dev/null"},
			}, &container.HostConfig{
				Resources: container.Resources{
					CPUShares: 512,
				},
			}, nil, nil, containerName)
			if err != nil {
				t.Fatalf("container create failed: %v", err)
			}
			defer c.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})

			inspect, err := c.ContainerInspect(ctx, resp.ID)
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
			resp, err := c.ContainerCreate(ctx, &container.Config{
				Image: "alpine",
				Cmd:   []string{"tail", "-f", "/dev/null"},
			}, &container.HostConfig{
				Resources: container.Resources{
					Memory:    256 * 1024 * 1024, // 256MB
					CPUShares: 1024,
				},
			}, nil, nil, containerName)
			if err != nil {
				t.Fatalf("container create failed: %v", err)
			}
			defer c.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})

			inspect, err := c.ContainerInspect(ctx, resp.ID)
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
