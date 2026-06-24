package main

import (
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
)

// s3ImplementedOps enumerates the S3 operations the sim implements. S3 is REST:
// its operation isn't a router registration but is composed at request time from
// method + path + the query subresource (s3BucketOperationName /
// s3ObjectOperationName). So we drive those functions over the method ×
// subresource matrix and collect the operation names they yield — the REST
// analogue of reading the awsJson/query routers.
func s3ImplementedOps() map[string]bool {
	ops := map[string]bool{"ListBuckets": true}
	bucketReq := func(method, rawquery string) string {
		r := httptest.NewRequest(method, "/bucket?"+rawquery, nil)
		return s3BucketOperationName(r, nil)
	}
	objReq := func(method, rawquery string, hdr map[string]string) string {
		r := httptest.NewRequest(method, "/bucket/key?"+rawquery, nil)
		for k, v := range hdr {
			r.Header.Set(k, v)
		}
		return s3ObjectOperationName(r, nil)
	}
	add := func(op string) {
		if op != "" {
			ops[op] = true
		}
	}

	bucketSubresources := []string{
		"acl", "cors", "lifecycle", "policy", "versioning", "website", "logging",
		"requestPayment", "accelerate", "replication", "encryption", "tagging",
		"notification", "publicAccessBlock", "object-lock", "ownershipControls",
		"intelligent-tiering", "inventory", "analytics", "metrics",
		"uploads", "versions", "location", "policyStatus", "delete",
	}
	for _, m := range []string{"GET", "PUT", "DELETE", "HEAD", "POST"} {
		add(bucketReq(m, ""))
		for _, sr := range bucketSubresources {
			add(bucketReq(m, sr+"="))
			add(bucketReq(m, sr+"=&id=x"))
		}
	}
	// ListObjectsV2 is the same GET-bucket request with list-type=2 (a bare GET
	// resolves to the V1 ListObjects).
	add(bucketReq("GET", "list-type=2"))
	objQueries := []string{
		"", "tagging=", "uploads=", "uploadId=x", "uploadId=x&partNumber=1",
		"acl=", "retention=", "legal-hold=", "attributes=", "torrent=", "restore=", "select=",
	}
	for _, m := range []string{"GET", "PUT", "DELETE", "HEAD", "POST"} {
		for _, q := range objQueries {
			add(objReq(m, q, nil))
		}
	}
	add(objReq("PUT", "", map[string]string{"x-amz-copy-source": "/src/key"}))
	// UploadPartCopy is an UploadPart request (uploadId+partNumber) carrying an
	// x-amz-copy-source header.
	add(objReq("PUT", "uploadId=x&partNumber=1", map[string]string{"x-amz-copy-source": "/src/key"}))

	// The sim composes some op names with a "Bucket" infix that the real S3 API
	// omits (GetBucketPublicAccessBlock vs the API's GetPublicAccessBlock) and
	// names a couple of subresources more verbosely; expose the API-canonical
	// aliases so functional coverage isn't undercounted by a naming difference.
	for op := range ops {
		if alias := strings.Replace(op, "BucketPublicAccessBlock", "PublicAccessBlock", 1); alias != op {
			ops[alias] = true
		}
	}
	if ops["DeleteBucketLifecycleConfiguration"] {
		ops["DeleteBucketLifecycle"] = true
	}
	return ops
}

// Service API conformance: a spec-derived, mechanical measure of how completely a
// sim service implements its real AWS operation surface, so gaps are *measured*
// (and ratcheted) rather than discovered later by a consumer. This is the
// service-API analogue of the IAM policy-engine conformance gate
// (iam_conformance_test.go); the repeatable process is docs/SERVICE_CONFORMANCE.md.
//
// For a service it loads the vendored Smithy model (the authoritative operation
// list), computes which operations the sim's routers register, and reports
// coverage. TestServiceConformance_Ratchet locks each service's set of
// not-yet-implemented operations: the list IS the live non-conformity report,
// and an op that gets implemented (or a model that grows) must be reflected here.

