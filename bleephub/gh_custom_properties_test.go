package bleephub

import (
	"testing"
)

func TestOrgCustomProperties_SchemaCRUD(t *testing.T) {
	org := createTestOrg(t)

	// PUT a single_select definition.
	resp := ghPut(t, "/api/v3/orgs/"+org+"/properties/schema/environment", defaultToken, map[string]interface{}{
		"value_type":     "single_select",
		"required":       true,
		"default_value":  "production",
		"description":    "Prod or dev environment",
		"allowed_values": []string{"production", "development"},
	})
	if resp.StatusCode != 200 {
		t.Fatalf("put property: %d", resp.StatusCode)
	}
	created := decodeJSON(t, resp)
	if created["property_name"] != "environment" || created["value_type"] != "single_select" {
		t.Fatalf("created = %v", created)
	}
	if created["required"] != true || created["default_value"] != "production" {
		t.Fatalf("created = %v", created)
	}
	if created["source_type"] != "organization" {
		t.Fatalf("source_type = %v", created["source_type"])
	}
	if created["values_editable_by"] != "org_actors" {
		t.Fatalf("values_editable_by default = %v", created["values_editable_by"])
	}

	// PATCH the schema in a batch.
	resp = ghPatch(t, "/api/v3/orgs/"+org+"/properties/schema", defaultToken, map[string]interface{}{
		"properties": []map[string]interface{}{
			{"property_name": "service", "value_type": "string"},
			{"property_name": "team", "value_type": "string", "description": "Team owning the repository"},
		},
	})
	if resp.StatusCode != 200 {
		t.Fatalf("patch schema: %d", resp.StatusCode)
	}
	if batch := decodeJSONArray(t, resp); len(batch) != 2 {
		t.Fatalf("batch = %v", batch)
	}

	// GET all definitions, sorted by name.
	resp = ghGet(t, "/api/v3/orgs/"+org+"/properties/schema", defaultToken)
	if resp.StatusCode != 200 {
		t.Fatalf("get schema: %d", resp.StatusCode)
	}
	all := decodeJSONArray(t, resp)
	if len(all) != 3 {
		t.Fatalf("schema = %v", all)
	}
	if all[0]["property_name"] != "environment" || all[1]["property_name"] != "service" || all[2]["property_name"] != "team" {
		t.Fatalf("schema order = %v", all)
	}

	// GET one definition.
	resp = ghGet(t, "/api/v3/orgs/"+org+"/properties/schema/team", defaultToken)
	if resp.StatusCode != 200 {
		t.Fatalf("get property: %d", resp.StatusCode)
	}
	if got := decodeJSON(t, resp); got["description"] != "Team owning the repository" {
		t.Fatalf("get property = %v", got)
	}

	// Replacing a definition overwrites missing optional values.
	resp = ghPut(t, "/api/v3/orgs/"+org+"/properties/schema/team", defaultToken, map[string]interface{}{
		"value_type": "string",
	})
	if resp.StatusCode != 200 {
		t.Fatalf("replace property: %d", resp.StatusCode)
	}
	if got := decodeJSON(t, resp); got["description"] != nil {
		t.Fatalf("replaced property should drop description, got %v", got)
	}

	// DELETE.
	resp = ghDelete(t, "/api/v3/orgs/"+org+"/properties/schema/team", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("delete property: %d", resp.StatusCode)
	}
	resp = ghGet(t, "/api/v3/orgs/"+org+"/properties/schema/team", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("get deleted property: %d", resp.StatusCode)
	}
}

func TestOrgCustomProperties_SchemaValidation(t *testing.T) {
	org := createTestOrg(t)

	// Bad value type.
	resp := ghPut(t, "/api/v3/orgs/"+org+"/properties/schema/bad", defaultToken, map[string]interface{}{
		"value_type": "integer",
	})
	resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Fatalf("bad value_type: %d", resp.StatusCode)
	}

	// Required without a default.
	resp = ghPut(t, "/api/v3/orgs/"+org+"/properties/schema/bad", defaultToken, map[string]interface{}{
		"value_type": "string",
		"required":   true,
	})
	resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Fatalf("required without default: %d", resp.StatusCode)
	}

	// allowed_values on a non-select type.
	resp = ghPut(t, "/api/v3/orgs/"+org+"/properties/schema/bad", defaultToken, map[string]interface{}{
		"value_type":     "string",
		"allowed_values": []string{"a"},
	})
	resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Fatalf("allowed_values on string: %d", resp.StatusCode)
	}
}

