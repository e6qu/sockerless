package core

import "testing"

func TestParseImageRef(t *testing.T) {
	cases := []struct {
		in   string
		want ImageRef
	}{
		{"alpine", ImageRef{Path: "alpine"}},
		{"alpine:latest", ImageRef{Path: "alpine", Tag: "latest"}},
		{"library/alpine", ImageRef{Path: "library/alpine"}},
		{"library/alpine:3.20", ImageRef{Path: "library/alpine", Tag: "3.20"}},
		{"docker.io/library/alpine:latest", ImageRef{Domain: "docker.io", Path: "library/alpine", Tag: "latest"}},
		{"public.ecr.aws/docker/library/alpine:3.20", ImageRef{Domain: "public.ecr.aws", Path: "docker/library/alpine", Tag: "3.20"}},
		{"123456789.dkr.ecr.eu-west-1.amazonaws.com/skls/alpine:test",
			ImageRef{Domain: "123456789.dkr.ecr.eu-west-1.amazonaws.com", Path: "skls/alpine", Tag: "test"}},
		{"localhost:5000/myimage:dev",
			ImageRef{Domain: "localhost:5000", Path: "myimage", Tag: "dev"}},
		{"alpine@sha256:abc",
			ImageRef{Path: "alpine", Digest: "sha256:abc"}},
		{"alpine:3.20@sha256:abc",
			ImageRef{Path: "alpine", Tag: "3.20", Digest: "sha256:abc"}},
		{"ghcr.io/owner/repo:v1.2.3@sha256:def",
			ImageRef{Domain: "ghcr.io", Path: "owner/repo", Tag: "v1.2.3", Digest: "sha256:def"}},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got, err := ParseImageRef(c.in)
			if err != nil {
				t.Fatalf("ParseImageRef(%q): unexpected error %v", c.in, err)
			}
			if got != c.want {
				t.Errorf("ParseImageRef(%q):\n got  %+v\n want %+v", c.in, got, c.want)
			}
			if round := got.String(); round != c.in {
				t.Errorf("round-trip String(): got %q, want %q", round, c.in)
			}
		})
	}
}

func TestParseImageRef_Errors(t *testing.T) {
	bad := []string{
		"",
		"alpine:",
		"alpine@",
		":tag",
	}
	for _, b := range bad {
		t.Run(b, func(t *testing.T) {
			if _, err := ParseImageRef(b); err == nil {
				t.Errorf("ParseImageRef(%q): expected error, got nil", b)
			}
		})
	}
}

func TestImageRef_NameTag_FullName(t *testing.T) {
	r := ImageRef{Domain: "ghcr.io", Path: "owner/repo", Tag: "v1.2.3"}
	if name, tag := r.NameTag(); name != "owner/repo" || tag != "v1.2.3" {
		t.Errorf("NameTag: got %q,%q", name, tag)
	}
	if r.FullName() != "ghcr.io/owner/repo" {
		t.Errorf("FullName: got %q", r.FullName())
	}

	bare := ImageRef{Path: "alpine"}
	if bare.FullName() != "alpine" {
		t.Errorf("FullName (no domain): got %q", bare.FullName())
	}
}
