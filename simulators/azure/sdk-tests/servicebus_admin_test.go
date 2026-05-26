package azure_sdk_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus/admin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type sbAdminTransport struct {
	inner *http.Client
}

func (t sbAdminTransport) Do(req *http.Request) (*http.Response, error) {
	rewritten := req.Clone(req.Context())
	u := *req.URL
	rewritten.Host = req.URL.Host
	u.Scheme = "http"
	u.Host = strings.TrimPrefix(baseURL, "http://")
	rewritten.URL = &u
	client := t.inner
	if client == nil {
		client = http.DefaultClient
	}
	return client.Do(rewritten)
}

func sbAdminClient(t *testing.T, namespace string) *admin.Client {
	t.Helper()
	hostPort := strings.TrimPrefix(baseURL, "http://")
	_, port, ok := strings.Cut(hostPort, ":")
	require.True(t, ok, "baseURL must include a port: %s", baseURL)
	conn := fmt.Sprintf("Endpoint=sb://%s.servicebus.localhost:%s/;SharedAccessKeyName=RootManageSharedAccessKey;SharedAccessKey=AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=", namespace, port)
	client, err := admin.NewClientFromConnectionString(conn, &admin.ClientOptions{
		ClientOptions: azcore.ClientOptions{Transport: sbAdminTransport{}},
	})
	require.NoError(t, err)
	return client
}

func TestServiceBusAdmin_QueueSDKLifecycle(t *testing.T) {
	client := sbAdminClient(t, "sdk-admin-queue")
	ctx := context.Background()
	queueName := "q1"
	userMetadata := "queue metadata"
	maxSize := int32(1024)

	created, err := client.CreateQueue(ctx, queueName, &admin.CreateQueueOptions{
		Properties: &admin.QueueProperties{
			MaxSizeInMegabytes: &maxSize,
			UserMetadata:       &userMetadata,
		},
	})
	require.NoError(t, err)
	assert.Equal(t, queueName, created.QueueName)
	require.NotNil(t, created.UserMetadata)
	assert.Equal(t, userMetadata, *created.UserMetadata)

	got, err := client.GetQueue(ctx, queueName, nil)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, queueName, got.QueueName)
	require.NotNil(t, got.MaxSizeInMegabytes)
	assert.Equal(t, maxSize, *got.MaxSizeInMegabytes)

	pager := client.NewListQueuesPager(&admin.ListQueuesOptions{MaxPageSize: 1})
	require.True(t, pager.More())
	page, err := pager.NextPage(ctx)
	require.NoError(t, err)
	require.Len(t, page.Queues, 1)
	assert.Equal(t, queueName, page.Queues[0].QueueName)

	_, err = client.DeleteQueue(ctx, queueName, nil)
	require.NoError(t, err)
	got, err = client.GetQueue(ctx, queueName, nil)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestServiceBusAdmin_TopicSubscriptionRuleSDKLifecycle(t *testing.T) {
	client := sbAdminClient(t, "sdk-admin-topic")
	ctx := context.Background()
	topicName := "topic1"
	subName := "sub1"
	ruleName := "important"

	createdTopic, err := client.CreateTopic(ctx, topicName, nil)
	require.NoError(t, err)
	assert.Equal(t, topicName, createdTopic.TopicName)

	createdSub, err := client.CreateSubscription(ctx, topicName, subName, nil)
	require.NoError(t, err)
	assert.Equal(t, subName, createdSub.SubscriptionName)

	subPager := client.NewListSubscriptionsPager(topicName, nil)
	subPage, err := subPager.NextPage(ctx)
	require.NoError(t, err)
	require.Len(t, subPage.Subscriptions, 1)
	assert.Equal(t, subName, subPage.Subscriptions[0].SubscriptionName)

	createdRule, err := client.CreateRule(ctx, topicName, subName, &admin.CreateRuleOptions{
		Name:   to.Ptr(ruleName),
		Filter: &admin.SQLFilter{Expression: "priority = 'high'"},
	})
	require.NoError(t, err)
	assert.Equal(t, ruleName, createdRule.Name)

	rulePager := client.NewListRulesPager(topicName, subName, nil)
	rulePage, err := rulePager.NextPage(ctx)
	require.NoError(t, err)
	require.Len(t, rulePage.Rules, 2)
	assert.ElementsMatch(t, []string{"$Default", ruleName}, []string{rulePage.Rules[0].Name, rulePage.Rules[1].Name})

	_, err = client.DeleteRule(ctx, topicName, subName, ruleName, nil)
	require.NoError(t, err)
	_, err = client.DeleteSubscription(ctx, topicName, subName, nil)
	require.NoError(t, err)
	_, err = client.DeleteTopic(ctx, topicName, nil)
	require.NoError(t, err)
}
