package awscommon

import "testing"

func TestExtractAccountID(t *testing.T) {
	cases := []struct {
		arn, want string
	}{
		{"arn:aws:iam::123456789012:role/sockerless-live-execution-role", "123456789012"},
		{"arn:aws:ecs:eu-west-1:729079515331:cluster/sockerless-live", "729079515331"},
		{"not-an-arn", ""},
	}
	for _, tc := range cases {
		if got := ExtractAccountID(tc.arn); got != tc.want {
			t.Errorf("ExtractAccountID(%q) = %q, want %q", tc.arn, got, tc.want)
		}
	}
}

// TestUpstreamRegistryFor: the public registries sockerless routes through
// a pull-through cache map onto ECR's upstream enumeration; Docker Hub is
// not among them because its library images go to the AWS Public Gallery.
func TestUpstreamRegistryFor(t *testing.T) {
	cases := map[string]string{
		"ghcr.io":           "github-container-registry",
		"quay.io":           "quay",
		"registry.k8s.io":   "k8s",
		"k8s.gcr.io":        "k8s",
		"mcr.microsoft.com": "azure-container-registry",
		"public.ecr.aws":    "ecr-public",
	}
	for host, want := range cases {
		if got := string(UpstreamRegistryFor(host)); got != want {
			t.Errorf("UpstreamRegistryFor(%q) = %q, want %q", host, got, want)
		}
	}
}

func TestPullThroughCacheNaming(t *testing.T) {
	if got := PullThroughCachePrefix("registry.k8s.io"); got != "registry-k8s-io" {
		t.Errorf("PullThroughCachePrefix = %q", got)
	}
	got := PullThroughCacheURI("123456789012", "eu-west-1", "ghcr-io", "owner/repo", "v2")
	if want := "123456789012.dkr.ecr.eu-west-1.amazonaws.com/ghcr-io/owner/repo:v2"; got != want {
		t.Errorf("PullThroughCacheURI = %q, want %q", got, want)
	}
	if !IsECRImageURI(got) || IsECRImageURI("public.ecr.aws/docker/library/alpine") || IsECRImageURI("alpine") {
		t.Error("IsECRImageURI")
	}
}
