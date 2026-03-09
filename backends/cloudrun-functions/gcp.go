package gcf

import (
	"context"
	"fmt"
	"net/url"
	"strconv"

	functions "cloud.google.com/go/functions/apiv2"
	"cloud.google.com/go/logging/logadmin"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
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

	// logadmin uses gRPC — connect to the simulator's gRPC port (HTTP port + 1)
	grpcAddr, err := grpcAddrFromEndpoint(endpointURL)
	if err != nil {
		_ = functionsClient.Close()
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
		return nil, err
	}

	return &GCPClients{
		Functions: functionsClient,
		LogAdmin:  logAdminClient,
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
