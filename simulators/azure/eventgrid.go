package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	sim "github.com/sockerless/simulator"
)

// Microsoft.EventGrid ARM control plane plus custom-topic publish
// data plane. Topics are addressed through ARM; events publish to
// the topic endpoint's /api/events path and synchronously fan out to
// webhook event subscriptions.

type EventGridTopic struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Type       string            `json:"type"`
	Location   string            `json:"location,omitempty"`
	Tags       map[string]string `json:"tags,omitempty"`
	Properties map[string]any    `json:"properties,omitempty"`
}

type EventGridEventSubscription struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	Type       string         `json:"type"`
	Properties map[string]any `json:"properties,omitempty"`
}

var (
	eventGridTopics        sim.Store[EventGridTopic]
	eventGridSubscriptions sim.Store[EventGridEventSubscription]
	eventGridListenersMu   sync.Mutex
	eventGridListeners     = map[string]*eventGridTopicListener{}
)

type eventGridTopicListener struct {
	url    string
	server *http.Server
	ln     net.Listener
}

func registerEventGrid(srv *sim.Server) {
	eventGridTopics = sim.MakeStore[EventGridTopic](srv.DB(), "eventgrid_topics")
	eventGridSubscriptions = sim.MakeStore[EventGridEventSubscription](srv.DB(), "eventgrid_subscriptions")

	const topicsBase = "/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.EventGrid/topics"
	srv.HandleFunc("PUT "+topicsBase+"/{topicName}", handleEventGridCreateTopic)
	srv.HandleFunc("GET "+topicsBase+"/{topicName}", handleEventGridGetTopic)
	srv.HandleFunc("POST "+topicsBase+"/{topicName}/listKeys", handleEventGridListTopicKeys)
	srv.HandleFunc("DELETE "+topicsBase+"/{topicName}", handleEventGridDeleteTopic)
	srv.HandleFunc("GET "+topicsBase, handleEventGridListTopicsByRG)
	srv.HandleFunc("GET /subscriptions/{subscriptionId}/providers/Microsoft.EventGrid/topics", handleEventGridListTopicsBySubscription)

	srv.HandleFunc("PUT "+topicsBase+"/{topicName}/providers/Microsoft.EventGrid/eventSubscriptions/{eventSubscriptionName}", handleEventGridCreateEventSubscription)
	srv.HandleFunc("GET "+topicsBase+"/{topicName}/providers/Microsoft.EventGrid/eventSubscriptions/{eventSubscriptionName}", handleEventGridGetEventSubscription)
	srv.HandleFunc("DELETE "+topicsBase+"/{topicName}/providers/Microsoft.EventGrid/eventSubscriptions/{eventSubscriptionName}", handleEventGridDeleteEventSubscription)
	srv.HandleFunc("GET "+topicsBase+"/{topicName}/providers/Microsoft.EventGrid/eventSubscriptions", handleEventGridListEventSubscriptions)
	srv.HandleFunc("GET "+topicsBase+"/{topicName}/eventSubscriptions", handleEventGridListEventSubscriptions)

	srv.WrapHandler(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			host := r.Host
			if i := strings.LastIndex(host, ":"); i >= 0 {
				host = host[:i]
			}
			if strings.Contains(host, ".eventgrid.") && r.Method == http.MethodPost && strings.TrimRight(r.URL.Path, "/") == "/api/events" {
				handleEventGridPublishEvents(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	})
	srv.HandleFunc("POST /api/events", handleEventGridPublishEvents)
}

func eventGridTopicID(sub, rg, name string) string {
	return fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.EventGrid/topics/%s", sub, rg, name)
}

func eventGridSubscriptionID(topicID, name string) string {
	return topicID + "/providers/Microsoft.EventGrid/eventSubscriptions/" + name
}

func eventGridEndpointHost(r *http.Request, topic string) string {
	hostname, portSuffix := azureRequestHostParts(r)
	if net.ParseIP(hostname) != nil {
		hostname = "localhost"
	}
	return strings.Join([]string{topic, "eventgrid", hostname}, ".") + portSuffix
}

func eventGridTopicWithEndpoint(r *http.Request, topic EventGridTopic) (EventGridTopic, error) {
	props := topic.Properties
	if props == nil {
		props = map[string]any{}
	}
	hostname, _ := azureRequestHostParts(r)
	if isLocalAzureHost(hostname) {
		endpoint, err := ensureEventGridTopicListener(topic)
		if err != nil {
			return EventGridTopic{}, err
		}
		props["endpoint"] = endpoint
	} else {
		props["endpoint"] = fmt.Sprintf("%s://%s/api/events", azureRequestScheme(r), eventGridEndpointHost(r, topic.Name))
	}
	topic.Properties = props
	return topic, nil
}

func isLocalAzureHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func ensureEventGridTopicListener(topic EventGridTopic) (string, error) {
	eventGridListenersMu.Lock()
	defer eventGridListenersMu.Unlock()
	if existing := eventGridListeners[topic.ID]; existing != nil {
		return existing.url, nil
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	egSrv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost || strings.TrimRight(r.URL.Path, "/") != "/api/events" {
				sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "unknown Event Grid data-plane path %q", r.URL.Path)
				return
			}
			publishEventGridTopic(w, r, topic)
		}),
	}
	listener := &eventGridTopicListener{
		url:    "http://127.0.0.1:" + strconv.Itoa(ln.Addr().(*net.TCPAddr).Port) + "/api/events",
		server: egSrv,
		ln:     ln,
	}
	eventGridListeners[topic.ID] = listener
	go func() {
		_ = egSrv.Serve(ln)
	}()
	return listener.url, nil
}

