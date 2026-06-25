package aws_sdk_test

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	cwltypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/aws/smithy-go/middleware"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// cwLogsStreamClient builds a CloudWatch Logs client whose StartLiveTail /
// GetLogObject calls reach the simulator's single host. Both ops carry the
// smithy `@endpoint(hostPrefix: "stream-")` trait, so the SDK would otherwise
// rewrite the host to `stream-<host>` — unreachable behind a custom endpoint.
// Disabling endpoint host-prefix injection is the same option a real consumer
// targeting a custom CloudWatch Logs endpoint (a VPC endpoint, LocalStack) uses.
func cwLogsStreamClient() *cloudwatchlogs.Client {
	return cloudwatchlogs.NewFromConfig(sdkConfig(), func(o *cloudwatchlogs.Options) {
		o.BaseEndpoint = aws.String(baseURL)
		o.APIOptions = append(o.APIOptions, func(st *middleware.Stack) error {
			return st.Initialize.Add(middleware.InitializeMiddlewareFunc("disableEndpointHostPrefix",
				func(ctx context.Context, in middleware.InitializeInput, next middleware.InitializeHandler) (middleware.InitializeOutput, middleware.Metadata, error) {
					return next.HandleInitialize(smithyhttp.DisableEndpointHostPrefix(ctx, true), in)
				}), middleware.Before)
		})
	})
}

// seedLogGroupWithEvents creates a log group + stream and ingests events,
// returning the group ARN. The streaming ops surface this stored history.
func seedLogGroupWithEvents(t *testing.T, cw *cloudwatchlogs.Client, group, stream string, messages []string) string {
	t.Helper()
	_, err := cw.CreateLogGroup(ctx, &cloudwatchlogs.CreateLogGroupInput{
		LogGroupName: aws.String(group),
	})
	require.NoError(t, err)
	_, err = cw.CreateLogStream(ctx, &cloudwatchlogs.CreateLogStreamInput{
		LogGroupName:  aws.String(group),
		LogStreamName: aws.String(stream),
	})
	require.NoError(t, err)

	now := time.Now().UnixMilli()
	events := make([]cwltypes.InputLogEvent, 0, len(messages))
	for i, m := range messages {
		events = append(events, cwltypes.InputLogEvent{
			Message:   aws.String(m),
			Timestamp: aws.Int64(now + int64(i)),
		})
	}
	_, err = cw.PutLogEvents(ctx, &cloudwatchlogs.PutLogEventsInput{
		LogGroupName:  aws.String(group),
		LogStreamName: aws.String(stream),
		LogEvents:     events,
	})
	require.NoError(t, err)

	desc, err := cw.DescribeLogGroups(ctx, &cloudwatchlogs.DescribeLogGroupsInput{
		LogGroupNamePrefix: aws.String(group),
	})
	require.NoError(t, err)
	require.NotEmpty(t, desc.LogGroups)
	return aws.ToString(desc.LogGroups[0].Arn)
}

// TestLogs_StartLiveTail opens a Live Tail session over the event stream and
// reassembles the sessionStart + sessionUpdate frames the handler emits,
// asserting the stored log events surface in the update.
func TestLogs_StartLiveTail(t *testing.T) {
	cw := cwLogsStreamClient()
	group := "livetail-group"
	stream := "livetail-stream"
	groupArn := seedLogGroupWithEvents(t, cw, group, stream, []string{
		"first live tail line",
		"second live tail line",
	})

	out, err := cw.StartLiveTail(ctx, &cloudwatchlogs.StartLiveTailInput{
		LogGroupIdentifiers: []string{groupArn},
	})
	require.NoError(t, err)

	es := out.GetStream()
	defer es.Close()

	sawStart := false
	sawUpdate := false
	var collected []string
	for ev := range es.Events() {
		switch v := ev.(type) {
		case *cwltypes.StartLiveTailResponseStreamMemberSessionStart:
			sawStart = true
			require.NotEmpty(t, aws.ToString(v.Value.SessionId))
			require.NotEmpty(t, aws.ToString(v.Value.RequestId))
			assert.Contains(t, v.Value.LogGroupIdentifiers, groupArn)
		case *cwltypes.StartLiveTailResponseStreamMemberSessionUpdate:
			sawUpdate = true
			for _, le := range v.Value.SessionResults {
				collected = append(collected, aws.ToString(le.Message))
				assert.Equal(t, groupArn, aws.ToString(le.LogGroupIdentifier))
				assert.Equal(t, stream, aws.ToString(le.LogStreamName))
			}
		}
	}
	require.NoError(t, es.Err())
	assert.True(t, sawStart, "stream must open with a sessionStart event")
	assert.True(t, sawUpdate, "stream must carry at least one sessionUpdate event")
	assert.ElementsMatch(t, []string{"first live tail line", "second live tail line"}, collected)
}

// TestLogs_StartLiveTail_UnknownGroup returns a ResourceNotFoundException
// before the stream opens for an unresolvable log group.
func TestLogs_StartLiveTail_UnknownGroup(t *testing.T) {
	cw := cwLogsStreamClient()
	_, err := cw.StartLiveTail(ctx, &cloudwatchlogs.StartLiveTailInput{
		LogGroupIdentifiers: []string{"arn:aws:logs:us-east-1:123456789012:log-group:no-such-group"},
	})
	require.Error(t, err)
}

// TestLogs_GetLogObject streams a stored log object back over the event stream
// and asserts the FieldsData carries the referenced event's bytes.
func TestLogs_GetLogObject(t *testing.T) {
	cw := cwLogsStreamClient()
	group := "getlogobject-group"
	stream := "getlogobject-stream"
	seedLogGroupWithEvents(t, cw, group, stream, []string{
		"object-line-0",
		"object-line-1",
	})

	// The pointer the sim honors is "<group>:<stream>:<index>".
	pointer := group + ":" + stream + ":1"
	out, err := cw.GetLogObject(ctx, &cloudwatchlogs.GetLogObjectInput{
		LogObjectPointer: aws.String(pointer),
	})
	require.NoError(t, err)

	es := out.GetStream()
	defer es.Close()

	var data []byte
	sawFields := false
	for ev := range es.Events() {
		if f, ok := ev.(*cwltypes.GetLogObjectResponseStreamMemberFields); ok {
			sawFields = true
			data = f.Value.Data
		}
	}
	require.NoError(t, es.Err())
	assert.True(t, sawFields, "stream must carry a fields event")
	assert.Equal(t, "object-line-1", string(data))
}
