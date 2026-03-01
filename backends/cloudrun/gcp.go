package cloudrun

import (
	"context"
	"fmt"
	"net/url"
	"strconv"

	logging "cloud.google.com/go/logging"
	"cloud.google.com/go/logging/logadmin"
	run "cloud.google.com/go/run/apiv2"
	"cloud.google.com/go/storage"
	"google.golang.org/api/dns/v1"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
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

	// logadmin uses gRPC — connect to the simulator's gRPC port (HTTP port + 1)
	grpcAddr, err := grpcAddrFromEndpoint(endpointURL)
	if err != nil {
		_ = jobsClient.Close()
		_ = execClient.Close()
		return nil, fmt.Errorf("failed to derive gRPC address: %w", err)
	}
	logAdminOpts := []option.ClientOption{
		option.WithEndpoint(grpcAddr),
		option.WithoutAuthentication(),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
	}
	logAdminClient, err := logadmin.NewClient(ctx, project, logAdminOpts...)
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

// grpcAddrFromEndpoint derives the gRPC address (host:port+1) from an HTTP endpoint URL.
// The GCP simulator runs gRPC on the HTTP port + 1.
func grpcAddrFromEndpoint(endpointURL string) (string, error) {
	u, err := url.Parse(endpointURL)
	if err != nil {
		return "", err
	}
	host := u.Hostname()
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		return "", fmt.Errorf("invalid port in endpoint %q: %w", endpointURL, err)
	}
	return fmt.Sprintf("%s:%d", host, port+1), nil
}
