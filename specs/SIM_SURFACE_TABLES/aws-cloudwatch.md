# Sim surface — aws-cloudwatch

Surface registered in `simulators/aws/cloudwatch.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with a BUG / deferred-subtask reference; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no terraform-provider resource for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `Action Logs_20140328.CreateLogGroup` | ✓ `simulators/aws/cloudwatch.go:69::handleCWCreateLogGroup` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action Logs_20140328.DescribeLogGroups` | ✓ `simulators/aws/cloudwatch.go:70::handleCWDescribeLogGroups` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action Logs_20140328.DeleteLogGroup` | ✓ `simulators/aws/cloudwatch.go:71::handleCWDeleteLogGroup` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action Logs_20140328.CreateLogStream` | ✓ `simulators/aws/cloudwatch.go:72::handleCWCreateLogStream` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action Logs_20140328.DescribeLogStreams` | ✓ `simulators/aws/cloudwatch.go:73::handleCWDescribeLogStreams` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action Logs_20140328.PutLogEvents` | ✓ `simulators/aws/cloudwatch.go:74::handleCWPutLogEvents` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action Logs_20140328.GetLogEvents` | ✓ `simulators/aws/cloudwatch.go:75::handleCWGetLogEvents` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action Logs_20140328.FilterLogEvents` | ✓ `simulators/aws/cloudwatch.go:76::handleCWFilterLogEvents` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action Logs_20140328.PutRetentionPolicy` | ✓ `simulators/aws/cloudwatch.go:77::handleCWPutRetentionPolicy` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action Logs_20140328.ListTagsForResource` | ✓ `simulators/aws/cloudwatch.go:78::handleCWListTagsForResource` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action Logs_20140328.TagResource` | ✓ `simulators/aws/cloudwatch.go:79::handleCWTagResource` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |

## Open subtasks staged forward

- sdk-test / tf-test columns are ✗-with-deferral for every row above. Each subsequent surface-touching PR fills in the column for the rows it covers; remaining ✗s are tracked under BUG-1159 (paged-iterator sweep) + BUG-1147 (tf-test parity sweep).
- Missing ops (not in HandleFunc but documented by the cloud provider) get ✗ rows added when a community-filed issue surfaces them or a periodic audit lands a sweep.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
