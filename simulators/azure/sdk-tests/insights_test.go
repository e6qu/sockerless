package azure_sdk_test

import (
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/applicationinsights/armapplicationinsights"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAppInsights_ComponentCRUD(t *testing.T) {
	const (
		rgName        = "insights-sdk-rg"
		componentName = "sdk-test-appinsights"
	)

	ensureRG(t, rgName)

	client, err := armapplicationinsights.NewComponentsClient(subscriptionID, &fakeCredential{}, clientOpts())
	require.NoError(t, err)

	// Create
	createResp, err := client.CreateOrUpdate(ctx, rgName, componentName, armapplicationinsights.Component{
		Location: ptrStr("eastus"),
		Kind:     ptrStr("web"),
		Properties: &armapplicationinsights.ComponentProperties{
			ApplicationType: ptrApplicationType(armapplicationinsights.ApplicationTypeWeb),
		},
	}, nil)
	require.NoError(t, err)
	assert.Equal(t, componentName, *createResp.Name)
	require.NotNil(t, createResp.Properties)
	assert.NotEmpty(t, *createResp.Properties.InstrumentationKey)
	assert.NotEmpty(t, *createResp.Properties.ConnectionString)
	assert.Contains(t, *createResp.Properties.ConnectionString, "InstrumentationKey=")

	// Get
	getResp, err := client.Get(ctx, rgName, componentName, nil)
	require.NoError(t, err)
	assert.Equal(t, componentName, *getResp.Name)
	assert.Equal(t, *createResp.Properties.InstrumentationKey, *getResp.Properties.InstrumentationKey)

	// Delete
	_, err = client.Delete(ctx, rgName, componentName, nil)
	require.NoError(t, err)

	// Get after delete — must 404
	_, err = client.Get(ctx, rgName, componentName, nil)
	assert.Error(t, err, "Get after delete should fail with 404")
}

func TestAppInsights_InstrumentationKeyStableOnUpsert(t *testing.T) {
	const (
		rgName = "insights-key-rg"
		name   = "stable-key-component"
	)
	ensureRG(t, rgName)

	client, err := armapplicationinsights.NewComponentsClient(subscriptionID, &fakeCredential{}, clientOpts())
	require.NoError(t, err)

	first, err := client.CreateOrUpdate(ctx, rgName, name, armapplicationinsights.Component{
		Location: ptrStr("westus"),
		Kind:     ptrStr("web"),
		Properties: &armapplicationinsights.ComponentProperties{
			ApplicationType: ptrApplicationType(armapplicationinsights.ApplicationTypeWeb),
		},
	}, nil)
	require.NoError(t, err)

	// Re-create (upsert) — instrumentation key must be stable across updates
	second, err := client.CreateOrUpdate(ctx, rgName, name, armapplicationinsights.Component{
		Location: ptrStr("westus"),
		Kind:     ptrStr("web"),
		Properties: &armapplicationinsights.ComponentProperties{
			ApplicationType: ptrApplicationType(armapplicationinsights.ApplicationTypeWeb),
		},
	}, nil)
	require.NoError(t, err)
	assert.Equal(t, *first.Properties.InstrumentationKey, *second.Properties.InstrumentationKey)
}

func ptrApplicationType(s armapplicationinsights.ApplicationType) *armapplicationinsights.ApplicationType {
	return &s
}
