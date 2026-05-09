package awscommon

import (
	"context"
	"strings"
	"testing"

	core "github.com/sockerless/backend-core"
)

func TestEFSEphemeralDriver_BackingMatches(t *testing.T) {
	d := NewEFSEphemeralDriver(nil)
	if d.Backing() != core.BackingEFSEphemeral {
		t.Errorf("Backing() = %q, want %q", d.Backing(), core.BackingEFSEphemeral)
	}
}

func TestEFSEphemeralDriver_CloudSpecRequiresIDs(t *testing.T) {
	d := NewEFSEphemeralDriver(nil)
	_, err := d.CloudSpec(core.SharedVolumeRef{Name: "v1"})
	if err == nil || !strings.Contains(err.Error(), "AccessPointID required") {
		t.Errorf("missing AP: err = %v; want AccessPointID required", err)
	}
	_, err = d.CloudSpec(core.SharedVolumeRef{Name: "v1", EFSAccessPointID: "fsap-x"})
	if err == nil || !strings.Contains(err.Error(), "FileSystemID required") {
		t.Errorf("missing FS: err = %v; want FileSystemID required", err)
	}
}

func TestEFSEphemeralDriver_CloudSpecPopulates(t *testing.T) {
	d := NewEFSEphemeralDriver(nil)
	spec, err := d.CloudSpec(core.SharedVolumeRef{
		Name:             "v1",
		EFSAccessPointID: "fsap-abc",
		EFSFileSystemID:  "fs-xyz",
		ReadOnly:         true,
	})
	if err != nil {
		t.Fatalf("CloudSpec: unexpected err %v", err)
	}
	if spec.Kind != core.BackingEFSEphemeral || spec.EFSEphemeral == nil {
		t.Fatalf("CloudSpec kind/payload mismatch: %#v", spec)
	}
	if spec.EFSEphemeral.AccessPointID != "fsap-abc" || spec.EFSEphemeral.FileSystemID != "fs-xyz" || !spec.EFSEphemeral.ReadOnly {
		t.Errorf("payload wrong: %#v", spec.EFSEphemeral)
	}
}

func TestEFSEphemeralDriver_PreExecPostExecAreNoOps(t *testing.T) {
	d := NewEFSEphemeralDriver(nil)
	hints, err := d.PreExec(context.Background(), core.SharedVolumeRef{}, "x", "/l", "/r")
	if err != nil || hints != nil {
		t.Errorf("PreExec: got %v, %v; want nil, nil", hints, err)
	}
	if err := d.PostExec(context.Background(), core.SharedVolumeRef{}, "x", "/l"); err != nil {
		t.Errorf("PostExec: got %v; want nil", err)
	}
}
