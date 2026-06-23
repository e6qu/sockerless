package main

import (
	"sort"
	"strings"
	"testing"
)

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
// so their op-coverage needs a REST-route enumeration harness — a tracked
// follow-on, not yet measured here.
var serviceConformanceCatalog = map[string][]string{
	"AmazonSQS": {
		"CancelMessageMoveTask", "ListDeadLetterSourceQueues", "ListMessageMoveTasks",
		"StartMessageMoveTask",
	},
	"AmazonSimpleNotificationService": {
		"CheckIfPhoneNumberIsOptedOut", "CreatePlatformApplication", "CreatePlatformEndpoint",
		"CreateSMSSandboxPhoneNumber", "DeleteEndpoint", "DeletePlatformApplication",
		"DeleteSMSSandboxPhoneNumber", "GetDataProtectionPolicy", "GetEndpointAttributes",
		"GetPlatformApplicationAttributes", "GetSMSAttributes", "GetSMSSandboxAccountStatus",
		"ListEndpointsByPlatformApplication", "ListOriginationNumbers", "ListPhoneNumbersOptedOut",
		"ListPlatformApplications", "ListSMSSandboxPhoneNumbers", "OptInPhoneNumber",
		"PutDataProtectionPolicy", "SetEndpointAttributes", "SetPlatformApplicationAttributes",
		"SetSMSAttributes", "VerifySMSSandboxPhoneNumber",
	},
	"AWSEvents": {
		"ActivateEventSource", "CancelReplay", "CreateApiDestination", "CreateConnection",
		"CreateEndpoint", "CreatePartnerEventSource", "DeactivateEventSource", "DeauthorizeConnection",
		"DeleteApiDestination", "DeleteConnection", "DeleteEndpoint", "DeletePartnerEventSource",
		"DescribeApiDestination", "DescribeConnection", "DescribeEndpoint", "DescribeEventSource",
		"DescribePartnerEventSource", "ListApiDestinations", "ListConnections", "ListEndpoints",
		"ListEventSources", "ListPartnerEventSourceAccounts", "ListPartnerEventSources",
		"PutPartnerEvents", "UpdateApiDestination", "UpdateArchive", "UpdateConnection",
		"UpdateEndpoint",
	},
	"DynamoDB_20120810": {
		"CreateBackup", "CreateGlobalTable", "DeleteBackup", "DeleteResourcePolicy",
		"DescribeBackup", "DescribeContributorInsights", "DescribeEndpoints", "DescribeExport",
		"DescribeGlobalTable", "DescribeGlobalTableSettings", "DescribeImport",
		"DescribeKinesisStreamingDestination", "DescribeTableReplicaAutoScaling",
		"DisableKinesisStreamingDestination", "EnableKinesisStreamingDestination",
		"ExportTableToPointInTime", "GetResourcePolicy", "ImportTable", "ListBackups",
		"ListContributorInsights", "ListExports", "ListGlobalTables", "ListImports",
		"PutResourcePolicy", "RestoreTableFromBackup", "RestoreTableToPointInTime",
		"UpdateContributorInsights", "UpdateGlobalTable", "UpdateGlobalTableSettings",
		"UpdateKinesisStreamingDestination", "UpdateTableReplicaAutoScaling",
	},
	"AmazonEC2ContainerServiceV20141113": {
		"ContinueServiceDeployment", "CreateCapacityProvider", "CreateDaemon", "CreateTaskSet",
		"DeleteAccountSetting", "DeleteAttributes", "DeleteCapacityProvider", "DeleteDaemon",
		"DeleteDaemonTaskDefinition", "DeleteTaskDefinitions", "DeleteTaskSet",
		"DeregisterContainerInstance", "DescribeContainerInstances", "DescribeDaemon",
		"DescribeDaemonDeployments", "DescribeDaemonRevisions", "DescribeDaemonTaskDefinition",
		"DescribeServiceDeployments", "DescribeServiceRevisions", "DescribeTaskSets",
		"DiscoverPollEndpoint", "GetTaskProtection", "ListAccountSettings", "ListAttributes",
		"ListContainerInstances", "ListDaemonDeployments", "ListDaemonTaskDefinitions",
		"ListDaemons", "ListServiceDeployments", "ListServicesByNamespace", "PutAccountSetting",
		"PutAccountSettingDefault", "PutAttributes", "RegisterContainerInstance",
		"RegisterDaemonTaskDefinition", "StartTask", "StopServiceDeployment",
		"SubmitAttachmentStateChanges", "SubmitContainerStateChange", "SubmitTaskStateChange",
		"UpdateCapacityProvider", "UpdateContainerAgent", "UpdateContainerInstancesState",
		"UpdateDaemon", "UpdateServicePrimaryTaskSet", "UpdateTaskProtection", "UpdateTaskSet",
	},
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
	if m.Version != "" {
		for _, a := range versioned[m.Version] {
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
