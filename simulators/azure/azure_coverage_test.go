package main

import (
	"sort"
	"strings"
	"testing"

	sim "github.com/sockerless/simulator"
)

// Azure operation-coverage gate — the Swagger-spec analogue of the AWS
// service-conformance ratchet and the GCP Discovery-doc ratchet. For each
// vendored Azure ARM Swagger document it counts how many of the service's
// operations the simulator implements: an operation is "covered" when some
// registered route matches its HTTP method + (normalized) path under the same
// matchAzureSegs rules the route-validity gate already uses. The count is locked
// by azureMethodFloor — a drop is a regression, an increase is a ratchet-up that
// must bump the floor. This makes Azure coverage a measured, gated number rather
// than something discovered later by a consumer.

// azureMethodFloor locks the implemented-operation COUNT per vendored Azure
// Swagger document (keyed by file name without the .swagger.json.gz suffix).
// Implement an operation (or grow the vendored spec) and the matching floor must
// move with it. The bulk of the large surfaces (web-arm ~692, cosmos-db ~124,
// logic ~106) is intentionally far from 100% — the floor records the honest
// implemented count, not an aspiration.
var azureMethodFloor = map[string]int{
	"apimanagement-arm-apimapis-2022-08-01":                           91,
	"apimanagement-arm-apimbackends-2022-08-01":                       7,
	"apimanagement-arm-apimdeletedservices-2022-08-01":                3,
	"apimanagement-arm-apimdeployment-2022-08-01":                     15,
	"apimanagement-arm-apimnamedvalues-2022-08-01":                    8,
	"apimanagement-arm-apimproducts-2022-08-01":                       31,
	"apimanagement-arm-apimsubscriptions-2022-08-01":                  9,
	"app-arm-containerapps-2025-01-01":                                11,
	"app-arm-jobs-2025-01-01":                                         12,
	"app-arm-managedenvironments-2025-01-01":                          19,
	"app-arm-managedenvironmentsstorages-2025-01-01":                  4,
	"applicationinsights-arm-components_api-2020-02-02":               8,
	"applicationinsights-arm-featuresandpricing-2015-05-01":           2,
	"applicationinsights-dataplane-appinsights-v1-preview":            1,
	"authorization-arm-authorization-roleassignmentscalls-2022-04-01": 9,
	"authorization-arm-authorization-roledefinitionscalls-2022-04-01": 5,
	"compute-arm-computerpcommon-2022-03-01":                          1,
	"compute-arm-skus-2021-07-01":                                     1,
	"compute-arm-virtualmachine-2022-03-01":                           11,
	"containerinstance-arm-containerinstance-2021-10-01":              18,
	"containerregistry-arm-containerregistry-2023-07-01":              52,
	"containerregistry-arm-containerregistry-2025-11-01":              58,
	"containerregistry-arm-registrytasks-2019-06-01-preview":          25,
	"containerregistry-dataplane-containerregistry-2021-07-01":        13,
	"cosmos-db-arm-cosmos-db-2021-10-15":                              121,
	"cosmos-db-arm-cosmos-db-2024-08-15":                              124,
	"cosmos-db-arm-privateendpointconnection-2021-10-15":              4,
	"cosmos-db-arm-privateendpointconnection-2024-08-15":              4,
	"cosmos-db-dataplane-table-2019-02-02":                            3,
	"dns-arm-dns-2018-05-01":                                          14,
	"eventgrid-arm-eventgrid-2021-12-01":                              61,
	"eventgrid-arm-eventgrid-2022-06-15":                              127,
	"eventgrid-dataplane-eventgrid-2018-01-01":                        3,
	"eventhub-arm-authorizationrules-2024-01-01":                      15,
	"eventhub-arm-consumergroups-2024-01-01":                          4,
	"eventhub-arm-eventhubs-2024-01-01":                               4,
	"eventhub-arm-namespaces-2024-01-01":                              14,
	"eventhub-arm-networkrulessets-2024-01-01":                        3,
	"imds-dataplane-imds-2021-02-01":                                  2,
	"keyvault-arm-keyvault-2023-07-01":                                17,
	"keyvault-dataplane-certificates-2025-07-01":                      0,
	"keyvault-dataplane-keys-2025-07-01":                              0,
	"keyvault-dataplane-secrets-2025-07-01":                           0,
	"logic-arm-logic-2019-05-01":                                      106,
	"monitor-dataplane-datacollectionrules-2023-01-01":                1,
	"monitor-dataplane-operationalinsights-v1":                        5,
	"msi-arm-managedidentity-2024-11-30":                              12,
	"network-arm-applicationgateway-2025-03-01":                       0,
	"network-arm-applicationsecuritygroup-2025-03-01":                 0,
	"network-arm-loadbalancer-2025-03-01":                             27,
	"network-arm-natgateway-2025-03-01":                               6,
	"network-arm-networkinterface-2025-03-01":                         15,
	"network-arm-networkmanager-2025-03-01":                           0,
	"network-arm-networkprofile-2025-03-01":                           0,
	"network-arm-networksecuritygroup-2025-03-01":                     12,
	"network-arm-networkwatcher-2025-03-01":                           0,
	"network-arm-privateendpoint-2025-03-01":                          0,
	"network-arm-privatelinkservice-2025-03-01":                       0,
	"network-arm-publicipaddress-2025-03-01":                          9,
	"network-arm-publicipprefix-2025-03-01":                           6,
	"network-arm-routetable-2025-03-01":                               10,
	"network-arm-serviceendpointpolicy-2025-03-01":                    0,
	"network-arm-virtualnetwork-2025-03-01":                           21,
	"network-arm-virtualnetworktap-2025-03-01":                        0,
	"operationalinsights-arm-sharedkeys-2020-08-01":                   1,
	"operationalinsights-arm-workspaces-2020-08-01":                   8,
	"postgresql-arm-openapi-2025-08-01":                               66,
	"privatedns-arm-privatedns-2024-06-01":                            17,
	"redis-arm-redis-2024-11-01":                                      41,
	"resources-arm-resources-2021-04-01":                              36,
	"resources-arm-subscriptions-2022-12-01":                          7,
	"servicebus-arm-authorizationrules-2021-11-01":                    21,
	"servicebus-arm-disasterrecoveryconfigs-2021-11-01":               6,
	"servicebus-arm-migrationconfigs-2021-11-01":                      6,
	"servicebus-arm-namespace-preview-2021-11-01":                     11,
	"servicebus-arm-namespaces-2024-01-01":                            11,
	"servicebus-arm-networksets-2021-11-01":                           3,
	"servicebus-arm-queue-2021-11-01":                                 4,
	"servicebus-arm-subscriptions-2021-11-01":                         4,
	"servicebus-arm-topics-2021-11-01":                                4,
	"servicebus-dataplane-servicebus-2021-05":                         3,
	"storage-arm-blob-2024-01-01":                                     17,
	"storage-arm-file-2024-01-01":                                     12,
	"storage-arm-queue-2024-01-01":                                    8,
	"storage-arm-storage-2024-01-01":                                  44,
	"storage-arm-table-2024-01-01":                                    8,
	"storage-dataplane-blob-2026-04-06":                               60,
	"storage-dataplane-file-2026-04-06":                               31,
	"storage-dataplane-queue-2018-03-28":                              6,
	"subscription-arm-subscriptions-2021-10-01":                       7,
	"web-arm-openapi-2025-03-01":                                      161,
}

