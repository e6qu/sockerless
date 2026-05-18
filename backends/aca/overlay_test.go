package aca

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"strings"
	"testing"
)

func TestACAOverlayContentTagIgnoresUserCommand(t *testing.T) {
	base := acaOverlaySpec{
		BaseImageRef:        "myacr.azurecr.io/app:v1",
		BootstrapBinaryPath: "/opt/sockerless/sockerless-aca-bootstrap",
		BootstrapBinaryHash: "abc123",
	}
	want := acaOverlayContentTag("aca-", base)

	for _, spec := range []acaOverlaySpec{
		base,
		{BaseImageRef: base.BaseImageRef, BootstrapBinaryPath: base.BootstrapBinaryPath, BootstrapBinaryHash: base.BootstrapBinaryHash},
	} {
		if got := acaOverlayContentTag("aca-", spec); got != want {
			t.Fatalf("overlay content tag = %q, want %q", got, want)
		}
	}
}

func TestRenderACAOverlayDockerfile(t *testing.T) {
	df, err := renderACAOverlayDockerfile(acaOverlaySpec{
		BaseImageRef:        "myacr.azurecr.io/app:v1",
		BootstrapBinaryPath: "/opt/sockerless/sockerless-aca-bootstrap",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"FROM myacr.azurecr.io/app:v1",
		"COPY sockerless-aca-bootstrap /opt/sockerless/sockerless-aca-bootstrap",
		"ENTRYPOINT [\"/opt/sockerless/sockerless-aca-bootstrap\"]",
	} {
		if !strings.Contains(df, want) {
			t.Fatalf("Dockerfile missing %q:\n%s", want, df)
		}
	}
	for _, banned := range []string{"SOCKERLESS_USER_ENTRYPOINT", "SOCKERLESS_USER_CMD", "SOCKERLESS_USER_WORKDIR"} {
		if strings.Contains(df, banned) {
			t.Fatalf("Dockerfile must not bake runtime user env %s:\n%s", banned, df)
		}
	}
}

func TestACAOverlayUserEnvCarriesRuntimeCommand(t *testing.T) {
	env := strings.Join(acaOverlayUserEnv([]string{"/entry"}, []string{"serve", "--flag"}, "/work"), "\n")
	for _, want := range []string{
		"SOCKERLESS_USER_ENTRYPOINT=",
		"SOCKERLESS_USER_CMD=",
		"SOCKERLESS_USER_WORKDIR=/work",
	} {
		if !strings.Contains(env, want) {
			t.Fatalf("runtime env missing %q in:\n%s", want, env)
		}
	}
}

func TestTarACAOverlayContextIncludesDockerfileAndBootstrap(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	bootstrap := t.TempDir() + "/sockerless-aca-bootstrap"
	if err := os.WriteFile(bootstrap, []byte("bootstrap-binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := tarACAOverlayContext(acaOverlaySpec{
		BaseImageRef:        "myacr.azurecr.io/app:v1",
		BootstrapBinaryPath: bootstrap,
	})
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
	}
	if !strings.Contains(seen["Dockerfile"], "FROM myacr.azurecr.io/app:v1") {
		t.Fatalf("Dockerfile not present or wrong: %q", seen["Dockerfile"])
	}
	if got := seen["sockerless-aca-bootstrap"]; got != "bootstrap-binary" {
		t.Fatalf("bootstrap file = %q, want bootstrap-binary", got)
	}
}
