package bleephub

import (
	"testing"
)

func TestExpandMatrix2x2(t *testing.T) {
	m := &MatrixDef{
		Values: map[string][]interface{}{
			"os": {"ubuntu", "macos"},
			"go": {"1.21", "1.22"},
		},
	}
	combos := ExpandMatrix(m)
	if len(combos) != 4 {
		t.Fatalf("combos = %d, want 4", len(combos))
	}

	// Check all 4 combinations exist
	found := map[string]bool{}
	for _, c := range combos {
		key := c["go"].(string) + "-" + c["os"].(string)
		found[key] = true
	}
	for _, want := range []string{"1.21-ubuntu", "1.21-macos", "1.22-ubuntu", "1.22-macos"} {
		if !found[want] {
			t.Errorf("missing combo: %s", want)
		}
	}
}

func TestExpandMatrixWithInclude(t *testing.T) {
	m := &MatrixDef{
		Values: map[string][]interface{}{
			"os": {"ubuntu"},
			"go": {"1.21", "1.22"},
		},
		Include: []map[string]interface{}{
			{"os": "ubuntu", "go": "1.23"},
		},
	}
	combos := ExpandMatrix(m)
	if len(combos) != 3 {
		t.Fatalf("combos = %d, want 3 (2 base + 1 include)", len(combos))
	}
}

func TestExpandMatrixWithExclude(t *testing.T) {
	m := &MatrixDef{
		Values: map[string][]interface{}{
			"os": {"ubuntu", "macos"},
			"go": {"1.21", "1.22"},
		},
		Exclude: []map[string]interface{}{
			{"os": "macos", "go": "1.21"},
		},
	}
	combos := ExpandMatrix(m)
	if len(combos) != 3 {
		t.Fatalf("combos = %d, want 3 (4 base - 1 exclude)", len(combos))
	}

	// Verify the excluded combo is absent
	for _, c := range combos {
		if c["os"] == "macos" && c["go"] == "1.21" {
			t.Error("excluded combo should not be present")
		}
	}
}

func TestExpandMatrixEmpty(t *testing.T) {
	m := &MatrixDef{}
	combos := ExpandMatrix(m)
	if combos != nil {
		t.Errorf("combos = %v, want nil", combos)
	}
}

func TestMatrixJobName(t *testing.T) {
	name := MatrixJobName("test", map[string]interface{}{
		"os": "ubuntu",
		"go": "1.22",
	})
	// Keys sorted alphabetically: go, os
	if name != "test (1.22, ubuntu)" {
		t.Errorf("name = %q", name)
	}
}

func TestMatrixJobNameEmpty(t *testing.T) {
	name := MatrixJobName("test", nil)
	if name != "test" {
		t.Errorf("name = %q, want test", name)
	}
}

func TestExpandMatrixSingle(t *testing.T) {
	m := &MatrixDef{
		Values: map[string][]interface{}{
			"version": {"1.0"},
		},
	}
	combos := ExpandMatrix(m)
	if len(combos) != 1 {
		t.Fatalf("combos = %d, want 1", len(combos))
	}
	if combos[0]["version"] != "1.0" {
		t.Errorf("version = %v", combos[0]["version"])
	}
}

func TestExpandMatrixIncludeExtraKey(t *testing.T) {
	m := &MatrixDef{
		Values: map[string][]interface{}{
			"os": {"ubuntu"},
		},
		Include: []map[string]interface{}{
			{"os": "ubuntu", "experimental": true},
		},
	}
	combos := ExpandMatrix(m)
	if len(combos) != 1 {
		t.Fatalf("combos = %d, want 1", len(combos))
	}
	if combos[0]["experimental"] != true {
		t.Errorf("expected experimental=true, got %v", combos[0]["experimental"])
	}
}
