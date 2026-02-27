package aws_sdk_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	cwltypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	lambdatypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func lambdaClient() *lambda.Client {
	return lambda.NewFromConfig(sdkConfig(), func(o *lambda.Options) {
		o.BaseEndpoint = aws.String(baseURL)
	})
}

func cwLogsClient() *cloudwatchlogs.Client {
	return cloudwatchlogs.NewFromConfig(sdkConfig(), func(o *cloudwatchlogs.Options) {
		o.BaseEndpoint = aws.String(baseURL)
	})
}

func TestLambda_InvokeCreatesLogStream(t *testing.T) {
	lc := lambdaClient()
	cw := cwLogsClient()

	fnName := "test-log-inject-fn"

	// Create a Lambda function
	_, err := lc.CreateFunction(ctx, &lambda.CreateFunctionInput{
		FunctionName: aws.String(fnName),
		Role:         aws.String("arn:aws:iam::123456789012:role/test-role"),
		Runtime:      lambdatypes.RuntimeNodejs18x,
		Handler:      aws.String("index.handler"),
		Code:         &lambdatypes.FunctionCode{ZipFile: []byte("fake")},
	})
	require.NoError(t, err)
	defer lc.DeleteFunction(ctx, &lambda.DeleteFunctionInput{FunctionName: aws.String(fnName)})

	// Invoke the function
	_, err = lc.Invoke(ctx, &lambda.InvokeInput{
		FunctionName: aws.String(fnName),
	})
	require.NoError(t, err)

	// Verify log group was created
	logGroupName := "/aws/lambda/" + fnName
	groups, err := cw.DescribeLogGroups(ctx, &cloudwatchlogs.DescribeLogGroupsInput{
		LogGroupNamePrefix: aws.String(logGroupName),
	})
	require.NoError(t, err)
	require.NotEmpty(t, groups.LogGroups, "expected log group to be created")
	assert.Equal(t, logGroupName, *groups.LogGroups[0].LogGroupName)

	// Verify log stream was created (using OrderBy=LastEventTime, Descending=true, Limit=1)
	streams, err := cw.DescribeLogStreams(ctx, &cloudwatchlogs.DescribeLogStreamsInput{
		LogGroupName: aws.String(logGroupName),
		OrderBy:      cwltypes.OrderByLastEventTime,
		Descending:   aws.Bool(true),
		Limit:        aws.Int32(1),
	})
	require.NoError(t, err)
	require.Len(t, streams.LogStreams, 1, "expected exactly one log stream")

	streamName := *streams.LogStreams[0].LogStreamName
	assert.Contains(t, streamName, "[$LATEST]", "stream name should contain [$LATEST]")

	// Verify log events were injected
	events, err := cw.GetLogEvents(ctx, &cloudwatchlogs.GetLogEventsInput{
		LogGroupName:  aws.String(logGroupName),
		LogStreamName: aws.String(streamName),
		StartFromHead: aws.Bool(true),
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(events.Events), 3, "expected at least 3 log events (START, END, REPORT)")

	// Verify log event content
	assert.Contains(t, *events.Events[0].Message, "START RequestId:")
	assert.Contains(t, *events.Events[1].Message, "END RequestId:")
	assert.Contains(t, *events.Events[2].Message, "REPORT RequestId:")

	// Cleanup: delete the log group
	cw.DeleteLogGroup(ctx, &cloudwatchlogs.DeleteLogGroupInput{
		LogGroupName: aws.String(logGroupName),
	})
}

func TestLambda_MultipleInvokesCreateMultipleStreams(t *testing.T) {
	lc := lambdaClient()
	cw := cwLogsClient()

	fnName := "test-multi-invoke-fn"

	_, err := lc.CreateFunction(ctx, &lambda.CreateFunctionInput{
		FunctionName: aws.String(fnName),
		Role:         aws.String("arn:aws:iam::123456789012:role/test-role"),
		Runtime:      lambdatypes.RuntimeNodejs18x,
		Handler:      aws.String("index.handler"),
		Code:         &lambdatypes.FunctionCode{ZipFile: []byte("fake")},
	})
	require.NoError(t, err)
	defer lc.DeleteFunction(ctx, &lambda.DeleteFunctionInput{FunctionName: aws.String(fnName)})

	logGroupName := "/aws/lambda/" + fnName

	// Invoke twice
	_, err = lc.Invoke(ctx, &lambda.InvokeInput{FunctionName: aws.String(fnName)})
	require.NoError(t, err)
	_, err = lc.Invoke(ctx, &lambda.InvokeInput{FunctionName: aws.String(fnName)})
	require.NoError(t, err)

	// Should have 2 streams (each invoke creates a new one)
	streams, err := cw.DescribeLogStreams(ctx, &cloudwatchlogs.DescribeLogStreamsInput{
		LogGroupName: aws.String(logGroupName),
	})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(streams.LogStreams), 2, "expected at least 2 log streams")

	// With Descending=true + Limit=1, we should get only the most recent stream
	latest, err := cw.DescribeLogStreams(ctx, &cloudwatchlogs.DescribeLogStreamsInput{
		LogGroupName: aws.String(logGroupName),
		OrderBy:      cwltypes.OrderByLastEventTime,
		Descending:   aws.Bool(true),
		Limit:        aws.Int32(1),
	})
	require.NoError(t, err)
	require.Len(t, latest.LogStreams, 1)

	// Cleanup
	cw.DeleteLogGroup(ctx, &cloudwatchlogs.DeleteLogGroupInput{
		LogGroupName: aws.String(logGroupName),
	})
}
