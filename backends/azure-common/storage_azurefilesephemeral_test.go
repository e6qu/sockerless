package azurecommon

import (
	"context"
	"strings"
	"testing"

	core "github.com/sockerless/backend-core"
)

func TestAzureFilesEphemeralDriver_BackingMatches(t *testing.T) {
	d := NewAzureFilesEphemeralDriver("acct")
	if d.Backing() != core.BackingAzureFilesEphemeral {
		t.Errorf("Backing() = %q, want %q", d.Backing(), core.BackingAzureFilesEphemeral)
	}
}

func TestAzureFilesEphemeralDriver_CloudSpecRequiresAccountAndShare(t *testing.T) {
	d := NewAzureFilesEphemeralDriver("")
	_, err := d.CloudSpec(core.SharedVolumeRef{Name: "v1"})
	if err == nil || !strings.Contains(err.Error(), "StorageAccount required") {
		t.Errorf("missing acct: err = %v; want StorageAccount required", err)
	}
	_, err = d.CloudSpec(core.SharedVolumeRef{Name: "v1", AzureStorageAccount: "acct"})
	if err == nil || !strings.Contains(err.Error(), "ShareName required") {
		t.Errorf("missing share: err = %v; want ShareName required", err)
	}
}

func TestAzureFilesEphemeralDriver_CloudSpecUsesDefaults(t *testing.T) {
	d := NewAzureFilesEphemeralDriver("default-acct")
	spec, err := d.CloudSpec(core.SharedVolumeRef{Name: "v1", AzureShareName: "share-1"})
	if err != nil {
		t.Fatalf("CloudSpec: unexpected err %v", err)
	}
	if spec.AzureFilesEphemeral.StorageAccount != "default-acct" || spec.AzureFilesEphemeral.ShareName != "share-1" {
		t.Errorf("payload wrong: %#v", spec.AzureFilesEphemeral)
	}
}

func TestAzureFilesEphemeralDriver_CloudSpecOverrides(t *testing.T) {
	d := NewAzureFilesEphemeralDriver("default-acct")
	spec, err := d.CloudSpec(core.SharedVolumeRef{
		Name:                "v1",
		AzureStorageAccount: "override-acct",
		AzureShareName:      "share-1",
		ReadOnly:            true,
	})
	if err != nil {
		t.Fatalf("CloudSpec: unexpected err %v", err)
	}
	if spec.AzureFilesEphemeral.StorageAccount != "override-acct" || !spec.AzureFilesEphemeral.ReadOnly {
		t.Errorf("override ignored: %#v", spec.AzureFilesEphemeral)
	}
}

func TestAzureFilesEphemeralDriver_PreExecPostExecAreNoOps(t *testing.T) {
	d := NewAzureFilesEphemeralDriver("acct")
	hints, err := d.PreExec(context.Background(), core.SharedVolumeRef{}, "x", "/l", "/r")
	if err != nil || hints != nil {
		t.Errorf("PreExec: got %v, %v; want nil, nil", hints, err)
	}
	if err := d.PostExec(context.Background(), core.SharedVolumeRef{}, "x", "/l"); err != nil {
		t.Errorf("PostExec: got %v; want nil", err)
	}
}
