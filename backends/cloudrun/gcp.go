package cloudrun

import (
	"context"
	"fmt"
	"net/url"
	"os"
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

// urlHost returns "host:port" from a URL, or an error if the URL is
// malformed. Used to build STORAGE_EMULATOR_HOST for cloud.google.com/go/storage.
func urlHost(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	return u.Host, nil
}

// GCPClients holds all GCP SDK clients.
type GCPClients struct {
	Jobs       *run.JobsClient
	Executions *run.ExecutionsClient
	Services   *run.ServicesClient // used when Config.UseService is true
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

	servicesClient, err := run.NewServicesRESTClient(ctx, opts...)
	if err != nil {
		_ = jobsClient.Close()
		_ = execClient.Close()
		return nil, err
	}

	// logadmin uses gRPC — connect to the simulator's gRPC port (HTTP port + 1)
	grpcAddr, err := grpcAddrFromEndpoint(endpointURL)
	if err != nil {
		_ = jobsClient.Close()
		_ = execClient.Close()
		_ = servicesClient.Close()
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
		_ = servicesClient.Close()
		return nil, err
	}

	// The cloud.google.com/go/storage client honours STORAGE_EMULATOR_HOST
	// (used by the official gcloud emulator) and builds the canonical
	// `/storage/v1/b...` paths against it. WithEndpoint alone is NOT
	// enough — the storage client would skip the `/storage/v1/` prefix
	// and send bare `/b` requests that the sim doesn't route. Set the
	// env var so bucket CRUD + object ops hit the right paths.
	if host, err := urlHost(endpointURL); err == nil {
		_ = os.Setenv("STORAGE_EMULATOR_HOST", host)
	}
	storageClient, err := storage.NewClient(ctx, opts...)
	if err != nil {
		_ = jobsClient.Close()
		_ = execClient.Close()
		_ = servicesClient.Close()
		_ = logAdminClient.Close()
		return nil, err
	}

	dnsService, err := dns.NewService(ctx, opts...)
	if err != nil {
		_ = jobsClient.Close()
		_ = execClient.Close()
		_ = servicesClient.Close()
		_ = logAdminClient.Close()
		_ = storageClient.Close()
		return nil, err
	}

	return &GCPClients{
		Jobs:       jobsClient,
		Executions: execClient,
		Services:   servicesClient,
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

	servicesClient, err := run.NewServicesClient(ctx)
	if err != nil {
		_ = jobsClient.Close()
		_ = execClient.Close()
		return nil, err
	}

	loggingClient, err := logging.NewClient(ctx, project)
	if err != nil {
		_ = jobsClient.Close()
		_ = execClient.Close()
		_ = servicesClient.Close()
		return nil, err
	}

	logAdminClient, err := logadmin.NewClient(ctx, project)
	if err != nil {
		_ = jobsClient.Close()
		_ = execClient.Close()
		_ = servicesClient.Close()
		_ = loggingClient.Close()
		return nil, err
	}

	storageClient, err := storage.NewClient(ctx)
	if err != nil {
		_ = jobsClient.Close()
		_ = execClient.Close()
		_ = servicesClient.Close()
		_ = loggingClient.Close()
		_ = logAdminClient.Close()
		return nil, err
	}

	dnsService, err := dns.NewService(ctx)
	if err != nil {
		_ = jobsClient.Close()
		_ = execClient.Close()
		_ = servicesClient.Close()
		_ = loggingClient.Close()
		_ = logAdminClient.Close()
		_ = storageClient.Close()
		return nil, err
	}

	return &GCPClients{
		Jobs:       jobsClient,
		Executions: execClient,
		Services:   servicesClient,
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
