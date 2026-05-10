package cloudrun

import (
	"strings"
	"testing"

	core "github.com/sockerless/backend-core"
)

func TestRunpbVolumeFromBackingMemory(t *testing.T) {
	spec := core.BackingSpec{
		Kind:   core.BackingMemory,
		Memory: &core.MemorySpec{SizeMB: 128},
	}
	got, err := runpbVolumeFromBackingSpec("ws", spec)
	if err != nil {
		t.Fatalf("translation failed: %v", err)
	}
	if got.Name != "ws" {
		t.Errorf("name = %q, want ws", got.Name)
	}
	emptyDir := got.GetEmptyDir()
	if emptyDir == nil {
		t.Fatalf("expected EmptyDir volume type, got %T", got.VolumeType)
	}
	if emptyDir.SizeLimit != "128Mi" {
		t.Errorf("SizeLimit = %q, want 128Mi", emptyDir.SizeLimit)
	}
}

func TestRunpbVolumeFromBackingMemoryNoSize(t *testing.T) {
	// SizeMB=0 → no SizeLimit set; cloud uses container's memory.
	spec := core.BackingSpec{
		Kind:   core.BackingMemory,
		Memory: &core.MemorySpec{SizeMB: 0},
	}
	got, err := runpbVolumeFromBackingSpec("ws", spec)
	if err != nil {
		t.Fatalf("translation failed: %v", err)
	}
	emptyDir := got.GetEmptyDir()
	if emptyDir == nil {
		t.Fatalf("expected EmptyDir volume type")
	}
	if emptyDir.SizeLimit != "" {
		t.Errorf("SizeLimit should be empty for SizeMB=0, got %q", emptyDir.SizeLimit)
	}
}

func TestRunpbVolumeFromBackingMemoryNilSpec(t *testing.T) {
	// Memory spec nil — driver default kicks in elsewhere; translator
	// should still produce an EmptyDir with no SizeLimit.
	spec := core.BackingSpec{Kind: core.BackingMemory}
	got, err := runpbVolumeFromBackingSpec("ws", spec)
	if err != nil {
		t.Fatalf("translation failed: %v", err)
	}
	if got.GetEmptyDir() == nil {
		t.Errorf("expected EmptyDir volume type")
	}
}

func TestRunpbVolumeFromBackingPDEphemeralRejected(t *testing.T) {
	// Cloud Run Services don't expose PD as a volume primitive.
	// Translator rejects loudly with concrete pointers at gcs-fuse /
	// gcs-sync alternatives + the GCE-backend bookmark.
	spec := core.BackingSpec{
		Kind: core.BackingPDEphemeral,
		PDEphemeral: &core.PDEphemeralSpec{
			DiskSizeGB: 10,
			Zone:       "us-central1-a",
		},
	}
	_, err := runpbVolumeFromBackingSpec("ws", spec)
	if err == nil {
		t.Fatal("expected error for BackingPDEphemeral on Cloud Run")
	}
	msg := err.Error()
	for _, want := range []string{"pd-ephemeral", "Cloud Run", "gcs-fuse"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error missing %q: %s", want, msg)
		}
	}
}
