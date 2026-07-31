# Sim surface — aws-eventbridge

Surface registered in `simulators/aws/eventbridge.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with an open BUG or issue; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no meaningful client/provider surface for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `Action AWSEvents.CreateEventBus` | ✓ `simulators/aws/eventbridge.go:124::handleEBCreateEventBus` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.DescribeEventBus` | ✓ `simulators/aws/eventbridge.go:125::handleEBDescribeEventBus` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.ListEventBuses` | ✓ `simulators/aws/eventbridge.go:126::handleEBListEventBuses` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.DeleteEventBus` | ✓ `simulators/aws/eventbridge.go:127::handleEBDeleteEventBus` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.PutPermission` | ✓ `simulators/aws/eventbridge.go:128::handleEBPutPermission` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.RemovePermission` | ✓ `simulators/aws/eventbridge.go:129::handleEBRemovePermission` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.PutRule` | ✓ `simulators/aws/eventbridge.go:130::handleEBPutRule` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.DescribeRule` | ✓ `simulators/aws/eventbridge.go:131::handleEBDescribeRule` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.ListRules` | ✓ `simulators/aws/eventbridge.go:132::handleEBListRules` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.ListRuleNamesByTarget` | ✓ `simulators/aws/eventbridge.go:133::handleEBListRuleNamesByTarget` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.TestEventPattern` | ✓ `simulators/aws/eventbridge.go:134::handleEBTestEventPattern` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.UpdateEventBus` | ✓ `simulators/aws/eventbridge.go:135::handleEBUpdateEventBus` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.DeleteRule` | ✓ `simulators/aws/eventbridge.go:136::handleEBDeleteRule` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.EnableRule` | ✓ `simulators/aws/eventbridge.go:137::handleEBEnableRule` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.DisableRule` | ✓ `simulators/aws/eventbridge.go:138::handleEBDisableRule` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.PutTargets` | ✓ `simulators/aws/eventbridge.go:139::handleEBPutTargets` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.ListTargetsByRule` | ✓ `simulators/aws/eventbridge.go:140::handleEBListTargetsByRule` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.RemoveTargets` | ✓ `simulators/aws/eventbridge.go:141::handleEBRemoveTargets` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.PutEvents` | ✓ `simulators/aws/eventbridge.go:142::handleEBPutEvents` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.TagResource` | ✓ `simulators/aws/eventbridge.go:143::handleEBTagResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.UntagResource` | ✓ `simulators/aws/eventbridge.go:144::handleEBUntagResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.ListTagsForResource` | ✓ `simulators/aws/eventbridge.go:145::handleEBListTagsForResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.CreateArchive` | ✓ `simulators/aws/eventbridge.go:146::handleEBCreateArchive` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.DescribeArchive` | ✓ `simulators/aws/eventbridge.go:147::handleEBDescribeArchive` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.ListArchives` | ✓ `simulators/aws/eventbridge.go:148::handleEBListArchives` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.DeleteArchive` | ✓ `simulators/aws/eventbridge.go:149::handleEBDeleteArchive` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.StartReplay` | ✓ `simulators/aws/eventbridge.go:150::handleEBStartReplay` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.DescribeReplay` | ✓ `simulators/aws/eventbridge.go:151::handleEBDescribeReplay` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.ListReplays` | ✓ `simulators/aws/eventbridge.go:152::handleEBListReplays` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.UpdateArchive` | ✓ `simulators/aws/eventbridge.go:153::handleEBUpdateArchive` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.CancelReplay` | ✓ `simulators/aws/eventbridge.go:154::handleEBCancelReplay` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.CreateApiDestination` | ✓ `simulators/aws/eventbridge_connectivity.go:102::handleEBCreateApiDestination` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.DescribeApiDestination` | ✓ `simulators/aws/eventbridge_connectivity.go:103::handleEBDescribeApiDestination` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.ListApiDestinations` | ✓ `simulators/aws/eventbridge_connectivity.go:104::handleEBListApiDestinations` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.UpdateApiDestination` | ✓ `simulators/aws/eventbridge_connectivity.go:105::handleEBUpdateApiDestination` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.DeleteApiDestination` | ✓ `simulators/aws/eventbridge_connectivity.go:106::handleEBDeleteApiDestination` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.CreateConnection` | ✓ `simulators/aws/eventbridge_connectivity.go:108::handleEBCreateConnection` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.DescribeConnection` | ✓ `simulators/aws/eventbridge_connectivity.go:109::handleEBDescribeConnection` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.ListConnections` | ✓ `simulators/aws/eventbridge_connectivity.go:110::handleEBListConnections` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.UpdateConnection` | ✓ `simulators/aws/eventbridge_connectivity.go:111::handleEBUpdateConnection` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.DeauthorizeConnection` | ✓ `simulators/aws/eventbridge_connectivity.go:112::handleEBDeauthorizeConnection` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.DeleteConnection` | ✓ `simulators/aws/eventbridge_connectivity.go:113::handleEBDeleteConnection` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.CreateEndpoint` | ✓ `simulators/aws/eventbridge_connectivity.go:115::handleEBCreateEndpoint` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.DescribeEndpoint` | ✓ `simulators/aws/eventbridge_connectivity.go:116::handleEBDescribeEndpoint` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.ListEndpoints` | ✓ `simulators/aws/eventbridge_connectivity.go:117::handleEBListEndpoints` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.UpdateEndpoint` | ✓ `simulators/aws/eventbridge_connectivity.go:118::handleEBUpdateEndpoint` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.DeleteEndpoint` | ✓ `simulators/aws/eventbridge_connectivity.go:119::handleEBDeleteEndpoint` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.CreatePartnerEventSource` | ✓ `simulators/aws/eventbridge_connectivity.go:121::handleEBCreatePartnerEventSource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.DescribePartnerEventSource` | ✓ `simulators/aws/eventbridge_connectivity.go:122::handleEBDescribePartnerEventSource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.ListPartnerEventSources` | ✓ `simulators/aws/eventbridge_connectivity.go:123::handleEBListPartnerEventSources` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.ListPartnerEventSourceAccounts` | ✓ `simulators/aws/eventbridge_connectivity.go:124::handleEBListPartnerEventSourceAccounts` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.DeletePartnerEventSource` | ✓ `simulators/aws/eventbridge_connectivity.go:125::handleEBDeletePartnerEventSource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.PutPartnerEvents` | ✓ `simulators/aws/eventbridge_connectivity.go:126::handleEBPutPartnerEvents` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.ActivateEventSource` | ✓ `simulators/aws/eventbridge_connectivity.go:128::handleEBActivateEventSource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.DeactivateEventSource` | ✓ `simulators/aws/eventbridge_connectivity.go:129::handleEBDeactivateEventSource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.DescribeEventSource` | ✓ `simulators/aws/eventbridge_connectivity.go:130::handleEBDescribeEventSource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.ListEventSources` | ✓ `simulators/aws/eventbridge_connectivity.go:131::handleEBListEventSources` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
