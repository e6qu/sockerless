package aca

import (
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appcontainers/armappcontainers/v3"
	core "github.com/sockerless/backend-core"
)

func TestTranslateBackingSpecMemoryEmptyDir(t *testing.T) {
	// ACA revisions support StorageTypeEmptyDir as a first-class
	// volume type — direct match for the cloud-agnostic memory
	// backing.
	spec := core.BackingSpec{
		Kind:   core.BackingMemory,
		Memory: &core.MemorySpec{SizeMB: 128},
	}
	got, err := translateBackingSpecToACAVolume("ws", "", spec)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if got.StorageType == nil || *got.StorageType != armappcontainers.StorageTypeEmptyDir {
		t.Errorf("StorageType = %v, want EmptyDir", got.StorageType)
	}
	if got.StorageName != nil && *got.StorageName != "" {
		t.Errorf("StorageName should be unset for EmptyDir, got %q", *got.StorageName)
	}
}

func TestTranslateBackingSpecAzureFilesEphemeralOK(t *testing.T) {
	spec := core.BackingSpec{
		Kind: core.BackingAzureFilesEphemeral,
		AzureFilesEphemeral: &core.AzureFilesEphemeralSpec{
			StorageAccount: "myacct",
			ShareName:      "myshare",
		},
	}
	got, err := translateBackingSpecToACAVolume("ws", "myshare", spec)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if got.StorageType == nil || *got.StorageType != armappcontainers.StorageTypeAzureFile {
		t.Errorf("StorageType = %v, want AzureFile", got.StorageType)
	}
}
