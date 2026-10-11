package lambda

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
	// AgentBinaryHash and BootstrapBinaryHash identify the content of the
	// two binaries (core.HashBootstrapBinary), so replacing either one
	// yields a new overlay tag instead of reusing an image that carries
	// the old binary.
	AgentBinaryHash     string
	BootstrapBinaryHash string
	// Platform is the `linux/<arch>` the overlay is built for: the
	// architecture of the function that runs it.
	Platform string
	// UserEntrypoint is the original Dockerfile ENTRYPOINT from the
	// container's create request. May be empty (image default used).
	UserEntrypoint []string
	// UserCmd is the original Dockerfile CMD.
	UserCmd []string
	// BindLinks is the set of `<dst>=<mnt-target>` symlink mappings
	// the overlay should create at build time (Lambda's root
	// filesystem is read-only at runtime, so the bootstrap can't
	// create them on first boot). One `RUN ln -sfn <target> <dst>`
	// directive is emitted per entry. Same env-var format the
	// bootstrap recognises if BindLinks isn't pre-baked.
	BindLinks []string
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
	// Pre-create bind-link symlinks at build time. Lambda's runtime
	// container filesystem is read-only outside /tmp + the EFS mount,
	// so the bootstrap can't create these on first boot. Targets are
	// stored as path strings — the destination dir doesn't need to
	// exist at build time.
	for _, entry := range spec.BindLinks {
		dst, target, ok := splitBindLink(entry)
		if !ok {
			continue
		}
		fmt.Fprintf(&b, "RUN mkdir -p %s && ln -sfn %s %s\n", shellQuote(filepath.Dir(dst)), shellQuote(target), shellQuote(dst))
	}
	if ep := core.JoinForEnv(spec.UserEntrypoint); ep != "" {
		fmt.Fprintf(&b, "ENV SOCKERLESS_USER_ENTRYPOINT=%s\n", ep)
	}
	if cmd := core.JoinForEnv(spec.UserCmd); cmd != "" {
		fmt.Fprintf(&b, "ENV SOCKERLESS_USER_CMD=%s\n", cmd)
	}
	fmt.Fprintln(&b, `ENTRYPOINT ["/opt/sockerless/sockerless-lambda-bootstrap"]`)
	return b.String(), nil
}

// PodOverlaySpec describes the merged-rootfs pod overlay built when
// sockerless materialises a multi-container pod into one Lambda
// Function. Per spec § "Podman pods on FaaS backends — supervisor-in-overlay",
// each pod member's rootfs is COPY --from'd into a per-name subdir
// (`/containers/<name>`) of a single image; the bootstrap (PID 1) then
// chroots and exec's each member.
//
// Mirrors the gcf backend's PodOverlaySpec exactly so the design is
// stable across FaaS backends. See backends/cloudrun-functions/image_inject.go.
type PodOverlaySpec struct {
	PodName             string
	MainName            string
	AgentBinaryPath     string
	BootstrapBinaryPath string
	// AgentBinaryHash, BootstrapBinaryHash and Platform carry the same
	// meaning as on OverlayImageSpec.
	AgentBinaryHash     string
	BootstrapBinaryHash string
	Platform            string
	BindLinks           []string
	Members             []core.PodMemberSpec
}

