package core

import (
	"context"
	"testing"
)

func TestStorageBackingRegistry_DefaultEmptyDir(t *testing.T) {
	r := NewStorageBackingRegistry()
	d := r.Resolve(BackingEmptyDir)
	if d.Backing() != BackingEmptyDir {
		t.Errorf("Resolve(emptyDir).Backing() = %q, want %q", d.Backing(), BackingEmptyDir)
	}
}

func TestStorageBackingRegistry_UnknownFallsBackToEmptyDir(t *testing.T) {
	r := NewStorageBackingRegistry()
	d := r.Resolve(StorageBacking("nonexistent"))
	if d == nil {
		t.Fatal("Resolve returned nil for unknown backing; want fallback to emptyDir")
	}
	if d.Backing() != BackingEmptyDir {
		t.Errorf("Resolve(unknown).Backing() = %q, want emptyDir", d.Backing())
	}
}

func TestStorageBackingRegistry_EmptyResolvesToEmptyDir(t *testing.T) {
	r := NewStorageBackingRegistry()
	d := r.Resolve("")
	if d.Backing() != BackingEmptyDir {
		t.Errorf("Resolve(empty).Backing() = %q, want emptyDir (default for unset Backing field)", d.Backing())
	}
}

func TestStorageBackingRegistry_CustomDriverRegistration(t *testing.T) {
	r := NewStorageBackingRegistry()
	mock := &mockDriver{backing: BackingGCSSync}
	r.Register(mock)
	got := r.Resolve(BackingGCSSync)
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
	hints, err := d.PreExec(context.Background(), SharedVolumeRef{Name: "x"}, "exec1", "/tmp")
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

// mockDriver is a minimal StorageBackingDriver used for registry tests.
type mockDriver struct {
	backing StorageBacking
}

func (m *mockDriver) Backing() StorageBacking { return m.backing }
func (m *mockDriver) CloudSpec(vol SharedVolumeRef) (BackingSpec, error) {
	return BackingSpec{Kind: m.backing}, nil
}
func (m *mockDriver) PreExec(ctx context.Context, vol SharedVolumeRef, execID, localPath string) (map[string]string, error) {
	return nil, nil
}
func (m *mockDriver) PostExec(ctx context.Context, vol SharedVolumeRef, execID, localPath string) error {
	return nil
}
