package gcp_sdk_test

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pubsub "google.golang.org/api/pubsub/v1"
	"google.golang.org/api/option"
)

func pubsubService(t *testing.T) *pubsub.Service {
	t.Helper()
	svc, err := pubsub.NewService(ctx,
		option.WithEndpoint(baseURL),
		option.WithoutAuthentication(),
	)
	require.NoError(t, err)
	return svc
}

// TestPubSub_TopicAndSubscriptionLifecycle exercises the canonical
// fan-out flow: create topic → create subscription → publish → pull
// → ack. Regression guard for BUG-1102 (issue #177, Pub/Sub slice).
func TestPubSub_TopicAndSubscriptionLifecycle(t *testing.T) {
	svc := pubsubService(t)
	project := "test-project"
	topicName := "projects/" + project + "/topics/lifecycle-topic"
	subName := "projects/" + project + "/subscriptions/lifecycle-sub"

	topic, err := svc.Projects.Topics.Create(topicName, &pubsub.Topic{Name: topicName}).Do()
	require.NoError(t, err)
	assert.Equal(t, topicName, topic.Name)
	t.Cleanup(func() {
		_, _ = svc.Projects.Topics.Delete(topicName).Do()
	})

	// List should include the new topic.
	list, err := svc.Projects.Topics.List("projects/" + project).Do()
	require.NoError(t, err)
	found := false
	for _, x := range list.Topics {
		if x.Name == topicName {
			found = true
			break
		}
	}
	assert.True(t, found, "topic should appear in ListTopics")

	// Create subscription.
	sub, err := svc.Projects.Subscriptions.Create(subName, &pubsub.Subscription{
		Topic:              topicName,
		AckDeadlineSeconds: 30,
	}).Do()
	require.NoError(t, err)
	assert.Equal(t, topicName, sub.Topic)
	assert.Equal(t, int64(30), sub.AckDeadlineSeconds)
	t.Cleanup(func() {
		_, _ = svc.Projects.Subscriptions.Delete(subName).Do()
	})

	// Publish two messages.
	pubResp, err := svc.Projects.Topics.Publish(topicName, &pubsub.PublishRequest{
		Messages: []*pubsub.PubsubMessage{
			{Data: base64.StdEncoding.EncodeToString([]byte("hello"))},
			{Data: base64.StdEncoding.EncodeToString([]byte("world"))},
		},
	}).Do()
	require.NoError(t, err)
	require.Len(t, pubResp.MessageIds, 2)

	// Pull them back.
	pullResp, err := svc.Projects.Subscriptions.Pull(subName, &pubsub.PullRequest{
		MaxMessages: 10,
	}).Do()
	require.NoError(t, err)
	require.Len(t, pullResp.ReceivedMessages, 2)

	// Verify payloads decode correctly.
	bodies := map[string]bool{}
	var ackIds []string
	for _, m := range pullResp.ReceivedMessages {
		decoded, err := base64.StdEncoding.DecodeString(m.Message.Data)
		require.NoError(t, err)
		bodies[string(decoded)] = true
		ackIds = append(ackIds, m.AckId)
	}
	assert.True(t, bodies["hello"], "expected 'hello' payload")
	assert.True(t, bodies["world"], "expected 'world' payload")

	// Acknowledge.
	_, err = svc.Projects.Subscriptions.Acknowledge(subName, &pubsub.AcknowledgeRequest{
		AckIds: ackIds,
	}).Do()
	require.NoError(t, err)

	// Second pull should be empty (queue drained by ack).
	pullResp2, err := svc.Projects.Subscriptions.Pull(subName, &pubsub.PullRequest{
		MaxMessages: 10,
	}).Do()
	require.NoError(t, err)
	assert.Empty(t, pullResp2.ReceivedMessages,
		"subscription queue should be empty after ack")
}

// TestPubSub_NonExistentTopic confirms canonical 404 envelope.
func TestPubSub_NonExistentTopic(t *testing.T) {
	svc := pubsubService(t)
	_, err := svc.Projects.Topics.Get("projects/test-project/topics/missing").Do()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Topic not found")
}
