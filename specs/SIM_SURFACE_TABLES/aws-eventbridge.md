# Sim surface — aws-eventbridge

Surface registered in `simulators/aws/eventbridge.go`. Rows below are the EventBridge operations implemented by the AWS simulator as the `AWSEvents.*` JSON protocol.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with a BUG / deferred-subtask reference; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no terraform-provider resource for this op

## Implemented ops

| Op | sim handler | sdk-test | cli-test | tf-test | notes |
|---|---|---|---|---|---|
| CreateEventBus | ✓ `eventbridge.go::handleEBCreateEventBus` | ✓ `sdk-tests/eventbridge_test.go::TestEventBridge_BusArchiveReplaySDK` | ✓ `cli-tests/eventbridge_test.go::TestEventBridgeCLI_BusArchiveReplay` | ✓ `aws_cloudwatch_event_bus` | Creates custom event buses with tags and ARN. |
| DescribeEventBus | ✓ `eventbridge.go::handleEBDescribeEventBus` | ✓ | ✓ | ✓ Terraform read | Returns default and custom bus metadata and policy. |
| ListEventBuses | ✓ `eventbridge.go::handleEBListEventBuses` | ✓ | ✓ | n/a | Lists default and custom buses. |
| DeleteEventBus | ✓ `eventbridge.go::handleEBDeleteEventBus` | ✓ cleanup | ✓ cleanup | ✓ destroy | Deletes custom event buses and scoped rules. |
| PutPermission | ✓ `eventbridge.go::handleEBPutPermission` | ✓ | ✓ | ✓ `aws_cloudwatch_event_permission` | Stores EventBridge bus policy statements. |
| RemovePermission | ✓ `eventbridge.go::handleEBRemovePermission` | ✓ | ✓ | ✓ destroy | Removes bus policy statements by statement ID. |
| PutRule | ✓ `eventbridge.go::handleEBPutRule` | ✓ `sdk-tests/eventbridge_test.go::TestEventBridge_RuleTargetPutEventsSDK`; ✓ `TestEventBridge_BusArchiveReplaySDK` | ✓ `cli-tests/eventbridge_test.go::TestEventBridgeCLI_RuleTargetPutEvents`; ✓ `TestEventBridgeCLI_BusArchiveReplay` | ✓ `aws_cloudwatch_event_rule` | Creates/updates rule state, pattern, schedule, tags, and custom-bus scope. |
| DescribeRule | ✓ `eventbridge.go::handleEBDescribeRule` | ✓ | ✓ via Terraform read | ✓ `aws_cloudwatch_event_rule` read | Returns EventBridge rule shape. |
| ListRules | ✓ `eventbridge.go::handleEBListRules` | ✓ | n/a | n/a | SDK coverage verifies prefix filtering. |
| DeleteRule | ✓ `eventbridge.go::handleEBDeleteRule` | ✓ cleanup | ✓ cleanup | ✓ destroy | Enforces target precondition unless forced. |
| EnableRule | ✓ `eventbridge.go::handleEBEnableRule` | n/a | n/a | n/a | Covered by shared state mutation path with DisableRule. |
| DisableRule | ✓ `eventbridge.go::handleEBDisableRule` | n/a | n/a | n/a | Covered by shared state mutation path with EnableRule. |
| PutTargets | ✓ `eventbridge.go::handleEBPutTargets` | ✓ | ✓ | ✓ `aws_cloudwatch_event_target` | Stores target IDs/ARNs for delivery. |
| ListTargetsByRule | ✓ `eventbridge.go::handleEBListTargetsByRule` | ✓ | ✓ | ✓ Terraform read | Returns rule targets. |
| RemoveTargets | ✓ `eventbridge.go::handleEBRemoveTargets` | ✓ cleanup | ✓ cleanup | ✓ destroy | Removes target IDs. |
| PutEvents | ✓ `eventbridge.go::handleEBPutEvents` | ✓ | ✓ | n/a | Records events and delivers matching events to SQS/SNS targets. |
| TagResource | ✓ `eventbridge.go::handleEBTagResource` | ✓ via PutRule/ListTags | n/a | ✓ rule tags | Persists rule tags. |
| UntagResource | ✓ `eventbridge.go::handleEBUntagResource` | n/a | n/a | ✓ tag diff/destroy | Removes rule tags. |
| ListTagsForResource | ✓ `eventbridge.go::handleEBListTagsForResource` | ✓ | n/a | ✓ Terraform read | Returns rule and event-bus tags. |
| CreateArchive | ✓ `eventbridge.go::handleEBCreateArchive` | ✓ `TestEventBridge_BusArchiveReplaySDK` | ✓ `TestEventBridgeCLI_BusArchiveReplay` | ✓ `aws_cloudwatch_event_archive` | Creates archives for event-bus source ARNs and optional event patterns. |
| DescribeArchive | ✓ `eventbridge.go::handleEBDescribeArchive` | ✓ | ✓ | ✓ Terraform read | Returns archive state, retention, and counters. |
| ListArchives | ✓ `eventbridge.go::handleEBListArchives` | ✓ | ✓ | n/a | Lists archives and supports source filtering. |
| DeleteArchive | ✓ `eventbridge.go::handleEBDeleteArchive` | ✓ cleanup | ✓ cleanup | ✓ destroy | Deletes archived event state. |
| StartReplay | ✓ `eventbridge.go::handleEBStartReplay` | ✓ | ✓ | n/a | Replays stored archive events through the target event bus. |
| DescribeReplay | ✓ `eventbridge.go::handleEBDescribeReplay` | ✓ | ✓ | n/a | Returns replay status and destination. |
| ListReplays | ✓ `eventbridge.go::handleEBListReplays` | ✓ | ✓ | n/a | Lists started replays by archive. |

## Closed bugs

- BUG-1197 — foundational AWS EventBridge slice added with SDK, CLI, and Terraform coverage.
- BUG-1213 / issue #249 — event buses, bus policies, archives, and replays added with SDK, CLI, and Terraform coverage where provider resources exist.

## Open subtasks staged forward

- No EventBridge subtasks remain from issue #249. Stream/event ingestion is tracked separately as BUG-1200 for Kinesis.
