package main

import (
	"fmt"
	"net/http"
	"sort"
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

type SBRule struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	Type       string         `json:"type"`
	Properties map[string]any `json:"properties,omitempty"`
}

type SBNetworkRuleSet struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	Type       string         `json:"type"`
	Properties map[string]any `json:"properties,omitempty"`
}

// SBAuthorizationRule is a SAS authorization rule on a namespace,
// queue, or topic. Real Azure auto-provisions `RootManageSharedAccessKey`
// on every namespace; operators add named rules with scoped rights.
type SBAuthorizationRule struct {
	ID         string                  `json:"id"`
	Name       string                  `json:"name"`
	Type       string                  `json:"type"`
	Properties SBAuthorizationRuleSpec `json:"properties"`
}

type SBAuthorizationRuleSpec struct {
	Rights []string `json:"rights"`
}

var (
	sbNamespaces    sim.Store[SBNamespace]
	sbQueues        sim.Store[SBQueue]
	sbTopics        sim.Store[SBTopic]
	sbSubscriptions sim.Store[SBSubscription]
	sbRules         sim.Store[SBRule]
	sbAuthRules     sim.Store[SBAuthorizationRule]
	sbNetworkRules  sim.Store[SBNetworkRuleSet]
)

func registerServiceBus(srv *sim.Server) {
	sbNamespaces = sim.MakeStore[SBNamespace](srv.DB(), "sb_namespaces")
	sbQueues = sim.MakeStore[SBQueue](srv.DB(), "sb_queues")
	sbTopics = sim.MakeStore[SBTopic](srv.DB(), "sb_topics")
	sbSubscriptions = sim.MakeStore[SBSubscription](srv.DB(), "sb_subscriptions")
	sbRules = sim.MakeStore[SBRule](srv.DB(), "sb_rules")
	sbAuthRules = sim.MakeStore[SBAuthorizationRule](srv.DB(), "sb_auth_rules")
	sbNetworkRules = sim.MakeStore[SBNetworkRuleSet](srv.DB(), "sb_network_rule_sets")

	const ns = "/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.ServiceBus/namespaces"

	srv.HandleFunc("PUT "+ns+"/{name}", handleSBCreateNamespace)
	srv.HandleFunc("GET "+ns+"/{name}", handleSBGetNamespace)
	srv.HandleFunc("DELETE "+ns+"/{name}", handleSBDeleteNamespace)
	srv.HandleFunc("GET "+ns, handleSBListNamespacesByRG)
	srv.HandleFunc("GET "+ns+"/{name}/networkRuleSets/default", handleSBGetNamespaceNetworkRuleSet)
	srv.HandleFunc("PUT "+ns+"/{name}/networkRuleSets/default", handleSBPutNamespaceNetworkRuleSet)
	srv.HandleFunc("GET "+ns+"/{name}/networkRuleSets", handleSBListNamespaceNetworkRuleSets)
	srv.HandleFunc("GET "+ns+"/{name}/disasterRecoveryConfigs/{alias}", handleSBGetDisasterRecoveryConfig)
	srv.HandleFunc("GET "+ns+"/{name}/disasterRecoveryConfigs", handleSBListDisasterRecoveryConfigs)
	srv.HandleFunc("GET "+ns+"/{name}/migrationConfigurations/{config}", handleSBGetMigrationConfiguration)
	srv.HandleFunc("GET "+ns+"/{name}/migrationConfigurations", handleSBListMigrationConfigurations)

	// AuthorizationRules at namespace, queue, and topic scope. Real
	// Azure auto-provisions `RootManageSharedAccessKey` on namespace
	// PUT; named rules can be added with scoped rights (Listen, Send,
	// Manage). The listKeys + regenerateKeys actions return real-
	// shape SAS keys derived from the rule's resource ID.
	srv.HandleFunc("PUT "+ns+"/{name}/authorizationRules/{rule}", sbAuthRuleCreate("Microsoft.ServiceBus/namespaces/authorizationRules", "namespaces"))
	srv.HandleFunc("GET "+ns+"/{name}/authorizationRules/{rule}", sbAuthRuleGet("namespaces"))
	srv.HandleFunc("DELETE "+ns+"/{name}/authorizationRules/{rule}", sbAuthRuleDelete("namespaces"))
	srv.HandleFunc("GET "+ns+"/{name}/authorizationRules", sbAuthRuleList("namespaces"))
	srv.HandleFunc("POST "+ns+"/{name}/authorizationRules/{rule}/listKeys", sbAuthRuleListKeys("namespaces"))
	srv.HandleFunc("POST "+ns+"/{name}/authorizationRules/{rule}/regenerateKeys", sbAuthRuleRegenerateKeys("namespaces"))

	srv.HandleFunc("PUT "+ns+"/{name}/queues/{queue}/authorizationRules/{rule}", sbAuthRuleCreate("Microsoft.ServiceBus/namespaces/queues/authorizationRules", "queues"))
	srv.HandleFunc("GET "+ns+"/{name}/queues/{queue}/authorizationRules/{rule}", sbAuthRuleGet("queues"))
	srv.HandleFunc("DELETE "+ns+"/{name}/queues/{queue}/authorizationRules/{rule}", sbAuthRuleDelete("queues"))
	srv.HandleFunc("GET "+ns+"/{name}/queues/{queue}/authorizationRules", sbAuthRuleList("queues"))
	srv.HandleFunc("POST "+ns+"/{name}/queues/{queue}/authorizationRules/{rule}/listKeys", sbAuthRuleListKeys("queues"))
	srv.HandleFunc("POST "+ns+"/{name}/queues/{queue}/authorizationRules/{rule}/regenerateKeys", sbAuthRuleRegenerateKeys("queues"))

	srv.HandleFunc("PUT "+ns+"/{name}/topics/{topic}/authorizationRules/{rule}", sbAuthRuleCreate("Microsoft.ServiceBus/namespaces/topics/authorizationRules", "topics"))
	srv.HandleFunc("GET "+ns+"/{name}/topics/{topic}/authorizationRules/{rule}", sbAuthRuleGet("topics"))
	srv.HandleFunc("DELETE "+ns+"/{name}/topics/{topic}/authorizationRules/{rule}", sbAuthRuleDelete("topics"))
	srv.HandleFunc("GET "+ns+"/{name}/topics/{topic}/authorizationRules", sbAuthRuleList("topics"))
	srv.HandleFunc("POST "+ns+"/{name}/topics/{topic}/authorizationRules/{rule}/listKeys", sbAuthRuleListKeys("topics"))
	srv.HandleFunc("POST "+ns+"/{name}/topics/{topic}/authorizationRules/{rule}/regenerateKeys", sbAuthRuleRegenerateKeys("topics"))

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

func sbNetworkRuleSetID(sub, rg, name string) string {
	return sbNamespaceID(sub, rg, name) + "/networkRuleSets/default"
}

func handleSBCreateNamespace(w http.ResponseWriter, r *http.Request) {
	sub := sim.PathParam(r, "subscriptionId")
	rg := sim.PathParam(r, "resourceGroupName")
	name := sim.PathParam(r, "name")
	var req SBNamespace
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AzureErrorf(w, "BadRequest", http.StatusBadRequest, "invalid request body: %v", err)
		return
	}
	id := sbNamespaceID(sub, rg, name)
	n := SBNamespace{
		ID:       id,
		Name:     name,
		Type:     "Microsoft.ServiceBus/namespaces",
		Location: req.Location,
		Sku:      req.Sku,
		Tags:     req.Tags,
		Properties: map[string]any{
			"provisioningState":  "Succeeded",
			"serviceBusEndpoint": azureServiceBusEndpointURL(r, name),
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

	// Real Azure auto-provisions `RootManageSharedAccessKey` (Listen+
	// Send+Manage) on every new namespace. Only create on first PUT —
	// preserve any operator edits across subsequent PUTs.
	rootID := id + "/authorizationRules/RootManageSharedAccessKey"
	if _, ok := sbAuthRules.Get(rootID); !ok {
		sbAuthRules.Put(rootID, SBAuthorizationRule{
			ID:   rootID,
			Name: "RootManageSharedAccessKey",
			Type: "Microsoft.ServiceBus/namespaces/authorizationRules",
			Properties: SBAuthorizationRuleSpec{
				Rights: []string{"Listen", "Send", "Manage"},
			},
		})
	}

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
	for _, rule := range sbRules.List() {
		if strings.HasPrefix(rule.ID, prefix) {
			sbRules.Delete(rule.ID)
		}
	}
	for _, rule := range sbAuthRules.List() {
		if strings.HasPrefix(rule.ID, prefix) {
			sbAuthRules.Delete(rule.ID)
		}
	}
	for _, ruleSet := range sbNetworkRules.List() {
		if strings.HasPrefix(ruleSet.ID, prefix) {
			sbNetworkRules.Delete(ruleSet.ID)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func handleSBListNamespacesByRG(w http.ResponseWriter, r *http.Request) {
	prefix := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.ServiceBus/namespaces/",
		sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"))
	all := sbNamespaces.List()
	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })
	out := make([]SBNamespace, 0, len(all))
	for _, n := range all {
		if strings.HasPrefix(n.ID, prefix) {
			out = append(out, n)
		}
	}
	writeSBPagedList(w, r, out)
}

// writeSBPagedList emits a {value, nextLink} ARM list envelope, paginating only
// when the client supplies an explicit $top (armPage returns the full list
// otherwise).
func writeSBPagedList[T any](w http.ResponseWriter, r *http.Request, items []T) {
	page, next := armPage(r, items)
	if page == nil {
		page = []T{}
	}
	out := map[string]any{"value": page}
	if next != "" {
		out["nextLink"] = armNextLink(r, next)
	}
	sim.WriteJSON(w, http.StatusOK, out)
}

func handleSBGetNamespaceNetworkRuleSet(w http.ResponseWriter, r *http.Request) {
	sub, rg, name := sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "name")
	id := sbNetworkRuleSetID(sub, rg, name)
	if _, ok := sbNamespaces.Get(sbNamespaceID(sub, rg, name)); !ok {
		sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "namespace not found")
		return
	}
	ruleSet, ok := sbNetworkRules.Get(id)
	if !ok {
		ruleSet = sbDefaultNetworkRuleSet(id)
	}
	sim.WriteJSON(w, http.StatusOK, ruleSet)
}

func handleSBPutNamespaceNetworkRuleSet(w http.ResponseWriter, r *http.Request) {
	sub, rg, name := sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "name")
	if _, ok := sbNamespaces.Get(sbNamespaceID(sub, rg, name)); !ok {
		sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "namespace not found")
		return
	}
	var req SBNetworkRuleSet
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AzureErrorf(w, "BadRequest", http.StatusBadRequest, "invalid request body: %v", err)
		return
	}
	id := sbNetworkRuleSetID(sub, rg, name)
	ruleSet := sbDefaultNetworkRuleSet(id)
	for k, v := range req.Properties {
		ruleSet.Properties[k] = v
	}
	applySBNetworkRuleSetDefaults(ruleSet.Properties)
	sbNetworkRules.Put(id, ruleSet)
	sim.WriteJSON(w, http.StatusOK, ruleSet)
}

