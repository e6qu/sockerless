package azure_sdk_test

import (
	"io"
	"strings"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/containers/azcontainerregistry"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerregistry/armcontainerregistry"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// acrDataPlaneClient creates an azcontainerregistry.Client pointed at the simulator.
func acrDataPlaneClient(t *testing.T) *azcontainerregistry.Client {
	t.Helper()
	opts := &azcontainerregistry.ClientOptions{
		ClientOptions: azcore.ClientOptions{
			InsecureAllowCredentialWithHTTP: true,
		},
	}
	client, err := azcontainerregistry.NewClient(baseURL, &fakeCredential{}, opts)
	require.NoError(t, err)
	return client
}

// sampleManifestJSON is a minimal OCI v2 manifest body — no actual layer data needed.
const sampleManifestJSON = `{
  "schemaVersion": 2,
  "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
  "config": {
    "mediaType": "application/vnd.docker.container.image.v1+json",
    "size": 7023,
    "digest": "sha256:0000000000000000000000000000000000000000000000000000000000000000"
  },
  "layers": []
}`

func TestACR_ImageManifestPushGetDelete(t *testing.T) {
	const (
		rgName       = "acr-imageops-rg"
		registryName = "imageopsregistry"
		imageName    = "myapp"
		imageTag     = "v1.0"
	)

	ensureRG(t, rgName)

	// Create ACR registry via ARM so it exists in the store
	registriesClient, err := armcontainerregistry.NewRegistriesClient(subscriptionID, &fakeCredential{}, clientOpts())
	require.NoError(t, err)
	regPoller, err := registriesClient.BeginCreate(ctx, rgName, registryName, armcontainerregistry.Registry{
		Location: ptrStr("eastus"),
		SKU:      &armcontainerregistry.SKU{Name: ptrSKU(armcontainerregistry.SKUNameBasic)},
	}, nil)
	require.NoError(t, err)
	_, err = regPoller.PollUntilDone(ctx, nil)
	require.NoError(t, err)

	dp := acrDataPlaneClient(t)

	// Push manifest — UploadManifest requires io.ReadSeekCloser
	uploadResp, err := dp.UploadManifest(ctx, imageName, imageTag,
		azcontainerregistry.ContentTypeApplicationVndDockerDistributionManifestV2JSON,
		nopSeekCloser(strings.NewReader(sampleManifestJSON)), nil)
	require.NoError(t, err)
	require.NotNil(t, uploadResp.DockerContentDigest)
	digest := *uploadResp.DockerContentDigest
	assert.True(t, strings.HasPrefix(digest, "sha256:"), "digest must be sha256: prefixed, got %s", digest)

	// Get manifest back by tag — verify the digest matches what was pushed
	getResp, err := dp.GetManifest(ctx, imageName, imageTag, nil)
	require.NoError(t, err)
	getResp.ManifestData.Close()
	require.NotNil(t, getResp.DockerContentDigest)
	assert.Equal(t, digest, *getResp.DockerContentDigest)

	// List repositories — must include imageName
	pager := dp.NewListRepositoriesPager(nil)
	var foundRepo bool
	for pager.More() {
		page, err := pager.NextPage(ctx)
		require.NoError(t, err)
		for _, name := range page.Names {
			if name != nil && *name == imageName {
				foundRepo = true
			}
		}
	}
	assert.True(t, foundRepo, "expected ListRepositories to include %q", imageName)

	// List tags for the image — must include imageTag
	tagsPager := dp.NewListTagsPager(imageName, nil)
	var foundTag bool
	for tagsPager.More() {
		page, err := tagsPager.NextPage(ctx)
		require.NoError(t, err)
		for _, tag := range page.Tags {
			if tag != nil && tag.Name != nil && *tag.Name == imageTag {
				foundTag = true
			}
		}
	}
	assert.True(t, foundTag, "expected ListTags to include tag %q", imageTag)

	// Delete manifest by digest
	_, err = dp.DeleteManifest(ctx, imageName, digest, nil)
	require.NoError(t, err)

	// Get after delete — must fail
	_, err = dp.GetManifest(ctx, imageName, imageTag, nil)
	assert.Error(t, err, "GetManifest after DeleteManifest should fail")
}

// TestACR_CacheRuleCRUD covers the `cacheRules` sub-resource.
// Create + Get + List + Delete round-trip via
// `armcontainerregistry.CacheRulesClient` against the simulator.
func TestACR_CacheRuleCRUD(t *testing.T) {
	const (
		rgName       = "acr-cache-rg"
		registryName = "clitestcacheregistry"
		ruleName     = "docker-hub"
	)

	rgClient, err := armresources.NewResourceGroupsClient(subscriptionID, &fakeCredential{}, clientOpts())
	require.NoError(t, err)
	_, err = rgClient.CreateOrUpdate(ctx, rgName, armresources.ResourceGroup{
		Location: ptrStr("eastus"),
	}, nil)
	require.NoError(t, err)

	// Registry must exist before cache rule create (real ACR validates this).
	registriesClient, err := armcontainerregistry.NewRegistriesClient(subscriptionID, &fakeCredential{}, clientOpts())
	require.NoError(t, err)
	regPoller, err := registriesClient.BeginCreate(ctx, rgName, registryName, armcontainerregistry.Registry{
		Location: ptrStr("eastus"),
		SKU:      &armcontainerregistry.SKU{Name: ptrSKU(armcontainerregistry.SKUNameBasic)},
	}, nil)
	require.NoError(t, err)
	_, err = regPoller.PollUntilDone(ctx, nil)
	require.NoError(t, err)

	// CacheRule CRUD
	rulesClient, err := armcontainerregistry.NewCacheRulesClient(subscriptionID, &fakeCredential{}, clientOpts())
	require.NoError(t, err)

	// Create
	createPoller, err := rulesClient.BeginCreate(ctx, rgName, registryName, ruleName, armcontainerregistry.CacheRule{
		Properties: &armcontainerregistry.CacheRuleProperties{
			SourceRepository: ptrStr("docker.io/library/*"),
			TargetRepository: ptrStr("docker-hub/library/*"),
		},
	}, nil)
	require.NoError(t, err)
	createResp, err := createPoller.PollUntilDone(ctx, nil)
	require.NoError(t, err)
	assert.Equal(t, ruleName, *createResp.Name)
	require.NotNil(t, createResp.Properties)
	assert.Equal(t, "docker.io/library/*", *createResp.Properties.SourceRepository)
	assert.Equal(t, "docker-hub/library/*", *createResp.Properties.TargetRepository)

	// Get
	getResp, err := rulesClient.Get(ctx, rgName, registryName, ruleName, nil)
	require.NoError(t, err)
	assert.Equal(t, ruleName, *getResp.Name)
	assert.Equal(t, "docker.io/library/*", *getResp.Properties.SourceRepository)

	// List — should contain our rule.
	pager := rulesClient.NewListPager(rgName, registryName, nil)
	var found bool
	for pager.More() {
		page, err := pager.NextPage(ctx)
		require.NoError(t, err)
		for _, r := range page.Value {
			if r != nil && r.Name != nil && *r.Name == ruleName {
				found = true
			}
		}
	}
	assert.True(t, found, "expected List to return the created cache rule")

	// Delete
	delPoller, err := rulesClient.BeginDelete(ctx, rgName, registryName, ruleName, nil)
	require.NoError(t, err)
	_, err = delPoller.PollUntilDone(ctx, nil)
	require.NoError(t, err)

	// Get after delete — should 404.
	_, err = rulesClient.Get(ctx, rgName, registryName, ruleName, nil)
	assert.Error(t, err, "Get after delete should fail")
}

// TestACR_CacheRuleIdempotentUpsert ensures the simulator's PUT is
// idempotent — re-creating the same rule updates in place rather than
// erroring. Real ACR's BeginCreate behaves this way too (hence the
// name, which matches the SDK method).
func TestACR_CacheRuleIdempotentUpsert(t *testing.T) {
	const (
		rgName       = "acr-upsert-rg"
		registryName = "upsertregistry"
		ruleName     = "ghcr-io"
	)

	rgClient, err := armresources.NewResourceGroupsClient(subscriptionID, &fakeCredential{}, clientOpts())
	require.NoError(t, err)
	_, err = rgClient.CreateOrUpdate(ctx, rgName, armresources.ResourceGroup{
		Location: ptrStr("eastus"),
	}, nil)
	require.NoError(t, err)

	registriesClient, err := armcontainerregistry.NewRegistriesClient(subscriptionID, &fakeCredential{}, clientOpts())
	require.NoError(t, err)
	regPoller, err := registriesClient.BeginCreate(ctx, rgName, registryName, armcontainerregistry.Registry{
		Location: ptrStr("eastus"),
		SKU:      &armcontainerregistry.SKU{Name: ptrSKU(armcontainerregistry.SKUNameBasic)},
	}, nil)
	require.NoError(t, err)
	_, err = regPoller.PollUntilDone(ctx, nil)
	require.NoError(t, err)

	rulesClient, err := armcontainerregistry.NewCacheRulesClient(subscriptionID, &fakeCredential{}, clientOpts())
	require.NoError(t, err)

	first, err := rulesClient.BeginCreate(ctx, rgName, registryName, ruleName, armcontainerregistry.CacheRule{
		Properties: &armcontainerregistry.CacheRuleProperties{
			SourceRepository: ptrStr("ghcr.io/owner/app"),
			TargetRepository: ptrStr("ghcr-io/owner/app"),
		},
	}, nil)
	require.NoError(t, err)
	_, err = first.PollUntilDone(ctx, nil)
	require.NoError(t, err)

	// Second create with an updated target — must upsert cleanly.
	second, err := rulesClient.BeginCreate(ctx, rgName, registryName, ruleName, armcontainerregistry.CacheRule{
		Properties: &armcontainerregistry.CacheRuleProperties{
			SourceRepository: ptrStr("ghcr.io/owner/app"),
			TargetRepository: ptrStr("ghcr-io/owner/app-v2"),
		},
	}, nil)
	require.NoError(t, err)
	secondResp, err := second.PollUntilDone(ctx, nil)
	require.NoError(t, err)
	assert.Equal(t, "ghcr-io/owner/app-v2", *secondResp.Properties.TargetRepository)
}

func ptrSKU(s armcontainerregistry.SKUName) *armcontainerregistry.SKUName { return &s }

// nopSeekCloser wraps an io.ReadSeeker with a no-op Close, satisfying io.ReadSeekCloser.
type nopSeekCloserT struct{ io.ReadSeeker }

func (nopSeekCloserT) Close() error                   { return nil }
func nopSeekCloser(r io.ReadSeeker) io.ReadSeekCloser { return nopSeekCloserT{r} }
