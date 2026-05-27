package azure_sdk_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/eventgrid/armeventgrid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventGrid_TopicSubscriptionPublishSDK(t *testing.T) {
	rg := "sdk-eventgrid-rg"
	ensureRG(t, rg)

	deliveries := make(chan []map[string]any, 4)
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		var events []map[string]any
		require.NoError(t, json.Unmarshal(body, &events))
		deliveries <- events
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(hook.Close)

	cred := &fakeCredential{}
	topics, err := armeventgrid.NewTopicsClient(subscriptionID, cred, clientOpts())
	require.NoError(t, err)
	subs, err := armeventgrid.NewEventSubscriptionsClient(subscriptionID, cred, clientOpts())
	require.NoError(t, err)

	topicName := "sdk-topic"
	topicPoller, err := topics.BeginCreateOrUpdate(ctx, rg, topicName, armeventgrid.Topic{
		Location: to.Ptr("eastus"),
		Tags: map[string]*string{
			"env": to.Ptr("test"),
		},
	}, nil)
	require.NoError(t, err)
	topicResp, err := topicPoller.PollUntilDone(ctx, nil)
	require.NoError(t, err)
	require.NotNil(t, topicResp.Properties)
	require.NotNil(t, topicResp.Properties.Endpoint)
	assert.Contains(t, *topicResp.Properties.Endpoint, "/api/events")
	keysResp, err := topics.ListSharedAccessKeys(ctx, rg, topicName, nil)
	require.NoError(t, err)
	require.NotNil(t, keysResp.Key1)
	require.NotNil(t, keysResp.Key2)
	assert.Len(t, *keysResp.Key1, 44)
	assert.Len(t, *keysResp.Key2, 44)
	t.Cleanup(func() {
		poller, err := topics.BeginDelete(ctx, rg, topicName, nil)
		if err == nil {
			_, _ = poller.PollUntilDone(ctx, nil)
		}
	})

	scope := "/subscriptions/" + subscriptionID + "/resourceGroups/" + rg + "/providers/Microsoft.EventGrid/topics/" + topicName
	subPoller, err := subs.BeginCreateOrUpdate(ctx, scope, "sdk-sub", armeventgrid.EventSubscription{
		Properties: &armeventgrid.EventSubscriptionProperties{
			Destination: &armeventgrid.WebHookEventSubscriptionDestination{
				EndpointType: to.Ptr(armeventgrid.EndpointTypeWebHook),
				Properties: &armeventgrid.WebHookEventSubscriptionDestinationProperties{
					EndpointURL: to.Ptr(hook.URL),
				},
			},
			EventDeliverySchema: to.Ptr(armeventgrid.EventDeliverySchemaEventGridSchema),
		},
	}, nil)
	require.NoError(t, err)
	subResp, err := subPoller.PollUntilDone(ctx, nil)
	require.NoError(t, err)
	require.NotNil(t, subResp.Properties)
	assert.Equal(t, scope, *subResp.Properties.Topic)

	pager := subs.NewListByResourcePager(rg, "Microsoft.EventGrid", "topics", topicName, nil)
	require.True(t, pager.More())
	page, err := pager.NextPage(ctx)
	require.NoError(t, err)
	require.Len(t, page.Value, 1)
	assert.Equal(t, "sdk-sub", *page.Value[0].Name)

	// Drain the subscription-validation event sent during create.
	select {
	case <-deliveries:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Event Grid subscription validation delivery")
	}

	body := `[{"id":"evt-1","eventType":"sockerless.test","subject":"/sdk","eventTime":"2026-05-27T00:00:00Z","data":{"ok":true},"dataVersion":"1"}]`
	req, err := http.NewRequest(http.MethodPost, *topicResp.Properties.Endpoint+"?api-version=2018-01-01", bytes.NewBufferString(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	respBody, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, string(respBody))

	select {
	case events := <-deliveries:
		require.Len(t, events, 1)
		assert.Equal(t, "evt-1", events[0]["id"])
		assert.Equal(t, "sockerless.test", events[0]["eventType"])
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Event Grid event delivery")
	}
}