func handleSBListNamespaceNetworkRuleSets(w http.ResponseWriter, r *http.Request) {
	sub, rg, name := sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "name")
	id := sbNetworkRuleSetID(sub, rg, name)
	if _, ok := sbNamespaces.Get(sbNamespaceID(sub, rg, name)); !ok {
		sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "namespace not found")
		return
	}
	ruleSet, ok := sbNetworkRules.Get(id)
	if !ok {
		ruleSet = sbDefaultNetworkRuleSet(id)
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"value": []SBNetworkRuleSet{ruleSet}})
}

func handleSBListDisasterRecoveryConfigs(w http.ResponseWriter, r *http.Request) {
	if _, ok := sbNamespaces.Get(sbNamespaceID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "name"))); !ok {
		sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "namespace not found")
		return
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"value": []any{}})
}

func handleSBGetDisasterRecoveryConfig(w http.ResponseWriter, r *http.Request) {
	if _, ok := sbNamespaces.Get(sbNamespaceID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "name"))); !ok {
		sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "namespace not found")
		return
	}
	sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "disaster recovery config not found")
}

func handleSBListMigrationConfigurations(w http.ResponseWriter, r *http.Request) {
	if _, ok := sbNamespaces.Get(sbNamespaceID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "name"))); !ok {
		sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "namespace not found")
		return
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"value": []any{}})
}

