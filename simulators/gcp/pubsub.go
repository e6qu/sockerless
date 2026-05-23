package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	sim "github.com/sockerless/simulator"
)

// Cloud Pub/Sub v1 — REST surface scoped to topic + subscription
// CRUD plus publish / pull / acknowledge / modifyAckDeadline. The
// real API exposes ~30 ops; sim implements the load-bearing slice
// for fan-out / fan-in integration tests. No LRO — every op
// returns synchronously.
//
// Wire paths (per discovery doc, https://pubsub.googleapis.com/$discovery/rest):
//
//	PUT    /v1/projects/{p}/topics/{t}                            CreateTopic
//	GET    /v1/projects/{p}/topics/{t}                            GetTopic
//	GET    /v1/projects/{p}/topics                                ListTopics
//	DELETE /v1/projects/{p}/topics/{t}                            DeleteTopic
//	POST   /v1/projects/{p}/topics/{t}:publish                    Publish
//	PUT    /v1/projects/{p}/subscriptions/{s}                     CreateSubscription
//	GET    /v1/projects/{p}/subscriptions/{s}                     GetSubscription
//	GET    /v1/projects/{p}/subscriptions                         ListSubscriptions
//	DELETE /v1/projects/{p}/subscriptions/{s}                     DeleteSubscription
//	POST   /v1/projects/{p}/subscriptions/{s}:pull                Pull
//	POST   /v1/projects/{p}/subscriptions/{s}:acknowledge         Acknowledge
//	POST   /v1/projects/{p}/subscriptions/{s}:modifyAckDeadline   ModifyAckDeadline
//
// The `:verb` suffix dispatch uses the same pattern as
// `simulators/gcp/secretmanager.go`: register one POST route with
// a wildcard, then strip-and-switch inside the handler.

type PSTopic struct {
	Name   string            `json:"name"` // projects/{p}/topics/{t}
	Labels map[string]string `json:"labels,omitempty"`
}

type PSSubscription struct {
	Name               string            `json:"name"` // projects/{p}/subscriptions/{s}
	Topic              string            `json:"topic"`
	AckDeadlineSeconds int               `json:"ackDeadlineSeconds,omitempty"`
	Labels             map[string]string `json:"labels,omitempty"`
	PushConfig         *PSPushConfig     `json:"pushConfig,omitempty"`
}

type PSPushConfig struct {
	PushEndpoint string `json:"pushEndpoint,omitempty"`
}

type PSMessage struct {
	MessageId   string            `json:"messageId"`
	PublishTime string            `json:"publishTime"`
	Data        string            `json:"data,omitempty"` // base64 (per API)
	Attributes  map[string]string `json:"attributes,omitempty"`
}

// PSDeliveredMessage tracks an in-flight pulled message awaiting
// acknowledge. AckId is unique per pull.
type PSDeliveredMessage struct {
	AckId        string
	Subscription string
	Message      PSMessage
	DeliveredAt  time.Time
	AckDeadline  time.Time
}

var (
	psTopics        sim.Store[PSTopic]
	psSubscriptions sim.Store[PSSubscription]
	// Per-subscription queues (FIFO, in-memory).
	psQueues sim.Store[psQueue]
	// In-flight pulled messages keyed by ackId.
	psInFlight sim.Store[PSDeliveredMessage]
)

// psQueue holds the pending messages for a subscription. Wrapping
// in a struct so it round-trips through the JSON-serializing Store.
type psQueue struct {
	Subscription string
	Messages     []PSMessage
}

func registerPubSub(srv *sim.Server) {
	psTopics = sim.MakeStore[PSTopic](srv.DB(), "pubsub_topics")
	psSubscriptions = sim.MakeStore[PSSubscription](srv.DB(), "pubsub_subscriptions")
	psQueues = sim.MakeStore[psQueue](srv.DB(), "pubsub_queues")
	psInFlight = sim.MakeStore[PSDeliveredMessage](srv.DB(), "pubsub_inflight")

	// Topics.
	srv.HandleFunc("PUT /v1/projects/{project}/topics/{topic}", handlePSCreateTopic)
	srv.HandleFunc("GET /v1/projects/{project}/topics/{topic}", handlePSGetTopic)
	srv.HandleFunc("GET /v1/projects/{project}/topics", handlePSListTopics)
	srv.HandleFunc("DELETE /v1/projects/{project}/topics/{topic}", handlePSDeleteTopic)
	srv.HandleFunc("POST /v1/projects/{project}/topics/{topicVerb}", handlePSTopicVerb)

	// Subscriptions.
	srv.HandleFunc("PUT /v1/projects/{project}/subscriptions/{sub}", handlePSCreateSubscription)
	srv.HandleFunc("GET /v1/projects/{project}/subscriptions/{sub}", handlePSGetSubscription)
	srv.HandleFunc("GET /v1/projects/{project}/subscriptions", handlePSListSubscriptions)
	srv.HandleFunc("DELETE /v1/projects/{project}/subscriptions/{sub}", handlePSDeleteSubscription)
	srv.HandleFunc("POST /v1/projects/{project}/subscriptions/{subVerb}", handlePSSubscriptionVerb)
}

