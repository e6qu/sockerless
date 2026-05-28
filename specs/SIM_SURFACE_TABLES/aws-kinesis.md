# Sim surface — aws-kinesis

Surface registered in `simulators/aws/kinesis.go` as AWS JSON protocol target `Kinesis_20131202.*`.

## Status legend

- ✓ — implemented + tested
- n/a — not a canonical client surface for this operation in the repo harness

## Implemented ops

| Operation | sim handler | sdk-test | cli-test | tf-test | notes |
|---|---|---|---|---|---|
| `CreateStream` | ✓ `handleKinesisCreateStream` | ✓ `TestKinesisSDK_StreamLifecycleAndRecords` | ✓ `TestKinesisCLI_StreamAndRecords` | ✓ `aws_kinesis_stream.tf_stream` | Creates active provisioned streams, shard ranges, tags, ARN, retention, encryption, and monitoring metadata. |
| `DeleteStream` | ✓ `handleKinesisDeleteStream` | ✓ `TestKinesisSDK_StreamLifecycleAndRecords` | ✓ `TestKinesisCLI_StreamAndRecords` | ✓ `aws_kinesis_stream.tf_stream` | Deletes stream records, iterators, and metadata. |
| `DescribeStream` | ✓ `handleKinesisDescribeStream` | ✓ `TestKinesisSDK_StreamLifecycleAndRecords` | ✓ `TestKinesisCLI_StreamAndRecords` | ✓ `aws_kinesis_stream.tf_stream` | Returns stream description with shards and status. |
| `DescribeStreamSummary` | ✓ `handleKinesisDescribeStreamSummary` | ✓ `TestKinesisSDK_StreamLifecycleAndRecords` | n/a | ✓ `aws_kinesis_stream.tf_stream` | Returns open shard count and summary metadata. |
| `ListStreams` | ✓ `handleKinesisListStreams` | ✓ `TestKinesisSDK_StreamLifecycleAndRecords` | n/a | n/a | Supports stream-name enumeration. |
| `ListShards` | ✓ `handleKinesisListShards` | ✓ `TestKinesisSDK_StreamLifecycleAndRecords` | n/a | n/a | Returns shard IDs and hash-key ranges. |
| `PutRecord` | ✓ `handleKinesisPutRecord` | ✓ `TestKinesisSDK_StreamLifecycleAndRecords` | ✓ `TestKinesisCLI_StreamAndRecords` | n/a | Stores real record payload bytes with partition-key shard routing and sequence numbers. |
| `PutRecords` | ✓ `handleKinesisPutRecords` | ✓ `TestKinesisSDK_StreamLifecycleAndRecords` | n/a | n/a | Stores each record and returns per-entry shard/sequence metadata. |
| `GetShardIterator` | ✓ `handleKinesisGetShardIterator` | ✓ `TestKinesisSDK_StreamLifecycleAndRecords` | ✓ `TestKinesisCLI_StreamAndRecords` | n/a | Supports iterator-based reads from shard positions. |
| `GetRecords` | ✓ `handleKinesisGetRecords` | ✓ `TestKinesisSDK_StreamLifecycleAndRecords` | ✓ `TestKinesisCLI_StreamAndRecords` | n/a | Returns stored record payloads and advances the iterator. |
| `AddTagsToStream` | ✓ `handleKinesisAddTagsToStream` | ✓ `TestKinesisSDK_StreamLifecycleAndRecords` | n/a | ✓ `aws_kinesis_stream.tf_stream` | Merges stream tags. |
| `RemoveTagsFromStream` | ✓ `handleKinesisRemoveTagsFromStream` | n/a | n/a | ✓ `aws_kinesis_stream.tf_stream` | Removes named stream tags during provider updates/destroy. |
| `ListTagsForStream` | ✓ `handleKinesisListTagsForStream` | ✓ `TestKinesisSDK_StreamLifecycleAndRecords` | n/a | ✓ `aws_kinesis_stream.tf_stream` | Returns stream tags. |
| `IncreaseStreamRetentionPeriod` | ✓ `handleKinesisIncreaseStreamRetentionPeriod` | n/a | n/a | ✓ `aws_kinesis_stream.tf_stream` | Updates retention-period metadata. |
| `DecreaseStreamRetentionPeriod` | ✓ `handleKinesisDecreaseStreamRetentionPeriod` | n/a | n/a | ✓ `aws_kinesis_stream.tf_stream` | Updates retention-period metadata. |
| `EnableEnhancedMonitoring` | ✓ `handleKinesisEnableEnhancedMonitoring` | n/a | n/a | n/a | Persists requested shard-level metrics. |
| `DisableEnhancedMonitoring` | ✓ `handleKinesisDisableEnhancedMonitoring` | n/a | n/a | n/a | Removes requested shard-level metrics. |
| `StartStreamEncryption` | ✓ `handleKinesisStartStreamEncryption` | n/a | n/a | n/a | Persists encryption type and key ID. |
| `StopStreamEncryption` | ✓ `handleKinesisStopStreamEncryption` | n/a | n/a | n/a | Clears stream encryption metadata. |
| `UpdateShardCount` | ✓ `handleKinesisUpdateShardCount` | n/a | n/a | n/a | Rebuilds shard hash ranges for the requested shard count. |
| `DescribeLimits` | ✓ `handleKinesisDescribeLimits` | n/a | n/a | n/a | Returns account-level stream and shard limits. |

## Follow-up audit note

This table records the foundational Kinesis slice added for stream-ingestion parity. Future Kinesis expansions should add rows before implementation for any newly required public operations and cover them through the official SDK, AWS CLI, and Terraform provider when those client surfaces expose the operation.
