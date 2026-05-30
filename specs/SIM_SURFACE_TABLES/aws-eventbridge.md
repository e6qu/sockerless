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
| `Action AWSEvents.CreateEventBus` | ✓ `simulators/aws/eventbridge.go:109::handleEBCreateEventBus` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.DescribeEventBus` | ✓ `simulators/aws/eventbridge.go:110::handleEBDescribeEventBus` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.ListEventBuses` | ✓ `simulators/aws/eventbridge.go:111::handleEBListEventBuses` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.DeleteEventBus` | ✓ `simulators/aws/eventbridge.go:112::handleEBDeleteEventBus` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.PutPermission` | ✓ `simulators/aws/eventbridge.go:113::handleEBPutPermission` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.RemovePermission` | ✓ `simulators/aws/eventbridge.go:114::handleEBRemovePermission` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.PutRule` | ✓ `simulators/aws/eventbridge.go:115::handleEBPutRule` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.DescribeRule` | ✓ `simulators/aws/eventbridge.go:116::handleEBDescribeRule` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.ListRules` | ✓ `simulators/aws/eventbridge.go:117::handleEBListRules` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.DeleteRule` | ✓ `simulators/aws/eventbridge.go:118::handleEBDeleteRule` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.EnableRule` | ✓ `simulators/aws/eventbridge.go:119::handleEBEnableRule` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.DisableRule` | ✓ `simulators/aws/eventbridge.go:120::handleEBDisableRule` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.PutTargets` | ✓ `simulators/aws/eventbridge.go:121::handleEBPutTargets` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.ListTargetsByRule` | ✓ `simulators/aws/eventbridge.go:122::handleEBListTargetsByRule` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.RemoveTargets` | ✓ `simulators/aws/eventbridge.go:123::handleEBRemoveTargets` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.PutEvents` | ✓ `simulators/aws/eventbridge.go:124::handleEBPutEvents` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.TagResource` | ✓ `simulators/aws/eventbridge.go:125::handleEBTagResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.UntagResource` | ✓ `simulators/aws/eventbridge.go:126::handleEBUntagResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.ListTagsForResource` | ✓ `simulators/aws/eventbridge.go:127::handleEBListTagsForResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.CreateArchive` | ✓ `simulators/aws/eventbridge.go:128::handleEBCreateArchive` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.DescribeArchive` | ✓ `simulators/aws/eventbridge.go:129::handleEBDescribeArchive` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.ListArchives` | ✓ `simulators/aws/eventbridge.go:130::handleEBListArchives` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.DeleteArchive` | ✓ `simulators/aws/eventbridge.go:131::handleEBDeleteArchive` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.StartReplay` | ✓ `simulators/aws/eventbridge.go:132::handleEBStartReplay` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.DescribeReplay` | ✓ `simulators/aws/eventbridge.go:133::handleEBDescribeReplay` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSEvents.ListReplays` | ✓ `simulators/aws/eventbridge.go:134::handleEBListReplays` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
