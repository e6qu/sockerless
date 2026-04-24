package ecs

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
func dockerHubCredentialARN() string {
	return os.Getenv("SOCKERLESS_ECR_DOCKERHUB_CREDENTIAL_ARN")
}

// resolveImageURI converts a Docker image reference to an ECR URI that
// Fargate can pull. Short references like `alpine` or `node:20` are
// rewritten to the account's ECR pull-through cache, which fetches from
// Docker Hub (or another upstream) on first request. Already-ECR URIs
// are returned unchanged.
// Examples:
// - "alpine:latest" → "<account>.dkr.ecr.<region>.amazonaws.com/docker-hub/library/alpine:latest"
// - "node:20" → "<account>.dkr.ecr.<region>.amazonaws.com/docker-hub/library/node:20"
// - "ghcr.io/owner/repo:v1" → "<account>.dkr.ecr.<region>.amazonaws.com/ghcr-io/owner/repo:v1"
// - "<account>.dkr.ecr.<region>.amazonaws.com/repo:tag" → used as-is
// Returns an error (not a silent fallback) if pull-through cache
// setup fails, so the operator sees the real failure (e.g. missing
// docker-hub credential ARN — see.
func (s *Server) resolveImageURI(ctx context.Context, ref string) (string, error) {
	if strings.Contains(ref, ".dkr.ecr.") && strings.Contains(ref, ".amazonaws.com") {
		return ref, nil
	}
	// ECR Public (`public.ecr.aws/...`) is pullable by Fargate with no
	// authentication. Pass through without rewriting through a
	// pull-through cache rule.
	if strings.HasPrefix(ref, "public.ecr.aws/") {
		return ref, nil
	}

	registry, repo, tag := parseDockerRef(ref)

	var cachePrefix, upstreamURL string
	var upstreamKind ecrtypes.UpstreamRegistry
	switch registry {
	case "", "docker.io", "registry-1.docker.io":
		cachePrefix = "docker-hub"
		upstreamURL = "registry-1.docker.io"
		upstreamKind = ecrtypes.UpstreamRegistryDockerHub
		if !strings.Contains(repo, "/") {
			repo = "library/" + repo
		}
	default:
		cachePrefix = strings.ReplaceAll(registry, ".", "-")
		upstreamURL = registry
		// Default to DockerHub-style upstream; ECR also supports ghcr, quay,
		// Microsoft and others via distinct UpstreamRegistry values. For
		// unknown registries the caller will see a clear ECR error on the
		// first pull attempt rather than a silent misroute.
		upstreamKind = ecrtypes.UpstreamRegistryDockerHub
	}

	accountID := extractAccountID(s.config.ExecutionRoleARN)
	if accountID == "" {
		return ref, fmt.Errorf("cannot determine AWS account ID from ExecutionRoleARN %q", s.config.ExecutionRoleARN)
	}

	if err := s.ensurePullThroughCache(ctx, cachePrefix, upstreamURL, upstreamKind); err != nil {
		return ref, fmt.Errorf("ECR pull-through cache setup for %q: %w", cachePrefix, err)
	}

	ecrURI := fmt.Sprintf("%s.dkr.ecr.%s.amazonaws.com/%s/%s:%s", accountID, s.config.Region, cachePrefix, repo, tag)
	s.Logger.Debug().Str("original", ref).Str("ecr", ecrURI).Msg("resolved image to ECR pull-through cache URI")
	return ecrURI, nil
}

// ensurePullThroughCache creates an ECR pull-through cache rule if
// one doesn't already exist for the given prefix + upstream pair.
// Idempotent on repeated calls. For docker-hub upstream,
// AWS now requires a Secrets Manager `CredentialArn` containing the
// upstream registry credentials — read from
// `SOCKERLESS_ECR_DOCKERHUB_CREDENTIAL_ARN`. Returns an explicit
// error when the credential is needed but not configured.
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
	// Docker Hub upstream rules require a Secrets Manager credential
	// ARN on real AWS; wire it through when the operator has set one.
	// The simulator accepts either shape, so we let the actual API
	// decide rather than pre-checking — on real AWS the caller gets a
	// wrapped error naming the env var; on the simulator the call
	// succeeds and the tests run.
	if upstreamKind == ecrtypes.UpstreamRegistryDockerHub && upstreamURL == "registry-1.docker.io" {
		if credARN := dockerHubCredentialARN(); credARN != "" {
			in.CredentialArn = aws.String(credARN)
		}
	}

	if _, err := s.aws.ECR.CreatePullThroughCacheRule(ctx, in); err != nil {
		if strings.Contains(err.Error(), "PullThroughCacheRuleAlreadyExists") {
			return nil
		}
		if upstreamKind == ecrtypes.UpstreamRegistryDockerHub && upstreamURL == "registry-1.docker.io" && dockerHubCredentialARN() == "" && strings.Contains(strings.ToLower(err.Error()), "credential") {
			return fmt.Errorf("docker-hub pull-through cache requires SOCKERLESS_ECR_DOCKERHUB_CREDENTIAL_ARN (Secrets Manager ARN with {username, accessToken} JSON): %w", err)
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
