package main

import (
	"encoding/json"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Deriving the resource ARN a request names, for the services whose requests
// carry it.
//
// The work splits in two, and only one half is ours. Which resource type an
// action authorizes against is AWS's answer: iamActionResourceTypes is
// generated from the vendored AWS Service Reference
// (specs/cloud-api/aws/service-reference/), the same data the Service
// Authorization Reference publishes. This file supplies the other half —
// pulling the resource's identifier out of the request the SDK actually sent,
// which no specification can state — and assembles the ARN in the shape the
// reference declares for that type.
//
// Getting it wrong in either direction denies a call the policy allows: an
// action the gate leaves undivined is authorized against a literal "*", which
// matches only a policy whose Resource is itself "*".

// iamDerivedResourceARNs returns the ARNs a request names, or nil when the
// action declares no resource type (AWS's way of saying it does not support
// resource-level permissions, so the request targets "*") or when this file
// cannot read the resource out of the request.
//
// arn builds "arn:aws:<svc>:<region>:<account>:<resource>" for the request's
// region and the simulator's account.
func iamDerivedResourceARNs(r *http.Request, service, op, region, account string) []string {
	types := iamActionResourceTypes[service+":"+op]
	if len(types) == 0 {
		return nil
	}
	arn := func(svc, resource string) string {
		return "arn:aws:" + svc + ":" + region + ":" + account + ":" + resource
	}
	// Every service here spells its tagging operations the same way: the
	// resource is named by its own ARN, which needs no assembly. This is also
	// what resolves the tagging actions' long resource-type lists — TagResource
	// accepts nine ECS types, and the ARN says which one this call means.
	if a := iamRequestARNField(r); a != "" {
		return []string{a}
	}
	switch service {
	case "ec2":
		return iamEC2ResourceARNs(r, types, region, account)
	case "ecs":
		return iamECSResourceARNs(r, op, types, arn)
	case "logs":
		return iamLogsResourceARNs(r, types, arn)
	case "codebuild":
		return iamCodeBuildResourceARNs(r, types, arn)
	case "wafv2":
		return iamWAFv2ResourceARNs(r, op, types)
	case "iam":
		return iamIAMResourceARNs(r, types)
	}
	return nil
}

// ===== AWS Identity and Access Management =====

// iamIAMResourceARNs derives the ARNs an IAM request names. IAM is a global
// service and its ARNs carry no region — "arn:aws:iam::<account>:role/<name>" —
// so they are assembled here rather than through the regional builder the
// other services use.
//
// IAM speaks the query protocol, so the identifiers are form parameters. A
// name may carry a path ("/team/", giving "role/team/name"); the API takes the
// path separately on create and folds it into the ARN, which is what the
// resource types call a "NameWithPath".
func iamIAMResourceARNs(r *http.Request, types []string) []string {
	account := awsAccountID()
	build := func(resourceType, path, name string) string {
		path = strings.Trim(path, "/")
		if path != "" {
			path += "/"
		}
		return "arn:aws:iam::" + account + ":" + resourceType + "/" + path + name
	}
	// The operations that act on a policy, an OIDC provider or a SAML provider
	// name it by ARN outright.
	for _, field := range []string{"PolicyArn", "OpenIDConnectProviderArn", "SAMLProviderArn", "PolicySourceArn"} {
		if v := r.FormValue(field); strings.HasPrefix(v, "arn:") {
			return []string{v}
		}
	}
	path := r.FormValue("Path")
	for _, candidate := range []struct{ resourceType, field string }{
		{"role", "RoleName"},
		{"user", "UserName"},
		{"group", "GroupName"},
		{"instance-profile", "InstanceProfileName"},
		{"server-certificate", "ServerCertificateName"},
		{"policy", "PolicyName"},
		{"mfa", "VirtualMFADeviceName"},
	} {
		if !iamHasType(types, candidate.resourceType) {
			continue
		}
		if name := r.FormValue(candidate.field); name != "" {
			return []string{build(candidate.resourceType, path, name)}
		}
	}
	// An MFA device is named by its serial number, which for a virtual device
	// already is the ARN.
	if iamHasType(types, "mfa") {
		if serial := r.FormValue("SerialNumber"); strings.HasPrefix(serial, "arn:") {
			return []string{serial}
		}
	}
	return nil
}

// iamRequestARNField returns the ARN a request names directly. AWS is not
// consistent about the casing across services (ECS and CloudWatch Logs send
// resourceArn, WAFv2 sends ResourceARN, and WAFv2's association operations
// send the governed resource as ResourceArn), so all the real spellings are
// read.
func iamRequestARNField(r *http.Request) string {
	for _, field := range []string{"resourceArn", "ResourceARN", "ResourceArn", "resourceARN"} {
		if v := iamJSONBodyField(r, field); strings.HasPrefix(v, "arn:") {
			return v
		}
	}
	return ""
}

// iamJSONBodyStrings reads a top-level list-of-strings field from an awsJson
// request body, restoring the body for the handler.
func iamJSONBodyStrings(r *http.Request, field string) []string {
	body := iamRequestBody(r)
	if len(body) == 0 {
		return nil
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(body, &m) != nil {
		return nil
	}
	raw, ok := m[field]
	if !ok {
		return nil
	}
	var out []string
	if json.Unmarshal(raw, &out) != nil {
		return nil
	}
	return out
}

// iamFirstJSONField returns the first of several alternative field names the
// request carries a value for. Operations within one service name the same
// resource differently (ECS CreateService sends serviceName where
// DescribeServices sends services), so the caller lists the spellings rather
// than the gate carrying a per-operation table.
func iamFirstJSONField(r *http.Request, fields ...string) string {
	for _, f := range fields {
		if v := iamJSONBodyField(r, f); v != "" {
			return v
		}
	}
	return ""
}

// iamNamesFrom collects the identifiers a request carries under any of the
// given singular or plural field names, preserving order and dropping
// duplicates so a batch operation authorizes each distinct resource once.
func iamNamesFrom(r *http.Request, singular []string, plural []string) []string {
	var names []string
	seen := map[string]struct{}{}
	add := func(v string) {
		if v == "" {
			return
		}
		if _, dup := seen[v]; dup {
			return
		}
		seen[v] = struct{}{}
		names = append(names, v)
	}
	for _, f := range singular {
		add(iamJSONBodyField(r, f))
	}
	for _, f := range plural {
		for _, v := range iamJSONBodyStrings(r, f) {
			add(v)
		}
	}
	return names
}

// iamHasType reports whether AWS declares resourceType for the action.
func iamHasType(types []string, resourceType string) bool {
	for _, t := range types {
		if t == resourceType {
			return true
		}
	}
	return false
}

// ===== Amazon Elastic Compute Cloud =====

// iamEC2ResourceARNs derives the ARNs an Amazon EC2 request names. EC2 declares
// 112 resource types across 515 actions — too many to transcribe, and a
// transcription would rot — so the derivation is driven by the reference
// itself. Each type's published ARN format ends in the variable naming its
// identifier ("...:volume/${VolumeId}"), and EC2's query protocol carries that
// identifier in a request parameter of the same name.
//
// Filling the published format, rather than assembling a resource path, is what
// keeps the irregular shapes right: an Amazon Machine Image and a snapshot
// carry no account, the Amazon VPC IP Address Manager types carry no region,
// and five of EC2's types name a resource belonging to another service outright
// — a certificate is an AWS Certificate Manager ARN and a role an AWS Identity
// and Access Management one.
//
// Where a parameter is spelled differently from the variable the difference is
// one of two kinds. EC2 drops the resource's own leading word from some of them
// — a security group's ${SecurityGroupId} arrives as GroupId, a dedicated
// host's ${DedicatedHostId} as HostId — which is mechanical. The rest are
// genuine renamings, listed in iamEC2ParameterAliases.
func iamEC2ResourceARNs(r *http.Request, types []string, region, account string) []string {
	params := iamEC2RequestParameters(r)

	// Resolve each declared type to the parameter naming it before building
	// anything, because two types can resolve to the same one and the
	// identifier belongs to only one of them.
	//
	// Which one is sometimes answerable. RunInstances authorizes against a
	// subnet and a secondary subnet, and a SubnetId is the subnet's published
	// variable outright where it reaches the secondary only through the
	// prefix-drop rule — the exact spelling is the stronger claim and takes it.
	// Where the claims are equally strong the request genuinely does not say:
	// AssociateRouteTable authorizes against an internet gateway and a virtual
	// private gateway and takes a single GatewayId, and CancelImportTask against
	// an image-import and a snapshot-import task, both under ImportTaskId.
	// Building both would invent an ARN for a resource that does not exist and
	// then require it to be allowed, denying a policy that named the real one,
	// so neither is derived — and the request's other parameters still are.
	type resolved struct {
		format, parameter string
		rank              int
	}
	best := map[string]int{}
	found := make([]resolved, 0, len(types))
	for _, resourceType := range types {
		format, declared := iamResourceARNFormats["ec2:"+resourceType]
		if !declared {
			continue
		}
		variable := iamARNFormatVariable(format)
		if variable == "" {
			continue
		}
		parameter, rank, named := iamEC2Parameter(params, variable)
		if !named {
			continue
		}
		if seen, ok := best[parameter]; !ok || rank < seen {
			best[parameter] = rank
		}
		found = append(found, resolved{format, parameter, rank})
	}
	contested := map[string]int{}
	for _, f := range found {
		if f.rank == best[f.parameter] {
			contested[f.parameter]++
		}
	}

	var out []string
	seen := map[string]struct{}{}
	for _, f := range found {
		if f.rank != best[f.parameter] || contested[f.parameter] > 1 {
			continue
		}
		for _, id := range params[f.parameter] {
			// A few types are named by another service's ARN outright
			// (CertificateArn, RoleArn), which needs no assembly.
			resource := id
			if !strings.HasPrefix(resource, "arn:") {
				resource = iamFillARNFormat(f.format, region, account, id)
			}
			if _, dup := seen[resource]; dup {
				continue
			}
			seen[resource] = struct{}{}
			out = append(out, resource)
		}
	}
	return out
}

// iamEC2ParameterAliases maps an ARN format's identifier variable to the
// request parameters EC2 actually spells it as, where the two differ by more
// than the mechanical prefix drop. Every entry is a rename the API made and
// the reference did not follow: an endpoint service is addressed as ServiceId,
// a network ACL's ${NaclId} arrives as NetworkAclId, a key pair is named by
// KeyName, and the copy operations name their *source* resource.
//
// TestIAMEC2ParameterAliasesAreRealRequestParameters holds every entry to a
// parameter the vendored Amazon EC2 model declares on an operation that
// authorizes against that resource type, so a guess or a stale rename fails
// rather than silently deriving nothing.
var iamEC2ParameterAliases = map[string][]string{
	"CapacityReservationId":           {"SourceCapacityReservationId"},
	"CertificateId":                   {"CertificateArn"},
	"DeclarativePoliciesReportId":     {"ReportId"},
	"FpgaImageId":                     {"SourceFpgaImageId"},
	"ImageUsageReportId":              {"ReportId"},
	"ImportImageTaskId":               {"ImportTaskId"},
	"ImportSnapshotTaskId":            {"ImportTaskId"},
	"IpamScopeId":                     {"DestinationIpamScopeId"},
	"Ipv4PoolCoipId":                  {"CoipPoolId", "PoolId"},
	"Ipv4PoolEc2Id":                   {"PoolId"},
	"Ipv6PoolEc2Id":                   {"PoolId"},
	"KeyPairName":                     {"KeyName"},
	"NaclId":                          {"NetworkAclId"},
	"PrefixListId":                    {"DestinationPrefixListId"},
	"ReservationId":                   {"ReservedInstancesId", "ReservedInstanceId"},
	"RoleNameWithPath":                {"RoleArn"},
	"SnapshotId":                      {"SourceSnapshotId"},
	"VolumeId":                        {"SourceVolumeId"},
	"VpcBlockPublicAccessExclusionId": {"ExclusionId"},
	"VpcEndpointServiceId":            {"ServiceId"},
}

// iamEC2Parameter returns the request parameter that names an ARN format's
// variable, trying each spelling it goes by and taking the first the request
// supplies. The rank is that spelling's position in the list, which runs most
// specific first, so a lower rank is the stronger claim on a parameter two
// resource types both resolve to.
func iamEC2Parameter(params map[string][]string, variable string) (string, int, bool) {
	for rank, name := range iamEC2ParameterNames(variable) {
		key := strings.ToLower(name)
		if len(params[key]) > 0 {
			return key, rank, true
		}
	}
	return "", 0, false
}

// iamEC2ParameterNames returns the request-parameter spellings an ARN format
// variable can arrive under, most specific first.
func iamEC2ParameterNames(variable string) []string {
	names := []string{variable}
	if unprefixed := iamEC2UnprefixedParameter(variable); unprefixed != "" {
		names = append(names, unprefixed)
	}
	return append(names, iamEC2ParameterAliases[variable]...)
}

// iamEC2PrefixedVariable matches a variable whose leading word names the
// resource itself, capturing the rest — the form EC2 abbreviates to
// (SecurityGroupId → GroupId, PlacementGroupName → GroupName).
var iamEC2PrefixedVariable = regexp.MustCompile(`^[A-Z][a-z]+([A-Z].*(?:Id|Name))$`)

func iamEC2UnprefixedParameter(variable string) string {
	if m := iamEC2PrefixedVariable.FindStringSubmatch(variable); m != nil {
		return m[1]
	}
	return ""
}

// iamEC2RequestParameters indexes a query-protocol request's flat parameters by
// lower-cased name. EC2 serializes a list by repeating the member's singular
// name with a 1-based index (InstanceId.1, InstanceId.2), so the indices are
// collapsed back into one ordered slice and every element authorizes
// separately — terminating three instances must be allowed for all three, not
// only the first. Members of a nested structure (Filter.1.Name,
// TagSpecification.1.Tag.1.Key) name no resource and are left out.
func iamEC2RequestParameters(r *http.Request) map[string][]string {
	_ = r.ParseForm()
	byIndex := map[string]map[int]string{}
	for key, values := range r.Form {
		if len(values) == 0 || values[0] == "" {
			continue
		}
		name, index := key, 0
		if dot := strings.LastIndexByte(key, '.'); dot >= 0 {
			n, err := strconv.Atoi(key[dot+1:])
			if err != nil {
				continue
			}
			name, index = key[:dot], n
		}
		if strings.ContainsRune(name, '.') {
			continue
		}
		name = strings.ToLower(name)
		if byIndex[name] == nil {
			byIndex[name] = map[int]string{}
		}
		byIndex[name][index] = values[0]
	}
	params := make(map[string][]string, len(byIndex))
	for name, indexed := range byIndex {
		indices := make([]int, 0, len(indexed))
		for i := range indexed {
			indices = append(indices, i)
		}
		sort.Ints(indices)
		for _, i := range indices {
			params[name] = append(params[name], indexed[i])
		}
	}
	return params
}

// iamARNTrailingVariable matches the variable a published ARN format ends in,
// which names the resource's own identifier.
var iamARNTrailingVariable = regexp.MustCompile(`\$\{([A-Za-z0-9]+)\}$`)

func iamARNFormatVariable(format string) string {
	if m := iamARNTrailingVariable.FindStringSubmatch(format); m != nil {
		return m[1]
	}
	return ""
}

// iamFillARNFormat completes a published ARN format for one identifier. The
// simulator supplies the partition, region and account; a format that carries
// no account or no region has none to supply, and is left as AWS publishes it.
func iamFillARNFormat(format, region, account, id string) string {
	filled := strings.NewReplacer(
		"${Partition}", "aws",
		"${Region}", region,
		"${Account}", account,
	).Replace(format)
	return iamARNTrailingVariable.ReplaceAllLiteralString(filled, id)
}

// ===== Amazon Elastic Container Service =====

// iamECSResourceARNs derives the ARNs an Amazon ECS request names. ECS resource
// ARNs below the cluster embed the cluster in their path
// (task/<cluster>/<id>, service/<cluster>/<name>), so the cluster is resolved
// first; a request that omits it means the "default" cluster, exactly as the
// API does.
func iamECSResourceARNs(r *http.Request, op string, types []string, arn func(svc, resource string) string) []string {
	cluster := iamECSClusterName(r)
	var out []string
	add := func(resource string) {
		out = append(out, arn("ecs", resource))
	}
	// An identifier that already is an ARN is the resource itself; only bare
	// names need assembling.
	addNamed := func(prefix string, names []string) {
		for _, name := range names {
			if strings.HasPrefix(name, "arn:") {
				out = append(out, name)
				continue
			}
			add(prefix + name)
		}
	}

	if iamHasType(types, "task-set") {
		addNamed("task-set/"+cluster+"/"+iamFirstJSONField(r, "service")+"/",
			iamNamesFrom(r, []string{"taskSet"}, []string{"taskSets"}))
	}
	if iamHasType(types, "task") {
		addNamed("task/"+cluster+"/", iamNamesFrom(r, []string{"task"}, []string{"tasks"}))
	}
	if iamHasType(types, "service") {
		addNamed("service/"+cluster+"/",
			iamNamesFrom(r, []string{"service", "serviceName"}, []string{"services"}))
	}
	if iamHasType(types, "container-instance") {
		addNamed("container-instance/"+cluster+"/",
			iamNamesFrom(r, []string{"containerInstance"}, []string{"containerInstances"}))
	}
	if iamHasType(types, "capacity-provider") {
		addNamed("capacity-provider/",
			iamNamesFrom(r, []string{"capacityProvider", "name"}, []string{"capacityProviders"}))
	}
	if iamHasType(types, "task-definition") {
		addNamed("task-definition/", iamECSTaskDefinitionIDs(r))
	}
	if iamHasType(types, "cluster") {
		// DescribeClusters names them in a list; everything else names one, and
		// an omitted cluster is the default one.
		if named := iamNamesFrom(r, []string{"cluster", "clusterName"}, []string{"clusters"}); len(named) > 0 {
			addNamed("cluster/", named)
		} else if len(out) == 0 {
			add("cluster/" + cluster)
		}
	}
	return out
}

// iamECSClusterName resolves the cluster a request targets, accepting the name
// or the ARN the API accepts interchangeably and defaulting to "default".
func iamECSClusterName(r *http.Request) string {
	name := iamFirstJSONField(r, "cluster", "clusterName")
	if name == "" {
		if clusters := iamJSONBodyStrings(r, "clusters"); len(clusters) > 0 {
			name = clusters[0]
		}
	}
	if name == "" {
		return "default"
	}
	if i := strings.LastIndex(name, "/"); i >= 0 {
		return name[i+1:]
	}
	return name
}

// iamECSTaskDefinitionIDs returns the "<family>:<revision>" identifiers a
// request names. RegisterTaskDefinition is the interesting case: AWS
// authorizes it against the task definition it is about to create, so the
// requested resource carries the revision the call will be assigned — which
// the simulator, owning the revision counter, knows before it assigns it.
func iamECSTaskDefinitionIDs(r *http.Request) []string {
	if ids := iamNamesFrom(r, []string{"taskDefinition"}, []string{"taskDefinitions"}); len(ids) > 0 {
		return ids
	}
	family := iamJSONBodyField(r, "family")
	if family == "" {
		return nil
	}
	ecsRevisionMu.Lock()
	next := ecsRevisions[family] + 1
	ecsRevisionMu.Unlock()
	return []string{family + ":" + strconv.Itoa(next)}
}

// ===== Amazon CloudWatch Logs =====

// iamLogsResourceARNs derives the ARNs an Amazon CloudWatch Logs request names.
// The service defines two nested resource types and the distinction is not
// cosmetic: a log stream's ARN is the group's with ":log-stream:<name>"
// appended, so a policy granting the group alone does not cover the four
// stream-scoped actions and a policy written "<group-arn>:*" covers those but
// not the group-scoped reads. The gate requests whichever type AWS declares.
func iamLogsResourceARNs(r *http.Request, types []string, arn func(svc, resource string) string) []string {
	groups := iamNamesFrom(r,
		[]string{"logGroupName", "logGroupIdentifier"},
		[]string{"logGroupNames", "logGroupIdentifiers"})
	if len(groups) == 0 {
		return nil
	}
	stream := ""
	if iamHasType(types, "log-stream") {
		stream = iamJSONBodyField(r, "logStreamName")
	}
	var out []string
	for _, group := range groups {
		// A logGroupIdentifier may be the group's ARN rather than its name.
		base := arn("logs", "log-group:"+group)
		if strings.HasPrefix(group, "arn:") {
			base = strings.TrimSuffix(group, ":*")
		}
		if stream != "" {
			base += ":log-stream:" + stream
		}
		out = append(out, base)
	}
	return out
}

// ===== AWS CodeBuild =====

// iamCodeBuildResourceARNs derives the ARNs an AWS CodeBuild request names.
// The build-scoped operations authorize against the build's project, and a
// build id is "<projectName>:<uuid>", so the project comes out of the id.
func iamCodeBuildResourceARNs(r *http.Request, types []string, arn func(svc, resource string) string) []string {
	var out []string
	addNamed := func(prefix string, names []string) {
		for _, name := range names {
			if strings.HasPrefix(name, "arn:") {
				out = append(out, name)
				continue
			}
			out = append(out, arn("codebuild", prefix+name))
		}
	}
	if iamHasType(types, "project") {
		names := iamNamesFrom(r, []string{"projectName", "name"}, []string{"names"})
		for _, id := range iamNamesFrom(r, []string{"id"}, []string{"ids"}) {
			if project, _, ok := strings.Cut(id, ":"); ok && project != "" {
				names = append(names, project)
			}
		}
		addNamed("project/", names)
	}
	if iamHasType(types, "report-group") {
		addNamed("report-group/",
			iamNamesFrom(r, []string{"reportGroupArn", "name"}, []string{"reportGroupArns"}))
	}
	if iamHasType(types, "fleet") {
		addNamed("fleet/", iamNamesFrom(r, []string{"name"}, []string{"names"}))
	}
	return out
}

// ===== AWS WAFv2 =====

// iamWAFv2ResourceARNs derives the ARNs an AWS WAFv2 request names. WAFv2 is
// the one service here whose resource ARN cannot be assembled from the request
// alone in a general way: the ARN carries the resource's generated id and its
// scope path, and the type is the operation's own suffix (GetIPSet names an
// ipset, GetWebACL a webacl). wafARN builds it exactly as the handlers do, so
// a derived ARN is the ARN the resource actually has.
//
// Operations that reference other entities inside a rule statement —
// CreateWebACL naming rule groups and IP sets — authorize against the entity
// the request creates or reads; the referenced entities are not derived, and
// the gate never widens beyond what the request names.
func iamWAFv2ResourceARNs(r *http.Request, op string, types []string) []string {
	resourceType := ""
	for _, candidate := range []struct{ suffix, resource string }{
		{"WebACL", "webacl"},
		{"IPSet", "ipset"},
		{"RuleGroup", "rulegroup"},
		{"RegexPatternSet", "regexpatternset"},
		{"ManagedRuleSet", "managedruleset"},
	} {
		if strings.HasSuffix(op, candidate.suffix) && iamHasType(types, candidate.resource) {
			resourceType = candidate.resource
			break
		}
	}
	if resourceType == "" {
		return nil
	}
	if a := iamFirstJSONField(r, "ARN", "WebACLArn"); strings.HasPrefix(a, "arn:") {
		return []string{a}
	}
	name, id := iamJSONBodyField(r, "Name"), iamJSONBodyField(r, "Id")
	if name == "" {
		return nil
	}
	return []string{wafARN(iamJSONBodyField(r, "Scope"), resourceType, name, id)}
}
