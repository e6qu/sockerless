package main

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The call-time IAM gate authorizes a request against the resource it names.
// Getting that resource wrong is invisible in every other test the simulator
// has: the request still succeeds for a caller whose policy says Resource "*",
// and only a caller with a resource-scoped grant — the caller least-privilege
// exists for — is denied. These tests bind the gate to the data AWS publishes
// about which resource each action authorizes against, so a service the gate
// silently fails to derive is a counted, failing number rather than a defect a
// consumer discovers in production.

// awsSignedJSONRequest builds the signed awsJson request an AWS SDK sends: the
// operation in X-Amz-Target and the region the gate derives ARNs from in the
// SigV4 credential scope.
func awsSignedJSONRequest(target, body string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/x-amz-json-1.1")
	r.Header.Set("X-Amz-Target", target)
	r.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential=ASIAEXAMPLECREDENTIAL/20260801/us-east-1/aws/aws4_request, SignedHeaders=host;x-amz-target, Signature=00")
	return r
}

// awsServiceReference is the parsed surface of one vendored AWS Service
// Reference document.
type awsServiceReference struct {
	Name    string
	Actions []struct {
		Name      string
		Resources []struct{ Name string }
	}
}

func loadServiceReferences(t *testing.T) map[string]*awsServiceReference {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("..", "..", "specs", "cloud-api", "aws",
		"service-reference", "*.servicereference.json.gz"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no vendored Service Reference documents (glob err: %v) — run scripts/fetch-aws-service-reference.sh", err)
	}
	out := map[string]*awsServiceReference{}
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			t.Fatalf("open %s: %v", p, err)
		}
		gz, err := gzip.NewReader(f)
		if err != nil {
			t.Fatalf("gunzip %s: %v", p, err)
		}
		var doc awsServiceReference
		if err := json.NewDecoder(gz).Decode(&doc); err != nil {
			t.Fatalf("decode %s: %v", p, err)
		}
		_ = gz.Close()
		_ = f.Close()
		if doc.Name == "" || len(doc.Actions) == 0 {
			t.Fatalf("%s: not a Service Reference document", p)
		}
		out[doc.Name] = &doc
	}
	return out
}

// resourceTypes returns the resource types the reference declares for an
// action, and whether the action exists at all.
func (s *awsServiceReference) resourceTypes(action string) ([]string, bool) {
	for _, a := range s.Actions {
		if a.Name != action {
			continue
		}
		types := make([]string, 0, len(a.Resources))
		for _, r := range a.Resources {
			types = append(types, r.Name)
		}
		sort.Strings(types)
		return types, true
	}
	return nil, false
}

// TestIAMResourceTypesTableMatchesTheVendoredReference proves the generated
// table is the vendored reference and nothing else. The table is the gate's
// only statement of which resource type an action authorizes against, so a
// hand-edit or a stale regeneration would silently change authorization
// decisions; this rebuilds it from the vendored documents and compares.
func TestIAMResourceTypesTableMatchesTheVendoredReference(t *testing.T) {
	refs := loadServiceReferences(t)

	// The services the generator emits, read back from the table itself so the
	// test does not carry its own copy of the list.
	services := map[string]bool{}
	for action := range iamActionResourceTypes {
		service, _, ok := strings.Cut(action, ":")
		if !ok {
			t.Fatalf("table key %q is not service:Action shaped", action)
		}
		services[service] = true
	}
	if len(services) == 0 {
		t.Fatal("iamActionResourceTypes is empty — run scripts/gen-aws-iam-resource-types.sh")
	}

	want := map[string][]string{}
	for service := range services {
		ref, ok := refs[service]
		if !ok {
			t.Fatalf("the table covers %q but no Service Reference is vendored for it", service)
		}
		for _, a := range ref.Actions {
			if len(a.Resources) == 0 {
				continue
			}
			types, _ := ref.resourceTypes(a.Name)
			want[service+":"+a.Name] = types
		}
	}

	var problems []string
	for action, wantTypes := range want {
		gotTypes, ok := iamActionResourceTypes[action]
		if !ok {
			problems = append(problems, action+": declared by the reference, absent from the table")
			continue
		}
		got := append([]string(nil), gotTypes...)
		sort.Strings(got)
		if strings.Join(got, ",") != strings.Join(wantTypes, ",") {
			problems = append(problems, fmt.Sprintf("%s: table has [%s], reference declares [%s]",
				action, strings.Join(got, ", "), strings.Join(wantTypes, ", ")))
		}
	}
	for action := range iamActionResourceTypes {
		if _, ok := want[action]; !ok {
			problems = append(problems, action+": in the table, not declared by the reference")
		}
	}
	sort.Strings(problems)
	if len(problems) > 0 {
		t.Fatalf("iam_resource_types_gen.go is out of date with the vendored Service Reference "+
			"(run scripts/gen-aws-iam-resource-types.sh):\n  %s", strings.Join(problems, "\n  "))
	}
}

