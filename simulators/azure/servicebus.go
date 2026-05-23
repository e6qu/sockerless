package main

import (
	"fmt"
	"net/http"
	"strings"

	sim "github.com/sockerless/simulator"
)

// Microsoft.ServiceBus ARM control plane. Real Azure exposes
// namespace + queue + topic + subscription + rule + auth-rule
// CRUD. The sim implements the namespace + queue + topic +
// subscription slice — sufficient for terraform-provider-azurerm
// `azurerm_servicebus_*` resources. AMQP data plane out of scope
// for the first cut; the REST data plane (Send / Receive /
// Peek-Lock) can land in a follow-up when an integration test
// surfaces a need.

type SBNamespace struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Type       string            `json:"type"`
	Location   string            `json:"location,omitempty"`
	Sku        map[string]any    `json:"sku,omitempty"`
	Properties map[string]any    `json:"properties,omitempty"`
	Tags       map[string]string `json:"tags,omitempty"`
}

type SBQueue struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	Type       string         `json:"type"`
	Properties map[string]any `json:"properties,omitempty"`
}

type SBTopic struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	Type       string         `json:"type"`
	Properties map[string]any `json:"properties,omitempty"`
}

type SBSubscription struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	Type       string         `json:"type"`
	Properties map[string]any `json:"properties,omitempty"`
}

var (
	sbNamespaces    sim.Store[SBNamespace]
	sbQueues        sim.Store[SBQueue]
	sbTopics        sim.Store[SBTopic]
	sbSubscriptions sim.Store[SBSubscription]
)

func registerServiceBus(srv *sim.Server) {
	sbNamespaces = sim.MakeStore[SBNamespace](srv.DB(), "sb_namespaces")
	sbQueues = sim.MakeStore[SBQueue](srv.DB(), "sb_queues")
	sbTopics = sim.MakeStore[SBTopic](srv.DB(), "sb_topics")
	sbSubscriptions = sim.MakeStore[SBSubscription](srv.DB(), "sb_subscriptions")

	const ns = "/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.ServiceBus/namespaces"

	srv.HandleFunc("PUT "+ns+"/{name}", handleSBCreateNamespace)
	srv.HandleFunc("GET "+ns+"/{name}", handleSBGetNamespace)
	srv.HandleFunc("DELETE "+ns+"/{name}", handleSBDeleteNamespace)
	srv.HandleFunc("GET "+ns, handleSBListNamespacesByRG)

	srv.HandleFunc("PUT "+ns+"/{name}/queues/{queue}", handleSBCreateQueue)
	srv.HandleFunc("GET "+ns+"/{name}/queues/{queue}", handleSBGetQueue)
	srv.HandleFunc("DELETE "+ns+"/{name}/queues/{queue}", handleSBDeleteQueue)
	srv.HandleFunc("GET "+ns+"/{name}/queues", handleSBListQueues)

	srv.HandleFunc("PUT "+ns+"/{name}/topics/{topic}", handleSBCreateTopic)
	srv.HandleFunc("GET "+ns+"/{name}/topics/{topic}", handleSBGetTopic)
	srv.HandleFunc("DELETE "+ns+"/{name}/topics/{topic}", handleSBDeleteTopic)
	srv.HandleFunc("GET "+ns+"/{name}/topics", handleSBListTopics)

	srv.HandleFunc("PUT "+ns+"/{name}/topics/{topic}/subscriptions/{sub}", handleSBCreateSubscription)
	srv.HandleFunc("GET "+ns+"/{name}/topics/{topic}/subscriptions/{sub}", handleSBGetSubscription)
	srv.HandleFunc("DELETE "+ns+"/{name}/topics/{topic}/subscriptions/{sub}", handleSBDeleteSubscription)
	srv.HandleFunc("GET "+ns+"/{name}/topics/{topic}/subscriptions", handleSBListSubscriptions)
}

func sbNamespaceID(sub, rg, name string) string {
	return fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.ServiceBus/namespaces/%s", sub, rg, name)
}

