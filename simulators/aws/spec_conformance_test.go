package main

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	sim "github.com/sockerless/simulator"
)

// These tests enforce the simulator's core fidelity invariant: every
// operation the AWS simulator serves must be a REAL AWS operation — the
// simulator must not invent X-Amz-Target values, query actions, or REST
// paths under the AWS namespace. The registered operation tables (built
// in-process via buildSimulator) are validated against the official
// Smithy 2.0 service models vendored (gzipped, pinned) under
// specs/cloud-api/aws/. Refresh a model with scripts/fetch-aws-spec.sh.

// smithyOp is one operation in a vendored model: its short name plus the
// smithy.api#http binding when the protocol is REST-based.
type smithyOp struct {
	HTTPMethod string
	HTTPURI    string
}

// smithyService is the parsed surface of one vendored Smithy model.
type smithyService struct {
	File      string
	ShapeName string // short service shape name == X-Amz-Target prefix for awsJson protocols
	Version   string // service shape version == query-protocol Version parameter
	Protocols map[string]bool
	Ops       map[string]smithyOp
}

func loadSmithyModels(t *testing.T) []*smithyService {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("..", "..", "specs", "cloud-api", "aws", "*.smithy.json.gz"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no vendored Smithy models found (glob err: %v) — run scripts/fetch-aws-spec.sh", err)
	}

	var out []*smithyService
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
			Shapes map[string]struct {
				Type    string                     `json:"type"`
				Version string                     `json:"version"`
				Traits  map[string]json.RawMessage `json:"traits"`
			} `json:"shapes"`
		}
		err = json.NewDecoder(gz).Decode(&doc)
		_ = gz.Close()
		_ = f.Close()
		if err != nil {
			t.Fatalf("decode %s: %v", p, err)
		}

		svc := &smithyService{
			File:      filepath.Base(p),
			Protocols: map[string]bool{},
			Ops:       map[string]smithyOp{},
		}
		for id, shape := range doc.Shapes {
			short := id
			if i := strings.Index(id, "#"); i >= 0 {
				short = id[i+1:]
			}
			switch shape.Type {
			case "service":
				svc.ShapeName = short
				svc.Version = shape.Version
				for trait := range shape.Traits {
					if strings.HasPrefix(trait, "aws.protocols#") || strings.HasPrefix(trait, "smithy.protocols#") {
						svc.Protocols[strings.SplitN(trait, "#", 2)[1]] = true
					}
				}
			case "operation":
				op := smithyOp{}
				if raw, ok := shape.Traits["smithy.api#http"]; ok {
					var h struct {
						Method string `json:"method"`
						URI    string `json:"uri"`
					}
					if err := json.Unmarshal(raw, &h); err != nil {
						t.Fatalf("%s: bad smithy.api#http on %s: %v", p, id, err)
					}
					op.HTTPMethod = h.Method
					op.HTTPURI = h.URI
				}
				svc.Ops[short] = op
			}
		}
		if svc.ShapeName == "" {
			t.Fatalf("%s: no service shape found", p)
		}
		out = append(out, svc)
	}
	return out
}

func buildConformanceSimulator(t *testing.T) (*sim.Server, *sim.AWSRouter, *sim.AWSQueryRouter) {
	t.Helper()
	t.Setenv("SIM_RUNTIME", "process")
	srv, jsonRouter, queryRouter, err := buildSimulator(sim.Config{
		Provider: "aws", ListenAddr: ":0", LogLevel: "error",
	})
	if err != nil {
		t.Fatalf("buildSimulator: %v", err)
	}
	return srv, jsonRouter, queryRouter
}

func TestJSONTargetsExistInSmithyModels(t *testing.T) {
	models := loadSmithyModels(t)
	_, jsonRouter, _ := buildConformanceSimulator(t)

	byShapeName := map[string]*smithyService{}
	for _, m := range models {
		byShapeName[m.ShapeName] = m
	}

	var offenders []string
	for _, target := range jsonRouter.Targets() {
		i := strings.LastIndex(target, ".")
		if i < 0 {
			offenders = append(offenders, target+"  (not Service.Operation shaped)")
			continue
		}
		prefix, op := target[:i], target[i+1:]
		// The Go SDK sends the bare service shape name as the target
		// prefix; botocore sends the model's legacy targetPrefix, which
		// for some services is the java-style namespace-qualified name
		// (com.amazonaws.cloudtrail.v20131101.CloudTrail_20131101).
		// Both are real wire prefixes, so a dotted prefix is valid when
		// its last component is the service shape name.
		m, ok := byShapeName[prefix]
		if !ok {
			if j := strings.LastIndex(prefix, "."); j >= 0 {
				m, ok = byShapeName[prefix[j+1:]]
			}
		}
		if !ok {
			offenders = append(offenders, target+"  (no vendored model has service shape "+prefix+")")
			continue
		}
		if _, ok := m.Ops[op]; !ok {
			offenders = append(offenders, target+"  (operation "+op+" not in "+m.File+")")
		}
	}

	reportOffenders(t, "X-Amz-Target", offenders)
}

