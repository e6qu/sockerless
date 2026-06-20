package azurecommon

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerregistry/armcontainerregistry"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/rs/zerolog"
	core "github.com/sockerless/backend-core"
)

// Compile-time check.
var _ core.CloudBuildService = (*ACRBuildService)(nil)

// ACRBuildService builds Docker images using Azure Container Registry Tasks.
type ACRBuildService struct {
	acr              *armcontainerregistry.RegistriesClient
	runs             *armcontainerregistry.RunsClient
	blobClient       *azblob.Client
	blobEndpoint     string // resolved blob service URL (public account URL or the account's ARM-advertised endpoint)
	subscriptionID   string
	resourceGroup    string
	acrName          string
	storageAccount   string
	containerName    string // blob container for context upload
	cred             azcore.TokenCredential
	endpointURL      string             // custom/simulated Azure endpoint, "" for the public cloud
	armOpts          *arm.ClientOptions // nil for the public cloud
	registryEndpoint string             // overrides <acrName>.azurecr.io (sovereign/custom/simulated registry)
	logger           zerolog.Logger
}

// NewACRBuildService creates an ACR Tasks-backed build service.
// Returns nil if required params are empty. When endpointURL is set (a
// custom/sovereign cloud or the local simulator), every Azure client is
// pointed at it: the ARM clients via a cloud.Configuration override, and
// the blob client at the storage account's advertised endpoint (discovered
// lazily on first build — see ensureBlobClient). With endpointURL empty the
// public-cloud well-known endpoints are used.
func NewACRBuildService(cred azcore.TokenCredential, subscriptionID, resourceGroup, acrName, storageAccount, containerName, endpointURL string, logger zerolog.Logger) (*ACRBuildService, error) {
	if acrName == "" || storageAccount == "" {
		return nil, nil
	}

	var armOpts *arm.ClientOptions
	if endpointURL != "" {
		armOpts = &arm.ClientOptions{
			ClientOptions: azcore.ClientOptions{
				Cloud: cloud.Configuration{
					Services: map[cloud.ServiceName]cloud.ServiceConfiguration{
						cloud.ResourceManager: {
							Endpoint: endpointURL,
							Audience: "https://management.azure.com/",
						},
					},
				},
				InsecureAllowCredentialWithHTTP: true,
			},
		}
	}

	regClient, err := armcontainerregistry.NewRegistriesClient(subscriptionID, cred, armOpts)
	if err != nil {
		return nil, fmt.Errorf("create ACR registries client: %w", err)
	}

	runsClient, err := armcontainerregistry.NewRunsClient(subscriptionID, cred, armOpts)
	if err != nil {
		return nil, fmt.Errorf("create ACR runs client: %w", err)
	}

	if containerName == "" {
		containerName = "build-context"
	}

	s := &ACRBuildService{
		acr:              regClient,
		runs:             runsClient,
		subscriptionID:   subscriptionID,
		resourceGroup:    resourceGroup,
		acrName:          acrName,
		storageAccount:   storageAccount,
		containerName:    containerName,
		cred:             cred,
		endpointURL:      endpointURL,
		armOpts:          armOpts,
		registryEndpoint: os.Getenv("SOCKERLESS_AZURE_ACR_ENDPOINT"),
		logger:           logger,
	}

	// Public cloud: the blob endpoint is the well-known account URL, so the
	// client can be built eagerly. For a custom endpoint the blob service
	// URL is whatever the account advertises (e.g. the simulator's
	// host-routed endpoint), discovered on first build once the account is
	// guaranteed to exist.
	if endpointURL == "" {
		blobURL := fmt.Sprintf("https://%s.blob.core.windows.net", storageAccount)
		blobClient, err := azblob.NewClient(blobURL, cred, nil)
		if err != nil {
			return nil, fmt.Errorf("create blob client: %w", err)
		}
		s.blobClient = blobClient
		s.blobEndpoint = blobURL
	}

	return s, nil
}

