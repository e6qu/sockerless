package azure_sdk_test

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sbReq points at the Service Bus REST data plane subdomain
// (`{namespace}.servicebus.<host>`).
func sbReq(t *testing.T, method, namespace, path string, body []byte, headers map[string]string) *http.Response {
	t.Helper()
	var br io.Reader
	if body != nil {
		br = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, baseURL+path, br)
	require.NoError(t, err)
	req.Host = namespace + ".servicebus.localhost"
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

// TestServiceBus_QueueRESTRoundTrip exercises the canonical
// SendMessage → ReceiveAndDelete flow with real status codes.
func TestServiceBus_QueueRESTRoundTrip(t *testing.T) {
	ns := "ns1"
	queue := "myqueue"

	// SendMessage must return 201, not 200.
	resp := sbReq(t, "POST", ns, "/"+queue+"/messages", []byte("hello sb body"),
		map[string]string{
			"Content-Type":     "application/atom+xml;type=entry;charset=utf-8",
			"BrokerProperties": `{"Label":"l1"}`,
		})
	require.Equal(t, http.StatusCreated, resp.StatusCode,
		"SendMessage must return 201 Created")
	resp.Body.Close()

	// ReceiveAndDelete on a queue with messages must return 200 + body.
	resp = sbReq(t, "DELETE", ns, "/"+queue+"/messages/head", nil, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode,
		"ReceiveAndDelete with messages must return 200")
	got, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	assert.Equal(t, "hello sb body", string(got),
		"received body must match sent body")
	brokerProps := resp.Header.Get("BrokerProperties")
	assert.NotEmpty(t, brokerProps, "ReceiveAndDelete must carry BrokerProperties header")
	assert.Contains(t, brokerProps, `"MessageId":`)

	// ReceiveAndDelete on empty queue must return 204.
	resp = sbReq(t, "DELETE", ns, "/"+queue+"/messages/head", nil, nil)
	require.Equal(t, http.StatusNoContent, resp.StatusCode,
		"ReceiveAndDelete on empty queue must return 204")
	resp.Body.Close()
}

// TestServiceBus_PeekLockComplete exercises PeekLock → CompleteLock
// (the read-with-lock-then-ack flow that gives at-least-once delivery
// guarantees in real Service Bus).
func TestServiceBus_PeekLockComplete(t *testing.T) {
	ns := "ns2"
	queue := "lockqueue"

	// Send a message.
	resp := sbReq(t, "POST", ns, "/"+queue+"/messages", []byte("locked body"), nil)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	// PeekLock must return 201 + body + Location header pointing at
	// the lock-token URL.
	resp = sbReq(t, "POST", ns, "/"+queue+"/messages/head", nil, nil)
	require.Equal(t, http.StatusCreated, resp.StatusCode,
		"PeekLock with messages must return 201")
	got, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	assert.Equal(t, "locked body", string(got))
	location := resp.Header.Get("Location")
	require.NotEmpty(t, location, "PeekLock must carry Location header")

	// Same message must NOT be returned by a second PeekLock (it's locked).
	resp = sbReq(t, "POST", ns, "/"+queue+"/messages/head", nil, nil)
	require.Equal(t, http.StatusNoContent, resp.StatusCode,
		"PeekLock on a locked queue must return 204 (no other unlocked messages)")
	resp.Body.Close()

	// CompleteLock follows the Location header. The sim emits an
	// absolute URL with the request Host (`{ns}.servicebus.<host>`)
	// — Azure SDK clients resolve that via DNS in production. The
	// test harness has no DNS for `*.servicebus.localhost`, so it
	// resolves the Location to the same connection target as
	// sbReq (baseURL) while preserving the namespace-prefixed Host
	// header. Path + query come from Location verbatim.
	locURL, err := url.Parse(location)
	require.NoError(t, err)
	require.Equal(t, ns+".servicebus.localhost", locURL.Host,
		"Location must point at the namespace subdomain host")
	require.NotEmpty(t, locURL.Path, "Location path must not be empty")
	require.Contains(t, locURL.Path, "/messages/",
		"Location must include the /messages/{id}/{token} path")
	completeReq, err := http.NewRequest("DELETE", baseURL+locURL.RequestURI(), nil)
	require.NoError(t, err)
	completeReq.Host = locURL.Host
	completeResp, err := http.DefaultClient.Do(completeReq)
	require.NoError(t, err)
	completeResp.Body.Close()
	require.Equal(t, http.StatusNoContent, completeResp.StatusCode,
		"CompleteLock must return 204")

	// Queue is now empty.
	resp = sbReq(t, "DELETE", ns, "/"+queue+"/messages/head", nil, nil)
	require.Equal(t, http.StatusNoContent, resp.StatusCode,
		"Queue should be empty after CompleteLock")
	resp.Body.Close()
}

// TestServiceBus_TopicSubscriptionRoundTrip exercises the topic +
// subscription variant: POST /{topic}/messages then DELETE
// /{topic}/subscriptions/{sub}/messages/head.
func TestServiceBus_TopicSubscriptionRoundTrip(t *testing.T) {
	ns := "ns3"
	topic := "mytopic"
	sub := "mysub"

	// Send to topic.
	resp := sbReq(t, "POST", ns, "/"+topic+"/subscriptions/"+sub+"/messages",
		[]byte("topic body"), nil)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	// Subscription Receive.
	resp = sbReq(t, "DELETE", ns,
		"/"+topic+"/subscriptions/"+sub+"/messages/head", nil, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	got, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	assert.Equal(t, "topic body", string(got))

	// Empty after consumption.
	resp = sbReq(t, "DELETE", ns,
		"/"+topic+"/subscriptions/"+sub+"/messages/head", nil, nil)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	resp.Body.Close()
}
