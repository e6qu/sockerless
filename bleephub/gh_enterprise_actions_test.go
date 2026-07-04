package bleephub

import (
	"net/http"
	"testing"
)

func TestEnterpriseActionsCacheLimits(t *testing.T) {
	// GitHub Enterprise Server ships with a 14-day retention limit and a
	// 10 GB storage limit.
	resp := ghGet(t, enterpriseAPI+"/actions/cache/retention-limit", defaultToken)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("get retention: got %d, want 200", resp.StatusCode)
	}
	ret := decodeJSON(t, resp)
	if ret["max_cache_retention_days"] != float64(14) {
		t.Fatalf("max_cache_retention_days = %v, want 14 (GHES default)", ret["max_cache_retention_days"])
	}

	resp = ghGet(t, enterpriseAPI+"/actions/cache/storage-limit", defaultToken)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("get storage: got %d, want 200", resp.StatusCode)
	}
	sto := decodeJSON(t, resp)
	if sto["max_cache_size_gb"] != float64(10) {
		t.Fatalf("max_cache_size_gb = %v, want 10 (GHES default)", sto["max_cache_size_gb"])
	}

	// Set new limits → 204, and they round-trip.
	resp = ghPut(t, enterpriseAPI+"/actions/cache/retention-limit", defaultToken, map[string]interface{}{
		"max_cache_retention_days": 30,
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("put retention: got %d, want 204", resp.StatusCode)
	}
	resp = ghPut(t, enterpriseAPI+"/actions/cache/storage-limit", defaultToken, map[string]interface{}{
		"max_cache_size_gb": 25,
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("put storage: got %d, want 204", resp.StatusCode)
	}

	resp = ghGet(t, enterpriseAPI+"/actions/cache/retention-limit", defaultToken)
	if got := decodeJSON(t, resp)["max_cache_retention_days"]; got != float64(30) {
		t.Fatalf("retention after put = %v, want 30", got)
	}
	resp = ghGet(t, enterpriseAPI+"/actions/cache/storage-limit", defaultToken)
	if got := decodeJSON(t, resp)["max_cache_size_gb"]; got != float64(25) {
		t.Fatalf("storage after put = %v, want 25", got)
	}

	// Invalid values → 400 (the documented bad-request status).
	resp = ghPut(t, enterpriseAPI+"/actions/cache/retention-limit", defaultToken, map[string]interface{}{
		"max_cache_retention_days": 0,
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("put retention 0: got %d, want 400", resp.StatusCode)
	}
	resp = ghPut(t, enterpriseAPI+"/actions/cache/storage-limit", defaultToken, map[string]interface{}{})
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("put storage without member: got %d, want 400", resp.StatusCode)
	}

	// Non-owner → 403.
	memberTok := createEnterpriseTestUser(t, "ent-cache-member")
	resp = ghPut(t, enterpriseAPI+"/actions/cache/retention-limit", memberTok, map[string]interface{}{
		"max_cache_retention_days": 5,
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("non-owner put: got %d, want 403", resp.StatusCode)
	}

	// Restore the defaults for other tests.
	resp = ghPut(t, enterpriseAPI+"/actions/cache/retention-limit", defaultToken, map[string]interface{}{
		"max_cache_retention_days": 14,
	})
	resp.Body.Close()
	resp = ghPut(t, enterpriseAPI+"/actions/cache/storage-limit", defaultToken, map[string]interface{}{
		"max_cache_size_gb": 10,
	})
	resp.Body.Close()
}

func TestEnterpriseActionsOIDCCustomProperties(t *testing.T) {
	base := enterpriseAPI + "/actions/oidc/customization/properties/repo"

	// Create an inclusion.
	resp := ghPost(t, base, defaultToken, map[string]interface{}{
		"custom_property_name": "environment_tier",
	})
	if resp.StatusCode != http.StatusCreated {
		resp.Body.Close()
		t.Fatalf("create inclusion: got %d, want 201", resp.StatusCode)
	}
	created := decodeJSON(t, resp)
	if created["custom_property_name"] != "environment_tier" {
		t.Fatalf("custom_property_name = %v", created["custom_property_name"])
	}
	if created["inclusion_source"] != "enterprise" {
		t.Fatalf("inclusion_source = %v, want enterprise", created["inclusion_source"])
	}

	// Duplicate → 422.
	resp = ghPost(t, base, defaultToken, map[string]interface{}{
		"custom_property_name": "environment_tier",
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("duplicate inclusion: got %d, want 422", resp.StatusCode)
	}

	// Missing name → 422.
	resp = ghPost(t, base, defaultToken, map[string]interface{}{})
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("missing name: got %d, want 422", resp.StatusCode)
	}

	// List contains it.
	resp = ghGet(t, base, defaultToken)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("list inclusions: got %d, want 200", resp.StatusCode)
	}
	found := false
	for _, item := range decodeJSONArray(t, resp) {
		if item["custom_property_name"] == "environment_tier" {
			found = true
			if item["inclusion_source"] != "enterprise" {
				t.Fatalf("listed inclusion_source = %v", item["inclusion_source"])
			}
		}
	}
	if !found {
		t.Fatal("inclusion list does not contain environment_tier")
	}

	// Delete → 204; deleting again → 404.
	resp = ghDelete(t, base+"/environment_tier", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete inclusion: got %d, want 204", resp.StatusCode)
	}
	resp = ghDelete(t, base+"/environment_tier", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("delete absent inclusion: got %d, want 404", resp.StatusCode)
	}
}
