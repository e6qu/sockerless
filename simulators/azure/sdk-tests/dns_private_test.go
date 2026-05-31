package azure_sdk_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/privatedns/armprivatedns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrivateDNS_ListZonesAndVirtualNetworkLinks(t *testing.T) {
	rg := "dns-private-rg"
	ensureRG(t, rg)
	zoneName := "sdk-private.example.internal"
	linkName := "sdk-vnet-link"

	putARMJSON(t,
		"/subscriptions/"+subscriptionID+"/resourceGroups/"+rg+"/providers/Microsoft.Network/privateDnsZones/"+zoneName+"?api-version=2018-09-01",
		`{"location":"global","tags":{"env":"sdk"}}`)
	putARMJSON(t,
		"/subscriptions/"+subscriptionID+"/resourceGroups/"+rg+"/providers/Microsoft.Network/privateDnsZones/"+zoneName+"/virtualNetworkLinks/"+linkName+"?api-version=2018-09-01",
		`{"location":"global","properties":{"virtualNetwork":{"id":"/subscriptions/00000000-0000-0000-0000-000000000001/resourceGroups/dns-private-rg/providers/Microsoft.Network/virtualNetworks/sdk-vnet"},"registrationEnabled":false}}`)

	zones, err := armprivatedns.NewPrivateZonesClient(subscriptionID, &fakeCredential{}, clientOpts())
	require.NoError(t, err)
	zonePager := zones.NewListByResourceGroupPager(rg, nil)
	require.True(t, zonePager.More())
	zonePage, err := zonePager.NextPage(ctx)
	require.NoError(t, err)

	var foundZone *armprivatedns.PrivateZone
	for _, zone := range zonePage.Value {
		if zone.Name != nil && *zone.Name == zoneName {
			foundZone = zone
			break
		}
	}
	require.NotNil(t, foundZone)
	require.NotNil(t, foundZone.Properties)
	require.NotNil(t, foundZone.Properties.NumberOfRecordSets)
	assert.Equal(t, int64(1), *foundZone.Properties.NumberOfRecordSets)

	links, err := armprivatedns.NewVirtualNetworkLinksClient(subscriptionID, &fakeCredential{}, clientOpts())
	require.NoError(t, err)
	linkPager := links.NewListPager(rg, zoneName, nil)
	require.True(t, linkPager.More())
	linkPage, err := linkPager.NextPage(ctx)
	require.NoError(t, err)

	var foundLink *armprivatedns.VirtualNetworkLink
	for _, link := range linkPage.Value {
		if link.Name != nil && *link.Name == linkName {
			foundLink = link
			break
		}
	}
	require.NotNil(t, foundLink)
	require.NotNil(t, foundLink.Properties)
	require.NotNil(t, foundLink.Properties.ProvisioningState)
	assert.Equal(t, armprivatedns.ProvisioningStateSucceeded, *foundLink.Properties.ProvisioningState)
}

func putARMJSON(t *testing.T, path, body string) {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, baseURL+path, strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer fake-token")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}
