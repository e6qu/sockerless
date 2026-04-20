package ecs

import "testing"

func TestParseDockerRef(t *testing.T) {
	cases := []struct {
		in                         string
		wantReg, wantRepo, wantTag string
	}{
		{"alpine", "", "alpine", "latest"},
		{"alpine:latest", "", "alpine", "latest"},
		{"node:20", "", "node", "20"},
		{"myorg/app:v1", "", "myorg/app", "v1"},
		{"ghcr.io/owner/repo:v2", "ghcr.io", "owner/repo", "v2"},
		{"registry.example.com:5000/team/app:sha-abc", "registry.example.com:5000", "team/app", "sha-abc"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			reg, repo, tag := parseDockerRef(tc.in)
			if reg != tc.wantReg || repo != tc.wantRepo || tag != tc.wantTag {
				t.Fatalf("parseDockerRef(%q) = (%q, %q, %q), want (%q, %q, %q)",
					tc.in, reg, repo, tag, tc.wantReg, tc.wantRepo, tc.wantTag)
			}
		})
	}
}

func TestExtractAccountID(t *testing.T) {
	cases := []struct {
		arn, want string
	}{
		{"arn:aws:iam::123456789012:role/sockerless-live-execution-role", "123456789012"},
		{"arn:aws:ecs:eu-west-1:729079515331:cluster/sockerless-live", "729079515331"},
		{"not-an-arn", ""},
	}
	for _, tc := range cases {
		if got := extractAccountID(tc.arn); got != tc.want {
			t.Errorf("extractAccountID(%q) = %q, want %q", tc.arn, got, tc.want)
		}
	}
}
