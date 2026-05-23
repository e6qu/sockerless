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
	ID         string                 `json:"id"`
	Name       string                 `json:"name"`
	Type       string                 `json:"type"`
	Location   string                 `json:"location,omitempty"`
	Properties map[string]any         `json:"properties,omitempty"`
	Tags       map[string]string      `json:"tags,omitempty"`
}

var redisCaches sim.Store[RedisCache]

func registerCacheRedis(srv *sim.Server) {
	redisCaches = sim.MakeStore[RedisCache](srv.DB(), "redis_caches")

	const armBase = "/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.Cache/Redis"

	srv.HandleFunc("PUT "+armBase+"/{name}", handleRedisCacheCreate)
	srv.HandleFunc("GET "+armBase+"/{name}", handleRedisCacheGet)
	srv.HandleFunc("DELETE "+armBase+"/{name}", handleRedisCacheDelete)
	srv.HandleFunc("GET "+armBase, handleRedisCacheListByRG)
	srv.HandleFunc("GET /subscriptions/{subscriptionId}/providers/Microsoft.Cache/Redis", handleRedisCacheListBySubscription)
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
		Tags:     req.Tags,
		Properties: map[string]any{
			"provisioningState": "Succeeded",
			"redisVersion":      "7.0",
			"sslPort":           6380,
			"port":              6379,
			"hostName":          name + ".redis.cache.windows.net",
		},
	}
	// Merge operator-supplied properties (e.g. sku, redisConfiguration).
	if req.Properties != nil {
		for k, v := range req.Properties {
			cache.Properties[k] = v
		}
		cache.Properties["provisioningState"] = "Succeeded"
	}
	redisCaches.Put(id, cache)
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
	w.WriteHeader(http.StatusAccepted)
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
