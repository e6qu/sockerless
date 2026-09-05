package core

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

// OverlayImageSpec describes the image a FaaS-shaped backend builds on
// top of the user's image so the cloud runs a sockerless bootstrap as the
// container's entrypoint. The bootstrap serves the user's command per
// invocation, which is what lets an ordinary image behave as an HTTP
// service on Google Cloud Run, Cloud Run Functions, Azure Container Apps,
// or Azure Functions, and lets the backend exec into it by request.
//
// The user's entrypoint, command, and working directory are not baked
// into the image. They travel as runtime environment on each deployment
// (see OverlayUserEnv), so one overlay image serves any user command and a
// warm pool of deployments can be reused across container types.
type OverlayImageSpec struct {
	// BaseImageRef is the user's image, already resolved to a reference the
	// cloud can pull.
	BaseImageRef string
	// BootstrapBinaryPath is the host path of the bootstrap binary COPYed
	// into /opt/sockerless. Its basename becomes the in-image binary name.
	BootstrapBinaryPath string
	// BootstrapBinaryHash is a content identifier for the binary at
	// BootstrapBinaryPath (see HashBootstrapBinary). It feeds
	// OverlayContentTag so replacing the binary invalidates cached overlays.
	BootstrapBinaryHash string
	// UserEntrypoint, UserCmd, and UserWorkdir are the container's own
	// command. They do not affect the image or its tag.
	UserEntrypoint []string
	UserCmd        []string
	UserWorkdir    string
}

func (s OverlayImageSpec) bootstrapName() string {
	return filepath.Base(s.BootstrapBinaryPath)
}

// RenderOverlayDockerfile returns the Dockerfile that, built against a
// context staged by TarOverlayContext, produces the bootstrap-fronted
// image.
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
	fmt.Fprintf(&b, "ENTRYPOINT [\"/opt/sockerless/%s\"]\n", name)
	return b.String(), nil
}

// OverlayContentTag returns a stable tag for the overlay image: the
// same (user image, bootstrap path, bootstrap hash) tuple always yields
// the same tag, so a second container on the same image reuses the built
// overlay. prefix scopes the tag namespace per backend.
func OverlayContentTag(prefix string, spec OverlayImageSpec) string {
	h := sha256.New()
	fmt.Fprintln(h, spec.BaseImageRef)
	fmt.Fprintln(h, spec.BootstrapBinaryPath)
	fmt.Fprintln(h, spec.BootstrapBinaryHash)
	sum := h.Sum(nil)
	return prefix + hex.EncodeToString(sum[:8])
}

// HashBootstrapBinary returns a 16-hex-character SHA-256 prefix of the
// file at path, for OverlayImageSpec.BootstrapBinaryHash.
func HashBootstrapBinary(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	sum := h.Sum(nil)
	return hex.EncodeToString(sum[:8]), nil
}

// TarOverlayContext packages the Dockerfile and the bootstrap binary into
// a gzipped tar suitable as a CloudBuildService build context.
func TarOverlayContext(spec OverlayImageSpec) ([]byte, error) {
	dockerfile, err := RenderOverlayDockerfile(spec)
	if err != nil {
		return nil, err
	}
	var raw bytes.Buffer
	gz := gzip.NewWriter(&raw)
	tw := tar.NewWriter(gz)
	if err := WriteTarEntry(tw, "Dockerfile", []byte(dockerfile), 0o644); err != nil {
		return nil, err
	}
	if err := WriteTarFile(tw, spec.BootstrapBinaryPath, spec.bootstrapName()); err != nil {
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

// WriteTarEntry writes data into the tar under name with the given mode.
func WriteTarEntry(tw *tar.Writer, name string, data []byte, mode int64) error {
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: mode, Size: int64(len(data))}); err != nil {
		return err
	}
	_, err := tw.Write(data)
	return err
}

// WriteTarFile writes the file at src into the tar under name as an
// executable.
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
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: info.Size()}); err != nil {
		return err
	}
	_, err = io.Copy(tw, f)
	return err
}

// JoinForEnv encodes an argv list as base64(JSON) for transport in an
// environment variable. The base64 alphabet needs no shell or Dockerfile
// quoting, so every byte of every argument round-trips. Empty input
// yields the empty string so the variable can be omitted.
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

// OverlayUserEnv returns the runtime environment that tells a bootstrap
// which command to run: SOCKERLESS_USER_ENTRYPOINT and SOCKERLESS_USER_CMD
// as JoinForEnv lists, and SOCKERLESS_USER_WORKDIR verbatim. Empty inputs
// produce no entry.
func OverlayUserEnv(entrypoint, cmd []string, workdir string) []string {
	var out []string
	if v := JoinForEnv(entrypoint); v != "" {
		out = append(out, "SOCKERLESS_USER_ENTRYPOINT="+v)
	}
	if v := JoinForEnv(cmd); v != "" {
		out = append(out, "SOCKERLESS_USER_CMD="+v)
	}
	if workdir != "" {
		out = append(out, "SOCKERLESS_USER_WORKDIR="+workdir)
	}
	return out
}

// OverlayRepositoryName is the registry repository path under which every
// backend pushes its overlay images (`<registry>/<...>/sockerless-overlay/<backend>:<tag>`).
const OverlayRepositoryName = "sockerless-overlay"

// HasOverlayRepo reports whether an image reference already points into
// the sockerless overlay repository, so a backend does not wrap an image
// that already carries the bootstrap.
func HasOverlayRepo(image string) bool {
	return strings.HasPrefix(image, OverlayRepositoryName+"/") || strings.Contains(image, "/"+OverlayRepositoryName+"/")
}
