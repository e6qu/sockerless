package gcf

import (
	"context"

	functions "cloud.google.com/go/functions/apiv2"
	"cloud.google.com/go/logging/logadmin"
	"google.golang.org/api/option"
)

// GCPClients holds all GCP SDK clients for the Cloud Run Functions backend.
type GCPClients struct {
	Functions *functions.FunctionClient
	LogAdmin  *logadmin.Client
}

// NewGCPClients initializes GCP SDK clients.
func NewGCPClients(ctx context.Context, project string, endpointURL string) (*GCPClients, error) {
	if endpointURL != "" {
		return newGCPClientsWithEndpoint(ctx, project, endpointURL)
	}
	return newGCPClientsDefault(ctx, project)
}

func newGCPClientsWithEndpoint(ctx context.Context, project string, endpointURL string) (*GCPClients, error) {
	opts := []option.ClientOption{
		option.WithEndpoint(endpointURL),
		option.WithoutAuthentication(),
	}

	functionsClient, err := functions.NewFunctionRESTClient(ctx, opts...)
	if err != nil {
		return nil, err
	}

	logAdminClient, err := logadmin.NewClient(ctx, project, opts...)
	if err != nil {
		_ = functionsClient.Close()
		return nil, err
	}

	return &GCPClients{
		Functions: functionsClient,
		LogAdmin:  logAdminClient,
	}, nil
}

func newGCPClientsDefault(ctx context.Context, project string) (*GCPClients, error) {
	functionsClient, err := functions.NewFunctionClient(ctx)
	if err != nil {
		return nil, err
	}

	logAdminClient, err := logadmin.NewClient(ctx, project)
	if err != nil {
		_ = functionsClient.Close()
		return nil, err
	}

	return &GCPClients{
		Functions: functionsClient,
		LogAdmin:  logAdminClient,
	}, nil
}
