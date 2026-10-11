package awscommon

import (
	"testing"

	"github.com/rs/zerolog"
)

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
	host := ECRRegistryHost("123456789012", "eu-west-1", "")
	got := PullThroughCacheURI(host, "ghcr-io", "owner/repo", ":v2")
	if want := "123456789012.dkr.ecr.eu-west-1.amazonaws.com/ghcr-io/owner/repo:v2"; got != want {
		t.Errorf("PullThroughCacheURI = %q, want %q", got, want)
	}
	pinned := PullThroughCacheURI(host, "ghcr-io", "owner/repo", "@sha256:abc")
	if want := "123456789012.dkr.ecr.eu-west-1.amazonaws.com/ghcr-io/owner/repo@sha256:abc"; pinned != want {
		t.Errorf("PullThroughCacheURI = %q, want %q", pinned, want)
	}
	if !IsECRImageURI(got, "") || IsECRImageURI("public.ecr.aws/docker/library/alpine", "") || IsECRImageURI("alpine", "") {
		t.Error("IsECRImageURI")
	}
}

// TestECRRegistryCoordinate: the coordinate relocates the registry host every
// reference carries, is recognised as Amazon ECR, and is dialed at its own
// URL; a reference written against the real host is moved onto it, and
// nothing changes when no coordinate is set.
func TestECRRegistryCoordinate(t *testing.T) {
	const endpoint = "http://127.0.0.1:4566/"
	host := ECRRegistryHost("123456789012", "us-east-1", endpoint)
	if host != "127.0.0.1:4566" {
		t.Fatalf("ECRRegistryHost = %q", host)
	}
	ref := PullThroughCacheURI(host, "public-ecr-aws", "docker/library/alpine", ":latest")
	if ref != "127.0.0.1:4566/public-ecr-aws/docker/library/alpine:latest" {
		t.Errorf("PullThroughCacheURI at the coordinate = %q", ref)
	}
	if !IsECRImageURI(ref, endpoint) {
		t.Errorf("a reference at the coordinate is an Amazon ECR reference")
	}
	if IsECRImageURI(ref, "") {
		t.Errorf("a loopback reference is not Amazon ECR when no coordinate is set")
	}
	real := "123456789012.dkr.ecr.us-east-1.amazonaws.com/sockerless-simulator-lambda"
	if got := RelocateECRReference(real, endpoint); got != "127.0.0.1:4566/sockerless-simulator-lambda" {
		t.Errorf("RelocateECRReference = %q", got)
	}
	if got := RelocateECRReference(real, ""); got != real {
		t.Errorf("RelocateECRReference without a coordinate = %q", got)
	}
	if got := RelocateECRReference("public.ecr.aws/docker/library/alpine", endpoint); got != "public.ecr.aws/docker/library/alpine" {
		t.Errorf("RelocateECRReference moved a non-ECR reference: %q", got)
	}

	auth := NewECRAuthProvider(nil, endpoint, zerolog.Nop(), nil)
	if !auth.IsCloudRegistry("127.0.0.1:4566") || !auth.IsCloudRegistry("123456789012.dkr.ecr.us-east-1.amazonaws.com") || auth.IsCloudRegistry("ghcr.io") {
		t.Error("IsCloudRegistry")
	}
	if got := auth.RegistryEndpoint("127.0.0.1:4566"); got != "http://127.0.0.1:4566" {
		t.Errorf("RegistryEndpoint = %q", got)
	}
	if got := NewECRAuthProvider(nil, "", zerolog.Nop(), nil).RegistryEndpoint("123456789012.dkr.ecr.us-east-1.amazonaws.com"); got != "" {
		t.Errorf("RegistryEndpoint without a coordinate = %q", got)
	}
}