func closeEventGridTopicListener(topicID string) {
	eventGridListenersMu.Lock()
	listener := eventGridListeners[topicID]
	delete(eventGridListeners, topicID)
	eventGridListenersMu.Unlock()
	if listener != nil {
		_ = listener.server.Close()
	}
}

func handleEventGridCreateTopic(w http.ResponseWriter, r *http.Request) {
	sub := sim.PathParam(r, "subscriptionId")
	rg := sim.PathParam(r, "resourceGroupName")
	name := sim.PathParam(r, "topicName")
	var req EventGridTopic
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AzureErrorf(w, "InvalidRequestContent", http.StatusBadRequest, "invalid request body: %v", err)
		return
	}
	id := eventGridTopicID(sub, rg, name)
	props := req.Properties
	if props == nil {
		props = map[string]any{}
	}
	props["provisioningState"] = "Succeeded"
	if _, ok := props["inputSchema"]; !ok {
		props["inputSchema"] = "EventGridSchema"
	}
	topic := EventGridTopic{
		ID:         id,
		Name:       name,
		Type:       "Microsoft.EventGrid/topics",
		Location:   req.Location,
		Tags:       req.Tags,
		Properties: props,
	}
	topic, err := eventGridTopicWithEndpoint(r, topic)
	if err != nil {
		sim.AzureErrorf(w, "InternalError", http.StatusInternalServerError, "failed to allocate Event Grid topic endpoint: %v", err)
		return
	}
	eventGridTopics.Put(id, topic)
	sim.WriteJSON(w, http.StatusCreated, topic)
}

func handleEventGridGetTopic(w http.ResponseWriter, r *http.Request) {
	id := eventGridTopicID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "topicName"))
	topic, ok := eventGridTopics.Get(id)
	if !ok {
		sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "topic %q not found", id)
		return
	}
	topic, err := eventGridTopicWithEndpoint(r, topic)
	if err != nil {
		sim.AzureErrorf(w, "InternalError", http.StatusInternalServerError, "failed to allocate Event Grid topic endpoint: %v", err)
		return
	}
	eventGridTopics.Put(id, topic)
	sim.WriteJSON(w, http.StatusOK, topic)
}

func handleEventGridListTopicKeys(w http.ResponseWriter, r *http.Request) {
	id := eventGridTopicID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "topicName"))
	if _, ok := eventGridTopics.Get(id); !ok {
		sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "topic %q not found", id)
		return
	}
	sim.WriteJSON(w, http.StatusOK, map[string]string{
		"key1": simListKey32(id, "key1"),
		"key2": simListKey32(id, "key2"),
	})
}

func handleEventGridDeleteTopic(w http.ResponseWriter, r *http.Request) {
	id := eventGridTopicID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "topicName"))
	if !eventGridTopics.Delete(id) {
		sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "topic %q not found", id)
		return
	}
	closeEventGridTopicListener(id)
	for _, sub := range eventGridSubscriptions.List() {
		if strings.HasPrefix(sub.ID, id+"/providers/Microsoft.EventGrid/eventSubscriptions/") {
			eventGridSubscriptions.Delete(sub.ID)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func handleEventGridListTopicsByRG(w http.ResponseWriter, r *http.Request) {
	sub := sim.PathParam(r, "subscriptionId")
	rg := sim.PathParam(r, "resourceGroupName")
	prefix := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.EventGrid/topics/", sub, rg)
	out := make([]EventGridTopic, 0)
	for _, topic := range eventGridTopics.List() {
		if strings.HasPrefix(topic.ID, prefix) {
			topic, err := eventGridTopicWithEndpoint(r, topic)
			if err != nil {
				sim.AzureErrorf(w, "InternalError", http.StatusInternalServerError, "failed to allocate Event Grid topic endpoint: %v", err)
				return
			}
			out = append(out, topic)
		}
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"value": out})
}

func handleEventGridListTopicsBySubscription(w http.ResponseWriter, r *http.Request) {
	sub := sim.PathParam(r, "subscriptionId")
	prefix := fmt.Sprintf("/subscriptions/%s/resourceGroups/", sub)
	out := make([]EventGridTopic, 0)
	for _, topic := range eventGridTopics.List() {
		if strings.HasPrefix(topic.ID, prefix) {
			topic, err := eventGridTopicWithEndpoint(r, topic)
			if err != nil {
				sim.AzureErrorf(w, "InternalError", http.StatusInternalServerError, "failed to allocate Event Grid topic endpoint: %v", err)
				return
			}
			out = append(out, topic)
		}
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"value": out})
}

