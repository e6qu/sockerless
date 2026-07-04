package bleephub

import (
	"bytes"
	"io"
	"testing"
)

// seedNetworkSettings provisions a hosted compute network settings resource
// the way the Azure private networking onboarding flow does on real GitHub.
func seedNetworkSettings(t *testing.T, org, name string) string {
	t.Helper()
	resp, err := authedPost("/internal/orgs/"+org+"/network-settings", "application/json", bytes.NewReader(mustJSON(map[string]interface{}{
		"name":      name,
		"subnet_id": "/subscriptions/14839728-3ad9-43ab-bd2b-fa6ad0f75e2a/resourceGroups/my-rg/providers/Microsoft.Network/virtualNetworks/my-vnet/subnets/" + name,
		"region":    "eastus",
	})))
	if err != nil {
		t.Fatalf("seed network settings: %v", err)
	}
	if resp.StatusCode != 201 {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("seed network settings: %d %s", resp.StatusCode, b)
	}
	return decodeJSON(t, resp)["id"].(string)
}

func TestOrgNetworkConfigurations_CRUD(t *testing.T) {
	org := createTestOrg(t)
	base := "/api/v3/orgs/" + org + "/settings/network-configurations"
	settingsID := seedNetworkSettings(t, org, "primary-subnet")

	// Create.
	resp := ghPost(t, base, defaultToken, map[string]interface{}{
		"name":                 "my-network-configuration",
		"compute_service":      "actions",
		"network_settings_ids": []string{settingsID},
	})
	if resp.StatusCode != 201 {
		t.Fatalf("create network configuration: %d", resp.StatusCode)
	}
	created := decodeJSON(t, resp)
	if created["name"] != "my-network-configuration" || created["compute_service"] != "actions" {
		t.Fatalf("created = %v", created)
	}
	ids := created["network_settings_ids"].([]interface{})
	if len(ids) != 1 || ids[0] != settingsID {
		t.Fatalf("network_settings_ids = %v", ids)
	}
	configID := created["id"].(string)

	// List.
	resp = ghGet(t, base, defaultToken)
	if resp.StatusCode != 200 {
		t.Fatalf("list network configurations: %d", resp.StatusCode)
	}
	list := decodeJSON(t, resp)
	if list["total_count"].(float64) != 1 {
		t.Fatalf("list = %v", list)
	}

	// GET one.
	resp = ghGet(t, base+"/"+configID, defaultToken)
	if resp.StatusCode != 200 {
		t.Fatalf("get network configuration: %d", resp.StatusCode)
	}
	if got := decodeJSON(t, resp); got["id"] != configID {
		t.Fatalf("get = %v", got)
	}

	// The settings resource now links back to the configuration.
	resp = ghGet(t, "/api/v3/orgs/"+org+"/settings/network-settings/"+settingsID, defaultToken)
	if resp.StatusCode != 200 {
		t.Fatalf("get network settings: %d", resp.StatusCode)
	}
	settings := decodeJSON(t, resp)
	if settings["network_configuration_id"] != configID {
		t.Fatalf("settings = %v", settings)
	}
	if settings["region"] != "eastus" || settings["subnet_id"] == nil {
		t.Fatalf("settings = %v", settings)
	}

	// PATCH: rename and swap the settings resource.
	otherSettingsID := seedNetworkSettings(t, org, "secondary-subnet")
	resp = ghPatch(t, base+"/"+configID, defaultToken, map[string]interface{}{
		"name":                 "renamed-configuration",
		"network_settings_ids": []string{otherSettingsID},
	})
	if resp.StatusCode != 200 {
		t.Fatalf("patch network configuration: %d", resp.StatusCode)
	}
	patched := decodeJSON(t, resp)
	if patched["name"] != "renamed-configuration" {
		t.Fatalf("patched = %v", patched)
	}

	// The old settings resource is unlinked, the new one linked.
	resp = ghGet(t, "/api/v3/orgs/"+org+"/settings/network-settings/"+settingsID, defaultToken)
	if got := decodeJSON(t, resp); got["network_configuration_id"] != nil {
		t.Fatalf("old settings still linked: %v", got)
	}
	resp = ghGet(t, "/api/v3/orgs/"+org+"/settings/network-settings/"+otherSettingsID, defaultToken)
	if got := decodeJSON(t, resp); got["network_configuration_id"] != configID {
		t.Fatalf("new settings not linked: %v", got)
	}

	// DELETE unlinks and removes.
	resp = ghDelete(t, base+"/"+configID, defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("delete network configuration: %d", resp.StatusCode)
	}
	resp = ghGet(t, base+"/"+configID, defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("get deleted configuration: %d", resp.StatusCode)
	}
	resp = ghGet(t, "/api/v3/orgs/"+org+"/settings/network-settings/"+otherSettingsID, defaultToken)
	if got := decodeJSON(t, resp); got["network_configuration_id"] != nil {
		t.Fatalf("settings still linked after delete: %v", got)
	}
}

func TestOrgNetworkConfigurations_Validation(t *testing.T) {
	org := createTestOrg(t)
	base := "/api/v3/orgs/" + org + "/settings/network-configurations"

	// Unknown settings resource.
	resp := ghPost(t, base, defaultToken, map[string]interface{}{
		"name":                 "bad-settings",
		"network_settings_ids": []string{"DOESNOTEXIST0001"},
	})
	resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Fatalf("unknown settings id: %d", resp.StatusCode)
	}

	// Missing settings resource list.
	resp = ghPost(t, base, defaultToken, map[string]interface{}{
		"name": "no-settings",
	})
	resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Fatalf("missing settings ids: %d", resp.StatusCode)
	}

	// A name with unsupported characters.
	settingsID := seedNetworkSettings(t, org, "validation-subnet")
	resp = ghPost(t, base, defaultToken, map[string]interface{}{
		"name":                 "spaces are not allowed",
		"network_settings_ids": []string{settingsID},
	})
	resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Fatalf("bad name: %d", resp.StatusCode)
	}

	// An unsupported compute service.
	resp = ghPost(t, base, defaultToken, map[string]interface{}{
		"name":                 "bad-compute",
		"compute_service":      "codespaces",
		"network_settings_ids": []string{settingsID},
	})
	resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Fatalf("bad compute_service: %d", resp.StatusCode)
	}

	// Unknown settings resource lookups are 404s.
	resp = ghGet(t, "/api/v3/orgs/"+org+"/settings/network-settings/UNKNOWN123", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("unknown settings: %d", resp.StatusCode)
	}
}
