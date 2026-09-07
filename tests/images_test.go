package tests

import (
	"testing"

	"github.com/moby/moby/client"
)

func TestImagePull(t *testing.T) {
	rc, err := dockerClient.ImagePull(ctx, "alpine", client.ImagePullOptions{})
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

	img, err := dockerClient.ImageInspect(ctx, "alpine")
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

	_, err := dockerClient.ImageTag(ctx, client.ImageTagOptions{Source: "alpine", Target: "myrepo:mytag"})
	if err != nil {
		t.Fatalf("image tag failed: %v", err)
	}

	// Inspect with new tag
	img, err := dockerClient.ImageInspect(ctx, "myrepo:mytag")
	if err != nil {
		t.Fatalf("inspect tagged image failed: %v", err)
	}

	// The Moby client normalises image refs to fully-qualified form
	// (e.g. `myrepo:mytag` → `docker.io/library/myrepo:mytag`); the
	// engine may report either shape.
	found := false
	for _, tag := range img.RepoTags {
		if tag == "myrepo:mytag" || tag == "docker.io/library/myrepo:mytag" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected myrepo:mytag (or docker.io/library/myrepo:mytag) in %v", img.RepoTags)
	}
}
