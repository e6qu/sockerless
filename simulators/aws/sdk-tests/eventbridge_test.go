package aws_sdk_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge"
	ebtypes "github.com/aws/aws-sdk-go-v2/service/eventbridge/types"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func eventbridgeClient() *eventbridge.Client {
	return eventbridge.NewFromConfig(sdkConfig(), func(o *eventbridge.Options) {
		o.BaseEndpoint = aws.String(baseURL)
	})
}

func TestEventBridge_BusArchiveReplaySDK(t *testing.T) {
	eb := eventbridgeClient()
	sqsC := sqsClient()

	busName := "eb-sdk-bus"
	createBus, err := eb.CreateEventBus(ctx, &eventbridge.CreateEventBusInput{
		Name:        aws.String(busName),
		Description: aws.String("sdk bus"),
		Tags:        []ebtypes.Tag{{Key: aws.String("env"), Value: aws.String("sdk")}},
	})
	require.NoError(t, err)
	require.NotEmpty(t, aws.ToString(createBus.EventBusArn))
	t.Cleanup(func() {
		_, _ = eb.DeleteArchive(ctx, &eventbridge.DeleteArchiveInput{ArchiveName: aws.String("eb-sdk-archive")})
		_, _ = eb.DeleteEventBus(ctx, &eventbridge.DeleteEventBusInput{Name: aws.String(busName)})
	})

	describeBus, err := eb.DescribeEventBus(ctx, &eventbridge.DescribeEventBusInput{Name: aws.String(busName)})
	require.NoError(t, err)
	assert.Equal(t, busName, aws.ToString(describeBus.Name))
	assert.Equal(t, aws.ToString(createBus.EventBusArn), aws.ToString(describeBus.Arn))

	buses, err := eb.ListEventBuses(ctx, &eventbridge.ListEventBusesInput{NamePrefix: aws.String("eb-sdk")})
	require.NoError(t, err)
	require.Len(t, buses.EventBuses, 1)
	assert.Equal(t, busName, aws.ToString(buses.EventBuses[0].Name))

	_, err = eb.PutPermission(ctx, &eventbridge.PutPermissionInput{
		EventBusName: aws.String(busName),
		StatementId:  aws.String("sdk-permission"),
		Action:       aws.String("events:PutEvents"),
		Principal:    aws.String("123456789012"),
	})
	require.NoError(t, err)
	describeBus, err = eb.DescribeEventBus(ctx, &eventbridge.DescribeEventBusInput{Name: aws.String(busName)})
	require.NoError(t, err)
	require.NotEmpty(t, aws.ToString(describeBus.Policy))
	var policy struct {
		Statement []struct {
			Sid string `json:"Sid"`
		} `json:"Statement"`
	}
	require.NoError(t, json.Unmarshal([]byte(aws.ToString(describeBus.Policy)), &policy))
	require.Len(t, policy.Statement, 1)
	assert.Equal(t, "sdk-permission", policy.Statement[0].Sid)

	_, err = eb.RemovePermission(ctx, &eventbridge.RemovePermissionInput{
		EventBusName: aws.String(busName),
		StatementId:  aws.String("sdk-permission"),
	})
	require.NoError(t, err)
	describeBus, err = eb.DescribeEventBus(ctx, &eventbridge.DescribeEventBusInput{Name: aws.String(busName)})
	require.NoError(t, err)
	assert.Empty(t, aws.ToString(describeBus.Policy))

	createArchive, err := eb.CreateArchive(ctx, &eventbridge.CreateArchiveInput{
		ArchiveName:    aws.String("eb-sdk-archive"),
		EventSourceArn: createBus.EventBusArn,
		Description:    aws.String("sdk archive"),
		EventPattern:   aws.String(`{"source":["sockerless.archive"]}`),
	})
	require.NoError(t, err)
	require.NotEmpty(t, aws.ToString(createArchive.ArchiveArn))
	assert.Equal(t, ebtypes.ArchiveStateEnabled, createArchive.State)

	archive, err := eb.DescribeArchive(ctx, &eventbridge.DescribeArchiveInput{ArchiveName: aws.String("eb-sdk-archive")})
	require.NoError(t, err)
	assert.Equal(t, "eb-sdk-archive", aws.ToString(archive.ArchiveName))
	assert.Equal(t, aws.ToString(createBus.EventBusArn), aws.ToString(archive.EventSourceArn))

	archives, err := eb.ListArchives(ctx, &eventbridge.ListArchivesInput{EventSourceArn: createBus.EventBusArn})
	require.NoError(t, err)
	require.Len(t, archives.Archives, 1)
	// List entries are the summary Archive shape — identity + state only
	// (ArchiveArn / Description / EventPattern ride DescribeArchive).
	assert.Equal(t, "eb-sdk-archive", aws.ToString(archives.Archives[0].ArchiveName))
	assert.Equal(t, aws.ToString(createBus.EventBusArn), aws.ToString(archives.Archives[0].EventSourceArn))
	assert.Equal(t, ebtypes.ArchiveStateEnabled, archives.Archives[0].State)
	assert.NotNil(t, archives.Archives[0].CreationTime)

	q, err := sqsC.CreateQueue(ctx, &sqs.CreateQueueInput{QueueName: aws.String("eb-sdk-replay-q")})
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = sqsC.DeleteQueue(ctx, &sqs.DeleteQueueInput{QueueUrl: q.QueueUrl}) })
	attrs, err := sqsC.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
		QueueUrl:       q.QueueUrl,
		AttributeNames: []sqstypes.QueueAttributeName{"QueueArn"},
	})
	require.NoError(t, err)
	queueARN := attrs.Attributes["QueueArn"]
	_, err = eb.PutRule(ctx, &eventbridge.PutRuleInput{
		Name:         aws.String("eb-sdk-replay-rule"),
		EventBusName: aws.String(busName),
		EventPattern: aws.String(`{"source":["sockerless.archive"]}`),
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = eb.RemoveTargets(ctx, &eventbridge.RemoveTargetsInput{
			EventBusName: aws.String(busName),
			Rule:         aws.String("eb-sdk-replay-rule"),
			Ids:          []string{"queue"},
		})
		_, _ = eb.DeleteRule(ctx, &eventbridge.DeleteRuleInput{
			EventBusName: aws.String(busName),
			Name:         aws.String("eb-sdk-replay-rule"),
		})
	})
	_, err = eb.PutTargets(ctx, &eventbridge.PutTargetsInput{
		EventBusName: aws.String(busName),
		Rule:         aws.String("eb-sdk-replay-rule"),
		Targets: []ebtypes.Target{{
			Id:  aws.String("queue"),
			Arn: aws.String(queueARN),
		}},
	})
	require.NoError(t, err)

	_, err = eb.PutEvents(ctx, &eventbridge.PutEventsInput{
		Entries: []ebtypes.PutEventsRequestEntry{{
			EventBusName: aws.String(busName),
			Source:       aws.String("sockerless.archive"),
			DetailType:   aws.String("example"),
			Detail:       aws.String(`{"replayed":true}`),
		}},
	})
	require.NoError(t, err)

	original, err := sqsC.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		QueueUrl:            q.QueueUrl,
		MaxNumberOfMessages: 1,
	})
	require.NoError(t, err)
	require.Len(t, original.Messages, 1)
	assert.JSONEq(t, `{"replayed":true}`, aws.ToString(original.Messages[0].Body))
	_, err = sqsC.DeleteMessage(ctx, &sqs.DeleteMessageInput{
		QueueUrl:      q.QueueUrl,
		ReceiptHandle: original.Messages[0].ReceiptHandle,
	})
	require.NoError(t, err)

	start := time.Now().Add(-time.Hour)
	end := time.Now().Add(time.Hour)
	replay, err := eb.StartReplay(ctx, &eventbridge.StartReplayInput{
		ReplayName:     aws.String("eb-sdk-replay"),
		EventSourceArn: createArchive.ArchiveArn,
		EventStartTime: aws.Time(start),
		EventEndTime:   aws.Time(end),
		Destination: &ebtypes.ReplayDestination{
			Arn: createBus.EventBusArn,
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, aws.ToString(replay.ReplayArn))

	describeReplay, err := eb.DescribeReplay(ctx, &eventbridge.DescribeReplayInput{ReplayName: aws.String("eb-sdk-replay")})
	require.NoError(t, err)
	assert.Equal(t, ebtypes.ReplayStateCompleted, describeReplay.State)

	replays, err := eb.ListReplays(ctx, &eventbridge.ListReplaysInput{EventSourceArn: createArchive.ArchiveArn})
	require.NoError(t, err)
	require.Len(t, replays.Replays, 1)
	// List entries are the summary Replay shape — identity + state +
	// timestamps only (ReplayArn / Description ride DescribeReplay).
	assert.Equal(t, "eb-sdk-replay", aws.ToString(replays.Replays[0].ReplayName))
	assert.Equal(t, aws.ToString(createArchive.ArchiveArn), aws.ToString(replays.Replays[0].EventSourceArn))
	assert.Equal(t, ebtypes.ReplayStateCompleted, replays.Replays[0].State)
	assert.NotNil(t, replays.Replays[0].ReplayStartTime)

	received, err := sqsC.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		QueueUrl:            q.QueueUrl,
		MaxNumberOfMessages: 1,
	})
	require.NoError(t, err)
	require.Len(t, received.Messages, 1)
	assert.JSONEq(t, `{"replayed":true}`, aws.ToString(received.Messages[0].Body))
}