func handleSBCreateNamespace(w http.ResponseWriter, r *http.Request) {
	sub := sim.PathParam(r, "subscriptionId")
	rg := sim.PathParam(r, "resourceGroupName")
	name := sim.PathParam(r, "name")
	var req SBNamespace
	_ = sim.ReadJSON(r, &req)
	id := sbNamespaceID(sub, rg, name)
	n := SBNamespace{
		ID:       id,
		Name:     name,
		Type:     "Microsoft.ServiceBus/namespaces",
		Location: req.Location,
		Sku:      req.Sku,
		Tags:     req.Tags,
		Properties: map[string]any{
			"provisioningState":   "Succeeded",
			"serviceBusEndpoint":  "https://" + name + ".servicebus.windows.net:443/",
		},
	}
	if req.Properties != nil {
		for k, v := range req.Properties {
			n.Properties[k] = v
		}
		n.Properties["provisioningState"] = "Succeeded"
	}
	if n.Sku == nil {
		n.Sku = map[string]any{"name": "Standard", "tier": "Standard"}
	}
	sbNamespaces.Put(id, n)
	sim.WriteJSON(w, http.StatusOK, n)
}

func handleSBGetNamespace(w http.ResponseWriter, r *http.Request) {
	id := sbNamespaceID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "name"))
	n, ok := sbNamespaces.Get(id)
	if !ok {
		sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "namespace not found")
		return
	}
	sim.WriteJSON(w, http.StatusOK, n)
}

func handleSBDeleteNamespace(w http.ResponseWriter, r *http.Request) {
	id := sbNamespaceID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "name"))
	if !sbNamespaces.Delete(id) {
		sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "namespace not found")
		return
	}
	prefix := id + "/"
	for _, q := range sbQueues.List() {
		if strings.HasPrefix(q.ID, prefix) {
			sbQueues.Delete(q.ID)
		}
	}
	for _, t := range sbTopics.List() {
		if strings.HasPrefix(t.ID, prefix) {
			sbTopics.Delete(t.ID)
		}
	}
	for _, s := range sbSubscriptions.List() {
		if strings.HasPrefix(s.ID, prefix) {
			sbSubscriptions.Delete(s.ID)
		}
	}
	w.WriteHeader(http.StatusAccepted)
}

func handleSBListNamespacesByRG(w http.ResponseWriter, r *http.Request) {
	prefix := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.ServiceBus/namespaces/",
		sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"))
	var out []SBNamespace
	for _, n := range sbNamespaces.List() {
		if strings.HasPrefix(n.ID, prefix) {
			out = append(out, n)
		}
	}
	if out == nil {
		out = []SBNamespace{}
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"value": out})
}

func handleSBCreateQueue(w http.ResponseWriter, r *http.Request) {
	parent := sbNamespaceID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "name"))
	if _, ok := sbNamespaces.Get(parent); !ok {
		sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "namespace not found")
		return
	}
	qName := sim.PathParam(r, "queue")
	var req SBQueue
	_ = sim.ReadJSON(r, &req)
	id := parent + "/queues/" + qName
	q := SBQueue{
		ID: id, Name: qName, Type: "Microsoft.ServiceBus/namespaces/queues",
		Properties: map[string]any{
			"maxSizeInMegabytes": 1024,
			"status":             "Active",
		},
	}
	if req.Properties != nil {
		for k, v := range req.Properties {
			q.Properties[k] = v
		}
	}
	sbQueues.Put(id, q)
	sim.WriteJSON(w, http.StatusOK, q)
}

func handleSBGetQueue(w http.ResponseWriter, r *http.Request) {
	id := sbNamespaceID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "name")) +
		"/queues/" + sim.PathParam(r, "queue")
	q, ok := sbQueues.Get(id)
	if !ok {
		sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "queue not found")
		return
	}
	sim.WriteJSON(w, http.StatusOK, q)
}

func handleSBDeleteQueue(w http.ResponseWriter, r *http.Request) {
	id := sbNamespaceID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "name")) +
		"/queues/" + sim.PathParam(r, "queue")
	if !sbQueues.Delete(id) {
		sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "queue not found")
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func handleSBListQueues(w http.ResponseWriter, r *http.Request) {
	prefix := sbNamespaceID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "name")) + "/queues/"
	var out []SBQueue
	for _, q := range sbQueues.List() {
		if strings.HasPrefix(q.ID, prefix) {
			out = append(out, q)
		}
	}
	if out == nil {
		out = []SBQueue{}
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"value": out})
}

