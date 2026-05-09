package core

import (
	"context"
	"testing"
)

func TestStorageBackingRegistry_ResolveEmptyDir(t *testing.T) {
	r := NewStorageBackingRegistry()
	d, err := r.Resolve(BackingEmptyDir)
	if err != nil {
		t.Fatalf("Resolve(emptyDir) returned error: %v", err)
	}
	if d.Backing() != BackingEmptyDir {
		t.Errorf("Resolve(emptyDir).Backing() = %q, want %q", d.Backing(), BackingEmptyDir)
	}
}

// TestStorageBackingRegistry_UnknownFailsLoudly enforces the no-fallback
// architectural choice: silent default selection would mask operator
// misconfiguration (e.g. cells 7+8 needing gcs-fuse vs cells 5+6 needing
// gcs-sync — emptyDir "works" until the first cross-Service read).
func TestStorageBackingRegistry_UnknownFailsLoudly(t *testing.T) {
	r := NewStorageBackingRegistry()
	d, err := r.Resolve(StorageBacking("nonexistent"))
	if err == nil {
		t.Fatal("Resolve(unknown) should return error per no-fallbacks directive")
	}
	if d != nil {
		t.Errorf("Resolve(unknown) should return nil driver alongside the error, got %v", d)
	}
}

// TestStorageBackingRegistry_EmptyFailsLoudly — same rationale: empty
// SharedVolume.Backing is operator misconfiguration, not a default.
func TestStorageBackingRegistry_EmptyFailsLoudly(t *testing.T) {
	r := NewStorageBackingRegistry()
	d, err := r.Resolve("")
	if err == nil {
		t.Fatal("Resolve(empty) should return error per no-fallbacks directive — Backing is operator-required")
	}
	if d != nil {
		t.Errorf("Resolve(empty) should return nil driver, got %v", d)
	}
}

func TestStorageBackingRegistry_CustomDriverRegistration(t *testing.T) {
	r := NewStorageBackingRegistry()
	mock := &mockDriver{backing: BackingGCSSync}
	r.Register(mock)
	got, err := r.Resolve(BackingGCSSync)
	if err != nil {
		t.Fatalf("Resolve(gcs-sync) after Register: %v", err)
	}
	if got != mock {
		t.Errorf("Resolve(gcs-sync) returned wrong driver — want the registered mock")
	}
}

func TestStorageBackingRegistry_Register_NilPanics(t *testing.T) {
	r := NewStorageBackingRegistry()
	defer func() {
		if recover() == nil {
			t.Error("Register(nil) should panic")
		}
	}()
	r.Register(nil)
}

func TestEmptyDirDriver_CloudSpec(t *testing.T) {
	d := &EmptyDirDriver{}
	spec, err := d.CloudSpec(SharedVolumeRef{Name: "x", ContainerPath: "/y"})
	if err != nil {
		t.Fatal(err)
	}
	if spec.Kind != BackingEmptyDir {
		t.Errorf("Kind = %q, want emptyDir", spec.Kind)
	}
	if spec.EmptyDir == nil {
		t.Error("EmptyDir spec should be set")
	}
	if spec.GCS != nil {
		t.Error("GCS spec must be nil for emptyDir backing")
	}
}

func TestEmptyDirDriver_HooksAreNoop(t *testing.T) {
	d := &EmptyDirDriver{}
	hints, err := d.PreExec(context.Background(), SharedVolumeRef{Name: "x"}, "exec1", "/tmp", "/tmp")
	if err != nil {
		t.Errorf("PreExec returned error: %v", err)
	}
	if hints != nil {
		t.Errorf("PreExec hints = %v, want nil for emptyDir (no sync)", hints)
	}
	if err := d.PostExec(context.Background(), SharedVolumeRef{Name: "x"}, "exec1", "/tmp"); err != nil {
		t.Errorf("PostExec returned error: %v", err)
	}
}

func TestMemoryDriver_BackingMatches(t *testing.T) {
	d := NewMemoryDriver(64)
	if d.Backing() != BackingMemory {
		t.Errorf("Backing() = %q, want %q", d.Backing(), BackingMemory)
	}
}

func TestMemoryDriver_CloudSpec_UsesDefault(t *testing.T) {
	d := NewMemoryDriver(64)
	spec, err := d.CloudSpec(SharedVolumeRef{Name: "x"})
	if err != nil {
		t.Fatalf("unexpected err %v", err)
	}
	if spec.Kind != BackingMemory || spec.Memory == nil {
		t.Fatalf("kind/payload mismatch: %#v", spec)
	}
	if spec.Memory.SizeMB != 64 {
		t.Errorf("default size = %d, want 64", spec.Memory.SizeMB)
	}
}

func TestMemoryDriver_CloudSpec_OverrideWins(t *testing.T) {
	d := NewMemoryDriver(64)
	spec, err := d.CloudSpec(SharedVolumeRef{Name: "x", MemorySizeMB: 256})
	if err != nil {
		t.Fatalf("unexpected err %v", err)
	}
	if spec.Memory.SizeMB != 256 {
		t.Errorf("override = %d, want 256", spec.Memory.SizeMB)
	}
}

func TestMemoryDriver_CloudSpec_RejectsZero(t *testing.T) {
	d := NewMemoryDriver(0)
	_, err := d.CloudSpec(SharedVolumeRef{Name: "x"})
	if err == nil {
		t.Fatal("zero size should error")
	}
}

func TestMemoryDriver_PreExecPostExec_NoOps(t *testing.T) {
	d := NewMemoryDriver(64)
	hints, err := d.PreExec(context.Background(), SharedVolumeRef{}, "x", "/l", "/r")
	if err != nil || hints != nil {
		t.Errorf("PreExec: got %v, %v; want nil, nil", hints, err)
	}
	if err := d.PostExec(context.Background(), SharedVolumeRef{}, "x", "/l"); err != nil {
		t.Errorf("PostExec: got %v; want nil", err)
	}
}

// mockDriver is a minimal StorageBackingDriver used for registry tests.
type mockDriver struct {
	backing StorageBacking
}

func (m *mockDriver) Backing() StorageBacking { return m.backing }
func (m *mockDriver) CloudSpec(vol SharedVolumeRef) (BackingSpec, error) {
	return BackingSpec{Kind: m.backing}, nil
}
func (m *mockDriver) PreExec(ctx context.Context, vol SharedVolumeRef, execID, localPath, remotePath string) (map[string][]string, error) {
	return nil, nil
}
func (m *mockDriver) PostExec(ctx context.Context, vol SharedVolumeRef, execID, localPath string) error {
	return nil
}
