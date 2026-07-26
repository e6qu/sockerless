package main

import (
	"sort"
	"strings"
	"testing"

	sim "github.com/sockerless/simulator"
)

// GCP operation-coverage gate — the Discovery-document analogue of the AWS
// service-conformance ratchet (simulators/aws/service_conformance_test.go).
//
// For each vendored Google Discovery document, it counts how many of the
// service's REST methods the simulator implements: a method is "covered" when
// some registered route matches its HTTP method + (normalized) path under the
// same matchSegs rules the route-validity gate already uses. The count is
// locked by gcpMethodFloor — a drop is a regression, an increase is a ratchet-up
// that must bump the floor. This makes GCP coverage a measured, gated number
// rather than something discovered later by a consumer.
//
// The bulk of Compute Engine's ~2000-method surface (and other large
// data-plane/admin surfaces) is intentionally far from 100% — the floor records
// the honest implemented count, not an aspiration.

// gcpMethodFloor locks the implemented-method COUNT per Discovery document
// (keyed by file name without the .discovery.json.gz suffix). Implement a
// method (or grow the vendored doc) and the matching floor must move with it.
var gcpMethodFloor = map[string]int{
	"compute-v1":              1110,
	"logging-v2":              480,
	"cloudresourcemanager-v3": 105,
	"sqladmin-v1beta4":        146,
	"bigtableadmin-v2":        136,
	"cloudrun-v1":             99,
	"dataflow-v1b3":           84,
	"cloudrun-v2":             90,
	"iam-v1":                  264,
	"bigquery-v2":             86,
	"dns-v1":                  74,
	"storage-v1":              84,
	"artifactregistry-v1":     144,
	"cloudkms-v1":             157,
	"eventarc-v1":             124,
	"cloudfunctions-v2":       31,
	"cloudbuild-v1":           130,
	"redis-v1":                90,
	"firestore-v1":            112,
	"pubsub-v1":               86,
	"secretmanager-v1":        60,
	"sqladmin-v1":             148,
	"spanner-v1":              198,
	"apigateway-v1":           57,
	"iamcredentials-v1":       14,
	"vpcaccess-v1":            16,
	"serviceusage-v1":         20,
}

// gcpSimRoute is one registered simulator route, pre-split into normalized
// segments with the mount prefix (e.g. /spanner) stripped so it aligns with the
// Discovery method's project-relative path.
type gcpSimRoute struct {
	method string
	segs   []string
}

// gcpSimRoutes builds the simulator and returns its registered routes,
// normalized + mount-stripped, ready to match against Discovery methods.
func gcpSimRoutes(t *testing.T) []gcpSimRoute {
	t.Helper()
	t.Setenv("SIM_RUNTIME", "process")
	srv, err := buildSimulator(sim.Config{Provider: "gcp", ListenAddr: ":0", LogLevel: "error"})
	if err != nil {
		t.Fatalf("buildSimulator: %v", err)
	}
	var routes []gcpSimRoute
	for _, p := range srv.RoutePatterns() {
		method, path, ok := strings.Cut(p, " ")
		if !ok {
			continue
		}
		for prefix := range gcpMountPrefixes {
			if strings.HasPrefix(path, prefix+"/") {
				path = strings.TrimPrefix(path, prefix)
			}
		}
		routes = append(routes, gcpSimRoute{method: method, segs: splitNormSegs(path)})
	}
	return routes
}

// gcpMethodCovered reports whether a Discovery method is served by some route.
func gcpMethodCovered(routes []gcpSimRoute, s specPath) bool {
	for _, r := range routes {
		if r.method == s.Method && matchSegs(r.segs, s.Segs) {
			return true
		}
	}
	return false
}

// gcpDocCoverage returns the implemented count for one Discovery document.
func gcpDocCoverage(routes []gcpSimRoute, d *discoveryDoc) int {
	impl := 0
	for _, m := range d.Methods {
		if gcpMethodCovered(routes, specPath{Method: m.HTTPMethod, Segs: splitNormSegs(m.Path)}) {
			impl++
		}
	}
	return impl
}

// TestServiceConformance_GCPCoverageFloor locks each Discovery document's
// implemented-method count: an exact-equality ratchet (a drop is a regression;
// more requires bumping the floor).
func TestServiceConformance_GCPCoverageFloor(t *testing.T) {
	docs := loadDiscoveryDocs(t)
	routes := gcpSimRoutes(t)
	byFile := map[string]*discoveryDoc{}
	for _, d := range docs {
		byFile[strings.TrimSuffix(d.File, ".discovery.json.gz")] = d
	}
	for name, floor := range gcpMethodFloor {
		d, ok := byFile[name]
		if !ok {
			t.Errorf("%s: floor set but no vendored Discovery document found", name)
			continue
		}
		impl := gcpDocCoverage(routes, d)
		if impl != floor {
			t.Errorf("%s: coverage %d/%d != floor %d — update gcpMethodFloor (a drop is a regression; more is a ratchet-up).",
				name, impl, len(d.Methods), floor)
		}
	}
}

// TestServiceConformance_GCPCoverage reports per-document coverage (informational
// — never fails) so the implemented fraction is visible at a glance.
func TestServiceConformance_GCPCoverage(t *testing.T) {
	docs := loadDiscoveryDocs(t)
	routes := gcpSimRoutes(t)
	type row struct {
		name        string
		impl, total int
	}
	var rows []row
	for _, d := range docs {
		rows = append(rows, row{strings.TrimSuffix(d.File, ".discovery.json.gz"), gcpDocCoverage(routes, d), len(d.Methods)})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].name < rows[j].name })
	ti, tt := 0, 0
	for _, r := range rows {
		ti += r.impl
		tt += r.total
		t.Logf("%-32s %d/%d", r.name, r.impl, r.total)
	}
	t.Logf("TOTAL: %d/%d GCP Discovery methods implemented", ti, tt)
}
