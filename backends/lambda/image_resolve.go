package lambda

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	ecrtypes "github.com/aws/aws-sdk-go-v2/service/ecr/types"
)

// dockerHubCredentialARN returns the Secrets Manager ARN holding docker-hub
// credentials for ECR pull-through cache, or "" if not configured.
// Secret JSON shape: {"username":"...","accessToken":"..."}
// (sibling of ECS: AWS rejects pull-through cache rules
// for docker-hub upstream without a CredentialArn.
func dockerHubCredentialARN() string {
	return os.Getenv("SOCKERLESS_ECR_DOCKERHUB_CREDENTIAL_ARN")
}

// resolveImageURI converts a Docker image reference to an ECR URI that Lambda can use.
// Lambda only supports private ECR image URIs. For non-ECR images (e.g., Docker Hub),
// this method ensures an ECR pull-through cache rule exists and rewrites the reference
// to the cache URI. ECR automatically pulls from Docker Hub on first access.
// Examples:
// - "alpine:latest" → "<account>.dkr.ecr.<region>.amazonaws.com/docker-hub/library/alpine:latest"
// - "nginx:alpine" → "<account>.dkr.ecr.<region>.amazonaws.com/docker-hub/library/nginx:alpine"
// - "myorg/app:v1" → "<account>.dkr.ecr.<region>.amazonaws.com/docker-hub/myorg/app:v1"
// - "<account>.dkr.ecr.<region>.amazonaws.com/repo:tag" → used as-is
func (s *Server) resolveImageURI(ctx context.Context, ref string) (string, error) {
	// Already an ECR URI — use as-is
	if strings.Contains(ref, ".dkr.ecr.") && strings.Contains(ref, ".amazonaws.com") {
		return ref, nil
	}
	// ECR Public (`public.ecr.aws/...`) is pullable directly without a
	// pull-through cache rule; routing through one yields a multi-arch
	// manifest that Lambda's CreateFunction rejects. Match the ECS
	// short-circuit.
	if strings.HasPrefix(ref, "public.ecr.aws/") {
		return ref, nil
	}

	// Parse Docker Hub reference
	registry, repo, tag := parseDockerRef(ref)

	// Determine the pull-through cache prefix based on upstream registry
	var cachePrefix, upstreamURL string
	switch registry {
	case "", "docker.io", "registry-1.docker.io":
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

	// no silent fallback. If pull-through cache setup fails
	// (most commonly: missing docker-hub CredentialArn — see,
	// surface the real error. The historical pushToECR auto-fallback was
	// removed because it silently swapped the image source without the
	// operator's knowledge — and only worked when the image happened to
	// be pre-loaded in the local store.
	if err := s.ensurePullThroughCache(ctx, cachePrefix, upstreamURL); err != nil {
		return ref, fmt.Errorf("ECR pull-through cache setup for %q: %w", cachePrefix, err)
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

// ensurePullThroughCache creates an ECR pull-through cache rule if it
// doesn't exist. For docker-hub upstream, AWS now requires a
// Secrets Manager CredentialArn — read from
// SOCKERLESS_ECR_DOCKERHUB_CREDENTIAL_ARN. Returns explicit error when
// the credential is needed but not configured.
func (s *Server) ensurePullThroughCache(ctx context.Context, prefix, upstreamURL string) error {
	rules, err := s.aws.ECR.DescribePullThroughCacheRules(ctx, &ecr.DescribePullThroughCacheRulesInput{
		EcrRepositoryPrefixes: []string{prefix},
	})
	if err == nil && len(rules.PullThroughCacheRules) > 0 {
		return nil
	}

	in := &ecr.CreatePullThroughCacheRuleInput{
		EcrRepositoryPrefix: aws.String(prefix),
		UpstreamRegistryUrl: aws.String(upstreamURL),
		UpstreamRegistry:    ecrtypes.UpstreamRegistryDockerHub,
	}
	if upstreamURL == "registry-1.docker.io" {
		credARN := dockerHubCredentialARN()
		if credARN == "" {
			return fmt.Errorf("docker-hub pull-through cache requires SOCKERLESS_ECR_DOCKERHUB_CREDENTIAL_ARN (Secrets Manager ARN with {username, accessToken} JSON) — AWS rejects unauthenticated docker-hub upstream rules")
		}
		in.CredentialArn = aws.String(credARN)
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
