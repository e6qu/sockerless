package aca

import (
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/monitor/azquery"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appcontainers/armappcontainers/v3"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerregistry/armcontainerregistry"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v8"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/privatedns/armprivatedns"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage"
	azurecommon "github.com/sockerless/azure-common"
)

// AzureClients holds all Azure SDK clients.
type AzureClients struct {
	Jobs              *armappcontainers.JobsClient
	Executions        *armappcontainers.JobsExecutionsClient
	ContainerApps     *armappcontainers.ContainerAppsClient // used when Config.UseApp is true
	Logs              azurecommon.LogsQuerier
	PrivateDNSZones   *armprivatedns.PrivateZonesClient
	PrivateDNSRecords *armprivatedns.RecordSetsClient
	NSG               *armnetwork.SecurityGroupsClient
	NSGRules          *armnetwork.SecurityRulesClient
	// Azure Container Registry + its cache-rule sub-resource.
	// Used by the image resolver to rewrite Docker Hub refs through the
	// configured ACR pull-through cache, parallel to AWS ECR + GCP AR.
	Registries    *armcontainerregistry.RegistriesClient
	ACRCacheRules *armcontainerregistry.CacheRulesClient
	// Azure Files + managed-env storages for named volumes.
	// FileShares provisions a share inside the operator-configured
	// storage account; EnvStorages binds the share to the managed
	// environment so Container Apps/Jobs can mount it.
	FileShares  *armstorage.FileSharesClient
	EnvStorages *armappcontainers.ManagedEnvironmentsStoragesClient
	Cred        azcore.TokenCredential
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
	// (IDENTITY_ENDPOINT/IDENTITY_HEADER for the App Service / Container
	// Apps managed identity, AZURE_* for a service principal). Inside a real
	// ACA app the platform injects those; against a custom endpoint the
	// operator/harness injects the equivalent coordinate pointing at that
	// endpoint's token authority. There is no sim-only credential.
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

	jobsClient, err := armappcontainers.NewJobsClient(subscriptionID, cred, opts)
	if err != nil {
		return nil, err
	}

	executionsClient, err := armappcontainers.NewJobsExecutionsClient(subscriptionID, cred, opts)
	if err != nil {
		return nil, err
	}

	containerAppsClient, err := armappcontainers.NewContainerAppsClient(subscriptionID, cred, opts)
	if err != nil {
		return nil, err
	}

	logsClient, err := azurecommon.NewLogsQuerier(cred, &opts.ClientOptions, endpointURL)
	if err != nil {
		return nil, err
	}

	privateZonesClient, err := armprivatedns.NewPrivateZonesClient(subscriptionID, cred, opts)
	if err != nil {
		return nil, err
	}
	recordSetsClient, err := armprivatedns.NewRecordSetsClient(subscriptionID, cred, opts)
	if err != nil {
		return nil, err
	}
	nsgFactory, err := armnetwork.NewClientFactory(subscriptionID, cred, opts)
	if err != nil {
		return nil, err
	}
	acrFactory, err := armcontainerregistry.NewClientFactory(subscriptionID, cred, opts)
	if err != nil {
		return nil, err
	}
	fileSharesClient, err := armstorage.NewFileSharesClient(subscriptionID, cred, opts)
	if err != nil {
		return nil, err
	}
	envStoragesClient, err := armappcontainers.NewManagedEnvironmentsStoragesClient(subscriptionID, cred, opts)
	if err != nil {
		return nil, err
	}

	clients := &AzureClients{
		Jobs:              jobsClient,
		Executions:        executionsClient,
		ContainerApps:     containerAppsClient,
		Logs:              logsClient,
		PrivateDNSZones:   privateZonesClient,
		PrivateDNSRecords: recordSetsClient,
		NSG:               nsgFactory.NewSecurityGroupsClient(),
		NSGRules:          nsgFactory.NewSecurityRulesClient(),
		Registries:        acrFactory.NewRegistriesClient(),
		ACRCacheRules:     acrFactory.NewCacheRulesClient(),
		FileShares:        fileSharesClient,
		EnvStorages:       envStoragesClient,
		Cred:              cred,
	}

	return clients, nil
}

func newAzureClientsDefault(subscriptionID string) (*AzureClients, error) {
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, err
	}

	jobsClient, err := armappcontainers.NewJobsClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, err
	}

	executionsClient, err := armappcontainers.NewJobsExecutionsClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, err
	}

	containerAppsClient, err := armappcontainers.NewContainerAppsClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, err
	}

	logsClient, err := azurecommon.NewLogsQuerier(cred, nil, "")
	if err != nil {
		return nil, err
	}

	privateZonesClient, err := armprivatedns.NewPrivateZonesClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, err
	}
	recordSetsClient, err := armprivatedns.NewRecordSetsClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, err
	}
	nsgFactory, err := armnetwork.NewClientFactory(subscriptionID, cred, nil)
	if err != nil {
		return nil, err
	}
	acrFactory, err := armcontainerregistry.NewClientFactory(subscriptionID, cred, nil)
	if err != nil {
		return nil, err
	}
	fileSharesClient, err := armstorage.NewFileSharesClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, err
	}
	envStoragesClient, err := armappcontainers.NewManagedEnvironmentsStoragesClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, err
	}

	return &AzureClients{
		Jobs:              jobsClient,
		Executions:        executionsClient,
		ContainerApps:     containerAppsClient,
		Logs:              logsClient,
		PrivateDNSZones:   privateZonesClient,
		PrivateDNSRecords: recordSetsClient,
		NSG:               nsgFactory.NewSecurityGroupsClient(),
		NSGRules:          nsgFactory.NewSecurityRulesClient(),
		Registries:        acrFactory.NewRegistriesClient(),
		ACRCacheRules:     acrFactory.NewCacheRulesClient(),
		FileShares:        fileSharesClient,
		EnvStorages:       envStoragesClient,
		Cred:              cred,
	}, nil
}
