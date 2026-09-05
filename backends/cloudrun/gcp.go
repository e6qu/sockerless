package cloudrun

import (
	"context"
	"fmt"
	"os"

	logging "cloud.google.com/go/logging"
	"cloud.google.com/go/logging/logadmin"
	run "cloud.google.com/go/run/apiv2"
	"cloud.google.com/go/storage"
	gcpcommon "github.com/sockerless/gcp-common"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/dns/v1"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

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
//
// In real-cloud mode (endpointURL == ""), all clients hit their canonical
// Google API endpoints (run.googleapis.com, logging.googleapis.com, etc).
//
// In sim mode (endpointURL != ""), the caller MUST also pass
// logAdminEndpoint pointing at the sim's gRPC listener (Cloud Logging is
// a separate API at logging.googleapis.com in production, so the sim
// models it as a separate endpoint URL too). No default derivation —
// the caller wires both explicitly.
func NewGCPClients(ctx context.Context, project, endpointURL, logAdminEndpoint string) (*GCPClients, error) {
	if endpointURL != "" {
		if logAdminEndpoint == "" {
			return nil, fmt.Errorf("NewGCPClients: logAdminEndpoint is required when endpointURL is set " +
				"(Cloud Logging is a distinct API in real GCP; the sim mirrors that — pass the sim's gRPC host:port)")
		}
		return newGCPClientsWithEndpoint(ctx, project, endpointURL, logAdminEndpoint)
	}
	return newGCPClientsDefault(ctx, project)
}

func newGCPClientsWithEndpoint(ctx context.Context, project, endpointURL, logAdminEndpoint string) (*GCPClients, error) {
	// The REST data plane (Cloud Run, Cloud Logging admin JSON, Cloud DNS,
	// Artifact Registry, Cloud Build, Storage) verifies an OAuth2 bearer
	// on every request — exactly as the real Google APIs do. The faithful
	// way to obtain that bearer is the same one a workload uses on real
	// GCE: the GCE metadata server issues a token for the runtime service
	// account. `GCE_METADATA_HOST` is Google's own coordinate for pointing
	// the metadata client at a non-default host; setting it to the sim host
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
		_ = jobsClient.Close()
		_ = execClient.Close()
		_ = servicesClient.Close()
		return nil, err
	}

	// The cloud.google.com/go/storage client honours STORAGE_EMULATOR_HOST
	// (official gcloud emulator convention) and builds canonical
	// `/storage/v1/b...` paths against it. option.WithEndpoint alone
	// makes the client send XML-API-style `/b/...` paths that the GCP
	// sim doesn't route. In emulator mode the client hard-forces
	// WithoutAuthentication, so a bearer cannot be supplied via a token
	// source (it conflicts with WithoutAuthentication and fails client
	// construction). Instead, hand it an *http.Client whose transport
	// injects the same GCE-metadata bearer — this is how a Storage client
	// authenticates against an endpoint that both routes emulator paths
	// and enforces a bearer, and it round-trips the identical sim-signed
	// token the REST clients present.
	storageOpts := []option.ClientOption{
		option.WithHTTPClient(oauth2.NewClient(ctx, tokenSource)),
	}
	if host, err := gcpcommon.URLHost(endpointURL); err == nil {
		_ = os.Setenv("STORAGE_EMULATOR_HOST", host)
	}
	storageClient, err := storage.NewClient(ctx, storageOpts...)
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
