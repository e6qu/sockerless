# Sim surface — aws-kinesis

Surface registered in `simulators/aws/kinesis.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with an open BUG or issue; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no meaningful client/provider surface for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `Action Kinesis_20131202.CreateStream` | ✓ `simulators/aws/kinesis.go:67::handleKinesisCreateStream` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Kinesis_20131202.DeleteStream` | ✓ `simulators/aws/kinesis.go:68::handleKinesisDeleteStream` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Kinesis_20131202.DescribeStream` | ✓ `simulators/aws/kinesis.go:69::handleKinesisDescribeStream` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Kinesis_20131202.DescribeStreamSummary` | ✓ `simulators/aws/kinesis.go:70::handleKinesisDescribeStreamSummary` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Kinesis_20131202.ListStreams` | ✓ `simulators/aws/kinesis.go:71::handleKinesisListStreams` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Kinesis_20131202.ListShards` | ✓ `simulators/aws/kinesis.go:72::handleKinesisListShards` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Kinesis_20131202.PutRecord` | ✓ `simulators/aws/kinesis.go:73::handleKinesisPutRecord` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Kinesis_20131202.PutRecords` | ✓ `simulators/aws/kinesis.go:74::handleKinesisPutRecords` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Kinesis_20131202.GetShardIterator` | ✓ `simulators/aws/kinesis.go:75::handleKinesisGetShardIterator` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Kinesis_20131202.GetRecords` | ✓ `simulators/aws/kinesis.go:76::handleKinesisGetRecords` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Kinesis_20131202.AddTagsToStream` | ✓ `simulators/aws/kinesis.go:77::handleKinesisAddTagsToStream` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Kinesis_20131202.RemoveTagsFromStream` | ✓ `simulators/aws/kinesis.go:78::handleKinesisRemoveTagsFromStream` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Kinesis_20131202.ListTagsForStream` | ✓ `simulators/aws/kinesis.go:79::handleKinesisListTagsForStream` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Kinesis_20131202.IncreaseStreamRetentionPeriod` | ✓ `simulators/aws/kinesis.go:80::handleKinesisIncreaseStreamRetentionPeriod` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Kinesis_20131202.DecreaseStreamRetentionPeriod` | ✓ `simulators/aws/kinesis.go:81::handleKinesisDecreaseStreamRetentionPeriod` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Kinesis_20131202.EnableEnhancedMonitoring` | ✓ `simulators/aws/kinesis.go:82::handleKinesisEnableEnhancedMonitoring` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Kinesis_20131202.DisableEnhancedMonitoring` | ✓ `simulators/aws/kinesis.go:83::handleKinesisDisableEnhancedMonitoring` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Kinesis_20131202.StartStreamEncryption` | ✓ `simulators/aws/kinesis.go:84::handleKinesisStartStreamEncryption` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Kinesis_20131202.StopStreamEncryption` | ✓ `simulators/aws/kinesis.go:85::handleKinesisStopStreamEncryption` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Kinesis_20131202.UpdateShardCount` | ✓ `simulators/aws/kinesis.go:86::handleKinesisUpdateShardCount` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Kinesis_20131202.DescribeLimits` | ✓ `simulators/aws/kinesis.go:87::handleKinesisDescribeLimits` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
