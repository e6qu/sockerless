package aws_sdk_test

import (
	"strings"
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

func TestLambda_InvokeExecutesCommand(t *testing.T) {
	lc := lambdaClient()

	fnName := "exec-test-fn"

	// Create an Image-type Lambda function with a command
	_, err := lc.CreateFunction(ctx, &lambda.CreateFunctionInput{
		FunctionName: aws.String(fnName),
		Role:         aws.String("arn:aws:iam::123456789012:role/test-role"),
		PackageType:  lambdatypes.PackageTypeImage,
		Code:         &lambdatypes.FunctionCode{ImageUri: aws.String("test:latest")},
		ImageConfig: &lambdatypes.ImageConfig{
			Command: []string{"echo", "hello"},
		},
	})
	require.NoError(t, err)
	defer lc.DeleteFunction(ctx, &lambda.DeleteFunctionInput{FunctionName: aws.String(fnName)})

	// Invoke the function
	invokeOut, err := lc.Invoke(ctx, &lambda.InvokeInput{
		FunctionName: aws.String(fnName),
	})
	require.NoError(t, err)

	// Response body should contain "hello"
	assert.Contains(t, string(invokeOut.Payload), "hello")
}

func TestLambda_InvokeNonZeroExit(t *testing.T) {
	lc := lambdaClient()
	cw := cwLogsClient()

	fnName := "exec-fail-fn"

	_, err := lc.CreateFunction(ctx, &lambda.CreateFunctionInput{
		FunctionName: aws.String(fnName),
		Role:         aws.String("arn:aws:iam::123456789012:role/test-role"),
		PackageType:  lambdatypes.PackageTypeImage,
		Code:         &lambdatypes.FunctionCode{ImageUri: aws.String("test:latest")},
		ImageConfig: &lambdatypes.ImageConfig{
			Command: []string{"sh", "-c", "exit 1"},
		},
	})
	require.NoError(t, err)
	defer lc.DeleteFunction(ctx, &lambda.DeleteFunctionInput{FunctionName: aws.String(fnName)})

	// Invoke the function
	_, err = lc.Invoke(ctx, &lambda.InvokeInput{
		FunctionName: aws.String(fnName),
	})
	require.NoError(t, err)

	// Verify CloudWatch logs contain error
	logGroupName := "/aws/lambda/" + fnName
	events, err := cw.FilterLogEvents(ctx, &cloudwatchlogs.FilterLogEventsInput{
		LogGroupName: aws.String(logGroupName),
	})
	require.NoError(t, err)
	require.NotEmpty(t, events.Events, "expected log events for failed invocation")

	var messages []string
	for _, e := range events.Events {
		messages = append(messages, *e.Message)
	}
	// Should have ERROR entry about exit code
	found := false
	for _, m := range messages {
		if strings.Contains(m, "ERROR") && strings.Contains(m, "exit") {
			found = true
		}
	}
	assert.True(t, found, "expected ERROR log entry about non-zero exit, got: %v", messages)

	// Cleanup
	cw.DeleteLogGroup(ctx, &cloudwatchlogs.DeleteLogGroupInput{LogGroupName: aws.String(logGroupName)})
}

func TestLambda_InvokeLogsToCloudWatch(t *testing.T) {
	lc := lambdaClient()
	cw := cwLogsClient()

	fnName := "exec-logs-fn"

	_, err := lc.CreateFunction(ctx, &lambda.CreateFunctionInput{
		FunctionName: aws.String(fnName),
		Role:         aws.String("arn:aws:iam::123456789012:role/test-role"),
		PackageType:  lambdatypes.PackageTypeImage,
		Code:         &lambdatypes.FunctionCode{ImageUri: aws.String("test:latest")},
		ImageConfig: &lambdatypes.ImageConfig{
			Command: []string{"echo", "real-lambda-output"},
		},
	})
	require.NoError(t, err)
	defer lc.DeleteFunction(ctx, &lambda.DeleteFunctionInput{FunctionName: aws.String(fnName)})

	// Invoke the function
	_, err = lc.Invoke(ctx, &lambda.InvokeInput{
		FunctionName: aws.String(fnName),
	})
	require.NoError(t, err)

	// Verify CloudWatch logs contain the real output
	logGroupName := "/aws/lambda/" + fnName
	events, err := cw.FilterLogEvents(ctx, &cloudwatchlogs.FilterLogEventsInput{
		LogGroupName: aws.String(logGroupName),
	})
	require.NoError(t, err)
	require.NotEmpty(t, events.Events, "expected log events")

	var messages []string
	for _, e := range events.Events {
		messages = append(messages, *e.Message)
	}

	// Should have START, real output, END, REPORT
	assert.Contains(t, messages, "real-lambda-output", "process stdout should appear in CloudWatch logs")

	hasStart := false
	hasEnd := false
	hasReport := false
	for _, m := range messages {
		if strings.Contains(m, "START RequestId:") {
			hasStart = true
		}
		if strings.Contains(m, "END RequestId:") {
			hasEnd = true
		}
		if strings.Contains(m, "REPORT RequestId:") {
			hasReport = true
		}
	}
	assert.True(t, hasStart, "should have START log entry")
	assert.True(t, hasEnd, "should have END log entry")
	assert.True(t, hasReport, "should have REPORT log entry")

	// Cleanup
	cw.DeleteLogGroup(ctx, &cloudwatchlogs.DeleteLogGroupInput{LogGroupName: aws.String(logGroupName)})
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
