package tests

import (
	"strings"
	"testing"

	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/volume"
)

// The ECS backend rejects every named-volume operation with a clear
// NotImplemented error — there's no silent metadata-only store.
// These tests pin that contract so a reintroduced silent store would
// fail CI. Real EFS access-point provisioning is queued as its own
// phase; when that lands, these tests are rewritten to exercise the
// new end-to-end volume lifecycle.

const volumeNotImpl = "does not support named volumes"

func assertVolumeNotImpl(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected NotImplemented error; got nil")
	}
	if !strings.Contains(err.Error(), volumeNotImpl) {
		t.Errorf("error = %q, want substring %q", err.Error(), volumeNotImpl)
	}
}

func TestVolumeCreate_NotImplemented(t *testing.T) {
	_, err := dockerClient.VolumeCreate(ctx, volume.CreateOptions{Name: "test-vol-create"})
	assertVolumeNotImpl(t, err)
}

func TestVolumeInspect_NotImplemented(t *testing.T) {
	_, err := dockerClient.VolumeInspect(ctx, "any-name")
	assertVolumeNotImpl(t, err)
}

func TestVolumeList_NotImplemented(t *testing.T) {
	_, err := dockerClient.VolumeList(ctx, volume.ListOptions{})
	assertVolumeNotImpl(t, err)
}

func TestVolumeRemove_NotImplemented(t *testing.T) {
	err := dockerClient.VolumeRemove(ctx, "any-name", true)
	assertVolumeNotImpl(t, err)
}

// Ensure the filters import is used
var _ = filters.Args{}
