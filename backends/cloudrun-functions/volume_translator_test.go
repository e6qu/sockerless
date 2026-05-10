package gcf

import (
	"strings"
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

func TestRunpbVolumeFromBackingPDEphemeralRejectedGCF(t *testing.T) {
	spec := core.BackingSpec{
		Kind: core.BackingPDEphemeral,
		PDEphemeral: &core.PDEphemeralSpec{
			DiskSizeGB: 10,
			Zone:       "us-central1-a",
		},
	}
	_, err := runpbVolumeFromBackingSpec("scratch", spec)
	if err == nil {
		t.Fatal("expected error for BackingPDEphemeral on GCF")
	}
	msg := err.Error()
	for _, want := range []string{"pd-ephemeral", "Cloud Functions", "gcs-sync"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error missing %q: %s", want, msg)
		}
	}
}

func TestRunpbVolumeFromBackingGCSFuseRejectedGCF(t *testing.T) {
	// gcs-fuse on Gen2 functions is broken per BUG-944 — Gen2 sits on
	// Cloud Run Services which rejects the cache-TTL flags. Translator
	// must reject with a concrete pointer at gcs-sync.
	spec := core.BackingSpec{
		Kind: core.BackingGCSFuse,
		GCS:  &core.GCSSpec{Bucket: "test-bucket"},
	}
	_, err := runpbVolumeFromBackingSpec("scratch", spec)
	if err == nil {
		t.Fatal("expected error for BackingGCSFuse on GCF")
	}
	msg := err.Error()
	for _, want := range []string{"gcs-fuse", "Cloud Functions", "gcs-sync", "BUG-944"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error missing %q: %s", want, msg)
		}
	}
}
