// Package awscommon provides shared AWS utilities for ECS and Lambda backends.
package awscommon

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	ecrtypes "github.com/aws/aws-sdk-go-v2/service/ecr/types"
	"github.com/rs/zerolog"
	core "github.com/sockerless/backend-core"
)

// Compile-time check that ECRAuthProvider implements core.AuthProvider.
var _ core.AuthProvider = (*ECRAuthProvider)(nil)

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

// OnPush creates an ECR repository (if needed) and pushes the image via OCI registry v2 API.
// Failures are returned to the caller (ImageManager) which aggregates
// them and surfaces via HTTP error so the operator can rerun rmi/push
// or inspect the cloud-side state. Per the project's no-fallbacks
// rule, these are not silent warnings.
func (p *ECRAuthProvider) OnPush(imageID, registry, repo, tag string) error {
	if !p.IsCloudRegistry(registry) {
		return nil
	}

	// Ensure repository exists (ignore AlreadyExists error)
	_, err := p.ecr.CreateRepository(p.ctx(), &ecr.CreateRepositoryInput{
		RepositoryName: aws.String(repo),
	})
	if err != nil && !isECRAlreadyExistsError(err) {
		p.logger.Warn().Err(err).Str("repo", repo).Msg("ECR CreateRepository failed during push")
		return err
	}

	// CreateRepository is the only ECR-specific bookkeeping needed
	// before the push; the actual blob upload is done by
	// BaseServer.ImagePush via core.OCIPush, which has access to the
	// image's layer data through the local store. OnPush used to also
	// call OCIPush here without layer data, which always failed.
	return nil
}

// OnTag pushes the image with a new tag via OCI registry v2 API.
// Failures are returned to the caller (ImageManager) which aggregates
// them and surfaces via HTTP error so the operator can rerun rmi/push
// or inspect the cloud-side state. Per the project's no-fallbacks
// rule, these are not silent warnings.
func (p *ECRAuthProvider) OnTag(imageID, registry, repo, newTag string) error {
	if !p.IsCloudRegistry(registry) {
		return nil
	}

	// Ensure repository exists
	_, err := p.ecr.CreateRepository(p.ctx(), &ecr.CreateRepositoryInput{
		RepositoryName: aws.String(repo),
	})
	if err != nil && !isECRAlreadyExistsError(err) {
		p.logger.Warn().Err(err).Str("repo", repo).Msg("ECR CreateRepository failed during tag")
		return err
	}

	// OnTag triggers a fresh tag upload on the registry. The actual
	// manifest re-PUT is done by BaseServer.ImagePush — we only need
	// to ensure the repo exists (CreateRepository above).

	return nil
}

// OnRemove removes image tags from ECR via BatchDeleteImage.
// Failures are returned to the caller (ImageManager) which aggregates
// them and surfaces via HTTP error so the operator can rerun rmi/push
// or inspect the cloud-side state. Per the project's no-fallbacks
// rule, these are not silent warnings.
func (p *ECRAuthProvider) OnRemove(registry, repo string, tags []string) error {
	if !p.IsCloudRegistry(registry) {
		return nil
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
		return err
	}

	return nil
}

// isECRAlreadyExistsError checks if an error is an ECR RepositoryAlreadyExistsException.
func isECRAlreadyExistsError(err error) bool {
	return strings.Contains(err.Error(), "RepositoryAlreadyExistsException")
}
