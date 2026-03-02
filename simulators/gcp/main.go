// Command simulator-gcp runs the GCP service simulator.
//
// It simulates the subset of GCP APIs used by the Sockerless Cloud Run and
// Cloud Functions backends: Cloud Run Jobs, Cloud Logging, Cloud DNS, GCS,
// Artifact Registry, and Cloud Functions v2.
//
// Configure with environment variables:
//
//	SIM_LISTEN_ADDR     — HTTP listen address (default ":4567")
//	SIM_GCP_GRPC_PORT   — gRPC listen port for Cloud Logging (default: HTTP port + 1)
//	SIM_TLS_CERT        — TLS certificate file (optional)
//	SIM_TLS_KEY         — TLS key file (optional)
//	SIM_LOG_LEVEL       — log level: trace, debug, info, warn, error (default "info")
//
// SDK configuration:
//
//	option.WithEndpoint("http://localhost:4567")
//	option.WithoutAuthentication()
package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"

	sim "github.com/sockerless/simulator"
	"google.golang.org/grpc"
)

func main() {
	cfg := sim.ConfigFromEnv("gcp")
	if cfg.ListenAddr == ":8443" {
		cfg.ListenAddr = ":4567" // GCP simulator default port
	}

	if port := os.Getenv("SIM_GCP_PORT"); port != "" {
		cfg.ListenAddr = ":" + port
	}

	srv := sim.NewServer(cfg)

	// Register GCP service routes (HTTP/REST)
	registerCloudRunJobs(srv)
	registerCloudLogging(srv)
	registerCloudDNS(srv)
	registerGCS(srv)
	registerArtifactRegistry(srv)
	registerCloudFunctions(srv)
	registerOperations(srv)

	// Infrastructure services
	registerServiceUsage(srv)
	registerCompute(srv)
	registerVPCAccess(srv)
	registerIAM(srv)

	// Dashboard summary endpoints for UI
	registerDashboard(srv)

	// Embedded UI (no-op with -tags noui)
	registerUI(srv)

	// Start gRPC server for Cloud Logging
	grpcPort := grpcPortFromConfig(cfg.ListenAddr)
	if p := os.Getenv("SIM_GCP_GRPC_PORT"); p != "" {
		grpcPort = p
	}
	go startGRPCServer(grpcPort)

	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

// grpcPortFromConfig derives the gRPC port from the HTTP listen address.
// Default: HTTP port + 1.
func grpcPortFromConfig(listenAddr string) string {
	// Extract port from ":4567" or "0.0.0.0:4567"
	_, portStr, err := net.SplitHostPort(listenAddr)
	if err != nil {
		// Might be just ":4567"
		portStr = strings.TrimPrefix(listenAddr, ":")
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "4568"
	}
	return strconv.Itoa(port + 1)
}

func startGRPCServer(port string) {
	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("gRPC: failed to listen on :%s: %v", port, err)
	}

	gs := grpc.NewServer()
	registerCloudLoggingGRPC(gs)

	fmt.Fprintf(os.Stderr, "  gRPC Cloud Logging on :%s\n", port)
	if err := gs.Serve(lis); err != nil {
		log.Fatalf("gRPC: failed to serve: %v", err)
	}
}
