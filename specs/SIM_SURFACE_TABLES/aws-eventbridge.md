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
| PutRule | ✓ `eventbridge.go::handleEBPutRule` | ✓ `sdk-tests/eventbridge_test.go::TestEventBridge_RuleTargetPutEventsSDK` | ✓ `cli-tests/eventbridge_test.go::TestEventBridgeCLI_RuleTargetPutEvents` | ✓ `aws_cloudwatch_event_rule` | Creates/updates rule state, pattern, schedule, tags. |
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
| ListTagsForResource | ✓ `eventbridge.go::handleEBListTagsForResource` | ✓ | n/a | ✓ Terraform read | Returns rule tags. |

## Closed bugs

- BUG-1197 — foundational AWS EventBridge slice added with SDK, CLI, and Terraform coverage.

## Open subtasks staged forward

- BUG-1213 / issue #249 tracks remaining EventBridge parity for event buses, bus policies, archives/replays, and other advanced resources exposed by official clients.