func psTopicName(project, topic string) string {
	return fmt.Sprintf("projects/%s/topics/%s", project, topic)
}
func psSubName(project, sub string) string {
	return fmt.Sprintf("projects/%s/subscriptions/%s", project, sub)
}

func handlePSCreateTopic(w http.ResponseWriter, r *http.Request) {
	project := sim.PathParam(r, "project")
	topic := sim.PathParam(r, "topic")
	var req PSTopic
	if err := sim.ReadJSON(r, &req); err != nil {
		gcpError(w, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error())
		return
	}
	t := PSTopic{
		Name:   psTopicName(project, topic),
		Labels: req.Labels,
	}
	psTopics.Put(t.Name, t)
	sim.WriteJSON(w, http.StatusOK, t)
}

func handlePSGetTopic(w http.ResponseWriter, r *http.Request) {
	name := psTopicName(sim.PathParam(r, "project"), sim.PathParam(r, "topic"))
	t, ok := psTopics.Get(name)
	if !ok {
		gcpError(w, http.StatusNotFound, "NOT_FOUND", "Topic not found: "+name)
		return
	}
	sim.WriteJSON(w, http.StatusOK, t)
}

func handlePSListTopics(w http.ResponseWriter, r *http.Request) {
	project := sim.PathParam(r, "project")
	prefix := fmt.Sprintf("projects/%s/topics/", project)
	var out []PSTopic
	for _, t := range psTopics.List() {
		if strings.HasPrefix(t.Name, prefix) {
			out = append(out, t)
		}
	}
	if out == nil {
		out = []PSTopic{}
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"topics": out})
}

func handlePSDeleteTopic(w http.ResponseWriter, r *http.Request) {
	name := psTopicName(sim.PathParam(r, "project"), sim.PathParam(r, "topic"))
	if !psTopics.Delete(name) {
		gcpError(w, http.StatusNotFound, "NOT_FOUND", "Topic not found: "+name)
		return
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{})
}

func handlePSTopicVerb(w http.ResponseWriter, r *http.Request) {
	tv := sim.PathParam(r, "topicVerb")
	parts := strings.SplitN(tv, ":", 2)
	if len(parts) != 2 {
		gcpError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "Expected <topic>:<verb>")
		return
	}
	topic, verb := parts[0], parts[1]
	switch verb {
	case "publish":
		handlePSPublish(w, r, sim.PathParam(r, "project"), topic)
	default:
		gcpError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "Unknown verb: "+verb)
	}
}