// serviceConformanceCatalog maps a service's Smithy shape name to the set of its
// real operations the sim does NOT implement yet — the tracked non-conformities.
// The ratchet locks this; implement an op (or grow the model) and the entry must
// shrink/grow with it. Discovered + maintained via TestServiceConformance_Coverage.
//
// Scope: the awsJson (X-Amz-Target) + awsQuery (versioned Action) services, whose
// operations are introspectable from the routers. REST services compose their
// operation from method + path + query subresource at request time (e.g. S3's
// op-name is `Get/Put/Delete` + a subresource suffix, not a router registration),
// so they are measured by their own enumeration harness instead — S3 via
// s3ImplementedOps + TestServiceConformance_S3Ratchet.
var serviceConformanceCatalog = map[string][]string{
	// CloudWatch monitoring: the dataset/KMS, OTel-enrichment, managed-insight-rule,
	// and metric-widget-image surfaces remain.
	"GraniteServiceVersion20100801": {
		"AssociateDatasetKmsKey", "DescribeAlarmContributors", "DisassociateDatasetKmsKey",
		"GetDataset", "GetInsightRuleReport", "GetMetricWidgetImage", "GetOTelEnrichment",
		"ListManagedInsightRules", "PutManagedInsightRules", "StartOTelEnrichment",
		"StopOTelEnrichment",
	},
	// Organizations: the GovCloud account, responsibility-transfer, and
	// effective-policy-validation surfaces remain.
	"AWSOrganizationsV20161128": {
		"CreateGovCloudAccount", "DescribeResponsibilityTransfer",
		"InviteOrganizationToTransferResponsibility", "LeaveOrganization",
		"ListAccountsWithInvalidEffectivePolicy", "ListEffectivePolicyValidationErrors",
		"ListInboundResponsibilityTransfers", "ListOutboundResponsibilityTransfers",
		"TerminateResponsibilityTransfer", "UpdateResponsibilityTransfer",
	},
	// SSM: Parameter Store + documents + maintenance windows + patch baselines +
	// service settings + resource data sync are implemented; the run/automation/
	// session/inventory/compliance/ops-item/association execution subsystems remain.
	"AmazonSSM": {
		"AssociateOpsItemRelatedItem", "CancelCommand", "CancelMaintenanceWindowExecution",
		"CreateActivation", "CreateAssociation", "CreateAssociationBatch", "CreateOpsItem",
		"CreateOpsMetadata", "DeleteActivation", "DeleteAssociation", "DeleteInventory",
		"DeleteOpsItem", "DeleteOpsMetadata", "DeleteResourcePolicy",
		"DeregisterManagedInstance", "DeregisterPatchBaselineForPatchGroup",
		"DescribeActivations", "DescribeAssociation", "DescribeAssociationExecutionTargets",
		"DescribeAssociationExecutions", "DescribeAutomationExecutions",
		"DescribeAutomationStepExecutions", "DescribeAvailablePatches",
		"DescribeDocumentPermission", "DescribeEffectiveInstanceAssociations",
		"DescribeEffectivePatchesForPatchBaseline", "DescribeInstanceAssociationsStatus",
		"DescribeInstanceInformation", "DescribeInstancePatchStates",
		"DescribeInstancePatchStatesForPatchGroup", "DescribeInstancePatches",
		"DescribeInstanceProperties", "DescribeInventoryDeletions",
		"DescribeMaintenanceWindowExecutionTaskInvocations",
		"DescribeMaintenanceWindowExecutionTasks", "DescribeMaintenanceWindowExecutions",
		"DescribeMaintenanceWindowSchedule", "DescribeMaintenanceWindowsForTarget",
		"DescribeOpsItems", "DescribePatchGroupState", "DescribePatchGroups",
		"DescribePatchProperties", "DescribeSessions", "DisassociateOpsItemRelatedItem",
		"GetAccessToken", "GetAutomationExecution", "GetCalendarState", "GetCommandInvocation",
		"GetConnectionStatus", "GetDeployablePatchSnapshotForInstance", "GetExecutionPreview",
		"GetInventory", "GetInventorySchema", "GetMaintenanceWindowExecution",
		"GetMaintenanceWindowExecutionTask", "GetMaintenanceWindowExecutionTaskInvocation",
		"GetMaintenanceWindowTask", "GetOpsItem", "GetOpsMetadata", "GetOpsSummary",
		"GetParameterHistory", "GetPatchBaselineForPatchGroup", "GetResourcePolicies",
		"LabelParameterVersion", "ListAssociationVersions", "ListAssociations",
		"ListCommandInvocations", "ListCommands", "ListComplianceItems",
		"ListComplianceSummaries", "ListDocumentMetadataHistory", "ListInventoryEntries",
		"ListNodes", "ListNodesSummary", "ListOpsItemEvents", "ListOpsItemRelatedItems",
		"ListOpsMetadata", "ListResourceComplianceSummaries", "ModifyDocumentPermission",
		"PutComplianceItems", "PutInventory", "PutResourcePolicy",
		"RegisterPatchBaselineForPatchGroup", "ResumeSession", "SendAutomationSignal",
		"SendCommand", "StartAccessRequest", "StartAssociationsOnce", "StartAutomationExecution",
		"StartChangeRequestExecution", "StartExecutionPreview", "StartSession",
		"StopAutomationExecution", "TerminateSession", "UnlabelParameterVersion",
		"UpdateAssociation", "UpdateAssociationStatus", "UpdateDocumentMetadata",
		"UpdateMaintenanceWindowTarget", "UpdateMaintenanceWindowTask",
		"UpdateManagedInstanceRole", "UpdateOpsItem", "UpdateOpsMetadata",
	},
	// Step Functions / ACM / Secrets Manager / Application Auto Scaling: all
	// operations implemented (conformance-complete).
	"AWSStepFunctions":        {},
	"CertificateManager":      {},
	"secretsmanager":          {},
	"AnyScaleFrontendService": {},
	// Kinesis: complete except the HTTP/2 event-stream consumer subscription.
	"Kinesis_20131202": {"SubscribeToShard"},
	// KMS: all operations implemented (real Go-stdlib crypto for Sign/Verify/MAC/
	// data-key-pairs/ECDH; custom key stores; grants; multi-region keys).
	"TrentService": {},
	// ELBv2: all operations implemented (the mutual-TLS trust-store surface closed).
	"ElasticLoadBalancing_v10": {},
	"AmazonSQS": {
		"CancelMessageMoveTask", "ListDeadLetterSourceQueues", "ListMessageMoveTasks",
		"StartMessageMoveTask",
	},
	// SNS / EventBridge / DynamoDB: all remaining operations are implemented.
	"AmazonSimpleNotificationService": {},
	"AWSEvents":                       {},
	"DynamoDB_20120810":               {},
	// ECS: all real operations are implemented.
	"AmazonEC2ContainerServiceV20141113": {},
}

