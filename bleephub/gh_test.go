package bleephub

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"
)

const defaultToken = "bleephub-admin-token-00000000000000000000"

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

func TestUnknownRoutesDoNotReturnSuccess(t *testing.T) {
	resp := ghGet(t, "/api/v3/definitely-not-a-route", defaultToken)
	if resp.StatusCode != http.StatusNotFound {
		resp.Body.Close()
		t.Fatalf("GitHub API unknown route status = %d, want 404", resp.StatusCode)
	}
	data := decodeJSON(t, resp)
	if data["message"] != "Not Found" {
		t.Fatalf("GitHub API unknown route message = %v, want Not Found", data["message"])
	}

	resp = ghGet(t, "/definitely-not-a-route", "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("non-API unknown route status = %d, want 404", resp.StatusCode)
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
	admin := testServer.store.LookupUserByLogin("admin")
	if admin == nil {
		t.Fatal("admin user not seeded")
	}
	app := testServer.store.CreateOAuthApp(admin.ID, "device flow test", "", "https://example.test", "http://callback/")

	// Step 1: Request device code
	form := url.Values{"client_id": {app.ClientID}, "scope": {"repo"}}
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
	if userCode == "" || !strings.Contains(userCode, "-") {
		t.Fatalf("expected non-empty formatted user_code, got %s", userCode)
	}

	// Step 2: Polling before browser approval returns authorization_pending.
	form2 := url.Values{
		"client_id":   {app.ClientID},
		"device_code": {deviceCode},
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
	}
	tokReq, _ := http.NewRequest("POST", testBaseURL+"/login/oauth/access_token", strings.NewReader(form2.Encode()))
	tokReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	tokReq.Header.Set("Accept", "application/json") // real clients (gh CLI) negotiate JSON
	resp2, err := http.DefaultClient.Do(tokReq)
	if err != nil {
		t.Fatal(err)
	}
	if resp2.StatusCode != 200 {
		resp2.Body.Close()
		t.Fatalf("expected 200, got %d", resp2.StatusCode)
	}
	pendingData := decodeJSON(t, resp2)
	if pendingData["error"] != "authorization_pending" {
		t.Fatalf("expected authorization_pending before approval, got %v", pendingData)
	}

	// Step 3: Sign in through the browser flow and approve the displayed user code.
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}
	loginForm := url.Values{"login": {"admin"}, "password": {defaultToken}}
	loginResp, err := client.Post(testBaseURL+"/login", "application/x-www-form-urlencoded", strings.NewReader(loginForm.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	_ = loginResp.Body.Close()
	if loginResp.StatusCode != http.StatusOK && loginResp.StatusCode != http.StatusFound {
		t.Fatalf("login status = %d", loginResp.StatusCode)
	}
	approveForm := url.Values{"user_code": {userCode}}
	approveResp, err := client.Post(testBaseURL+"/login/device", "application/x-www-form-urlencoded", strings.NewReader(approveForm.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	_ = approveResp.Body.Close()
	if approveResp.StatusCode != http.StatusOK {
		t.Fatalf("device approve status = %d", approveResp.StatusCode)
	}

	// Step 4: Exchange device code for token.
	tokReq, _ = http.NewRequest("POST", testBaseURL+"/login/oauth/access_token", strings.NewReader(form2.Encode()))
	tokReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	tokReq.Header.Set("Accept", "application/json")
	resp2, err = http.DefaultClient.Do(tokReq)
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
	if !strings.HasPrefix(accessToken, "ghp_") && !strings.HasPrefix(accessToken, "gho_") {
		t.Fatalf("expected ghp_ or gho_ prefix, got %s", accessToken)
	}

	// Step 5: Use the new token to hit /api/v3/
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
	health := decodeJSON(t, resp)
	if health["enterprise_slug"] != defaultEnterpriseSlug {
		t.Fatalf("/health enterprise_slug = %v, want %s", health["enterprise_slug"], defaultEnterpriseSlug)
	}

	// /_apis/connectionData
	resp2 := ghGet(t, "/_apis/connectionData", "")
	defer resp2.Body.Close()
	if resp2.StatusCode != 200 {
		t.Fatalf("/_apis/connectionData: expected 200, got %d", resp2.StatusCode)
	}
}
