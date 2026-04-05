package azurecommon

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerregistry/armcontainerregistry"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/rs/zerolog"
	core "github.com/sockerless/backend-core"
)

// Compile-time check.
var _ core.CloudBuildService = (*ACRBuildService)(nil)

// ACRBuildService builds Docker images using Azure Container Registry Tasks.
type ACRBuildService struct {
	acr            *armcontainerregistry.RegistriesClient
	runs           *armcontainerregistry.RunsClient
	blobClient     *azblob.Client
	subscriptionID string
	resourceGroup  string
	acrName        string
	storageAccount string
	containerName  string // blob container for context upload
	logger         zerolog.Logger
}

// NewACRBuildService creates an ACR Tasks-backed build service.
// Returns nil if required params are empty.
func NewACRBuildService(cred azcore.TokenCredential, subscriptionID, resourceGroup, acrName, storageAccount, containerName string, logger zerolog.Logger) (*ACRBuildService, error) {
	if acrName == "" || storageAccount == "" {
		return nil, nil
	}

	regClient, err := armcontainerregistry.NewRegistriesClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("create ACR registries client: %w", err)
	}

	runsClient, err := armcontainerregistry.NewRunsClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("create ACR runs client: %w", err)
	}

	blobURL := fmt.Sprintf("https://%s.blob.core.windows.net", storageAccount)
	blobClient, err := azblob.NewClient(blobURL, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("create blob client: %w", err)
	}

	if containerName == "" {
		containerName = "build-context"
	}

	return &ACRBuildService{
		acr:            regClient,
		runs:           runsClient,
		blobClient:     blobClient,
		subscriptionID: subscriptionID,
		resourceGroup:  resourceGroup,
		acrName:        acrName,
		storageAccount: storageAccount,
		containerName:  containerName,
		logger:         logger,
	}, nil
}

func (s *ACRBuildService) Available() bool {
	return s.acrName != "" && s.storageAccount != ""
}

func (s *ACRBuildService) Build(ctx context.Context, opts core.CloudBuildOptions) (*core.CloudBuildResult, error) {
	start := time.Now()

	// Upload context to blob storage
	var contextBuf bytes.Buffer
	if _, err := io.Copy(&contextBuf, opts.ContextTar); err != nil {
		return nil, fmt.Errorf("read build context: %w", err)
	}

	blobName := fmt.Sprintf("build-context/%d.tar.gz", time.Now().UnixNano())
	_, err := s.blobClient.UploadBuffer(ctx, s.containerName, blobName, contextBuf.Bytes(), nil)
	if err != nil {
		return nil, fmt.Errorf("upload context to blob storage: %w", err)
	}

	sourceURL := fmt.Sprintf("https://%s.blob.core.windows.net/%s/%s", s.storageAccount, s.containerName, blobName)

	// Determine target image name
	tag := "latest"
	repo := "sockerless"
	if len(opts.Tags) > 0 {
		t := opts.Tags[0]
		if idx := strings.LastIndex(t, ":"); idx >= 0 {
			repo = t[:idx]
			tag = t[idx+1:]
		} else {
			repo = t
		}
	}
	imageName := fmt.Sprintf("%s.azurecr.io/%s:%s", s.acrName, repo, tag)

	// Build arguments
	var arguments []*armcontainerregistry.Argument
	for k, v := range opts.BuildArgs {
		arguments = append(arguments, &armcontainerregistry.Argument{
			Name:     to.Ptr(k),
			Value:    to.Ptr(v),
			IsSecret: to.Ptr(false),
		})
	}
	for k, v := range opts.Secrets {
		arguments = append(arguments, &armcontainerregistry.Argument{
			Name:     to.Ptr(k),
			Value:    to.Ptr(v),
			IsSecret: to.Ptr(true),
		})
	}

	dockerfile := opts.Dockerfile
	if dockerfile == "" {
		dockerfile = "Dockerfile"
	}

	// Schedule ACR Task run
	runRequest := &armcontainerregistry.DockerBuildRequest{
		Type:           to.Ptr("DockerBuildRequest"),
		DockerFilePath: to.Ptr(dockerfile),
		ImageNames:     []*string{to.Ptr(imageName)},
		SourceLocation: to.Ptr(sourceURL),
		IsPushEnabled:  to.Ptr(true),
		NoCache:        to.Ptr(opts.NoCache),
		Arguments:      arguments,
	}
	if opts.Target != "" {
		runRequest.Target = to.Ptr(opts.Target)
	}
	if opts.Platform != "" {
		parts := strings.SplitN(opts.Platform, "/", 2)
		os := parts[0]
		arch := ""
		if len(parts) > 1 {
			arch = parts[1]
		}
		runRequest.Platform = &armcontainerregistry.PlatformProperties{
			OS:           (*armcontainerregistry.OS)(to.Ptr(os)),
			Architecture: (*armcontainerregistry.Architecture)(to.Ptr(arch)),
		}
	}

	poller, err := s.acr.BeginScheduleRun(ctx, s.resourceGroup, s.acrName, runRequest, nil)
	if err != nil {
		return nil, fmt.Errorf("schedule ACR run: %w", err)
	}

	s.logger.Info().Str("acr", s.acrName).Str("image", imageName).Msg("ACR Task started")

	// Wait for completion
	result, err := poller.PollUntilDone(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("ACR Task failed: %w", err)
	}

	runID := ""
	if result.Run.Properties != nil {
		if result.Run.Properties.Status != nil && *result.Run.Properties.Status != armcontainerregistry.RunStatusSucceeded {
			return nil, fmt.Errorf("ACR Task %s", *result.Run.Properties.Status)
		}
		if result.Run.Properties.RunID != nil {
			runID = *result.Run.Properties.RunID
		}
	}

	s.logger.Info().Str("image", imageName).Str("runID", runID).Dur("duration", time.Since(start)).Msg("ACR Task succeeded")

	return &core.CloudBuildResult{
		ImageRef:  imageName,
		ImageID:   "",
		Duration:  time.Since(start),
		LogStream: runID,
	}, nil
}