func handleSBGetMigrationConfiguration(w http.ResponseWriter, r *http.Request) {
	if _, ok := sbNamespaces.Get(sbNamespaceID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "name"))); !ok {
		sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "namespace not found")
		return
	}
	sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "migration configuration not found")
}

func sbDefaultNetworkRuleSet(id string) SBNetworkRuleSet {
	props := map[string]any{}
	applySBNetworkRuleSetDefaults(props)
	return SBNetworkRuleSet{
		ID:         id,
		Name:       "default",
		Type:       "Microsoft.ServiceBus/namespaces/networkRuleSets",
		Properties: props,
	}
}

func applySBNetworkRuleSetDefaults(props map[string]any) {
	if _, ok := props["defaultAction"]; !ok {
		props["defaultAction"] = "Allow"
	}
	if _, ok := props["publicNetworkAccess"]; !ok {
		props["publicNetworkAccess"] = "Enabled"
	}
	if _, ok := props["trustedServiceAccessEnabled"]; !ok {
		props["trustedServiceAccessEnabled"] = false
	}
	if _, ok := props["virtualNetworkRules"]; !ok {
		props["virtualNetworkRules"] = []any{}
	}
	if _, ok := props["ipRules"]; !ok {
		props["ipRules"] = []any{}
	}
}

