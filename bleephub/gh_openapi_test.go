package bleephub

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// Schema definitions loaded from testdata.
type schemaProperty struct {
	Type   string `json:"type"`
	Format string `json:"format,omitempty"`
}

type schemaDefinition struct {
	Required   []string                  `json:"required"`
	Properties map[string]schemaProperty `json:"properties"`
}

var openAPISchemas map[string]schemaDefinition

func loadSchemas(t *testing.T) {
	t.Helper()
	if openAPISchemas != nil {
		return
	}
	data, err := os.ReadFile("testdata/openapi-schemas.json")
	if err != nil {
		t.Fatalf("failed to load schemas: %v", err)
	}
	if err := json.Unmarshal(data, &openAPISchemas); err != nil {
		t.Fatalf("failed to parse schemas: %v", err)
	}
}

// validateSchema checks a JSON object against a named schema.
func validateSchema(t *testing.T, schemaName string, obj map[string]interface{}) {
	t.Helper()
	loadSchemas(t)

	schema, ok := openAPISchemas[schemaName]
	if !ok {
		t.Fatalf("unknown schema: %s", schemaName)
	}

	// Check required fields
	for _, field := range schema.Required {
		if _, exists := obj[field]; !exists {
			t.Errorf("[%s] missing required field: %s", schemaName, field)
		}
	}

	// Check types for present fields
	for field, prop := range schema.Properties {
		val, exists := obj[field]
		if !exists {
			continue
		}
		if val == nil {
			continue // null is valid for optional fields
		}

		switch prop.Type {
		case "string":
			s, ok := val.(string)
			if !ok {
				t.Errorf("[%s.%s] expected string, got %T", schemaName, field, val)
			} else if prop.Format == "date-time" {
				if _, err := time.Parse(time.RFC3339, s); err != nil {
					t.Errorf("[%s.%s] invalid date-time: %s", schemaName, field, s)
				}
			}
		case "number":
			if _, ok := val.(float64); !ok {
				t.Errorf("[%s.%s] expected number, got %T", schemaName, field, val)
			}
		case "boolean":
			if _, ok := val.(bool); !ok {
				t.Errorf("[%s.%s] expected boolean, got %T", schemaName, field, val)
			}
		case "array":
			if _, ok := val.([]interface{}); !ok {
				t.Errorf("[%s.%s] expected array, got %T", schemaName, field, val)
			}
		case "object":
			if _, ok := val.(map[string]interface{}); !ok {
				t.Errorf("[%s.%s] expected object, got %T", schemaName, field, val)
			}
		}
	}
}

func TestOpenAPIRepo(t *testing.T) {
	resp := ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name":        "oa-repo",
		"description": "OpenAPI test",
	})
	if resp.StatusCode != 201 {
		resp.Body.Close()
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)
	validateSchema(t, "Repository", data)

	// Also validate GET
	resp2 := ghGet(t, "/api/v3/repos/admin/oa-repo", "")
	if resp2.StatusCode != 200 {
		resp2.Body.Close()
		t.Fatalf("expected 200, got %d", resp2.StatusCode)
	}
	data2 := decodeJSON(t, resp2)
	validateSchema(t, "Repository", data2)
}

func TestOpenAPIIssue(t *testing.T) {
	createTestIssueRepo(t, "oa-issue")

	resp := ghPost(t, "/api/v3/repos/admin/oa-issue/issues", defaultToken, map[string]interface{}{
		"title": "OpenAPI issue",
		"body":  "Testing schema",
	})
	if resp.StatusCode != 201 {
		resp.Body.Close()
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)
	validateSchema(t, "Issue", data)

	// GET
	resp2 := ghGet(t, "/api/v3/repos/admin/oa-issue/issues/1", "")
	data2 := decodeJSON(t, resp2)
	validateSchema(t, "Issue", data2)
}

func TestOpenAPIPullRequest(t *testing.T) {
	createTestPRRepo(t, "oa-pr")

	resp := ghPost(t, "/api/v3/repos/admin/oa-pr/pulls", defaultToken, map[string]interface{}{
		"title": "OpenAPI PR",
		"head":  "feature",
		"base":  "main",
	})
	if resp.StatusCode != 201 {
		resp.Body.Close()
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)
	validateSchema(t, "PullRequest", data)

	// GET
	resp2 := ghGet(t, "/api/v3/repos/admin/oa-pr/pulls/1", "")
	data2 := decodeJSON(t, resp2)
	validateSchema(t, "PullRequest", data2)
}

