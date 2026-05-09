package gcpcommon

import (
	"context"
	"strings"
	"testing"

	core "github.com/sockerless/backend-core"
)

func TestPDEphemeralDriver_BackingMatches(t *testing.T) {
	d := NewPDEphemeralDriver("us-central1-a", 10)
	if d.Backing() != core.BackingPDEphemeral {
		t.Errorf("Backing() = %q, want %q", d.Backing(), core.BackingPDEphemeral)
	}
}

func TestPDEphemeralDriver_CloudSpecUsesDefaults(t *testing.T) {
	d := NewPDEphemeralDriver("us-central1-a", 10)
	spec, err := d.CloudSpec(core.SharedVolumeRef{Name: "v1"})
	if err != nil {
		t.Fatalf("CloudSpec: unexpected err %v", err)
	}
	if spec.Kind != core.BackingPDEphemeral || spec.PDEphemeral == nil {
		t.Fatalf("CloudSpec: kind/payload mismatch: %#v", spec)
	}
	if spec.PDEphemeral.DiskSizeGB != 10 || spec.PDEphemeral.Zone != "us-central1-a" {
		t.Errorf("CloudSpec defaults wrong: %#v", spec.PDEphemeral)
	}
}

func TestPDEphemeralDriver_CloudSpecOverrides(t *testing.T) {
	d := NewPDEphemeralDriver("us-central1-a", 10)
	spec, err := d.CloudSpec(core.SharedVolumeRef{Name: "v1", PDDiskSizeGB: 50, PDZone: "europe-west1-b"})
	if err != nil {
		t.Fatalf("CloudSpec: unexpected err %v", err)
	}
	if spec.PDEphemeral.DiskSizeGB != 50 || spec.PDEphemeral.Zone != "europe-west1-b" {
		t.Errorf("CloudSpec overrides ignored: %#v", spec.PDEphemeral)
	}
}

func TestPDEphemeralDriver_RejectsZeroSize(t *testing.T) {
	d := NewPDEphemeralDriver("us-central1-a", 0)
	_, err := d.CloudSpec(core.SharedVolumeRef{Name: "v1"})
	if err == nil {
		t.Fatal("CloudSpec with zero size should error")
	}
	if !strings.Contains(err.Error(), "size must be > 0") {
		t.Errorf("err = %v; want size-> 0 message", err)
	}
}

func TestPDEphemeralDriver_PreExecPostExecAreNoOps(t *testing.T) {
	d := NewPDEphemeralDriver("us-central1-a", 10)
	hints, err := d.PreExec(context.Background(), core.SharedVolumeRef{}, "x", "/l", "/r")
	if err != nil || hints != nil {
		t.Errorf("PreExec: got %v, %v; want nil, nil", hints, err)
	}
	if err := d.PostExec(context.Background(), core.SharedVolumeRef{}, "x", "/l"); err != nil {
		t.Errorf("PostExec: got %v; want nil", err)
	}
}