func handleEventGridCreateEventSubscription(w http.ResponseWriter, r *http.Request) {
	topicID := eventGridTopicID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "topicName"))
	if _, ok := eventGridTopics.Get(topicID); !ok {
		sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "topic %q not found", topicID)
		return
	}
	name := sim.PathParam(r, "eventSubscriptionName")
	var req EventGridEventSubscription
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AzureErrorf(w, "InvalidRequestContent", http.StatusBadRequest, "invalid request body: %v", err)
		return
	}
	props := req.Properties
	if props == nil {
		props = map[string]any{}
	}
	props["provisioningState"] = "Succeeded"
	props["topic"] = topicID
	es := EventGridEventSubscription{
		ID:         eventGridSubscriptionID(topicID, name),
		Name:       name,
		Type:       "Microsoft.EventGrid/eventSubscriptions",
		Properties: props,
	}
	eventGridSubscriptions.Put(es.ID, es)
	deliverEventGridValidation(es)
	sim.WriteJSON(w, http.StatusCreated, es)
}

func handleEventGridGetEventSubscription(w http.ResponseWriter, r *http.Request) {
	id := eventGridSubscriptionID(
		eventGridTopicID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "topicName")),
		sim.PathParam(r, "eventSubscriptionName"),
	)
	es, ok := eventGridSubscriptions.Get(id)
	if !ok {
		sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "event subscription %q not found", id)
		return
	}
	sim.WriteJSON(w, http.StatusOK, es)
}

func handleEventGridDeleteEventSubscription(w http.ResponseWriter, r *http.Request) {
	id := eventGridSubscriptionID(
		eventGridTopicID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "topicName")),
		sim.PathParam(r, "eventSubscriptionName"),
	)
	if !eventGridSubscriptions.Delete(id) {
		sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "event subscription %q not found", id)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func handleEventGridListEventSubscriptions(w http.ResponseWriter, r *http.Request) {
	topicID := eventGridTopicID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "topicName"))
	prefix := topicID + "/providers/Microsoft.EventGrid/eventSubscriptions/"
	out := make([]EventGridEventSubscription, 0)
	for _, es := range eventGridSubscriptions.List() {
		if strings.HasPrefix(es.ID, prefix) {
			out = append(out, es)
		}
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"value": out})
}

func handleEventGridPublishEvents(w http.ResponseWriter, r *http.Request) {
	topic, ok := eventGridTopicFromHost(r.Host)
	if !ok {
		sim.AzureErrorf(w, "TopicNotFound", http.StatusNotFound, "event grid topic host %q not found", r.Host)
		return
	}
	publishEventGridTopic(w, r, topic)
}

func publishEventGridTopic(w http.ResponseWriter, r *http.Request, topic EventGridTopic) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		sim.AzureErrorf(w, "InvalidRequestContent", http.StatusBadRequest, "failed to read request body: %v", err)
		return
	}
	for _, es := range eventGridSubscriptions.List() {
		if !eventGridSubscriptionBelongsToTopic(es, topic.ID) {
			continue
		}
		if endpoint := eventGridWebhookEndpoint(es); endpoint != "" {
			_, _ = http.Post(endpoint, "application/json", bytes.NewReader(body))
		}
	}
	w.WriteHeader(http.StatusOK)
}

func eventGridSubscriptionBelongsToTopic(es EventGridEventSubscription, topicID string) bool {
	if strings.HasPrefix(es.ID, topicID+"/providers/Microsoft.EventGrid/eventSubscriptions/") {
		return true
	}
	if es.Properties != nil && es.Properties["topic"] == topicID {
		return true
	}
	return false
}

func eventGridTopicFromHost(host string) (EventGridTopic, bool) {
	hostname := host
	if i := strings.LastIndex(hostname, ":"); i >= 0 {
		hostname = hostname[:i]
	}
	name := strings.Split(hostname, ".")[0]
	for _, topic := range eventGridTopics.List() {
		if topic.Name == name {
			return topic, true
		}
	}
	return EventGridTopic{}, false
}

func eventGridWebhookEndpoint(es EventGridEventSubscription) string {
	dest, ok := es.Properties["destination"].(map[string]any)
	if !ok {
		return ""
	}
	props, ok := dest["properties"].(map[string]any)
	if !ok {
		return ""
	}
	if endpoint, _ := props["endpointUrl"].(string); endpoint != "" {
		return endpoint
	}
	if endpoint, _ := props["endpointBaseUrl"].(string); endpoint != "" {
		return endpoint
	}
	return ""
}

func deliverEventGridValidation(es EventGridEventSubscription) {
	endpoint := eventGridWebhookEndpoint(es)
	if endpoint == "" {
		return
	}
	event := []map[string]any{{
		"id":        generateUUID(),
		"eventType": "Microsoft.EventGrid.SubscriptionValidationEvent",
		"subject":   "",
		"eventTime": time.Now().UTC().Format(time.RFC3339Nano),
		"data": map[string]any{
			"validationCode": generateUUID(),
			"validationUrl":  endpoint,
		},
		"dataVersion": "1",
	}}
	payload, _ := json.Marshal(event)
	_, _ = http.Post(endpoint, "application/json", bytes.NewReader(payload))
}
