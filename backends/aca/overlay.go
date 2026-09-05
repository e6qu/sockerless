package aca

import (
	"bytes"
	"context"
	"fmt"

	core "github.com/sockerless/backend-core"
)

// ensureACAOverlayImage builds the bootstrap-fronted overlay of the
// user's image through Azure Container Registry Tasks and returns the
// image reference the Container App runs. The tag is content-addressed
// (see core.OverlayContentTag), so a second container on the same image
// reuses the built overlay.
func (s *Server) ensureACAOverlayImage(ctx context.Context, spec core.OverlayImageSpec, contentTag string) (string, error) {
	if s.config.ACRName == "" {
		return "", fmt.Errorf("SOCKERLESS_AZURE_ACR_NAME is required for ACA App overlay images")
	}
	if s.images == nil || s.images.BuildService == nil {
		return "", fmt.Errorf("ACR build service is required for ACA App overlay images (set SOCKERLESS_AZURE_ACR_NAME, SOCKERLESS_AZURE_BUILD_STORAGE_ACCOUNT, and SOCKERLESS_AZURE_BUILD_CONTAINER)")
	}
	contextTar, err := core.TarOverlayContext(spec)
	if err != nil {
		return "", fmt.Errorf("tar overlay context: %w", err)
	}
	tag := fmt.Sprintf("%s/aca:%s", core.OverlayRepositoryName, contentTag)
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

// useACAOverlayPath reports whether the image needs the bootstrap overlay:
// only on the Apps path, and never for an image that already is one.
func (s *Server) useACAOverlayPath(image string) bool {
	if !s.config.UseApp {
		return false
	}
	return !core.HasOverlayRepo(image)
}
