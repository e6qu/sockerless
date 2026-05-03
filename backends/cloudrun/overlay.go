package cloudrun

import (
	"bytes"
	"context"
	"fmt"

	core "github.com/sockerless/backend-core"
	gcpcommon "github.com/sockerless/gcp-common"
)

// ensureOverlayImage builds + pushes the Cloud Run overlay image for
// `spec` to AR if it isn't already present. Returns the AR image URI.
//
// Cache check uses Cloud Build's source-deduplication and AR's
// content-addressed tag scheme: identical (user-image, bootstrap,
// entrypoint, cmd, workdir) tuples reuse the same overlay image
// across `docker create` invocations.
//
// The overlay COPYs sockerless-cloudrun-bootstrap into the user's
// image and sets it as ENTRYPOINT. The bootstrap is an HTTP server
// bound to $PORT (Cloud Run sets it) that serves the user's CMD on
// each request — turning any stock image (golang:1.22-alpine,
// postgres:16-alpine, etc.) into a Cloud-Run-compatible HTTP service
// that sockerless can `docker exec` into via Path B HTTP POST.
// See specs/CLOUD_RESOURCE_MAPPING.md § Lesson 8 for the design
// rationale.
func (s *Server) ensureOverlayImage(ctx context.Context, spec gcpcommon.OverlayImageSpec, contentTag string) (string, error) {
	imageURI := fmt.Sprintf(
		"%s-docker.pkg.dev/%s/sockerless-overlay/cloudrun:%s",
		s.config.Region, s.config.Project, contentTag,
	)
	if s.images.BuildService == nil {
		return "", fmt.Errorf("cloud Build service is required for cloudrun overlay image (set SOCKERLESS_GCP_BUILD_BUCKET)")
	}
	contextTar, err := gcpcommon.TarOverlayContext(spec)
	if err != nil {
		return "", fmt.Errorf("tar overlay context: %w", err)
	}
	result, err := s.images.BuildService.Build(ctx, core.CloudBuildOptions{
		Dockerfile: "Dockerfile",
		ContextTar: bytes.NewReader(contextTar),
		Tags:       []string{imageURI},
		Platform:   "linux/amd64",
	})
	if err != nil {
		return "", fmt.Errorf("cloud build %s: %w", imageURI, err)
	}
	return result.ImageRef, nil
}

// useOverlayPath reports whether ContainerCreate should build an overlay
// image for this container. Phase 122g rule: overlay required when
// BootstrapBinaryPath is configured (operator opted in) AND the image
// isn't already a sockerless-built overlay (avoid recursion).
func (s *Server) useOverlayPath(image string) bool {
	if s.config.BootstrapBinaryPath == "" {
		return false
	}
	// Avoid double-overlay: if the image URI is already in the
	// sockerless-overlay AR repo, the bootstrap is already baked in.
	if hasSockerlessOverlayRepo(image) {
		return false
	}
	return true
}

func hasSockerlessOverlayRepo(image string) bool {
	// Format: <region>-docker.pkg.dev/<project>/sockerless-overlay/...
	return contains(image, "/sockerless-overlay/")
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