// serviceRegisteredOps returns the set of operations the sim registers for the
// given Smithy model, across the awsJson (X-Amz-Target) and awsQuery routers.
func serviceRegisteredOps(m *smithyService, jsonTargets []string, versioned map[string][]string) map[string]bool {
	out := map[string]bool{}
	for _, target := range jsonTargets {
		i := strings.LastIndex(target, ".")
		if i < 0 {
			continue
		}
		prefix, op := target[:i], target[i+1:]
		if prefix == m.ShapeName || (strings.Contains(prefix, ".") && prefix[strings.LastIndex(prefix, ".")+1:] == m.ShapeName) || strings.HasPrefix(prefix, m.ShapeName+"_") {
			out[op] = true
		}
	}
	for _, a := range versioned[m.Version] {
		out[a] = true
	}
	// The query router's legacy bucket (version "") holds actions registered
	// without an explicit API version — EC2, STS, and other services use the
	// unversioned r.Register. Intersecting it with the model's own op set (in
	// serviceCoverage) attributes those actions to the right service.
	if m.Version != "" {
		for _, a := range versioned[""] {
			out[a] = true
		}
	}
	return out
}

// serviceCoverage computes a model's operation coverage: the ops registered and
// the ops still missing (model ∖ registered), sorted.
func serviceCoverage(m *smithyService, jsonTargets []string, versioned map[string][]string) (registered, missing []string) {
	reg := serviceRegisteredOps(m, jsonTargets, versioned)
	for op := range m.Ops {
		if reg[op] {
			registered = append(registered, op)
		} else {
			missing = append(missing, op)
		}
	}
	sort.Strings(registered)
	sort.Strings(missing)
	return registered, missing
}

// restConformanceSources maps a REST service's Smithy shape to its CloudTrail
// event source, so the gate reads restRegisteredOps (populated at registration
// by cloudTrailRecordedREST) for its operation coverage — the REST analogue of
// reading the awsJson/awsQuery routers.
var restConformanceSources = map[string]string{
	"Cloudfront2020_05_31":         "cloudfront.amazonaws.com", // Amazon CloudFront
	"ApiGatewayV2":                 "apigateway.amazonaws.com", // Amazon API Gateway v2
	"AWSGirApiService":             "lambda.amazonaws.com",     // AWS Lambda
	"AWSBatchV20160810":            "batch.amazonaws.com",      // AWS Batch
	"BackplaneControlService":      "apigateway.amazonaws.com", // Amazon API Gateway
	"Amplify":                      "amplify.amazonaws.com",
	"AWSChronosService":            "scheduler.amazonaws.com",         // EventBridge Scheduler
	"AWSDnsV20130401":              "route53.amazonaws.com",           // Amazon Route 53
	"MagnolioAPIService_v20150201": "elasticfilesystem.amazonaws.com", // Amazon EFS
}

