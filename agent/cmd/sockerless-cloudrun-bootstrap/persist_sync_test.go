package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseSyncVolumes(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		want    []syncVolume
		wantErr bool
	}{
		{
			name:  "empty",
			input: "",
			want:  nil,
		},
		{
			name:  "single",
			input: "v1=/mnt=gs://b1/o1",
			want: []syncVolume{
				{Name: "v1", MountPath: "/mnt", Bucket: "b1", Object: "o1"},
			},
		},
		{
			name:  "two volumes",
			input: "runner-workspace=/tmp/runner-work=gs://buck/workspace/runner-workspace/exec1.tar.gz,runner-externals=/opt/externals=gs://buck/workspace/runner-externals/exec1.tar.gz",
			want: []syncVolume{
				{Name: "runner-workspace", MountPath: "/tmp/runner-work", Bucket: "buck", Object: "workspace/runner-workspace/exec1.tar.gz"},
				{Name: "runner-externals", MountPath: "/opt/externals", Bucket: "buck", Object: "workspace/runner-externals/exec1.tar.gz"},
			},
		},
		{
			name:    "missing bucket prefix",
			input:   "v1=/mnt=b1/o1",
			wantErr: true,
		},
		{
			name:    "missing object",
			input:   "v1=/mnt=gs://b1/",
			wantErr: true,
		},
		{
			name:    "missing path",
			input:   "v1==gs://b1/o1",
			wantErr: true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseSyncVolumes(c.input)
			if c.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil. result=%v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("got %+v, want %+v", got, c.want)
			}
		})
	}
}

func TestTarGzRoundtrip(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("alpha"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(src, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "sub", "b.txt"), []byte("beta"), 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := tarGzFrom(&buf, src); err != nil {
		t.Fatalf("tarGzFrom: %v", err)
	}
	if buf.Len() == 0 {
		t.Fatal("tar.gz output was empty")
	}

	dst := t.TempDir()
	if err := untarGzInto(buf.Bytes(), dst); err != nil {
		t.Fatalf("untarGzInto: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dst, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "alpha" {
		t.Errorf("a.txt = %q, want alpha", got)
	}
	got, err = os.ReadFile(filepath.Join(dst, "sub", "b.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "beta" {
		t.Errorf("sub/b.txt = %q, want beta", got)
	}
}

func TestUntarGzInto_EmptyData(t *testing.T) {
	dst := t.TempDir()
	if err := untarGzInto(nil, dst); err != nil {
		t.Errorf("empty data should be no-op, got %v", err)
	}
}

func TestExtractSyncVolumesEnv(t *testing.T) {
	cases := []struct {
		name string
		env  []string
		want string
	}{
		{name: "absent", env: []string{"FOO=bar", "BAR=baz"}, want: ""},
		{name: "present", env: []string{"FOO=bar", "SOCKERLESS_SYNC_VOLUMES=v1=/mnt=gs://b/o", "BAR=baz"}, want: "v1=/mnt=gs://b/o"},
		{name: "empty value", env: []string{"SOCKERLESS_SYNC_VOLUMES="}, want: ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := extractSyncVolumesEnv(c.env)
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}
