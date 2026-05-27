package azure_cli_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventGridCLI_TopicSubscriptionPublish(t *testing.T) {
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

	topicURL := baseURL + "/subscriptions/" + subscriptionID + "/resourceGroups/" + resourceGroup +
		"/providers/Microsoft.EventGrid/topics/cli-topic?api-version=2021-12-01"
	out := runCLI(t, azRest("PUT", topicURL, `{"location":"eastus","tags":{"env":"test"}}`))
	var topic struct {
		ID         string `json:"id"`
		Properties struct {
			Endpoint string `json:"endpoint"`
		} `json:"properties"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &topic))
	require.NotEmpty(t, topic.ID)
	require.NotEmpty(t, topic.Properties.Endpoint)
	t.Cleanup(func() {
		runCLI(t, azRest("DELETE", topicURL, ""))
	})
	keysURL := baseURL + topic.ID + "/listKeys?api-version=2021-12-01"
	out = runCLI(t, azRest("POST", keysURL, ""))
	var keys struct {
		Key1 string `json:"key1"`
		Key2 string `json:"key2"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &keys))
	assert.Len(t, keys.Key1, 44)
	assert.Len(t, keys.Key2, 44)

	subURL := baseURL + topic.ID + "/providers/Microsoft.EventGrid/eventSubscriptions/cli-sub?api-version=2021-12-01"
	body := `{"properties":{"destination":{"endpointType":"WebHook","properties":{"endpointUrl":"` + hook.URL + `"}},"eventDeliverySchema":"EventGridSchema"}}`
	runCLI(t, azRest("PUT", subURL, body))

	select {
	case <-deliveries:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Event Grid subscription validation delivery")
	}

	listURL := baseURL + topic.ID + "/providers/Microsoft.EventGrid/eventSubscriptions?api-version=2021-12-01"
	out = runCLI(t, azRest("GET", listURL, ""))
	var list struct {
		Value []struct {
			Name string `json:"name"`
		} `json:"value"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &list))
	require.Len(t, list.Value, 1)
	assert.Equal(t, "cli-sub", list.Value[0].Name)

	publishBody := `[{"id":"cli-evt","eventType":"sockerless.cli","subject":"/cli","eventTime":"2026-05-27T00:00:00Z","data":{"ok":true},"dataVersion":"1"}]`
	runCLI(t, azRest("POST", topic.Properties.Endpoint+"?api-version=2018-01-01", publishBody, "--headers", "Content-Type=application/json"))

	select {
	case events := <-deliveries:
		require.Len(t, events, 1)
		assert.Equal(t, "cli-evt", events[0]["id"])
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Event Grid event delivery")
	}
}
