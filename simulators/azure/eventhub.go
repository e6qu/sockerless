package main

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	amqp "github.com/Azure/go-amqp"
	sim "github.com/sockerless/simulator"
)

type EHNamespace struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Type       string            `json:"type"`
	Location   string            `json:"location,omitempty"`
	SKU        map[string]any    `json:"sku,omitempty"`
	Properties map[string]any    `json:"properties,omitempty"`
	Tags       map[string]string `json:"tags,omitempty"`
}

type EHEventHub struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	Type       string         `json:"type"`
	Properties map[string]any `json:"properties,omitempty"`
	CreatedAt  time.Time      `json:"-"`
}

type EHConsumerGroup struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	Type       string         `json:"type"`
	Properties map[string]any `json:"properties,omitempty"`
}

type EHAuthorizationRule struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	Type       string         `json:"type"`
	Properties map[string]any `json:"properties,omitempty"`
}

type ehEventRecord struct {
	SequenceNumber int64
	Offset         string
	EnqueuedTime   time.Time
	Body           []byte
	Properties     map[string]any
}

var (
	ehNamespaces      sim.Store[EHNamespace]
	ehEventHubs       sim.Store[EHEventHub]
	ehConsumerGroups  sim.Store[EHConsumerGroup]
	ehAuthRules       sim.Store[EHAuthorizationRule]
	ehPartitionEvents sim.Store[[]ehEventRecord]
	ehMu              sync.Mutex
)

func registerEventHubs(srv *sim.Server) {
	ehNamespaces = sim.MakeStore[EHNamespace](srv.DB(), "eventhub_namespaces")
	ehEventHubs = sim.MakeStore[EHEventHub](srv.DB(), "eventhub_eventhubs")
	ehConsumerGroups = sim.MakeStore[EHConsumerGroup](srv.DB(), "eventhub_consumer_groups")
	ehAuthRules = sim.MakeStore[EHAuthorizationRule](srv.DB(), "eventhub_auth_rules")
	ehPartitionEvents = sim.MakeStore[[]ehEventRecord](srv.DB(), "eventhub_partition_events")

	const ns = "/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.EventHub/namespaces"

	srv.HandleFunc("PUT "+ns+"/{name}", handleEHCreateNamespace)
	srv.HandleFunc("GET "+ns+"/{name}", handleEHGetNamespace)
	srv.HandleFunc("DELETE "+ns+"/{name}", handleEHDeleteNamespace)
	srv.HandleFunc("GET "+ns, handleEHListNamespacesByRG)
	srv.HandleFunc("GET "+ns+"/{name}/networkRuleSets/default", handleEHGetNamespaceNetworkRuleSet)

	srv.HandleFunc("PUT "+ns+"/{name}/authorizationRules/{rule}", ehAuthRuleCreate("Microsoft.EventHub/namespaces/authorizationRules", "namespaces"))
	srv.HandleFunc("GET "+ns+"/{name}/authorizationRules/{rule}", ehAuthRuleGet("namespaces"))
	srv.HandleFunc("DELETE "+ns+"/{name}/authorizationRules/{rule}", ehAuthRuleDelete("namespaces"))
	srv.HandleFunc("GET "+ns+"/{name}/authorizationRules", ehAuthRuleList("namespaces"))
	srv.HandleFunc("POST "+ns+"/{name}/authorizationRules/{rule}/listKeys", ehAuthRuleListKeys("namespaces"))
	srv.HandleFunc("POST "+ns+"/{name}/authorizationRules/{rule}/regenerateKeys", ehAuthRuleRegenerateKeys("namespaces"))

	srv.HandleFunc("PUT "+ns+"/{name}/eventhubs/{eventhub}", handleEHCreateEventHub)
	srv.HandleFunc("GET "+ns+"/{name}/eventhubs/{eventhub}", handleEHGetEventHub)
	srv.HandleFunc("DELETE "+ns+"/{name}/eventhubs/{eventhub}", handleEHDeleteEventHub)
	srv.HandleFunc("GET "+ns+"/{name}/eventhubs", handleEHListEventHubs)

	srv.HandleFunc("PUT "+ns+"/{name}/eventhubs/{eventhub}/authorizationRules/{rule}", ehAuthRuleCreate("Microsoft.EventHub/namespaces/eventhubs/authorizationRules", "eventhubs"))
	srv.HandleFunc("GET "+ns+"/{name}/eventhubs/{eventhub}/authorizationRules/{rule}", ehAuthRuleGet("eventhubs"))
	srv.HandleFunc("DELETE "+ns+"/{name}/eventhubs/{eventhub}/authorizationRules/{rule}", ehAuthRuleDelete("eventhubs"))
	srv.HandleFunc("GET "+ns+"/{name}/eventhubs/{eventhub}/authorizationRules", ehAuthRuleList("eventhubs"))
	srv.HandleFunc("POST "+ns+"/{name}/eventhubs/{eventhub}/authorizationRules/{rule}/listKeys", ehAuthRuleListKeys("eventhubs"))
	srv.HandleFunc("POST "+ns+"/{name}/eventhubs/{eventhub}/authorizationRules/{rule}/regenerateKeys", ehAuthRuleRegenerateKeys("eventhubs"))

	srv.HandleFunc("PUT "+ns+"/{name}/eventhubs/{eventhub}/consumerGroups/{consumerGroup}", handleEHCreateConsumerGroup)
	srv.HandleFunc("GET "+ns+"/{name}/eventhubs/{eventhub}/consumerGroups/{consumerGroup}", handleEHGetConsumerGroup)
	srv.HandleFunc("DELETE "+ns+"/{name}/eventhubs/{eventhub}/consumerGroups/{consumerGroup}", handleEHDeleteConsumerGroup)
	srv.HandleFunc("GET "+ns+"/{name}/eventhubs/{eventhub}/consumerGroups", handleEHListConsumerGroups)
	srv.HandleFunc("PUT "+ns+"/{name}/eventhubs/{eventhub}/consumergroups/{consumerGroup}", handleEHCreateConsumerGroup)
	srv.HandleFunc("GET "+ns+"/{name}/eventhubs/{eventhub}/consumergroups/{consumerGroup}", handleEHGetConsumerGroup)
	srv.HandleFunc("DELETE "+ns+"/{name}/eventhubs/{eventhub}/consumergroups/{consumerGroup}", handleEHDeleteConsumerGroup)
	srv.HandleFunc("GET "+ns+"/{name}/eventhubs/{eventhub}/consumergroups", handleEHListConsumerGroups)
}

