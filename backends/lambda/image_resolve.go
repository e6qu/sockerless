package lambda

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	ecrtypes "github.com/aws/aws-sdk-go-v2/service/ecr/types"
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
	if strings.Contains(ref, ".dkr.ecr.") && strings.Contains(ref, ".amazonaws.com") {
		return ref, nil
	}

	// Decompose the reference. For public.ecr.aws/<path> we treat it
	// the same as any registry-prefixed ref so it goes through a
	// pull-through cache (Lambda needs single-arch ECR-hosted images).
	registry, repo, tag := parseDockerRef(ref)

	var cachePrefix string
	var upstreamURL string
	var upstreamKind ecrtypes.UpstreamRegistry

	switch registry {
	case "", "docker.io", "registry-1.docker.io":
		// Library images live on AWS Public Gallery as `docker/library/<name>`.
		if strings.Contains(repo, "/") {
			return ref, fmt.Errorf("docker hub user/org image %q is not on AWS Public Gallery; push it to your ECR repository first (sockerless avoids Docker Hub PAT credentials by design — use `docker push <ecr-uri>` to host the image yourself, or reference its public.ecr.aws equivalent if one exists)", ref)
		}
		upstreamURL = "public.ecr.aws"
		upstreamKind = ecrtypes.UpstreamRegistryEcrPublic
		repo = "docker/library/" + repo
		cachePrefix = strings.ReplaceAll(upstreamURL, ".", "-")
	case "public.ecr.aws":
		upstreamURL = "public.ecr.aws"
		upstreamKind = ecrtypes.UpstreamRegistryEcrPublic
		cachePrefix = strings.ReplaceAll(upstreamURL, ".", "-")
	default:
		cachePrefix = strings.ReplaceAll(registry, ".", "-")
		upstreamURL = registry
		upstreamKind = upstreamRegistryFor(registry)
	}

	if err := s.ensurePullThroughCache(ctx, cachePrefix, upstreamURL, upstreamKind); err != nil {
		return ref, fmt.Errorf("ECR pull-through cache setup for %q: %w", cachePrefix, err)
	}

	accountID := extractAccountID(s.config.RoleARN)
	if accountID == "" {
		return "", fmt.Errorf("cannot determine AWS account ID from role ARN %q", s.config.RoleARN)
	}

	ecrURI := fmt.Sprintf("%s.dkr.ecr.%s.amazonaws.com/%s/%s:%s", accountID, s.config.Region, cachePrefix, repo, tag)
	s.Logger.Info().Str("original", ref).Str("ecr", ecrURI).Msg("resolved image to ECR pull-through cache URI")
	return ecrURI, nil
}

// upstreamRegistryFor maps a hostname to ECR's UpstreamRegistry enum.
func upstreamRegistryFor(registry string) ecrtypes.UpstreamRegistry {
	switch registry {
	case "ghcr.io":
		return ecrtypes.UpstreamRegistryGitHubContainerRegistry
	case "quay.io":
		return ecrtypes.UpstreamRegistryQuay
	case "registry.k8s.io", "k8s.gcr.io":
		return ecrtypes.UpstreamRegistryK8s
	case "mcr.microsoft.com":
		return ecrtypes.UpstreamRegistryAzureContainerRegistry
	case "public.ecr.aws":
		return ecrtypes.UpstreamRegistryEcrPublic
	default:
		return ecrtypes.UpstreamRegistryEcrPublic
	}
}

// ensurePullThroughCache creates an ECR pull-through cache rule if it
// doesn't exist. No CredentialArn is set — sockerless only routes
// public registries through pull-through cache (Docker Hub library
// refs go via AWS Public Gallery, see resolveImageURI). Operators who
// need authenticated upstreams should provision the rule and the
// secret out of band; sockerless will pick up the existing rule via
// the describe call above and use it as-is.
func (s *Server) ensurePullThroughCache(ctx context.Context, prefix, upstreamURL string, upstreamKind ecrtypes.UpstreamRegistry) error {
	rules, err := s.aws.ECR.DescribePullThroughCacheRules(ctx, &ecr.DescribePullThroughCacheRulesInput{
		EcrRepositoryPrefixes: []string{prefix},
	})
	if err == nil && len(rules.PullThroughCacheRules) > 0 {
		return nil
	}

	in := &ecr.CreatePullThroughCacheRuleInput{
		EcrRepositoryPrefix: aws.String(prefix),
		UpstreamRegistryUrl: aws.String(upstreamURL),
		UpstreamRegistry:    upstreamKind,
	}
	if _, err := s.aws.ECR.CreatePullThroughCacheRule(ctx, in); err != nil {
		if strings.Contains(err.Error(), "PullThroughCacheRuleAlreadyExists") {
			return nil
		}
		return fmt.Errorf("create pull-through cache rule: %w", err)
	}
	s.Logger.Info().Str("prefix", prefix).Str("upstream", upstreamURL).Msg("created ECR pull-through cache rule")
	return nil
}

// parseDockerRef splits "nginx:alpine" into ("", "nginx", "alpine").
func parseDockerRef(ref string) (registry, repo, tag string) {
	tag = "latest"

	// Check for explicit registry (contains. or:port before /)
	if i := strings.IndexByte(ref, '/'); i > 0 {
		prefix := ref[:i]
		if strings.ContainsAny(prefix, ".:") {
			registry = prefix
			ref = ref[i+1:]
		}
	}

	// Split repo:tag
	if i := strings.LastIndexByte(ref, ':'); i > 0 {
		repo = ref[:i]
		tag = ref[i+1:]
	} else {
		repo = ref
	}
	return
}

// extractAccountID returns the AWS account ID from an IAM ARN.
// "arn:aws:iam::123456789012:role/name" → "123456789012"
func extractAccountID(arn string) string {
	parts := strings.Split(arn, ":")
	if len(parts) >= 5 {
		return parts[4]
	}
	return ""
}
