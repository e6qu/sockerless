package core

import (
	"io"
	"net/http"
	"testing"

	"github.com/rs/zerolog"
	"github.com/sockerless/api"
)

func TestSplitImageReference(t *testing.T) {
	cases := []struct{ in, repository, tag, digest string }{
		{"alpine", "alpine", "", ""},
		{"alpine:3.20", "alpine", "3.20", ""},
		{"alpine@sha256:abc", "alpine", "", "sha256:abc"},
		{"alpine:3.20@sha256:abc", "alpine", "3.20", "sha256:abc"},
		{"localhost:5000/foo", "localhost:5000/foo", "", ""},
		{"localhost:5000/foo:dev", "localhost:5000/foo", "dev", ""},
		{"localhost:5000/foo@sha256:abc", "localhost:5000/foo", "", "sha256:abc"},
		{"public.ecr.aws/docker/library/nginx:alpine@sha256:abc", "public.ecr.aws/docker/library/nginx", "alpine", "sha256:abc"},
	}
	for _, c := range cases {
		repository, tag, digest := SplitImageReference(c.in)
		if repository != c.repository || tag != c.tag || digest != c.digest {
			t.Errorf("SplitImageReference(%q) = (%q, %q, %q), want (%q, %q, %q)",
				c.in, repository, tag, digest, c.repository, c.tag, c.digest)
		}
	}
}

func newImageTestServer() *BaseServer {
	s := &BaseServer{
		Store:          NewStore(),
		Logger:         zerolog.Nop(),
		Mux:            http.NewServeMux(),
		EventBus:       NewEventBus(),
		PendingCreates: NewStateStore[api.Container](),
	}
	s.InitDrivers()
	s.self = s
	return s
}

func pullForTest(t *testing.T, s *BaseServer, ref, manifestDigest string) {
	t.Helper()
	rc, err := s.ImagePullWithMetadata(ref, "", &ImageMetadataResult{
		Config:         &api.ContainerConfig{Cmd: []string{"/bin/sh"}},
		ConfigDigest:   "sha256:config" + manifestDigest[len("sha256:"):],
		ManifestDigest: manifestDigest,
		Architecture:   "amd64",
		OS:             "linux",
	})
	if err != nil {
		t.Fatalf("pull %s: %v", ref, err)
	}
	if _, err := io.Copy(io.Discard, rc); err != nil {
		t.Fatalf("read pull %s: %v", ref, err)
	}
	if err := rc.Close(); err != nil {
		t.Fatalf("close pull %s: %v", ref, err)
	}
}

// Docker records an image pulled by digest under its repository digest and
// no tag, and resolves any `name[:tag]@digest` to it.
func TestImagePullByDigest(t *testing.T) {
	s := newImageTestServer()
	const repo = "public.ecr.aws/docker/library/alpine"
	const digest = "sha256:1111"
	pullForTest(t, s, repo+"@"+digest, digest)

	for _, ref := range []string{repo + "@" + digest, repo + ":3.20@" + digest} {
		img, ok := s.Store.ResolveImage(ref)
		if !ok {
			t.Fatalf("ResolveImage(%q) found nothing", ref)
		}
		if len(img.RepoTags) != 0 {
			t.Errorf("RepoTags = %v, want none for a digest pull", img.RepoTags)
		}
		if len(img.RepoDigests) != 1 || img.RepoDigests[0] != repo+"@"+digest {
			t.Errorf("RepoDigests = %v, want [%s@%s]", img.RepoDigests, repo, digest)
		}
		if name, ok := ImageReference(img); !ok || name != repo+"@"+digest {
			t.Errorf("ImageReference = %q, %v", name, ok)
		}
	}
	if _, ok := s.Store.ResolveImage(repo + "@sha256:2222"); ok {
		t.Error("a different digest resolved to the pulled image")
	}
	if _, ok := s.Store.ResolveImage(repo + ":3.20"); ok {
		t.Error("a tag the digest pull never recorded resolved to the image")
	}

	img, _ := s.Store.ResolveImage(repo + "@" + digest)
	if _, err := s.ImageRemove(repo+"@"+digest, false, false); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, ok := s.Store.ResolveImage(repo + "@" + digest); ok {
		t.Error("the image still resolves by digest after removal")
	}
	if _, ok := s.Store.Images.Get(img.ID); ok {
		t.Error("the image still resolves by ID after removal")
	}
}

// A registry port is not a tag: `localhost:5000/foo` pulls latest, and its
// repository digest keeps the port.
func TestImagePullRegistryPortDefaultsLatest(t *testing.T) {
	s := newImageTestServer()
	pullForTest(t, s, "localhost:5000/foo", "sha256:3333")

	img, ok := s.Store.ResolveImage("localhost:5000/foo:latest")
	if !ok {
		t.Fatal("localhost:5000/foo:latest does not resolve")
	}
	if len(img.RepoTags) != 1 || img.RepoTags[0] != "localhost:5000/foo:latest" {
		t.Errorf("RepoTags = %v", img.RepoTags)
	}
	if len(img.RepoDigests) != 1 || img.RepoDigests[0] != "localhost:5000/foo@sha256:3333" {
		t.Errorf("RepoDigests = %v", img.RepoDigests)
	}
	if _, ok := s.Store.ResolveImage("localhost:5000/foo"); !ok {
		t.Error("localhost:5000/foo does not resolve to latest")
	}
}