func TestRepoCustomPropertyValues_SetAndRead(t *testing.T) {
	org := createTestOrg(t)
	repoName, _ := createOrgRepoForGovernance(t, org)
	repoPath := org + "/" + repoName

	// Definitions: one with a default, one select, one boolean.
	resp := ghPatch(t, "/api/v3/orgs/"+org+"/properties/schema", defaultToken, map[string]interface{}{
		"properties": []map[string]interface{}{
			{"property_name": "environment", "value_type": "single_select", "default_value": "production",
				"allowed_values": []string{"production", "development"}},
			{"property_name": "team", "value_type": "string"},
			{"property_name": "archived_ok", "value_type": "true_false"},
		},
	})
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("schema: %d", resp.StatusCode)
	}

	// Before any assignment the default is the effective value.
	resp = ghGet(t, "/api/v3/repos/"+repoPath+"/properties/values", defaultToken)
	if resp.StatusCode != 200 {
		t.Fatalf("get repo values: %d", resp.StatusCode)
	}
	values := decodeJSONArray(t, resp)
	if len(values) != 1 || values[0]["property_name"] != "environment" || values[0]["value"] != "production" {
		t.Fatalf("default values = %v", values)
	}

	// PATCH repo values.
	resp = ghPatch(t, "/api/v3/repos/"+repoPath+"/properties/values", defaultToken, map[string]interface{}{
		"properties": []map[string]interface{}{
			{"property_name": "environment", "value": "development"},
			{"property_name": "team", "value": "octocats"},
			{"property_name": "archived_ok", "value": "true"},
		},
	})
	resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("patch repo values: %d", resp.StatusCode)
	}
	resp = ghGet(t, "/api/v3/repos/"+repoPath+"/properties/values", defaultToken)
	values = decodeJSONArray(t, resp)
	if len(values) != 3 {
		t.Fatalf("values = %v", values)
	}
	byName := map[string]interface{}{}
	for _, v := range values {
		byName[v["property_name"].(string)] = v["value"]
	}
	if byName["environment"] != "development" || byName["team"] != "octocats" || byName["archived_ok"] != "true" {
		t.Fatalf("values = %v", byName)
	}

	// Null unsets; environment falls back to the default.
	resp = ghPatch(t, "/api/v3/repos/"+repoPath+"/properties/values", defaultToken, map[string]interface{}{
		"properties": []map[string]interface{}{
			{"property_name": "environment", "value": nil},
			{"property_name": "team", "value": nil},
		},
	})
	resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("unset repo values: %d", resp.StatusCode)
	}
	resp = ghGet(t, "/api/v3/repos/"+repoPath+"/properties/values", defaultToken)
	values = decodeJSONArray(t, resp)
	if len(values) != 2 {
		t.Fatalf("values after unset = %v", values)
	}
	if values[0]["property_name"] != "archived_ok" || values[1]["value"] != "production" {
		t.Fatalf("values after unset = %v", values)
	}

	// Value outside allowed_values is a 422.
	resp = ghPatch(t, "/api/v3/repos/"+repoPath+"/properties/values", defaultToken, map[string]interface{}{
		"properties": []map[string]interface{}{
			{"property_name": "environment", "value": "staging"},
		},
	})
	resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Fatalf("invalid select value: %d", resp.StatusCode)
	}

	// Unknown property is a 422.
	resp = ghPatch(t, "/api/v3/repos/"+repoPath+"/properties/values", defaultToken, map[string]interface{}{
		"properties": []map[string]interface{}{
			{"property_name": "no-such-prop", "value": "x"},
		},
	})
	resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Fatalf("unknown property: %d", resp.StatusCode)
	}
}

func TestOrgCustomProperties_OrgValues(t *testing.T) {
	org := createTestOrg(t)
	repoA, _ := createOrgRepoForGovernance(t, org)
	repoB, _ := createOrgRepoForGovernance(t, org)

	resp := ghPatch(t, "/api/v3/orgs/"+org+"/properties/schema", defaultToken, map[string]interface{}{
		"properties": []map[string]interface{}{
			{"property_name": "team", "value_type": "string"},
		},
	})
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("schema: %d", resp.StatusCode)
	}

	// Batch-apply values to both repositories.
	resp = ghPatch(t, "/api/v3/orgs/"+org+"/properties/values", defaultToken, map[string]interface{}{
		"repository_names": []string{repoA, repoB},
		"properties": []map[string]interface{}{
			{"property_name": "team", "value": "platform"},
		},
	})
	resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("patch org values: %d", resp.StatusCode)
	}

	// List org repo values.
	resp = ghGet(t, "/api/v3/orgs/"+org+"/properties/values", defaultToken)
	if resp.StatusCode != 200 {
		t.Fatalf("list org values: %d", resp.StatusCode)
	}
	entries := decodeJSONArray(t, resp)
	if len(entries) != 2 {
		t.Fatalf("entries = %v", entries)
	}
	for _, entry := range entries {
		props := entry["properties"].([]interface{})
		if len(props) != 1 || props[0].(map[string]interface{})["value"] != "platform" {
			t.Fatalf("entry = %v", entry)
		}
		if entry["repository_id"] == nil || entry["repository_full_name"] == nil {
			t.Fatalf("entry = %v", entry)
		}
	}

	// repository_query filters by name.
	resp = ghGet(t, "/api/v3/orgs/"+org+"/properties/values?repository_query="+repoA, defaultToken)
	filtered := decodeJSONArray(t, resp)
	if len(filtered) != 1 || filtered[0]["repository_name"] != repoA {
		t.Fatalf("filtered = %v", filtered)
	}

	// Unknown repository name in the batch is a 422.
	resp = ghPatch(t, "/api/v3/orgs/"+org+"/properties/values", defaultToken, map[string]interface{}{
		"repository_names": []string{"no-such-repo-here"},
		"properties": []map[string]interface{}{
			{"property_name": "team", "value": "x"},
		},
	})
	resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Fatalf("unknown repo: %d", resp.StatusCode)
	}
}
