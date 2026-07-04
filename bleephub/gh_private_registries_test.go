package bleephub

import (
	"testing"
)

func TestOrgPrivateRegistries_PublicKey(t *testing.T) {
	org := createTestOrg(t)
	resp := ghGet(t, "/api/v3/orgs/"+org+"/private-registries/public-key", defaultToken)
	if resp.StatusCode != 200 {
		t.Fatalf("public key: %d", resp.StatusCode)
	}
	key := decodeJSON(t, resp)
	if key["key_id"] == nil || key["key"] == nil {
		t.Fatalf("public key = %v", key)
	}
}

// privateRegistriesKeyID fetches the org public key ID used to seal values.
func privateRegistriesKeyID(t *testing.T, org string) string {
	t.Helper()
	resp := ghGet(t, "/api/v3/orgs/"+org+"/private-registries/public-key", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("public key: %d", resp.StatusCode)
	}
	return decodeJSON(t, resp)["key_id"].(string)
}

func TestOrgPrivateRegistries_CRUD(t *testing.T) {
	org := createTestOrg(t)
	keyID := privateRegistriesKeyID(t, org)
	base := "/api/v3/orgs/" + org + "/private-registries"

	// Create a token-authenticated Maven repository registry.
	resp := ghPost(t, base, defaultToken, map[string]interface{}{
		"registry_type":   "maven_repository",
		"url":             "https://maven.pkg.example.com/org/",
		"username":        "monalisa",
		"replaces_base":   true,
		"encrypted_value": "c2VjcmV0",
		"key_id":          keyID,
		"visibility":      "private",
	})
	if resp.StatusCode != 201 {
		t.Fatalf("create registry: %d", resp.StatusCode)
	}
	created := decodeJSON(t, resp)
	if created["name"] != "MAVEN_REPOSITORY_SECRET" {
		t.Fatalf("derived name = %v", created["name"])
	}
	if created["registry_type"] != "maven_repository" || created["visibility"] != "private" || created["replaces_base"] != true {
		t.Fatalf("created = %v", created)
	}
	if created["auth_type"] != "token" {
		t.Fatalf("default auth_type = %v", created["auth_type"])
	}
	if _, leaked := created["encrypted_value"]; leaked {
		t.Fatalf("encrypted_value must never be emitted: %v", created)
	}

	// A second registry of the same type gets a suffixed name.
	resp = ghPost(t, base, defaultToken, map[string]interface{}{
		"registry_type":   "maven_repository",
		"url":             "https://maven2.pkg.example.com/org/",
		"encrypted_value": "c2VjcmV0",
		"key_id":          keyID,
		"visibility":      "all",
	})
	if resp.StatusCode != 201 {
		t.Fatalf("create second registry: %d", resp.StatusCode)
	}
	if second := decodeJSON(t, resp); second["name"] != "MAVEN_REPOSITORY_SECRET_2" {
		t.Fatalf("second name = %v", second["name"])
	}

	// List.
	resp = ghGet(t, base, defaultToken)
	if resp.StatusCode != 200 {
		t.Fatalf("list registries: %d", resp.StatusCode)
	}
	list := decodeJSON(t, resp)
	if list["total_count"].(float64) != 2 {
		t.Fatalf("list = %v", list)
	}
	if configs := list["configurations"].([]interface{}); len(configs) != 2 {
		t.Fatalf("configurations = %v", configs)
	}

	// GET one.
	resp = ghGet(t, base+"/MAVEN_REPOSITORY_SECRET", defaultToken)
	if resp.StatusCode != 200 {
		t.Fatalf("get registry: %d", resp.StatusCode)
	}
	if got := decodeJSON(t, resp); got["username"] != "monalisa" {
		t.Fatalf("get = %v", got)
	}

	// PATCH updates in place.
	resp = ghPatch(t, base+"/MAVEN_REPOSITORY_SECRET", defaultToken, map[string]interface{}{
		"url":        "https://maven-new.pkg.example.com/org/",
		"visibility": "all",
	})
	resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("patch registry: %d", resp.StatusCode)
	}
	resp = ghGet(t, base+"/MAVEN_REPOSITORY_SECRET", defaultToken)
	updated := decodeJSON(t, resp)
	if updated["url"] != "https://maven-new.pkg.example.com/org/" || updated["visibility"] != "all" {
		t.Fatalf("updated = %v", updated)
	}

	// The auth type cannot change after creation.
	resp = ghPatch(t, base+"/MAVEN_REPOSITORY_SECRET", defaultToken, map[string]interface{}{
		"auth_type": "oidc_azure",
	})
	resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Fatalf("auth_type change: %d", resp.StatusCode)
	}

	// DELETE.
	resp = ghDelete(t, base+"/MAVEN_REPOSITORY_SECRET", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("delete registry: %d", resp.StatusCode)
	}
	resp = ghGet(t, base+"/MAVEN_REPOSITORY_SECRET", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("get deleted registry: %d", resp.StatusCode)
	}
}

func TestOrgPrivateRegistries_OIDCAndValidation(t *testing.T) {
	org := createTestOrg(t)
	keyID := privateRegistriesKeyID(t, org)
	base := "/api/v3/orgs/" + org + "/private-registries"

	// OIDC registries omit encrypted_value/key_id.
	resp := ghPost(t, base, defaultToken, map[string]interface{}{
		"registry_type": "docker_registry",
		"url":           "https://myregistry.azurecr.io",
		"auth_type":     "oidc_azure",
		"visibility":    "all",
		"tenant_id":     "12345678-1234-1234-1234-123456789012",
		"client_id":     "abcdef01-2345-6789-abcd-ef0123456789",
	})
	if resp.StatusCode != 201 {
		t.Fatalf("create OIDC registry: %d", resp.StatusCode)
	}
	oidc := decodeJSON(t, resp)
	if oidc["auth_type"] != "oidc_azure" || oidc["tenant_id"] != "12345678-1234-1234-1234-123456789012" {
		t.Fatalf("oidc = %v", oidc)
	}

	// An OIDC create carrying encrypted_value is rejected.
	resp = ghPost(t, base, defaultToken, map[string]interface{}{
		"registry_type":   "npm_registry",
		"url":             "https://npm.example.com",
		"auth_type":       "oidc_aws",
		"visibility":      "all",
		"encrypted_value": "c2VjcmV0",
		"key_id":          keyID,
	})
	resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Fatalf("oidc with encrypted_value: %d", resp.StatusCode)
	}

	// A token create without encrypted_value is rejected.
	resp = ghPost(t, base, defaultToken, map[string]interface{}{
		"registry_type": "npm_registry",
		"url":           "https://npm.example.com",
		"visibility":    "all",
	})
	resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Fatalf("token without encrypted_value: %d", resp.StatusCode)
	}

	// A stale key_id is rejected.
	resp = ghPost(t, base, defaultToken, map[string]interface{}{
		"registry_type":   "npm_registry",
		"url":             "https://npm.example.com",
		"visibility":      "all",
		"encrypted_value": "c2VjcmV0",
		"key_id":          "not-the-current-key",
	})
	resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Fatalf("stale key_id: %d", resp.StatusCode)
	}

	// An unknown registry type is rejected.
	resp = ghPost(t, base, defaultToken, map[string]interface{}{
		"registry_type":   "carrier_pigeon",
		"url":             "https://coop.example.com",
		"visibility":      "all",
		"encrypted_value": "c2VjcmV0",
		"key_id":          keyID,
	})
	resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Fatalf("unknown registry_type: %d", resp.StatusCode)
	}
}
