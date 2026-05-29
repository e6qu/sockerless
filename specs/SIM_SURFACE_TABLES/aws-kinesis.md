# Sim surface — aws-kinesis

Surface registered in `simulators/aws/kinesis.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with a BUG / deferred-subtask reference; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no terraform-provider resource for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `Action Kinesis_20131202.CreateStream` | ✓ `simulators/aws/kinesis.go:67::handleKinesisCreateStream` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action Kinesis_20131202.DeleteStream` | ✓ `simulators/aws/kinesis.go:68::handleKinesisDeleteStream` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action Kinesis_20131202.DescribeStream` | ✓ `simulators/aws/kinesis.go:69::handleKinesisDescribeStream` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action Kinesis_20131202.DescribeStreamSummary` | ✓ `simulators/aws/kinesis.go:70::handleKinesisDescribeStreamSummary` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action Kinesis_20131202.ListStreams` | ✓ `simulators/aws/kinesis.go:71::handleKinesisListStreams` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action Kinesis_20131202.ListShards` | ✓ `simulators/aws/kinesis.go:72::handleKinesisListShards` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action Kinesis_20131202.PutRecord` | ✓ `simulators/aws/kinesis.go:73::handleKinesisPutRecord` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action Kinesis_20131202.PutRecords` | ✓ `simulators/aws/kinesis.go:74::handleKinesisPutRecords` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action Kinesis_20131202.GetShardIterator` | ✓ `simulators/aws/kinesis.go:75::handleKinesisGetShardIterator` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action Kinesis_20131202.GetRecords` | ✓ `simulators/aws/kinesis.go:76::handleKinesisGetRecords` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action Kinesis_20131202.AddTagsToStream` | ✓ `simulators/aws/kinesis.go:77::handleKinesisAddTagsToStream` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action Kinesis_20131202.RemoveTagsFromStream` | ✓ `simulators/aws/kinesis.go:78::handleKinesisRemoveTagsFromStream` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action Kinesis_20131202.ListTagsForStream` | ✓ `simulators/aws/kinesis.go:79::handleKinesisListTagsForStream` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action Kinesis_20131202.IncreaseStreamRetentionPeriod` | ✓ `simulators/aws/kinesis.go:80::handleKinesisIncreaseStreamRetentionPeriod` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action Kinesis_20131202.DecreaseStreamRetentionPeriod` | ✓ `simulators/aws/kinesis.go:81::handleKinesisDecreaseStreamRetentionPeriod` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action Kinesis_20131202.EnableEnhancedMonitoring` | ✓ `simulators/aws/kinesis.go:82::handleKinesisEnableEnhancedMonitoring` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action Kinesis_20131202.DisableEnhancedMonitoring` | ✓ `simulators/aws/kinesis.go:83::handleKinesisDisableEnhancedMonitoring` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action Kinesis_20131202.StartStreamEncryption` | ✓ `simulators/aws/kinesis.go:84::handleKinesisStartStreamEncryption` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action Kinesis_20131202.StopStreamEncryption` | ✓ `simulators/aws/kinesis.go:85::handleKinesisStopStreamEncryption` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action Kinesis_20131202.UpdateShardCount` | ✓ `simulators/aws/kinesis.go:86::handleKinesisUpdateShardCount` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action Kinesis_20131202.DescribeLimits` | ✓ `simulators/aws/kinesis.go:87::handleKinesisDescribeLimits` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |

## Open subtasks staged forward

- sdk-test / tf-test columns are ✗-with-deferral for every row above. Each subsequent surface-touching PR fills in the column for the rows it covers; remaining ✗s are tracked under BUG-1159 (paged-iterator sweep) + BUG-1147 (tf-test parity sweep).
- Missing ops (not in HandleFunc but documented by the cloud provider) get ✗ rows added when a community-filed issue surfaces them or a periodic audit lands a sweep.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
