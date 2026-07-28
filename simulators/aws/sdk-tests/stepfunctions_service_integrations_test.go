package aws_sdk_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge"
	eventtypes "github.com/aws/aws-sdk-go-v2/service/eventbridge/types"
	"github.com/aws/aws-sdk-go-v2/service/sfn"
	sfntypes "github.com/aws/aws-sdk-go-v2/service/sfn/types"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSFN_EventingAndObservabilityIntegrations_SDK proves the workflow runtime
// executes the real service integrations rather than merely accepting their
// Amazon States Language definitions. Every resource and observation travels
// through an official AWS SDK client at the simulator coordinate.
func TestSFN_EventingAndObservabilityIntegrations_SDK(t *testing.T) {
	queueAPI := sqsClient()
	topicAPI := snsClient()
	eventAPI := eventbridgeClient()
	statesAPI := sfnClient()
	metricsAPI := cloudwatchClient()

	queue, err := queueAPI.CreateQueue(ctx, &sqs.CreateQueueInput{QueueName: aws.String("sfn-integrations")})
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = queueAPI.DeleteQueue(ctx, &sqs.DeleteQueueInput{QueueUrl: queue.QueueUrl}) })
	attributes, err := queueAPI.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
		QueueUrl: queue.QueueUrl, AttributeNames: []sqstypes.QueueAttributeName{sqstypes.QueueAttributeNameQueueArn},
	})
	require.NoError(t, err)
	queueARN := attributes.Attributes["QueueArn"]
	policy := fmt.Sprintf(
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":"*","Action":"sqs:SendMessage","Resource":%q}]}`,
		queueARN,
	)
	_, err = queueAPI.SetQueueAttributes(ctx, &sqs.SetQueueAttributesInput{
		QueueUrl: queue.QueueUrl, Attributes: map[string]string{"Policy": policy},
	})
	require.NoError(t, err)

	topic, err := topicAPI.CreateTopic(ctx, &sns.CreateTopicInput{Name: aws.String("sfn-integrations")})
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = topicAPI.DeleteTopic(ctx, &sns.DeleteTopicInput{TopicArn: topic.TopicArn}) })
	_, err = topicAPI.Subscribe(ctx, &sns.SubscribeInput{
		TopicArn: topic.TopicArn, Protocol: aws.String("sqs"), Endpoint: aws.String(queueARN),
	})
	require.NoError(t, err)

	ruleName := "sfn-integrations"
	pattern := `{"source":["sfn.integration"]}`
	_, err = eventAPI.PutRule(ctx, &eventbridge.PutRuleInput{
		Name: aws.String(ruleName), EventPattern: aws.String(pattern),
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = eventAPI.RemoveTargets(ctx, &eventbridge.RemoveTargetsInput{
			Rule: aws.String(ruleName), Ids: []string{"queue"},
		})
		_, _ = eventAPI.DeleteRule(ctx, &eventbridge.DeleteRuleInput{Name: aws.String(ruleName)})
	})
	_, err = eventAPI.PutTargets(ctx, &eventbridge.PutTargetsInput{
		Rule:    aws.String(ruleName),
		Targets: []eventtypes.Target{{Id: aws.String("queue"), Arn: aws.String(queueARN)}},
	})
	require.NoError(t, err)

	definition, err := json.Marshal(map[string]any{
		"StartAt": "Queue",
		"States": map[string]any{
			"Queue": map[string]any{
				"Type": "Task", "Resource": "arn:aws:states:::sqs:sendMessage",
				"Parameters": map[string]any{"QueueUrl": aws.ToString(queue.QueueUrl), "MessageBody": "from-step-functions-sqs"},
				"Next":       "Topic",
			},
			"Topic": map[string]any{
				"Type": "Task", "Resource": "arn:aws:states:::sns:publish",
				"Parameters": map[string]any{"TopicArn": aws.ToString(topic.TopicArn), "Message": "from-step-functions-sns"},
				"Next":       "Event",
			},
			"Event": map[string]any{
				"Type": "Task", "Resource": "arn:aws:states:::events:putEvents",
				"Parameters": map[string]any{"Entries": []any{map[string]any{
					"Source": "sfn.integration", "DetailType": "Workflow", "Detail": `{"delivered":true}`,
				}}},
				"Next": "Metric",
			},
			"Metric": map[string]any{
				"Type": "Task", "Resource": "arn:aws:states:::aws-sdk:cloudwatch:putMetricData",
				"Parameters": map[string]any{
					"Namespace":  "Sockerless/StepFunctions",
					"MetricData": []any{map[string]any{"MetricName": "Completed", "Value": 1}},
				},
				"End": true,
			},
		},
	})
	require.NoError(t, err)
	machine, err := statesAPI.CreateStateMachine(ctx, &sfn.CreateStateMachineInput{
		Name: aws.String("sfn-service-integrations"), Definition: aws.String(string(definition)),
		RoleArn: aws.String("arn:aws:iam::123456789012:role/sfn-role"),
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = statesAPI.DeleteStateMachine(ctx, &sfn.DeleteStateMachineInput{StateMachineArn: machine.StateMachineArn})
	})
	execution, err := statesAPI.StartExecution(ctx, &sfn.StartExecutionInput{StateMachineArn: machine.StateMachineArn})
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		described, describeErr := statesAPI.DescribeExecution(ctx, &sfn.DescribeExecutionInput{ExecutionArn: execution.ExecutionArn})
		return describeErr == nil && described.Status == sfntypes.ExecutionStatusSucceeded
	}, 10*time.Second, 100*time.Millisecond)

	var bodies []string
	require.Eventually(t, func() bool {
		received, receiveErr := queueAPI.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
			QueueUrl: queue.QueueUrl, MaxNumberOfMessages: 10, VisibilityTimeout: 0,
		})
		if receiveErr != nil {
			return false
		}
		bodies = bodies[:0]
		for _, message := range received.Messages {
			bodies = append(bodies, aws.ToString(message.Body))
		}
		return len(bodies) == 3
	}, 5*time.Second, 100*time.Millisecond)
	assert.Contains(t, bodies, "from-step-functions-sqs")
	assert.Condition(t, func() bool {
		for _, body := range bodies {
			if len(body) > 0 && body[0] == '{' && strings.Contains(body, "from-step-functions-sns") {
				return true
			}
		}
		return false
	}, "Amazon SNS must deliver its notification envelope")
	assert.Condition(t, func() bool {
		for _, body := range bodies {
			if strings.Contains(body, `"delivered":true`) {
				return true
			}
		}
		return false
	}, "Amazon EventBridge must deliver the matched event")

	history, err := statesAPI.GetExecutionHistory(ctx, &sfn.GetExecutionHistoryInput{ExecutionArn: execution.ExecutionArn})
	require.NoError(t, err)
	var succeededTasks int
	for _, event := range history.Events {
		if event.Type == sfntypes.HistoryEventTypeTaskSucceeded {
			succeededTasks++
		}
	}
	assert.Equal(t, 4, succeededTasks, "every external service integration must complete")

	now := time.Now().UTC()
	metrics, err := metricsAPI.GetMetricData(ctx, &cloudwatch.GetMetricDataInput{
		StartTime: aws.Time(now.Add(-time.Minute)),
		EndTime:   aws.Time(now.Add(time.Minute)),
		MetricDataQueries: []cwtypes.MetricDataQuery{{
			Id: aws.String("completed"),
			MetricStat: &cwtypes.MetricStat{
				Metric: &cwtypes.Metric{
					Namespace:  aws.String("Sockerless/StepFunctions"),
					MetricName: aws.String("Completed"),
				},
				Period: aws.Int32(60),
				Stat:   aws.String("Sum"),
			},
		}},
	})
	require.NoError(t, err)
	require.Len(t, metrics.MetricDataResults, 1)
	assert.Equal(t, []float64{1}, metrics.MetricDataResults[0].Values)
}