func handlePSPublish(w http.ResponseWriter, r *http.Request, project, topic string) {
	tName := psTopicName(project, topic)
	if _, ok := psTopics.Get(tName); !ok {
		gcpError(w, http.StatusNotFound, "NOT_FOUND", "Topic not found: "+tName)
		return
	}
	var req struct {
		Messages []PSMessage `json:"messages"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		gcpError(w, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error())
		return
	}
	var msgIds []string
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, m := range req.Messages {
		msgID := generateUUIDLocal()
		m.MessageId = msgID
		m.PublishTime = now
		msgIds = append(msgIds, msgID)
		// Fan out to every subscription on this topic.
		for _, sub := range psSubscriptions.List() {
			if sub.Topic != tName {
				continue
			}
			psQueues.Update(sub.Name, func(q *psQueue) {
				q.Subscription = sub.Name
				q.Messages = append(q.Messages, m)
			})
			// Ensure the queue entry exists (Update is a no-op
			// when the key isn't present in the store).
			if _, ok := psQueues.Get(sub.Name); !ok {
				psQueues.Put(sub.Name, psQueue{Subscription: sub.Name, Messages: []PSMessage{m}})
			}
		}
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"messageIds": msgIds})
}

func handlePSCreateSubscription(w http.ResponseWriter, r *http.Request) {
	project := sim.PathParam(r, "project")
	sub := sim.PathParam(r, "sub")
	var req PSSubscription
	if err := sim.ReadJSON(r, &req); err != nil {
		gcpError(w, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error())
		return
	}
	if req.Topic == "" {
		gcpError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "topic is required")
		return
	}
	if _, ok := psTopics.Get(req.Topic); !ok {
		gcpError(w, http.StatusNotFound, "NOT_FOUND", "Topic not found: "+req.Topic)
		return
	}
	if req.AckDeadlineSeconds == 0 {
		req.AckDeadlineSeconds = 10
	}
	s := PSSubscription{
		Name:               psSubName(project, sub),
		Topic:              req.Topic,
		AckDeadlineSeconds: req.AckDeadlineSeconds,
		Labels:             req.Labels,
		PushConfig:         req.PushConfig,
	}
	psSubscriptions.Put(s.Name, s)
	sim.WriteJSON(w, http.StatusOK, s)
}

func handlePSGetSubscription(w http.ResponseWriter, r *http.Request) {
	name := psSubName(sim.PathParam(r, "project"), sim.PathParam(r, "sub"))
	s, ok := psSubscriptions.Get(name)
	if !ok {
		gcpError(w, http.StatusNotFound, "NOT_FOUND", "Subscription not found: "+name)
		return
	}
	sim.WriteJSON(w, http.StatusOK, s)
}

func handlePSListSubscriptions(w http.ResponseWriter, r *http.Request) {
	project := sim.PathParam(r, "project")
	prefix := fmt.Sprintf("projects/%s/subscriptions/", project)
	var out []PSSubscription
	for _, s := range psSubscriptions.List() {
		if strings.HasPrefix(s.Name, prefix) {
			out = append(out, s)
		}
	}
	if out == nil {
		out = []PSSubscription{}
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"subscriptions": out})
}

func handlePSDeleteSubscription(w http.ResponseWriter, r *http.Request) {
	name := psSubName(sim.PathParam(r, "project"), sim.PathParam(r, "sub"))
	if !psSubscriptions.Delete(name) {
		gcpError(w, http.StatusNotFound, "NOT_FOUND", "Subscription not found: "+name)
		return
	}
	psQueues.Delete(name)
	sim.WriteJSON(w, http.StatusOK, map[string]any{})
}

func handlePSSubscriptionVerb(w http.ResponseWriter, r *http.Request) {
	sv := sim.PathParam(r, "subVerb")
	parts := strings.SplitN(sv, ":", 2)
	if len(parts) != 2 {
		gcpError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "Expected <sub>:<verb>")
		return
	}
	sub, verb := parts[0], parts[1]
	name := psSubName(sim.PathParam(r, "project"), sub)
	switch verb {
	case "pull":
		handlePSPull(w, r, name)
	case "acknowledge":
		handlePSAck(w, r, name)
	case "modifyAckDeadline":
		handlePSModifyAck(w, r, name)
	default:
		gcpError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "Unknown verb: "+verb)
	}
}

func handlePSPull(w http.ResponseWriter, r *http.Request, subName string) {
	sub, ok := psSubscriptions.Get(subName)
	if !ok {
		gcpError(w, http.StatusNotFound, "NOT_FOUND", "Subscription not found: "+subName)
		return
	}
	var req struct {
		MaxMessages int `json:"maxMessages"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		gcpError(w, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error())
		return
	}
	if req.MaxMessages <= 0 {
		req.MaxMessages = 1
	}
	q, _ := psQueues.Get(subName)
	if len(q.Messages) == 0 {
		sim.WriteJSON(w, http.StatusOK, map[string]any{"receivedMessages": []any{}})
		return
	}
	n := req.MaxMessages
	if n > len(q.Messages) {
		n = len(q.Messages)
	}
	picked := q.Messages[:n]
	rest := q.Messages[n:]
	q.Messages = rest
	psQueues.Put(subName, q)

	now := time.Now()
	deadline := now.Add(time.Duration(sub.AckDeadlineSeconds) * time.Second)
	out := make([]map[string]any, 0, n)
	for _, m := range picked {
		ackID := generateUUIDLocal()
		psInFlight.Put(ackID, PSDeliveredMessage{
			AckId:        ackID,
			Subscription: subName,
			Message:      m,
			DeliveredAt:  now,
			AckDeadline:  deadline,
		})
		out = append(out, map[string]any{
			"ackId":   ackID,
			"message": m,
		})
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"receivedMessages": out})
}

func handlePSAck(w http.ResponseWriter, r *http.Request, subName string) {
	if _, ok := psSubscriptions.Get(subName); !ok {
		gcpError(w, http.StatusNotFound, "NOT_FOUND", "Subscription not found: "+subName)
		return
	}
	var req struct {
		AckIds []string `json:"ackIds"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		gcpError(w, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error())
		return
	}
	for _, id := range req.AckIds {
		psInFlight.Delete(id)
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{})
}

func handlePSModifyAck(w http.ResponseWriter, r *http.Request, subName string) {
	if _, ok := psSubscriptions.Get(subName); !ok {
		gcpError(w, http.StatusNotFound, "NOT_FOUND", "Subscription not found: "+subName)
		return
	}
	var req struct {
		AckIds             []string `json:"ackIds"`
		AckDeadlineSeconds int      `json:"ackDeadlineSeconds"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		gcpError(w, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error())
		return
	}
	now := time.Now()
	for _, id := range req.AckIds {
		psInFlight.Update(id, func(m *PSDeliveredMessage) {
			m.AckDeadline = now.Add(time.Duration(req.AckDeadlineSeconds) * time.Second)
		})
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{})
}

// gcpError mirrors the canonical Google API error envelope.
func gcpError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	fmt.Fprintf(w, `{"error":{"code":%d,"message":%q,"status":%q}}`, status, message, code)
}

// generateUUIDLocal is a Pub/Sub-scoped UUID helper that produces
// short opaque IDs for messageId / ackId. Independent of the GCS
// generateUUID helper to avoid a cross-file dependency tangle.
func generateUUIDLocal() string {
	return fmt.Sprintf("%d-%s", time.Now().UnixNano(), randHex(8))
}

func randHex(n int) string {
	const hexChars = "0123456789abcdef"
	out := make([]byte, n)
	t := time.Now().UnixNano()
	for i := 0; i < n; i++ {
		out[i] = hexChars[(t>>uint(i*4))&0xf]
	}
	return string(out)
}
