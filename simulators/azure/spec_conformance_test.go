package main

import (
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	sim "github.com/sockerless/simulator"
)

// These tests enforce the simulator's core fidelity invariant: every HTTP
// route the Azure simulator serves must be a REAL Azure API path — the
// simulator must not invent paths under the ARM / data-plane namespaces.
// The registered route table (built in-process via buildSimulator) is
// validated against the official Swagger 2.0 specs vendored (gzipped,
// pinned) under specs/cloud-api/azure/. Refresh with
// scripts/fetch-azure-spec.sh.
//
// Scope: routes registered on the server mux. The host-addressed storage
// data planes (blob/file/queue/table on *.localhost subdomains) attach
// via WrapHandler middlewares, not mux patterns, so they are exercised by
// the SDK test suites rather than this static gate.

type swaggerPath struct {
	Method string
	Segs   []string
}

func loadSwaggerPaths(t *testing.T) (all []swaggerPath, byFile map[string][]swaggerPath) {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("..", "..", "specs", "cloud-api", "azure", "*.swagger.json.gz"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no vendored Swagger specs found (glob err: %v) — run scripts/fetch-azure-spec.sh", err)
	}

	byFile = map[string][]swaggerPath{}
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			t.Fatalf("open %s: %v", p, err)
		}
		gz, err := gzip.NewReader(f)
		if err != nil {
			t.Fatalf("gunzip %s: %v", p, err)
		}
		var doc struct {
			BasePath string                                `json:"basePath"`
			Paths    map[string]map[string]json.RawMessage `json:"paths"`
			XMSPaths map[string]map[string]json.RawMessage `json:"x-ms-paths"`
		}
		err = json.NewDecoder(gz).Decode(&doc)
		_ = gz.Close()
		_ = f.Close()
		if err != nil {
			t.Fatalf("decode %s: %v", p, err)
		}

		file := filepath.Base(p)
		// The Log Analytics query data plane is served under the /v1
		// endpoint prefix (https://api.loganalytics.io/v1/...), which its
		// swagger expresses via the host version rather than basePath.
		if doc.BasePath == "" && strings.HasPrefix(file, "monitor-dataplane-operationalinsights-v1") {
			doc.BasePath = "/v1"
		}
		// The EventGrid publish data plane's swagger folds the whole
		// publish URL into its x-ms-parameterized-host template; its
		// x-ms-paths keys are pure query-string overloads ("?overload=
		// cloudEvent"). The documented publish URL is
		// https://<topic>.<region>.eventgrid.azure.net/api/events.
		if strings.HasPrefix(file, "eventgrid-dataplane-") {
			doc.BasePath = "/api/events"
		}
		add := func(rawPath string, methods map[string]json.RawMessage) {
			// x-ms-paths keys may carry a query discriminator
			// ("/path?comp=list"); routing matches on the path part.
			if i := strings.Index(rawPath, "?"); i >= 0 {
				rawPath = rawPath[:i]
			}
			full := rawPath
			if doc.BasePath != "" && doc.BasePath != "/" {
				full = strings.TrimSuffix(doc.BasePath, "/") + "/" + strings.TrimPrefix(rawPath, "/")
			}
			segs := splitAzureSegs(full)
			for method := range methods {
				switch method {
				case "get", "put", "post", "patch", "delete", "head", "options":
					sp := swaggerPath{Method: strings.ToUpper(method), Segs: segs}
					all = append(all, sp)
					byFile[file] = append(byFile[file], sp)
				}
			}
		}
		for rawPath, methods := range doc.Paths {
			add(rawPath, methods)
		}
		for rawPath, methods := range doc.XMSPaths {
			add(rawPath, methods)
		}
	}
	return all, byFile
}

// Path normalization + matching (splitAzureSegs / matchAzureSegs) live
// in spec_validator.go — the runtime wire-shape validator and this
// static gate share one matcher.

func matchesAnySwagger(specs []swaggerPath, method, path string) bool {
	simSegs := splitAzureSegs(path)
	for _, s := range specs {
		if s.Method == method && matchAzureSegs(simSegs, s.Segs, true) {
			return true
		}
	}
	return false
}