func TestOpenAPILabel(t *testing.T) {
	createTestIssueRepo(t, "oa-label")

	resp := ghPost(t, "/api/v3/repos/admin/oa-label/labels", defaultToken, map[string]interface{}{
		"name": "oa-bug", "color": "d73a4a", "description": "A bug",
	})
	if resp.StatusCode != 201 {
		resp.Body.Close()
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)
	validateSchema(t, "Label", data)

	// GET
	resp2 := ghGet(t, "/api/v3/repos/admin/oa-label/labels/oa-bug", "")
	data2 := decodeJSON(t, resp2)
	validateSchema(t, "Label", data2)
}

func TestOpenAPIMilestone(t *testing.T) {
	createTestIssueRepo(t, "oa-milestone")

	resp := ghPost(t, "/api/v3/repos/admin/oa-milestone/milestones", defaultToken, map[string]interface{}{
		"title": "v1.0", "description": "First release",
	})
	if resp.StatusCode != 201 {
		resp.Body.Close()
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)
	validateSchema(t, "Milestone", data)
}

func TestOpenAPIUser(t *testing.T) {
	resp := ghGet(t, "/api/v3/user", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)
	validateSchema(t, "User", data)

	// Users endpoint
	resp2 := ghGet(t, "/api/v3/users/admin", "")
	data2 := decodeJSON(t, resp2)
	validateSchema(t, "User", data2)
}

func TestOpenAPIOrg(t *testing.T) {
	ghPost(t, "/api/v3/user/orgs", defaultToken, map[string]interface{}{
		"login": fmt.Sprintf("oa-org-%d", time.Now().UnixNano()),
		"name":  "OpenAPI Org",
	}).Body.Close()

	orgs := func() string {
		resp := ghGet(t, "/api/v3/user/orgs", defaultToken)
		defer resp.Body.Close()
		var list []map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&list)
		if len(list) == 0 {
			t.Fatal("no orgs found")
		}
		return list[0]["login"].(string)
	}()

	resp := ghGet(t, "/api/v3/orgs/"+orgs, "")
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)
	validateSchema(t, "Organization", data)
}

func TestOpenAPIError(t *testing.T) {
	resp := ghGet(t, "/api/v3/repos/nobody/nothing", "")
	if resp.StatusCode != 404 {
		resp.Body.Close()
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)
	validateSchema(t, "Error", data)

	// 401 error
	resp2 := ghGet(t, "/api/v3/user", "")
	if resp2.StatusCode != 401 {
		resp2.Body.Close()
		t.Fatalf("expected 401, got %d", resp2.StatusCode)
	}
	data2 := decodeJSON(t, resp2)
	validateSchema(t, "Error", data2)
}

func TestOpenAPIListSchemaConsistency(t *testing.T) {
	createTestIssueRepo(t, "oa-list")
	ghPost(t, "/api/v3/repos/admin/oa-list/issues", defaultToken, map[string]interface{}{
		"title": "List item",
	}).Body.Close()

	resp := ghGet(t, "/api/v3/repos/admin/oa-list/issues", "")
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	defer resp.Body.Close()

	var issues []map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&issues)
	if len(issues) == 0 {
		t.Fatal("expected at least 1 issue")
	}

	// Each item in list should conform to Issue schema
	for i, issue := range issues {
		t.Run(fmt.Sprintf("issue-%d", i), func(t *testing.T) {
			validateSchema(t, "Issue", issue)
		})
	}
}

func TestOpenAPIRepoListSchemaConsistency(t *testing.T) {
	resp := ghGet(t, "/api/v3/user/repos", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	defer resp.Body.Close()

	var repos []map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&repos)
	if len(repos) == 0 {
		t.Fatal("expected at least 1 repo")
	}

	// Validate first repo against schema
	validateSchema(t, "Repository", repos[0])
}

func TestOpenAPICharsetInContentType(t *testing.T) {
	resp := ghGet(t, "/api/v3/repos/admin/oa-list", "")
	defer resp.Body.Close()

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Fatalf("expected application/json in Content-Type, got %s", ct)
	}
	if !strings.Contains(ct, "charset=utf-8") {
		t.Fatalf("expected charset=utf-8 in Content-Type, got %s", ct)
	}
}
