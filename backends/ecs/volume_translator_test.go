package ecs

import (
	"strings"
	"testing"

	core "github.com/sockerless/backend-core"
)

func TestTranslateBackingSpecMemoryRejected(t *testing.T) {
	// ECS task-def Volumes don't have a tmpfs primitive; tmpfs lives
	// at the ContainerDefinition.LinuxParameters layer. Translator
	// rejects loudly rather than silently substituting Host{}
	// (disk-backed).
	spec := core.BackingSpec{
		Kind:   core.BackingMemory,
		Memory: &core.MemorySpec{SizeMB: 64},
	}
	_, err := translateBackingSpecToECSVolume("ws", spec)
	if err == nil {
		t.Fatal("expected error for BackingMemory on ECS")
	}
	msg := err.Error()
	for _, want := range []string{"memory", "Tmpfs", "emptyDir"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message missing %q: %s", want, msg)
		}
	}
}

func TestTranslateBackingSpecEFSEphemeralOK(t *testing.T) {
	spec := core.BackingSpec{
		Kind: core.BackingEFSEphemeral,
		EFSEphemeral: &core.EFSEphemeralSpec{
			FileSystemID:  "fs-abc",
			AccessPointID: "fsap-xyz",
		},
	}
	got, err := translateBackingSpecToECSVolume("ws", spec)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if got.EfsVolumeConfiguration == nil {
		t.Fatal("EfsVolumeConfiguration is nil")
	}
	if *got.EfsVolumeConfiguration.FileSystemId != "fs-abc" {
		t.Errorf("FileSystemId mismatch: %s", *got.EfsVolumeConfiguration.FileSystemId)
	}
}
