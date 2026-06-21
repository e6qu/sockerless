package aws_sdk_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sqsClient() *sqs.Client {
	return sqs.NewFromConfig(sdkConfig(), func(o *sqs.Options) {
		o.BaseEndpoint = aws.String(baseURL)
	})
}

// TestSQS_ReceiveMessageAttributeSubsetMD5 asserts that requesting a subset of
// message attributes recomputes MD5OfMessageAttributes over exactly the returned
// set. aws-sdk-go-v2's ValidateMessageChecksums middleware fails the call on a
// digest mismatch, so a successful subset receive proves the recompute (the sim
// previously re-emitted the stored full-set digest).
func TestSQS_ReceiveMessageAttributeSubsetMD5(t *testing.T) {
	client := sqsClient()
	create, err := client.CreateQueue(ctx, &sqs.CreateQueueInput{QueueName: aws.String("attr-md5-q")})
	require.NoError(t, err)
	url := aws.ToString(create.QueueUrl)

	_, err = client.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:    aws.String(url),
		MessageBody: aws.String("hello"),
		MessageAttributes: map[string]sqstypes.MessageAttributeValue{
			"alpha": {DataType: aws.String("String"), StringValue: aws.String("one")},
			"beta":  {DataType: aws.String("String"), StringValue: aws.String("two")},
		},
	})
	require.NoError(t, err)

	recv, err := client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		QueueUrl:              aws.String(url),
		MessageAttributeNames: []string{"alpha"},
	})
	require.NoError(t, err, "subset MD5 must validate against the returned attribute set")
	require.Len(t, recv.Messages, 1)
	assert.Len(t, recv.Messages[0].MessageAttributes, 1)
	assert.Contains(t, recv.Messages[0].MessageAttributes, "alpha")
	assert.NotContains(t, recv.Messages[0].MessageAttributes, "beta")
}

// TestSQS_QueueLifecycle exercises the 90th-percentile produce-
// consume-ack flow that real consumers run against SQS.
func TestSQS_QueueLifecycle(t *testing.T) {
	client := sqsClient()

	// CreateQueue + GetQueueUrl round-trip.
	create, err := client.CreateQueue(ctx, &sqs.CreateQueueInput{
		QueueName: aws.String("life-q"),
	})
	require.NoError(t, err)
	require.NotNil(t, create.QueueUrl)
	queueURL := *create.QueueUrl
	t.Cleanup(func() {
		_, _ = client.DeleteQueue(ctx, &sqs.DeleteQueueInput{QueueUrl: aws.String(queueURL)})
	})

	urlOut, err := client.GetQueueUrl(ctx, &sqs.GetQueueUrlInput{
		QueueName: aws.String("life-q"),
	})
	require.NoError(t, err)
	assert.Equal(t, queueURL, aws.ToString(urlOut.QueueUrl))

	// ListQueues + prefix filter.
	list, err := client.ListQueues(ctx, &sqs.ListQueuesInput{
		QueueNamePrefix: aws.String("life-"),
	})
	require.NoError(t, err)
	assert.Contains(t, list.QueueUrls, queueURL)

	// GetQueueAttributes — empty queue should report 0 messages.
	attrs, err := client.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
		QueueUrl:       aws.String(queueURL),
		AttributeNames: []sqstypes.QueueAttributeName{"All"},
	})
	require.NoError(t, err)
	assert.Equal(t, "0", attrs.Attributes["ApproximateNumberOfMessages"])
	assert.NotEmpty(t, attrs.Attributes["QueueArn"])

	_, err = client.SetQueueAttributes(ctx, &sqs.SetQueueAttributesInput{
		QueueUrl: aws.String(queueURL),
		Attributes: map[string]string{
			"VisibilityTimeout": "7",
		},
	})
	require.NoError(t, err)
	updatedAttrs, err := client.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
		QueueUrl:       aws.String(queueURL),
		AttributeNames: []sqstypes.QueueAttributeName{sqstypes.QueueAttributeNameVisibilityTimeout},
	})
	require.NoError(t, err)
	assert.Equal(t, "7", updatedAttrs.Attributes["VisibilityTimeout"])

	// Send + Receive round-trip.
	send, err := client.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:    aws.String(queueURL),
		MessageBody: aws.String("hello sqs"),
	})
	require.NoError(t, err)
	require.NotNil(t, send.MessageId)
	require.NotEmpty(t, aws.ToString(send.MD5OfMessageBody))

	recv, err := client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		QueueUrl: aws.String(queueURL),
	})
	require.NoError(t, err)
	require.Len(t, recv.Messages, 1)
	assert.Equal(t, "hello sqs", aws.ToString(recv.Messages[0].Body))
	receiptHandle := aws.ToString(recv.Messages[0].ReceiptHandle)
	require.NotEmpty(t, receiptHandle)

	// Visibility timeout: a second ReceiveMessage right away should
	// see nothing (the message is in-flight under the receipt handle).
	recv2, err := client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		QueueUrl: aws.String(queueURL),
	})
	require.NoError(t, err)
	assert.Empty(t, recv2.Messages,
		"message should be invisible during the visibility-timeout window")

	// DeleteMessage acks it.
	_, err = client.DeleteMessage(ctx, &sqs.DeleteMessageInput{
		QueueUrl:      aws.String(queueURL),
		ReceiptHandle: aws.String(receiptHandle),
	})
	require.NoError(t, err)

	// Attributes should show 0 messages again (ack removed it).
	attrsAfter, err := client.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
		QueueUrl:       aws.String(queueURL),
		AttributeNames: []sqstypes.QueueAttributeName{"All"},
	})
	require.NoError(t, err)
	assert.Equal(t, "0", attrsAfter.Attributes["ApproximateNumberOfMessages"])
}

