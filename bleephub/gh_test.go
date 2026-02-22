package bleephub

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

const defaultToken = "bph_0000000000000000000000000000000000000000"

func ghGet(t *testing.T, path string, token string) *http.Response {
	t.Helper()
	req, err := http.NewRequest("GET", testBaseURL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "token "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func ghPost(t *testing.T, path string, token string, body interface{}) *http.Response {
	t.Helper()
	var bodyReader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(b)
	}
	req, err := http.NewRequest("POST", testBaseURL+path, bodyReader)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "token "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func decodeJSON(t *testing.T, resp *http.Response) map[string]interface{} {
	t.Helper()
	defer resp.Body.Close()
	var data map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}
	return data
}

// TestGHApiRoot verifies GET /api/v3/ with and without valid token.
func TestGHApiRoot(t *testing.T) {
	// With valid token
	resp := ghGet(t, "/api/v3/", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)
	if data["current_user_url"] == nil {
		t.Fatal("missing current_user_url in API root")
	}

	// Without token — 401
	resp2 := ghGet(t, "/api/v3/", "")
	defer resp2.Body.Close()
	if resp2.StatusCode != 401 {
		t.Fatalf("expected 401 without token, got %d", resp2.StatusCode)
	}
}

// TestGHScopeHeaders verifies X-OAuth-Scopes header is present.
func TestGHScopeHeaders(t *testing.T) {
	resp := ghGet(t, "/api/v3/", defaultToken)
	defer resp.Body.Close()

	scopes := resp.Header.Get("X-OAuth-Scopes")
	if scopes == "" {
		t.Fatal("missing X-OAuth-Scopes header")
	}
	if !strings.Contains(scopes, "repo") {
		t.Fatalf("expected 'repo' in scopes, got: %s", scopes)
	}
	if !strings.Contains(scopes, "read:org") {
		t.Fatalf("expected 'read:org' in scopes, got: %s", scopes)
	}
}

// TestGHRateLimitHeaders verifies X-RateLimit-* headers are present.
func TestGHRateLimitHeaders(t *testing.T) {
	resp := ghGet(t, "/api/v3/", defaultToken)
	defer resp.Body.Close()

	for _, header := range []string{
		"X-RateLimit-Limit",
		"X-RateLimit-Remaining",
		"X-RateLimit-Used",
		"X-RateLimit-Reset",
		"X-RateLimit-Resource",
	} {
		if resp.Header.Get(header) == "" {
			t.Fatalf("missing header: %s", header)
		}
	}
	if resp.Header.Get("X-RateLimit-Limit") != "5000" {
		t.Fatalf("expected X-RateLimit-Limit=5000, got %s", resp.Header.Get("X-RateLimit-Limit"))
	}
}

// TestGHRequestIdHeader verifies X-GitHub-Request-Id is present.
func TestGHRequestIdHeader(t *testing.T) {
	resp := ghGet(t, "/api/v3/", defaultToken)
	defer resp.Body.Close()

	reqID := resp.Header.Get("X-GitHub-Request-Id")
	if reqID == "" {
		t.Fatal("missing X-GitHub-Request-Id header")
	}
}

// TestGHUser verifies GET /api/v3/user returns authenticated user.
func TestGHUser(t *testing.T) {
	resp := ghGet(t, "/api/v3/user", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)

	if data["login"] != "admin" {
		t.Fatalf("expected login=admin, got %v", data["login"])
	}
	if data["id"] == nil {
		t.Fatal("missing id")
	}
	if data["node_id"] == nil {
		t.Fatal("missing node_id")
	}
}

// TestGHUserByLogin verifies GET /api/v3/users/{username}.
func TestGHUserByLogin(t *testing.T) {
	// Existing user
	resp := ghGet(t, "/api/v3/users/admin", "")
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)
	if data["login"] != "admin" {
		t.Fatalf("expected login=admin, got %v", data["login"])
	}

	// Nonexistent user
	resp2 := ghGet(t, "/api/v3/users/nonexistent", "")
	defer resp2.Body.Close()
	if resp2.StatusCode != 404 {
		t.Fatalf("expected 404 for nonexistent user, got %d", resp2.StatusCode)
	}
}