func handleSBCreateTopic(w http.ResponseWriter, r *http.Request) {
	parent := sbNamespaceID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "name"))
	if _, ok := sbNamespaces.Get(parent); !ok {
		sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "namespace not found")
		return
	}
	tName := sim.PathParam(r, "topic")
	var req SBTopic
	_ = sim.ReadJSON(r, &req)
	id := parent + "/topics/" + tName
	t := SBTopic{
		ID: id, Name: tName, Type: "Microsoft.ServiceBus/namespaces/topics",
		Properties: map[string]any{
			"maxSizeInMegabytes": 1024,
			"status":             "Active",
		},
	}
	if req.Properties != nil {
		for k, v := range req.Properties {
			t.Properties[k] = v
		}
	}
	sbTopics.Put(id, t)
	sim.WriteJSON(w, http.StatusOK, t)
}

func handleSBGetTopic(w http.ResponseWriter, r *http.Request) {
	id := sbNamespaceID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "name")) +
		"/topics/" + sim.PathParam(r, "topic")
	t, ok := sbTopics.Get(id)
	if !ok {
		sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "topic not found")
		return
	}
	sim.WriteJSON(w, http.StatusOK, t)
}

func handleSBDeleteTopic(w http.ResponseWriter, r *http.Request) {
	id := sbNamespaceID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "name")) +
		"/topics/" + sim.PathParam(r, "topic")
	if !sbTopics.Delete(id) {
		sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "topic not found")
		return
	}
	// Cascade subscriptions under this topic.
	prefix := id + "/subscriptions/"
	for _, s := range sbSubscriptions.List() {
		if strings.HasPrefix(s.ID, prefix) {
			sbSubscriptions.Delete(s.ID)
		}
	}
	w.WriteHeader(http.StatusAccepted)
}

func handleSBListTopics(w http.ResponseWriter, r *http.Request) {
	prefix := sbNamespaceID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "name")) + "/topics/"
	var out []SBTopic
	for _, t := range sbTopics.List() {
		if strings.HasPrefix(t.ID, prefix) && !strings.Contains(strings.TrimPrefix(t.ID, prefix), "/") {
			out = append(out, t)
		}
	}
	if out == nil {
		out = []SBTopic{}
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"value": out})
}

func handleSBCreateSubscription(w http.ResponseWriter, r *http.Request) {
	parent := sbNamespaceID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "name")) +
		"/topics/" + sim.PathParam(r, "topic")
	if _, ok := sbTopics.Get(parent); !ok {
		sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "topic not found")
		return
	}
	sName := sim.PathParam(r, "sub")
	var req SBSubscription
	_ = sim.ReadJSON(r, &req)
	id := parent + "/subscriptions/" + sName
	s := SBSubscription{
		ID: id, Name: sName, Type: "Microsoft.ServiceBus/namespaces/topics/subscriptions",
		Properties: map[string]any{
			"status": "Active",
		},
	}
	if req.Properties != nil {
		for k, v := range req.Properties {
			s.Properties[k] = v
		}
	}
	sbSubscriptions.Put(id, s)
	sim.WriteJSON(w, http.StatusOK, s)
}

func handleSBGetSubscription(w http.ResponseWriter, r *http.Request) {
	id := sbNamespaceID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "name")) +
		"/topics/" + sim.PathParam(r, "topic") + "/subscriptions/" + sim.PathParam(r, "sub")
	s, ok := sbSubscriptions.Get(id)
	if !ok {
		sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "subscription not found")
		return
	}
	sim.WriteJSON(w, http.StatusOK, s)
}

func handleSBDeleteSubscription(w http.ResponseWriter, r *http.Request) {
	id := sbNamespaceID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "name")) +
		"/topics/" + sim.PathParam(r, "topic") + "/subscriptions/" + sim.PathParam(r, "sub")
	if !sbSubscriptions.Delete(id) {
		sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "subscription not found")
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func handleSBListSubscriptions(w http.ResponseWriter, r *http.Request) {
	prefix := sbNamespaceID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "name")) +
		"/topics/" + sim.PathParam(r, "topic") + "/subscriptions/"
	var out []SBSubscription
	for _, s := range sbSubscriptions.List() {
		if strings.HasPrefix(s.ID, prefix) {
			out = append(out, s)
		}
	}
	if out == nil {
		out = []SBSubscription{}
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"value": out})
}
