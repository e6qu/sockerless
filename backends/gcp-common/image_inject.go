// Package gcpcommon shared overlay-image renderer + tarball builder for
// Cloud Run + Cloud Run Functions. Both backends inject a sockerless
// bootstrap binary as the image ENTRYPOINT so the cloud-side container
// is an HTTP server that runs user argv per request (Path B exec model
// — see specs/CLOUD_RESOURCE_MAPPING.md § Lesson 8). The renderer is
// shared so the wire format (env-var encoding, COPY layout, content tag)
// stays identical across cloudrun + gcf.
package gcpcommon

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// OverlayImageSpec describes the per-container overlay image for a GCP
// FaaS-shaped backend. Cloud Build packages a Dockerfile that COPYs the
// bootstrap binary on top of the user's resolved image and sets the
// bootstrap as ENTRYPOINT; user's original entrypoint+cmd ride as
// base64(JSON) env vars for the bootstrap to exec on each invocation.
type OverlayImageSpec struct {
	// BaseImageRef is the user's image, already resolved via
	// ResolveGCPImageURI to an AR-routable reference.
	BaseImageRef string
	// BootstrapBinaryPath is the host path of the bootstrap binary
	// to COPY into /opt/sockerless inside the overlay. The basename
	// becomes the in-image binary name (so callers control whether
	// it's sockerless-gcf-bootstrap, sockerless-cloudrun-bootstrap,
	// etc. via where they point this path).
	BootstrapBinaryPath string
	// UserEntrypoint is the original Dockerfile ENTRYPOINT. May be
	// empty (image default).
	UserEntrypoint []string
	// UserCmd is the original Dockerfile CMD.
	UserCmd []string
	// UserWorkdir is the WorkingDir for the user subprocess.
	UserWorkdir string
}

// bootstrapName returns the in-image basename for the bootstrap binary,
// derived from BootstrapBinaryPath. Empty path → empty name; the renderer
// errors before reaching here.
func (s OverlayImageSpec) bootstrapName() string {
	return filepath.Base(s.BootstrapBinaryPath)
}

// RenderOverlayDockerfile returns the Dockerfile content that, when
// built against a context staged by TarOverlayContext, produces an
// HTTP-serving image with the bootstrap as ENTRYPOINT.
func RenderOverlayDockerfile(spec OverlayImageSpec) (string, error) {
	if spec.BaseImageRef == "" {
		return "", fmt.Errorf("BaseImageRef is required")
	}
	if spec.BootstrapBinaryPath == "" {
		return "", fmt.Errorf("BootstrapBinaryPath is required")
	}
	name := spec.bootstrapName()

	var b strings.Builder
	fmt.Fprintf(&b, "FROM %s\n", spec.BaseImageRef)
	fmt.Fprintf(&b, "COPY %s /opt/sockerless/%s\n", name, name)
	fmt.Fprintf(&b, "RUN chmod +x /opt/sockerless/%s\n", name)
	if ep := JoinForEnv(spec.UserEntrypoint); ep != "" {
		fmt.Fprintf(&b, "ENV SOCKERLESS_USER_ENTRYPOINT=%s\n", ep)
	}
	if cmd := JoinForEnv(spec.UserCmd); cmd != "" {
		fmt.Fprintf(&b, "ENV SOCKERLESS_USER_CMD=%s\n", cmd)
	}
	if spec.UserWorkdir != "" {
		fmt.Fprintf(&b, "ENV SOCKERLESS_USER_WORKDIR=%s\n", spec.UserWorkdir)
	}
	fmt.Fprintf(&b, "ENTRYPOINT [\"/opt/sockerless/%s\"]\n", name)
	return b.String(), nil
}

// JoinForEnv encodes a []string as base64(JSON) for env-var transport.
// Used by both gcf + cloudrun overlays so bootstrap argv decoders stay
// cross-cloud consistent (mirrors the Lambda backend's encoding too).
func JoinForEnv(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	b, err := json.Marshal(parts)
	if err != nil {
		return ""
	}
	return base64.StdEncoding.EncodeToString(b)
}

// OverlayContentTag returns a stable content-addressed tag for the
// overlay image. Identical (user-image, bootstrap, entrypoint, cmd,
// workdir) tuples reuse the already-built overlay. `prefix` lets each
// caller scope its tag namespace (e.g. `gcf-`, `cloudrun-`) so the
// per-cloud image cache doesn't collide.
func OverlayContentTag(prefix string, spec OverlayImageSpec) string {
	h := sha256.New()
	fmt.Fprintln(h, spec.BaseImageRef)
	fmt.Fprintln(h, spec.BootstrapBinaryPath)
	if epb, err := json.Marshal(spec.UserEntrypoint); err == nil {
		h.Write(epb)
	}
	if cmdb, err := json.Marshal(spec.UserCmd); err == nil {
		h.Write(cmdb)
	}
	fmt.Fprintln(h, spec.UserWorkdir)
	sum := h.Sum(nil)
	return prefix + hex.EncodeToString(sum[:8])
}

// TarOverlayContext packages the Dockerfile + bootstrap binary into a
// gzipped tar suitable as the build context for GCPBuildService.Build.
func TarOverlayContext(spec OverlayImageSpec) ([]byte, error) {
	dockerfile, err := RenderOverlayDockerfile(spec)
	if err != nil {
		return nil, err
	}
	name := spec.bootstrapName()

	var raw bytes.Buffer
	gz := gzip.NewWriter(&raw)
	tw := tar.NewWriter(gz)
	if err := WriteTarEntry(tw, "Dockerfile", []byte(dockerfile), 0o644); err != nil {
		return nil, err
	}
	if err := WriteTarFile(tw, spec.BootstrapBinaryPath, name); err != nil {
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

// WriteTarEntry writes a constant-bytes file into the tar.
func WriteTarEntry(tw *tar.Writer, name string, data []byte, mode int64) error {
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

// WriteTarFile writes a file from disk into the tar under `name`.
func WriteTarFile(tw *tar.Writer, src, name string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat %s: %w", src, err)
	}
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
