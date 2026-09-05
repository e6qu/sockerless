package gcf

import (
	"context"
	"fmt"
	"os"

	functions "cloud.google.com/go/functions/apiv2"
	"cloud.google.com/go/logging/logadmin"
	run "cloud.google.com/go/run/apiv2"
	"cloud.google.com/go/storage"
	gcpcommon "github.com/sockerless/gcp-common"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
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

func newGCPClientsWithEndpoint(ctx context.Context, project, endpointURL, logAdminEndpoint string) (*GCPClients, error) {
	// The REST data plane (Cloud Run Functions, Cloud Run, Storage,
	// Artifact Registry, Cloud Build) verifies an OAuth2 bearer on every
	// request — exactly as the real Google APIs do. The faithful way to
	// obtain that bearer is the same one a workload uses on real GCE: the
	// GCE metadata server issues a token for the runtime service account.
	// `GCE_METADATA_HOST` is Google's own coordinate for pointing the
	// metadata client at a non-default host; setting it to the sim host
	// (derived from the endpoint URL — the sim serves `/computeMetadata/*`
	// on the same port) makes `google.ComputeTokenSource` fetch a real,
	// sim-signed token. The construction below is identical to the
	// real-cloud path in mechanism (a GCE metadata token source); only the
	// metadata + API coordinates differ. `ComputeTokenSource` short-circuits
	// on `metadata.OnGCE()`, which returns true once `GCE_METADATA_HOST` is
	// set, so set it before creating any client.
	if host, err := gcpcommon.URLHost(endpointURL); err == nil {
		_ = os.Setenv("GCE_METADATA_HOST", host)
	}
	tokenSource := google.ComputeTokenSource("")

	opts := []option.ClientOption{
		option.WithEndpoint(endpointURL),
		option.WithTokenSource(tokenSource),
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

	// Cloud Logging admin is a gRPC data plane. The simulator's gRPC
	// server (SIM_GCP_GRPC_PORT) is a plain `grpc.NewServer()` with no
	// auth interceptor — unlike the HTTP mux it does not enforce a bearer
	// — so the faithful client for it uses an insecure channel and no
	// token (gRPC refuses per-RPC OAuth credentials over an insecure
	// transport anyway). Against real Cloud Logging this same client
	// construction goes through newGCPClientsDefault, which dials TLS with
	// ADC. The difference is purely the transport/endpoint coordinate the
	// sim's gRPC plane exposes.
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
	// same fix as Cloud Run's gcp.go so the JSON-API path is used. In
	// emulator mode the client hard-forces WithoutAuthentication, so the
	// bearer cannot be supplied via a token source (it conflicts and fails
	// construction). Instead hand it an *http.Client whose transport
	// injects the same GCE-metadata bearer the REST clients present, so it
	// authenticates against the enforcing sim while still routing emulator
	// JSON paths.
	storageOpts := []option.ClientOption{
		option.WithHTTPClient(oauth2.NewClient(ctx, tokenSource)),
	}
	if host, err := gcpcommon.URLHost(endpointURL); err == nil {
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
