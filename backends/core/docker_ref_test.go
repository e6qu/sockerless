package core

import "testing"

func TestSplitDockerRef(t *testing.T) {
	cases := []struct {
		in                         string
		wantReg, wantRepo, wantTag string
	}{
		{"alpine", "", "alpine", "latest"},
		{"alpine:latest", "", "alpine", "latest"},
		{"node:20", "", "node", "20"},
		{"myorg/app:v1", "", "myorg/app", "v1"},
		{"docker.io/library/alpine:3.18", "docker.io", "library/alpine", "3.18"},
		{"ghcr.io/owner/repo:v2", "ghcr.io", "owner/repo", "v2"},
		{"ghcr.io/owner/repo", "ghcr.io", "owner/repo", "latest"},
		{"registry.example.com:5000/team/app:sha-abc", "registry.example.com:5000", "team/app", "sha-abc"},
		{"localhost/x", "", "localhost/x", "latest"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			reg, repo, tag := SplitDockerRef(tc.in)
			if reg != tc.wantReg || repo != tc.wantRepo || tag != tc.wantTag {
				t.Fatalf("SplitDockerRef(%q) = (%q, %q, %q), want (%q, %q, %q)",
					tc.in, reg, repo, tag, tc.wantReg, tc.wantRepo, tc.wantTag)
			}
		})
	}
}

func TestArchFromPlatform(t *testing.T) {
	for in, want := range map[string]string{"linux/arm64": "arm64", "linux/amd64": "amd64", "arm64": "arm64", "linux/AARCH64 ": "arm64", "": "amd64", "linux/386": "amd64"} {
		if got := ArchFromPlatform(in); got != want {
			t.Errorf("ArchFromPlatform(%q) = %q, want %q", in, got, want)
		}
	}
}
