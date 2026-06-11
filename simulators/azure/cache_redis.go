package main

import (
	"fmt"
	"net/http"
	"strings"

	sim "github.com/sockerless/simulator"
)

// Microsoft.Cache/Redis ARM control plane. Real Azure exposes a
// full instance lifecycle (create / get / list / patch / delete)
// plus FirewallRule + LinkedServer + Access Policy sub-resources;
// the sim implements the load-bearing CRUD slice. Data plane
// (the actual Redis protocol) is out of scope — terraform's
// `azurerm_redis_cache` resource only needs the ARM lifecycle.

type RedisCache struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Type       string            `json:"type"`
	Location   string            `json:"location,omitempty"`
	SKU        map[string]any    `json:"sku,omitempty"`
	Properties map[string]any    `json:"properties,omitempty"`
	Tags       map[string]string `json:"tags,omitempty"`
}

// RedisFirewallRule is one per-cache firewall rule (start..end IP).
// Real Azure stores them as sub-resources at
// /Redis/{cache}/firewallRules/{name}; terraform-provider-azurerm
// uses azurerm_redis_firewall_rule.
type RedisFirewallRule struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	Type       string         `json:"type"`
	Properties map[string]any `json:"properties"`
}

var (
	redisCaches        sim.Store[RedisCache]
	redisFirewallRules sim.Store[RedisFirewallRule]
)

func registerCacheRedis(srv *sim.Server) {
	redisCaches = sim.MakeStore[RedisCache](srv.DB(), "redis_caches")
	redisFirewallRules = sim.MakeStore[RedisFirewallRule](srv.DB(), "redis_firewall_rules")

	const armBase = "/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.Cache/Redis"

	srv.HandleFunc("PUT "+armBase+"/{name}", handleRedisCacheCreate)
	srv.HandleFunc("PATCH "+armBase+"/{name}", handleRedisCachePatch)
	srv.HandleFunc("GET "+armBase+"/{name}", handleRedisCacheGet)
	srv.HandleFunc("DELETE "+armBase+"/{name}", handleRedisCacheDelete)
	srv.HandleFunc("GET "+armBase, handleRedisCacheListByRG)
	srv.HandleFunc("GET /subscriptions/{subscriptionId}/providers/Microsoft.Cache/Redis", handleRedisCacheListBySubscription)

	srv.HandleFunc("PUT "+armBase+"/{name}/firewallRules/{rule}", handleRedisCacheCreateFirewallRule)
	srv.HandleFunc("GET "+armBase+"/{name}/firewallRules/{rule}", handleRedisCacheGetFirewallRule)
	srv.HandleFunc("DELETE "+armBase+"/{name}/firewallRules/{rule}", handleRedisCacheDeleteFirewallRule)
	srv.HandleFunc("GET "+armBase+"/{name}/firewallRules", handleRedisCacheListFirewallRules)

	srv.HandleFunc("POST "+armBase+"/{name}/listKeys", handleRedisCacheListKeys)
}

func redisCacheID(sub, rg, name string) string {
	return fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Cache/Redis/%s", sub, rg, name)
}

func redisFirewallRuleKey(sub, rg, cache, rule string) string {
	return sub + "/" + rg + "/" + cache + "/" + rule
}

