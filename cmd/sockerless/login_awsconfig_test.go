package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpsertINISectionCreatesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "aws", "config")
	err := upsertINISection(path, "profile sockerless-dev", []iniKV{
		{"role_arn", "arn:aws:iam::123456789012:role/cli"},
		{"region", "us-east-1"},
	})
	if err != nil {
		t.Fatalf("upsertINISection: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "[profile sockerless-dev]\nrole_arn = arn:aws:iam::123456789012:role/cli\nregion = us-east-1\n"
	if string(content) != want {
		t.Fatalf("content = %q, want %q", content, want)
	}
}

func TestUpsertINISectionPreservesOtherSections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	original := `# my aws settings
[default]
region = eu-west-1
output = json

[profile work]
; personal note about work
role_arn = arn:aws:iam::999999999999:role/work
source_profile = default
`
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := upsertINISection(path, "profile sockerless-dev", []iniKV{{"region", "us-east-1"}}); err != nil {
		t.Fatalf("upsertINISection: %v", err)
	}
	content, _ := os.ReadFile(path)
	want := original + "\n[profile sockerless-dev]\nregion = us-east-1\n"
	if string(content) != want {
		t.Fatalf("append changed other content:\n got: %q\nwant: %q", content, want)
	}

	// Updating our section must leave every other byte untouched.
	if err := upsertINISection(path, "profile sockerless-dev", []iniKV{{"region", "eu-central-1"}, {"endpoint_url", "http://127.0.0.1:29310"}}); err != nil {
		t.Fatalf("upsertINISection update: %v", err)
	}
	content, _ = os.ReadFile(path)
	want = original + "\n[profile sockerless-dev]\nregion = eu-central-1\nendpoint_url = http://127.0.0.1:29310\n"
	if string(content) != want {
		t.Fatalf("update changed other content:\n got: %q\nwant: %q", content, want)
	}
}

func TestUpsertINISectionUpdatesMiddleSection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	original := "[profile sockerless-dev]\nregion = us-east-1\n\n[profile work]\nregion = eu-west-1\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := upsertINISection(path, "profile sockerless-dev", []iniKV{{"region", "us-west-2"}}); err != nil {
		t.Fatalf("upsertINISection: %v", err)
	}
	content, _ := os.ReadFile(path)
	want := "[profile sockerless-dev]\nregion = us-west-2\n\n[profile work]\nregion = eu-west-1\n"
	if string(content) != want {
		t.Fatalf("content = %q, want %q", content, want)
	}
}

func TestRemoveINISection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	original := "[default]\nregion = eu-west-1\n\n[profile sockerless-dev]\nregion = us-east-1\n\n[profile work]\nregion = eu-west-1\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	found, err := removeINISection(path, "profile sockerless-dev")
	if err != nil || !found {
		t.Fatalf("removeINISection = (%v, %v), want (true, nil)", found, err)
	}
	content, _ := os.ReadFile(path)
	if strings.Contains(string(content), "sockerless-dev") {
		t.Fatalf("section still present: %q", content)
	}
	if !strings.Contains(string(content), "[default]\nregion = eu-west-1\n") || !strings.Contains(string(content), "[profile work]\nregion = eu-west-1\n") {
		t.Fatalf("other sections damaged: %q", content)
	}

	found, err = removeINISection(path, "profile sockerless-dev")
	if err != nil || found {
		t.Fatalf("second removeINISection = (%v, %v), want (false, nil)", found, err)
	}
	found, err = removeINISection(filepath.Join(t.TempDir(), "missing"), "profile x")
	if err != nil || found {
		t.Fatalf("removeINISection on missing file = (%v, %v), want (false, nil)", found, err)
	}
}

func TestUpsertINISectionPreservesFileMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(path, []byte("[default]\nregion = eu-west-1\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := upsertINISection(path, "profile p", []iniKV{{"region", "us-east-1"}}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("mode = %o, want 640", info.Mode().Perm())
	}
}
