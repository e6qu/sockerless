package core

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestOverlayContentTagIndependentOfUserCommand: the tag must not change
// when only the user's entrypoint, command, or working directory differ.
// Those are runtime environment on each deployment, so one overlay image
// (and one warm pool of it) serves every container type built on the
// same image.
func TestOverlayContentTagIndependentOfUserCommand(t *testing.T) {
	base := OverlayImageSpec{
		BaseImageRef:        "registry.gitlab.com/foo/runner-helper:v17",
		BootstrapBinaryPath: "/opt/sockerless/sockerless-gcf-bootstrap",
		BootstrapBinaryHash: "abc123",
	}
	want := OverlayContentTag("gcf-", base)
	for i, spec := range []OverlayImageSpec{
		base,
		{BaseImageRef: base.BaseImageRef, BootstrapBinaryPath: base.BootstrapBinaryPath, BootstrapBinaryHash: base.BootstrapBinaryHash,
			UserEntrypoint: []string{"/usr/bin/dumb-init", "/entrypoint"}, UserCmd: []string{"helper", "cache-init", "/cache"}},
		{BaseImageRef: base.BaseImageRef, BootstrapBinaryPath: base.BootstrapBinaryPath, BootstrapBinaryHash: base.BootstrapBinaryHash,
			UserCmd: []string{"helper", "set-permission", "/perm"}},
		{BaseImageRef: base.BaseImageRef, BootstrapBinaryPath: base.BootstrapBinaryPath, BootstrapBinaryHash: base.BootstrapBinaryHash,
			UserWorkdir: "/builds/repo"},
	} {
		if got := OverlayContentTag("gcf-", spec); got != want {
			t.Errorf("case %d: tag = %q, want %q (must be stable across entrypoint/cmd/workdir)", i, got, want)
		}
	}
	if !strings.HasPrefix(want, "gcf-") {
		t.Errorf("tag %q must carry the prefix", want)
	}
}

// TestOverlayContentTagDiffersOnImageBootstrapOrHash: the tag must change
// when any input that changes the image bytes changes, or a cached overlay
// would be served for a different image or a stale bootstrap.
func TestOverlayContentTagDiffersOnImageBootstrapOrHash(t *testing.T) {
	a := OverlayContentTag("p-", OverlayImageSpec{BaseImageRef: "image:a", BootstrapBinaryPath: "/bs", BootstrapBinaryHash: "h1"})
	b := OverlayContentTag("p-", OverlayImageSpec{BaseImageRef: "image:b", BootstrapBinaryPath: "/bs", BootstrapBinaryHash: "h1"})
	c := OverlayContentTag("p-", OverlayImageSpec{BaseImageRef: "image:a", BootstrapBinaryPath: "/other", BootstrapBinaryHash: "h1"})
	d := OverlayContentTag("p-", OverlayImageSpec{BaseImageRef: "image:a", BootstrapBinaryPath: "/bs", BootstrapBinaryHash: "h2"})
	if a == b || a == c || a == d {
		t.Errorf("tags must differ: image %q/%q bootstrap %q hash %q", a, b, c, d)
	}
}

