package aws_sdk_test

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	cwlogtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCloudWatch_GetLogEventsPagination(t *testing.T) {
	cw := cwLogsClient()

	logGroup := "/test/pagination"
	streamName := "test-stream"

	// Create log group and stream
	_, err := cw.CreateLogGroup(ctx, &cloudwatchlogs.CreateLogGroupInput{
		LogGroupName: aws.String(logGroup),
	})
	require.NoError(t, err)
	defer cw.DeleteLogGroup(ctx, &cloudwatchlogs.DeleteLogGroupInput{
		LogGroupName: aws.String(logGroup),
	})

	_, err = cw.CreateLogStream(ctx, &cloudwatchlogs.CreateLogStreamInput{
		LogGroupName:  aws.String(logGroup),
		LogStreamName: aws.String(streamName),
	})
	require.NoError(t, err)

	now := time.Now().UnixMilli()

	// Put initial batch of events
	_, err = cw.PutLogEvents(ctx, &cloudwatchlogs.PutLogEventsInput{
		LogGroupName:  aws.String(logGroup),
		LogStreamName: aws.String(streamName),
		LogEvents: []cwlogtypes.InputLogEvent{
			{Timestamp: aws.Int64(now), Message: aws.String("event-1")},
			{Timestamp: aws.Int64(now + 1), Message: aws.String("event-2")},
			{Timestamp: aws.Int64(now + 2), Message: aws.String("event-3")},
		},
	})
	require.NoError(t, err)

	// Read all events
	result1, err := cw.GetLogEvents(ctx, &cloudwatchlogs.GetLogEventsInput{
		LogGroupName:  aws.String(logGroup),
		LogStreamName: aws.String(streamName),
		StartFromHead: aws.Bool(true),
	})
	require.NoError(t, err)
	require.Len(t, result1.Events, 3)
	assert.Equal(t, "event-1", *result1.Events[0].Message)
	assert.Equal(t, "event-3", *result1.Events[2].Message)

	token := result1.NextForwardToken
	require.NotNil(t, token)

	// Read again with same token — no new events, should return empty
	result2, err := cw.GetLogEvents(ctx, &cloudwatchlogs.GetLogEventsInput{
		LogGroupName:  aws.String(logGroup),
		LogStreamName: aws.String(streamName),
		StartFromHead: aws.Bool(true),
		NextToken:     token,
	})
	require.NoError(t, err)
	assert.Empty(t, result2.Events, "should return no events when no new data")
	assert.Equal(t, *token, *result2.NextForwardToken, "token should stay the same when no new events")

	// Put more events
	_, err = cw.PutLogEvents(ctx, &cloudwatchlogs.PutLogEventsInput{
		LogGroupName:  aws.String(logGroup),
		LogStreamName: aws.String(streamName),
		LogEvents: []cwlogtypes.InputLogEvent{
			{Timestamp: aws.Int64(now + 10), Message: aws.String("event-4")},
			{Timestamp: aws.Int64(now + 11), Message: aws.String("event-5")},
		},
	})
	require.NoError(t, err)

	// Read with the old token — should get only new events
	result3, err := cw.GetLogEvents(ctx, &cloudwatchlogs.GetLogEventsInput{
		LogGroupName:  aws.String(logGroup),
		LogStreamName: aws.String(streamName),
		StartFromHead: aws.Bool(true),
		NextToken:     token,
	})
	require.NoError(t, err)
	require.Len(t, result3.Events, 2, "should return only the 2 new events")
	assert.Equal(t, "event-4", *result3.Events[0].Message)
	assert.Equal(t, "event-5", *result3.Events[1].Message)

	// New token should be different
	assert.NotEqual(t, *token, *result3.NextForwardToken, "token should change when new events exist")
}