func TestQueryActionsExistInSmithyModels(t *testing.T) {
	models := loadSmithyModels(t)
	_, _, queryRouter := buildConformanceSimulator(t)

	byVersion := map[string][]*smithyService{}
	var queryModels []*smithyService
	for _, m := range models {
		if m.Protocols["awsQuery"] || m.Protocols["ec2Query"] {
			byVersion[m.Version] = append(byVersion[m.Version], m)
			queryModels = append(queryModels, m)
		}
	}

	var offenders []string
	for version, actions := range queryRouter.VersionedActions() {
		for _, action := range actions {
			if version == "" {
				// Legacy bucket: globally-unique action names (EC2 / IAM /
				// STS / CloudWatch metrics query). The action must exist in
				// at least one query-protocol model.
				found := false
				for _, m := range queryModels {
					if _, ok := m.Ops[action]; ok {
						found = true
						break
					}
				}
				if !found {
					offenders = append(offenders, action+"  (no query-protocol model defines this action)")
				}
				continue
			}
			found := false
			for _, m := range byVersion[version] {
				if _, ok := m.Ops[action]; ok {
					found = true
					break
				}
			}
			if !found {
				offenders = append(offenders, version+" "+action+"  (no query-protocol model with version "+version+" defines this action)")
			}
		}
	}

	reportOffenders(t, "query (Version, Action)", offenders)
}

// normalizeAWSPath (spec_validator_xml.go) collapses path parameters so
// simulator mux patterns compare equal to Smithy URI templates; the
// runtime restXml validator shares it.

// allowedNonSpecRoutes lists registered route patterns that are real,
// documented AWS wire surfaces NOT described by any Smithy service model,
// plus the simulator's own control surface. Each entry MUST be justified —
// this is never a place to hide an invented AWS path. Keyed by the exact
// registered pattern.
var allowedNonSpecRoutes = map[string]string{
	"POST /": "protocol dispatch root: awsJson (X-Amz-Target) + awsQuery (Action/Version) services serve on POST /; per-operation validation happens in the target/action tests",

	// EC2 Instance Metadata Service (IMDSv2) — documented AWS surface
	// (SDK credential/region resolution), not modeled in any Smithy spec.
	"PUT /latest/api/token":                                 "IMDSv2 token endpoint",
	"GET /latest/meta-data/":                                "IMDS root listing",
	"GET /latest/meta-data/ami-id":                          "IMDS",
	"GET /latest/meta-data/instance-id":                     "IMDS",
	"GET /latest/meta-data/instance-type":                   "IMDS",
	"GET /latest/meta-data/placement/availability-zone":     "IMDS",
	"GET /latest/meta-data/placement/region":                "IMDS",
	"GET /latest/meta-data/iam/security-credentials/":       "IMDS credential listing",
	"GET /latest/meta-data/iam/security-credentials/{role}": "IMDS role credentials",
	"GET /latest/dynamic/instance-identity/document":        "IMDS identity document",

	// ECS task metadata endpoint v4 — documented AWS surface
	// (ECS_CONTAINER_METADATA_URI_V4), not in the ECS Smithy model.
	"GET /v4/{id}":      "ECS task metadata v4 (container)",
	"GET /v4/{id}/task": "ECS task metadata v4 (task)",

	// Docker Registry HTTP API V2 — ECR's real data plane (docker
	// push/pull). The wire contract is the OCI distribution spec, not
	// the ECR control-plane Smithy model.
	"GET /v2/":    "OCI registry data plane",
	"POST /v2/":   "OCI registry data plane",
	"PUT /v2/":    "OCI registry data plane",
	"PATCH /v2/":  "OCI registry data plane",
	"DELETE /v2/": "OCI registry data plane",
}

// allowedNonSpecPrefixes covers route families outside the AWS API
// namespace entirely.
var allowedNonSpecPrefixes = map[string]string{
	"/sim/v1/":     "simulator control + dashboard surface (sockerless-specific, documented in simulators/README.md)",
	"/sockerless/": "simulator-internal host-dispatch surface",
	"/ecs-exec/":   "ECS ExecuteCommand session WebSocket bridge — the real wire surface is SSM session-manager streaming, which has no Smithy HTTP model",
}

