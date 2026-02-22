package tests

import (
	"testing"

	"github.com/docker/docker/api/types/image"
)

func TestImagePull(t *testing.T) {
	rc, err := dockerClient.ImagePull(ctx, "alpine", image.PullOptions{})
	if err != nil {
		t.Fatalf("image pull failed: %v", err)
	}
	defer rc.Close()

	buf := make([]byte, 4096)
	totalRead := 0
	for {
		n, err := rc.Read(buf)
		totalRead += n
		if err != nil {
			break
		}
	}

	if totalRead == 0 {
		t.Error("expected some pull progress output")
	}
}

func TestImageInspect(t *testing.T) {
	pullImage(t, "alpine")

	img, _, err := dockerClient.ImageInspectWithRaw(ctx, "alpine")
	if err != nil {
		t.Fatalf("image inspect failed: %v", err)
	}

	if img.ID == "" {
		t.Error("expected non-empty image ID")
	}

	if len(img.RepoTags) == 0 {
		t.Error("expected at least one repo tag")
	}

	if img.Os == "" {
		t.Error("expected non-empty OS")
	}

	if img.Architecture == "" {
		t.Error("expected non-empty Architecture")
	}
}

func TestImageTag(t *testing.T) {
	pullImage(t, "alpine")

	err := dockerClient.ImageTag(ctx, "alpine", "myrepo:mytag")
	if err != nil {
		t.Fatalf("image tag failed: %v", err)
	}

	// Inspect with new tag
	img, _, err := dockerClient.ImageInspectWithRaw(ctx, "myrepo:mytag")
	if err != nil {
		t.Fatalf("inspect tagged image failed: %v", err)
	}

	found := false
	for _, tag := range img.RepoTags {
		if tag == "myrepo:mytag" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected tag myrepo:mytag in %v", img.RepoTags)
	}
}