// azureSimRoute is one registered simulator route, pre-split into normalized
// segments, ready to match against Swagger operations.
type azureSimRoute struct {
	method string
	segs   []string
}

func azureSimRoutes(t *testing.T) []azureSimRoute {
	t.Helper()
	t.Setenv("SIM_RUNTIME", "process")
	srv, err := buildSimulator(sim.Config{Provider: "azure", ListenAddr: ":0", LogLevel: "error"})
	if err != nil {
		t.Fatalf("buildSimulator: %v", err)
	}
	var routes []azureSimRoute
	for _, p := range srv.RoutePatterns() {
		method, path, ok := strings.Cut(p, " ")
		if !ok {
			continue
		}
		routes = append(routes, azureSimRoute{method: method, segs: splitAzureSegs(path)})
	}
	return routes
}

// azureSpecCovered reports whether a Swagger operation is served by some route.
func azureSpecCovered(routes []azureSimRoute, sp swaggerPath) bool {
	for _, r := range routes {
		if r.method == sp.Method && matchAzureSegs(r.segs, sp.Segs, true) {
			return true
		}
	}
	return false
}

// TestServiceConformance_AzureCoverage reports per-Swagger-file coverage
// (informational — never fails) so the implemented fraction is visible.
func TestServiceConformance_AzureCoverage(t *testing.T) {
	_, byFile := loadSwaggerPaths(t)
	routes := azureSimRoutes(t)
	type row struct {
		name        string
		impl, total int
	}
	var rows []row
	for file, specs := range byFile {
		if len(specs) == 0 {
			continue // pure-definitions swagger (common-types, etc.)
		}
		impl := 0
		for _, sp := range specs {
			if azureSpecCovered(routes, sp) {
				impl++
			}
		}
		rows = append(rows, row{strings.TrimSuffix(file, ".swagger.json.gz"), impl, len(specs)})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].name < rows[j].name })
	ti, tt := 0, 0
	for _, r := range rows {
		ti += r.impl
		tt += r.total
		if r.impl > 0 {
			t.Logf("%-60s %d/%d", r.name, r.impl, r.total)
		}
	}
	t.Logf("TOTAL: %d/%d Azure Swagger operations implemented", ti, tt)
}

// TestServiceConformance_AzureCoverageFloor locks each Swagger document's
// implemented-operation count: an exact-equality ratchet (a drop is a
// regression; more requires bumping the floor).
func TestServiceConformance_AzureCoverageFloor(t *testing.T) {
	_, byFile := loadSwaggerPaths(t)
	routes := azureSimRoutes(t)
	bySuffix := map[string][]swaggerPath{}
	for file, specs := range byFile {
		bySuffix[strings.TrimSuffix(file, ".swagger.json.gz")] = specs
	}
	for name, floor := range azureMethodFloor {
		specs, ok := bySuffix[name]
		if !ok {
			t.Errorf("%s: floor set but no vendored Swagger document found", name)
			continue
		}
		impl := 0
		for _, sp := range specs {
			if azureSpecCovered(routes, sp) {
				impl++
			}
		}
		if impl != floor {
			t.Errorf("%s: coverage %d/%d != floor %d — update azureMethodFloor (a drop is a regression; more is a ratchet-up).",
				name, impl, len(specs), floor)
		}
	}
}
