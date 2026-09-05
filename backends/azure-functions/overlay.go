package azf

import (
	"bytes"
	"context"
	"fmt"

	core "github.com/sockerless/backend-core"
)

// ensureAZFOverlayImage builds the bootstrap-fronted overlay of the
// user's image through Azure Container Registry Tasks and returns the
// image reference the Function App runs. The tag is content-addressed
// (see core.OverlayContentTag), so a second container on the same image
// reuses the built overlay.
func (s *Server) ensureAZFOverlayImage(ctx context.Context, spec core.OverlayImageSpec, contentTag string) (string, error) {
	if s.config.Registry == "" {
		return "", fmt.Errorf("SOCKERLESS_AZF_REGISTRY is required for Azure Functions overlay images")
	}
	if s.images == nil || s.images.BuildService == nil {
		return "", fmt.Errorf("ACR build service is required for Azure Functions overlay images (set SOCKERLESS_AZF_REGISTRY, SOCKERLESS_AZURE_BUILD_STORAGE_ACCOUNT, and SOCKERLESS_AZURE_BUILD_CONTAINER)")
	}
	contextTar, err := core.TarOverlayContext(spec)
	if err != nil {
		return "", fmt.Errorf("tar overlay context: %w", err)
	}
	tag := fmt.Sprintf("%s/azf:%s", core.OverlayRepositoryName, contentTag)
	result, err := s.images.BuildService.Build(ctx, core.CloudBuildOptions{
		Dockerfile: "Dockerfile",
		ContextTar: bytes.NewReader(contextTar),
		Tags:       []string{tag},
		Platform:   s.config.BuildPlatform,
	})
	if err != nil {
		return "", fmt.Errorf("ACR build %s/%s: %w", s.config.Registry, tag, err)
	}
	return result.ImageRef, nil
}

// useAZFOverlayPath reports whether the image needs the bootstrap overlay:
// only when a bootstrap binary is configured, and never for an image that
// already is one.
func (s *Server) useAZFOverlayPath(image string) bool {
	if s.config.BootstrapBinaryPath == "" {
		return false
	}
	return !core.HasOverlayRepo(image)
}
