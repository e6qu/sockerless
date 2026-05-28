package aws_sdk_test

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kinesis"
	ktypes "github.com/aws/aws-sdk-go-v2/service/kinesis/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func kinesisClient() *kinesis.Client {
	return kinesis.NewFromConfig(sdkConfig(), func(o *kinesis.Options) {
		o.BaseEndpoint = aws.String(baseURL)
	})
}

func TestKinesisSDK_StreamLifecycleAndRecords(t *testing.T) {
	client := kinesisClient()
	streamName := "sdk-kinesis-stream"

	_, err := client.CreateStream(ctx, &kinesis.CreateStreamInput{
		StreamName: aws.String(streamName),
		ShardCount: aws.Int32(2),
		Tags: map[string]string{
			"env": "sdk",
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = client.DeleteStream(ctx, &kinesis.DeleteStreamInput{
			StreamName: aws.String(streamName),
		})
	})

	desc, err := client.DescribeStream(ctx, &kinesis.DescribeStreamInput{
		StreamName: aws.String(streamName),
	})
	require.NoError(t, err)
	require.NotNil(t, desc.StreamDescription)
	assert.Equal(t, streamName, aws.ToString(desc.StreamDescription.StreamName))
	assert.Equal(t, ktypes.StreamStatusActive, desc.StreamDescription.StreamStatus)
	require.Len(t, desc.StreamDescription.Shards, 2)

	summary, err := client.DescribeStreamSummary(ctx, &kinesis.DescribeStreamSummaryInput{
		StreamName: aws.String(streamName),
	})
	require.NoError(t, err)
	require.NotNil(t, summary.StreamDescriptionSummary)
	assert.Equal(t, int32(2), aws.ToInt32(summary.StreamDescriptionSummary.OpenShardCount))

	list, err := client.ListStreams(ctx, &kinesis.ListStreamsInput{})
	require.NoError(t, err)
	assert.Contains(t, list.StreamNames, streamName)

	_, err = client.AddTagsToStream(ctx, &kinesis.AddTagsToStreamInput{
		StreamName: aws.String(streamName),
		Tags:       map[string]string{"team": "platform"},
	})
	require.NoError(t, err)
	tags, err := client.ListTagsForStream(ctx, &kinesis.ListTagsForStreamInput{
		StreamName: aws.String(streamName),
	})
	require.NoError(t, err)
	tagMap := map[string]string{}
	for _, tag := range tags.Tags {
		tagMap[aws.ToString(tag.Key)] = aws.ToString(tag.Value)
	}
	assert.Equal(t, map[string]string{"env": "sdk", "team": "platform"}, tagMap)

	_, err = client.RemoveTagsFromStream(ctx, &kinesis.RemoveTagsFromStreamInput{
		StreamName: aws.String(streamName),
		TagKeys:    []string{"team"},
	})
	require.NoError(t, err)

	_, err = client.IncreaseStreamRetentionPeriod(ctx, &kinesis.IncreaseStreamRetentionPeriodInput{
		StreamName:           aws.String(streamName),
		RetentionPeriodHours: aws.Int32(48),
	})
	require.NoError(t, err)
	_, err = client.DecreaseStreamRetentionPeriod(ctx, &kinesis.DecreaseStreamRetentionPeriodInput{
		StreamName:           aws.String(streamName),
		RetentionPeriodHours: aws.Int32(24),
	})
	require.NoError(t, err)

	enabled, err := client.EnableEnhancedMonitoring(ctx, &kinesis.EnableEnhancedMonitoringInput{
		StreamName:        aws.String(streamName),
		ShardLevelMetrics: []ktypes.MetricsName{ktypes.MetricsNameIncomingBytes},
	})
	require.NoError(t, err)
	assert.Contains(t, enabled.CurrentShardLevelMetrics, ktypes.MetricsNameIncomingBytes)
	disabled, err := client.DisableEnhancedMonitoring(ctx, &kinesis.DisableEnhancedMonitoringInput{
		StreamName:        aws.String(streamName),
		ShardLevelMetrics: []ktypes.MetricsName{ktypes.MetricsNameIncomingBytes},
	})
	require.NoError(t, err)
	assert.NotContains(t, disabled.CurrentShardLevelMetrics, ktypes.MetricsNameIncomingBytes)

	_, err = client.StartStreamEncryption(ctx, &kinesis.StartStreamEncryptionInput{
		StreamName:     aws.String(streamName),
		EncryptionType: ktypes.EncryptionTypeKms,
		KeyId:          aws.String("alias/aws/kinesis"),
	})
	require.NoError(t, err)
	_, err = client.StopStreamEncryption(ctx, &kinesis.StopStreamEncryptionInput{
		StreamName:     aws.String(streamName),
		EncryptionType: ktypes.EncryptionTypeKms,
		KeyId:          aws.String("alias/aws/kinesis"),
	})
	require.NoError(t, err)

	update, err := client.UpdateShardCount(ctx, &kinesis.UpdateShardCountInput{
		StreamName:       aws.String(streamName),
		TargetShardCount: aws.Int32(3),
		ScalingType:      ktypes.ScalingTypeUniformScaling,
	})
	require.NoError(t, err)
	assert.Equal(t, int32(3), aws.ToInt32(update.TargetShardCount))

	limits, err := client.DescribeLimits(ctx, &kinesis.DescribeLimitsInput{})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, aws.ToInt32(limits.OpenShardCount), int32(3))
	assert.Greater(t, aws.ToInt32(limits.ShardLimit), int32(0))

	put, err := client.PutRecord(ctx, &kinesis.PutRecordInput{
		StreamName:   aws.String(streamName),
		PartitionKey: aws.String("pk-1"),
		Data:         []byte("one"),
	})
	require.NoError(t, err)
	require.NotEmpty(t, aws.ToString(put.ShardId))
	require.NotEmpty(t, aws.ToString(put.SequenceNumber))

	putMany, err := client.PutRecords(ctx, &kinesis.PutRecordsInput{
		StreamName: aws.String(streamName),
		Records: []ktypes.PutRecordsRequestEntry{
			{PartitionKey: aws.String("pk-1"), Data: []byte("two")},
			{PartitionKey: aws.String("pk-2"), Data: []byte("three")},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, int32(0), aws.ToInt32(putMany.FailedRecordCount))
	require.Len(t, putMany.Records, 2)

	shards, err := client.ListShards(ctx, &kinesis.ListShardsInput{
		StreamName: aws.String(streamName),
	})
	require.NoError(t, err)
	require.NotEmpty(t, shards.Shards)
	bodies := map[string]bool{}
	require.Eventually(t, func() bool {
		for _, shard := range shards.Shards {
			iter, err := client.GetShardIterator(ctx, &kinesis.GetShardIteratorInput{
				StreamName:        aws.String(streamName),
				ShardId:           shard.ShardId,
				ShardIteratorType: ktypes.ShardIteratorTypeTrimHorizon,
			})
			require.NoError(t, err)
			require.NotEmpty(t, aws.ToString(iter.ShardIterator))
			out, err := client.GetRecords(ctx, &kinesis.GetRecordsInput{
				ShardIterator: iter.ShardIterator,
				Limit:         aws.Int32(10),
			})
			require.NoError(t, err)
			for _, record := range out.Records {
				bodies[string(record.Data)] = true
			}
		}
		return len(bodies) == 3
	}, 2*time.Second, 50*time.Millisecond)
	assert.Equal(t, map[string]bool{"one": true, "two": true, "three": true}, bodies)
}