func ehNamespaceID(sub, rg, name string) string {
	return fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.EventHub/namespaces/%s", sub, rg, name)
}

func ehEventHubID(sub, rg, ns, hub string) string {
	return ehNamespaceID(sub, rg, ns) + "/eventhubs/" + hub
}

func ehConsumerGroupID(sub, rg, ns, hub, group string) string {
	return ehEventHubID(sub, rg, ns, hub) + "/consumerGroups/" + group
}

func handleEHCreateNamespace(w http.ResponseWriter, r *http.Request) {
	sub := sim.PathParam(r, "subscriptionId")
	rg := sim.PathParam(r, "resourceGroupName")
	name := sim.PathParam(r, "name")
	var req EHNamespace
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AzureErrorf(w, "BadRequest", http.StatusBadRequest, "invalid request body: %v", err)
		return
	}
	now := time.Now().UTC()
	id := ehNamespaceID(sub, rg, name)
	n := EHNamespace{
		ID:       id,
		Name:     name,
		Type:     "Microsoft.EventHub/namespaces",
		Location: req.Location,
		SKU:      req.SKU,
		Tags:     req.Tags,
		Properties: map[string]any{
			"provisioningState":  "Creating",
			"status":             "Activating",
			"createdAt":          now.Format(time.RFC3339Nano),
			"updatedAt":          now.Format(time.RFC3339Nano),
			"serviceBusEndpoint": azureServiceBusEndpointURL(r, name),
		},
	}
	if n.SKU == nil {
		n.SKU = map[string]any{}
	}
	ehApplyNamespaceDefaults(&n, r)
	for k, v := range req.Properties {
		n.Properties[k] = v
	}
	ehApplyNamespaceDefaults(&n, r)
	n.Properties["provisioningState"] = "Creating"
	n.Properties["status"] = "Activating"
	ehNamespaces.Put(id, n)
	rootID := id + "/authorizationRules/RootManageSharedAccessKey"
	if _, ok := ehAuthRules.Get(rootID); !ok {
		ehAuthRules.Put(rootID, EHAuthorizationRule{
			ID:   rootID,
			Name: "RootManageSharedAccessKey",
			Type: "Microsoft.EventHub/namespaces/authorizationRules",
			Properties: map[string]any{
				"rights": []string{"Listen", "Send", "Manage"},
			},
		})
	}
	opID := issueAzureAsyncOperation(func() {
		ehNamespaces.Update(id, func(stored *EHNamespace) {
			if stored.Properties == nil {
				stored.Properties = map[string]any{}
			}
			stored.Properties["provisioningState"] = "Succeeded"
			stored.Properties["status"] = "Active"
			stored.Properties["updatedAt"] = time.Now().UTC().Format(time.RFC3339Nano)
		})
	})
	opURL := azureAsyncOperationHeader(r, sub, "Microsoft.EventHub", n.Location, "operationResults", opID, r.URL.Query().Get("api-version"))
	writeAzureAsyncCreateHeaders(w, opURL, azureCurrentRequestURL(r))
	sim.WriteJSON(w, http.StatusCreated, n)
}

