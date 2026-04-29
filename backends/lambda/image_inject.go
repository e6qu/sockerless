package lambda

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	core "github.com/sockerless/backend-core"
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

// Build-context-relative file names for the agent + bootstrap binaries
// inside the overlay tarball. Both the renderer (for Dockerfile COPY
// sources) and the tar packager (for in-tar entry names) use these so
// the layout is stable regardless of where the binaries live on the
// host running sockerless.
const (
	overlayAgentContextName     = "sockerless-agent"
	overlayBootstrapContextName = "sockerless-lambda-bootstrap"
)

// RenderOverlayDockerfile returns the Dockerfile content that, when
// built against a context containing the agent + bootstrap binaries
// (named per `overlayAgentContextName` / `overlayBootstrapContextName`),
// produces a Lambda-compatible image with `sockerless-lambda-bootstrap`
// at the entrypoint.
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
	fmt.Fprintf(&b, "COPY %s /opt/sockerless/sockerless-agent\n", overlayAgentContextName)
	fmt.Fprintf(&b, "COPY %s /opt/sockerless/sockerless-lambda-bootstrap\n", overlayBootstrapContextName)
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
// builds + pushes via either a CloudBuildService (when running with
// no local docker daemon — the runner-Lambda case) or local
// `docker build` / `docker push` (laptop case). When a build service
// is supplied AND `Available()` returns true, that path is preferred.
//
// The destRef must be a fully-qualified registry reference — e.g.
// `<account>.dkr.ecr.<region>.amazonaws.com/<repo>:<tag>`. Callers
// derive the tag via `OverlayContentTag(spec)` for content-addressed
// caching, or pass an explicit tag when content addressing isn't
// useful (e.g. integration tests).
//
// The returned ImageURI is the digest-or-tag form that Lambda should
// pull. Callers wire this into `CreateFunctionInput.Code.ImageUri`.
func BuildAndPushOverlayImage(ctx context.Context, spec OverlayImageSpec, destRef string, builder core.CloudBuildService) (*OverlayBuildResult, error) {
	if destRef == "" {
		return nil, fmt.Errorf("BuildAndPushOverlayImage: destRef is required")
	}
	if builder != nil && builder.Available() {
		return buildOverlayViaCloudBuild(ctx, spec, destRef, builder)
	}
	return buildOverlayViaLocalDocker(ctx, spec, destRef)
}

// buildOverlayViaCloudBuild routes the overlay build through a
// `core.CloudBuildService` (e.g. AWS CodeBuild). Used when sockerless
// is running inside a Lambda function (no docker daemon) and during
// any other docker-less environment. The build context tar contains
// the rendered Dockerfile + the agent + bootstrap binaries staged at
// their declared paths.
func buildOverlayViaCloudBuild(ctx context.Context, spec OverlayImageSpec, destRef string, builder core.CloudBuildService) (*OverlayBuildResult, error) {
	contextTar, err := TarOverlayContext(spec)
	if err != nil {
		return nil, fmt.Errorf("tar overlay context: %w", err)
	}
	result, err := builder.Build(ctx, core.CloudBuildOptions{
		Dockerfile: "Dockerfile",
		ContextTar: bytes.NewReader(contextTar),
		Tags:       []string{destRef},
		Platform:   "linux/amd64",
	})
	if err != nil {
		return nil, fmt.Errorf("cloud build %s: %w", destRef, err)
	}
	return &OverlayBuildResult{ImageURI: result.ImageRef}, nil
}

// buildOverlayViaLocalDocker is the legacy path: shell out to local
// `docker build` + `docker push`. Used on a laptop with Docker Desktop
// or an external Podman VM available via DOCKER_HOST.
func buildOverlayViaLocalDocker(ctx context.Context, spec OverlayImageSpec, destRef string) (*OverlayBuildResult, error) {
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

// OverlayContentTag returns a stable, content-addressed tag for the
// overlay image identified by `spec`. The tag is `overlay-<sha256[:16]>`
// computed over the inputs that determine the image's bytes:
//
//   - BaseImageRef (the user image we layer on top of)
//   - AgentBinaryPath + BootstrapBinaryPath (paths used by COPY in the
//     rendered Dockerfile)
//   - User entrypoint + cmd (encoded into ENV in the rendered Dockerfile)
//
// Two `docker create` calls with the same base image, same user
// entrypoint/cmd, and the same agent/bootstrap binaries collide on the
// same tag — Lambda's CreateFunction can reuse the already-pushed image
// and skip the rebuild. Different inputs produce different tags.
//
// Callers append `-amd64` (or another platform suffix) externally if
// they support multiple Lambda architectures from the same overlay
// pipeline.
func OverlayContentTag(spec OverlayImageSpec) string {
	h := sha256.New()
	fmt.Fprintln(h, spec.BaseImageRef)
	fmt.Fprintln(h, spec.AgentBinaryPath)
	fmt.Fprintln(h, spec.BootstrapBinaryPath)
	if epb, err := json.Marshal(spec.UserEntrypoint); err == nil {
		h.Write(epb)
	}
	if cmdb, err := json.Marshal(spec.UserCmd); err == nil {
		h.Write(cmdb)
	}
	sum := h.Sum(nil)
	return "overlay-" + hex.EncodeToString(sum[:8])
}

// TarOverlayContext packages a Dockerfile + binaries into a gzipped
// tarball suitable for `docker ImageBuild` or upload to CodeBuild's S3
// build-context source. Tar entries use stable, context-relative names
// (`Dockerfile`, `sockerless-agent`, `sockerless-lambda-bootstrap`)
// matching the COPY directives in `RenderOverlayDockerfile`.
func TarOverlayContext(spec OverlayImageSpec) ([]byte, error) {
	dockerfile, err := RenderOverlayDockerfile(spec)
	if err != nil {
		return nil, err
	}

	var raw bytes.Buffer
	gz := gzip.NewWriter(&raw)
	tw := tar.NewWriter(gz)
	if err := writeTarEntry(tw, "Dockerfile", []byte(dockerfile), 0o644); err != nil {
		return nil, err
	}
	if err := writeTarFile(tw, spec.AgentBinaryPath, overlayAgentContextName); err != nil {
		return nil, err
	}
	if err := writeTarFile(tw, spec.BootstrapBinaryPath, overlayBootstrapContextName); err != nil {
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return raw.Bytes(), nil
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
