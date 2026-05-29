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
//	SIM_SERVICEBUS_AMQP_LISTEN_ADDR — raw Service Bus AMQP/TLS listen address (optional)
//	SIM_LOG_LEVEL    — log level: trace, debug, info, warn, error (default "info")
//
// SDK configuration:
//
//	Use custom cloud.Configuration with ARM endpoint http://localhost:4568
package main

import (
	"context"
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
	if _, err := azureAdvertisedEndpointConfigFromEnv(); err != nil {
		log.Fatal(err)
	}

	// Stash listen addr so cloud-product translators can wire
	// IDENTITY_ENDPOINT + IMDS env onto workload containers.
	simListenAddr = cfg.ListenAddr

	obs, err := sim.InitObservability("sockerless-sim-azure")
	if err != nil {
		log.Fatalf("init observability: %v", err)
	}
	defer func() { _ = obs.Shutdown(context.Background()) }()
	cfg.LogWriter = obs.LogWriter

	srv, err := sim.NewServer(cfg)
	if err != nil {
		log.Fatalf("simulator startup: %v", err)
	}

	// Clean double slashes in request paths. The azurerm v3 provider (via
	// go-azure-sdk) appends a trailing slash to the resourceManager endpoint,
	// producing paths like //subscriptions/... Go's default mux would 301
	// redirect these, which changes PUT→GET and breaks creates.
	srv.WrapHandler(CleanPathMiddleware)

	// Wrap with auth middleware for OAuth2 token requests (must be outside
	// the mux to avoid route conflicts with ACR's /v2/{path...})
	srv.WrapHandler(AzureAuthMiddleware)

	// Register Azure service routes
	// Monitor must be registered first to initialize monitorLogs store
	// used by Container Apps and Functions log injection.
	registerAzureMonitor(srv)
	registerContainerApps(srv)
	registerContainerAppsApps(srv)
	registerAzureFiles(srv)
	registerACR(srv)
	registerPrivateDNS(srv)
	registerAzureFunctions(srv)
	registerApplicationInsights(srv)

	// Cloud metadata (for Terraform provider metadata_host)
	registerMetadata(srv)

	// Infrastructure services
	registerResourceGroups(srv)
	registerTags(srv)
	registerNetwork(srv)
	registerManagedIdentity(srv)
	registerKeyVault(srv)
	registerPublicDNS(srv)
	registerBlobDataPlane(srv)
	registerStorageDataPlane(srv)
	registerCacheRedis(srv)
	registerPGFlexibleServer(srv)
	registerCosmosDB(srv)
	registerServiceBus(srv)
	registerServiceBusDataPlane(srv)
	registerEventHubs(srv)
	registerEventGrid(srv)
	registerAPIM(srv)
	registerAuthorization(srv)
	registerContainerAppEnvironment(srv)
	registerAppServicePlan(srv)
	registerSubscription(srv)

	// Dashboard summary endpoints for UI
	registerDashboard(srv)

	// Embedded UI (no-op with -tags noui)
	registerUI(srv)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if addr, enabled, err := startAzureDNSFromEnv(ctx); err != nil {
		log.Fatal(err)
	} else if enabled {
		log.Printf("Azure simulator DNS listening on %s", addr)
	}
	if amqpAddr := os.Getenv("SIM_SERVICEBUS_AMQP_LISTEN_ADDR"); amqpAddr != "" {
		certFile := envOrDefault("SIM_SERVICEBUS_AMQP_TLS_CERT", cfg.TLSCert)
		keyFile := envOrDefault("SIM_SERVICEBUS_AMQP_TLS_KEY", cfg.TLSKey)
		ln, err := startSBAMQPTLSListener(ctx, amqpAddr, certFile, keyFile)
		if err != nil {
			log.Fatal(err)
		}
		defer func() { _ = ln.Close() }()
		log.Printf("Service Bus raw AMQP/TLS listening on %s", ln.Addr())
	}

	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
