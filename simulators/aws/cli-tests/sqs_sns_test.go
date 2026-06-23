package aws_cli_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setQueuePolicyAllowingSNSCLI attaches, via `aws sqs set-queue-attributes`, a
// resource policy granting sns.amazonaws.com sqs:SendMessage scoped to the
// topic — the policy real SNS→SQS delivery requires on the subscriber queue.
func setQueuePolicyAllowingSNSCLI(t *testing.T, queueURL, queueARN, topicARN string) {
	t.Helper()
	policy := fmt.Sprintf(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"sns.amazonaws.com"},"Action":"sqs:SendMessage","Resource":%q,"Condition":{"ArnEquals":{"aws:SourceArn":%q}}}]}`, queueARN, topicARN)
	attrs, err := json.Marshal(map[string]string{"Policy": policy})
	require.NoError(t, err)
	runCLI(t, awsCLI("sqs", "set-queue-attributes",
		"--queue-url", queueURL,
		"--attributes", string(attrs)))
}

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

	setQueuePolicyAllowingSNSCLI(t, queue.QueueUrl, queueARN, topic.TopicArn)

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

// TestSQSCLI_FifoCouplingAndGroupId asserts the FIFO name↔attribute
// coupling at create-queue and the MessageGroupId requirement on send.
func TestSQSCLI_FifoCouplingAndGroupId(t *testing.T) {
	// .fifo name without FifoQueue=true → InvalidParameterValue.
	errOut := runCLIExpectError(t, awsCLI("sqs", "create-queue", "--queue-name", "cli-bad.fifo"))
	assert.Contains(t, errOut, "InvalidParameterValue")

	// Proper FIFO queue.
	out := runCLI(t, awsCLI("sqs", "create-queue",
		"--queue-name", "cli-fifo.fifo",
		"--attributes", "FifoQueue=true,ContentBasedDeduplication=true"))
	var created struct {
		QueueUrl string `json:"QueueUrl"`
	}
	parseJSON(t, out, &created)
	require.NotEmpty(t, created.QueueUrl)
	t.Cleanup(func() {
		_ = awsCLI("sqs", "delete-queue", "--queue-url", created.QueueUrl).Run()
	})

	// Send without MessageGroupId → MissingParameter.
	errOut = runCLIExpectError(t, awsCLI("sqs", "send-message",
		"--queue-url", created.QueueUrl,
		"--message-body", "nogroup"))
	assert.Contains(t, errOut, "MissingParameter")

	// Send with MessageGroupId → succeeds.
	runCLI(t, awsCLI("sqs", "send-message",
		"--queue-url", created.QueueUrl,
		"--message-body", "withgroup",
		"--message-group-id", "g1"))
}

// TestSQSCLI_SendMessageBatch exercises a batch send plus a duplicate-Id
// batch-level error.
func TestSQSCLI_SendMessageBatch(t *testing.T) {
	out := runCLI(t, awsCLI("sqs", "create-queue", "--queue-name", "cli-batch-q"))
	var created struct {
		QueueUrl string `json:"QueueUrl"`
	}
	parseJSON(t, out, &created)
	t.Cleanup(func() {
		_ = awsCLI("sqs", "delete-queue", "--queue-url", created.QueueUrl).Run()
	})

	out = runCLI(t, awsCLI("sqs", "send-message-batch",
		"--queue-url", created.QueueUrl,
		"--entries",
		`[{"Id":"a","MessageBody":"body-a"},{"Id":"b","MessageBody":"body-b"}]`))
	var batch struct {
		Successful []struct {
			Id        string `json:"Id"`
			MessageId string `json:"MessageId"`
		} `json:"Successful"`
		Failed []struct {
			Id   string `json:"Id"`
			Code string `json:"Code"`
		} `json:"Failed"`
	}
	parseJSON(t, out, &batch)
	require.Len(t, batch.Successful, 2)
	assert.Empty(t, batch.Failed)

	// Both messages landed in the queue.
	out = runCLI(t, awsCLI("sqs", "receive-message",
		"--queue-url", created.QueueUrl,
		"--max-number-of-messages", "10"))
	var recv struct {
		Messages []struct {
			Body string `json:"Body"`
		} `json:"Messages"`
	}
	parseJSON(t, out, &recv)
	require.Len(t, recv.Messages, 2)

	// Duplicate Ids → BatchEntryIdsNotDistinct.
	errOut := runCLIExpectError(t, awsCLI("sqs", "send-message-batch",
		"--queue-url", created.QueueUrl,
		"--entries",
		`[{"Id":"dup","MessageBody":"1"},{"Id":"dup","MessageBody":"2"}]`))
	assert.Contains(t, errOut, "BatchEntryIdsNotDistinct")
}

// TestSNSCLI_FifoCouplingAndGroupId asserts the SNS FIFO name↔attribute
// coupling at create-topic and the MessageGroupId requirement on publish.
func TestSNSCLI_FifoCouplingAndGroupId(t *testing.T) {
	// .fifo name without FifoTopic=true → InvalidParameter.
	errOut := runCLIExpectError(t, awsCLI("sns", "create-topic", "--name", "cli-sns-bad.fifo"))
	assert.Contains(t, errOut, "InvalidParameter")

	out := runCLI(t, awsCLI("sns", "create-topic",
		"--name", "cli-sns-fifo.fifo",
		"--attributes", "FifoTopic=true,ContentBasedDeduplication=true"))
	var topic struct {
		TopicArn string `json:"TopicArn"`
	}
	parseJSON(t, out, &topic)
	require.NotEmpty(t, topic.TopicArn)
	t.Cleanup(func() {
		_ = awsCLI("sns", "delete-topic", "--topic-arn", topic.TopicArn).Run()
	})

	// Publish without MessageGroupId → InvalidParameter.
	errOut = runCLIExpectError(t, awsCLI("sns", "publish",
		"--topic-arn", topic.TopicArn,
		"--message", "nogroup"))
	assert.Contains(t, errOut, "InvalidParameter")

	// Publish with MessageGroupId → succeeds.
	runCLI(t, awsCLI("sns", "publish",
		"--topic-arn", topic.TopicArn,
		"--message", "withgroup",
		"--message-group-id", "g1"))
}

// TestSNSCLI_PublishBatch exercises a batch publish, fan-out to an SQS
// subscriber, and a duplicate-Id batch-level error.
func TestSNSCLI_PublishBatch(t *testing.T) {
	out := runCLI(t, awsCLI("sns", "create-topic", "--name", "cli-pubbatch-t"))
	var topic struct {
		TopicArn string `json:"TopicArn"`
	}
	parseJSON(t, out, &topic)
	t.Cleanup(func() {
		_ = awsCLI("sns", "delete-topic", "--topic-arn", topic.TopicArn).Run()
	})

	out = runCLI(t, awsCLI("sqs", "create-queue", "--queue-name", "cli-pubbatch-q"))
	var queue struct {
		QueueUrl string `json:"QueueUrl"`
	}
	parseJSON(t, out, &queue)
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
	setQueuePolicyAllowingSNSCLI(t, queue.QueueUrl, queueARN, topic.TopicArn)
	runCLI(t, awsCLI("sns", "subscribe",
		"--topic-arn", topic.TopicArn,
		"--protocol", "sqs",
		"--notification-endpoint", queueARN))

	out = runCLI(t, awsCLI("sns", "publish-batch",
		"--topic-arn", topic.TopicArn,
		"--publish-batch-request-entries",
		`[{"Id":"e1","Message":"msg-1"},{"Id":"e2","Message":"msg-2"}]`))
	var batch struct {
		Successful []struct {
			Id        string `json:"Id"`
			MessageId string `json:"MessageId"`
		} `json:"Successful"`
		Failed []any `json:"Failed"`
	}
	parseJSON(t, out, &batch)
	require.Len(t, batch.Successful, 2)
	assert.Empty(t, batch.Failed)

	// Both fanned out to the SQS subscriber.
	out = runCLI(t, awsCLI("sqs", "receive-message",
		"--queue-url", queue.QueueUrl,
		"--max-number-of-messages", "10"))
	var recv struct {
		Messages []struct {
			Body string `json:"Body"`
		} `json:"Messages"`
	}
	parseJSON(t, out, &recv)
	require.Len(t, recv.Messages, 2)
	assert.True(t,
		strings.Contains(recv.Messages[0].Body, `"Message":"msg-1"`) ||
			strings.Contains(recv.Messages[0].Body, `"Message":"msg-2"`))

	// Duplicate Ids → BatchEntryIdsNotDistinct.
	errOut := runCLIExpectError(t, awsCLI("sns", "publish-batch",
		"--topic-arn", topic.TopicArn,
		"--publish-batch-request-entries",
		`[{"Id":"dup","Message":"1"},{"Id":"dup","Message":"2"}]`))
	assert.Contains(t, errOut, "BatchEntryIdsNotDistinct")
}