func handleEHGetNamespace(w http.ResponseWriter, r *http.Request) {
	id := ehNamespaceID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "name"))
	n, ok := ehNamespaces.Get(id)
	if !ok {
		sim.AzureError(w, "ResourceNotFound", "namespace not found", http.StatusNotFound)
		return
	}
	ehApplyNamespaceDefaults(&n, r)
	sim.WriteJSON(w, http.StatusOK, n)
}

func handleEHDeleteNamespace(w http.ResponseWriter, r *http.Request) {
	id := ehNamespaceID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "name"))
	if !ehNamespaces.Delete(id) {
		sim.AzureError(w, "ResourceNotFound", "namespace not found", http.StatusNotFound)
		return
	}
	prefix := id + "/"
	for _, hub := range ehEventHubs.List() {
		if strings.HasPrefix(hub.ID, prefix) {
			ehEventHubs.Delete(hub.ID)
		}
	}
	for _, group := range ehConsumerGroups.List() {
		if strings.HasPrefix(group.ID, prefix) {
			ehConsumerGroups.Delete(group.ID)
		}
	}
	for _, rule := range ehAuthRules.List() {
		if strings.HasPrefix(rule.ID, prefix) {
			ehAuthRules.Delete(rule.ID)
		}
	}
	w.WriteHeader(http.StatusOK)
}

func handleEHListNamespacesByRG(w http.ResponseWriter, r *http.Request) {
	prefix := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.EventHub/namespaces/",
		sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"))
	var out []EHNamespace
	for _, n := range ehNamespaces.List() {
		if strings.HasPrefix(n.ID, prefix) {
			ehApplyNamespaceDefaults(&n, r)
			out = append(out, n)
		}
	}
	if out == nil {
		out = []EHNamespace{}
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"value": out})
}

func handleEHGetNamespaceNetworkRuleSet(w http.ResponseWriter, r *http.Request) {
	id := ehNamespaceID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "name"))
	if _, ok := ehNamespaces.Get(id); !ok {
		sim.AzureError(w, "ResourceNotFound", "namespace not found", http.StatusNotFound)
		return
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"id":   id + "/networkRuleSets/default",
		"name": "default",
		"type": "Microsoft.EventHub/namespaces/networkRuleSets",
		"properties": map[string]any{
			"defaultAction":               "Allow",
			"publicNetworkAccess":         "Enabled",
			"trustedServiceAccessEnabled": false,
			"virtualNetworkRules":         []any{},
			"ipRules":                     []any{},
		},
	})
}

