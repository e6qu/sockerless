package lambda

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	ecrtypes "github.com/aws/aws-sdk-go-v2/service/ecr/types"
)

// resolveImageURI converts a Docker image reference to an ECR URI that Lambda can use.
//
// Lambda only supports private ECR image URIs. For non-ECR images (e.g., Docker Hub),
// this method ensures an ECR pull-through cache rule exists and rewrites the reference
// to the cache URI. ECR automatically pulls from Docker Hub on first access.
//
// Examples:
//   - "alpine:latest" → "<account>.dkr.ecr.<region>.amazonaws.com/docker-hub/library/alpine:latest"
//   - "nginx:alpine"  → "<account>.dkr.ecr.<region>.amazonaws.com/docker-hub/library/nginx:alpine"
//   - "myorg/app:v1"  → "<account>.dkr.ecr.<region>.amazonaws.com/docker-hub/myorg/app:v1"
//   - "<account>.dkr.ecr.<region>.amazonaws.com/repo:tag" → used as-is
func (s *Server) resolveImageURI(ctx context.Context, ref string) (string, error) {
	// Already an ECR URI — use as-is
	if strings.Contains(ref, ".dkr.ecr.") && strings.Contains(ref, ".amazonaws.com") {
		return ref, nil
	}

	// Parse Docker Hub reference
	registry, repo, tag := parseDockerRef(ref)

	// Determine the pull-through cache prefix based on upstream registry
	var cachePrefix, upstreamURL string
	switch {
	case registry == "" || registry == "docker.io" || registry == "registry-1.docker.io":
		cachePrefix = "docker-hub"
		upstreamURL = "registry-1.docker.io"
		// Docker Hub library images: "alpine" → "library/alpine"
		if !strings.Contains(repo, "/") {
			repo = "library/" + repo
		}
	default:
		// Other registries (ghcr.io, quay.io, etc.)
		cachePrefix = strings.ReplaceAll(registry, ".", "-")
		upstreamURL = "https://" + registry
	}

	// Ensure the pull-through cache rule exists
	if err := s.ensurePullThroughCache(ctx, cachePrefix, upstreamURL); err != nil {
		s.Logger.Warn().Err(err).Str("prefix", cachePrefix).Msg("pull-through cache setup failed, falling back to auto-push")
		// Fall back to pushing the image to ECR
		return s.pushToECR(ctx, ref, repo, tag)
	}

	// Get account ID from role ARN
	accountID := extractAccountID(s.config.RoleARN)
	if accountID == "" {
		return "", fmt.Errorf("cannot determine AWS account ID from role ARN %q", s.config.RoleARN)
	}

	// Construct the pull-through cache URI
	ecrURI := fmt.Sprintf("%s.dkr.ecr.%s.amazonaws.com/%s/%s:%s", accountID, s.config.Region, cachePrefix, repo, tag)
	s.Logger.Info().Str("original", ref).Str("ecr", ecrURI).Msg("resolved image to ECR pull-through cache URI")
	return ecrURI, nil
}

// ensurePullThroughCache creates an ECR pull-through cache rule if it doesn't exist.
func (s *Server) ensurePullThroughCache(ctx context.Context, prefix, upstreamURL string) error {
	// Check if rule already exists
	rules, err := s.aws.ECR.DescribePullThroughCacheRules(ctx, &ecr.DescribePullThroughCacheRulesInput{
		EcrRepositoryPrefixes: []string{prefix},
	})
	if err == nil && len(rules.PullThroughCacheRules) > 0 {
		return nil // already exists
	}

	// Create the rule
	_, err = s.aws.ECR.CreatePullThroughCacheRule(ctx, &ecr.CreatePullThroughCacheRuleInput{
		EcrRepositoryPrefix: aws.String(prefix),
		UpstreamRegistryUrl: aws.String(upstreamURL),
		UpstreamRegistry:    ecrtypes.UpstreamRegistryDockerHub,
	})
	if err != nil {
		// Ignore AlreadyExists — another process may have created it
		if strings.Contains(err.Error(), "PullThroughCacheRuleAlreadyExists") {
			return nil
		}
		return fmt.Errorf("create pull-through cache rule: %w", err)
	}
	s.Logger.Info().Str("prefix", prefix).Str("upstream", upstreamURL).Msg("created ECR pull-through cache rule")
	return nil
}

// pushToECR pushes the image to private ECR as a fallback when pull-through cache is unavailable.
func (s *Server) pushToECR(ctx context.Context, ref, repo, tag string) (string, error) {
	accountID := extractAccountID(s.config.RoleARN)
	if accountID == "" {
		return "", fmt.Errorf("cannot determine AWS account ID")
	}

	ecrRepo := fmt.Sprintf("sockerless/%s", strings.ReplaceAll(repo, "/", "-"))
	ecrURI := fmt.Sprintf("%s.dkr.ecr.%s.amazonaws.com/%s:%s", accountID, s.config.Region, ecrRepo, tag)

	// Use the ImageManager's auth provider to push
	if s.images != nil && s.images.Auth != nil {
		registry := fmt.Sprintf("%s.dkr.ecr.%s.amazonaws.com", accountID, s.config.Region)
		if img, ok := s.Store.ResolveImage(ref); ok {
			if err := s.images.Auth.OnPush(img.ID, registry, ecrRepo, tag); err != nil {
				return "", fmt.Errorf("push to ECR failed: %w", err)
			}
			s.Logger.Info().Str("original", ref).Str("ecr", ecrURI).Msg("pushed image to ECR")
			return ecrURI, nil
		}
	}

	return "", fmt.Errorf("image %q not found in local store — pull it first", ref)
}

// parseDockerRef splits "nginx:alpine" into ("", "nginx", "alpine").
func parseDockerRef(ref string) (registry, repo, tag string) {
	tag = "latest"

	// Check for explicit registry (contains . or :port before /)
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
