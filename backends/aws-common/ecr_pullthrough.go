package awscommon

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	ecrtypes "github.com/aws/aws-sdk-go-v2/service/ecr/types"
	"github.com/rs/zerolog"
)

// ECRPullThroughCache routes public-registry images through Amazon
// Elastic Container Registry pull-through cache rules so an AWS workload
// pulls from ECR. No credential is attached to a rule: sockerless routes
// only public upstreams this way, and an operator who needs an
// authenticated upstream provisions the rule and its secret out of band,
// which Ensure then adopts as-is.
type ECRPullThroughCache struct {
	Client *ecr.Client
	Logger zerolog.Logger
}

// Ensure creates the pull-through cache rule for prefix → upstreamURL if
// it does not exist. Idempotent.
func (c ECRPullThroughCache) Ensure(ctx context.Context, prefix, upstreamURL string, upstreamKind ecrtypes.UpstreamRegistry) error {
	rules, err := c.Client.DescribePullThroughCacheRules(ctx, &ecr.DescribePullThroughCacheRulesInput{
		EcrRepositoryPrefixes: []string{prefix},
	})
	if err == nil && len(rules.PullThroughCacheRules) > 0 {
		return nil
	}
	_, err = c.Client.CreatePullThroughCacheRule(ctx, &ecr.CreatePullThroughCacheRuleInput{
		EcrRepositoryPrefix: aws.String(prefix),
		UpstreamRegistryUrl: aws.String(upstreamURL),
		UpstreamRegistry:    upstreamKind,
	})
	if err != nil {
		if strings.Contains(err.Error(), "PullThroughCacheRuleAlreadyExists") {
			return nil
		}
		return fmt.Errorf("create pull-through cache rule: %w", err)
	}
	c.Logger.Info().Str("prefix", prefix).Str("upstream", upstreamURL).Msg("created ECR pull-through cache rule")
	return nil
}

// UpstreamRegistryFor maps a registry hostname to ECR's UpstreamRegistry
// enumeration. A hostname ECR does not model is passed as ECR Public;
// ECR rejects an unknown upstream at rule creation with its own error
// rather than sockerless guessing.
func UpstreamRegistryFor(registry string) ecrtypes.UpstreamRegistry {
	switch registry {
	case "ghcr.io":
		return ecrtypes.UpstreamRegistryGitHubContainerRegistry
	case "quay.io":
		return ecrtypes.UpstreamRegistryQuay
	case "registry.k8s.io", "k8s.gcr.io":
		return ecrtypes.UpstreamRegistryK8s
	case "mcr.microsoft.com":
		return ecrtypes.UpstreamRegistryAzureContainerRegistry
	default:
		return ecrtypes.UpstreamRegistryEcrPublic
	}
}

// PullThroughCachePrefix is the ECR repository prefix a cache rule for the
// registry uses: the hostname with dots replaced by hyphens.
func PullThroughCachePrefix(registry string) string {
	return strings.ReplaceAll(registry, ".", "-")
}

// PullThroughCacheURI is the ECR image URI a workload pulls a cached
// image from.
func PullThroughCacheURI(accountID, region, prefix, repo, tag string) string {
	return fmt.Sprintf("%s.dkr.ecr.%s.amazonaws.com/%s/%s:%s", accountID, region, prefix, repo, tag)
}

// ExtractAccountID returns the account ID from an IAM ARN
// (`arn:aws:iam::123456789012:role/name` → `123456789012`), or the empty
// string when the ARN does not carry one.
func ExtractAccountID(arn string) string {
	parts := strings.Split(arn, ":")
	if len(parts) >= 5 {
		return parts[4]
	}
	return ""
}

// IsECRImageURI reports whether ref already names a private ECR repository.
func IsECRImageURI(ref string) bool {
	return strings.Contains(ref, ".dkr.ecr.") && strings.Contains(ref, ".amazonaws.com")
}