func ehApplyNamespaceDefaults(n *EHNamespace, r *http.Request) {
	if n.SKU == nil {
		n.SKU = map[string]any{}
	}
	if _, ok := n.SKU["name"]; !ok {
		n.SKU["name"] = "Standard"
	}
	if _, ok := n.SKU["tier"]; !ok {
		n.SKU["tier"] = n.SKU["name"]
	}
	if _, ok := n.SKU["capacity"]; !ok {
		n.SKU["capacity"] = 1
	}
	if n.Properties == nil {
		n.Properties = map[string]any{}
	}
	if _, ok := n.Properties["provisioningState"]; !ok {
		n.Properties["provisioningState"] = "Succeeded"
	}
	if _, ok := n.Properties["status"]; !ok {
		n.Properties["status"] = "Active"
	}
	if _, ok := n.Properties["createdAt"]; !ok {
		n.Properties["createdAt"] = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if _, ok := n.Properties["updatedAt"]; !ok {
		n.Properties["updatedAt"] = n.Properties["createdAt"]
	}
	if _, ok := n.Properties["serviceBusEndpoint"]; !ok {
		n.Properties["serviceBusEndpoint"] = azureServiceBusEndpointURL(r, n.Name)
	}
	if _, ok := n.Properties["isAutoInflateEnabled"]; !ok {
		n.Properties["isAutoInflateEnabled"] = false
	}
	if _, ok := n.Properties["maximumThroughputUnits"]; !ok {
		n.Properties["maximumThroughputUnits"] = 0
	}
	if _, ok := n.Properties["publicNetworkAccess"]; !ok {
		n.Properties["publicNetworkAccess"] = "Enabled"
	}
	if _, ok := n.Properties["minimumTlsVersion"]; !ok {
		n.Properties["minimumTlsVersion"] = "1.2"
	}
	if _, ok := n.Properties["disableLocalAuth"]; !ok {
		n.Properties["disableLocalAuth"] = false
	}
}

func handleEHCreateEventHub(w http.ResponseWriter, r *http.Request) {
	sub, rg, nsName, hubName := sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "name"), sim.PathParam(r, "eventhub")
	if _, ok := ehNamespaces.Get(ehNamespaceID(sub, rg, nsName)); !ok {
		sim.AzureError(w, "ResourceNotFound", "namespace not found", http.StatusNotFound)
		return
	}
	var req EHEventHub
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AzureErrorf(w, "BadRequest", http.StatusBadRequest, "invalid request body: %v", err)
		return
	}
	now := time.Now().UTC()
	partitionCount := ehPartitionCount(req.Properties)
	props := map[string]any{
		"createdAt":              now.Format(time.RFC3339Nano),
		"updatedAt":              now.Format(time.RFC3339Nano),
		"messageRetentionInDays": 1,
		"partitionCount":         partitionCount,
		"partitionIds":           ehPartitionIDs(partitionCount),
		"status":                 "Active",
	}
	for k, v := range req.Properties {
		props[k] = v
	}
	props["partitionCount"] = partitionCount
	props["partitionIds"] = ehPartitionIDs(partitionCount)
	hub := EHEventHub{
		ID:         ehEventHubID(sub, rg, nsName, hubName),
		Name:       hubName,
		Type:       "Microsoft.EventHub/namespaces/eventhubs",
		Properties: props,
		CreatedAt:  now,
	}
	ehEventHubs.Put(hub.ID, hub)
	defaultGroupID := hub.ID + "/consumerGroups/$Default"
	if _, ok := ehConsumerGroups.Get(defaultGroupID); !ok {
		ehConsumerGroups.Put(defaultGroupID, EHConsumerGroup{
			ID:   defaultGroupID,
			Name: "$Default",
			Type: "Microsoft.EventHub/namespaces/eventhubs/consumergroups",
			Properties: map[string]any{
				"createdAt": now.Format(time.RFC3339Nano),
				"updatedAt": now.Format(time.RFC3339Nano),
			},
		})
	}
	sim.WriteJSON(w, http.StatusOK, hub)
}

func handleEHGetEventHub(w http.ResponseWriter, r *http.Request) {
	id := ehEventHubID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "name"), sim.PathParam(r, "eventhub"))
	hub, ok := ehEventHubs.Get(id)
	if !ok {
		sim.AzureError(w, "ResourceNotFound", "event hub not found", http.StatusNotFound)
		return
	}
	sim.WriteJSON(w, http.StatusOK, hub)
}

