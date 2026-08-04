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

// loadEC2RequestParameters returns, per Amazon EC2 operation, the request
// parameters the vendored model says the operation takes, spelled as they
// arrive on the wire and lower-cased for comparison.
//
// The ec2Query protocol does not send a member under its member name. The
// aws.protocols#ec2QueryName trait wins where present; otherwise the
// smithy.api#xmlName trait does, with its first letter upper-cased; otherwise
// the member name stands. That is why a list arrives singular — TerminateInstances
// takes InstanceIds and sends InstanceId.1 — and reading the member names
// instead would look for parameters no request ever carries.
func loadEC2RequestParameters(t *testing.T) map[string]map[string]bool {
	t.Helper()
	path := filepath.Join("..", "..", "specs", "cloud-api", "aws", "ec2.smithy.json.gz")
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v — run scripts/fetch-aws-spec.sh ec2", path, err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gunzip %s: %v", path, err)
	}
	defer gz.Close()

	var doc struct {
		Shapes map[string]struct {
			Type    string                  `json:"type"`
			Input   struct{ Target string } `json:"input"`
			Members map[string]struct {
				Traits struct {
					EC2QueryName string `json:"aws.protocols#ec2QueryName"`
					XMLName      string `json:"smithy.api#xmlName"`
				} `json:"traits"`
			} `json:"members"`
		} `json:"shapes"`
	}
	if err := json.NewDecoder(gz).Decode(&doc); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}

	out := map[string]map[string]bool{}
	for id, shape := range doc.Shapes {
		if shape.Type != "operation" || shape.Input.Target == "" {
			continue
		}
		params := map[string]bool{}
		for member, m := range doc.Shapes[shape.Input.Target].Members {
			name := member
			switch {
			case m.Traits.EC2QueryName != "":
				name = m.Traits.EC2QueryName
			case m.Traits.XMLName != "":
				name = strings.ToUpper(m.Traits.XMLName[:1]) + m.Traits.XMLName[1:]
			}
			params[strings.ToLower(name)] = true
		}
		out[id[strings.Index(id, "#")+1:]] = params
	}
	if len(out) == 0 {
		t.Fatalf("%s declares no operations", path)
	}
	return out
}

// iamEC2DerivesItsResource reports whether the derivation produces an ARN for
// an operation — not whether the table knows which type it would build. It runs
// the production path against a request carrying every parameter the model
// declares for the operation: if nothing is derived from all of them, no real
// request derives anything either. Measuring it this way rather than
// re-deciding the rules here is what makes the count reflect the code, so a
// rule that stops firing shows up as coverage falling.
//
// An operation that creates its resource (CreateInternetGateway) carries no
// identifier for it and correctly derives nothing.
func iamEC2DerivesItsResource(operation string, params map[string]bool) bool {
	types := iamActionResourceTypes["ec2:"+operation]
	if len(types) == 0 {
		return false
	}
	values := make(map[string]string, len(params))
	for name := range params {
		if name == "action" || name == "version" {
			continue // the request already carries these
		}
		values[name] = "probe"
	}
	return len(iamEC2ResourceARNs(iamEC2Request(operation, values), types,
		"us-east-1", "123456789012")) > 0
}

// TestIAMEC2ParameterAliasesAreRealRequestParameters holds every alias to a
// parameter the vendored model declares on an operation that authorizes
// against the aliased resource type. The aliases are the one hand-written part
// of Amazon EC2's derivation — the renamings the Service Reference did not
// follow — so an alias that is a guess, a typo, or one the API has since
// dropped derives nothing and would be invisible: the request still succeeds
// for a caller whose policy says Resource "*".
func TestIAMEC2ParameterAliasesAreRealRequestParameters(t *testing.T) {
	byOperation := loadEC2RequestParameters(t)

	// The resource types each variable identifies, and the operations that
	// authorize against them.
	typesByVariable := map[string][]string{}
	for key, format := range iamResourceARNFormats {
		service, resourceType, _ := strings.Cut(key, ":")
		if service != "ec2" {
			continue
		}
		if variable := iamARNFormatVariable(format); variable != "" {
			typesByVariable[variable] = append(typesByVariable[variable], resourceType)
		}
	}

	var problems []string
	for variable, aliases := range iamEC2ParameterAliases {
		resourceTypes := typesByVariable[variable]
		if len(resourceTypes) == 0 {
			problems = append(problems, fmt.Sprintf(
				"%s: no Amazon EC2 resource type is identified by this variable — the alias is dead", variable))
			continue
		}
		for _, alias := range aliases {
			used := false
			for action, declared := range iamActionResourceTypes {
				service, operation, _ := strings.Cut(action, ":")
				if service != "ec2" {
					continue
				}
				if !slicesIntersect(declared, resourceTypes) {
					continue
				}
				if byOperation[operation][strings.ToLower(alias)] {
					used = true
					break
				}
			}
			if !used {
				problems = append(problems, fmt.Sprintf(
					"%s -> %s: no Amazon EC2 operation authorizing against %s takes a %s parameter",
					variable, alias, strings.Join(resourceTypes, "/"), alias))
			}
		}
	}
	sort.Strings(problems)
	if len(problems) > 0 {
		t.Fatalf("iamEC2ParameterAliases does not match the vendored Amazon EC2 model:\n  %s",
			strings.Join(problems, "\n  "))
	}
}

func slicesIntersect(a, b []string) bool {
	for _, x := range a {
		for _, y := range b {
			if x == y {
				return true
			}
		}
	}
	return false
}

// iamHandwrittenDerivationServices are the services whose target resource is
// read by a per-service case in iamResourceARNsForRequest rather than from the
// generated resource-type table. They predate the table and are listed here so
// the coverage report does not read them as having no derivation at all — their
// coverage is per-request (the case fires only when the request carries the
// field it reads), which is precisely why the table-driven form replaced it.
var iamHandwrittenDerivationServices = map[string]bool{
	"sns": true, "sqs": true, "dynamodb": true, "lambda": true,
	"kms": true, "secretsmanager": true, "states": true, "kinesis": true, "ecr": true,
}

// iamDerivationCoverageFloor is the number of served operations that both
// authorize against a resource type and derive it from the resource type AWS
// declares. It only rises: an operation whose resource the gate cannot derive
// is authorized against a literal "*", which matches only a policy whose
// Resource is itself "*", so every resource-scoped grant written for it is
// denied. Raising this number is how that defect class is burned down; the test
// prints what is left.
// Amazon EC2's 55 remaining operations are the ones whose resource the request
// genuinely does not name: an operation that creates its resource has no
// identifier for it yet, the Disassociate/Detach family names an association
// rather than either end of it, CreateTags carries identifiers of mixed types
// with nothing published to map an id back to its type, and CancelImportTask's
// one identifier could belong to either resource type it authorizes against.
const iamDerivationCoverageFloor = 818

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

	// Amazon EC2 is measured against the request rather than the table. Its
	// derivation is generated for all 112 resource types at once, so table
	// membership would count an operation that creates its resource — and so
	// carries no identifier for it — as covered. The other table-driven
	// services read hand-listed field spellings, for which membership is the
	// only statement available.
	ec2Parameters := loadEC2RequestParameters(t)

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
		_, derived := iamActionResourceTypes[o.service+":"+o.name]
		if o.service == "ec2" {
			derived = iamEC2DerivesItsResource(o.name, ec2Parameters[o.name])
		}
		if derived {
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
