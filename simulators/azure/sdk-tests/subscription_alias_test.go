package azure_sdk_test

// Microsoft.Subscription alias API (2021-10-01) through the official
// armsubscription SDK module. Routes exercised:
//
//	PUT /providers/Microsoft.Subscription/aliases/{aliasName}
//	GET /providers/Microsoft.Subscription/aliases/{aliasName}
//	DELETE /providers/Microsoft.Subscription/aliases/{aliasName}
//	GET /providers/Microsoft.Subscription/aliases
//	POST /subscriptions/{subscriptionId}/providers/Microsoft.Subscription/cancel
//	POST /subscriptions/{subscriptionId}/providers/Microsoft.Subscription/rename
//	POST /subscriptions/{subscriptionId}/providers/Microsoft.Subscription/enable

import (
	"errors"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armsubscriptions"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/subscription/armsubscription"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSubscriptionAlias_CreateLifecycle drives the full programmatic
// subscription-creation flow: the alias PUT under a billing scope, the SDK's
// long-running-operation poller to Succeeded, visibility of the created
// subscription through the subscriptions list, a resource group created
// inside the new subscription, and the rename/cancel/enable actions.
func TestSubscriptionAlias_CreateLifecycle(t *testing.T) {
	aliasClient, err := armsubscription.NewAliasClient(&fakeCredential{}, clientOpts())
	require.NoError(t, err)

	const aliasName = "sdk-test-sub-alias"
	poller, err := aliasClient.BeginCreate(ctx, aliasName, armsubscription.PutAliasRequest{
		Properties: &armsubscription.PutAliasRequestProperties{
			DisplayName:  to.Ptr("SDK Created Subscription"),
			Workload:     to.Ptr(armsubscription.WorkloadProduction),
			BillingScope: to.Ptr("/providers/Microsoft.Billing/billingAccounts/sim-billing-account/enrollmentAccounts/sim-enrollment"),
		},
	}, nil)
	require.NoError(t, err)
	created, err := poller.PollUntilDone(ctx, nil)
	require.NoError(t, err)
	require.NotNil(t, created.Properties)
	require.NotNil(t, created.Properties.ProvisioningState)
	assert.Equal(t, armsubscription.ProvisioningStateSucceeded, *created.Properties.ProvisioningState)
	require.NotNil(t, created.Properties.SubscriptionID)
	newSubID := *created.Properties.SubscriptionID
	require.NotEmpty(t, newSubID)
	require.NotEqual(t, subscriptionID, newSubID, "a created subscription must get its own id")

	// A repeated PUT with the same intent is idempotent: same alias, same
	// subscription, no second creation.
	again, err := aliasClient.BeginCreate(ctx, aliasName, armsubscription.PutAliasRequest{
		Properties: &armsubscription.PutAliasRequestProperties{
			DisplayName:  to.Ptr("SDK Created Subscription"),
			Workload:     to.Ptr(armsubscription.WorkloadProduction),
			BillingScope: to.Ptr("/providers/Microsoft.Billing/billingAccounts/sim-billing-account/enrollmentAccounts/sim-enrollment"),
		},
	}, nil)
	require.NoError(t, err)
	againResult, err := again.PollUntilDone(ctx, nil)
	require.NoError(t, err)
	require.NotNil(t, againResult.Properties)
	assert.Equal(t, newSubID, *againResult.Properties.SubscriptionID)

	get, err := aliasClient.Get(ctx, aliasName, nil)
	require.NoError(t, err)
	assert.Equal(t, aliasName, *get.Name)
	assert.Equal(t, "Microsoft.Subscription/aliases", *get.Type)
	assert.Equal(t, newSubID, *get.Properties.SubscriptionID)

	list, err := aliasClient.List(ctx, nil)
	require.NoError(t, err)
	found := false
	for _, alias := range list.Value {
		if alias.Name != nil && *alias.Name == aliasName {
			found = true
		}
	}
	assert.True(t, found, "alias list must include %q", aliasName)

	// The created subscription is a real, Enabled subscription: readable,
	// listed, and usable as a scope for resource creation.
	subsClient, err := armsubscriptions.NewClient(&fakeCredential{}, clientOpts())
	require.NoError(t, err)
	sub, err := subsClient.Get(ctx, newSubID, nil)
	require.NoError(t, err)
	assert.Equal(t, "SDK Created Subscription", *sub.DisplayName)
	assert.Equal(t, armsubscriptions.SubscriptionStateEnabled, *sub.State)

	pager := subsClient.NewListPager(nil)
	listed := false
	for pager.More() {
		page, err := pager.NextPage(ctx)
		require.NoError(t, err)
		for _, s := range page.Value {
			if s.SubscriptionID != nil && *s.SubscriptionID == newSubID {
				listed = true
			}
		}
	}
	assert.True(t, listed, "created subscription %q must appear in the subscriptions list", newSubID)

	rgClient, err := armresources.NewResourceGroupsClient(newSubID, &fakeCredential{}, clientOpts())
	require.NoError(t, err)
	rg, err := rgClient.CreateOrUpdate(ctx, "sdk-test-sub-alias-rg", armresources.ResourceGroup{
		Location: to.Ptr("eastus"),
	}, nil)
	require.NoError(t, err)
	assert.Contains(t, *rg.ID, "/subscriptions/"+newSubID+"/resourceGroups/sdk-test-sub-alias-rg")

	// Rename, cancel, and enable — the subscription-scoped actions.
	subActions, err := armsubscription.NewClient(&fakeCredential{}, clientOpts())
	require.NoError(t, err)
	renamed, err := subActions.Rename(ctx, newSubID, armsubscription.Name{
		SubscriptionName: to.Ptr("SDK Renamed Subscription"),
	}, nil)
	require.NoError(t, err)
	assert.Equal(t, newSubID, *renamed.SubscriptionID)
	sub, err = subsClient.Get(ctx, newSubID, nil)
	require.NoError(t, err)
	assert.Equal(t, "SDK Renamed Subscription", *sub.DisplayName)

	cancelled, err := subActions.Cancel(ctx, newSubID, nil)
	require.NoError(t, err)
	assert.Equal(t, newSubID, *cancelled.SubscriptionID)
	sub, err = subsClient.Get(ctx, newSubID, nil)
	require.NoError(t, err)
	assert.Equal(t, armsubscriptions.SubscriptionStateDisabled, *sub.State)

	enabled, err := subActions.Enable(ctx, newSubID, nil)
	require.NoError(t, err)
	assert.Equal(t, newSubID, *enabled.SubscriptionID)
	sub, err = subsClient.Get(ctx, newSubID, nil)
	require.NoError(t, err)
	assert.Equal(t, armsubscriptions.SubscriptionStateEnabled, *sub.State)

	_, err = aliasClient.Delete(ctx, aliasName, nil)
	require.NoError(t, err)
	_, err = aliasClient.Get(ctx, aliasName, nil)
	var respErr *azcore.ResponseError
	require.ErrorAs(t, err, &respErr)
	assert.Equal(t, 404, respErr.StatusCode)

	// Deleting the alias never deletes the subscription.
	sub, err = subsClient.Get(ctx, newSubID, nil)
	require.NoError(t, err)
	assert.Equal(t, armsubscriptions.SubscriptionStateEnabled, *sub.State)
}

// TestSubscriptionAlias_AdoptExistingSubscription creates an alias for an
// already-existing subscription id — the second creation mode the alias API
// supports (properties.subscriptionId instead of a billing scope).
func TestSubscriptionAlias_AdoptExistingSubscription(t *testing.T) {
	aliasClient, err := armsubscription.NewAliasClient(&fakeCredential{}, clientOpts())
	require.NoError(t, err)

	const aliasName = "sdk-test-sub-alias-adopted"
	const adoptedSubID = "00000000-0000-0000-0000-00000000ad07"
	poller, err := aliasClient.BeginCreate(ctx, aliasName, armsubscription.PutAliasRequest{
		Properties: &armsubscription.PutAliasRequestProperties{
			SubscriptionID: to.Ptr(adoptedSubID),
		},
	}, nil)
	require.NoError(t, err)
	created, err := poller.PollUntilDone(ctx, nil)
	require.NoError(t, err)
	require.NotNil(t, created.Properties)
	assert.Equal(t, adoptedSubID, *created.Properties.SubscriptionID)
	assert.Equal(t, armsubscription.ProvisioningStateSucceeded, *created.Properties.ProvisioningState)

	_, err = aliasClient.Delete(ctx, aliasName, nil)
	require.NoError(t, err)
}

// TestSubscriptionAlias_RequiresBillingScopeOrSubscriptionID asserts the
// documented request contract: an alias PUT must carry either a billing
// scope (creation) or a subscription id (adoption).
func TestSubscriptionAlias_RequiresBillingScopeOrSubscriptionID(t *testing.T) {
	aliasClient, err := armsubscription.NewAliasClient(&fakeCredential{}, clientOpts())
	require.NoError(t, err)

	_, err = aliasClient.BeginCreate(ctx, "sdk-test-sub-alias-invalid", armsubscription.PutAliasRequest{
		Properties: &armsubscription.PutAliasRequestProperties{
			DisplayName: to.Ptr("No Billing Scope"),
		},
	}, nil)
	var respErr *azcore.ResponseError
	require.True(t, errors.As(err, &respErr), "alias PUT without billingScope or subscriptionId must fail, got %v", err)
	assert.Equal(t, 400, respErr.StatusCode)
}
