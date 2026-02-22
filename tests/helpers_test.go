package tests

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// availableRunnerClients returns all runner-capable backends currently reachable.
// Memory is always included. Cloud backends are included when socket env var is set.
func availableRunnerClients(t *testing.T) map[string]*client.Client {
	t.Helper()
	clients := map[string]*client.Client{
		"memory": dockerClient,
	}
	for name, envVar := range map[string]string{
		"ecs":      "SOCKERLESS_ECS_SOCKET",
		"cloudrun": "SOCKERLESS_CLOUDRUN_SOCKET",
		"aca":      "SOCKERLESS_ACA_SOCKET",
		"docker":   "SOCKERLESS_DOCKER_SOCKET",
	} {
		if socket := os.Getenv(envVar); socket != "" {
			c, err := client.NewClientWithOpts(
				client.WithHost("unix://"+socket),
				client.WithAPIVersionNegotiation(),
			)
			if err != nil {
				t.Fatalf("failed to create %s client: %v", name, err)
			}
			clients[name] = c
		}
	}
	return clients
}

// generateTestID returns a time-based ID, optionally including extra parts to avoid collisions.
func generateTestID(parts ...string) string {
	id := time.Now().Format("150405")
	for _, p := range parts {
		id += "-" + p
	}
	return id
}

// pullImage pulls an image and waits for completion.
func pullImage(t *testing.T, ref string) {
	t.Helper()
	rc, err := dockerClient.ImagePull(ctx, ref, image.PullOptions{})
	if err != nil {
		t.Fatalf("image pull failed: %v", err)
	}
	defer rc.Close()
	// Read to completion
	buf := make([]byte, 4096)
	for {
		_, err := rc.Read(buf)
		if err != nil {
			break
		}
	}
}

// createContainer creates a container and returns its ID.
func createContainer(t *testing.T, name string, config *container.Config, hostConfig *container.HostConfig) string {
	t.Helper()
	resp, err := dockerClient.ContainerCreate(ctx, config, hostConfig, nil, nil, name)
	if err != nil {
		t.Fatalf("container create failed: %v", err)
	}
	return resp.ID
}

// removeContainer removes a container with force.
func removeContainer(t *testing.T, id string) {
	t.Helper()
	dockerClient.ContainerRemove(ctx, id, container.RemoveOptions{Force: true})
}

// createNetwork creates a network and returns its ID.
func createNetwork(t *testing.T, name string) string {
	t.Helper()
	resp, err := dockerClient.NetworkCreate(ctx, name, network.CreateOptions{
		Driver: "bridge",
	})
	if err != nil {
		t.Fatalf("network create failed: %v", err)
	}
	return resp.ID
}

// removeNetwork removes a network.
func removeNetwork(t *testing.T, id string) {
	t.Helper()
	dockerClient.NetworkRemove(ctx, id)
}

// createVolume creates a volume and returns its name.
func createVolume(t *testing.T, name string) string {
	t.Helper()
	vol, err := dockerClient.VolumeCreate(ctx, volume.CreateOptions{Name: name})
	if err != nil {
		t.Fatalf("volume create failed: %v", err)
	}
	return vol.Name
}

// removeVolume removes a volume.
func removeVolume(t *testing.T, name string) {
	t.Helper()
	dockerClient.VolumeRemove(ctx, name, true)
}

// used to satisfy the platform parameter in ContainerCreate
var defaultPlatform *ocispec.Platform

func withContext() context.Context {
	return ctx
}
