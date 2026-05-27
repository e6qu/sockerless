package aws_sdk_test

import (
	"testing"

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
