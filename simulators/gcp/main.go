// Command simulator-gcp runs the GCP service simulator.
//
// It simulates the subset of GCP APIs used by the Sockerless Cloud Run and
// Cloud Functions backends: Cloud Run Jobs, Cloud Logging, Cloud DNS, GCS,
// Artifact Registry, and Cloud Functions v2.
//
// Configure with environment variables:
//
//	SIM_LISTEN_ADDR  — listen address (default ":4567")
//	SIM_TLS_CERT     — TLS certificate file (optional)
//	SIM_TLS_KEY      — TLS key file (optional)
//	SIM_LOG_LEVEL    — log level: trace, debug, info, warn, error (default "info")
//
// SDK configuration:
//
//	option.WithEndpoint("http://localhost:4567")
//	option.WithoutAuthentication()
package main

import (
	"log"
	"os"

	sim "github.com/sockerless/simulator"
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

	// Register GCP service routes
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

	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
