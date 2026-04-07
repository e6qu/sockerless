package tests

import (
	"testing"

	"github.com/docker/docker/api/types/container"
)

func TestContainerLabels_RoundTrip(t *testing.T) {
	pullImage(t, "alpine")

	for name, c := range availableRunnerClients(t) {
		t.Run(name, func(t *testing.T) {
			labels := map[string]string{
				"app":                    "test",
				"env":                    "ci",
				"com.example.maintainer": "platform-team",
			}
			containerName := "label-test-" + generateTestID(name)
			resp, err := c.ContainerCreate(ctx, &container.Config{
				Image:  "alpine",
				Labels: labels,
				Cmd:    []string{"tail", "-f", "/dev/null"},
			}, nil, nil, nil, containerName)
			if err != nil {
				t.Fatalf("container create failed: %v", err)
			}
			defer c.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})

			inspect, err := c.ContainerInspect(ctx, resp.ID)
			if err != nil {
				t.Fatalf("inspect failed: %v", err)
			}

			for k, v := range labels {
				got, ok := inspect.Config.Labels[k]
				if !ok {
					t.Errorf("label %q not found in inspect result", k)
					continue
				}
				if got != v {
					t.Errorf("label %q: expected %q, got %q", k, v, got)
				}
			}
		})
	}
}

func TestContainerLabels_EmptyValue(t *testing.T) {
	pullImage(t, "alpine")

	for name, c := range availableRunnerClients(t) {
		t.Run(name, func(t *testing.T) {
			labels := map[string]string{
				"marker": "",
			}
			containerName := "label-empty-" + generateTestID(name)
			resp, err := c.ContainerCreate(ctx, &container.Config{
				Image:  "alpine",
				Labels: labels,
				Cmd:    []string{"tail", "-f", "/dev/null"},
			}, nil, nil, nil, containerName)
			if err != nil {
				t.Fatalf("container create failed: %v", err)
			}
			defer c.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})

			inspect, err := c.ContainerInspect(ctx, resp.ID)
			if err != nil {
				t.Fatalf("inspect failed: %v", err)
			}

			got, ok := inspect.Config.Labels["marker"]
			if !ok {
				t.Error("label 'marker' not found in inspect result")
			} else if got != "" {
				t.Errorf("expected empty label value, got %q", got)
			}
		})
	}
}

func TestContainerLabels_InList(t *testing.T) {
	pullImage(t, "alpine")

	for name, c := range availableRunnerClients(t) {
		t.Run(name, func(t *testing.T) {
			labels := map[string]string{
				"list-filter-test": "unique-value",
			}
			containerName := "label-list-" + generateTestID(name)
			resp, err := c.ContainerCreate(ctx, &container.Config{
				Image:  "alpine",
				Labels: labels,
				Cmd:    []string{"tail", "-f", "/dev/null"},
			}, nil, nil, nil, containerName)
			if err != nil {
				t.Fatalf("container create failed: %v", err)
			}
			defer c.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})

			// List containers and verify labels are present
			containers, err := c.ContainerList(ctx, container.ListOptions{All: true})
			if err != nil {
				t.Fatalf("list failed: %v", err)
			}

			found := false
			for _, ctr := range containers {
				if ctr.ID == resp.ID {
					found = true
					if ctr.Labels["list-filter-test"] != "unique-value" {
						t.Errorf("expected label in list, got %v", ctr.Labels)
					}
					break
				}
			}
			if !found {
				t.Error("container not found in list")
			}
		})
	}
}
