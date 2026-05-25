package gcf

import (
	"context"
	"fmt"
	"net/url"
	"os"

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
//
// In real-cloud mode (endpointURL == ""), all clients hit their canonical
// Google API endpoints (cloudfunctions.googleapis.com, run.googleapis.com,
// logging.googleapis.com, etc).
//
// In sim mode (endpointURL != ""), the caller MUST also pass
// logAdminEndpoint pointing at the sim's gRPC listener. Cloud Logging
// is a distinct API in real GCP (logging.googleapis.com); the sim
// mirrors that — no derivation from endpointURL.
func NewGCPClients(ctx context.Context, project, endpointURL, logAdminEndpoint string) (*GCPClients, error) {
	if endpointURL != "" {
		if logAdminEndpoint == "" {
			return nil, fmt.Errorf("NewGCPClients: logAdminEndpoint is required when endpointURL is set " +
				"(Cloud Logging is a distinct API in real GCP; pass the sim's gRPC host:port)")
		}
		return newGCPClientsWithEndpoint(ctx, project, endpointURL, logAdminEndpoint)
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

func newGCPClientsWithEndpoint(ctx context.Context, project, endpointURL, logAdminEndpoint string) (*GCPClients, error) {
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

	logAdminOpts := []option.ClientOption{
		option.WithEndpoint(logAdminEndpoint),
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
