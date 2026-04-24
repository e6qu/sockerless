package lambda

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// OverlayImageSpec describes the per-container overlay image that the
// Lambda backend builds on top of the user's requested image so the
// function can run the sockerless-agent + sockerless-lambda-bootstrap
// alongside the user's entrypoint. See docs/LAMBDA_EXEC_DESIGN.md.
//
// The Dockerfile produced by RenderOverlayDockerfile is built and
// pushed to ECR by the Lambda backend's existing image pipeline (see
// image_resolve.go), with the resulting URI used as the Lambda
// function's image URI.
type OverlayImageSpec struct {
	// BaseImageRef is the user's requested image, already resolved to
	// an ECR-compatible reference (e.g. via resolveImageURI).
	BaseImageRef string
	// AgentBinaryPath is the in-build-context path of the
	// sockerless-agent binary that should be COPYed into /opt/sockerless.
	AgentBinaryPath string
	// BootstrapBinaryPath is the in-build-context path of the
	// sockerless-lambda-bootstrap binary.
	BootstrapBinaryPath string
	// UserEntrypoint is the original Dockerfile ENTRYPOINT from the
	// container's create request. May be empty (image default used).
	UserEntrypoint []string
	// UserCmd is the original Dockerfile CMD.
	UserCmd []string
}

// RenderOverlayDockerfile returns the Dockerfile content that, when
// built against a context containing the agent + bootstrap binaries,
// produces a Lambda-compatible image with exec routed through the
// reverse agent.
//
// The user's original ENTRYPOINT and CMD are captured in env vars
// (SOCKERLESS_USER_ENTRYPOINT / SOCKERLESS_USER_CMD) rather than
// preserved directly, because the bootstrap needs to own the
// ENTRYPOINT to serve the Lambda Runtime API.
func RenderOverlayDockerfile(spec OverlayImageSpec) (string, error) {
	if spec.BaseImageRef == "" {
		return "", fmt.Errorf("BaseImageRef is required")
	}
	if spec.AgentBinaryPath == "" {
		return "", fmt.Errorf("AgentBinaryPath is required")
	}
	if spec.BootstrapBinaryPath == "" {
		return "", fmt.Errorf("BootstrapBinaryPath is required")
	}

	var b strings.Builder
	fmt.Fprintf(&b, "FROM %s\n", spec.BaseImageRef)
	fmt.Fprintf(&b, "COPY %s /opt/sockerless/sockerless-agent\n", spec.AgentBinaryPath)
	fmt.Fprintf(&b, "COPY %s /opt/sockerless/sockerless-lambda-bootstrap\n", spec.BootstrapBinaryPath)
	fmt.Fprintln(&b, `RUN chmod +x /opt/sockerless/sockerless-agent /opt/sockerless/sockerless-lambda-bootstrap`)
	if ep := joinForEnv(spec.UserEntrypoint); ep != "" {
		fmt.Fprintf(&b, "ENV SOCKERLESS_USER_ENTRYPOINT=%s\n", ep)
	}
	if cmd := joinForEnv(spec.UserCmd); cmd != "" {
		fmt.Fprintf(&b, "ENV SOCKERLESS_USER_CMD=%s\n", cmd)
	}
	fmt.Fprintln(&b, `ENTRYPOINT ["/opt/sockerless/sockerless-lambda-bootstrap"]`)
	return b.String(), nil
}

// joinForEnv encodes a []string as base64-wrapped JSON for env-var
// transport. Empty input returns empty string so the ENV line can be
// omitted entirely. Base64 produces an alphabet (A-Z a-z 0-9 + / =)
// that needs no Dockerfile quoting and no shell escaping, so colons,
// quotes, newlines, and any other byte round-trip exactly through
// `ENV KEY=VALUE`. Matches the decoder in
// agent/cmd/sockerless-lambda-bootstrap/main.go::parseUserArgv.
func joinForEnv(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	b, err := json.Marshal(parts)
	if err != nil {
		return ""
	}
	return base64.StdEncoding.EncodeToString(b)
}

// OverlayBuildResult is what BuildAndPushOverlayImage returns — the
// pushed ECR URI of the overlay image, ready to reference as the
// Lambda function's `Code.ImageUri`.
type OverlayBuildResult struct {
	ImageURI string
}