// serviceImplementedCount returns how many of a model's operations the sim
// implements — from restRegisteredOps for a REST service, else from the routers.
func serviceImplementedCount(m *smithyService, jsonTargets []string, versioned map[string][]string) int {
	if src, ok := restConformanceSources[m.ShapeName]; ok {
		n := 0
		for op := range m.Ops {
			if restRegisteredOps[src][op] {
				n++
			}
		}
		return n
	}
	registered, _ := serviceCoverage(m, jsonTargets, versioned)
	return len(registered)
}

// serviceCoverageFloor locks the implemented-operation COUNT for services tracked
// by coverage rather than an exact missing-list: the awsQuery/ec2Query giants
// (Amazon EC2, RDS, Glue, …) whose hundreds of unimplemented ops would bloat the
// catalog, and the REST services (Route 53, EFS) measured via restRegisteredOps.
// The count must EQUAL the floor — a drop is a regression; implementing more ops
// must bump the floor (the ratchet ratchets up).
var serviceCoverageFloor = map[string]int{
	"AmazonEC2":                            122, // ec2Query
	"AWSSecurityTokenServiceV20110615":     4,   // STS (awsQuery, unversioned)
	"AmazonEC2ContainerRegistry_V20150921": 38,  // ECR
	"AmazonElastiCacheV9":                  41,
	"AmazonRDSv19":                         64,
	"AutoScaling_2011_01_01":               25,
	"AWSGlue":                              102,
	"AWSWAF_20190729":                      32,
	"CloudTrail_20131101":                  23,
	"CodeBuild_20161006":                   22,
	"Logs_20140328":                        36, // CloudWatch Logs
	"Route53AutoNaming_v20170314":          16, // Cloud Map / ServiceDiscovery
	"AWSDnsV20130401":                      33, // Route 53 (REST)
	"MagnolioAPIService_v20150201":         29, // EFS (REST)
	// restJson1 services measured via the REST registry (Part B).
	"AWSGirApiService":        62, // AWS Lambda
	"AWSBatchV20160810":       24, // AWS Batch
	"BackplaneControlService": 62, // Amazon API Gateway
	"Amplify":                 37,
	"AWSChronosService":       9,  // EventBridge Scheduler
	"ApiGatewayV2":            44, // Amazon API Gateway v2
	"Cloudfront2020_05_31":    67, // Amazon CloudFront (restXml)

}

// TestServiceConformance_CoverageFloor locks the implemented-op count for the
// coverage-tracked services (see serviceCoverageFloor).
func TestServiceConformance_CoverageFloor(t *testing.T) {
	models := loadSmithyModels(t)
	_, jsonRouter, queryRouter := buildConformanceSimulator(t)
	jsonTargets := jsonRouter.Targets()
	versioned := queryRouter.VersionedActions()
	for shape, floor := range serviceCoverageFloor {
		m := serviceModel(t, models, shape)
		impl := serviceImplementedCount(m, jsonTargets, versioned)
		if impl != floor {
			t.Errorf("%s: coverage %d/%d != floor %d — update serviceCoverageFloor (a drop is a regression; more is a ratchet-up).", shape, impl, len(m.Ops), floor)
		}
	}
}

func serviceModel(t *testing.T, models []*smithyService, shapeName string) *smithyService {
	t.Helper()
	for _, m := range models {
		if m.ShapeName == shapeName {
			return m
		}
	}
	t.Fatalf("no vendored Smithy model with service shape %q (run scripts/fetch-aws-spec.sh)", shapeName)
	return nil
}

// TestServiceConformance_Coverage reports, for each catalogued service, how many
// of its real operations the sim implements — a measured number, logged.
func TestServiceConformance_Coverage(t *testing.T) {
	models := loadSmithyModels(t)
	_, jsonRouter, queryRouter := buildConformanceSimulator(t)
	jsonTargets := jsonRouter.Targets()
	versioned := queryRouter.VersionedActions()

	for shape := range serviceConformanceCatalog {
		m := serviceModel(t, models, shape)
		registered, missing := serviceCoverage(m, jsonTargets, versioned)
		t.Logf("%s: %d/%d operations implemented; missing (%d): %v",
			shape, len(registered), len(m.Ops), len(missing), missing)
	}
}