func TestRenderOverlayDockerfile(t *testing.T) {
	df, err := RenderOverlayDockerfile(OverlayImageSpec{
		BaseImageRef:        "myacr.azurecr.io/app:v1",
		BootstrapBinaryPath: "/opt/sockerless/sockerless-aca-bootstrap",
		UserEntrypoint:      []string{"/should-not-bake"},
		UserCmd:             []string{"--also-no"},
		UserWorkdir:         "/no",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"FROM myacr.azurecr.io/app:v1",
		"COPY sockerless-aca-bootstrap /opt/sockerless/sockerless-aca-bootstrap",
		"RUN chmod +x /opt/sockerless/sockerless-aca-bootstrap",
		"ENTRYPOINT [\"/opt/sockerless/sockerless-aca-bootstrap\"]",
	} {
		if !strings.Contains(df, want) {
			t.Errorf("Dockerfile missing %q:\n%s", want, df)
		}
	}
	for _, banned := range []string{"SOCKERLESS_USER_ENTRYPOINT", "SOCKERLESS_USER_CMD", "SOCKERLESS_USER_WORKDIR"} {
		if strings.Contains(df, banned) {
			t.Errorf("Dockerfile must not bake runtime user env %s:\n%s", banned, df)
		}
	}
	if _, err := RenderOverlayDockerfile(OverlayImageSpec{BootstrapBinaryPath: "/bs"}); err == nil {
		t.Error("missing BaseImageRef must be rejected")
	}
	if _, err := RenderOverlayDockerfile(OverlayImageSpec{BaseImageRef: "img"}); err == nil {
		t.Error("missing BootstrapBinaryPath must be rejected")
	}
}

func TestOverlayUserEnvCarriesRuntimeCommand(t *testing.T) {
	env := OverlayUserEnv([]string{"/entry", "it's"}, []string{"serve", "--flag"}, "/work")
	if len(env) != 3 {
		t.Fatalf("env = %v, want three entries", env)
	}
	decode := func(entry, key string) []string {
		if !strings.HasPrefix(entry, key+"=") {
			t.Fatalf("entry %q does not start with %s=", entry, key)
		}
		raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(entry, key+"="))
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	if got := decode(env[0], "SOCKERLESS_USER_ENTRYPOINT"); len(got) != 2 || got[1] != "it's" {
		t.Errorf("entrypoint round-trip = %v", got)
	}
	if got := decode(env[1], "SOCKERLESS_USER_CMD"); len(got) != 2 || got[0] != "serve" {
		t.Errorf("cmd round-trip = %v", got)
	}
	if env[2] != "SOCKERLESS_USER_WORKDIR=/work" {
		t.Errorf("workdir = %q", env[2])
	}
	if got := OverlayUserEnv(nil, nil, ""); len(got) != 0 {
		t.Errorf("empty command must produce no env, got %v", got)
	}
}

func TestTarOverlayContextIncludesDockerfileAndBootstrap(t *testing.T) {
	bootstrap := filepath.Join(t.TempDir(), "sockerless-aca-bootstrap")
	if err := os.WriteFile(bootstrap, []byte("bootstrap-binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := TarOverlayContext(OverlayImageSpec{BaseImageRef: "myacr.azurecr.io/app:v1", BootstrapBinaryPath: bootstrap})
	if err != nil {
		t.Fatal(err)
	}
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	seen := map[string]string{}
	modes := map[string]int64{}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		var b bytes.Buffer
		if _, err := b.ReadFrom(tr); err != nil {
			t.Fatal(err)
		}
		seen[h.Name] = b.String()
		modes[h.Name] = h.Mode
	}
	if !strings.Contains(seen["Dockerfile"], "FROM myacr.azurecr.io/app:v1") {
		t.Fatalf("Dockerfile not present or wrong: %q", seen["Dockerfile"])
	}
	if got := seen["sockerless-aca-bootstrap"]; got != "bootstrap-binary" {
		t.Fatalf("bootstrap file = %q, want bootstrap-binary", got)
	}
	if modes["sockerless-aca-bootstrap"] != 0o755 {
		t.Fatalf("bootstrap mode = %o, want 755", modes["sockerless-aca-bootstrap"])
	}
}

func TestHashBootstrapBinary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bs")
	if err := os.WriteFile(path, []byte("v1"), 0o755); err != nil {
		t.Fatal(err)
	}
	h1, err := HashBootstrapBinary(path)
	if err != nil || len(h1) != 16 {
		t.Fatalf("hash = %q err=%v, want 16 hex chars", h1, err)
	}
	if err := os.WriteFile(path, []byte("v2"), 0o755); err != nil {
		t.Fatal(err)
	}
	if h2, _ := HashBootstrapBinary(path); h2 == h1 {
		t.Fatal("replacing the binary must change its hash")
	}
	if _, err := HashBootstrapBinary(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("a missing binary must error")
	}
}

func TestHasOverlayRepo(t *testing.T) {
	for image, want := range map[string]bool{
		"sockerless-overlay/aca:abc":                                   true,
		"myacr.azurecr.io/sockerless-overlay/aca:abc":                  true,
		"us-central1-docker.pkg.dev/p/sockerless-overlay/cloudrun:abc": true,
		"alpine:latest":                     false,
		"myacr.azurecr.io/library/alpine":   false,
		"registry/not-sockerless-overlay/x": false,
	} {
		if got := HasOverlayRepo(image); got != want {
			t.Errorf("HasOverlayRepo(%q) = %v, want %v", image, got, want)
		}
	}
}
