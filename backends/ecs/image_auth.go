package ecs

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	ecrtypes "github.com/aws/aws-sdk-go-v2/service/ecr/types"
	"github.com/rs/zerolog"
)

// ECRAuthProvider handles ECR authentication and image lifecycle operations.
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

// OnPush creates an ECR repository (if needed) and records the image via PutImage.
// All failures are non-fatal (logged as warnings).
func (p *ECRAuthProvider) OnPush(imageID, registry, repo, tag string) {
	if !p.IsCloudRegistry(registry) {
		return
	}

	// Ensure repository exists (ignore AlreadyExists error)
	_, err := p.ecr.CreateRepository(p.ctx(), &ecr.CreateRepositoryInput{
		RepositoryName: aws.String(repo),
	})
	if err != nil && !isECRAlreadyExistsError(err) {
		p.logger.Warn().Err(err).Str("repo", repo).Msg("ECR CreateRepository failed during push")
	}

	manifest := fmt.Sprintf(`{"schemaVersion":2,"mediaType":"application/vnd.docker.distribution.manifest.v2+json","config":{"digest":"%s"}}`, imageID)
	_, err = p.ecr.PutImage(p.ctx(), &ecr.PutImageInput{
		RepositoryName: aws.String(repo),
		ImageManifest:  aws.String(manifest),
		ImageTag:       aws.String(tag),
	})
	if err != nil {
		p.logger.Warn().Err(err).Str("repo", repo).Str("tag", tag).Msg("ECR PutImage failed during push")
	}
}

// OnTag records a new tag in ECR via PutImage.
// All failures are non-fatal (logged as warnings).
func (p *ECRAuthProvider) OnTag(imageID, registry, repo, newTag string) {
	if !p.IsCloudRegistry(registry) {
		return
	}

	// Ensure repository exists
	_, err := p.ecr.CreateRepository(p.ctx(), &ecr.CreateRepositoryInput{
		RepositoryName: aws.String(repo),
	})
	if err != nil && !isECRAlreadyExistsError(err) {
		p.logger.Warn().Err(err).Str("repo", repo).Msg("ECR CreateRepository failed during tag")
		return
	}

	manifest := fmt.Sprintf(`{"schemaVersion":2,"mediaType":"application/vnd.docker.distribution.manifest.v2+json","config":{"digest":"%s"}}`, imageID)
	_, err = p.ecr.PutImage(p.ctx(), &ecr.PutImageInput{
		RepositoryName: aws.String(repo),
		ImageManifest:  aws.String(manifest),
		ImageTag:       aws.String(newTag),
	})
	if err != nil {
		p.logger.Warn().Err(err).Str("repo", repo).Str("tag", newTag).Msg("ECR PutImage failed during tag")
	}
}

// OnRemove removes image tags from ECR via BatchDeleteImage.
// All failures are non-fatal (logged as warnings).
func (p *ECRAuthProvider) OnRemove(registry, repo string, tags []string) {
	if !p.IsCloudRegistry(registry) {
		return
	}

	ids := make([]ecrtypes.ImageIdentifier, len(tags))
	for i, t := range tags {
		ids[i] = ecrtypes.ImageIdentifier{ImageTag: aws.String(t)}
	}

	_, err := p.ecr.BatchDeleteImage(p.ctx(), &ecr.BatchDeleteImageInput{
		RepositoryName: aws.String(repo),
		ImageIds:       ids,
	})
	if err != nil {
		p.logger.Warn().Err(err).Str("repo", repo).Msg("ECR BatchDeleteImage failed during remove")
	}
}

// isECRAlreadyExistsError checks if an error is an ECR RepositoryAlreadyExistsException.
func isECRAlreadyExistsError(err error) bool {
	return strings.Contains(err.Error(), "RepositoryAlreadyExistsException")
}

// ParseImageRef splits an image reference into registry, repository, and tag.
// Exported for use by image methods in backend_impl.go.
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
