package lambda

import (
	"context"
	"fmt"
	"strings"

	ecrtypes "github.com/aws/aws-sdk-go-v2/service/ecr/types"
	awscommon "github.com/sockerless/aws-common"
	core "github.com/sockerless/backend-core"
)

// resolveImageURI converts a Docker image reference to a registry that
// Lambda can pull from. Lambda only supports ECR image URIs, so even
// public.ecr.aws references go through a pull-through cache (Lambda's
// CreateFunction rejects multi-arch manifests, and Public Gallery
// images come back as one — the cache rewrites to a single-arch ECR
// repo Lambda can ingest). Routing rules:
//
//  1. Already-ECR URIs pass through unchanged.
//  2. Docker Hub library refs (`alpine`, `node:20`, `nginx:alpine` —
//     i.e. the official-image namespace) get rewritten to a private
//     ECR pull-through cache that targets the AWS Public Gallery
//     mirror at `public.ecr.aws/docker/library/`. AWS hosts the
//     mirror; no Docker Hub credentials needed.
//  3. Other public registries (`public.ecr.aws/...`, `ghcr.io/...`,
//     `quay.io/...`, etc.) route through their own pull-through cache
//     rules — none need credentials when caching public images.
//  4. Docker Hub user/org refs (`myorg/myapp`) are rejected with a
//     clear error because (a) AWS Public Gallery only mirrors
//     `library/` and (b) Docker Hub user-image pull-through needs
//     Docker Hub PAT-backed auth which the project's
//     no-credentials-on-disk discipline avoids by design. Operators
//     who need such an image should `docker push` it to their ECR
//     repo first.
func (s *Server) resolveImageURI(ctx context.Context, ref string) (string, error) {
	if awscommon.IsECRImageURI(ref, s.config.ECRRegistryEndpoint) {
		return awscommon.RelocateECRReference(ref, s.config.ECRRegistryEndpoint), nil
	}

	// Digest-only refs (`sha256:...` with no preceding name) — resolve
	// via the local image Store. gitlab-runner's volume-permission
	// helper references images by digest after a prior pull; without
	// this lookup the reference would split into repo="sha256" tag="hex"
	// and the resulting Dockerfile FROM
	// `…/public-ecr-aws/docker/library/sha256:hex` 404s in CodeBuild.
	if strings.HasPrefix(ref, "sha256:") && !strings.ContainsAny(ref, "/@") {
		if img, ok := s.Store.ResolveImage(ref); ok {
			canonical, named := core.ImageReference(img)
			if !named {
				return ref, fmt.Errorf("image %s pulled for digest-only reference %q has neither a tag nor a repository digest", img.ID, ref)
			}
			s.Logger.Debug().Str("digest", ref).Str("canonical", canonical).
				Msg("resolved digest-only ref via image Store")
			return s.resolveImageURI(ctx, canonical)
		}
		return ref, fmt.Errorf(
			"digest-only image reference %q can't be resolved on Lambda — image Store has no record of this digest; reference the image by name+digest (`name@%s`) or pull it first via `docker pull <name>`",
			ref, ref,
		)
	}

	// Decompose the reference. For public.ecr.aws/<path> we treat it
	// the same as any registry-prefixed ref so it goes through a
	// pull-through cache (Lambda needs single-arch ECR-hosted images).
	registry, repo, version := core.SplitDockerRef(ref)

	var cachePrefix, upstreamURL string
	var upstreamKind ecrtypes.UpstreamRegistry
	switch registry {
	case "", "docker.io", "registry-1.docker.io":
		// Library images live on AWS Public Gallery as `docker/library/<name>`.
		// Strip the explicit `library/` prefix some clients emit
		// (gitlab-runner resolves `alpine:latest` to
		// `docker.io/library/alpine:latest` before Cmd dispatch) so the
		// "user/org image rejected" guard below doesn't false-positive.
		repo = strings.TrimPrefix(repo, "library/")
		if strings.Contains(repo, "/") {
			return ref, fmt.Errorf("docker hub user/org image %q is not on AWS Public Gallery; push it to your ECR repository first (sockerless avoids Docker Hub PAT credentials by design — use `docker push <ecr-uri>` to host the image yourself, or reference its public.ecr.aws equivalent if one exists)", ref)
		}
		upstreamURL = "public.ecr.aws"
		upstreamKind = ecrtypes.UpstreamRegistryEcrPublic
		repo = "docker/library/" + repo
	case "public.ecr.aws":
		upstreamURL = "public.ecr.aws"
		upstreamKind = ecrtypes.UpstreamRegistryEcrPublic
	default:
		upstreamURL = registry
		upstreamKind = awscommon.UpstreamRegistryFor(registry)
	}
	cachePrefix = awscommon.PullThroughCachePrefix(upstreamURL)

	cache := awscommon.ECRPullThroughCache{Client: s.aws.ECR, Logger: s.Logger}
	if err := cache.Ensure(ctx, cachePrefix, upstreamURL, upstreamKind); err != nil {
		return ref, fmt.Errorf("ECR pull-through cache setup for %q: %w", cachePrefix, err)
	}

	accountID := awscommon.ExtractAccountID(s.config.RoleARN)
	if accountID == "" {
		return "", fmt.Errorf("cannot determine AWS account ID from role ARN %q", s.config.RoleARN)
	}

	ecrURI := awscommon.PullThroughCacheURI(awscommon.ECRRegistryHost(accountID, s.config.Region, s.config.ECRRegistryEndpoint), cachePrefix, repo, version)
	s.Logger.Info().Str("original", ref).Str("ecr", ecrURI).Msg("resolved image to ECR pull-through cache URI")
	return ecrURI, nil
}