func TestCloudWatch_DescribeLogStreamsOrdering(t *testing.T) {
	cw := cwLogsClient()

	logGroup := "/test/stream-ordering"
	_, err := cw.CreateLogGroup(ctx, &cloudwatchlogs.CreateLogGroupInput{
		LogGroupName: aws.String(logGroup),
	})
	require.NoError(t, err)
	defer cw.DeleteLogGroup(ctx, &cloudwatchlogs.DeleteLogGroupInput{
		LogGroupName: aws.String(logGroup),
	})

	// Create 3 streams
	streams := []string{"stream-a", "stream-b", "stream-c"}
	for _, s := range streams {
		_, err := cw.CreateLogStream(ctx, &cloudwatchlogs.CreateLogStreamInput{
			LogGroupName:  aws.String(logGroup),
			LogStreamName: aws.String(s),
		})
		require.NoError(t, err)
	}

	now := time.Now().UnixMilli()

	// Put events at different times: stream-b gets the newest event
	_, err = cw.PutLogEvents(ctx, &cloudwatchlogs.PutLogEventsInput{
		LogGroupName:  aws.String(logGroup),
		LogStreamName: aws.String("stream-a"),
		LogEvents:     []cwlogtypes.InputLogEvent{{Timestamp: aws.Int64(now), Message: aws.String("old")}},
	})
	require.NoError(t, err)

	_, err = cw.PutLogEvents(ctx, &cloudwatchlogs.PutLogEventsInput{
		LogGroupName:  aws.String(logGroup),
		LogStreamName: aws.String("stream-c"),
		LogEvents:     []cwlogtypes.InputLogEvent{{Timestamp: aws.Int64(now + 100), Message: aws.String("medium")}},
	})
	require.NoError(t, err)

	_, err = cw.PutLogEvents(ctx, &cloudwatchlogs.PutLogEventsInput{
		LogGroupName:  aws.String(logGroup),
		LogStreamName: aws.String("stream-b"),
		LogEvents:     []cwlogtypes.InputLogEvent{{Timestamp: aws.Int64(now + 200), Message: aws.String("newest")}},
	})
	require.NoError(t, err)

	// Describe with OrderBy=LastEventTime, Descending=true
	result, err := cw.DescribeLogStreams(ctx, &cloudwatchlogs.DescribeLogStreamsInput{
		LogGroupName: aws.String(logGroup),
		OrderBy:      cwlogtypes.OrderByLastEventTime,
		Descending:   aws.Bool(true),
	})
	require.NoError(t, err)
	require.Len(t, result.LogStreams, 3)
	assert.Equal(t, "stream-b", *result.LogStreams[0].LogStreamName, "newest stream should be first")
	assert.Equal(t, "stream-c", *result.LogStreams[1].LogStreamName)
	assert.Equal(t, "stream-a", *result.LogStreams[2].LogStreamName, "oldest stream should be last")

	// With Limit=1, should return only the newest stream
	limited, err := cw.DescribeLogStreams(ctx, &cloudwatchlogs.DescribeLogStreamsInput{
		LogGroupName: aws.String(logGroup),
		OrderBy:      cwlogtypes.OrderByLastEventTime,
		Descending:   aws.Bool(true),
		Limit:        aws.Int32(1),
	})
	require.NoError(t, err)
	require.Len(t, limited.LogStreams, 1)
	assert.Equal(t, "stream-b", *limited.LogStreams[0].LogStreamName)

	// Default ordering (by name, ascending)
	byName, err := cw.DescribeLogStreams(ctx, &cloudwatchlogs.DescribeLogStreamsInput{
		LogGroupName: aws.String(logGroup),
	})
	require.NoError(t, err)
	require.Len(t, byName.LogStreams, 3)
	assert.Equal(t, "stream-a", *byName.LogStreams[0].LogStreamName)
	assert.Equal(t, "stream-b", *byName.LogStreams[1].LogStreamName)
	assert.Equal(t, "stream-c", *byName.LogStreams[2].LogStreamName)
}