// TestGHGraphQLViewer verifies the viewer query returns the authenticated user.
func TestGHGraphQLViewer(t *testing.T) {
	resp := ghPost(t, "/api/graphql", defaultToken, map[string]string{
		"query": "{viewer{login}}",
	})
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)

	d, _ := data["data"].(map[string]interface{})
	if d == nil {
		t.Fatalf("missing data in response: %v", data)
	}
	viewer, _ := d["viewer"].(map[string]interface{})
	if viewer == nil {
		t.Fatalf("missing viewer in data: %v", d)
	}
	if viewer["login"] != "admin" {
		t.Fatalf("expected login=admin, got %v", viewer["login"])
	}
}

// TestGHGraphQLIntrospection verifies built-in introspection works.
func TestGHGraphQLIntrospection(t *testing.T) {
	resp := ghPost(t, "/api/graphql", defaultToken, map[string]string{
		"query": "{__schema{queryType{name}}}",
	})
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)

	d, _ := data["data"].(map[string]interface{})
	schema, _ := d["__schema"].(map[string]interface{})
	qt, _ := schema["queryType"].(map[string]interface{})
	if qt["name"] != "Query" {
		t.Fatalf("expected queryType.name=Query, got %v", qt["name"])
	}
}

// TestGHGraphQLNoAuth verifies viewer returns null without auth.
func TestGHGraphQLNoAuth(t *testing.T) {
	resp := ghPost(t, "/api/graphql", "", map[string]string{
		"query": "{viewer{login}}",
	})
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)

	d, _ := data["data"].(map[string]interface{})
	if d["viewer"] != nil {
		t.Fatalf("expected null viewer without auth, got %v", d["viewer"])
	}
}

// TestGHDeviceFlow verifies the full device authorization flow.
func TestGHDeviceFlow(t *testing.T) {
	// Step 1: Request device code
	form := url.Values{"client_id": {"test"}, "scope": {"repo"}}
	resp, err := http.Post(testBaseURL+"/login/device/code", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	dcData := decodeJSON(t, resp)

	deviceCode, _ := dcData["device_code"].(string)
	if deviceCode == "" {
		t.Fatal("missing device_code")
	}
	userCode, _ := dcData["user_code"].(string)
	if userCode != "BLEE-PHUB" {
		t.Fatalf("expected user_code=BLEE-PHUB, got %s", userCode)
	}

	// Step 2: Exchange device code for token
	form2 := url.Values{
		"client_id":   {"test"},
		"device_code": {deviceCode},
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
	}
	resp2, err := http.Post(testBaseURL+"/login/oauth/access_token", "application/x-www-form-urlencoded", strings.NewReader(form2.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	if resp2.StatusCode != 200 {
		resp2.Body.Close()
		t.Fatalf("expected 200, got %d", resp2.StatusCode)
	}
	tokenData := decodeJSON(t, resp2)

	accessToken, _ := tokenData["access_token"].(string)
	if accessToken == "" {
		t.Fatal("missing access_token")
	}
	if !strings.HasPrefix(accessToken, "bph_") {
		t.Fatalf("expected bph_ prefix, got %s", accessToken)
	}

	// Step 3: Use the new token to hit /api/v3/
	resp3 := ghGet(t, "/api/v3/", accessToken)
	defer resp3.Body.Close()
	if resp3.StatusCode != 200 {
		t.Fatalf("expected 200 with new token, got %d", resp3.StatusCode)
	}
}

// TestGHErrorFormat verifies 401 error body format.
func TestGHErrorFormat(t *testing.T) {
	resp := ghGet(t, "/api/v3/", "")
	data := decodeJSON(t, resp)

	msg, _ := data["message"].(string)
	if msg != "Bad credentials" {
		t.Fatalf("expected 'Bad credentials', got %q", msg)
	}
	docURL, _ := data["documentation_url"].(string)
	if docURL == "" {
		t.Fatal("missing documentation_url in error response")
	}
}

// TestExistingRoutesUnaffected verifies runner protocol routes still work.
func TestExistingRoutesUnaffected(t *testing.T) {
	// /health
	resp := ghGet(t, "/health", "")
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("/health: expected 200, got %d", resp.StatusCode)
	}

	// /_apis/connectionData
	resp2 := ghGet(t, "/_apis/connectionData", "")
	defer resp2.Body.Close()
	if resp2.StatusCode != 200 {
		t.Fatalf("/_apis/connectionData: expected 200, got %d", resp2.StatusCode)
	}
}
