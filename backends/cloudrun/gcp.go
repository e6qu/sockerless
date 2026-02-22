package cloudrun

import (
	"context"

	logging "cloud.google.com/go/logging"
	"cloud.google.com/go/logging/logadmin"
	run "cloud.google.com/go/run/apiv2"
	"cloud.google.com/go/storage"
	"google.golang.org/api/dns/v1"
	"google.golang.org/api/option"
)

// GCPClients holds all GCP SDK clients.
type GCPClients struct {
	Jobs       *run.JobsClient
	Executions *run.ExecutionsClient
	Logging    *logging.Client
	LogAdmin   *logadmin.Client
	Storage    *storage.Client
	DNS        *dns.Service
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

	jobsClient, err := run.NewJobsRESTClient(ctx, opts...)
	if err != nil {
		return nil, err
	}

	execClient, err := run.NewExecutionsRESTClient(ctx, opts...)
	if err != nil {
		_ = jobsClient.Close()
		return nil, err
	}

	logAdminClient, err := logadmin.NewClient(ctx, project, opts...)
	if err != nil {
		_ = jobsClient.Close()
		_ = execClient.Close()
		return nil, err
	}

	storageClient, err := storage.NewClient(ctx, opts...)
	if err != nil {
		_ = jobsClient.Close()
		_ = execClient.Close()
		_ = logAdminClient.Close()
		return nil, err
	}

	dnsService, err := dns.NewService(ctx, opts...)
	if err != nil {
		_ = jobsClient.Close()
		_ = execClient.Close()
		_ = logAdminClient.Close()
		_ = storageClient.Close()
		return nil, err
	}

	return &GCPClients{
		Jobs:       jobsClient,
		Executions: execClient,
		Logging:    nil, // not used — only logadmin is used for reading logs
		LogAdmin:   logAdminClient,
		Storage:    storageClient,
		DNS:        dnsService,
	}, nil
}

func newGCPClientsDefault(ctx context.Context, project string) (*GCPClients, error) {
	jobsClient, err := run.NewJobsClient(ctx)
	if err != nil {
		return nil, err
	}

	execClient, err := run.NewExecutionsClient(ctx)
	if err != nil {
		_ = jobsClient.Close()
		return nil, err
	}

	loggingClient, err := logging.NewClient(ctx, project)
	if err != nil {
		_ = jobsClient.Close()
		_ = execClient.Close()
		return nil, err
	}

	logAdminClient, err := logadmin.NewClient(ctx, project)
	if err != nil {
		_ = jobsClient.Close()
		_ = execClient.Close()
		_ = loggingClient.Close()
		return nil, err
	}

	storageClient, err := storage.NewClient(ctx)
	if err != nil {
		_ = jobsClient.Close()
		_ = execClient.Close()
		_ = loggingClient.Close()
		_ = logAdminClient.Close()
		return nil, err
	}

	dnsService, err := dns.NewService(ctx)
	if err != nil {
		_ = jobsClient.Close()
		_ = execClient.Close()
		_ = loggingClient.Close()
		_ = logAdminClient.Close()
		_ = storageClient.Close()
		return nil, err
	}

	return &GCPClients{
		Jobs:       jobsClient,
		Executions: execClient,
		Logging:    loggingClient,
		LogAdmin:   logAdminClient,
		Storage:    storageClient,
		DNS:        dnsService,
	}, nil
}
