package azure_sdk_test

import (
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appcontainers/armappcontainers/v3"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appservice/armappservice/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// BUG-834 — sim was missing v2 ContainerApps Apps routes
// (Microsoft.App/containerApps); only Jobs were registered. The aca
// backend's UseApp path uses ContainerAppsClient.{BeginCreateOrUpdate,
// Get, BeginDelete} which silently 404'd against the sim. Pin the
// contract using the same SDK + types the backend uses.

func TestSDK_ContainerAppsApps_CreateGetDelete(t *testing.T) {
	rg := "sdk-aca-app-rg"
	ensureRG(t, rg)

	cred := &fakeCredential{}
	client, err := armappcontainers.NewContainerAppsClient(subscriptionID, cred, clientOpts())
	require.NoError(t, err)

	envID := "/subscriptions/" + subscriptionID + "/resourceGroups/" + rg +
		"/providers/Microsoft.App/managedEnvironments/sim-env"

	poller, err := client.BeginCreateOrUpdate(ctx, rg, "sdk-test-app", armappcontainers.ContainerApp{
		Location: to.Ptr("eastus"),
		Tags: map[string]*string{
			"sockerless-managed":      to.Ptr("true"),
			"sockerless-container-id": to.Ptr("abc123"),
		},
		Properties: &armappcontainers.ContainerAppProperties{
			EnvironmentID: to.Ptr(envID),
			Configuration: &armappcontainers.Configuration{
				ActiveRevisionsMode: to.Ptr(armappcontainers.ActiveRevisionsModeSingle),
				Ingress: &armappcontainers.Ingress{
					External:   to.Ptr(false),
					TargetPort: to.Ptr[int32](8080),
					Transport:  to.Ptr(armappcontainers.IngressTransportMethodAuto),
				},
			},
			Template: &armappcontainers.Template{
				Containers: []*armappcontainers.Container{
					{
						Name:  to.Ptr("main"),
						Image: to.Ptr("alpine:latest"),
					},
				},
				Scale: &armappcontainers.Scale{
					MinReplicas: to.Ptr[int32](1),
					MaxReplicas: to.Ptr[int32](1),
				},
			},
		},
	}, nil)
	require.NoError(t, err)

	app, err := poller.PollUntilDone(ctx, nil)
	require.NoError(t, err, "BeginCreateOrUpdate poller must complete")
	require.NotNil(t, app.Name)
	assert.Equal(t, "sdk-test-app", *app.Name)
	require.NotNil(t, app.Properties)
	require.NotNil(t, app.Properties.ProvisioningState)
	assert.Equal(t, "Succeeded", string(*app.Properties.ProvisioningState),
		"provisioningState must be Succeeded so backend's appContainerState reads 'running'")
	require.NotNil(t, app.Properties.LatestReadyRevisionName)
	assert.NotEmpty(t, *app.Properties.LatestReadyRevisionName,
		"LatestReadyRevisionName drives the Ready check in appContainerState")
	require.NotNil(t, app.Properties.LatestRevisionFqdn)
	assert.NotEmpty(t, *app.Properties.LatestRevisionFqdn,
		"LatestRevisionFqdn is what cloudServiceRegisterCNAME reads to seed Private DNS")

	// GET round-trip.
	getResp, err := client.Get(ctx, rg, "sdk-test-app", nil)
	require.NoError(t, err)
	require.NotNil(t, getResp.Name)
	assert.Equal(t, "sdk-test-app", *getResp.Name)
	require.NotNil(t, getResp.Tags["sockerless-managed"])
	assert.Equal(t, "true", *getResp.Tags["sockerless-managed"])

	// DELETE.
	delPoller, err := client.BeginDelete(ctx, rg, "sdk-test-app", nil)
	require.NoError(t, err)
	_, err = delPoller.PollUntilDone(ctx, nil)
	require.NoError(t, err)

	// GET after delete should 404.
	_, err = client.Get(ctx, rg, "sdk-test-app", nil)
	assert.Error(t, err, "Get after delete must fail")
}

// BUG-835 — sim was missing the WebApps.UpdateAzureStorageAccounts
// route. The azure-functions backend's volumes.go binds named docker
// volumes to Azure Files shares via this call; without it, function
// apps cannot mount user volumes.
func TestSDK_WebApps_UpdateAzureStorageAccounts(t *testing.T) {
	rg := "sdk-azf-storage-rg"
	ensureRG(t, rg)

	cred := &fakeCredential{}
	client, err := armappservice.NewWebAppsClient(subscriptionID, cred, clientOpts())
	require.NoError(t, err)

	// Create a site first.
	createPoller, err := client.BeginCreateOrUpdate(ctx, rg, "sdk-storage-site", armappservice.Site{
		Location: to.Ptr("eastus"),
		Kind:     to.Ptr("functionapp"),
		Properties: &armappservice.SiteProperties{
			ServerFarmID: to.Ptr("/subscriptions/" + subscriptionID + "/resourceGroups/" + rg +
				"/providers/Microsoft.Web/serverFarms/test-plan"),
		},
	}, nil)
	require.NoError(t, err)
	_, err = createPoller.PollUntilDone(ctx, nil)
	require.NoError(t, err)

	// Attach two named volumes via UpdateAzureStorageAccounts.
	resp, err := client.UpdateAzureStorageAccounts(ctx, rg, "sdk-storage-site",
		armappservice.AzureStoragePropertyDictionaryResource{
			Properties: map[string]*armappservice.AzureStorageInfoValue{
				"data": {
					Type:        to.Ptr(armappservice.AzureStorageTypeAzureFiles),
					AccountName: to.Ptr("simstorage"),
					ShareName:   to.Ptr("data-share"),
					AccessKey:   to.Ptr("fake-key"),
					MountPath:   to.Ptr("/mnt/data"),
				},
				"cache": {
					Type:        to.Ptr(armappservice.AzureStorageTypeAzureFiles),
					AccountName: to.Ptr("simstorage"),
					ShareName:   to.Ptr("cache-share"),
					AccessKey:   to.Ptr("fake-key"),
					MountPath:   to.Ptr("/mnt/cache"),
				},
			},
		}, nil)
	require.NoError(t, err, "UpdateAzureStorageAccounts must succeed against the sim")
	require.NotNil(t, resp.Properties)
	assert.Len(t, resp.Properties, 2, "both volume mappings should round-trip")
	require.Contains(t, resp.Properties, "data")
	require.NotNil(t, resp.Properties["data"])
	assert.Equal(t, "data-share", *resp.Properties["data"].ShareName)
	assert.Equal(t, "/mnt/data", *resp.Properties["data"].MountPath)
}