// ensureBlobClient lazily resolves the blob data-plane client for a custom
// endpoint by reading the storage account's advertised primaryEndpoints.blob
// via ARM — the faithful way to reach storage on a sovereign/custom/simulated
// cloud, where the public *.blob.core.windows.net suffix does not apply.
func (s *ACRBuildService) ensureBlobClient(ctx context.Context) error {
	if s.blobClient != nil {
		return nil
	}
	acctClient, err := armstorage.NewAccountsClient(s.subscriptionID, s.cred, s.armOpts)
	if err != nil {
		return fmt.Errorf("create storage accounts client: %w", err)
	}
	props, err := acctClient.GetProperties(ctx, s.resourceGroup, s.storageAccount, nil)
	if err != nil {
		return fmt.Errorf("get storage account %s: %w", s.storageAccount, err)
	}
	if props.Properties == nil || props.Properties.PrimaryEndpoints == nil || props.Properties.PrimaryEndpoints.Blob == nil {
		return fmt.Errorf("storage account %s advertises no blob endpoint", s.storageAccount)
	}
	blobEndpoint := *props.Properties.PrimaryEndpoints.Blob
	// No-credential client: the custom/simulator endpoint does not enforce
	// storage bearer auth, and azblob rejects bearer tokens over plain HTTP.
	blobClient, err := azblob.NewClientWithNoCredential(blobEndpoint, nil)
	if err != nil {
		return fmt.Errorf("create blob client for %s: %w", blobEndpoint, err)
	}
	s.blobClient = blobClient
	s.blobEndpoint = blobEndpoint
	return nil
}

func (s *ACRBuildService) Available() bool {
	return s.acrName != "" && s.storageAccount != ""
}

// AssembleMultiArchManifest delegates to the universal helper, with
// the ACR bearer token (mints via the registry's `/oauth2/token`
// exchange of an AAD access token). ACR honours the standard OCI
// distribution v2 PUT /v2/<repo>/manifests/<tag> request.
func (s *ACRBuildService) AssembleMultiArchManifest(ctx context.Context, opts core.MultiArchManifestOptions) error {
	return core.AssembleMultiArchManifest(ctx, opts, func(_ string) (string, error) {
		tok, err := s.cred.GetToken(ctx, policy.TokenRequestOptions{
			Scopes: []string{"https://management.azure.com/.default"},
		})
		if err != nil {
			return "", fmt.Errorf("AAD token: %w", err)
		}
		// ACR's OCI v2 endpoint accepts AAD bearer tokens directly
		// when the caller has AcrPush role. The /oauth2/token
		// exchange (AAD → ACR refresh token → ACR access token) is
		// only required for tools that need long-lived creds.
		return tok.Token, nil
	})
}

func (s *ACRBuildService) Build(ctx context.Context, opts core.CloudBuildOptions) (*core.CloudBuildResult, error) {
	start := time.Now()

	if err := s.ensureBlobClient(ctx); err != nil {
		return nil, err
	}

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

	// Build the ACR-Task source URL from the SAME blob endpoint the context
	// was uploaded through (the account's ARM-advertised endpoint on a
	// custom/sim cloud), not a re-hardcoded `.blob.core.windows.net` the
	// build environment can't reach there.
	sourceURL := fmt.Sprintf("%s/%s/%s", strings.TrimRight(s.blobEndpoint, "/"), s.containerName, blobName)

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
	// The registry endpoint is normally `<acrName>.azurecr.io`. An operator
	// may override it (sovereign clouds with a different suffix, or a
	// reachable local/simulated registry) via SOCKERLESS_AZURE_ACR_ENDPOINT;
	// the image is then built, pushed, and pulled at that host.
	registryHost := s.acrName + ".azurecr.io"
	if s.registryEndpoint != "" {
		registryHost = s.registryEndpoint
	}
	imageName := fmt.Sprintf("%s/%s:%s", registryHost, repo, tag)

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

	if result.Run.Properties == nil || result.Run.Properties.Status == nil {
		return nil, fmt.Errorf("ACR Task: poller returned no run status")
	}
	if *result.Run.Properties.Status != armcontainerregistry.RunStatusSucceeded {
		return nil, fmt.Errorf("ACR Task %s", *result.Run.Properties.Status)
	}
	runID := ""
	if result.Run.Properties.RunID != nil {
		runID = *result.Run.Properties.RunID
	}

	s.logger.Info().Str("image", imageName).Str("runID", runID).Dur("duration", time.Since(start)).Msg("ACR Task succeeded")

	return &core.CloudBuildResult{
		ImageRef:  imageName,
		ImageID:   "",
		Duration:  time.Since(start),
		LogStream: runID,
	}, nil
}