// RenderPodOverlayDockerfile returns the Dockerfile content for a pod
// overlay: first member's image as base + per-member `COPY --from=<image>
// / /containers/<name>/` for the rest + agent + bootstrap + bind-link
// symlinks + SOCKERLESS_POD_CONTAINERS env. The bootstrap (PID 1 via
// ENTRYPOINT) parses the manifest at init and runs in supervisor mode.
func RenderPodOverlayDockerfile(spec PodOverlaySpec) (string, error) {
	if spec.AgentBinaryPath == "" {
		return "", fmt.Errorf("AgentBinaryPath is required")
	}
	if spec.BootstrapBinaryPath == "" {
		return "", fmt.Errorf("BootstrapBinaryPath is required")
	}
	if len(spec.Members) == 0 {
		return "", fmt.Errorf("members must be non-empty")
	}
	for i, m := range spec.Members {
		if m.Name == "" {
			return "", fmt.Errorf("member %d: Name is required", i)
		}
		if m.BaseImageRef == "" {
			return "", fmt.Errorf("member %q: BaseImageRef is required", m.Name)
		}
	}
	manifest, err := core.EncodePodManifest(spec.Members)
	if err != nil {
		return "", fmt.Errorf("encode pod manifest: %w", err)
	}

	first := spec.Members[0]
	var b strings.Builder
	fmt.Fprintf(&b, "FROM %s\n", first.BaseImageRef)
	// Snapshot the base rootfs into the first member's chroot subdir
	// before subsequent COPYs would clobber any shared paths under /.
	// `cp -a` preserves symlinks/perms/times so the chroot looks
	// identical to the unwrapped base from the member's perspective.
	fmt.Fprintf(&b, "RUN mkdir -p /containers/%s && cp -a /. /containers/%s/ 2>/dev/null || true\n", first.Name, first.Name)
	for _, m := range spec.Members[1:] {
		fmt.Fprintf(&b, "COPY --from=%s / /containers/%s/\n", m.BaseImageRef, m.Name)
	}
	fmt.Fprintf(&b, "COPY %s /opt/sockerless/sockerless-agent\n", overlayAgentContextName)
	fmt.Fprintf(&b, "COPY %s /opt/sockerless/sockerless-lambda-bootstrap\n", overlayBootstrapContextName)
	fmt.Fprintln(&b, `RUN chmod +x /opt/sockerless/sockerless-agent /opt/sockerless/sockerless-lambda-bootstrap`)
	for _, entry := range spec.BindLinks {
		dst, target, ok := splitBindLink(entry)
		if !ok {
			continue
		}
		fmt.Fprintf(&b, "RUN mkdir -p %s && ln -sfn %s %s\n", shellQuote(filepath.Dir(dst)), shellQuote(target), shellQuote(dst))
	}
	fmt.Fprintf(&b, "ENV %s=%s\n", "SOCKERLESS_POD_CONTAINERS", manifest)
	if spec.MainName != "" {
		fmt.Fprintf(&b, "ENV %s=%s\n", "SOCKERLESS_POD_MAIN", spec.MainName)
	}
	fmt.Fprintln(&b, `ENTRYPOINT ["/opt/sockerless/sockerless-lambda-bootstrap"]`)
	return b.String(), nil
}

// PodOverlayContentTag returns a stable content-addressed tag for the
// pod overlay image. Identical pod manifests reuse the same ECR image.
// Format: `overlay-pod-<sha256[:16]>`.
func PodOverlayContentTag(spec PodOverlaySpec) string {
	h := sha256.New()
	fmt.Fprintln(h, spec.AgentBinaryPath)
	fmt.Fprintln(h, spec.BootstrapBinaryPath)
	fmt.Fprintln(h, spec.AgentBinaryHash)
	fmt.Fprintln(h, spec.BootstrapBinaryHash)
	fmt.Fprintln(h, spec.Platform)
	fmt.Fprintln(h, spec.MainName)
	for _, m := range spec.Members {
		fmt.Fprintln(h, m.Name)
		fmt.Fprintln(h, m.BaseImageRef)
		if epb, err := json.Marshal(m.Entrypoint); err == nil {
			h.Write(epb)
		}
		if cmdb, err := json.Marshal(m.Cmd); err == nil {
			h.Write(cmdb)
		}
		fmt.Fprintln(h, m.Workdir)
		if envb, err := json.Marshal(m.Env); err == nil {
			h.Write(envb)
		}
	}
	for _, entry := range spec.BindLinks {
		fmt.Fprintln(h, entry)
	}
	sum := h.Sum(nil)
	return "overlay-pod-" + hex.EncodeToString(sum[:8])
}