func handleEHDeleteEventHub(w http.ResponseWriter, r *http.Request) {
	id := ehEventHubID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "name"), sim.PathParam(r, "eventhub"))
	if !ehEventHubs.Delete(id) {
		sim.AzureError(w, "ResourceNotFound", "event hub not found", http.StatusNotFound)
		return
	}
	prefix := id + "/"
	for _, group := range ehConsumerGroups.List() {
		if strings.HasPrefix(group.ID, prefix) {
			ehConsumerGroups.Delete(group.ID)
		}
	}
	for _, rule := range ehAuthRules.List() {
		if strings.HasPrefix(rule.ID, prefix) {
			ehAuthRules.Delete(rule.ID)
		}
	}
	w.WriteHeader(http.StatusOK)
}

func handleEHListEventHubs(w http.ResponseWriter, r *http.Request) {
	prefix := ehNamespaceID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "name")) + "/eventhubs/"
	var out []EHEventHub
	for _, hub := range ehEventHubs.List() {
		if strings.HasPrefix(hub.ID, prefix) {
			out = append(out, hub)
		}
	}
	if out == nil {
		out = []EHEventHub{}
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"value": out})
}

func handleEHCreateConsumerGroup(w http.ResponseWriter, r *http.Request) {
	sub, rg, nsName, hubName, groupName := sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "name"), sim.PathParam(r, "eventhub"), sim.PathParam(r, "consumerGroup")
	if _, ok := ehEventHubs.Get(ehEventHubID(sub, rg, nsName, hubName)); !ok {
		sim.AzureError(w, "ResourceNotFound", "event hub not found", http.StatusNotFound)
		return
	}
	var req EHConsumerGroup
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AzureErrorf(w, "BadRequest", http.StatusBadRequest, "invalid request body: %v", err)
		return
	}
	now := time.Now().UTC()
	props := map[string]any{
		"createdAt": now.Format(time.RFC3339Nano),
		"updatedAt": now.Format(time.RFC3339Nano),
	}
	for k, v := range req.Properties {
		props[k] = v
	}
	group := EHConsumerGroup{
		ID:         ehConsumerGroupID(sub, rg, nsName, hubName, groupName),
		Name:       groupName,
		Type:       "Microsoft.EventHub/namespaces/eventhubs/consumergroups",
		Properties: props,
	}
	ehConsumerGroups.Put(group.ID, group)
	sim.WriteJSON(w, http.StatusOK, group)
}

func handleEHGetConsumerGroup(w http.ResponseWriter, r *http.Request) {
	id := ehConsumerGroupID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "name"), sim.PathParam(r, "eventhub"), sim.PathParam(r, "consumerGroup"))
	group, ok := ehConsumerGroups.Get(id)
	if !ok {
		sim.AzureError(w, "ResourceNotFound", "consumer group not found", http.StatusNotFound)
		return
	}
	sim.WriteJSON(w, http.StatusOK, group)
}

func handleEHDeleteConsumerGroup(w http.ResponseWriter, r *http.Request) {
	id := ehConsumerGroupID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "name"), sim.PathParam(r, "eventhub"), sim.PathParam(r, "consumerGroup"))
	if !ehConsumerGroups.Delete(id) {
		sim.AzureError(w, "ResourceNotFound", "consumer group not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func handleEHListConsumerGroups(w http.ResponseWriter, r *http.Request) {
	prefix := ehEventHubID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "name"), sim.PathParam(r, "eventhub")) + "/consumerGroups/"
	var out []EHConsumerGroup
	for _, group := range ehConsumerGroups.List() {
		if strings.HasPrefix(group.ID, prefix) {
			out = append(out, group)
		}
	}
	if out == nil {
		out = []EHConsumerGroup{}
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"value": out})
}

