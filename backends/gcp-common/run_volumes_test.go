package gcpcommon

import (
	"strings"
	"testing"

	core "github.com/sockerless/backend-core"
)

func TestRunVolumeFromBackingMemory(t *testing.T) {
	got, err := RunVolumeFromBackingSpec("ws", core.BackingSpec{Kind: core.BackingMemory, Memory: &core.MemorySpec{SizeMB: 128}}, "Cloud Run")
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

	// SizeMB=0 and a nil memory spec both leave the size to the container's
	// memory limit.
	for _, spec := range []core.BackingSpec{
		{Kind: core.BackingMemory, Memory: &core.MemorySpec{SizeMB: 0}},
		{Kind: core.BackingMemory},
	} {
		got, err := RunVolumeFromBackingSpec("ws", spec, "Cloud Run")
		if err != nil {
			t.Fatalf("translation failed: %v", err)
		}
		if got.GetEmptyDir() == nil || got.GetEmptyDir().SizeLimit != "" {
			t.Errorf("spec %+v: want EmptyDir without SizeLimit, got %+v", spec, got.GetEmptyDir())
		}
	}
}

func TestRunVolumeFromBackingEmptyDirAndSync(t *testing.T) {
	for _, kind := range []core.StorageBacking{core.BackingEmptyDir, core.BackingGCSSync} {
		got, err := RunVolumeFromBackingSpec("ws", core.BackingSpec{Kind: kind}, "Cloud Run")
		if err != nil || got.GetEmptyDir() == nil {
			t.Fatalf("%s: = %+v, %v; want an in-memory EmptyDir", kind, got, err)
		}
	}
}

// TestRunVolumeFromBackingRejections: backings Cloud Run cannot implement
// are refused with the rejected backing, the platform, and the recommended
// alternative in the message, so the operator can act on it.
func TestRunVolumeFromBackingRejections(t *testing.T) {
	cases := []struct {
		spec core.BackingSpec
		want []string
	}{
		{core.BackingSpec{Kind: core.BackingPDEphemeral, PDEphemeral: &core.PDEphemeralSpec{DiskSizeGB: 10, Zone: "us-central1-a"}}, []string{"pd-ephemeral", "Cloud Run Functions", "gcs-sync"}},
		{core.BackingSpec{Kind: core.BackingGCSFuse, GCS: &core.GCSSpec{Bucket: "test-bucket"}}, []string{"gcs-fuse", "Cloud Run Functions", "cache-TTL", "gcs-sync"}},
		{core.BackingSpec{Kind: core.StorageBacking("nope")}, []string{"unsupported backing kind", "nope"}},
	}
	for _, tc := range cases {
		_, err := RunVolumeFromBackingSpec("ws", tc.spec, "Cloud Run Functions")
		if err == nil {
			t.Fatalf("%s: expected rejection", tc.spec.Kind)
		}
		for _, want := range tc.want {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("%s: error missing %q: %s", tc.spec.Kind, want, err)
			}
		}
	}
}