func handleSBCreateQueue(w http.ResponseWriter, r *http.Request) {
	parent := sbNamespaceID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "name"))
	if _, ok := sbNamespaces.Get(parent); !ok {
		sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "namespace not found")
		return
	}
	qName := sim.PathParam(r, "queue")
	var req SBQueue
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AzureErrorf(w, "BadRequest", http.StatusBadRequest, "invalid request body: %v", err)
		return
	}
	id := parent + "/queues/" + qName
	q := SBQueue{
		ID: id, Name: qName, Type: "Microsoft.ServiceBus/namespaces/queues",
		Properties: sbDefaultQueueProperties(),
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
	w.WriteHeader(http.StatusNoContent)
}

func handleSBListQueues(w http.ResponseWriter, r *http.Request) {
	prefix := sbNamespaceID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "name")) + "/queues/"
	all := sbQueues.List()
	sort.Slice(all, func(i, j int) bool { return all[i].Name < all[j].Name })
	filtered := make([]SBQueue, 0, len(all))
	for _, q := range all {
		if strings.HasPrefix(q.ID, prefix) {
			filtered = append(filtered, q)
		}
	}
	writeSBPagedList(w, r, filtered)
}

func handleSBCreateTopic(w http.ResponseWriter, r *http.Request) {
	parent := sbNamespaceID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "name"))
	if _, ok := sbNamespaces.Get(parent); !ok {
		sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "namespace not found")
		return
	}
	tName := sim.PathParam(r, "topic")
	var req SBTopic
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AzureErrorf(w, "BadRequest", http.StatusBadRequest, "invalid request body: %v", err)
		return
	}
	id := parent + "/topics/" + tName
	t := SBTopic{
		ID: id, Name: tName, Type: "Microsoft.ServiceBus/namespaces/topics",
		Properties: sbDefaultTopicProperties(),
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
	for _, rule := range sbRules.List() {
		if strings.HasPrefix(rule.ID, prefix) {
			sbRules.Delete(rule.ID)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func handleSBListTopics(w http.ResponseWriter, r *http.Request) {
	prefix := sbNamespaceID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "name")) + "/topics/"
	all := sbTopics.List()
	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })
	out := make([]SBTopic, 0, len(all))
	for _, t := range all {
		if strings.HasPrefix(t.ID, prefix) && !strings.Contains(strings.TrimPrefix(t.ID, prefix), "/") {
			out = append(out, t)
		}
	}
	writeSBPagedList(w, r, out)
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
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AzureErrorf(w, "BadRequest", http.StatusBadRequest, "invalid request body: %v", err)
		return
	}
	id := parent + "/subscriptions/" + sName
	s := SBSubscription{
		ID: id, Name: sName, Type: "Microsoft.ServiceBus/namespaces/topics/subscriptions",
		Properties: sbDefaultSubscriptionProperties(),
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
	for _, rule := range sbRules.List() {
		if strings.HasPrefix(rule.ID, id+"/rules/") {
			sbRules.Delete(rule.ID)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// sbAuthRuleParentID returns the resource ID of the namespace / queue /
// topic that owns an authorization rule, based on the scope kind.
func sbAuthRuleParentID(r *http.Request, scope string) string {
	base := sbNamespaceID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "name"))
	switch scope {
	case "queues":
		return base + "/queues/" + sim.PathParam(r, "queue")
	case "topics":
		return base + "/topics/" + sim.PathParam(r, "topic")
	default:
		return base
	}
}

// sbAuthRuleParentExists returns true iff the parent namespace / queue
// / topic is present in its store.
func sbAuthRuleParentExists(parent, scope string) bool {
	switch scope {
	case "queues":
		_, ok := sbQueues.Get(parent)
		return ok
	case "topics":
		_, ok := sbTopics.Get(parent)
		return ok
	default:
		_, ok := sbNamespaces.Get(parent)
		return ok
	}
}

func sbAuthRuleCreate(armType, scope string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		parent := sbAuthRuleParentID(r, scope)
		if !sbAuthRuleParentExists(parent, scope) {
			sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "parent resource not found: %s", parent)
			return
		}
		rule := sim.PathParam(r, "rule")
		var req SBAuthorizationRule
		if err := sim.ReadJSON(r, &req); err != nil {
			sim.AzureErrorf(w, "BadRequest", http.StatusBadRequest, "invalid request body: %v", err)
			return
		}
		rights := req.Properties.Rights
		if len(rights) == 0 {
			rights = []string{"Listen"}
		}
		id := parent + "/authorizationRules/" + rule
		stored := SBAuthorizationRule{
			ID:   id,
			Name: rule,
			Type: armType,
			Properties: SBAuthorizationRuleSpec{
				Rights: rights,
			},
		}
		sbAuthRules.Put(id, stored)
		sim.WriteJSON(w, http.StatusOK, stored)
	}
}