// BuildAndPushOverlayImage materializes the overlay Dockerfile + a
// build context containing the agent and bootstrap binaries, then
// invokes `docker build` + `docker push` against the destination
// registry (real ECR live; sim-ECR in tests).
//
// The destRef must be a fully-qualified registry reference — e.g.
// `<account>.dkr.ecr.<region>.amazonaws.com/<repo>:skls-<id>`. The
// caller is responsible for choosing a tag that avoids cache
// collisions (the container ID is a good default).
//
// The returned ImageURI is the digest-or-tag form that Lambda should
// pull. Callers wire this into `CreateFunctionInput.Code.ImageUri`.
func BuildAndPushOverlayImage(ctx context.Context, spec OverlayImageSpec, destRef string) (*OverlayBuildResult, error) {
	if destRef == "" {
		return nil, fmt.Errorf("BuildAndPushOverlayImage: destRef is required")
	}
	dockerfile, err := RenderOverlayDockerfile(spec)
	if err != nil {
		return nil, fmt.Errorf("render Dockerfile: %w", err)
	}

	buildDir, err := os.MkdirTemp("", "sockerless-overlay-")
	if err != nil {
		return nil, fmt.Errorf("mktemp: %w", err)
	}
	defer os.RemoveAll(buildDir)

	// Stage the Dockerfile + binaries into the build context.
	dockerfilePath := filepath.Join(buildDir, "Dockerfile")
	if err := os.WriteFile(dockerfilePath, []byte(dockerfile), 0o644); err != nil {
		return nil, fmt.Errorf("write Dockerfile: %w", err)
	}
	if err := copyFile(spec.AgentBinaryPath, filepath.Join(buildDir, spec.AgentBinaryPath)); err != nil {
		return nil, fmt.Errorf("stage agent binary: %w", err)
	}
	if err := copyFile(spec.BootstrapBinaryPath, filepath.Join(buildDir, spec.BootstrapBinaryPath)); err != nil {
		return nil, fmt.Errorf("stage bootstrap binary: %w", err)
	}

	// docker build -t <destRef> <buildDir>
	buildCmd := exec.CommandContext(ctx, "docker", "build", "-t", destRef, buildDir)
	buildCmd.Stdout = os.Stderr
	buildCmd.Stderr = os.Stderr
	if err := buildCmd.Run(); err != nil {
		return nil, fmt.Errorf("docker build %s: %w", destRef, err)
	}

	// docker push <destRef>
	pushCmd := exec.CommandContext(ctx, "docker", "push", destRef)
	pushCmd.Stdout = os.Stderr
	pushCmd.Stderr = os.Stderr
	if err := pushCmd.Run(); err != nil {
		return nil, fmt.Errorf("docker push %s: %w", destRef, err)
	}

	return &OverlayBuildResult{ImageURI: destRef}, nil
}

// TarOverlayContext packages a Dockerfile + binaries into a tarball
// suitable for `docker ImageBuild` or an equivalent SDK call. Used by
// tests to assert the context contents without calling out to the
// docker CLI.
func TarOverlayContext(spec OverlayImageSpec) ([]byte, error) {
	dockerfile, err := RenderOverlayDockerfile(spec)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := writeTarEntry(tw, "Dockerfile", []byte(dockerfile), 0o644); err != nil {
		return nil, err
	}
	if err := writeTarFile(tw, spec.AgentBinaryPath, spec.AgentBinaryPath); err != nil {
		return nil, err
	}
	if err := writeTarFile(tw, spec.BootstrapBinaryPath, spec.BootstrapBinaryPath); err != nil {
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func writeTarEntry(tw *tar.Writer, name string, data []byte, mode int64) error {
	if err := tw.WriteHeader(&tar.Header{
		Name: name,
		Mode: mode,
		Size: int64(len(data)),
	}); err != nil {
		return err
	}
	_, err := tw.Write(data)
	return err
}

func writeTarFile(tw *tar.Writer, src, name string) error {
	info, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("stat %s: %w", src, err)
	}
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := tw.WriteHeader(&tar.Header{
		Name: name,
		Mode: 0o755,
		Size: info.Size(),
	}); err != nil {
		return err
	}
	_, err = io.Copy(tw, f)
	return err
}
