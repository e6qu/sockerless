package gcf

import (
	"testing"

	core "github.com/sockerless/backend-core"
)

func TestRunpbVolumeFromBackingMemoryGCF(t *testing.T) {
	spec := core.BackingSpec{
		Kind:   core.BackingMemory,
		Memory: &core.MemorySpec{SizeMB: 64},
	}
	got, err := runpbVolumeFromBackingSpec("scratch", spec)
	if err != nil {
		t.Fatalf("translation failed: %v", err)
	}
	if got.Name != "scratch" {
		t.Errorf("name = %q", got.Name)
	}
	emptyDir := got.GetEmptyDir()
	if emptyDir == nil {
		t.Fatalf("expected EmptyDir volume type")
	}
	if emptyDir.SizeLimit != "64Mi" {
		t.Errorf("SizeLimit = %q, want 64Mi", emptyDir.SizeLimit)
	}
}

func TestRunpbVolumeFromBackingMemoryGCFNoSize(t *testing.T) {
	spec := core.BackingSpec{
		Kind:   core.BackingMemory,
		Memory: &core.MemorySpec{SizeMB: 0},
	}
	got, err := runpbVolumeFromBackingSpec("scratch", spec)
	if err != nil {
		t.Fatalf("translation failed: %v", err)
	}
	if got.GetEmptyDir().SizeLimit != "" {
		t.Errorf("SizeLimit should be empty for SizeMB=0")
	}
}
