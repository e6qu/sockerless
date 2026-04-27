package ecs

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	ecrtypes "github.com/aws/aws-sdk-go-v2/service/ecr/types"
)

// resolveImageURI converts a Docker image reference to a registry that
// Fargate can pull from. The routing rules:
//
//  1. Already-ECR URIs (`<account>.dkr.ecr.<region>.amazonaws.com/...`)
//     pass through unchanged.
//  2. Already-public-gallery URIs (`public.ecr.aws/...`) pass through —
//     pullable by Fargate with no authentication.
//  3. Docker Hub library refs (`alpine`, `node:20`, `nginx:alpine` —
//     i.e. the official-image namespace) get rewritten to the AWS
//     Public Gallery Docker mirror at
//     `public.ecr.aws/docker/library/<name>:<tag>`. AWS hosts an
//     official mirror; no credentials needed.
//  4. Other public registries (`ghcr.io/...`, `quay.io/...`, etc.) get
//     routed through an ECR pull-through cache rule that points at the
//     upstream — none of these require credentials when caching public
//     images. Fargate then pulls the cached copy from ECR.
//  5. Docker Hub user/org refs (`myorg/myapp`) are rejected with a clear
//     error because (a) AWS Public Gallery only mirrors `library/` and
//     (b) Docker Hub user-image pull-through needs Docker Hub PAT-backed
//     auth which the project's no-credentials-on-disk discipline avoids
//     by design. Operators who need such an image should `docker push`
//     it to their ECR repo first.
func (s *Server) resolveImageURI(ctx context.Context, ref string) (string, error) {
	if strings.Contains(ref, ".dkr.ecr.") && strings.Contains(ref, ".amazonaws.com") {
		return ref, nil
	}
	if strings.HasPrefix(ref, "public.ecr.aws/") {
		return ref, nil
	}

	registry, repo, tag := parseDockerRef(ref)

	switch registry {
	case "", "docker.io", "registry-1.docker.io":
		// Docker Hub. Library images are mirrored on AWS Public Gallery
		// at `public.ecr.aws/docker/library/<name>`; user/org images
		// aren't.
		if strings.Contains(repo, "/") {
			return ref, fmt.Errorf("docker hub user/org image %q is not on AWS Public Gallery; push it to your ECR repository first (sockerless avoids Docker Hub PAT credentials by design — use `docker push <ecr-uri>` to host the image yourself, or reference its public.ecr.aws equivalent if one exists)", ref)
		}
		mirrored := fmt.Sprintf("public.ecr.aws/docker/library/%s:%s", repo, tag)
		s.Logger.Debug().Str("original", ref).Str("public", mirrored).Msg("resolved Docker Hub library ref via AWS Public Gallery")
		return mirrored, nil
	}

	// Other registries (ghcr.io, quay.io, k8s.gcr.io, registry.k8s.io,
	// etc.). Route through an ECR pull-through cache rule. Unauthenticated
	// upstreams (which is most public-image registries) don't need any
	// CredentialArn — the rule just needs to exist.
	cachePrefix := strings.ReplaceAll(registry, ".", "-")
	upstreamKind := upstreamRegistryFor(registry)

	accountID := extractAccountID(s.config.ExecutionRoleARN)
	if accountID == "" {
		return ref, fmt.Errorf("cannot determine AWS account ID from ExecutionRoleARN %q", s.config.ExecutionRoleARN)
	}

	if err := s.ensurePullThroughCache(ctx, cachePrefix, registry, upstreamKind); err != nil {
		return ref, fmt.Errorf("ECR pull-through cache setup for %q: %w", cachePrefix, err)
	}

	ecrURI := fmt.Sprintf("%s.dkr.ecr.%s.amazonaws.com/%s/%s:%s", accountID, s.config.Region, cachePrefix, repo, tag)
	s.Logger.Debug().Str("original", ref).Str("ecr", ecrURI).Msg("resolved image to ECR pull-through cache URI")
	return ecrURI, nil
}

// upstreamRegistryFor maps a hostname to ECR's UpstreamRegistry enum.
// Used for non-Docker-Hub registries; Docker Hub is handled inline by
// rewriting to AWS Public Gallery.
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
		// AWS rejects unknown upstream registries at create time with a
		// clear error; let the API surface it rather than guessing.
		return ecrtypes.UpstreamRegistryEcrPublic
	}
}

// ensurePullThroughCache creates an ECR pull-through cache rule if one
// doesn't already exist for the given prefix + upstream pair. Idempotent
// on repeated calls. No CredentialArn is set — sockerless only routes
// public registries through pull-through cache (Docker Hub library refs
// go via AWS Public Gallery directly, see resolveImageURI). Operators
// who need authenticated upstreams should provision the rule and the
// secret out of band; sockerless will pick up the existing rule via the
// describe call above and use it as-is.
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

// parseDockerRef splits a Docker image reference into registry, repo,
// and tag. Matches the Lambda backend's helper exactly (shared shape
// pending a move to aws-common).
func parseDockerRef(ref string) (registry, repo, tag string) {
	tag = "latest"
	if i := strings.IndexByte(ref, '/'); i > 0 {
		prefix := ref[:i]
		if strings.ContainsAny(prefix, ".:") {
			registry = prefix
			ref = ref[i+1:]
		}
	}
	if i := strings.LastIndexByte(ref, ':'); i > 0 {
		repo = ref[:i]
		tag = ref[i+1:]
	} else {
		repo = ref
	}
	return
}

// extractAccountID returns the AWS account ID from an IAM ARN.
// "arn:aws:iam::123456789012:role/name" → "123456789012".
func extractAccountID(arn string) string {
	parts := strings.Split(arn, ":")
	if len(parts) >= 5 {
		return parts[4]
	}
	return ""
}