// overlayECRRepo returns the fully-qualified ECR repo (no tag) where
// sockerless pushes overlay-converted user images: the operator's
// `SOCKERLESS_LAMBDA_OVERLAY_ECR_REPO` (the repository the Lambda
// Terraform module provisions, its `ecr_repository_url` output), else
// `sockerless-live-lambda` in the account's registry. Either one is
// reached at the registry endpoint coordinate when one is set. Tags are
// content-addressed via `OverlayContentTag(spec)`.
func (s *Server) overlayECRRepo() (string, error) {
	if s.config.OverlayECRRepo != "" {
		return awscommon.RelocateECRReference(s.config.OverlayECRRepo, s.config.ECRRegistryEndpoint), nil
	}
	accountID := awscommon.ExtractAccountID(s.config.RoleARN)
	if accountID == "" {
		return "", fmt.Errorf("cannot determine AWS account ID from role ARN %q for overlay ECR repo (set SOCKERLESS_LAMBDA_OVERLAY_ECR_REPO to override)", s.config.RoleARN)
	}
	return awscommon.ECRRegistryHost(accountID, s.config.Region, s.config.ECRRegistryEndpoint) + "/sockerless-live-lambda", nil
}

// overlayIdentity is what an overlay image's content depends on besides
// the user's image and command: the two binaries it carries and the
// platform it is built for.
type overlayIdentity struct {
	agentHash     string
	bootstrapHash string
	platform      string
}

// overlayBinaryIdentity hashes the agent and bootstrap binaries the
// overlay copies in and names the platform of the functions it runs on.
func (s *Server) overlayBinaryIdentity() (overlayIdentity, error) {
	agentHash, err := core.HashBootstrapBinary(s.config.AgentBinaryPath)
	if err != nil {
		return overlayIdentity{}, fmt.Errorf("read agent binary %s (SOCKERLESS_AGENT_BINARY): %w", s.config.AgentBinaryPath, err)
	}
	bootstrapHash, err := core.HashBootstrapBinary(s.config.BootstrapBinaryPath)
	if err != nil {
		return overlayIdentity{}, fmt.Errorf("read bootstrap binary %s (SOCKERLESS_LAMBDA_BOOTSTRAP): %w", s.config.BootstrapBinaryPath, err)
	}
	return overlayIdentity{
		agentHash:     agentHash,
		bootstrapHash: bootstrapHash,
		platform:      "linux/" + dockerArchFromLambda(s.config.Architecture),
	}, nil
}