func sbAuthRuleGet(scope string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := sbAuthRuleParentID(r, scope) + "/authorizationRules/" + sim.PathParam(r, "rule")
		rule, ok := sbAuthRules.Get(id)
		if !ok {
			sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "authorization rule not found: %s", id)
			return
		}
		sim.WriteJSON(w, http.StatusOK, rule)
	}
}

func sbAuthRuleDelete(scope string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := sbAuthRuleParentID(r, scope) + "/authorizationRules/" + sim.PathParam(r, "rule")
		if !sbAuthRules.Delete(id) {
			sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "authorization rule not found: %s", id)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func sbAuthRuleList(scope string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		parent := sbAuthRuleParentID(r, scope)
		prefix := parent + "/authorizationRules/"
		var out []SBAuthorizationRule
		for _, rule := range sbAuthRules.List() {
			if strings.HasPrefix(rule.ID, prefix) {
				out = append(out, rule)
			}
		}
		if out == nil {
			out = []SBAuthorizationRule{}
		}
		sim.WriteJSON(w, http.StatusOK, map[string]any{"value": out})
	}
}

// sbAuthRuleListKeysBody returns the canonical Service Bus AccessKeys
// shape. Keys are deterministic 44-char base64 strings derived from
// the rule resource ID (mirrors real-Azure SAS-key shape; same key
// across reads, distinct between primary / secondary).
func sbAuthRuleListKeysBody(r *http.Request, ruleID, namespace, ruleName string) map[string]any {
	primary := simListKey32(ruleID, "primary")
	secondary := simListKey32(ruleID, "secondary")
	// Real Azure builds Service Bus connection strings with the namespace
	// endpoint followed by the SAS rule name and key.
	endpoint := "Endpoint=" + azureServiceBusConnectionEndpoint(r, namespace)
	return map[string]any{
		"primaryKey":                primary,
		"secondaryKey":              secondary,
		"primaryConnectionString":   endpoint + ";SharedAccessKeyName=" + ruleName + ";SharedAccessKey=" + primary,
		"secondaryConnectionString": endpoint + ";SharedAccessKeyName=" + ruleName + ";SharedAccessKey=" + secondary,
		"keyName":                   ruleName,
	}
}

func sbAuthRuleListKeys(scope string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ruleName := sim.PathParam(r, "rule")
		id := sbAuthRuleParentID(r, scope) + "/authorizationRules/" + ruleName
		if _, ok := sbAuthRules.Get(id); !ok {
			sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "authorization rule not found: %s", id)
			return
		}
		ns := sim.PathParam(r, "name")
		sim.WriteJSON(w, http.StatusOK, sbAuthRuleListKeysBody(r, id, ns, ruleName))
	}
}

func sbAuthRuleRegenerateKeys(scope string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ruleName := sim.PathParam(r, "rule")
		id := sbAuthRuleParentID(r, scope) + "/authorizationRules/" + ruleName
		if _, ok := sbAuthRules.Get(id); !ok {
			sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "authorization rule not found: %s", id)
			return
		}
		// Real Azure rotates either primary or secondary based on the
		// body's `keyType`; the new key is random. The sim is
		// deterministic per resource ID, so post-rotation keys are
		// identical to pre-rotation. Operators relying on rotation
		// for security boundary testing should know — this is
		// documented in the bug; the wire shape is correct.
		ns := sim.PathParam(r, "name")
		sim.WriteJSON(w, http.StatusOK, sbAuthRuleListKeysBody(r, id, ns, ruleName))
	}
}

func handleSBListSubscriptions(w http.ResponseWriter, r *http.Request) {
	prefix := sbNamespaceID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "name")) +
		"/topics/" + sim.PathParam(r, "topic") + "/subscriptions/"
	all := sbSubscriptions.List()
	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })
	out := make([]SBSubscription, 0, len(all))
	for _, s := range all {
		if strings.HasPrefix(s.ID, prefix) {
			out = append(out, s)
		}
	}
	writeSBPagedList(w, r, out)
}
