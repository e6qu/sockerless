package aca

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
	"path/filepath"
	"strings"

	core "github.com/sockerless/backend-core"
)

type acaOverlaySpec struct {
	BaseImageRef        string
	BootstrapBinaryPath string
	BootstrapBinaryHash string
}

func (s acaOverlaySpec) bootstrapName() string {
	return filepath.Base(s.BootstrapBinaryPath)
}

func renderACAOverlayDockerfile(spec acaOverlaySpec) (string, error) {
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

func acaOverlayContentTag(prefix string, spec acaOverlaySpec) string {
	h := sha256.New()
	fmt.Fprintln(h, spec.BaseImageRef)
	fmt.Fprintln(h, spec.BootstrapBinaryPath)
	fmt.Fprintln(h, spec.BootstrapBinaryHash)
	sum := h.Sum(nil)
	return prefix + hex.EncodeToString(sum[:8])
}

func tarACAOverlayContext(spec acaOverlaySpec) ([]byte, error) {
	dockerfile, err := renderACAOverlayDockerfile(spec)
	if err != nil {
		return nil, err
	}
	name := spec.bootstrapName()

	var raw bytes.Buffer
	gz := gzip.NewWriter(&raw)
	tw := tar.NewWriter(gz)
	if err := writeTarEntry(tw, "Dockerfile", []byte(dockerfile), 0o644); err != nil {
		return nil, err
	}
	if err := writeTarFile(tw, spec.BootstrapBinaryPath, name); err != nil {
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

func hashBootstrapBinary(path string) (string, error) {
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

func (s *Server) ensureACAOverlayImage(ctx context.Context, spec acaOverlaySpec, contentTag string) (string, error) {
	if s.config.ACRName == "" {
		return "", fmt.Errorf("SOCKERLESS_AZURE_ACR_NAME is required for ACA App overlay images")
	}
	if s.images == nil || s.images.BuildService == nil {
		return "", fmt.Errorf("ACR build service is required for ACA App overlay images (set SOCKERLESS_AZURE_ACR_NAME, SOCKERLESS_AZURE_BUILD_STORAGE_ACCOUNT, and SOCKERLESS_AZURE_BUILD_CONTAINER)")
	}
	contextTar, err := tarACAOverlayContext(spec)
	if err != nil {
		return "", fmt.Errorf("tar overlay context: %w", err)
	}
	tag := fmt.Sprintf("sockerless-overlay/aca:%s", contentTag)
	result, err := s.images.BuildService.Build(ctx, core.CloudBuildOptions{
		Dockerfile: "Dockerfile",
		ContextTar: bytes.NewReader(contextTar),
		Tags:       []string{tag},
		Platform:   s.config.BuildPlatform,
	})
	if err != nil {
		return "", fmt.Errorf("ACR build %s.azurecr.io/%s: %w", s.config.ACRName, tag, err)
	}
	return result.ImageRef, nil
}

func (s *Server) useACAOverlayPath(image string) bool {
	if !s.config.UseApp {
		return false
	}
	return !hasACAOverlayRepo(image)
}

func hasACAOverlayRepo(image string) bool {
	return strings.Contains(image, ".azurecr.io/sockerless-overlay/") ||
		strings.Contains(image, "/sockerless-overlay/")
}

func acaOverlayUserEnv(entrypoint, cmd []string, workdir string) []string {
	var out []string
	if len(entrypoint) > 0 {
		if b, err := json.Marshal(entrypoint); err == nil {
			out = append(out, "SOCKERLESS_USER_ENTRYPOINT="+base64.StdEncoding.EncodeToString(b))
		}
	}
	if len(cmd) > 0 {
		if b, err := json.Marshal(cmd); err == nil {
			out = append(out, "SOCKERLESS_USER_CMD="+base64.StdEncoding.EncodeToString(b))
		}
	}
	if workdir != "" {
		out = append(out, "SOCKERLESS_USER_WORKDIR="+workdir)
	}
	return out
}
