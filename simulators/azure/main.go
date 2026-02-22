// Command simulator-azure runs the Azure service simulator.
//
// It simulates the subset of Azure APIs used by the Sockerless ACA and
// Azure Functions backends: Container Apps Jobs, Azure Monitor, Azure Files,
// ACR, Private DNS, Azure Functions, and Application Insights.
//
// Configure with environment variables:
//
//	SIM_LISTEN_ADDR  — listen address (default ":4568")
//	SIM_TLS_CERT     — TLS certificate file (optional)
//	SIM_TLS_KEY      — TLS key file (optional)
//	SIM_LOG_LEVEL    — log level: trace, debug, info, warn, error (default "info")
//
// SDK configuration:
//
//	Use custom cloud.Configuration with ARM endpoint http://localhost:4568
package main

import (
	"log"
	"os"

	sim "github.com/sockerless/simulator"
)

func main() {
	cfg := sim.ConfigFromEnv("azure")
	if cfg.ListenAddr == ":8443" {
		cfg.ListenAddr = ":4568" // Azure simulator default port
	}

	if port := os.Getenv("SIM_AZURE_PORT"); port != "" {
		cfg.ListenAddr = ":" + port
	}

	srv := sim.NewServer(cfg)

	// Clean double slashes in request paths. The azurerm v3 provider (via
	// go-azure-sdk) appends a trailing slash to the resourceManager endpoint,
	// producing paths like //subscriptions/... Go's default mux would 301
	// redirect these, which changes PUT→GET and breaks creates.
	srv.WrapHandler(CleanPathMiddleware)

	// Wrap with auth middleware for OAuth2 token requests (must be outside
	// the mux to avoid route conflicts with ACR's /v2/{path...})
	srv.WrapHandler(AzureAuthMiddleware)

	// Register Azure service routes
	registerContainerApps(srv)
	registerAzureMonitor(srv)
	registerAzureFiles(srv)
	registerACR(srv)
	registerPrivateDNS(srv)
	registerAzureFunctions(srv)
	registerApplicationInsights(srv)

	// Cloud metadata (for Terraform provider metadata_host)
	registerMetadata(srv)

	// Infrastructure services
	registerResourceGroups(srv)
	registerNetwork(srv)
	registerManagedIdentity(srv)
	registerAuthorization(srv)
	registerContainerAppEnvironment(srv)
	registerAppServicePlan(srv)
	registerSubscription(srv)

	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