// allowedNonSpecAzurePrefixes lists route families that are real,
// documented Azure wire surfaces with NO swagger in
// Azure/azure-rest-api-specs, plus the simulator's own control surface.
// Each entry MUST be justified — never a place to hide an invented path.
var allowedNonSpecAzurePrefixes = map[string]string{
	"/sim/v1/": "simulator control + dashboard surface (sockerless-specific)",

	// Cosmos DB SQL (core) data plane — documented REST API
	// (/dbs/{db}/colls/{coll}/docs) has no swagger in azure-rest-api-specs.
	"/dbs": "Cosmos DB SQL data plane (documented REST API, no upstream swagger)",

	// Microsoft Graph — real Graph v1.0 endpoints (users/groups/me);
	// Graph's OpenAPI lives in microsoftgraph/msgraph-metadata, not
	// azure-rest-api-specs.
	"/v1.0/": "Microsoft Graph v1.0 surface (spec lives in msgraph-metadata)",

	// Azure Functions host — the per-function invoke endpoint of the
	// Functions host runtime; no swagger exists.
	"/api/function": "Functions host invoke endpoint",

	// IMDS-adjacent identity endpoints: managed-identity token via IMDS
	// (/metadata/identity/oauth2/token), App Service MSI (/msi/token),
	// and the ARM /metadata/endpoints bootstrap used by terraform's
	// metadata_host — none are described in azure-rest-api-specs.
	"/metadata/identity/": "IMDS managed-identity token endpoint",
	"/msi/token":          "App Service MSI token endpoint",
	"/metadata/endpoints": "ARM cloud-environment metadata bootstrap (terraform metadata_host)",

	// ServiceBus HTTP data plane message operations — the vendored
	// 2021-05 data-plane swagger covers only the ATOM entity-management
	// surface; send/receive/lock REST operations are documented but
	// unswaggered.
	"/{entity}/messages": "ServiceBus HTTP message send/receive (documented, no upstream swagger)",
}

// allowedNonSpecAzureRoutes — exact-pattern variant of the above.
// (AAD/Entra OAuth2 token endpoints are intercepted ahead of the mux;
// the ACR exchange flow's /oauth2/* mux routes are covered by the ACR
// data-plane swagger and match there.)
var allowedNonSpecAzureRoutes = map[string]string{
	// ARM long-running-operation polling: clients poll whatever URL the
	// Azure-AsyncOperation / Location header carries — the URL shape is
	// service-internal and never appears in a swagger. These are the
	// sim-emitted polling targets.
	"GET /subscriptions/{subscriptionId}/providers/Microsoft.App/locations/{location}/operationStatuses/{opId}":                  "sim-emitted LRO polling URL (Azure-AsyncOperation)",
	"GET /subscriptions/{subscriptionId}/providers/Microsoft.KeyVault/locations/{location}/deletedVaults/{name}/purge/operation": "sim-emitted Azure Key Vault purge LRO polling URL (documented Location header)",
	"GET /subscriptions/{subscriptionId}/providers/{provider}/locations/{location}/operationResults/{opId}":                      "sim-emitted LRO polling URL (Location header)",
	"GET /subscriptions/{subscriptionId}/providers/{provider}/locations/{location}/operationStatuses/{opId}":                     "sim-emitted LRO polling URL (Azure-AsyncOperation)",

	// Exec bridges: the real APIs return an opaque WebSocket URI
	// (webSocketUri / execEndpoint) that clients connect to verbatim;
	// the sim shapes its own bridge URLs ARM-style. The session URL is
	// sim-emitted, never client-constructed.
	"GET /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.ContainerInstance/containerGroups/{containerGroupName}/containers/{containerName}/execSessions/{sessionID}":   "sim-emitted exec WebSocket bridge (real API returns an opaque webSocketUri)",
	"GET /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.ContainerInstance/containerGroups/{containerGroupName}/containers/{containerName}/attachSessions/{sessionID}": "sim-emitted attach WebSocket bridge (real API returns an opaque webSocketUri)",
	"POST /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.App/jobs/{jobName}/executions/{execName}/exec":                                                               "sim-emitted exec WebSocket bridge (real API returns an opaque execEndpoint)",
}

