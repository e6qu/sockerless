package aws_sdk_test

import (
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	smithy "github.com/aws/smithy-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCloudWatchLogs_FilterAndQueryFailLoud is the CloudWatch arm of the #652
// "silent incompleteness" prevention work (BUG-2170): a malformed FilterLogEvents
// filterPattern raises InvalidParameterException, and a malformed StartQuery
// query string raises MalformedQueryException — never a silently empty result
// that a consumer reads as "no matching logs".
func TestCloudWatchLogs_FilterAndQueryFailLoud(t *testing.T) {
	cw := cwLogsClient()
	group := "/test/failloud"
	_, err := cw.CreateLogGroup(ctx, &cloudwatchlogs.CreateLogGroupInput{LogGroupName: aws.String(group)})
	require.NoError(t, err)
	defer cw.DeleteLogGroup(ctx, &cloudwatchlogs.DeleteLogGroupInput{LogGroupName: aws.String(group)})

	errCode := func(err error) string {
		var ae smithy.APIError
		if errors.As(err, &ae) {
			return ae.ErrorCode()
		}
		return ""
	}

	// Malformed structured filter pattern (comparison missing its value).
	_, err = cw.FilterLogEvents(ctx, &cloudwatchlogs.FilterLogEventsInput{
		LogGroupName:  aws.String(group),
		FilterPattern: aws.String(`{ $.level = }`),
	})
	require.Error(t, err, "a malformed filterPattern must fail, not match nothing")
	assert.Equal(t, "InvalidParameterException", errCode(err))

	// A well-formed pattern that matches nothing is NOT an error.
	_, err = cw.FilterLogEvents(ctx, &cloudwatchlogs.FilterLogEventsInput{
		LogGroupName:  aws.String(group),
		FilterPattern: aws.String(`{ $.level = "NOPE" }`),
	})
	assert.NoError(t, err)

	// Malformed Insights query (unbalanced parenthesis in the filter stage).
	_, err = cw.StartQuery(ctx, &cloudwatchlogs.StartQueryInput{
		LogGroupName: aws.String(group),
		QueryString:  aws.String(`fields @message | filter (level = "ERROR"`),
		StartTime:    aws.Int64(0),
		EndTime:      aws.Int64(1),
	})
	require.Error(t, err, "a malformed query string must fail, not run as empty")
	assert.Equal(t, "MalformedQueryException", errCode(err))
}
