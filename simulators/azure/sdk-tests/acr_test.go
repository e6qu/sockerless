package azure_sdk_test

import (
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerregistry/armcontainerregistry"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestACR_CacheRuleCRUD covers the `cacheRules` sub-resource added for
// BUG-706. Create + Get + List + Delete round-trip via
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
