package lambda

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	core "github.com/sockerless/backend-core"
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
	// COPY sources are stable, context-relative names regardless of
	// the host-side AgentBinaryPath / BootstrapBinaryPath values —
	// TarOverlayContext stages them under those names.
	for _, want := range []string{
		"FROM 123.dkr.ecr.us-east-1.amazonaws.com/docker-hub/library/alpine:latest",
		"COPY sockerless-agent /opt/sockerless/sockerless-agent",
		"COPY sockerless-lambda-bootstrap /opt/sockerless/sockerless-lambda-bootstrap",
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
	// base64("[\"/bin/sh\",\"-c\"]") / base64("[\"echo\",\"hello\"]")
	wantEP := base64.StdEncoding.EncodeToString([]byte(`["/bin/sh","-c"]`))
	wantCmd := base64.StdEncoding.EncodeToString([]byte(`["echo","hello"]`))
	if !strings.Contains(df, "ENV SOCKERLESS_USER_ENTRYPOINT="+wantEP+"\n") {
		t.Errorf("entrypoint env var missing or malformed (want %s):\n%s", wantEP, df)
	}
	if !strings.Contains(df, "ENV SOCKERLESS_USER_CMD="+wantCmd+"\n") {
		t.Errorf("cmd env var missing or malformed (want %s):\n%s", wantCmd, df)
	}
}

// TestTarOverlayContext_HasDockerfileAndBinaries verifies the
// build-context tarball contains the rendered Dockerfile plus both
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

	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
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
	// Tar entries use the stable context names regardless of where the
	// host binaries live.
	if got, ok := seen["sockerless-agent"]; !ok {
		t.Errorf("agent binary missing at sockerless-agent (entries: %v)", keys(seen))
	} else if got != "agent-binary" {
		t.Errorf("agent binary content unexpected: %q", got)
	}
	if got, ok := seen["sockerless-lambda-bootstrap"]; !ok {
		t.Errorf("bootstrap binary missing at sockerless-lambda-bootstrap (entries: %v)", keys(seen))
	} else if got != "bootstrap-binary" {
		t.Errorf("bootstrap binary content unexpected: %q", got)
	}
}

// TestBuildAndPushOverlayImage_MissingDest verifies the input
// validation before any external tool runs.
func TestBuildAndPushOverlayImage_MissingDest(t *testing.T) {
	_, err := BuildAndPushOverlayImage(context.TODO(), OverlayImageSpec{BaseImageRef: "alpine"}, "", nil)
	if err == nil || !strings.Contains(err.Error(), "destRef is required") {
		t.Errorf("want destRef-required error, got %v", err)
	}
}

func TestOverlayContentTag(t *testing.T) {
	spec := OverlayImageSpec{
		BaseImageRef:        "alpine:latest",
		AgentBinaryPath:     "agent",
		BootstrapBinaryPath: "bootstrap",
		UserEntrypoint:      []string{"tail"},
		UserCmd:             []string{"-f", "/dev/null"},
	}
	first := OverlayContentTag(spec)
	second := OverlayContentTag(spec)
	if first != second {
		t.Fatalf("unstable tag: %q != %q", first, second)
	}
	if !strings.HasPrefix(first, "overlay-") {
		t.Errorf("tag missing overlay- prefix: %q", first)
	}

	// Mutating any input changes the tag.
	spec2 := spec
	spec2.UserCmd = []string{"date"}
	if got := OverlayContentTag(spec2); got == first {
		t.Errorf("tag did not change after UserCmd mutation: %q", got)
	}
	spec3 := spec
	spec3.BaseImageRef = "alpine:3.20"
	if got := OverlayContentTag(spec3); got == first {
		t.Errorf("tag did not change after BaseImageRef mutation: %q", got)
	}
}

// stubBuilder is a CloudBuildService test double that records the
// Build call and returns a canned result. Used to verify
// BuildAndPushOverlayImage routes through the cloud path when a
// builder is supplied.
type stubBuilder struct {
	available bool
	result    *core.CloudBuildResult
	err       error
	called    bool
	gotOpts   core.CloudBuildOptions
}

func (b *stubBuilder) Available() bool { return b.available }
func (b *stubBuilder) AssembleMultiArchManifest(_ context.Context, _ core.MultiArchManifestOptions) error {
	return nil
}
func (b *stubBuilder) Build(_ context.Context, opts core.CloudBuildOptions) (*core.CloudBuildResult, error) {
	b.called = true
	b.gotOpts = opts
	if b.err != nil {
		return nil, b.err
	}
	return b.result, nil
}

func TestBuildAndPushOverlayImage_RoutesViaCloudBuild(t *testing.T) {
	dir := t.TempDir()
	agentPath := filepath.Join(dir, "agent")
	bootPath := filepath.Join(dir, "bootstrap")
	if err := os.WriteFile(agentPath, []byte("agent-bytes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bootPath, []byte("boot-bytes"), 0o755); err != nil {
		t.Fatal(err)
	}
	spec := OverlayImageSpec{
		BaseImageRef:        "alpine:latest",
		AgentBinaryPath:     agentPath,
		BootstrapBinaryPath: bootPath,
		UserEntrypoint:      []string{"tail"},
	}
	dest := "729079515331.dkr.ecr.eu-west-1.amazonaws.com/sockerless-live-lambda:overlay-deadbeef"
	stub := &stubBuilder{
		available: true,
		result:    &core.CloudBuildResult{ImageRef: dest},
	}
	got, err := BuildAndPushOverlayImage(context.TODO(), spec, dest, stub)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !stub.called {
		t.Fatal("CloudBuildService.Build was not invoked")
	}
	if got.ImageURI != dest {
		t.Errorf("ImageURI = %q, want %q", got.ImageURI, dest)
	}
	if len(stub.gotOpts.Tags) != 1 || stub.gotOpts.Tags[0] != dest {
		t.Errorf("Tags = %v, want [%q]", stub.gotOpts.Tags, dest)
	}
	if stub.gotOpts.Platform != "linux/amd64" {
		t.Errorf("Platform = %q, want linux/amd64", stub.gotOpts.Platform)
	}
	if stub.gotOpts.Dockerfile != "Dockerfile" {
		t.Errorf("Dockerfile = %q, want Dockerfile", stub.gotOpts.Dockerfile)
	}
	// Context tar should be non-empty (Dockerfile + 2 binaries).
	if stub.gotOpts.ContextTar == nil {
		t.Fatal("ContextTar is nil")
	}
	buf, _ := io.ReadAll(stub.gotOpts.ContextTar)
	if len(buf) < 100 {
		t.Errorf("ContextTar too small: %d bytes", len(buf))
	}
}

func TestBuildAndPushOverlayImage_FallsBackToLocalDockerWhenBuilderUnavailable(t *testing.T) {
	stub := &stubBuilder{available: false}
	// The local-docker fallback will fail (no `docker` binary in test
	// env on most CI), so we don't assert on success — just that
	// stub.Build wasn't called when Available() returned false.
	dir := t.TempDir()
	agentPath := filepath.Join(dir, "agent")
	bootPath := filepath.Join(dir, "bootstrap")
	_ = os.WriteFile(agentPath, []byte("a"), 0o755)
	_ = os.WriteFile(bootPath, []byte("b"), 0o755)
	spec := OverlayImageSpec{
		BaseImageRef:        "alpine:latest",
		AgentBinaryPath:     agentPath,
		BootstrapBinaryPath: bootPath,
	}
	_, _ = BuildAndPushOverlayImage(context.TODO(), spec, "x:y", stub)
	if stub.called {
		t.Fatal("CloudBuildService.Build called even though Available()=false")
	}
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
