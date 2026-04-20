package aca

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/monitor/azquery"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appcontainers/armappcontainers/v2"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerregistry/armcontainerregistry"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v7"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/privatedns/armprivatedns"
)

type fakeCredential struct{}

func (f *fakeCredential) GetToken(_ context.Context, _ policy.TokenRequestOptions) (azcore.AccessToken, error) {
	return azcore.AccessToken{Token: "fake-token", ExpiresOn: time.Now().Add(time.Hour)}, nil
}

// AzureClients holds all Azure SDK clients.
type AzureClients struct {
	Jobs              *armappcontainers.JobsClient
	Executions        *armappcontainers.JobsExecutionsClient
	ContainerApps     *armappcontainers.ContainerAppsClient // Phase 88: used when Config.UseApp is true
	Logs              *azquery.LogsClient
	LogsHTTP          *httpLogsClient // Used when endpoint is HTTP (SDK rejects non-TLS bearer tokens)
	PrivateDNSZones   *armprivatedns.PrivateZonesClient
	PrivateDNSRecords *armprivatedns.RecordSetsClient
	NSG               *armnetwork.SecurityGroupsClient
	NSGRules          *armnetwork.SecurityRulesClient
	// Azure Container Registry + its cache-rule sub-resource.
	// Used by the image resolver to rewrite Docker Hub refs through the
	// configured ACR pull-through cache, parallel to AWS ECR + GCP AR.
	Registries    *armcontainerregistry.RegistriesClient
	ACRCacheRules *armcontainerregistry.CacheRulesClient
	Cred          azcore.TokenCredential
}

// httpLogsClient makes direct HTTP calls to Log Analytics when the Azure SDK's
// BearerTokenPolicy rejects non-TLS endpoints. This is needed because azquery
// v1.2.0 doesn't propagate InsecureAllowCredentialWithHTTP to its auth policy.
type httpLogsClient struct {
	endpoint string
}

func (c *httpLogsClient) QueryWorkspace(ctx context.Context, workspaceID string, body azquery.Body, _ *azquery.LogsClientQueryWorkspaceOptions) (azquery.LogsClientQueryWorkspaceResponse, error) {
	reqBody, _ := json.Marshal(body)
	url := fmt.Sprintf("%s/workspaces/%s/query", c.endpoint, workspaceID)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return azquery.LogsClientQueryWorkspaceResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return azquery.LogsClientQueryWorkspaceResponse{}, err
	}
	defer resp.Body.Close()

	var result azquery.LogsClientQueryWorkspaceResponse
	if err := json.NewDecoder(resp.Body).Decode(&result.Results); err != nil {
		return azquery.LogsClientQueryWorkspaceResponse{}, err
	}
	return result, nil
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

	logsClient, err := azquery.NewLogsClient(cred, &azquery.LogsClientOptions{
		ClientOptions: opts.ClientOptions,
	})
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
		Cred:              cred,
	}

	// azquery v1.2.0 doesn't propagate InsecureAllowCredentialWithHTTP to its
	// BearerTokenPolicy, causing QueryWorkspace to fail over HTTP endpoints.
	// Use a direct HTTP client for non-TLS endpoints.
	if strings.HasPrefix(endpointURL, "http://") {
		clients.LogsHTTP = &httpLogsClient{endpoint: endpointURL}
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

	logsClient, err := azquery.NewLogsClient(cred, nil)
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
		Cred:              cred,
	}, nil
}
