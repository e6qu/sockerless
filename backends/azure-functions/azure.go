package azf

import (
	"context"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/monitor/azquery"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appservice/armappservice/v5"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v8"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/privatedns/armprivatedns"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage"
)

type fakeCredential struct{}

func (f *fakeCredential) GetToken(_ context.Context, _ policy.TokenRequestOptions) (azcore.AccessToken, error) {
	return azcore.AccessToken{Token: "fake-token", ExpiresOn: time.Now().Add(time.Hour)}, nil
}

// AzureClients holds all Azure SDK clients for the Azure Functions backend.
type AzureClients struct {
	WebApps *armappservice.WebAppsClient
	Logs    *azquery.LogsClient
	Cred    azcore.TokenCredential

	// FileShares provisions sockerless-managed Azure Files shares
	// (shared with ACA via azurecommon.FileShareManager);
	// StorageAccounts fetches the access key at mount-attach time so
	// rotated keys take effect without a restart.
	FileShares      *armstorage.FileSharesClient
	StorageAccounts *armstorage.AccountsClient

	// Private DNS plumbing for the cloud-dns NetworkDiscovery driver.
	// Mirrors the ACA backend — same per-network-zone shape, with the
	// zone created by NetworkCreate and per-container A/CNAME records
	// written by azurecommon.PrivateDNSDiscovery.
	PrivateDNSZones   *armprivatedns.PrivateZonesClient
	PrivateDNSRecords *armprivatedns.RecordSetsClient

	// VNet plumbing for App Service regional VNet integration (cloud-dns
	// service discovery). NetworkCreate provisions a VNet + a subnet delegated
	// to Microsoft.Web/serverFarms + links the Private DNS zone to the VNet;
	// each site's container joins the VNet via WebApps swift integration.
	VirtualNetworks *armnetwork.VirtualNetworksClient
	Subnets         *armnetwork.SubnetsClient
	PrivateDNSLinks *armprivatedns.VirtualNetworkLinksClient
}

// NewAzureClients initializes Azure SDK clients.
func NewAzureClients(subscriptionID string, endpointURL string) (*AzureClients, error) {
	if endpointURL != "" {
		return newAzureClientsWithEndpoint(subscriptionID, endpointURL)
	}
	return newAzureClientsDefault(subscriptionID)
}

func newAzureClientsWithEndpoint(subscriptionID string, endpointURL string) (*AzureClients, error) {
	cred := &fakeCredential{}
	opts := &arm.ClientOptions{
		ClientOptions: azcore.ClientOptions{
			Cloud: cloud.Configuration{
				Services: map[cloud.ServiceName]cloud.ServiceConfiguration{
					cloud.ResourceManager: {
						Endpoint: endpointURL,
						Audience: "https://management.azure.com/",
					},
					azquery.ServiceNameLogs: {
						Endpoint: endpointURL,
						Audience: "https://api.loganalytics.io/",
					},
				},
			},
			InsecureAllowCredentialWithHTTP: true,
		},
	}

	webAppsClient, err := armappservice.NewWebAppsClient(subscriptionID, cred, opts)
	if err != nil {
		return nil, err
	}

	logsClient, err := azquery.NewLogsClient(cred, &azquery.LogsClientOptions{
		ClientOptions: opts.ClientOptions,
	})
	if err != nil {
		return nil, err
	}

	fileShares, err := armstorage.NewFileSharesClient(subscriptionID, cred, opts)
	if err != nil {
		return nil, err
	}
	storageAccounts, err := armstorage.NewAccountsClient(subscriptionID, cred, opts)
	if err != nil {
		return nil, err
	}
	privateZones, err := armprivatedns.NewPrivateZonesClient(subscriptionID, cred, opts)
	if err != nil {
		return nil, err
	}
	privateRecords, err := armprivatedns.NewRecordSetsClient(subscriptionID, cred, opts)
	if err != nil {
		return nil, err
	}
	privateLinks, err := armprivatedns.NewVirtualNetworkLinksClient(subscriptionID, cred, opts)
	if err != nil {
		return nil, err
	}
	vnets, err := armnetwork.NewVirtualNetworksClient(subscriptionID, cred, opts)
	if err != nil {
		return nil, err
	}
	subnets, err := armnetwork.NewSubnetsClient(subscriptionID, cred, opts)
	if err != nil {
		return nil, err
	}

	return &AzureClients{
		WebApps:           webAppsClient,
		Logs:              logsClient,
		Cred:              cred,
		FileShares:        fileShares,
		StorageAccounts:   storageAccounts,
		PrivateDNSZones:   privateZones,
		PrivateDNSRecords: privateRecords,
		PrivateDNSLinks:   privateLinks,
		VirtualNetworks:   vnets,
		Subnets:           subnets,
	}, nil
}

func newAzureClientsDefault(subscriptionID string) (*AzureClients, error) {
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, err
	}

	webAppsClient, err := armappservice.NewWebAppsClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, err
	}

	logsClient, err := azquery.NewLogsClient(cred, nil)
	if err != nil {
		return nil, err
	}

	fileShares, err := armstorage.NewFileSharesClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, err
	}
	storageAccounts, err := armstorage.NewAccountsClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, err
	}
	privateZones, err := armprivatedns.NewPrivateZonesClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, err
	}
	privateRecords, err := armprivatedns.NewRecordSetsClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, err
	}
	privateLinks, err := armprivatedns.NewVirtualNetworkLinksClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, err
	}
	vnets, err := armnetwork.NewVirtualNetworksClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, err
	}
	subnets, err := armnetwork.NewSubnetsClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, err
	}

	return &AzureClients{
		WebApps:           webAppsClient,
		Logs:              logsClient,
		Cred:              cred,
		FileShares:        fileShares,
		StorageAccounts:   storageAccounts,
		PrivateDNSZones:   privateZones,
		PrivateDNSRecords: privateRecords,
		PrivateDNSLinks:   privateLinks,
		VirtualNetworks:   vnets,
		Subnets:           subnets,
	}, nil
}
