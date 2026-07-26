package aws_sdk_test

import (
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sfn"
	sfntypes "github.com/aws/aws-sdk-go-v2/service/sfn/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSFN_Activities_SDK covers CreateActivity / DescribeActivity /
// ListActivities / DeleteActivity and the GetActivityTask + SendTask*
// task-token lifecycle.
func TestSFN_Activities_SDK(t *testing.T) {
	c := sfnClient()

	create, err := c.CreateActivity(ctx, &sfn.CreateActivityInput{
		Name: aws.String("sfn-sdk-activity"),
	})
	require.NoError(t, err)
	require.NotEmpty(t, aws.ToString(create.ActivityArn))
	t.Cleanup(func() {
		_, _ = c.DeleteActivity(ctx, &sfn.DeleteActivityInput{ActivityArn: create.ActivityArn})
	})

	desc, err := c.DescribeActivity(ctx, &sfn.DescribeActivityInput{ActivityArn: create.ActivityArn})
	require.NoError(t, err)
	assert.Equal(t, "sfn-sdk-activity", aws.ToString(desc.Name))
	assert.Equal(t, aws.ToString(create.ActivityArn), aws.ToString(desc.ActivityArn))

	list, err := c.ListActivities(ctx, &sfn.ListActivitiesInput{})
	require.NoError(t, err)
	found := false
	for _, a := range list.Activities {
		if aws.ToString(a.Name) == "sfn-sdk-activity" {
			found = true
		}
	}
	assert.True(t, found)

	// No work scheduled yet — GetActivityTask returns an empty token.
	task, err := c.GetActivityTask(ctx, &sfn.GetActivityTaskInput{
		ActivityArn: create.ActivityArn,
		WorkerName:  aws.String("worker-1"),
	})
	require.NoError(t, err)
	assert.Empty(t, aws.ToString(task.TaskToken))

	// SendTaskSuccess/Failure/Heartbeat against an unknown token raise
	// TaskDoesNotExist — exercises the full SendTask* surface.
	_, err = c.SendTaskHeartbeat(ctx, &sfn.SendTaskHeartbeatInput{TaskToken: aws.String("nope")})
	require.Error(t, err)
	_, err = c.SendTaskSuccess(ctx, &sfn.SendTaskSuccessInput{TaskToken: aws.String("nope"), Output: aws.String("{}")})
	require.Error(t, err)
	_, err = c.SendTaskFailure(ctx, &sfn.SendTaskFailureInput{TaskToken: aws.String("nope"), Error: aws.String("E")})
	require.Error(t, err)
}

// TestSFN_VersionsAndAliases_SDK covers PublishStateMachineVersion,
// CreateStateMachineAlias, DescribeStateMachineAlias,
// ListStateMachineAliases, UpdateStateMachineAlias,
// DeleteStateMachineAlias, DeleteStateMachineVersion.
func TestSFN_VersionsAndAliases_SDK(t *testing.T) {
	c := sfnClient()

	sm, err := c.CreateStateMachine(ctx, &sfn.CreateStateMachineInput{
		Name:       aws.String("sfn-sdk-ver-sm"),
		Definition: aws.String(`{"StartAt":"P","States":{"P":{"Type":"Pass","End":true}}}`),
		RoleArn:    aws.String("arn:aws:iam::123456789012:role/sfn-role"),
		Publish:    true,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = c.DeleteStateMachine(ctx, &sfn.DeleteStateMachineInput{StateMachineArn: sm.StateMachineArn})
	})

	pub, err := c.PublishStateMachineVersion(ctx, &sfn.PublishStateMachineVersionInput{
		StateMachineArn: sm.StateMachineArn,
		Description:     aws.String("v1"),
	})
	require.NoError(t, err)
	require.NotEmpty(t, aws.ToString(pub.StateMachineVersionArn))

	alias, err := c.CreateStateMachineAlias(ctx, &sfn.CreateStateMachineAliasInput{
		Name:        aws.String("PROD"),
		Description: aws.String("prod alias"),
		RoutingConfiguration: []sfntypes.RoutingConfigurationListItem{
			{StateMachineVersionArn: pub.StateMachineVersionArn, Weight: 100},
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, aws.ToString(alias.StateMachineAliasArn))
	t.Cleanup(func() {
		_, _ = c.DeleteStateMachineAlias(ctx, &sfn.DeleteStateMachineAliasInput{StateMachineAliasArn: alias.StateMachineAliasArn})
	})

	da, err := c.DescribeStateMachineAlias(ctx, &sfn.DescribeStateMachineAliasInput{
		StateMachineAliasArn: alias.StateMachineAliasArn,
	})
	require.NoError(t, err)
	assert.Equal(t, "PROD", aws.ToString(da.Name))
	require.Len(t, da.RoutingConfiguration, 1)
	assert.Equal(t, aws.ToString(pub.StateMachineVersionArn), aws.ToString(da.RoutingConfiguration[0].StateMachineVersionArn))
	assert.Equal(t, int32(100), da.RoutingConfiguration[0].Weight)

	la, err := c.ListStateMachineAliases(ctx, &sfn.ListStateMachineAliasesInput{
		StateMachineArn: sm.StateMachineArn,
	})
	require.NoError(t, err)
	require.Len(t, la.StateMachineAliases, 1)
	assert.Equal(t, aws.ToString(alias.StateMachineAliasArn), aws.ToString(la.StateMachineAliases[0].StateMachineAliasArn))

	_, err = c.UpdateStateMachineAlias(ctx, &sfn.UpdateStateMachineAliasInput{
		StateMachineAliasArn: alias.StateMachineAliasArn,
		Description:          aws.String("updated"),
	})
	require.NoError(t, err)
	da, err = c.DescribeStateMachineAlias(ctx, &sfn.DescribeStateMachineAliasInput{
		StateMachineAliasArn: alias.StateMachineAliasArn,
	})
	require.NoError(t, err)
	assert.Equal(t, "updated", aws.ToString(da.Description))

	_, err = c.DeleteStateMachineVersion(ctx, &sfn.DeleteStateMachineVersionInput{
		StateMachineVersionArn: pub.StateMachineVersionArn,
	})
	require.NoError(t, err)
}

// TestSFN_TestState_SDK runs a single Pass state synchronously and asserts the
// real evaluated output.
func TestSFN_TestState_SDK(t *testing.T) {
	c := sfnClient()

	out, err := c.TestState(ctx, &sfn.TestStateInput{
		Definition: aws.String(`{"Type":"Pass","Result":{"hello":"world"},"End":true}`),
		Input:      aws.String(`{"x":1}`),
		RoleArn:    aws.String("arn:aws:iam::123456789012:role/sfn-role"),
	})
	require.NoError(t, err)
	assert.Equal(t, sfntypes.TestExecutionStatusSucceeded, out.Status)
	assert.JSONEq(t, `{"hello":"world"}`, aws.ToString(out.Output))
}

// TestSFN_StartSyncExecution_SDK runs a whole EXPRESS-shaped state machine
// synchronously and returns the terminal result inline.
func TestSFN_StartSyncExecution_SDK(t *testing.T) {
	c := sfnClient()

	sm, err := c.CreateStateMachine(ctx, &sfn.CreateStateMachineInput{
		Name:       aws.String("sfn-sdk-sync-sm"),
		Definition: aws.String(`{"StartAt":"P","States":{"P":{"Type":"Pass","Result":{"ok":true},"End":true}}}`),
		RoleArn:    aws.String("arn:aws:iam::123456789012:role/sfn-role"),
		Type:       sfntypes.StateMachineTypeExpress,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = c.DeleteStateMachine(ctx, &sfn.DeleteStateMachineInput{StateMachineArn: sm.StateMachineArn})
	})

	// StartSyncExecution carries the modeled `sync-` endpoint host prefix, so
	// the request the SDK signs and sends addresses sync-states.<endpoint> —
	// the same host shape real AWS serves it on.
	var sent capturedRequest
	out, err := c.StartSyncExecution(ctx, &sfn.StartSyncExecutionInput{
		StateMachineArn: sm.StateMachineArn,
		Input:           aws.String(`{}`),
	}, func(o *sfn.Options) {
		o.APIOptions = append(o.APIOptions, captureSignedRequest(&sent))
	})
	require.NoError(t, err)
	assert.Equal(t, sfntypes.SyncExecutionStatusSucceeded, out.Status)
	assert.JSONEq(t, `{"ok":true}`, aws.ToString(out.Output))
	require.NotEmpty(t, aws.ToString(out.ExecutionArn))
	assert.Equal(t, fmt.Sprintf("sync-states.localhost:%d", simPort), sent.host)
	assert.Contains(t, sent.signedHeaders(), "host")
}

// TestSFN_DescribeStateMachineForExecution_SDK + RedriveExecution.
func TestSFN_DescribeForExecutionAndRedrive_SDK(t *testing.T) {
	c := sfnClient()

	sm, err := c.CreateStateMachine(ctx, &sfn.CreateStateMachineInput{
		Name:       aws.String("sfn-sdk-redrive-sm"),
		Definition: aws.String(`{"StartAt":"F","States":{"F":{"Type":"Fail","Error":"E","Cause":"boom"}}}`),
		RoleArn:    aws.String("arn:aws:iam::123456789012:role/sfn-role"),
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = c.DeleteStateMachine(ctx, &sfn.DeleteStateMachineInput{StateMachineArn: sm.StateMachineArn})
	})

	start, err := c.StartExecution(ctx, &sfn.StartExecutionInput{
		StateMachineArn: sm.StateMachineArn,
		Input:           aws.String(`{}`),
	})
	require.NoError(t, err)

	dfe, err := c.DescribeStateMachineForExecution(ctx, &sfn.DescribeStateMachineForExecutionInput{
		ExecutionArn: start.ExecutionArn,
	})
	require.NoError(t, err)
	assert.Equal(t, "sfn-sdk-redrive-sm", aws.ToString(dfe.Name))
	assert.Contains(t, aws.ToString(dfe.Definition), "Fail")

	// Wait until the execution reaches a terminal FAILED state, then redrive.
	require.Eventually(t, func() bool {
		d, err := c.DescribeExecution(ctx, &sfn.DescribeExecutionInput{ExecutionArn: start.ExecutionArn})
		return err == nil && d.Status == sfntypes.ExecutionStatusFailed
	}, 10e9, 1e8)

	rd, err := c.RedriveExecution(ctx, &sfn.RedriveExecutionInput{ExecutionArn: start.ExecutionArn})
	require.NoError(t, err)
	require.NotNil(t, rd.RedriveDate)
	assert.False(t, rd.RedriveDate.IsZero())
}

// TestSFN_MapRuns_SDK covers ListMapRuns (empty), and DescribeMapRun /
// UpdateMapRun against a Map Run seeded through the SDK is not directly
// creatable, so we assert the empty-list shape and the not-found errors.
func TestSFN_MapRuns_SDK(t *testing.T) {
	c := sfnClient()

	sm, err := c.CreateStateMachine(ctx, &sfn.CreateStateMachineInput{
		Name:       aws.String("sfn-sdk-maprun-sm"),
		Definition: aws.String(`{"StartAt":"P","States":{"P":{"Type":"Pass","End":true}}}`),
		RoleArn:    aws.String("arn:aws:iam::123456789012:role/sfn-role"),
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = c.DeleteStateMachine(ctx, &sfn.DeleteStateMachineInput{StateMachineArn: sm.StateMachineArn})
	})

	start, err := c.StartExecution(ctx, &sfn.StartExecutionInput{
		StateMachineArn: sm.StateMachineArn,
		Input:           aws.String(`{}`),
	})
	require.NoError(t, err)

	lmr, err := c.ListMapRuns(ctx, &sfn.ListMapRunsInput{ExecutionArn: start.ExecutionArn})
	require.NoError(t, err)
	assert.Empty(t, lmr.MapRuns)

	missingMapRun := aws.String("arn:aws:states:us-east-1:123456789012:mapRun:nope/x:1")
	_, err = c.DescribeMapRun(ctx, &sfn.DescribeMapRunInput{MapRunArn: missingMapRun})
	require.Error(t, err)

	// UpdateMapRun on a non-existent Map Run returns the faithful error (no
	// Distributed-Map execution has produced one).
	_, err = c.UpdateMapRun(ctx, &sfn.UpdateMapRunInput{
		MapRunArn:      missingMapRun,
		MaxConcurrency: aws.Int32(5),
	})
	require.Error(t, err)
}
