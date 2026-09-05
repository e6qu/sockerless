package azf

import (
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/monitor/azquery"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appservice/armappservice/v5"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v8"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/privatedns/armprivatedns"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage"
	azurecommon "github.com/sockerless/azure-common"
)

// AzureClients holds all Azure SDK clients for the Azure Functions backend.
type AzureClients struct {
	WebApps *armappservice.WebAppsClient
	Logs    azurecommon.LogsQuerier
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
	// Same credential as the real-cloud path (DefaultAzureCredential). The
	// only difference between talking to a custom ARM endpoint and to real
	// Azure is coordinates: the endpoint URL(s) below, plus the auth
	// coordinates DefaultAzureCredential reads from the environment
	// (IDENTITY_ENDPOINT/IDENTITY_HEADER for the App Service managed
	// identity, AZURE_* for a service principal). Inside a real Azure
	// Functions app the platform injects those; against a custom endpoint
	// the operator/harness injects the equivalent coordinate pointing at
	// that endpoint's token authority. There is no sim-only credential.
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, err
	}
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
			// Permit sending the bearer token to a plaintext-HTTP ARM
			// endpoint. This relaxes only the resource-request TLS check;
			// the token itself is a real, verifiable token minted by the
			// authority DefaultAzureCredential authenticates against.
			InsecureAllowCredentialWithHTTP: true,
		},
	}

	webAppsClient, err := armappservice.NewWebAppsClient(subscriptionID, cred, opts)
	if err != nil {
		return nil, err
	}

	logsClient, err := azurecommon.NewLogsQuerier(cred, &opts.ClientOptions, endpointURL)
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

	logsClient, err := azurecommon.NewLogsQuerier(cred, nil, "")
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