func TestEventBridge_RuleTargetPutEventsSDK(t *testing.T) {
	eb := eventbridgeClient()
	sqsC := sqsClient()

	q, err := sqsC.CreateQueue(ctx, &sqs.CreateQueueInput{QueueName: aws.String("eb-sdk-q")})
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = sqsC.DeleteQueue(ctx, &sqs.DeleteQueueInput{QueueUrl: q.QueueUrl}) })

	attrs, err := sqsC.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
		QueueUrl:       q.QueueUrl,
		AttributeNames: []sqstypes.QueueAttributeName{"QueueArn"},
	})
	require.NoError(t, err)
	queueARN := attrs.Attributes["QueueArn"]
	require.NotEmpty(t, queueARN)

	pattern := `{"source":["sockerless.test"],"detail-type":["example"]}`
	putRule, err := eb.PutRule(ctx, &eventbridge.PutRuleInput{
		Name:         aws.String("eb-sdk-rule"),
		EventPattern: aws.String(pattern),
		Tags:         []ebtypes.Tag{{Key: aws.String("env"), Value: aws.String("test")}},
	})
	require.NoError(t, err)
	require.NotEmpty(t, aws.ToString(putRule.RuleArn))
	t.Cleanup(func() {
		_, _ = eb.RemoveTargets(ctx, &eventbridge.RemoveTargetsInput{
			Rule: aws.String("eb-sdk-rule"),
			Ids:  []string{"queue"},
		})
		_, _ = eb.DeleteRule(ctx, &eventbridge.DeleteRuleInput{Name: aws.String("eb-sdk-rule")})
	})

	describe, err := eb.DescribeRule(ctx, &eventbridge.DescribeRuleInput{Name: aws.String("eb-sdk-rule")})
	require.NoError(t, err)
	assert.Equal(t, "eb-sdk-rule", aws.ToString(describe.Name))
	assert.Equal(t, pattern, aws.ToString(describe.EventPattern))

	rules, err := eb.ListRules(ctx, &eventbridge.ListRulesInput{NamePrefix: aws.String("eb-sdk")})
	require.NoError(t, err)
	require.Len(t, rules.Rules, 1)
	assert.Equal(t, "eb-sdk-rule", aws.ToString(rules.Rules[0].Name))

	tags, err := eb.ListTagsForResource(ctx, &eventbridge.ListTagsForResourceInput{
		ResourceARN: putRule.RuleArn,
	})
	require.NoError(t, err)
	require.Len(t, tags.Tags, 1)
	assert.Equal(t, "env", aws.ToString(tags.Tags[0].Key))
	assert.Equal(t, "test", aws.ToString(tags.Tags[0].Value))

	_, err = eb.TagResource(ctx, &eventbridge.TagResourceInput{
		ResourceARN: putRule.RuleArn,
		Tags:        []ebtypes.Tag{{Key: aws.String("owner"), Value: aws.String("sdk")}},
	})
	require.NoError(t, err)
	tags, err = eb.ListTagsForResource(ctx, &eventbridge.ListTagsForResourceInput{
		ResourceARN: putRule.RuleArn,
	})
	require.NoError(t, err)
	require.Len(t, tags.Tags, 2)

	_, err = eb.UntagResource(ctx, &eventbridge.UntagResourceInput{
		ResourceARN: putRule.RuleArn,
		TagKeys:     []string{"owner"},
	})
	require.NoError(t, err)
	tags, err = eb.ListTagsForResource(ctx, &eventbridge.ListTagsForResourceInput{
		ResourceARN: putRule.RuleArn,
	})
	require.NoError(t, err)
	require.Len(t, tags.Tags, 1)

	_, err = eb.DisableRule(ctx, &eventbridge.DisableRuleInput{Name: aws.String("eb-sdk-rule")})
	require.NoError(t, err)
	describe, err = eb.DescribeRule(ctx, &eventbridge.DescribeRuleInput{Name: aws.String("eb-sdk-rule")})
	require.NoError(t, err)
	assert.Equal(t, ebtypes.RuleStateDisabled, describe.State)

	_, err = eb.EnableRule(ctx, &eventbridge.EnableRuleInput{Name: aws.String("eb-sdk-rule")})
	require.NoError(t, err)
	describe, err = eb.DescribeRule(ctx, &eventbridge.DescribeRuleInput{Name: aws.String("eb-sdk-rule")})
	require.NoError(t, err)
	assert.Equal(t, ebtypes.RuleStateEnabled, describe.State)

	targets, err := eb.PutTargets(ctx, &eventbridge.PutTargetsInput{
		Rule: aws.String("eb-sdk-rule"),
		Targets: []ebtypes.Target{{
			Id:  aws.String("queue"),
			Arn: aws.String(queueARN),
		}},
	})
	require.NoError(t, err)
	assert.EqualValues(t, 0, targets.FailedEntryCount)

	listTargets, err := eb.ListTargetsByRule(ctx, &eventbridge.ListTargetsByRuleInput{
		Rule: aws.String("eb-sdk-rule"),
	})
	require.NoError(t, err)
	require.Len(t, listTargets.Targets, 1)
	assert.Equal(t, queueARN, aws.ToString(listTargets.Targets[0].Arn))

	putEvents, err := eb.PutEvents(ctx, &eventbridge.PutEventsInput{
		Entries: []ebtypes.PutEventsRequestEntry{{
			Source:     aws.String("sockerless.test"),
			DetailType: aws.String("example"),
			Detail:     aws.String(`{"ok":true}`),
		}},
	})
	require.NoError(t, err)
	assert.EqualValues(t, 0, putEvents.FailedEntryCount)
	require.Len(t, putEvents.Entries, 1)
	require.NotEmpty(t, aws.ToString(putEvents.Entries[0].EventId))

	received, err := sqsC.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		QueueUrl:            q.QueueUrl,
		MaxNumberOfMessages: 1,
	})
	require.NoError(t, err)
	require.Len(t, received.Messages, 1)
	assert.JSONEq(t, `{"ok":true}`, aws.ToString(received.Messages[0].Body))
}