func ehAuthRuleCreate(resourceType, scope string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		parent, ok := ehAuthRuleParentID(r, scope)
		if !ok {
			sim.AzureError(w, "ResourceNotFound", "parent not found", http.StatusNotFound)
			return
		}
		ruleName := sim.PathParam(r, "rule")
		var req EHAuthorizationRule
		if err := sim.ReadJSON(r, &req); err != nil {
			sim.AzureErrorf(w, "BadRequest", http.StatusBadRequest, "invalid request body: %v", err)
			return
		}
		rights := []string{"Listen", "Send"}
		if raw, ok := req.Properties["rights"].([]any); ok && len(raw) > 0 {
			rights = nil
			for _, v := range raw {
				rights = append(rights, fmt.Sprint(v))
			}
		}
		rule := EHAuthorizationRule{
			ID:   parent + "/authorizationRules/" + ruleName,
			Name: ruleName,
			Type: resourceType,
			Properties: map[string]any{
				"rights": rights,
			},
		}
		ehAuthRules.Put(rule.ID, rule)
		sim.WriteJSON(w, http.StatusOK, rule)
	}
}

func ehAuthRuleGet(scope string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		parent, ok := ehAuthRuleParentID(r, scope)
		if !ok {
			sim.AzureError(w, "ResourceNotFound", "parent not found", http.StatusNotFound)
			return
		}
		rule, ok := ehAuthRules.Get(parent + "/authorizationRules/" + sim.PathParam(r, "rule"))
		if !ok {
			sim.AzureError(w, "ResourceNotFound", "authorization rule not found", http.StatusNotFound)
			return
		}
		sim.WriteJSON(w, http.StatusOK, rule)
	}
}

func ehAuthRuleDelete(scope string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		parent, ok := ehAuthRuleParentID(r, scope)
		if !ok {
			sim.AzureError(w, "ResourceNotFound", "parent not found", http.StatusNotFound)
			return
		}
		ehAuthRules.Delete(parent + "/authorizationRules/" + sim.PathParam(r, "rule"))
		w.WriteHeader(http.StatusOK)
	}
}

func ehAuthRuleList(scope string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		parent, ok := ehAuthRuleParentID(r, scope)
		if !ok {
			sim.AzureError(w, "ResourceNotFound", "parent not found", http.StatusNotFound)
			return
		}
		prefix := parent + "/authorizationRules/"
		var out []EHAuthorizationRule
		for _, rule := range ehAuthRules.List() {
			if strings.HasPrefix(rule.ID, prefix) {
				out = append(out, rule)
			}
		}
		if out == nil {
			out = []EHAuthorizationRule{}
		}
		sim.WriteJSON(w, http.StatusOK, map[string]any{"value": out})
	}
}

func ehAuthRuleListKeys(scope string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ehWriteAuthKeys(w, r, scope)
	}
}

func ehAuthRuleRegenerateKeys(scope string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ehWriteAuthKeys(w, r, scope)
	}
}

func ehWriteAuthKeys(w http.ResponseWriter, r *http.Request, scope string) {
	parent, ok := ehAuthRuleParentID(r, scope)
	if !ok {
		sim.AzureError(w, "ResourceNotFound", "parent not found", http.StatusNotFound)
		return
	}
	ruleName := sim.PathParam(r, "rule")
	if _, ok := ehAuthRules.Get(parent + "/authorizationRules/" + ruleName); !ok {
		sim.AzureError(w, "ResourceNotFound", "authorization rule not found", http.StatusNotFound)
		return
	}
	namespace := sim.PathParam(r, "name")
	key := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	conn := fmt.Sprintf("Endpoint=%s;SharedAccessKeyName=%s;SharedAccessKey=%s", azureServiceBusConnectionEndpoint(r, namespace), ruleName, key)
	if scope == "eventhubs" {
		conn += ";EntityPath=" + sim.PathParam(r, "eventhub")
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"keyName":                   ruleName,
		"primaryKey":                key,
		"secondaryKey":              key,
		"primaryConnectionString":   conn,
		"secondaryConnectionString": conn,
	})
}

