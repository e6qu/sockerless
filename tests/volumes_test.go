package tests

import (
	"testing"

	"github.com/docker/docker/api/types/volume"
)

// Phase 91: the ECS backend backs Docker volumes with real EFS access
// points on a sockerless-owned filesystem. These tests exercise the
// full lifecycle (create → inspect → list → remove → 404) against
// whichever runner backend the harness has wired up.

func TestVolume_LifecycleEFSAccessPoint(t *testing.T) {
	name := "e2e-vol-" + generateTestID("lifecycle")

	created, err := dockerClient.VolumeCreate(ctx, volume.CreateOptions{Name: name})
	if err != nil {
		t.Fatalf("VolumeCreate: %v", err)
	}
	t.Cleanup(func() { _ = dockerClient.VolumeRemove(ctx, name, true) })

	if created.Name != name {
		t.Errorf("created.Name = %q, want %q", created.Name, name)
	}
	if created.Driver != "efs" {
		t.Errorf("created.Driver = %q, want efs", created.Driver)
	}
	if created.Options["accessPointId"] == "" {
		t.Errorf("created.Options missing accessPointId: %+v", created.Options)
	}

	inspected, err := dockerClient.VolumeInspect(ctx, name)
	if err != nil {
		t.Fatalf("VolumeInspect: %v", err)
	}
	if inspected.Name != name {
		t.Errorf("inspect Name = %q, want %q", inspected.Name, name)
	}

	listed, err := dockerClient.VolumeList(ctx, volume.ListOptions{})
	if err != nil {
		t.Fatalf("VolumeList: %v", err)
	}
	found := false
	for _, v := range listed.Volumes {
		if v.Name == name {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("VolumeList did not return %q; got %d volumes", name, len(listed.Volumes))
	}

	if err := dockerClient.VolumeRemove(ctx, name, false); err != nil {
		t.Fatalf("VolumeRemove: %v", err)
	}
	if _, err := dockerClient.VolumeInspect(ctx, name); err == nil {
		t.Errorf("VolumeInspect after remove: expected error, got success")
	}
}
