package lambda

import "testing"

func TestParseDockerRef(t *testing.T) {
	tests := []struct {
		input                           string
		wantRegistry, wantRepo, wantTag string
	}{
		{"alpine", "", "alpine", "latest"},
		{"alpine:3.19", "", "alpine", "3.19"},
		{"nginx:alpine", "", "nginx", "alpine"},
		{"myorg/myapp:v2", "", "myorg/myapp", "v2"},
		{"docker.io/library/alpine:latest", "docker.io", "library/alpine", "latest"},
		{"ghcr.io/owner/repo:sha-abc", "ghcr.io", "owner/repo", "sha-abc"},
		{"123456.dkr.ecr.eu-west-1.amazonaws.com/repo:tag", "123456.dkr.ecr.eu-west-1.amazonaws.com", "repo", "tag"},
	}
	for _, tt := range tests {
		registry, repo, tag := parseDockerRef(tt.input)
		if registry != tt.wantRegistry || repo != tt.wantRepo || tag != tt.wantTag {
			t.Errorf("parseDockerRef(%q) = (%q, %q, %q), want (%q, %q, %q)",
				tt.input, registry, repo, tag, tt.wantRegistry, tt.wantRepo, tt.wantTag)
		}
	}
}

func TestExtractAccountID(t *testing.T) {
	tests := []struct {
		arn  string
		want string
	}{
		{"arn:aws:iam::123456789012:role/name", "123456789012"},
		{"arn:aws:iam::987654321098:role/other", "987654321098"},
		{"invalid", ""},
		{"", ""},
	}
	for _, tt := range tests {
		got := extractAccountID(tt.arn)
		if got != tt.want {
			t.Errorf("extractAccountID(%q) = %q, want %q", tt.arn, got, tt.want)
		}
	}
}