func ehAuthRuleParentID(r *http.Request, scope string) (string, bool) {
	sub, rg, nsName := sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "name")
	switch scope {
	case "namespaces":
		id := ehNamespaceID(sub, rg, nsName)
		_, ok := ehNamespaces.Get(id)
		return id, ok
	case "eventhubs":
		id := ehEventHubID(sub, rg, nsName, sim.PathParam(r, "eventhub"))
		_, ok := ehEventHubs.Get(id)
		return id, ok
	default:
		return "", false
	}
}

func ehPartitionCount(props map[string]any) int {
	count := 1
	switch v := props["partitionCount"].(type) {
	case float64:
		count = int(v)
	case int:
		count = v
	}
	if count < 1 {
		count = 1
	}
	return count
}

func ehPartitionIDs(count int) []string {
	out := make([]string, 0, count)
	for i := 0; i < count; i++ {
		out = append(out, strconv.Itoa(i))
	}
	return out
}

func ehAMQPHandleRPC(namespace string, req *amqp.Message) (*amqp.Message, bool) {
	if req.ApplicationProperties == nil {
		return nil, false
	}
	if fmt.Sprint(req.ApplicationProperties["operation"]) != "READ" {
		return nil, false
	}
	entityType := fmt.Sprint(req.ApplicationProperties["type"])
	hubName := fmt.Sprint(req.ApplicationProperties["name"])
	switch entityType {
	case "com.microsoft:eventhub":
		hub, ok := ehAMQPFindHub(namespace, hubName)
		if !ok {
			return ehAMQPError(req, 404, "Event Hub not found"), true
		}
		partitions := ehPartitionIDs(ehPartitionCount(hub.Properties))
		return ehAMQPValue(req, map[string]any{
			"name":                  hub.Name,
			"created_at":            hub.CreatedAt,
			"partition_ids":         partitions,
			"georeplication_factor": int64(0),
		}), true
	case "com.microsoft:partition":
		partition := fmt.Sprint(req.ApplicationProperties["partition"])
		hub, ok := ehAMQPFindHub(namespace, hubName)
		if !ok {
			return ehAMQPError(req, 404, "Event Hub not found"), true
		}
		records, _ := ehPartitionEvents.Get(ehPartitionKey(namespace, hub.Name, partition))
		lastSeq := int64(-1)
		lastOffset := ""
		lastTime := time.Time{}
		if len(records) > 0 {
			last := records[len(records)-1]
			lastSeq = last.SequenceNumber
			lastOffset = last.Offset
			lastTime = last.EnqueuedTime
		}
		return ehAMQPValue(req, map[string]any{
			"name":                          hub.Name,
			"partition":                     partition,
			"begin_sequence_number":         int64(0),
			"last_enqueued_sequence_number": lastSeq,
			"last_enqueued_offset":          lastOffset,
			"last_enqueued_time_utc":        lastTime,
			"is_partition_empty":            len(records) == 0,
		}), true
	default:
		return nil, false
	}
}

func ehAMQPValue(req *amqp.Message, value map[string]any) *amqp.Message {
	return &amqp.Message{
		Properties:            &amqp.MessageProperties{CorrelationID: req.Properties.MessageID},
		ApplicationProperties: map[string]any{"status-code": int32(200), "status-description": "OK"},
		Value:                 value,
	}
}

func ehAMQPError(req *amqp.Message, code int32, description string) *amqp.Message {
	return &amqp.Message{
		Properties:            &amqp.MessageProperties{CorrelationID: req.Properties.MessageID},
		ApplicationProperties: map[string]any{"status-code": code, "status-description": description},
	}
}

func ehAMQPFindHub(namespace, hubName string) (EHEventHub, bool) {
	suffix := "/namespaces/" + namespace + "/eventhubs/" + hubName
	for _, hub := range ehEventHubs.List() {
		if strings.HasSuffix(hub.ID, suffix) {
			return hub, true
		}
	}
	return EHEventHub{}, false
}

func ehAMQPIsSenderAddress(namespace, address string) bool {
	hub, _, ok := ehAMQPParseEventHubAddress(address)
	if !ok {
		return false
	}
	_, exists := ehAMQPFindHub(namespace, hub)
	return exists
}

func ehAMQPIsReceiverAddress(namespace, address string) bool {
	hub, partition, ok := ehAMQPParseConsumerAddress(address)
	if !ok || partition == "" {
		return false
	}
	_, exists := ehAMQPFindHub(namespace, hub)
	return exists
}