// TarPodOverlayContext packages the pod-overlay Dockerfile + agent +
// bootstrap binaries into a gzipped tar archive suitable as the build
// context for `core.CloudBuildService.Build` (CodeBuild) or local
// `docker build`. Per-member rootfs is pulled by the build via the
// Dockerfile's `COPY --from=<image>` references.
func TarPodOverlayContext(spec PodOverlaySpec) ([]byte, error) {
	dockerfile, err := RenderPodOverlayDockerfile(spec)
	if err != nil {
		return nil, err
	}
	var raw bytes.Buffer
	gz := gzip.NewWriter(&raw)
	tw := tar.NewWriter(gz)
	if err := core.WriteTarEntry(tw, "Dockerfile", []byte(dockerfile), 0o644); err != nil {
		return nil, err
	}
	if err := core.WriteTarFile(tw, spec.AgentBinaryPath, overlayAgentContextName); err != nil {
		return nil, err
	}
	if err := core.WriteTarFile(tw, spec.BootstrapBinaryPath, overlayBootstrapContextName); err != nil {
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

// OverlayBuildResult is what BuildAndPushOverlayImage returns — the
// image reference the function is created from.
type OverlayBuildResult struct {
	ImageURI string
}

// BuildAndPushOverlayImage builds the overlay through the cloud build
// service (AWS CodeBuild) and pushes it to destRef, for the platform the
// spec names. The build runs on the cloud, as it does when sockerless
// itself runs inside a function with no container engine beside it, so
// the image a function runs is built the same way wherever the backend
// is hosted.
//
// The destRef must be a fully-qualified registry reference — e.g.
// `<account>.dkr.ecr.<region>.amazonaws.com/<repo>:<tag>`. Callers
// derive the tag via `OverlayContentTag(spec)` for content-addressed
// caching.
func BuildAndPushOverlayImage(ctx context.Context, spec OverlayImageSpec, destRef string, builder core.CloudBuildService) (*OverlayBuildResult, error) {
	if destRef == "" {
		return nil, fmt.Errorf("BuildAndPushOverlayImage: destRef is required")
	}
	if spec.Platform == "" {
		return nil, fmt.Errorf("BuildAndPushOverlayImage: Platform is required")
	}
	if builder == nil || !builder.Available() {
		return nil, fmt.Errorf("BuildAndPushOverlayImage: no AWS CodeBuild project and build bucket are configured (SOCKERLESS_LAMBDA_CODEBUILD_PROJECT, SOCKERLESS_LAMBDA_BUILD_BUCKET)")
	}
	contextTar, err := TarOverlayContext(spec)
	if err != nil {
		return nil, fmt.Errorf("tar overlay context: %w", err)
	}
	result, err := builder.Build(ctx, core.CloudBuildOptions{
		Dockerfile: "Dockerfile",
		ContextTar: bytes.NewReader(contextTar),
		Tags:       []string{destRef},
		Platform:   spec.Platform,
	})
	if err != nil {
		return nil, fmt.Errorf("cloud build %s: %w", destRef, err)
	}
	return &OverlayBuildResult{ImageURI: result.ImageRef}, nil
}

// splitBindLink parses a `<dst>=<target>` entry. Same format as the
// CSV in `SOCKERLESS_LAMBDA_BIND_LINKS`. Returns ok=false on malformed
// entries so callers can skip them silently (the bootstrap repeats the
// validation at runtime if they ever leak through).
func splitBindLink(entry string) (dst, target string, ok bool) {
	idx := strings.IndexByte(entry, '=')
	if idx <= 0 || idx == len(entry)-1 {
		return "", "", false
	}
	return strings.TrimSpace(entry[:idx]), strings.TrimSpace(entry[idx+1:]), true
}

// shellQuote produces a single-quoted shell literal. Used inside RUN
// directives where the path could contain shell metacharacters.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// OverlayContentTag returns a stable, content-addressed tag for the
// overlay image identified by `spec`. The tag is `overlay-<sha256[:16]>`
// computed over the inputs that determine the image's bytes:
//
//   - BaseImageRef (the user image we layer on top of)
//   - AgentBinaryPath + BootstrapBinaryPath (paths used by COPY in the
//     rendered Dockerfile) and the binaries' content hashes
//   - Platform (the architecture the image is built for)
//   - User entrypoint + cmd (encoded into ENV in the rendered Dockerfile)
//   - BindLinks (symlinks baked into the image)
//
// Two `docker create` calls with the same inputs collide on the same tag —
// Lambda's CreateFunction can reuse the already-pushed image and skip the
// rebuild. Different inputs produce different tags.
func OverlayContentTag(spec OverlayImageSpec) string {
	h := sha256.New()
	fmt.Fprintln(h, spec.BaseImageRef)
	fmt.Fprintln(h, spec.AgentBinaryPath)
	fmt.Fprintln(h, spec.BootstrapBinaryPath)
	fmt.Fprintln(h, spec.AgentBinaryHash)
	fmt.Fprintln(h, spec.BootstrapBinaryHash)
	fmt.Fprintln(h, spec.Platform)
	if epb, err := json.Marshal(spec.UserEntrypoint); err == nil {
		h.Write(epb)
	}
	if cmdb, err := json.Marshal(spec.UserCmd); err == nil {
		h.Write(cmdb)
	}
	for _, entry := range spec.BindLinks {
		fmt.Fprintln(h, entry)
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
	if err := core.WriteTarEntry(tw, "Dockerfile", []byte(dockerfile), 0o644); err != nil {
		return nil, err
	}
	if err := core.WriteTarFile(tw, spec.AgentBinaryPath, overlayAgentContextName); err != nil {
		return nil, err
	}
	if err := core.WriteTarFile(tw, spec.BootstrapBinaryPath, overlayBootstrapContextName); err != nil {
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
