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
| `Action Kinesis_20131202.CreateStream` | ✓ `simulators/aws/kinesis.go:99::handleKinesisCreateStream` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Kinesis_20131202.DeleteStream` | ✓ `simulators/aws/kinesis.go:100::handleKinesisDeleteStream` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Kinesis_20131202.DescribeStream` | ✓ `simulators/aws/kinesis.go:101::handleKinesisDescribeStream` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Kinesis_20131202.DescribeStreamSummary` | ✓ `simulators/aws/kinesis.go:102::handleKinesisDescribeStreamSummary` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Kinesis_20131202.ListStreams` | ✓ `simulators/aws/kinesis.go:103::handleKinesisListStreams` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Kinesis_20131202.ListShards` | ✓ `simulators/aws/kinesis.go:104::handleKinesisListShards` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Kinesis_20131202.PutRecord` | ✓ `simulators/aws/kinesis.go:105::handleKinesisPutRecord` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Kinesis_20131202.PutRecords` | ✓ `simulators/aws/kinesis.go:106::handleKinesisPutRecords` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Kinesis_20131202.GetShardIterator` | ✓ `simulators/aws/kinesis.go:107::handleKinesisGetShardIterator` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Kinesis_20131202.GetRecords` | ✓ `simulators/aws/kinesis.go:108::handleKinesisGetRecords` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Kinesis_20131202.AddTagsToStream` | ✓ `simulators/aws/kinesis.go:109::handleKinesisAddTagsToStream` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Kinesis_20131202.RemoveTagsFromStream` | ✓ `simulators/aws/kinesis.go:110::handleKinesisRemoveTagsFromStream` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Kinesis_20131202.ListTagsForStream` | ✓ `simulators/aws/kinesis.go:111::handleKinesisListTagsForStream` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Kinesis_20131202.IncreaseStreamRetentionPeriod` | ✓ `simulators/aws/kinesis.go:112::handleKinesisIncreaseStreamRetentionPeriod` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Kinesis_20131202.DecreaseStreamRetentionPeriod` | ✓ `simulators/aws/kinesis.go:113::handleKinesisDecreaseStreamRetentionPeriod` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Kinesis_20131202.EnableEnhancedMonitoring` | ✓ `simulators/aws/kinesis.go:114::handleKinesisEnableEnhancedMonitoring` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Kinesis_20131202.DisableEnhancedMonitoring` | ✓ `simulators/aws/kinesis.go:115::handleKinesisDisableEnhancedMonitoring` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Kinesis_20131202.StartStreamEncryption` | ✓ `simulators/aws/kinesis.go:116::handleKinesisStartStreamEncryption` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Kinesis_20131202.StopStreamEncryption` | ✓ `simulators/aws/kinesis.go:117::handleKinesisStopStreamEncryption` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Kinesis_20131202.UpdateShardCount` | ✓ `simulators/aws/kinesis.go:118::handleKinesisUpdateShardCount` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Kinesis_20131202.DescribeLimits` | ✓ `simulators/aws/kinesis.go:119::handleKinesisDescribeLimits` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Kinesis_20131202.RegisterStreamConsumer` | ✓ `simulators/aws/kinesis.go:120::handleKinesisRegisterStreamConsumer` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Kinesis_20131202.DeregisterStreamConsumer` | ✓ `simulators/aws/kinesis.go:121::handleKinesisDeregisterStreamConsumer` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Kinesis_20131202.DescribeStreamConsumer` | ✓ `simulators/aws/kinesis.go:122::handleKinesisDescribeStreamConsumer` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Kinesis_20131202.ListStreamConsumers` | ✓ `simulators/aws/kinesis.go:123::handleKinesisListStreamConsumers` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Kinesis_20131202.PutResourcePolicy` | ✓ `simulators/aws/kinesis.go:124::handleKinesisPutResourcePolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Kinesis_20131202.GetResourcePolicy` | ✓ `simulators/aws/kinesis.go:125::handleKinesisGetResourcePolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Kinesis_20131202.DeleteResourcePolicy` | ✓ `simulators/aws/kinesis.go:126::handleKinesisDeleteResourcePolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Kinesis_20131202.MergeShards` | ✓ `simulators/aws/kinesis.go:127::handleKinesisMergeShards` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Kinesis_20131202.SplitShard` | ✓ `simulators/aws/kinesis.go:128::handleKinesisSplitShard` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Kinesis_20131202.TagResource` | ✓ `simulators/aws/kinesis.go:129::handleKinesisTagResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Kinesis_20131202.UntagResource` | ✓ `simulators/aws/kinesis.go:130::handleKinesisUntagResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Kinesis_20131202.ListTagsForResource` | ✓ `simulators/aws/kinesis.go:131::handleKinesisListTagsForResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Kinesis_20131202.UpdateStreamMode` | ✓ `simulators/aws/kinesis.go:132::handleKinesisUpdateStreamMode` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Kinesis_20131202.DescribeAccountSettings` | ✓ `simulators/aws/kinesis.go:133::handleKinesisDescribeAccountSettings` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Kinesis_20131202.UpdateAccountSettings` | ✓ `simulators/aws/kinesis.go:134::handleKinesisUpdateAccountSettings` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Kinesis_20131202.UpdateMaxRecordSize` | ✓ `simulators/aws/kinesis.go:135::handleKinesisUpdateMaxRecordSize` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Kinesis_20131202.UpdateStreamWarmThroughput` | ✓ `simulators/aws/kinesis.go:136::handleKinesisUpdateStreamWarmThroughput` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Kinesis_20131202.SubscribeToShard` | ✓ `simulators/aws/kinesis_streaming.go:21::handleKinesisSubscribeToShard` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