func handleRedisCacheCreateFirewallRule(w http.ResponseWriter, r *http.Request) {
	sub := sim.PathParam(r, "subscriptionId")
	rg := sim.PathParam(r, "resourceGroupName")
	cache := sim.PathParam(r, "name")
	rule := sim.PathParam(r, "rule")
	if _, ok := redisCaches.Get(redisCacheID(sub, rg, cache)); !ok {
		sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound,
			"Redis cache %q not found", cache)
		return
	}
	var req struct {
		Properties map[string]any `json:"properties"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AzureError(w, "BadRequest", err.Error(), http.StatusBadRequest)
		return
	}
	fr := RedisFirewallRule{
		ID:         redisCacheID(sub, rg, cache) + "/firewallRules/" + rule,
		Name:       rule,
		Type:       "Microsoft.Cache/Redis/firewallRules",
		Properties: req.Properties,
	}
	redisFirewallRules.Put(redisFirewallRuleKey(sub, rg, cache, rule), fr)
	sim.WriteJSON(w, http.StatusOK, fr)
}

func handleRedisCacheGetFirewallRule(w http.ResponseWriter, r *http.Request) {
	sub := sim.PathParam(r, "subscriptionId")
	rg := sim.PathParam(r, "resourceGroupName")
	cache := sim.PathParam(r, "name")
	rule := sim.PathParam(r, "rule")
	fr, ok := redisFirewallRules.Get(redisFirewallRuleKey(sub, rg, cache, rule))
	if !ok {
		sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound,
			"Firewall rule %q not found on cache %q", rule, cache)
		return
	}
	sim.WriteJSON(w, http.StatusOK, fr)
}

func handleRedisCacheDeleteFirewallRule(w http.ResponseWriter, r *http.Request) {
	sub := sim.PathParam(r, "subscriptionId")
	rg := sim.PathParam(r, "resourceGroupName")
	cache := sim.PathParam(r, "name")
	rule := sim.PathParam(r, "rule")
	if !redisFirewallRules.Delete(redisFirewallRuleKey(sub, rg, cache, rule)) {
		sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound,
			"Firewall rule %q not found on cache %q", rule, cache)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func handleRedisCacheListFirewallRules(w http.ResponseWriter, r *http.Request) {
	sub := sim.PathParam(r, "subscriptionId")
	rg := sim.PathParam(r, "resourceGroupName")
	cache := sim.PathParam(r, "name")
	prefix := sub + "/" + rg + "/" + cache + "/"
	all := redisFirewallRules.Filter(func(fr RedisFirewallRule) bool {
		return strings.HasPrefix(fr.ID, redisCacheID(sub, rg, cache)+"/firewallRules/")
	})
	if all == nil {
		all = []RedisFirewallRule{}
	}
	_ = prefix
	sim.WriteJSON(w, http.StatusOK, map[string]any{"value": all})
}

// handleRedisCacheListKeys returns the primary + secondary access keys.
// Real Azure generates 32-byte random keys per cache; the sim emits
// deterministic 44-char base64 strings (sha256 of resource ID + key
// kind) so the wire shape matches what clients expect (Redis AUTH
// tokens, downstream connection strings that reference the
// `primary_access_key` attribute in terraform-provider-azurerm). Same
// key across reads; distinct between primary / secondary; distinct
// between caches.
func handleRedisCacheListKeys(w http.ResponseWriter, r *http.Request) {
	sub := sim.PathParam(r, "subscriptionId")
	rg := sim.PathParam(r, "resourceGroupName")
	name := sim.PathParam(r, "name")
	id := redisCacheID(sub, rg, name)
	if _, ok := redisCaches.Get(id); !ok {
		sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound,
			"Redis cache %q not found", name)
		return
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"primaryKey":   simListKey32(id, "primary"),
		"secondaryKey": simListKey32(id, "secondary"),
	})
}

func handleRedisCacheCreate(w http.ResponseWriter, r *http.Request) {
	sub := sim.PathParam(r, "subscriptionId")
	rg := sim.PathParam(r, "resourceGroupName")
	name := sim.PathParam(r, "name")
	var req RedisCache
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AzureErrorf(w, "BadRequest", http.StatusBadRequest, "invalid request body: %v", err)
		return
	}
	id := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Cache/Redis/%s", sub, rg, name)
	cache := RedisCache{
		ID:       id,
		Name:     name,
		Type:     "Microsoft.Cache/Redis",
		Location: req.Location,
		SKU:      req.SKU,
		Tags:     req.Tags,
		Properties: map[string]any{
			"provisioningState": "Creating",
			"redisVersion":      "6.0",
			"sslPort":           6380,
			"port":              6379,
			"hostName":          azureEndpointHostname(r, name, "redis", "cache"),
		},
	}
	// Merge operator-supplied properties (e.g. sku, redisConfiguration).
	if req.Properties != nil {
		for k, v := range req.Properties {
			cache.Properties[k] = v
		}
		cache.Properties["provisioningState"] = "Creating"
	}
	redisCaches.Put(id, cache)
	opID := issueAzureAsyncOperation(func() {
		redisCaches.Update(id, func(stored *RedisCache) {
			if stored.Properties == nil {
				stored.Properties = map[string]any{}
			}
			stored.Properties["provisioningState"] = "Succeeded"
		})
	})
	opURL := azureAsyncOperationHeader(r, sub, "Microsoft.Cache", cache.Location, "operationResults", opID, r.URL.Query().Get("api-version"))
	writeAzureAsyncCreateHeaders(w, opURL, azureCurrentRequestURL(r))
	sim.WriteJSON(w, http.StatusCreated, cache)
}

func handleRedisCachePatch(w http.ResponseWriter, r *http.Request) {
	sub := sim.PathParam(r, "subscriptionId")
	rg := sim.PathParam(r, "resourceGroupName")
	name := sim.PathParam(r, "name")
	id := redisCacheID(sub, rg, name)
	if _, ok := redisCaches.Get(id); !ok {
		sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound,
			"The Resource 'Microsoft.Cache/Redis/%s' under resource group '%s' was not found.", name, rg)
		return
	}
	var req RedisCache
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AzureErrorf(w, "BadRequest", http.StatusBadRequest, "invalid request body: %v", err)
		return
	}
	redisCaches.Update(id, func(cache *RedisCache) {
		if req.Location != "" {
			cache.Location = req.Location
		}
		if req.SKU != nil {
			cache.SKU = req.SKU
		}
		if req.Tags != nil {
			cache.Tags = req.Tags
		}
		if req.Properties != nil {
			if cache.Properties == nil {
				cache.Properties = map[string]any{}
			}
			for k, v := range req.Properties {
				cache.Properties[k] = v
			}
			cache.Properties["provisioningState"] = "Succeeded"
		}
	})
	cache, _ := redisCaches.Get(id)
	sim.WriteJSON(w, http.StatusOK, cache)
}

func handleRedisCacheGet(w http.ResponseWriter, r *http.Request) {
	sub := sim.PathParam(r, "subscriptionId")
	rg := sim.PathParam(r, "resourceGroupName")
	name := sim.PathParam(r, "name")
	id := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Cache/Redis/%s", sub, rg, name)
	cache, ok := redisCaches.Get(id)
	if !ok {
		sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound,
			"The Resource 'Microsoft.Cache/Redis/%s' under resource group '%s' was not found.", name, rg)
		return
	}
	sim.WriteJSON(w, http.StatusOK, cache)
}

func handleRedisCacheDelete(w http.ResponseWriter, r *http.Request) {
	sub := sim.PathParam(r, "subscriptionId")
	rg := sim.PathParam(r, "resourceGroupName")
	name := sim.PathParam(r, "name")
	id := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Cache/Redis/%s", sub, rg, name)
	if !redisCaches.Delete(id) {
		sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound,
			"The Resource 'Microsoft.Cache/Redis/%s' under resource group '%s' was not found.", name, rg)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func handleRedisCacheListByRG(w http.ResponseWriter, r *http.Request) {
	sub := sim.PathParam(r, "subscriptionId")
	rg := sim.PathParam(r, "resourceGroupName")
	prefix := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Cache/Redis/", sub, rg)
	var out []RedisCache
	for _, c := range redisCaches.List() {
		if strings.HasPrefix(c.ID, prefix) {
			out = append(out, c)
		}
	}
	if out == nil {
		out = []RedisCache{}
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"value": out})
}

func handleRedisCacheListBySubscription(w http.ResponseWriter, r *http.Request) {
	sub := sim.PathParam(r, "subscriptionId")
	prefix := fmt.Sprintf("/subscriptions/%s/", sub)
	var out []RedisCache
	for _, c := range redisCaches.List() {
		if strings.HasPrefix(c.ID, prefix) {
			out = append(out, c)
		}
	}
	if out == nil {
		out = []RedisCache{}
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"value": out})
}
