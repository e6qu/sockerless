package lambda

import (
	"archive/tar"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderOverlayDockerfile_RequiredFields(t *testing.T) {
	cases := []struct {
		name    string
		spec    OverlayImageSpec
		wantErr string
	}{
		{
			name:    "missing base",
			spec:    OverlayImageSpec{AgentBinaryPath: "a", BootstrapBinaryPath: "b"},
			wantErr: "BaseImageRef",
		},
		{
			name:    "missing agent",
			spec:    OverlayImageSpec{BaseImageRef: "alpine", BootstrapBinaryPath: "b"},
			wantErr: "AgentBinaryPath",
		},
		{
			name:    "missing bootstrap",
			spec:    OverlayImageSpec{BaseImageRef: "alpine", AgentBinaryPath: "a"},
			wantErr: "BootstrapBinaryPath",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := RenderOverlayDockerfile(tc.spec); err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestRenderOverlayDockerfile_BasicStructure(t *testing.T) {
	df, err := RenderOverlayDockerfile(OverlayImageSpec{
		BaseImageRef:        "123.dkr.ecr.us-east-1.amazonaws.com/docker-hub/library/alpine:latest",
		AgentBinaryPath:     "bin/sockerless-agent",
		BootstrapBinaryPath: "bin/sockerless-lambda-bootstrap",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{
		"FROM 123.dkr.ecr.us-east-1.amazonaws.com/docker-hub/library/alpine:latest",
		"COPY bin/sockerless-agent /opt/sockerless/sockerless-agent",
		"COPY bin/sockerless-lambda-bootstrap /opt/sockerless/sockerless-lambda-bootstrap",
		`ENTRYPOINT ["/opt/sockerless/sockerless-lambda-bootstrap"]`,
	} {
		if !strings.Contains(df, want) {
			t.Errorf("rendered Dockerfile missing %q\n---\n%s", want, df)
		}
	}
	// No user env vars when entrypoint/cmd are empty.
	if strings.Contains(df, "SOCKERLESS_USER_ENTRYPOINT") {
		t.Errorf("did not expect user entrypoint env var, got:\n%s", df)
	}
}

func TestRenderOverlayDockerfile_UserEntrypointAndCmd(t *testing.T) {
	df, err := RenderOverlayDockerfile(OverlayImageSpec{
		BaseImageRef:        "alpine:latest",
		AgentBinaryPath:     "a",
		BootstrapBinaryPath: "b",
		UserEntrypoint:      []string{"/bin/sh", "-c"},
		UserCmd:             []string{"echo", "hello"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(df, "ENV SOCKERLESS_USER_ENTRYPOINT=/bin/sh:-c") {
		t.Errorf("entrypoint env var missing or malformed:\n%s", df)
	}
	if !strings.Contains(df, "ENV SOCKERLESS_USER_CMD=echo:hello") {
		t.Errorf("cmd env var missing or malformed:\n%s", df)
	}
}

// TestTarOverlayContext_HasDockerfileAndBinaries verifies the Phase-86
// D.2 build-context tarball contains the rendered Dockerfile plus both
// staged binaries at the paths the Dockerfile COPYs from.
func TestTarOverlayContext_HasDockerfileAndBinaries(t *testing.T) {
	dir := t.TempDir()
	agentPath := filepath.Join(dir, "bin", "sockerless-agent")
	bootPath := filepath.Join(dir, "bin", "sockerless-lambda-bootstrap")
	if err := os.MkdirAll(filepath.Dir(agentPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(agentPath, []byte("agent-binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bootPath, []byte("bootstrap-binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	data, err := TarOverlayContext(OverlayImageSpec{
		BaseImageRef:        "alpine:latest",
		AgentBinaryPath:     agentPath,
		BootstrapBinaryPath: bootPath,
	})
	if err != nil {
		t.Fatalf("TarOverlayContext: %v", err)
	}

	tr := tar.NewReader(bytes.NewReader(data))
	seen := map[string]string{}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar read: %v", err)
		}
		buf, _ := io.ReadAll(tr)
		seen[hdr.Name] = string(buf)
	}

	if df, ok := seen["Dockerfile"]; !ok {
		t.Fatal("Dockerfile missing from tar")
	} else if !strings.Contains(df, "FROM alpine:latest") {
		t.Errorf("Dockerfile content wrong: %s", df)
	}
	if _, ok := seen[agentPath]; !ok {
		t.Errorf("agent binary missing at %q (entries: %v)", agentPath, keys(seen))
	}
	if _, ok := seen[bootPath]; !ok {
		t.Errorf("bootstrap binary missing at %q (entries: %v)", bootPath, keys(seen))
	}
}

// TestBuildAndPushOverlayImage_MissingDest verifies the input
// validation before any external tool runs.
func TestBuildAndPushOverlayImage_MissingDest(t *testing.T) {
	_, err := BuildAndPushOverlayImage(context.TODO(), OverlayImageSpec{BaseImageRef: "alpine"}, "")
	if err == nil || !strings.Contains(err.Error(), "destRef is required") {
		t.Errorf("want destRef-required error, got %v", err)
	}
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
