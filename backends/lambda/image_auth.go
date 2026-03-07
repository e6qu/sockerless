package lambda

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	"github.com/rs/zerolog"
)

// ECRAuthProvider handles ECR authentication for the Lambda backend.
type ECRAuthProvider struct {
	ecr    *ecr.Client
	logger zerolog.Logger
	ctx    func() context.Context
}

// NewECRAuthProvider creates a new ECRAuthProvider.
func NewECRAuthProvider(ecrClient *ecr.Client, logger zerolog.Logger, ctxFn func() context.Context) *ECRAuthProvider {
	return &ECRAuthProvider{
		ecr:    ecrClient,
		logger: logger,
		ctx:    ctxFn,
	}
}

// GetToken calls ECR GetAuthorizationToken and returns the raw base64 token
// formatted as "Basic <token>".
func (p *ECRAuthProvider) GetToken(registry string) (string, error) {
	result, err := p.ecr.GetAuthorizationToken(p.ctx(), &ecr.GetAuthorizationTokenInput{})
	if err != nil {
		return "", err
	}
	if len(result.AuthorizationData) == 0 {
		return "", fmt.Errorf("no authorization data returned")
	}

	token := aws.ToString(result.AuthorizationData[0].AuthorizationToken)
	return "Basic " + token, nil
}

// IsCloudRegistry returns true if the registry matches the ECR pattern
// (*.dkr.ecr.*.amazonaws.com).
func (p *ECRAuthProvider) IsCloudRegistry(registry string) bool {
	return strings.HasSuffix(registry, ".amazonaws.com") && strings.Contains(registry, ".dkr.ecr.")
}

// ParseImageRef splits an image reference into registry, repository, and tag.
func ParseImageRef(ref string) (registry, repo, tag string) {
	// Split off tag/digest
	tag = "latest"
	if idx := strings.LastIndex(ref, ":"); idx != -1 {
		// Make sure the colon is not in the registry part (contains /)
		afterColon := ref[idx+1:]
		if !strings.Contains(afterColon, "/") {
			tag = afterColon
			ref = ref[:idx]
		}
	}

	// Split registry from repo
	if strings.Contains(ref, ".") || strings.Contains(ref, ":") {
		parts := strings.SplitN(ref, "/", 2)
		if len(parts) == 2 {
			registry = parts[0]
			repo = parts[1]
			return
		}
	}

	// Docker Hub official images
	registry = "registry-1.docker.io"
	if !strings.Contains(ref, "/") {
		repo = "library/" + ref
	} else {
		repo = ref
	}
	return
}
