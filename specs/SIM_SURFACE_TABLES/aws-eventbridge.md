# Sim surface — aws-eventbridge

Surface registered in `simulators/aws/eventbridge.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with a BUG / deferred-subtask reference; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no terraform-provider resource for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `Action AWSEvents.CreateEventBus` | ✓ `simulators/aws/eventbridge.go:109::handleEBCreateEventBus` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AWSEvents.DescribeEventBus` | ✓ `simulators/aws/eventbridge.go:110::handleEBDescribeEventBus` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AWSEvents.ListEventBuses` | ✓ `simulators/aws/eventbridge.go:111::handleEBListEventBuses` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AWSEvents.DeleteEventBus` | ✓ `simulators/aws/eventbridge.go:112::handleEBDeleteEventBus` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AWSEvents.PutPermission` | ✓ `simulators/aws/eventbridge.go:113::handleEBPutPermission` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AWSEvents.RemovePermission` | ✓ `simulators/aws/eventbridge.go:114::handleEBRemovePermission` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AWSEvents.PutRule` | ✓ `simulators/aws/eventbridge.go:115::handleEBPutRule` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AWSEvents.DescribeRule` | ✓ `simulators/aws/eventbridge.go:116::handleEBDescribeRule` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AWSEvents.ListRules` | ✓ `simulators/aws/eventbridge.go:117::handleEBListRules` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AWSEvents.DeleteRule` | ✓ `simulators/aws/eventbridge.go:118::handleEBDeleteRule` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AWSEvents.EnableRule` | ✓ `simulators/aws/eventbridge.go:119::handleEBEnableRule` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AWSEvents.DisableRule` | ✓ `simulators/aws/eventbridge.go:120::handleEBDisableRule` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AWSEvents.PutTargets` | ✓ `simulators/aws/eventbridge.go:121::handleEBPutTargets` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AWSEvents.ListTargetsByRule` | ✓ `simulators/aws/eventbridge.go:122::handleEBListTargetsByRule` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AWSEvents.RemoveTargets` | ✓ `simulators/aws/eventbridge.go:123::handleEBRemoveTargets` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AWSEvents.PutEvents` | ✓ `simulators/aws/eventbridge.go:124::handleEBPutEvents` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AWSEvents.TagResource` | ✓ `simulators/aws/eventbridge.go:125::handleEBTagResource` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AWSEvents.UntagResource` | ✓ `simulators/aws/eventbridge.go:126::handleEBUntagResource` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AWSEvents.ListTagsForResource` | ✓ `simulators/aws/eventbridge.go:127::handleEBListTagsForResource` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AWSEvents.CreateArchive` | ✓ `simulators/aws/eventbridge.go:128::handleEBCreateArchive` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AWSEvents.DescribeArchive` | ✓ `simulators/aws/eventbridge.go:129::handleEBDescribeArchive` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AWSEvents.ListArchives` | ✓ `simulators/aws/eventbridge.go:130::handleEBListArchives` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AWSEvents.DeleteArchive` | ✓ `simulators/aws/eventbridge.go:131::handleEBDeleteArchive` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AWSEvents.StartReplay` | ✓ `simulators/aws/eventbridge.go:132::handleEBStartReplay` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AWSEvents.DescribeReplay` | ✓ `simulators/aws/eventbridge.go:133::handleEBDescribeReplay` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AWSEvents.ListReplays` | ✓ `simulators/aws/eventbridge.go:134::handleEBListReplays` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |

## Open subtasks staged forward

- sdk-test / tf-test columns are ✗-with-deferral for every row above. Each subsequent surface-touching PR fills in the column for the rows it covers; remaining ✗s are tracked under BUG-1159 (paged-iterator sweep) + BUG-1147 (tf-test parity sweep).
- Missing ops (not in HandleFunc but documented by the cloud provider) get ✗ rows added when a community-filed issue surfaces them or a periodic audit lands a sweep.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
