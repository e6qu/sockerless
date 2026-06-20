package azure_sdk_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/msi/armmsi"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIdentity_CreateUserAssigned(t *testing.T) {
	// Ensure resource group
	rgClient, err := armresources.NewResourceGroupsClient(subscriptionID, &fakeCredential{}, clientOpts())
	require.NoError(t, err)
	_, err = rgClient.CreateOrUpdate(ctx, "identity-rg", armresources.ResourceGroup{
		Location: ptrStr("eastus"),
	}, nil)
	require.NoError(t, err)

	client, err := armmsi.NewUserAssignedIdentitiesClient(subscriptionID, &fakeCredential{}, clientOpts())
	require.NoError(t, err)

	resp, err := client.CreateOrUpdate(ctx, "identity-rg", "test-identity", armmsi.Identity{
		Location: ptrStr("eastus"),
	}, nil)
	require.NoError(t, err)
	assert.Equal(t, "test-identity", *resp.Name)
	assert.NotEmpty(t, *resp.Properties.PrincipalID)
	assert.NotEmpty(t, *resp.Properties.ClientID)
}

func TestIdentity_GetUserAssigned(t *testing.T) {
	rgClient, err := armresources.NewResourceGroupsClient(subscriptionID, &fakeCredential{}, clientOpts())
	require.NoError(t, err)
	_, err = rgClient.CreateOrUpdate(ctx, "get-id-rg", armresources.ResourceGroup{
		Location: ptrStr("eastus"),
	}, nil)
	require.NoError(t, err)

	client, err := armmsi.NewUserAssignedIdentitiesClient(subscriptionID, &fakeCredential{}, clientOpts())
	require.NoError(t, err)

	_, err = client.CreateOrUpdate(ctx, "get-id-rg", "get-identity", armmsi.Identity{
		Location: ptrStr("eastus"),
	}, nil)
	require.NoError(t, err)

	resp, err := client.Get(ctx, "get-id-rg", "get-identity", nil)
	require.NoError(t, err)
	assert.Equal(t, "get-identity", *resp.Name)
}

func TestIdentity_DeleteUserAssigned(t *testing.T) {
	rgClient, err := armresources.NewResourceGroupsClient(subscriptionID, &fakeCredential{}, clientOpts())
	require.NoError(t, err)
	_, err = rgClient.CreateOrUpdate(ctx, "del-id-rg", armresources.ResourceGroup{
		Location: ptrStr("eastus"),
	}, nil)
	require.NoError(t, err)

	client, err := armmsi.NewUserAssignedIdentitiesClient(subscriptionID, &fakeCredential{}, clientOpts())
	require.NoError(t, err)

	_, err = client.CreateOrUpdate(ctx, "del-id-rg", "del-identity", armmsi.Identity{
		Location: ptrStr("eastus"),
	}, nil)
	require.NoError(t, err)

	_, err = client.Delete(ctx, "del-id-rg", "del-identity", nil)
	require.NoError(t, err)
}

// TestIdentity_CreateOrUpdateStatusCodes verifies the ARM PUT createOrUpdate
// status-code contract: 201 Created for a brand-new identity, 200 OK when the
// same resource is updated. The armmsi SDK hides the status code, so this
// drives the endpoint over raw HTTP.
func TestIdentity_CreateOrUpdateStatusCodes(t *testing.T) {
	rgClient, err := armresources.NewResourceGroupsClient(subscriptionID, &fakeCredential{}, clientOpts())
	require.NoError(t, err)
	_, err = rgClient.CreateOrUpdate(ctx, "id-status-rg", armresources.ResourceGroup{
		Location: ptrStr("eastus"),
	}, nil)
	require.NoError(t, err)

	url := baseURL + "/subscriptions/" + subscriptionID +
		"/resourceGroups/id-status-rg/providers/Microsoft.ManagedIdentity/userAssignedIdentities/status-identity?api-version=2023-01-31"
	put := func() int {
		req, _ := http.NewRequestWithContext(ctx, "PUT", url, strings.NewReader(`{"location":"eastus"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer fake-token")
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		resp.Body.Close()
		return resp.StatusCode
	}

	assert.Equal(t, http.StatusCreated, put(), "first PUT of a new identity must return 201 Created")
	assert.Equal(t, http.StatusOK, put(), "second PUT of the existing identity must return 200 OK")
}