// s3ConformanceMissing is S3's ratchet — the real S3 operations the REST sim does
// not implement (mostly newer/niche surfaces: S3 Express directory buckets, the
// bucket Metadata-table feature, ABAC, Object Lambda, S3 Select, Glacier restore,
// and object ACL/lock/retention/legal-hold). The list is locked by
// TestServiceConformance_S3Ratchet; implement one → remove it here.
var s3ConformanceMissing = []string{
	"CreateBucketMetadataConfiguration", "CreateBucketMetadataTableConfiguration",
	"CreateSession", "DeleteBucketMetadataConfiguration", "DeleteBucketMetadataTableConfiguration",
	"GetBucketAbac", "GetBucketMetadataConfiguration", "GetBucketMetadataTableConfiguration",
	"ListDirectoryBuckets", "PutBucketAbac", "RenameObject", "SelectObjectContent",
	"UpdateBucketMetadataInventoryTableConfiguration", "UpdateBucketMetadataJournalTableConfiguration",
	"UpdateObjectEncryption", "WriteGetObjectResponse",
}

// TestServiceConformance_S3Ratchet locks S3's REST operation-coverage gap set,
// measured by driving the request→operation-name composition over the method ×
// subresource matrix (s3ImplementedOps).
func TestServiceConformance_S3Ratchet(t *testing.T) {
	models := loadSmithyModels(t)
	m := serviceModel(t, models, "AmazonS3")
	impl := s3ImplementedOps()
	want := map[string]bool{}
	for _, op := range s3ConformanceMissing {
		want[op] = true
	}
	var newlyMissing, nowImplemented []string
	for op := range m.Ops {
		if !impl[op] && !want[op] {
			newlyMissing = append(newlyMissing, op)
		}
	}
	for op := range want {
		if impl[op] {
			nowImplemented = append(nowImplemented, op)
		}
	}
	sort.Strings(newlyMissing)
	sort.Strings(nowImplemented)
	if len(newlyMissing) > 0 {
		t.Errorf("AmazonS3: %d op(s) missing that the ratchet doesn't list — classify in s3ConformanceMissing: %v", len(newlyMissing), newlyMissing)
	}
	if len(nowImplemented) > 0 {
		t.Errorf("AmazonS3: %d op(s) now implemented — remove from s3ConformanceMissing: %v", len(nowImplemented), nowImplemented)
	}
}

// TestServiceConformance_S3Coverage reports S3's REST operation coverage.
func TestServiceConformance_S3Coverage(t *testing.T) {
	models := loadSmithyModels(t)
	m := serviceModel(t, models, "AmazonS3")
	impl := s3ImplementedOps()
	var missing []string
	for op := range m.Ops {
		if !impl[op] {
			missing = append(missing, op)
		}
	}
	sort.Strings(missing)
	t.Logf("AmazonS3: %d/%d operations implemented; missing (%d): %v",
		len(m.Ops)-len(missing), len(m.Ops), len(missing), missing)
}

// TestServiceConformance_Ratchet locks each catalogued service's set of
// not-yet-implemented operations. The recorded list IS the live non-conformity
// report: implement one → remove it here and the test passes with fewer gaps;
// a model update that adds an op fails until it's classified. This is what makes
// "is service X complete?" a measured, enforced number instead of a claim.
func TestServiceConformance_Ratchet(t *testing.T) {
	models := loadSmithyModels(t)
	_, jsonRouter, queryRouter := buildConformanceSimulator(t)
	jsonTargets := jsonRouter.Targets()
	versioned := queryRouter.VersionedActions()

	for shape, want := range serviceConformanceCatalog {
		m := serviceModel(t, models, shape)
		_, missing := serviceCoverage(m, jsonTargets, versioned)
		wantSet := map[string]bool{}
		for _, op := range want {
			wantSet[op] = true
		}
		gotSet := map[string]bool{}
		for _, op := range missing {
			gotSet[op] = true
		}
		var newlyMissing, nowImplemented []string
		for op := range gotSet {
			if !wantSet[op] {
				newlyMissing = append(newlyMissing, op)
			}
		}
		for op := range wantSet {
			if !gotSet[op] {
				nowImplemented = append(nowImplemented, op)
			}
		}
		sort.Strings(newlyMissing)
		sort.Strings(nowImplemented)
		if len(newlyMissing) > 0 {
			t.Errorf("%s: %d operation(s) missing that the ratchet doesn't list — classify them in serviceConformanceCatalog: %v", shape, len(newlyMissing), newlyMissing)
		}
		if len(nowImplemented) > 0 {
			t.Errorf("%s: %d operation(s) are now implemented — remove them from serviceConformanceCatalog so the gap count drops: %v", shape, len(nowImplemented), nowImplemented)
		}
	}
}
