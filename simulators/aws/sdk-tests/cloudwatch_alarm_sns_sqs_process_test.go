package aws_sdk_test

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCloudWatch_AlarmSNSActionToSQS_ProcessMode is the regression test for
// issue #741. It starts a fresh simulator-aws subprocess with SIM_RUNTIME=process
// and exercises the same shape as the adversarial CLI probe:
//   - SQS queue policy with Principal "*" and no aws:SourceArn condition
//   - a short settle window between Subscribe and PutMetricAlarm
//   - PutMetricData breaching the threshold
//   - DescribeAlarms surfacing ALARM
//   - SQS ReceiveMessage returning the alarm notification
//
// This proves delivery works in API-only process mode, independent of the
// shared TestMain simulator's runtime.
func TestCloudWatch_AlarmSNSActionToSQS_ProcessMode(t *testing.T) {
	url := startProcessModeSim(t)

	cw := cloudwatch.NewFromConfig(sdkConfig(), func(o *cloudwatch.Options) {
		o.BaseEndpoint = aws.String(url)
	})
	snsC := sns.NewFromConfig(sdkConfig(), func(o *sns.Options) {
		o.BaseEndpoint = aws.String(url)
	})
	sqsC := sqs.NewFromConfig(sdkConfig(), func(o *sqs.Options) {
		o.BaseEndpoint = aws.String(url)
	})

	ns := "Custom/AlarmProcessRepro"
	alarmName := "process-repro-alarm"

	tpc, err := snsC.CreateTopic(ctx, &sns.CreateTopicInput{Name: aws.String("process-repro-t")})
	require.NoError(t, err)
	topicARN := aws.ToString(tpc.TopicArn)
	t.Cleanup(func() {
		_, _ = snsC.DeleteTopic(ctx, &sns.DeleteTopicInput{TopicArn: tpc.TopicArn})
	})

	q, err := sqsC.CreateQueue(ctx, &sqs.CreateQueueInput{QueueName: aws.String("process-repro-q")})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = sqsC.DeleteQueue(ctx, &sqs.DeleteQueueInput{QueueUrl: q.QueueUrl})
	})
	queueAttrs, err := sqsC.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
		QueueUrl:       q.QueueUrl,
		AttributeNames: []sqstypes.QueueAttributeName{sqstypes.QueueAttributeNameQueueArn},
	})
	require.NoError(t, err)
	queueARN := queueAttrs.Attributes["QueueArn"]

	// Same permissive shape as the failing CLI probe: Principal "*", no
	// aws:SourceArn condition. Real AWS allows this; the sim must too.
	policy := fmt.Sprintf(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":"*","Action":"sqs:SendMessage","Resource":"%s"}]}`, queueARN)
	_, err = sqsC.SetQueueAttributes(ctx, &sqs.SetQueueAttributesInput{
		QueueUrl: q.QueueUrl,
		Attributes: map[string]string{
			"Policy": policy,
		},
	})
	require.NoError(t, err)

	_, err = snsC.Subscribe(ctx, &sns.SubscribeInput{
		TopicArn: tpc.TopicArn,
		Protocol: aws.String("sqs"),
		Endpoint: aws.String(queueARN),
	})
	require.NoError(t, err)

	// Settle window matching the CLI probe. This ensures the test does not
	// rely on a race between subscription creation and the alarm firing.
	time.Sleep(3 * time.Second)

	_, err = cw.PutMetricAlarm(ctx, &cloudwatch.PutMetricAlarmInput{
		AlarmName:          aws.String(alarmName),
		AlarmDescription:   aws.String("Adversarial probe CPU alarm"),
		Namespace:          aws.String(ns),
		MetricName:         aws.String("CPUUtilization"),
		ComparisonOperator: cwtypes.ComparisonOperatorGreaterThanThreshold,
		EvaluationPeriods:  aws.Int32(1),
		Period:             aws.Int32(60),
		Threshold:          aws.Float64(50),
		Statistic:          cwtypes.StatisticAverage,
		TreatMissingData:   aws.String("notBreaching"),
		ActionsEnabled:     aws.Bool(true),
		AlarmActions:       []string{topicARN},
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = cw.DeleteAlarms(ctx, &cloudwatch.DeleteAlarmsInput{AlarmNames: []string{alarmName}})
	})

	_, err = cw.PutMetricData(ctx, &cloudwatch.PutMetricDataInput{
		Namespace: aws.String(ns),
		MetricData: []cwtypes.MetricDatum{
			{MetricName: aws.String("CPUUtilization"), Value: aws.Float64(100), Timestamp: aws.Time(time.Now().UTC())},
		},
	})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		desc, err := cw.DescribeAlarms(ctx, &cloudwatch.DescribeAlarmsInput{AlarmNames: []string{alarmName}})
		require.NoError(t, err)
		return len(desc.MetricAlarms) == 1 && desc.MetricAlarms[0].StateValue == cwtypes.StateValueAlarm
	}, 15*time.Second, 500*time.Millisecond, "alarm should reach ALARM")

	// Give the background evaluator time to dispatch the ALARM action.
	time.Sleep(2 * time.Second)

	recv, err := sqsC.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{QueueUrl: q.QueueUrl})
	require.NoError(t, err)
	require.Len(t, recv.Messages, 1, "SQS subscriber should receive the alarm notification")

	var env map[string]any
	require.NoError(t, json.Unmarshal([]byte(aws.ToString(recv.Messages[0].Body)), &env))
	assert.Equal(t, "Notification", env["Type"])

	inner, ok := env["Message"].(string)
	require.True(t, ok, "SNS Message must be a string")
	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(inner), &body), "embedded SNS Message must be valid JSON")
	assert.Equal(t, alarmName, body["AlarmName"])
	assert.Equal(t, "ALARM", body["NewStateValue"])
	assert.Equal(t, "us-east-1", body["Region"])
	assert.Equal(t, "123456789012", body["AWSAccountId"])
}