func ehAMQPEnqueue(namespace, address string, msg *amqp.Message) {
	hubName, partitionID, ok := ehAMQPParseEventHubAddress(address)
	if !ok {
		return
	}
	hub, ok := ehAMQPFindHub(namespace, hubName)
	if !ok {
		return
	}
	if partitionID == "" {
		partitionID = ehSelectPartition(hub, msg)
	}
	ehMu.Lock()
	defer ehMu.Unlock()
	key := ehPartitionKey(namespace, hub.Name, partitionID)
	records, _ := ehPartitionEvents.Get(key)
	for _, event := range ehExpandAMQPEvents(msg) {
		seq := int64(len(records))
		records = append(records, ehEventRecord{
			SequenceNumber: seq,
			Offset:         strconv.FormatInt(seq, 10),
			EnqueuedTime:   time.Now().UTC(),
			Body:           event.GetData(),
			Properties:     event.ApplicationProperties,
		})
	}
	ehPartitionEvents.Put(key, records)
}

func ehExpandAMQPEvents(msg *amqp.Message) []*amqp.Message {
	events := make([]*amqp.Message, 0, len(msg.Data))
	for _, data := range msg.Data {
		var event amqp.Message
		if err := event.UnmarshalBinary(data); err != nil {
			continue
		}
		events = append(events, &event)
	}
	if len(events) == 0 {
		return []*amqp.Message{msg}
	}
	return events
}

func ehAMQPNextEvent(namespace, address string, index int) ([]byte, bool) {
	hubName, partitionID, ok := ehAMQPParseConsumerAddress(address)
	if !ok {
		return nil, false
	}
	records, _ := ehPartitionEvents.Get(ehPartitionKey(namespace, hubName, partitionID))
	if index < 0 || index >= len(records) {
		return nil, false
	}
	rec := records[index]
	out := &amqp.Message{
		DeliveryTag: []byte(generateUUID()),
		Annotations: amqp.Annotations{
			"x-opt-sequence-number": rec.SequenceNumber,
			"x-opt-enqueued-time":   rec.EnqueuedTime,
			"x-opt-offset":          rec.Offset,
		},
		ApplicationProperties: rec.Properties,
		Data:                  [][]byte{rec.Body},
	}
	body, err := out.MarshalBinary()
	if err != nil {
		return nil, false
	}
	return body, true
}

func ehAMQPParseEventHubAddress(address string) (hubName, partitionID string, ok bool) {
	segs := strings.Split(strings.Trim(address, "/"), "/")
	if len(segs) == 1 && segs[0] != "" {
		return segs[0], "", true
	}
	if len(segs) == 3 && strings.EqualFold(segs[1], "Partitions") {
		return segs[0], segs[2], true
	}
	return "", "", false
}

func ehAMQPParseConsumerAddress(address string) (hubName, partitionID string, ok bool) {
	segs := strings.Split(strings.Trim(address, "/"), "/")
	if len(segs) == 5 && strings.EqualFold(segs[1], "ConsumerGroups") && strings.EqualFold(segs[3], "Partitions") {
		return segs[0], segs[4], true
	}
	return "", "", false
}

func ehSelectPartition(hub EHEventHub, msg *amqp.Message) string {
	ids := ehPartitionIDs(ehPartitionCount(hub.Properties))
	if len(ids) == 1 {
		return ids[0]
	}
	key := ""
	if msg.Annotations != nil {
		key = fmt.Sprint(msg.Annotations["x-opt-partition-key"])
	}
	if key == "" && msg.Properties != nil && msg.Properties.MessageID != nil {
		key = fmt.Sprint(msg.Properties.MessageID)
	}
	if key == "" {
		return ids[0]
	}
	sum := md5.Sum([]byte(key))
	n, _ := strconv.ParseUint(hex.EncodeToString(sum[:8]), 16, 64)
	return ids[int(n%uint64(len(ids)))]
}

func ehPartitionKey(namespace, hub, partition string) string {
	return namespace + "/" + hub + "/" + partition
}