// TestSQS_TagQueueRoundTrip asserts tag CRUD per the SDK shape.
func TestSQS_TagQueueRoundTrip(t *testing.T) {
	client := sqsClient()
	out, err := client.CreateQueue(ctx, &sqs.CreateQueueInput{QueueName: aws.String("tagq")})
	require.NoError(t, err)
	url := aws.ToString(out.QueueUrl)
	t.Cleanup(func() {
		_, _ = client.DeleteQueue(ctx, &sqs.DeleteQueueInput{QueueUrl: aws.String(url)})
	})
	_, err = client.TagQueue(ctx, &sqs.TagQueueInput{
		QueueUrl: aws.String(url),
		Tags:     map[string]string{"env": "test", "team": "runner"},
	})
	require.NoError(t, err)
	listTags, err := client.ListQueueTags(ctx, &sqs.ListQueueTagsInput{QueueUrl: aws.String(url)})
	require.NoError(t, err)
	assert.Equal(t, "test", listTags.Tags["env"])
	assert.Equal(t, "runner", listTags.Tags["team"])
	_, err = client.UntagQueue(ctx, &sqs.UntagQueueInput{
		QueueUrl: aws.String(url),
		TagKeys:  []string{"team"},
	})
	require.NoError(t, err)
	listTags2, err := client.ListQueueTags(ctx, &sqs.ListQueueTagsInput{QueueUrl: aws.String(url)})
	require.NoError(t, err)
	assert.Equal(t, "test", listTags2.Tags["env"])
	_, untagged := listTags2.Tags["team"]
	assert.False(t, untagged, "team tag should be removed")
}

// TestSQS_NonExistentQueue confirms the canonical error envelope
// real callers handle on a missing queue.
func TestSQS_NonExistentQueue(t *testing.T) {
	client := sqsClient()
	_, err := client.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
		QueueUrl: aws.String("https://sqs.us-east-1.amazonaws.com/000000000000/missing"),
	})
	require.Error(t, err)
	var apiErr smithy.APIError
	require.True(t, errors.As(err, &apiErr), "expected AWS API error, got %T: %v", err, err)
	assert.Equal(t, "AWS.SimpleQueueService.NonExistentQueue", apiErr.ErrorCode())
}

func TestSQS_ReceiveMessageRejectsInvalidMaxNumberOfMessages(t *testing.T) {
	client := sqsClient()
	out, err := client.CreateQueue(ctx, &sqs.CreateQueueInput{QueueName: aws.String("max-invalid-q")})
	require.NoError(t, err)
	queueURL := aws.ToString(out.QueueUrl)
	t.Cleanup(func() {
		_, _ = client.DeleteQueue(ctx, &sqs.DeleteQueueInput{QueueUrl: aws.String(queueURL)})
	})

	_, err = client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		QueueUrl:            aws.String(queueURL),
		MaxNumberOfMessages: 25,
	})
	require.Error(t, err)
	var apiErr smithy.APIError
	require.True(t, errors.As(err, &apiErr), "expected AWS API error, got %T: %v", err, err)
	assert.Equal(t, "InvalidParameterValue", apiErr.ErrorCode())
	assert.Contains(t, apiErr.ErrorMessage(), "must be between 1 and 10")
}

// TestSQS_VisibilityTimeoutExpiry asserts a message returns to
// visible state after the per-receive timeout elapses.
func TestSQS_VisibilityTimeoutExpiry(t *testing.T) {
	if testing.Short() {
		t.Skip("visibility-timeout expiry requires a real wait; -short skip")
	}
	client := sqsClient()
	out, err := client.CreateQueue(ctx, &sqs.CreateQueueInput{
		QueueName: aws.String("vtq"),
		Attributes: map[string]string{
			"VisibilityTimeout": "1",
		},
	})
	require.NoError(t, err)
	url := aws.ToString(out.QueueUrl)
	t.Cleanup(func() {
		_, _ = client.DeleteQueue(ctx, &sqs.DeleteQueueInput{QueueUrl: aws.String(url)})
	})
	_, err = client.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl: aws.String(url), MessageBody: aws.String("vt"),
	})
	require.NoError(t, err)
	_, err = client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{QueueUrl: aws.String(url)})
	require.NoError(t, err)
	// Wait past the 1-second visibility timeout.
	time.Sleep(1500 * time.Millisecond)
	recv, err := client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{QueueUrl: aws.String(url)})
	require.NoError(t, err)
	require.Len(t, recv.Messages, 1,
		"message should be visible again after the visibility-timeout window elapsed")
	assert.Equal(t, "vt", aws.ToString(recv.Messages[0].Body))
}

func TestSQS_ListQueues_Pagination(t *testing.T) {
	client := sqsClient()
	names := []string{"pag-sqs-a", "pag-sqs-b", "pag-sqs-c"}
	for _, n := range names {
		out, err := client.CreateQueue(ctx, &sqs.CreateQueueInput{QueueName: aws.String(n)})
		require.NoError(t, err)
		url := aws.ToString(out.QueueUrl)
		t.Cleanup(func() { client.DeleteQueue(ctx, &sqs.DeleteQueueInput{QueueUrl: aws.String(url)}) })
	}

	seen := map[string]bool{}
	pager := sqs.NewListQueuesPaginator(client, &sqs.ListQueuesInput{MaxResults: aws.Int32(1)})
	for pager.HasMorePages() {
		page, err := pager.NextPage(ctx)
		require.NoError(t, err)
		for _, u := range page.QueueUrls {
			seen[u] = true
		}
	}
	for _, n := range names {
		found := false
		for u := range seen {
			if strings.Contains(u, n) {
				found = true
				break
			}
		}
		assert.True(t, found, "queue %s should appear via pagination", n)
	}
}
