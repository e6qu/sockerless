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

// TestLambda_InvokeRoundTrip exercises the Runtime API invoke path:
// the simulator implements the real AWS Lambda Runtime API slice.
// The handler image polls /next, echoes the payload back via
// /response. Round-trip proves the Runtime API slice is wired
// end-to-end (simulator-side per-invocation sidecar + container env +
// host.docker.internal + host gateway + /response propagation back
// to the SDK caller).
func TestLambda_InvokeRoundTrip(t *testing.T) {
	lc := lambdaClient()

	fnName := "roundtrip-fn"

	_, err := lc.CreateFunction(ctx, &lambda.CreateFunctionInput{
		FunctionName: aws.String(fnName),
		Role:         aws.String("arn:aws:iam::123456789012:role/test-role"),
		PackageType:  lambdatypes.PackageTypeImage,
		Code:         &lambdatypes.FunctionCode{ImageUri: aws.String(lambdaHandlerImageName)},
	})
	require.NoError(t, err)
	defer lc.DeleteFunction(ctx, &lambda.DeleteFunctionInput{FunctionName: aws.String(fnName)})

	payload := []byte(`{"ping":1,"msg":"hello"}`)
	invokeOut, err := lc.Invoke(ctx, &lambda.InvokeInput{
		FunctionName: aws.String(fnName),
		Payload:      payload,
	})
	require.NoError(t, err)
	assert.Nil(t, invokeOut.FunctionError, "unexpected function error: %v", aws.ToString(invokeOut.FunctionError))
	assert.JSONEq(t, string(payload), string(invokeOut.Payload),
		"handler should echo payload back via /response")
}

// TestLambda_InvokeHandlerError exercises the /error branch of the
// Runtime API: payload with "cause":"error" triggers a POST to
// /invocation/{id}/error; caller sees X-Amz-Function-Error: Unhandled.
func TestLambda_InvokeHandlerError(t *testing.T) {
	lc := lambdaClient()

	fnName := "error-fn"

	_, err := lc.CreateFunction(ctx, &lambda.CreateFunctionInput{
		FunctionName: aws.String(fnName),
		Role:         aws.String("arn:aws:iam::123456789012:role/test-role"),
		PackageType:  lambdatypes.PackageTypeImage,
		Code:         &lambdatypes.FunctionCode{ImageUri: aws.String(lambdaHandlerImageName)},
	})
	require.NoError(t, err)
	defer lc.DeleteFunction(ctx, &lambda.DeleteFunctionInput{FunctionName: aws.String(fnName)})

	invokeOut, err := lc.Invoke(ctx, &lambda.InvokeInput{
		FunctionName: aws.String(fnName),
		Payload:      []byte(`{"cause":"error"}`),
	})
	require.NoError(t, err)
	require.NotNil(t, invokeOut.FunctionError, "expected FunctionError set to 'Unhandled'")
	assert.Equal(t, "Unhandled", aws.ToString(invokeOut.FunctionError))
	assert.Contains(t, string(invokeOut.Payload), "errorMessage",
		"response body should be a Lambda error JSON")
	assert.Contains(t, string(invokeOut.Payload), "test error from handler")
}

// TestLambda_InvokeContainerCrash covers the case where the container
// exits before posting /response or /error (what real Lambda reports
// as "Runtime exited without providing a reason"). Alpine `sh -c "exit 1"`
// never talks the Runtime API; the simulator detects container exit
// without response and surfaces a proper Lambda error.
func TestLambda_InvokeContainerCrash(t *testing.T) {
	lc := lambdaClient()
	cw := cwLogsClient()

	fnName := "crash-fn"

	_, err := lc.CreateFunction(ctx, &lambda.CreateFunctionInput{
		FunctionName: aws.String(fnName),
		Role:         aws.String("arn:aws:iam::123456789012:role/test-role"),
		PackageType:  lambdatypes.PackageTypeImage,
		Code:         &lambdatypes.FunctionCode{ImageUri: aws.String("alpine:latest")},
		ImageConfig: &lambdatypes.ImageConfig{
			Command: []string{"sh", "-c", "exit 1"},
		},
	})
	require.NoError(t, err)
	defer lc.DeleteFunction(ctx, &lambda.DeleteFunctionInput{FunctionName: aws.String(fnName)})

	invokeOut, err := lc.Invoke(ctx, &lambda.InvokeInput{
		FunctionName: aws.String(fnName),
	})
	require.NoError(t, err)
	require.NotNil(t, invokeOut.FunctionError)
	assert.Equal(t, "Unhandled", aws.ToString(invokeOut.FunctionError))
	assert.Contains(t, string(invokeOut.Payload), "Runtime exited")

	// CloudWatch stream should have START/END/REPORT + an ERROR line.
	logGroupName := "/aws/lambda/" + fnName
	events, err := cw.FilterLogEvents(ctx, &cloudwatchlogs.FilterLogEventsInput{
		LogGroupName: aws.String(logGroupName),
	})
	require.NoError(t, err)
	require.NotEmpty(t, events.Events)
	var messages []string
	for _, e := range events.Events {
		messages = append(messages, *e.Message)
	}
	foundError := false
	for _, m := range messages {
		if strings.Contains(m, "ERROR") {
			foundError = true
		}
	}
	assert.True(t, foundError, "expected ERROR log entry for crashed container, got: %v", messages)
	cw.DeleteLogGroup(ctx, &cloudwatchlogs.DeleteLogGroupInput{LogGroupName: aws.String(logGroupName)})
}

// TestLambda_RuntimeAPILogsToCloudWatch verifies the simulator still
// injects START/END/REPORT log lines + captures the handler's stderr
// under the Runtime API path. The handler writes a log line to
// stderr describing the invocation; it should appear in the
// function's CloudWatch log stream.
func TestLambda_RuntimeAPILogsToCloudWatch(t *testing.T) {
	lc := lambdaClient()
	cw := cwLogsClient()

	fnName := "logs-fn"

	_, err := lc.CreateFunction(ctx, &lambda.CreateFunctionInput{
		FunctionName: aws.String(fnName),
		Role:         aws.String("arn:aws:iam::123456789012:role/test-role"),
		PackageType:  lambdatypes.PackageTypeImage,
		Code:         &lambdatypes.FunctionCode{ImageUri: aws.String(lambdaHandlerImageName)},
	})
	require.NoError(t, err)
	defer lc.DeleteFunction(ctx, &lambda.DeleteFunctionInput{FunctionName: aws.String(fnName)})

	_, err = lc.Invoke(ctx, &lambda.InvokeInput{
		FunctionName: aws.String(fnName),
		Payload:      []byte(`{"ping":1}`),
	})
	require.NoError(t, err)

	logGroupName := "/aws/lambda/" + fnName
	events, err := cw.FilterLogEvents(ctx, &cloudwatchlogs.FilterLogEventsInput{
		LogGroupName: aws.String(logGroupName),
	})
	require.NoError(t, err)
	require.NotEmpty(t, events.Events)

	var messages []string
	for _, e := range events.Events {
		messages = append(messages, *e.Message)
	}
	hasStart, hasEnd, hasReport, hasHandlerLog := false, false, false, false
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
		if strings.Contains(m, "lambda-runtime-handler: polling") {
			hasHandlerLog = true
		}
	}
	assert.True(t, hasStart, "should have START log entry")
	assert.True(t, hasEnd, "should have END log entry")
	assert.True(t, hasReport, "should have REPORT log entry")
	assert.True(t, hasHandlerLog, "should capture handler's stderr in CloudWatch: %v", messages)

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
