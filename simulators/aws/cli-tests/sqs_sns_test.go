package aws_cli_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSQSCLI_QueueLifecycle(t *testing.T) {
	out := runCLI(t, awsCLI("sqs", "create-queue", "--queue-name", "cli-sqs-q"))
	var created struct {
		QueueUrl string `json:"QueueUrl"`
	}
	parseJSON(t, out, &created)
	require.NotEmpty(t, created.QueueUrl)
	t.Cleanup(func() {
		_ = awsCLI("sqs", "delete-queue", "--queue-url", created.QueueUrl).Run()
	})

	out = runCLI(t, awsCLI("sqs", "get-queue-url", "--queue-name", "cli-sqs-q"))
	var located struct {
		QueueUrl string `json:"QueueUrl"`
	}
	parseJSON(t, out, &located)
	assert.Equal(t, created.QueueUrl, located.QueueUrl)

	runCLI(t, awsCLI("sqs", "send-message",
		"--queue-url", created.QueueUrl,
		"--message-body", "hello direct sqs cli"))

	out = runCLI(t, awsCLI("sqs", "receive-message",
		"--queue-url", created.QueueUrl,
		"--max-number-of-messages", "1"))
	var recv struct {
		Messages []struct {
			Body          string `json:"Body"`
			ReceiptHandle string `json:"ReceiptHandle"`
		} `json:"Messages"`
	}
	parseJSON(t, out, &recv)
	require.Len(t, recv.Messages, 1)
	assert.Equal(t, "hello direct sqs cli", recv.Messages[0].Body)

	runCLI(t, awsCLI("sqs", "delete-message",
		"--queue-url", created.QueueUrl,
		"--receipt-handle", recv.Messages[0].ReceiptHandle))
}

func TestSNSCLI_TopicSQSFanout(t *testing.T) {
	out := runCLI(t, awsCLI("sns", "create-topic", "--name", "cli-sns-topic"))
	var topic struct {
		TopicArn string `json:"TopicArn"`
	}
	parseJSON(t, out, &topic)
	require.NotEmpty(t, topic.TopicArn)
	t.Cleanup(func() {
		_ = awsCLI("sns", "delete-topic", "--topic-arn", topic.TopicArn).Run()
	})

	out = runCLI(t, awsCLI("sqs", "create-queue", "--queue-name", "cli-sns-q"))
	var queue struct {
		QueueUrl string `json:"QueueUrl"`
	}
	parseJSON(t, out, &queue)
	require.NotEmpty(t, queue.QueueUrl)
	t.Cleanup(func() {
		_ = awsCLI("sqs", "delete-queue", "--queue-url", queue.QueueUrl).Run()
	})

	out = runCLI(t, awsCLI("sqs", "get-queue-attributes",
		"--queue-url", queue.QueueUrl,
		"--attribute-names", "QueueArn"))
	var attrs struct {
		Attributes map[string]string `json:"Attributes"`
	}
	parseJSON(t, out, &attrs)
	queueARN := attrs.Attributes["QueueArn"]
	require.NotEmpty(t, queueARN)

	out = runCLI(t, awsCLI("sns", "subscribe",
		"--topic-arn", topic.TopicArn,
		"--protocol", "sqs",
		"--notification-endpoint", queueARN))
	var sub struct {
		SubscriptionArn string `json:"SubscriptionArn"`
	}
	parseJSON(t, out, &sub)
	require.NotEmpty(t, sub.SubscriptionArn)
	t.Cleanup(func() {
		_ = awsCLI("sns", "unsubscribe", "--subscription-arn", sub.SubscriptionArn).Run()
	})

	runCLI(t, awsCLI("sns", "publish",
		"--topic-arn", topic.TopicArn,
		"--message", `{"source":"sns-cli"}`))

	out = runCLI(t, awsCLI("sqs", "receive-message",
		"--queue-url", queue.QueueUrl,
		"--max-number-of-messages", "1"))
	var recv struct {
		Messages []struct {
			Body string `json:"Body"`
		} `json:"Messages"`
	}
	parseJSON(t, out, &recv)
	require.Len(t, recv.Messages, 1)
	assert.Contains(t, recv.Messages[0].Body, `"Message":"{\"source\":\"sns-cli\"}"`)
}
