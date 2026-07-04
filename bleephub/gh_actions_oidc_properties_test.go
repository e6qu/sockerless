package bleephub

import (
	"encoding/json"
	"testing"
)

func TestOIDCCustomPropertyInclusions_CRUD(t *testing.T) {
	org := createTestOrg(t)
	base := "/api/v3/orgs/" + org + "/actions/oidc/customization/properties/repo"

	// Empty to start.
	resp := ghGet(t, base, defaultToken)
	var list []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	resp.Body.Close()
	if len(list) != 0 {
		t.Fatalf("initial inclusions = %v, want empty", list)
	}

	// Create.
	created := decodeJSONWithStatus(t, ghPost(t, base, defaultToken, map[string]string{
		"custom_property_name": "environment_tier",
	}), 201)
	if created["custom_property_name"] != "environment_tier" || created["inclusion_source"] != "organization" {
		t.Fatalf("created inclusion = %v", created)
	}

	// Duplicate rejects.
	resp = ghPost(t, base, defaultToken, map[string]string{"custom_property_name": "environment_tier"})
	resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Fatalf("duplicate create = %d, want 422", resp.StatusCode)
	}

	// Missing name rejects.
	resp = ghPost(t, base, defaultToken, map[string]string{})
	resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Fatalf("missing name create = %d, want 422", resp.StatusCode)
	}

	// Listed.
	resp = ghGet(t, base, defaultToken)
	list = nil
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	resp.Body.Close()
	if len(list) != 1 || list[0]["custom_property_name"] != "environment_tier" {
		t.Fatalf("inclusions after create = %v", list)
	}

	// Delete, then a second delete 404s.
	resp = ghDelete(t, base+"/environment_tier", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("delete = %d, want 204", resp.StatusCode)
	}
	resp = ghDelete(t, base+"/environment_tier", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("delete again = %d, want 404", resp.StatusCode)
	}
}
