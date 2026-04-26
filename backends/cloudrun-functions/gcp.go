package gcf

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strconv"

	functions "cloud.google.com/go/functions/apiv2"
	"cloud.google.com/go/logging/logadmin"
	run "cloud.google.com/go/run/apiv2"
	"cloud.google.com/go/storage"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// GCPClients holds all GCP SDK clients for the Cloud Run Functions backend.
type GCPClients struct {
	Functions *functions.FunctionClient
	LogAdmin  *logadmin.Client
	// Services client is the escape hatch for GCS volumes —
	// Functions v2's ServiceConfig exposes only SecretVolumes, so every
	// other volume must be attached via the underlying Cloud Run
	// Service resource (`fn.ServiceConfig.Service`).
	Services *run.ServicesClient
	// Storage client for provisioning sockerless-managed GCS buckets
	// backing named volumes (reused across GCP backends via
	// gcpcommon.BucketManager).
	Storage *storage.Client
}

// NewGCPClients initializes GCP SDK clients.
func NewGCPClients(ctx context.Context, project string, endpointURL string) (*GCPClients, error) {
	if endpointURL != "" {
		return newGCPClientsWithEndpoint(ctx, project, endpointURL)
	}
	return newGCPClientsDefault(ctx, project)
}

// urlHost returns "host:port" from a URL, or an error if malformed.
func urlHost(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	return u.Host, nil
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

	servicesClient, err := run.NewServicesRESTClient(ctx, opts...)
	if err != nil {
		_ = functionsClient.Close()
		return nil, err
	}

	// logadmin uses gRPC — connect to the simulator's gRPC port (HTTP port + 1)
	grpcAddr, err := grpcAddrFromEndpoint(endpointURL)
	if err != nil {
		_ = functionsClient.Close()
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
		_ = functionsClient.Close()
		_ = servicesClient.Close()
		return nil, err
	}

	// Storage client honours STORAGE_EMULATOR_HOST, not option.WithEndpoint —
	// same fix as Cloud Run's gcp.go so the JSON-API path is used.
	storageOpts := []option.ClientOption{option.WithoutAuthentication()}
	if host, err := urlHost(endpointURL); err == nil {
		_ = os.Setenv("STORAGE_EMULATOR_HOST", host)
	}
	storageClient, err := storage.NewClient(ctx, storageOpts...)
	if err != nil {
		_ = functionsClient.Close()
		_ = servicesClient.Close()
		_ = logAdminClient.Close()
		return nil, err
	}

	return &GCPClients{
		Functions: functionsClient,
		LogAdmin:  logAdminClient,
		Services:  servicesClient,
		Storage:   storageClient,
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

func newGCPClientsDefault(ctx context.Context, project string) (*GCPClients, error) {
	functionsClient, err := functions.NewFunctionClient(ctx)
	if err != nil {
		return nil, err
	}

	servicesClient, err := run.NewServicesClient(ctx)
	if err != nil {
		_ = functionsClient.Close()
		return nil, err
	}

	logAdminClient, err := logadmin.NewClient(ctx, project)
	if err != nil {
		_ = functionsClient.Close()
		_ = servicesClient.Close()
		return nil, err
	}

	storageClient, err := storage.NewClient(ctx)
	if err != nil {
		_ = functionsClient.Close()
		_ = servicesClient.Close()
		_ = logAdminClient.Close()
		return nil, err
	}

	return &GCPClients{
		Functions: functionsClient,
		LogAdmin:  logAdminClient,
		Services:  servicesClient,
		Storage:   storageClient,
	}, nil
}