func TestRESTRoutesExistInSmithyModels(t *testing.T) {
	models := loadSmithyModels(t)
	srv, _, _ := buildConformanceSimulator(t)

	// Spec operation set: "METHOD <normalized-uri>" across every model
	// that binds operations to HTTP (restJson1 / restXml — http traits
	// are absent on pure awsJson/awsQuery models).
	specOps := map[string]bool{}
	for _, m := range models {
		for _, op := range m.Ops {
			if op.HTTPMethod == "" {
				continue
			}
			specOps[op.HTTPMethod+" "+normalizeAWSPath(op.HTTPURI)] = true
		}
	}

	// Smithy RPCv2 CBOR routes are "POST /service/<ServiceShape>/operation/<Op>".
	rpcv2 := regexp.MustCompile(`^POST /service/([^/]+)/operation/([^/]+)$`)
	byShapeName := map[string]*smithyService{}
	for _, m := range models {
		byShapeName[m.ShapeName] = m
	}

	var offenders []string
	for _, pattern := range srv.RoutePatterns() {
		if pattern == "GET /health" {
			continue // shared simulator health endpoint
		}
		if _, ok := allowedNonSpecRoutes[pattern]; ok {
			continue
		}
		method, path, found := strings.Cut(pattern, " ")
		if !found {
			offenders = append(offenders, pattern+"  (method-less pattern)")
			continue
		}
		prefixAllowed := false
		for prefix := range allowedNonSpecPrefixes {
			if strings.HasPrefix(path, prefix) {
				prefixAllowed = true
				break
			}
		}
		if prefixAllowed {
			continue
		}

		if m := rpcv2.FindStringSubmatch(pattern); m != nil {
			svc, ok := byShapeName[m[1]]
			if !ok {
				offenders = append(offenders, pattern+"  (rpcv2: no vendored model has service shape "+m[1]+")")
			} else if _, ok := svc.Ops[m[2]]; !ok {
				offenders = append(offenders, pattern+"  (rpcv2: operation "+m[2]+" not in "+svc.File+")")
			}
			continue
		}

		norm := method + " " + normalizeAWSPath(path)
		if specOps[norm] {
			continue
		}
		// HEAD is served by Go's mux through GET registrations; Smithy
		// models HEAD ops (HeadObject, HeadBucket) separately.
		if method == "GET" && specOps["HEAD "+normalizeAWSPath(path)] {
			continue
		}
		// A simulator greedy label ({arn...}) where the spec has a plain
		// label ({Resource}) accepts a superset of spec paths — valid
		// (mux needs greedy when the value can contain separators).
		// The reverse (sim plain where spec is greedy) stays an offense.
		degreedy := strings.ReplaceAll(norm, "{+}", "{}")
		if specOps[degreedy] {
			continue
		}
		if method == "GET" && specOps[strings.Replace(degreedy, "GET ", "HEAD ", 1)] {
			continue
		}
		offenders = append(offenders, pattern+"  (normalized: "+norm+")")
	}

	reportOffenders(t, "REST route", offenders)
}

func reportOffenders(t *testing.T, kind string, offenders []string) {
	t.Helper()
	if len(offenders) == 0 {
		return
	}
	sort.Strings(offenders)
	t.Errorf("%d registered %s(s) do not exist in the vendored AWS Smithy models (invented operation, wrong wire key/path shape, or a real-but-unmodeled surface that needs a justified allowlist entry):\n  %s",
		len(offenders), kind, strings.Join(offenders, "\n  "))
}

// TestVendoredModelsAreConsumed flags vendored spec files no registered
// operation references — a stale vendor is either a removed service or a
// fetch for the wrong model.
func TestVendoredModelsAreConsumed(t *testing.T) {
	models := loadSmithyModels(t)
	srv, jsonRouter, queryRouter := buildConformanceSimulator(t)

	used := map[string]bool{}
	byShapeName := map[string]*smithyService{}
	byVersion := map[string][]*smithyService{}
	for _, m := range models {
		byShapeName[m.ShapeName] = m
		byVersion[m.Version] = append(byVersion[m.Version], m)
	}
	for _, target := range jsonRouter.Targets() {
		if prefix, op, ok := strings.Cut(target, "."); ok {
			if m, found := byShapeName[prefix]; found {
				if _, has := m.Ops[op]; has {
					used[m.File] = true
				}
			}
		}
	}
	for version, actions := range queryRouter.VersionedActions() {
		for _, action := range actions {
			for _, m := range models {
				if version != "" && m.Version != version {
					continue
				}
				if !m.Protocols["awsQuery"] && !m.Protocols["ec2Query"] {
					continue
				}
				if _, has := m.Ops[action]; has {
					used[m.File] = true
				}
			}
		}
	}
	for _, pattern := range srv.RoutePatterns() {
		method, path, ok := strings.Cut(pattern, " ")
		if !ok {
			continue
		}
		norm := normalizeAWSPath(path)
		for _, m := range models {
			for _, op := range m.Ops {
				if op.HTTPMethod == "" {
					continue
				}
				if normalizeAWSPath(op.HTTPURI) == norm && (op.HTTPMethod == method || (method == "GET" && op.HTTPMethod == "HEAD")) {
					used[m.File] = true
				}
			}
		}
		if m := regexp.MustCompile(`^POST /service/([^/]+)/operation/`).FindStringSubmatch(pattern); m != nil {
			if svc, ok := byShapeName[m[1]]; ok {
				used[svc.File] = true
			}
		}
	}

	var stale []string
	for _, m := range models {
		if !used[m.File] {
			stale = append(stale, fmt.Sprintf("%s (service %s)", m.File, m.ShapeName))
		}
	}
	if len(stale) > 0 {
		sort.Strings(stale)
		t.Errorf("%d vendored Smithy model(s) are not referenced by any registered operation (stale vendor or wrong model):\n  %s",
			len(stale), strings.Join(stale, "\n  "))
	}
}
