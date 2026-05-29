# Sim surface — aws-sqs

Surface registered in `simulators/aws/sqs.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with a BUG / deferred-subtask reference; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no terraform-provider resource for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `Action AmazonSQS.CreateQueue` | ✓ `simulators/aws/sqs.go:85::handleSQSCreateQueue` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AmazonSQS.DeleteQueue` | ✓ `simulators/aws/sqs.go:86::handleSQSDeleteQueue` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AmazonSQS.GetQueueUrl` | ✓ `simulators/aws/sqs.go:87::handleSQSGetQueueURL` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AmazonSQS.ListQueues` | ✓ `simulators/aws/sqs.go:88::handleSQSListQueues` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AmazonSQS.GetQueueAttributes` | ✓ `simulators/aws/sqs.go:89::handleSQSGetQueueAttributes` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AmazonSQS.SetQueueAttributes` | ✓ `simulators/aws/sqs.go:90::handleSQSSetQueueAttributes` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AmazonSQS.SendMessage` | ✓ `simulators/aws/sqs.go:91::handleSQSSendMessage` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AmazonSQS.ReceiveMessage` | ✓ `simulators/aws/sqs.go:92::handleSQSReceiveMessage` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AmazonSQS.DeleteMessage` | ✓ `simulators/aws/sqs.go:93::handleSQSDeleteMessage` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AmazonSQS.TagQueue` | ✓ `simulators/aws/sqs.go:94::handleSQSTagQueue` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AmazonSQS.UntagQueue` | ✓ `simulators/aws/sqs.go:95::handleSQSUntagQueue` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AmazonSQS.ListQueueTags` | ✓ `simulators/aws/sqs.go:96::handleSQSListQueueTags` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AmazonSQS.PurgeQueue` | ✓ `simulators/aws/sqs.go:97::handleSQSPurgeQueue` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |

## Open subtasks staged forward

- sdk-test / tf-test columns are ✗-with-deferral for every row above. Each subsequent surface-touching PR fills in the column for the rows it covers; remaining ✗s are tracked under BUG-1159 (paged-iterator sweep) + BUG-1147 (tf-test parity sweep).
- Missing ops (not in HandleFunc but documented by the cloud provider) get ✗ rows added when a community-filed issue surfaces them or a periodic audit lands a sweep.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
