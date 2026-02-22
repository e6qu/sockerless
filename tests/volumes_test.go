package tests

import (
	"testing"

	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/volume"
)

func TestVolumeCreate(t *testing.T) {
	name := createVolume(t, "test-vol")
	defer removeVolume(t, name)

	if name != "test-vol" {
		t.Errorf("expected name test-vol, got %s", name)
	}
}

func TestVolumeInspect(t *testing.T) {
	name := createVolume(t, "test-vol-inspect")
	defer removeVolume(t, name)

	vol, err := dockerClient.VolumeInspect(ctx, name)
	if err != nil {
		t.Fatalf("volume inspect failed: %v", err)
	}

	if vol.Name != "test-vol-inspect" {
		t.Errorf("expected name test-vol-inspect, got %s", vol.Name)
	}

	if vol.Driver == "" {
		t.Error("expected non-empty driver")
	}

	if vol.Mountpoint == "" {
		t.Error("expected non-empty mountpoint")
	}
}

func TestVolumeList(t *testing.T) {
	name := createVolume(t, "test-vol-list")
	defer removeVolume(t, name)

	vols, err := dockerClient.VolumeList(ctx, volume.ListOptions{})
	if err != nil {
		t.Fatalf("volume list failed: %v", err)
	}

	found := false
	for _, v := range vols.Volumes {
		if v.Name == "test-vol-list" {
			found = true
			break
		}
	}
	if !found {
		t.Error("created volume not found in list")
	}
}

func TestVolumeRemove(t *testing.T) {
	name := createVolume(t, "test-vol-remove")

	if err := dockerClient.VolumeRemove(ctx, name, true); err != nil {
		t.Fatalf("volume remove failed: %v", err)
	}

	// Inspect should fail
	_, err := dockerClient.VolumeInspect(ctx, name)
	if err == nil {
		t.Error("expected error inspecting removed volume")
	}
}

// Ensure the filters import is used
var _ = filters.Args{}
