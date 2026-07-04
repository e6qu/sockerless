package bleephub

import (
	"fmt"
	"net/http"
	"testing"
)

func TestOrgWebhookConfig_GetAndPatch(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	org := testServer.store.CreateOrg(admin, "hook-config-org", "Hook Config Org", "")
	if org == nil {
		t.Fatal("create org failed")
	}

	resp := ghPost(t, "/api/v3/orgs/hook-config-org/hooks", defaultToken, map[string]interface{}{
		"name": "web",
		"config": map[string]interface{}{
			"url":          "https://example.test/hook",
			"content_type": "json",
			"secret":       "hush",
		},
		"events": []string{"push"},
	})
	if resp.StatusCode != http.StatusCreated {
		resp.Body.Close()
		t.Fatalf("create org hook: %d", resp.StatusCode)
	}
	created := decodeJSON(t, resp)
	hookID := int(created["id"].(float64))
	configPath := fmt.Sprintf("/api/v3/orgs/hook-config-org/hooks/%d/config", hookID)

	// GET returns the stored config with the secret masked.
	resp = ghGet(t, configPath, defaultToken)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("get hook config: %d", resp.StatusCode)
	}
	config := decodeJSON(t, resp)
	if config["url"] != "https://example.test/hook" || config["content_type"] != "json" || config["insecure_ssl"] != "0" {
		t.Fatalf("hook config wrong: %v", config)
	}
	if config["secret"] != "********" {
		t.Fatalf("hook config secret = %v, want masked", config["secret"])
	}

	// PATCH updates only the provided members.
	resp = ghPatch(t, configPath, defaultToken, map[string]interface{}{
		"url":          "https://example.test/hook2",
		"insecure_ssl": "1",
	})
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("patch hook config: %d", resp.StatusCode)
	}
	patched := decodeJSON(t, resp)
	if patched["url"] != "https://example.test/hook2" || patched["insecure_ssl"] != "1" || patched["content_type"] != "json" {
		t.Fatalf("patched hook config wrong: %v", patched)
	}

	// The parent hook resource reflects the config change.
	resp = ghGet(t, fmt.Sprintf("/api/v3/orgs/hook-config-org/hooks/%d", hookID), defaultToken)
	hook := decodeJSON(t, resp)
	hookConfig, _ := hook["config"].(map[string]interface{})
	if hookConfig == nil || hookConfig["url"] != "https://example.test/hook2" {
		t.Fatalf("hook resource config not updated: %v", hook)
	}

	// Unknown hook.
	resp = ghGet(t, "/api/v3/orgs/hook-config-org/hooks/999999/config", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown hook config: %d, want 404", resp.StatusCode)
	}
}