// vendoredForRefClosure lists swagger files vendored only because a
// path-bearing file the simulator implements $refs into them (the
// shape-validation layer needs the definitions); their own paths are
// services the simulator does not implement.
var vendoredForRefClosure = map[string]bool{
	"cosmos-db-arm-privateendpointconnection-2021-10-15.swagger.json.gz": true,
	"cosmos-db-arm-privateendpointconnection-2024-08-15.swagger.json.gz": true,
	"network-arm-applicationgateway-2025-03-01.swagger.json.gz":          true,
	"network-arm-applicationsecuritygroup-2025-03-01.swagger.json.gz":    true,
	"network-arm-networkmanager-2025-03-01.swagger.json.gz":              true,
	"network-arm-networkprofile-2025-03-01.swagger.json.gz":              true,
	"network-arm-networkwatcher-2025-03-01.swagger.json.gz":              true,
	"network-arm-privateendpoint-2025-03-01.swagger.json.gz":             true,
	"network-arm-privatelinkservice-2025-03-01.swagger.json.gz":          true,
	"network-arm-serviceendpointpolicy-2025-03-01.swagger.json.gz":       true,
	"network-arm-virtualnetworktap-2025-03-01.swagger.json.gz":           true,
}

// vendoredHostRoutedDataPlane lists swagger files for data planes the
// simulator serves through host-addressed WrapHandler middlewares
// (*.vault.azure.net-style subdomains) rather than mux patterns — the
// static route gate cannot see them; the SDK test suites exercise them.
var vendoredHostRoutedDataPlane = map[string]bool{
	"keyvault-dataplane-certificates-2025-07-01.swagger.json.gz": true,
	"keyvault-dataplane-keys-2025-07-01.swagger.json.gz":         true,
	"keyvault-dataplane-secrets-2025-07-01.swagger.json.gz":      true,
}

func TestRoutesExistInSwaggerSpecs(t *testing.T) {
	all, byFile := loadSwaggerPaths(t)
	_ = byFile
	t.Setenv("SIM_RUNTIME", "process")
	srv, err := buildSimulator(sim.Config{Provider: "azure", ListenAddr: ":0", LogLevel: "error"})
	if err != nil {
		t.Fatalf("buildSimulator: %v", err)
	}

	var offenders []string
	for _, pattern := range srv.RoutePatterns() {
		if pattern == "GET /health" {
			continue // shared simulator health endpoint
		}
		if _, ok := allowedNonSpecAzureRoutes[pattern]; ok {
			continue
		}
		method, path, found := strings.Cut(pattern, " ")
		if !found {
			offenders = append(offenders, pattern+"  (method-less pattern)")
			continue
		}
		prefixAllowed := false
		for prefix := range allowedNonSpecAzurePrefixes {
			if strings.HasPrefix(path, prefix) {
				prefixAllowed = true
				break
			}
		}
		if prefixAllowed {
			continue
		}
		if !matchesAnySwagger(all, method, path) {
			offenders = append(offenders, pattern)
		}
	}

	if len(offenders) > 0 {
		sort.Strings(offenders)
		t.Errorf("%d registered route(s) do not exist in the vendored Azure Swagger specs (invented path, wrong path shape, or a real-but-unswaggered surface that needs a justified allowlist entry):\n  %s",
			len(offenders), strings.Join(offenders, "\n  "))
	}
}

// TestVendoredSwaggerSpecsAreConsumed flags vendored swagger files no
// registered route references. Pure-definitions files (common-types,
// CommonDefinitions) carry no paths and are exempt — they exist as $ref
// targets for the shape-validation layer.
func TestVendoredSwaggerSpecsAreConsumed(t *testing.T) {
	_, byFile := loadSwaggerPaths(t)
	t.Setenv("SIM_RUNTIME", "process")
	srv, err := buildSimulator(sim.Config{Provider: "azure", ListenAddr: ":0", LogLevel: "error"})
	if err != nil {
		t.Fatalf("buildSimulator: %v", err)
	}

	used := map[string]bool{}
	for _, pattern := range srv.RoutePatterns() {
		method, path, found := strings.Cut(pattern, " ")
		if !found {
			continue
		}
		for file, specs := range byFile {
			if !used[file] && matchesAnySwagger(specs, method, path) {
				used[file] = true
			}
		}
	}

	var stale []string
	for file, specs := range byFile {
		if len(specs) == 0 {
			continue // pure-definitions file ($ref target)
		}
		if vendoredForRefClosure[file] || vendoredHostRoutedDataPlane[file] {
			continue
		}
		if !used[file] {
			stale = append(stale, file)
		}
	}
	if len(stale) > 0 {
		sort.Strings(stale)
		t.Errorf("%d vendored swagger file(s) with paths are not referenced by any registered route (stale vendor, wrong api-version, or a data plane outside the mux — move those to the documented exemptions):\n  %s",
			len(stale), strings.Join(stale, "\n  "))
	}
}
