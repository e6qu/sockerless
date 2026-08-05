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
	case "glue":
		return iamGlueResourceARNs(r, types, region, account)
	case "logs":
		return iamLogsResourceARNs(r, types, arn)
	case "rds":
		return iamRDSResourceARNs(r, types, region, account)
	case "ssm":
		return iamSSMResourceARNs(r, types, region, account)
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
	// The query-protocol services name it as a form parameter instead. Amazon
	// RDS is the one that does so under three spellings: its tagging operations
	// send the ARN as ResourceName, its activity streams as ResourceArn, and its
	// maintenance operations as ResourceIdentifier. Only a value that is an ARN
	// is taken, so a parameter of the same name carrying a bare name elsewhere
	// is left to that service's own derivation.
	for _, field := range []string{"ResourceName", "ResourceArn", "ResourceARN", "ResourceIdentifier"} {
		if v := r.FormValue(field); strings.HasPrefix(v, "arn:") {
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
	params := iamQueryRequestParameters(r)
	return iamTableDrivenARNs("ec2", types, region, account, iamEC2ParameterAliases,
		func(field string) []string { return params[strings.ToLower(field)] })
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

// iamQueryRequestParameters indexes a query-protocol request's flat parameters
// by lower-cased name, collapsing both encodings a list arrives in so every
// element authorizes separately — terminating three instances must be allowed
// for all three, not only the first.
//
// Amazon EC2's protocol flattens a list to the member's singular name with a
// 1-based index (InstanceId.1, InstanceId.2). The awsQuery protocol Amazon RDS
// speaks boxes it instead (Names.member.1) unless the member is flattened.
// Members of a nested structure (Filters.Filter.1.Name) name no resource and
// are left out.
func iamQueryRequestParameters(r *http.Request) map[string][]string {
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
		name = strings.TrimSuffix(name, ".member")
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

// ===== Deriving an ARN from the format the reference publishes =====

// iamTableDrivenARNs derives the ARNs a request names for a service whose
// resource types are in the generated table, by filling each type's published
// ARN format from the identifiers the request supplies. Everything that differs
// between services lives in the two arguments: aliases carries the renamings
// that service made and the reference did not follow, and lookup reads a field,
// which is the only thing the protocols disagree about — Amazon EC2 and Amazon
// RDS name their identifiers as query parameters, AWS Glue as JSON members.
//
// A format naming no identifier is a constant ARN — AWS Glue's root catalog is
// "arn:aws:glue:<region>:<account>:catalog" and every request that authorizes
// against it names it by existing — and is emitted as it stands.
//
// Two resource types can resolve to the same field, and then the identifier
// belongs to one of them without the request saying which. Sometimes that is
// answerable: RunInstances authorizes against a subnet and a secondary subnet,
// and a SubnetId is the subnet's published variable outright where it reaches
// the secondary only through the prefix-drop rule, so the exact spelling takes
// it. Where the claims are equally strong neither is derived — building both
// would invent an ARN for a resource that does not exist and then require it to
// be allowed, denying a policy that named the real one — and the request's
// other fields still are.
func iamTableDrivenARNs(service string, types []string, region, account string,
	aliases map[string][]string, lookup func(field string) []string) []string {

	type resolved struct {
		format string
		// variable is the last one the format declares, kept so the value that
		// fills it gets the transformation its name states.
		variable string
		// parents are the values filling every variable but the last: a Glue
		// table's ARN carries its database, which the request names once.
		parents []string
		// field and rank identify the last variable's source, which is the
		// resource's own identifier and the only one that may name several.
		field string
		rank  int
	}
	best := map[string]int{}
	var found []resolved
	var constants []string

	for _, resourceType := range types {
		format, declared := iamResourceARNFormats[service+":"+resourceType]
		if !declared {
			continue
		}
		variables := iamARNFormatVariables(format)
		if len(variables) == 0 {
			constants = append(constants, iamFillARNFormat(format, region, account, nil))
			continue
		}
		// Every variable has to be named. A partially filled ARN would carry a
		// literal "${DatabaseName}" and match nothing.
		parents := make([]string, 0, len(variables)-1)
		complete := true
		for _, variable := range variables[:len(variables)-1] {
			values := iamFirstNamed(lookup, aliases, resourceType, variable)
			if len(values) == 0 {
				complete = false
				break
			}
			parents = append(parents, iamARNValueForVariable(variable, values[0]))
		}
		if !complete {
			continue
		}
		field, rank, named := iamNamingField(lookup, aliases, resourceType, variables[len(variables)-1])
		if !named {
			continue
		}
		if seen, ok := best[field]; !ok || rank < seen {
			best[field] = rank
		}
		found = append(found, resolved{format, variables[len(variables)-1], parents, field, rank})
	}

	contested := map[string]int{}
	for _, f := range found {
		if f.rank == best[f.field] {
			contested[f.field]++
		}
	}

	var out []string
	seen := map[string]struct{}{}
	add := func(resource string) {
		if _, dup := seen[resource]; dup {
			return
		}
		seen[resource] = struct{}{}
		out = append(out, resource)
	}
	for _, f := range found {
		if f.rank != best[f.field] || contested[f.field] > 1 {
			continue
		}
		for _, id := range lookup(f.field) {
			// Some types are named by another service's ARN outright (Amazon
			// EC2's CertificateArn and RoleArn), which needs no assembly.
			if strings.HasPrefix(id, "arn:") {
				add(id)
				continue
			}
			add(iamFillARNFormat(f.format, region, account,
				append(append([]string{}, f.parents...), iamARNValueForVariable(f.variable, id))))
		}
	}
	for _, c := range constants {
		add(c)
	}
	return out
}

// iamFirstNamed returns the values a request carries for one ARN format
// variable, under whichever spelling it supplies.
func iamFirstNamed(lookup func(string) []string, aliases map[string][]string, resourceType, variable string) []string {
	if field, _, ok := iamNamingField(lookup, aliases, resourceType, variable); ok {
		return lookup(field)
	}
	return nil
}

// iamNamingField returns the field a request names an ARN format variable
// under. The rank is that spelling's position in the candidate list, which runs
// most specific first, so a lower rank is the stronger claim on a field two
// resource types both resolve to.
func iamNamingField(lookup func(string) []string, aliases map[string][]string, resourceType, variable string) (string, int, bool) {
	for rank, name := range iamVariableFieldNames(resourceType, variable, aliases) {
		if len(lookup(name)) > 0 {
			return name, rank, true
		}
	}
	return "", 0, false
}

// iamVariableFieldNames returns the field spellings an ARN format variable can
// arrive under, most specific first: the variable itself, then that service's
// declared renamings, then the form with the resource's own leading word
// dropped, and finally the plural of each — a batch operation names the same
// resource in a list (AWS Glue's BatchGetJobs sends JobNames).
//
// A declared renaming outranks the prefix drop because it is evidence and the
// drop is a guess: every alias is held to the vendored model by a test, where
// the drop is a rule about spelling that can land on a field meaning something
// else. AWS Glue is where the order shows. A catalog's ${CatalogName} drops to
// Name, which on GetTable is the *table's* name, so ranking the drop first made
// the catalog and the table claim one field and the ambiguity rule then
// discarded both. The catalog's declared CatalogId settles it.
func iamVariableFieldNames(resourceType, variable string, aliases map[string][]string) []string {
	names := []string{variable}
	// A variable name is only unique within a resource type: AWS Systems
	// Manager calls the identifier of a maintenance window, an OpsItem, its
	// metadata and a service setting all ${ResourceId}, and the request names
	// each of them differently. An entry keyed "<type>.<variable>" answers for
	// that type alone, so the four do not resolve to one another's field and
	// cancel each other out as an ambiguity.
	names = append(names, aliases[resourceType+"."+variable]...)
	names = append(names, aliases[variable]...)
	if unprefixed := iamUnprefixedVariable(variable); unprefixed != "" {
		names = append(names, unprefixed)
	}
	for _, n := range append([]string{}, names...) {
		names = append(names, n+"s")
	}
	return names
}

// iamPrefixedVariable matches a variable whose leading word names the resource
// itself, capturing the rest — the form the APIs abbreviate to
// (SecurityGroupId → GroupId, PlacementGroupName → GroupName, and AWS Glue's
// BlueprintName → Name).
var iamPrefixedVariable = regexp.MustCompile(`^[A-Z][a-z]+((?:[A-Z][a-z]*)*(?:Id|Name))$`)

func iamUnprefixedVariable(variable string) string {
	if m := iamPrefixedVariable.FindStringSubmatch(variable); m != nil && m[1] != "" {
		return m[1]
	}
	return ""
}

// iamARNVariable matches one ${...} placeholder in a published ARN format.
var iamARNVariable = regexp.MustCompile(`\$\{([A-Za-z0-9]+)\}`)

// iamARNFormatVariables returns the identifiers a published ARN format needs,
// in the order they appear. The partition, region and account are the
// simulator's own and are not among them.
func iamARNFormatVariables(format string) []string {
	var out []string
	for _, m := range iamARNVariable.FindAllStringSubmatch(format, -1) {
		switch m[1] {
		case "Partition", "Region", "Account":
			continue
		}
		out = append(out, m[1])
	}
	return out
}

// iamARNValueForVariable applies the transformation a variable's own name
// states. AWS Systems Manager publishes a parameter's ARN as
// "parameter/${ParameterNameWithoutLeadingSlash}", and a parameter is named
// "/db/password" in every request, so the ARN of that parameter is
// "…:parameter/db/password". Keeping the slash would build an ARN with an empty
// first path segment, matching no policy.
func iamARNValueForVariable(variable, value string) string {
	if strings.HasSuffix(variable, "WithoutLeadingSlash") {
		return strings.TrimPrefix(value, "/")
	}
	return value
}

// iamFillARNFormat completes a published ARN format. The simulator supplies the
// partition, region and account; the values fill the identifier variables in
// order. A format that carries no account or no region has none to supply and
// is left as AWS publishes it.
func iamFillARNFormat(format, region, account string, values []string) string {
	filled := strings.NewReplacer(
		"${Partition}", "aws",
		"${Region}", region,
		"${Account}", account,
	).Replace(format)
	i := 0
	return iamARNVariable.ReplaceAllStringFunc(filled, func(string) string {
		if i >= len(values) {
			return ""
		}
		v := values[i]
		i++
		return v
	})
}

// ===== AWS Glue =====

// iamGlueResourceARNs derives the ARNs an AWS Glue request names. Glue speaks
// awsJson, so the identifiers are members of the request body rather than query
// parameters, and it is the service that makes the two general shapes of the
// derivation earn their keep.
//
// Its ARN formats nest: a table is "table/${DatabaseName}/${TableName}" and a
// table version adds a third part, so an ARN is only built when the request
// names every part — a half-filled ARN would carry a literal ${DatabaseName}
// and match no policy. Its root catalog is the other extreme, a format naming
// no identifier at all, so every request that authorizes against the catalog
// names it by existing.
//
// Glue also abbreviates almost every identifier the same way: the reference
// calls a blueprint's identifier ${BlueprintName} and the request member is
// Name, which the prefix drop resolves, and a batch operation sends the plural
// of whichever spelling it uses.
func iamGlueResourceARNs(r *http.Request, types []string, region, account string) []string {
	fields := iamJSONRequestFields(r)
	return iamTableDrivenARNs("glue", types, region, account, iamGlueFieldAliases,
		func(field string) []string { return fields[strings.ToLower(field)] })
}

// iamGlueFieldAliases maps an ARN format's identifier variable to the request
// members AWS Glue spells it as, where the two differ by more than the
// mechanical prefix drop.
//
// TestIAMGlueFieldAliasesAreRealRequestMembers holds every entry to a member
// the vendored AWS Glue model declares on an operation authorizing against that
// resource type.
var iamGlueFieldAliases = map[string][]string{
	"CatalogName":             {"CatalogId"},
	"UserDefinedFunctionName": {"FunctionName"},
}

// iamJSONRequestFields indexes an awsJson request body's top-level members by
// lower-cased name, reading a member that carries one identifier and one that
// carries a list the same way, since a batch operation authorizes every entry.
// Members holding anything else name no resource and are left out.
func iamJSONRequestFields(r *http.Request) map[string][]string {
	body := iamRequestBody(r)
	if len(body) == 0 {
		return nil
	}
	var raw map[string]json.RawMessage
	if json.Unmarshal(body, &raw) != nil {
		return nil
	}
	fields := make(map[string][]string, len(raw))
	for name, value := range raw {
		var one string
		if json.Unmarshal(value, &one) == nil {
			if one != "" {
				fields[strings.ToLower(name)] = []string{one}
			}
			continue
		}
		var many []string
		if json.Unmarshal(value, &many) == nil {
			var kept []string
			for _, v := range many {
				if v != "" {
					kept = append(kept, v)
				}
			}
			if len(kept) > 0 {
				fields[strings.ToLower(name)] = kept
			}
		}
	}
	return fields
}

// ===== Amazon Relational Database Service =====

// iamRDSResourceARNs derives the ARNs an Amazon RDS request names. RDS speaks
// the awsQuery protocol, so the identifiers are form parameters, and it is the
// service where the reference and the API disagree about spelling on almost
// every resource: the reference calls a database instance ${DbInstanceName} and
// a cluster parameter group ${ClusterParameterGroupName} where the API sends
// DBInstanceIdentifier and DBClusterParameterGroupName. Those renamings are the
// whole of iamRDSFieldAliases, and each was read off the vendored model — the
// parameter the operations authorizing against that type actually take —
// rather than derived from the name by a pattern.
//
// Two of the twenty-four types derive nothing, and deliberately. A custom
// engine version's ARN carries an engine, a version and the version's own
// identifier, and a request names only the first two; a proxy target group's
// carries an identifier no request supplies. The simulator's own builders for
// both disagree with the published shape, which is a defect in the ARNs it
// assigns rather than something to paper over here.
func iamRDSResourceARNs(r *http.Request, types []string, region, account string) []string {
	params := iamQueryRequestParameters(r)
	return iamTableDrivenARNs("rds", types, region, account, iamRDSFieldAliases,
		func(field string) []string { return params[strings.ToLower(field)] })
}

// iamRDSFieldAliases maps an ARN format's identifier variable to the request
// parameter Amazon RDS spells it as.
//
// TestIAMRDSFieldAliasesAreRealRequestParameters holds every entry to a
// parameter the vendored model declares on an operation that authorizes
// against that resource type.
var iamRDSFieldAliases = map[string][]string{
	"ClusterParameterGroupName": {"DBClusterParameterGroupName"},
	"ClusterSnapshotName":       {"DBClusterSnapshotIdentifier"},
	"DbClusterEndpoint":         {"DBClusterEndpointIdentifier"},
	"DbClusterInstanceName":     {"DBClusterIdentifier"},
	"DbInstanceName":            {"DBInstanceIdentifier"},
	"DbProxyEndpointId":         {"DBProxyEndpointName"},
	"DbProxyId":                 {"DBProxyName"},
	"DbShardGroupResourceId":    {"DBShardGroupIdentifier"},
	"GlobalCluster":             {"GlobalClusterIdentifier"},
	"ParameterGroupName":        {"DBParameterGroupName"},
	"ReservedDbInstanceName":    {"ReservedDBInstanceId"},
	"SecurityGroupName":         {"DBSecurityGroupName"},
	"SnapshotName":              {"DBSnapshotIdentifier"},
	"SubnetGroupName":           {"DBSubnetGroupName"},
}

// ===== AWS Systems Manager =====

// iamSSMResourceARNs derives the ARNs an AWS Systems Manager request names.
// Systems Manager speaks awsJson, so the identifiers are request members, and
// it is the service that makes a variable's *name* carry more than a spelling.
//
// Four of its resource types — a maintenance window, an OpsItem, that item's
// metadata and a service setting — are all published as ${ResourceId}, and the
// request names each differently, so their aliases are keyed by resource type
// rather than by variable. A parameter is published as
// ${ParameterNameWithoutLeadingSlash} and named "/db/password" in every
// request, so the value loses its leading slash on the way into the ARN, which
// is the transformation the variable's own name states.
//
// Four more of its types are another service's resource outright: an instance
// is an Amazon EC2 ARN, a task an Amazon ECS one, a role an IAM one, and a
// bucket an Amazon S3 ARN carrying neither region nor account. Filling the
// published format is what keeps each of those right.
func iamSSMResourceARNs(r *http.Request, types []string, region, account string) []string {
	fields := iamJSONRequestFields(r)
	return iamTableDrivenARNs("ssm", types, region, account, iamSSMFieldAliases,
		func(field string) []string { return fields[strings.ToLower(field)] })
}

// iamSSMFieldAliases maps an ARN format's identifier variable to the request
// members AWS Systems Manager spells it as. An entry keyed "<type>.<variable>"
// answers for that resource type alone.
//
// TestIAMSSMFieldAliasesAreRealRequestMembers holds every entry to a member the
// vendored model declares on an operation authorizing against that type.
var iamSSMFieldAliases = map[string][]string{
	"maintenancewindow.ResourceId":               {"WindowId"},
	"opsitem.ResourceId":                         {"OpsItemId", "OpsItemArn"},
	"opsmetadata.ResourceId":                     {"OpsMetadataArn"},
	"servicesetting.ResourceId":                  {"SettingId"},
	"patchbaseline.PatchBaselineIdResourceId":    {"BaselineId"},
	"parameter.ParameterNameWithoutLeadingSlash": {"Name"},
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