// iamServiceForServedOperation resolves the IAM service prefix a served
// operation belongs to. JSON targets go through the production classifier so
// the test measures what the gate actually sees; query actions registered
// without a Version are shared by EC2, IAM and STS, and are resolved by which
// of those services the reference says defines the action.
func iamServiceForServedOperation(t *testing.T, refs map[string]*awsServiceReference,
	target, version, action string) (service, op string, ok bool) {
	t.Helper()
	if target != "" {
		r := awsSignedJSONRequest(target, "{}")
		full, classified := iamActionForRequest(r)
		if !classified {
			return "", "", false
		}
		service, op, _ = strings.Cut(full, ":")
		return service, op, true
	}
	byVersion := map[string]string{
		"2016-11-15": "ec2", "2011-01-01": "autoscaling", "2010-08-01": "cloudwatch",
		"2010-03-31": "sns", "2015-12-01": "elasticloadbalancing", "2014-10-31": "rds",
		"2015-02-02": "elasticache",
	}
	if svc, found := byVersion[version]; found {
		return svc, action, true
	}
	if version != "" {
		return "", "", false
	}
	// The unversioned bucket: EC2, IAM and STS register there because their
	// action names do not collide across AWS.
	for _, candidate := range []string{"ec2", "iam", "sts"} {
		if ref, found := refs[candidate]; found {
			if _, defined := ref.resourceTypes(action); defined {
				return candidate, action, true
			}
		}
	}
	return "", "", false
}

// iamHandwrittenDerivationServices are the services whose target resource is
// read by a per-service case in iamResourceARNsForRequest rather than from the
// generated resource-type table. They predate the table and are listed here so
// the coverage report does not read them as having no derivation at all — their
// coverage is per-request (the case fires only when the request carries the
// field it reads), which is precisely why the table-driven form replaced it.
var iamHandwrittenDerivationServices = map[string]bool{
	"sns": true, "sqs": true, "ec2": true, "dynamodb": true, "lambda": true,
	"kms": true, "secretsmanager": true, "states": true, "kinesis": true, "ecr": true,
}

// iamDerivationCoverageFloor is the number of served operations that both
// authorize against a resource type and derive it from the resource type AWS
// declares. It only rises: an operation whose resource the gate cannot derive
// is authorized against a literal "*", which matches only a policy whose
// Resource is itself "*", so every resource-scoped grant written for it is
// denied. Raising this number is how that defect class is burned down; the test
// prints what is left.
const iamDerivationCoverageFloor = 358

// TestIAMResourceDerivationCoverage measures how much of the simulator's served
// surface authorizes against a real resource rather than the "*" fallback, and
// ratchets it. The report names the services still falling back so the next
// increment is a decision about which service to derive, not a rediscovery of
// the gap.
func TestIAMResourceDerivationCoverage(t *testing.T) {
	refs := loadServiceReferences(t)
	_, jsonRouter, queryRouter := buildConformanceSimulator(t)

	type op struct{ service, name string }
	served := map[op]bool{}
	for _, target := range jsonRouter.Targets() {
		if service, name, ok := iamServiceForServedOperation(t, refs, target, "", ""); ok {
			served[op{service, name}] = true
		}
	}
	for version, actions := range queryRouter.VersionedActions() {
		for _, action := range actions {
			if service, name, ok := iamServiceForServedOperation(t, refs, "", version, action); ok {
				served[op{service, name}] = true
			}
		}
	}

	covered := 0
	missingByService := map[string][]string{}
	for o := range served {
		ref, ok := refs[o.service]
		if !ok {
			continue
		}
		types, defined := ref.resourceTypes(o.name)
		if !defined || len(types) == 0 {
			continue // AWS declares no resource type: "*" is the correct request
		}
		if _, derived := iamActionResourceTypes[o.service+":"+o.name]; derived {
			covered++
			continue
		}
		missingByService[o.service] = append(missingByService[o.service], o.name)
	}

	total := covered
	for _, ops := range missingByService {
		total += len(ops)
	}
	services := make([]string, 0, len(missingByService))
	for service := range missingByService {
		services = append(services, service)
	}
	sort.Slice(services, func(i, j int) bool {
		if len(missingByService[services[i]]) != len(missingByService[services[j]]) {
			return len(missingByService[services[i]]) > len(missingByService[services[j]])
		}
		return services[i] < services[j]
	})
	var report strings.Builder
	fmt.Fprintf(&report, "resource-scoped authorization: %d of %d served operations derive their resource\n",
		covered, total)
	for _, service := range services {
		note := ""
		if iamHandwrittenDerivationServices[service] {
			note = "  (per-request case in iamResourceARNsForRequest)"
		}
		fmt.Fprintf(&report, "  %-24s %3d operations not derived from the declared type%s\n",
			service, len(missingByService[service]), note)
	}
	t.Log(report.String())

	if covered < iamDerivationCoverageFloor {
		t.Fatalf("resource-derivation coverage fell from %d to %d served operations — "+
			"a service lost its derivation, which denies every resource-scoped grant written for it",
			iamDerivationCoverageFloor, covered)
	}
	if covered > iamDerivationCoverageFloor {
		t.Fatalf("resource-derivation coverage rose from %d to %d served operations — "+
			"raise iamDerivationCoverageFloor to %d to hold the gain",
			iamDerivationCoverageFloor, covered, covered)
	}
}
