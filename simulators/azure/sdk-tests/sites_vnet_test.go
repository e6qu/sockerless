package azure_sdk_test

import (
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appservice/armappservice/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSDK_SiteVNetIntegration_RoundTrip exercises App Service regional VNet
// integration (the "swift" virtual network connection) — the endpoint sockerless
// uses for cloud-dns service discovery. A site joins a Microsoft.Web-delegated
// subnet; the connection round-trips on GET and is removable.
func TestSDK_SiteVNetIntegration_RoundTrip(t *testing.T) {
	rg := "sdk-sitevnet-rg"
	site := "sitevnet-app"
	ensureRG(t, rg)

	cred := &fakeCredential{}
	client, err := armappservice.NewWebAppsClient(subscriptionID, cred, clientOpts())
	require.NoError(t, err)

	// A site with a sockerless-overlay image is an HTTP function site, so VNet
	// integration only records the connection (no service container to start) —
	// keeping this an ARM round-trip test, not a workload test.
	p, err := client.BeginCreateOrUpdate(ctx, rg, site, armappservice.Site{
		Location: to.Ptr("eastus"),
		Kind:     to.Ptr("functionapp,linux,container"),
		Properties: &armappservice.SiteProperties{
			SiteConfig: &armappservice.SiteConfig{
				LinuxFxVersion: to.Ptr("DOCKER|registry.example/sockerless-overlay/azf:test"),
			},
		},
	}, nil)
	require.NoError(t, err)
	_, err = p.PollUntilDone(ctx, nil)
	require.NoError(t, err)

	subnetID := "/subscriptions/" + subscriptionID + "/resourceGroups/" + rg +
		"/providers/Microsoft.Network/virtualNetworks/sdk-sitevnet/subnets/appservice"

	put, err := client.CreateOrUpdateSwiftVirtualNetworkConnectionWithCheck(ctx, rg, site, armappservice.SwiftVirtualNetwork{
		Properties: &armappservice.SwiftVirtualNetworkProperties{
			SubnetResourceID: to.Ptr(subnetID),
			SwiftSupported:   to.Ptr(true),
		},
	}, nil)
	require.NoError(t, err)
	require.NotNil(t, put.Properties)
	assert.Equal(t, subnetID, *put.Properties.SubnetResourceID)

	got, err := client.GetSwiftVirtualNetworkConnection(ctx, rg, site, nil)
	require.NoError(t, err)
	require.NotNil(t, got.Properties)
	assert.Equal(t, subnetID, *got.Properties.SubnetResourceID)

	_, err = client.DeleteSwiftVirtualNetwork(ctx, rg, site, nil)
	require.NoError(t, err)
}
