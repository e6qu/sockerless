package lambda

import (
	"strings"
	"testing"

	core "github.com/sockerless/backend-core"
)

func TestTranslateBackingSpecToLambdaEFS(t *testing.T) {
	spec := core.BackingSpec{
		Kind: core.BackingEFSEphemeral,
		EFSEphemeral: &core.EFSEphemeralSpec{
			FileSystemID:  "fs-abc",
			AccessPointID: "arn:aws:elasticfilesystem:us-east-1:123:access-point/fsap-xyz",
		},
	}
	got, err := translateBackingSpecToLambda(spec)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if got.Arn == nil || *got.Arn != "arn:aws:elasticfilesystem:us-east-1:123:access-point/fsap-xyz" {
		t.Errorf("Arn = %v", got.Arn)
	}
	if got.LocalMountPath == nil || *got.LocalMountPath != LambdaSharedMountPath {
		t.Errorf("LocalMountPath = %v, want %s", got.LocalMountPath, LambdaSharedMountPath)
	}
}

func TestTranslateBackingSpecToLambdaEFSMissingAP(t *testing.T) {
	spec := core.BackingSpec{
		Kind:         core.BackingEFSEphemeral,
		EFSEphemeral: &core.EFSEphemeralSpec{FileSystemID: "fs-abc"},
	}
	_, err := translateBackingSpecToLambda(spec)
	if err == nil {
		t.Fatal("expected error for missing AccessPointID")
	}
	if !strings.Contains(err.Error(), "AccessPointID") {
		t.Errorf("error should mention AccessPointID: %v", err)
	}
}

func TestTranslateBackingSpecToLambdaEFSNilPayload(t *testing.T) {
	spec := core.BackingSpec{Kind: core.BackingEFSEphemeral}
	_, err := translateBackingSpecToLambda(spec)
	if err == nil {
		t.Fatal("expected error for nil payload")
	}
	if !strings.Contains(err.Error(), "missing payload") {
		t.Errorf("error should mention missing payload: %v", err)
	}
}

func TestTranslateBackingSpecToLambdaMemoryRejected(t *testing.T) {
	// Lambda has no tmpfs at the FileSystemConfig layer.
	// /tmp is per-invocation scratch, not a Docker-style mount.
	// Translator rejects loudly with a clear pointer.
	spec := core.BackingSpec{
		Kind:   core.BackingMemory,
		Memory: &core.MemorySpec{SizeMB: 64},
	}
	_, err := translateBackingSpecToLambda(spec)
	if err == nil {
		t.Fatal("expected error for BackingMemory on Lambda")
	}
	msg := err.Error()
	for _, want := range []string{"memory", "/tmp", "efs-ephemeral"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message missing %q: %s", want, msg)
		}
	}
}

func TestTranslateBackingSpecToLambdaUnknown(t *testing.T) {
	spec := core.BackingSpec{Kind: core.StorageBacking("future-kind")}
	_, err := translateBackingSpecToLambda(spec)
	if err == nil {
		t.Fatal("expected error for unknown backing")
	}
	if !strings.Contains(err.Error(), "not supported") {
		t.Errorf("error should explain rejection: %v", err)
	}
}
