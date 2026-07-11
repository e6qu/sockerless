package bleephub

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

// --- Unit tests (store + JSON Web Token) ---

func TestAppStoreCreateAndGet(t *testing.T) {
	st := NewStore()
	app := st.CreateApp(1, "My Test App", "A test app", map[string]string{"contents": "read"}, []string{"push"})

	if app.ID == 0 {
		t.Fatal("expected non-zero app ID")
	}
	if app.Slug != "my-test-app" {
		t.Fatalf("expected slug=my-test-app, got %s", app.Slug)
	}
	if app.Name != "My Test App" {
		t.Fatalf("expected name=My Test App, got %s", app.Name)
	}
	if app.PEMPrivateKey == "" {
		t.Fatal("expected PEM private key to be set")
	}
	if !strings.Contains(app.PEMPrivateKey, "RSA PRIVATE KEY") {
		t.Fatal("PEM does not contain RSA PRIVATE KEY header")
	}
	if app.Permissions["contents"] != "read" {
		t.Fatalf("expected permissions[contents]=read, got %s", app.Permissions["contents"])
	}

	// Lookup by ID
	got := st.GetApp(app.ID)
	if got == nil || got.ID != app.ID {
		t.Fatal("GetApp by ID failed")
	}

	// Lookup by slug
	got2 := st.GetAppBySlug("my-test-app")
	if got2 == nil || got2.ID != app.ID {
		t.Fatal("GetAppBySlug failed")
	}

	// Not found
	if st.GetApp(999) != nil {
		t.Fatal("expected nil for nonexistent app ID")
	}
}

func TestInstallationStoreCreateAndList(t *testing.T) {
	st := NewStore()
	app := st.CreateApp(1, "Install App", "", nil, nil)

	inst := st.CreateInstallation(app.ID, "User", 42, "testuser", map[string]string{"issues": "write"}, []string{"issues"})
	if inst == nil {
		t.Fatal("expected installation to be created")
	}
	if inst.AppID != app.ID {
		t.Fatalf("expected AppID=%d, got %d", app.ID, inst.AppID)
	}
	if inst.TargetLogin != "testuser" {
		t.Fatalf("expected TargetLogin=testuser, got %s", inst.TargetLogin)
	}
	if inst.RepositorySelection != "all" {
		t.Fatalf("expected RepositorySelection=all, got %s", inst.RepositorySelection)
	}

	// Create a second installation
	st.CreateInstallation(app.ID, "Organization", 99, "myorg", nil, nil)

	list := st.ListAppInstallations(app.ID)
	if len(list) != 2 {
		t.Fatalf("expected 2 installations, got %d", len(list))
	}

	// GetInstallation
	got := st.GetInstallation(inst.ID)
	if got == nil || got.ID != inst.ID {
		t.Fatal("GetInstallation failed")
	}

	// GetRepoInstallation
	got2 := st.GetRepoInstallation("testuser")
	if got2 == nil || got2.ID != inst.ID {
		t.Fatal("GetRepoInstallation failed")
	}
	if st.GetRepoInstallation("nobody") != nil {
		t.Fatal("expected nil for nonexistent owner")
	}

	// Installation for nonexistent app
	if st.CreateInstallation(999, "User", 1, "x", nil, nil) != nil {
		t.Fatal("expected nil for nonexistent app")
	}
}

func TestInstallationTokenGeneration(t *testing.T) {
	st := NewStore()
	app := st.CreateApp(1, "Token App", "", nil, nil)
	inst := st.CreateInstallation(app.ID, "User", 1, "admin", nil, nil)

	token := st.CreateInstallationToken(inst.ID, app.ID, map[string]string{"contents": "read"}, nil)
	if !strings.HasPrefix(token.Token, "ghs_") {
		t.Fatalf("expected ghs_ prefix, got %s", token.Token)
	}
	if time.Until(token.ExpiresAt) < 59*time.Minute {
		t.Fatal("expected ~1h expiry")
	}
	if token.Permissions["contents"] != "read" {
		t.Fatalf("expected permissions[contents]=read, got %s", token.Permissions["contents"])
	}

	// Lookup
	tok, inst2 := st.LookupInstallationToken(token.Token)
	if tok == nil || inst2 == nil {
		t.Fatal("LookupInstallationToken failed")
	}
	if inst2.ID != inst.ID {
		t.Fatalf("expected installation ID=%d, got %d", inst.ID, inst2.ID)
	}
}

func TestInstallationTokenExpiry(t *testing.T) {
	st := NewStore()
	app := st.CreateApp(1, "Expiry App", "", nil, nil)
	inst := st.CreateInstallation(app.ID, "User", 1, "admin", nil, nil)

	token := st.CreateInstallationToken(inst.ID, app.ID, nil, nil)

	// Force expire
	st.mu.Lock()
	st.InstallationTokens[token.Token].ExpiresAt = time.Now().Add(-1 * time.Hour)
	st.mu.Unlock()

	tok, _ := st.LookupInstallationToken(token.Token)
	if tok != nil {
		t.Fatal("expected expired token to return nil")
	}
}

func TestManifestCodeOneTimeUse(t *testing.T) {
	st := NewStore()
	app := st.CreateApp(1, "Manifest App", "", nil, nil)

	code := st.RegisterManifestCode(app.ID)
	if code == "" {
		t.Fatal("expected non-empty code")
	}

	// First consume succeeds
	appID, ok := st.ConsumeManifestCode(code)
	if !ok || appID != app.ID {
		t.Fatalf("first consume: expected appID=%d ok=true, got %d ok=%v", app.ID, appID, ok)
	}

	// Second consume fails
	_, ok2 := st.ConsumeManifestCode(code)
	if ok2 {
		t.Fatal("expected second consume to fail")
	}
}

func TestJSONWebTokenSignAndVerify(t *testing.T) {
	st := NewStore()
	app := st.CreateApp(1, "JSON Web Token App", "", nil, nil)

	jwt, err := signAppJWT(app.PEMPrivateKey, app.ID, time.Now())
	if err != nil {
		t.Fatalf("signAppJSONWebToken: %v", err)
	}

	got, err := st.parseAndVerifyAppJWT(jwt)
	if err != nil {
		t.Fatalf("parseAndVerifyAppJSONWebToken: %v", err)
	}
	if got.ID != app.ID {
		t.Fatalf("expected app ID=%d, got %d", app.ID, got.ID)
	}
}

func TestJSONWebTokenExpiredRejected(t *testing.T) {
	st := NewStore()
	app := st.CreateApp(1, "Expired JSON Web Token App", "", nil, nil)

	// Sign with a time in the past
	jwt, err := signAppJWT(app.PEMPrivateKey, app.ID, time.Now().Add(-20*time.Minute))
	if err != nil {
		t.Fatalf("signAppJSONWebToken: %v", err)
	}

	_, err = st.parseAndVerifyAppJWT(jwt)
	if err == nil {
		t.Fatal("expected error for expired JSON Web Token")
	}
	if !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expected 'expired' in error, got: %v", err)
	}
}

// TestJSONWebTokenExpWindow pins GitHub's exp-claim window: exp may sit at most 10
// minutes (plus clock-drift tolerance) ahead of the SERVER clock. The
// distance between iat and exp is NOT constrained — a client that backdates
// iat for clock skew (ghinstallation sets iat=now-60) must stay valid.
func TestJSONWebTokenExpWindow(t *testing.T) {
	st := NewStore()
	app := st.CreateApp(1, "Long JSON Web Token App", "", nil, nil)

	block, _ := pemDecode(app.PEMPrivateKey)
	if block == nil {
		t.Fatal("failed to decode PEM")
	}
	privKey, _ := parseRSAKey(block)

	mintJSONWebToken := func(iat, exp int64) string {
		header := testBase64urlEncode([]byte(`{"alg":"RS256","typ":"JWT"}`))
		payload := fmt.Sprintf(`{"iss":"%d","iat":%d,"exp":%d}`, app.ID, iat, exp)
		payloadEnc := testBase64urlEncode([]byte(payload))
		signInput := header + "." + payloadEnc
		hash := sha256Sum([]byte(signInput))
		sig := rsaSign(privKey, hash)
		return signInput + "." + testBase64urlEncode(sig)
	}

	now := time.Now().Unix()

	// exp beyond now+600+drift → rejected.
	if _, err := st.parseAndVerifyAppJWT(mintJSONWebToken(now, now+700)); err == nil {
		t.Fatal("expected error for exp too far in the future")
	} else if !strings.Contains(err.Error(), "too far in the future") {
		t.Fatalf("expected 'too far in the future' in error, got: %v", err)
	}

	// Backdated iat with exp inside the window → valid even though
	// exp-iat exceeds 600 (matches real GitHub).
	if _, err := st.parseAndVerifyAppJWT(mintJSONWebToken(now-300, now+500)); err != nil {
		t.Fatalf("backdated-iat JSON Web Token inside the exp window must verify, got: %v", err)
	}

	// iat in the future beyond drift → rejected.
	if _, err := st.parseAndVerifyAppJWT(mintJSONWebToken(now+120, now+500)); err == nil {
		t.Fatal("expected error for future iat")
	}
}

func TestJSONWebTokenWrongAppIdentifier(t *testing.T) {
	st := NewStore()
	app := st.CreateApp(1, "Wrong ID App", "", nil, nil)

	// Sign with the wrong app identifier.
	jwt, err := signAppJWT(app.PEMPrivateKey, 9999, time.Now())
	if err != nil {
		t.Fatalf("signAppJSONWebToken: %v", err)
	}

	_, err = st.parseAndVerifyAppJWT(jwt)
	if err == nil {
		t.Fatal("expected error for wrong app ID")
	}
	if !strings.Contains(err.Error(), "app not found") {
		t.Fatalf("expected 'app not found' in error, got: %v", err)
	}
}

func TestJSONWebTokenInvalidSignature(t *testing.T) {
	st := NewStore()
	app := st.CreateApp(1, "Bad Sig App", "", nil, nil)

	jwt, err := signAppJWT(app.PEMPrivateKey, app.ID, time.Now())
	if err != nil {
		t.Fatalf("signAppJSONWebToken: %v", err)
	}

	parts := strings.SplitN(jwt, ".", 3)
	if len(parts) != 3 {
		t.Fatalf("expected JSON Web Token to have 3 parts, got %d", len(parts))
	}
	sig, err := base64urlDecode(parts[2])
	if err != nil {
		t.Fatalf("decode JSON Web Token signature: %v", err)
	}
	sig[0] ^= 0xff
	tampered := parts[0] + "." + parts[1] + "." + testBase64urlEncode(sig)

	_, err = st.parseAndVerifyAppJWT(tampered)
	if err == nil {
		t.Fatal("expected error for invalid signature")
	}
}

func TestDeleteInstallation(t *testing.T) {
	st := NewStore()
	app := st.CreateApp(1, "Del App", "", nil, nil)
	inst := st.CreateInstallation(app.ID, "User", 1, "admin", nil, nil)

	ok := st.DeleteInstallation(inst.ID)
	if !ok {
		t.Fatal("expected delete to succeed")
	}
	if st.GetInstallation(inst.ID) != nil {
		t.Fatal("expected installation to be gone")
	}
	if st.DeleteInstallation(inst.ID) {
		t.Fatal("expected second delete to return false")
	}
}

// --- Integration tests (HTTP) ---

func TestCreateAppViaManifest(t *testing.T) {
	data := createGitHubAppViaManifest(t, "Integration Test App",
		map[string]string{"contents": "read", "issues": "write"}, []string{"push", "issues"})

	if data["id"] == nil || data["id"].(float64) == 0 {
		t.Fatal("expected non-zero app ID")
	}
	if data["pem"] == nil || data["pem"].(string) == "" {
		t.Fatal("expected PEM in response")
	}
	if data["slug"] != "integration-test-app" {
		t.Fatalf("expected slug=integration-test-app, got %v", data["slug"])
	}
}

func TestAppManifestInvalidConversionCode404(t *testing.T) {
	resp2 := ghPost(t, "/api/v3/app-manifests/invalid-code/conversions", "", nil)
	if resp2.StatusCode != 404 {
		resp2.Body.Close()
		t.Fatalf("expected 404 for invalid code, got %d", resp2.StatusCode)
	}
	resp2.Body.Close()
}

func TestGetAuthenticatedApp(t *testing.T) {
	appData := createGitHubAppViaManifest(t, "JSON Web Token Auth App", nil, nil)
	appID := int(appData["id"].(float64))
	pem := appData["pem"].(string)

	// Sign the JSON Web Token.
	jwt, err := signAppJWT(pem, appID, time.Now())
	if err != nil {
		t.Fatalf("signAppJSONWebToken: %v", err)
	}

	// GET /app with a JSON Web Token.
	req, _ := http.NewRequest("GET", testBaseURL+"/api/v3/app", nil)
	req.Header.Set("Authorization", "Bearer "+jwt)
	httpResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if httpResp.StatusCode != 200 {
		httpResp.Body.Close()
		t.Fatalf("expected 200, got %d", httpResp.StatusCode)
	}
	data := decodeJSON(t, httpResp)
	if data["name"] != "JSON Web Token Auth App" {
		t.Fatalf("expected name=JSON Web Token Auth App, got %v", data["name"])
	}
	if data["pem"] != nil {
		t.Fatal("PEM should not be in GET /app response")
	}
	// GitHub's GET /app has none of these fields.
	for _, f := range []string{"events_url", "hooks_url", "installations_url"} {
		if _, present := data[f]; present {
			t.Errorf("GET /app must not include %q", f)
		}
	}
	// installations_count is a real integer (no installations here → 0).
	if c, ok := data["installations_count"].(float64); !ok || c != 0 {
		t.Errorf("installations_count = %v, want 0", data["installations_count"])
	}
	// owner is the real owning user (admin), not a hardcoded placeholder.
	owner, _ := data["owner"].(map[string]interface{})
	if owner == nil {
		t.Fatal("missing owner object")
	}
	if owner["login"] != "admin" {
		t.Errorf("owner.login = %v, want admin", owner["login"])
	}
	// owner is the simple-user shape with bleephub's own html_url, not a
	// hardcoded github.com link.
	if owner["html_url"] != "/admin" {
		t.Errorf("owner.html_url = %v, want /admin", owner["html_url"])
	}
	if _, present := owner["node_id"]; !present {
		t.Error("owner missing node_id")
	}
	if _, present := owner["site_admin"]; !present {
		t.Error("owner missing site_admin")
	}
}

func TestGetAuthenticatedAppNoJSONWebToken401(t *testing.T) {
	// GET /app with a personal access token, not a JSON Web Token, should 401.
	resp := ghGet(t, "/api/v3/app", defaultToken)
	if resp.StatusCode != 401 {
		resp.Body.Close()
		t.Fatalf("expected 401 for personal access token authentication on /app, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestCreateInstallationHTTP(t *testing.T) {
	appData := createGitHubAppViaManifest(t, "Install HTTP App", map[string]string{"contents": "read"}, []string{"push"})
	appID := int(appData["id"].(float64))
	appSlug := appData["slug"].(string)

	instData := installGitHubAppViaBrowser(t, appSlug, "admin", "all")
	if instData["app_id"].(float64) != float64(appID) {
		t.Fatalf("expected app_id=%d, got %v", appID, instData["app_id"])
	}
	if instData["repository_selection"] != "all" {
		t.Fatalf("expected repository_selection=all, got %v", instData["repository_selection"])
	}
}

func TestListAppInstallationsHTTP(t *testing.T) {
	appData := createGitHubAppViaManifest(t, "List Inst App", nil, nil)
	appID := int(appData["id"].(float64))
	pem := appData["pem"].(string)

	installGitHubAppViaBrowser(t, appData["slug"].(string), "admin", "all")

	// List via a JSON Web Token.
	jwt, _ := signAppJWT(pem, appID, time.Now())
	req, _ := http.NewRequest("GET", testBaseURL+"/api/v3/app/installations", nil)
	req.Header.Set("Authorization", "Bearer "+jwt)
	httpResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if httpResp.StatusCode != 200 {
		httpResp.Body.Close()
		t.Fatalf("expected 200, got %d", httpResp.StatusCode)
	}
	defer httpResp.Body.Close()
	var list []map[string]interface{}
	json.NewDecoder(httpResp.Body).Decode(&list)
	if len(list) < 1 {
		t.Fatal("expected at least 1 installation")
	}
}

func TestCreateInstallationTokenHTTP(t *testing.T) {
	appData := createGitHubAppViaManifest(t, "Token HTTP App", map[string]string{"contents": "write"}, nil)
	appID := int(appData["id"].(float64))
	pemKey := appData["pem"].(string)

	instData := installGitHubAppViaBrowser(t, appData["slug"].(string), "admin", "all")
	instID := int(instData["id"].(float64))

	// Create an installation token via a JSON Web Token.
	jwt, _ := signAppJWT(pemKey, appID, time.Now())
	req, _ := http.NewRequest("POST", fmt.Sprintf("%s/api/v3/app/installations/%d/access_tokens", testBaseURL, instID), nil)
	req.Header.Set("Authorization", "Bearer "+jwt)
	httpResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if httpResp.StatusCode != 201 {
		httpResp.Body.Close()
		t.Fatalf("expected 201, got %d", httpResp.StatusCode)
	}
	tokData := decodeJSON(t, httpResp)
	tokenStr, _ := tokData["token"].(string)
	if !strings.HasPrefix(tokenStr, "ghs_") {
		t.Fatalf("expected ghs_ prefix, got %s", tokenStr)
	}
	if tokData["expires_at"] == nil {
		t.Fatal("expected expires_at in response")
	}
	// repository_selection reflects the installation (all) when no subset given.
	if tokData["repository_selection"] != "all" {
		t.Errorf("repository_selection = %v, want all", tokData["repository_selection"])
	}
	// No repository subset requested → no repositories array.
	if _, present := tokData["repositories"]; present {
		t.Error("repositories should be absent when no subset requested")
	}
}

func TestInstallationTokenAuth(t *testing.T) {
	appData := createGitHubAppViaManifest(t, "Token Auth App", nil, nil)
	appID := int(appData["id"].(float64))
	pemKey := appData["pem"].(string)

	instData := installGitHubAppViaBrowser(t, appData["slug"].(string), "admin", "all")
	instID := int(instData["id"].(float64))

	jwt, _ := signAppJWT(pemKey, appID, time.Now())
	req, _ := http.NewRequest("POST", fmt.Sprintf("%s/api/v3/app/installations/%d/access_tokens", testBaseURL, instID), nil)
	req.Header.Set("Authorization", "Bearer "+jwt)
	httpResp, _ := http.DefaultClient.Do(req)
	tokData := decodeJSON(t, httpResp)
	ghsToken := tokData["token"].(string)

	// Use the installation token to call a GitHub application programming
	// interface endpoint.
	req2, _ := http.NewRequest("GET", testBaseURL+"/api/v3/user", nil)
	req2.Header.Set("Authorization", "Bearer "+ghsToken)
	resp3, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	if resp3.StatusCode != 200 {
		resp3.Body.Close()
		t.Fatalf("expected 200 with ghs_ token, got %d", resp3.StatusCode)
	}
	userData := decodeJSON(t, resp3)
	login, _ := userData["login"].(string)
	if !strings.Contains(login, "[bot]") {
		t.Fatalf("expected bot login, got %s", login)
	}
}

func TestInstallationTokenWrongApp(t *testing.T) {
	appA := createGitHubAppViaManifest(t, "App A Wrong", nil, nil)

	appB := createGitHubAppViaManifest(t, "App B Wrong", nil, nil)
	appBPEM := appB["pem"].(string)
	appBID := int(appB["id"].(float64))

	// Create an installation for app A.
	instData := installGitHubAppViaBrowser(t, appA["slug"].(string), "admin", "all")
	instAID := int(instData["id"].(float64))

	// Try to create a token for app A's installation using app B's JSON Web Token.
	jwt, _ := signAppJWT(appBPEM, appBID, time.Now())
	req, _ := http.NewRequest("POST", fmt.Sprintf("%s/api/v3/app/installations/%d/access_tokens", testBaseURL, instAID), nil)
	req.Header.Set("Authorization", "Bearer "+jwt)
	httpResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if httpResp.StatusCode != 403 {
		httpResp.Body.Close()
		t.Fatalf("expected 403, got %d", httpResp.StatusCode)
	}
	httpResp.Body.Close()
}

func TestGetRepoInstallationHTTP(t *testing.T) {
	// The endpoint resolves a REAL repo — provision an org-owned repo and
	// install the app on the org.
	createOrgViaAdminAPI(t, "repo-inst-owner")
	ghPost(t, "/api/v3/orgs/repo-inst-owner/repos", defaultToken, map[string]interface{}{
		"name": "somerepo",
	}).Body.Close()

	appData := createGitHubAppViaManifest(t, "Repo Inst App", nil, nil)
	appID := int(appData["id"].(float64))

	installGitHubAppViaBrowser(t, appData["slug"].(string), "repo-inst-owner", "all")

	// GET /repos/{owner}/{repo}/installation
	resp2 := ghGet(t, "/api/v3/repos/repo-inst-owner/somerepo/installation", defaultToken)
	if resp2.StatusCode != 200 {
		resp2.Body.Close()
		t.Fatalf("expected 200, got %d", resp2.StatusCode)
	}
	data := decodeJSON(t, resp2)
	if data["app_id"].(float64) != float64(appID) {
		t.Fatalf("expected app_id=%d, got %v", appID, data["app_id"])
	}

	// Repo that doesn't exist under a covered owner → 404.
	respNoRepo := ghGet(t, "/api/v3/repos/repo-inst-owner/no-such-repo/installation", defaultToken)
	if respNoRepo.StatusCode != 404 {
		respNoRepo.Body.Close()
		t.Fatalf("expected 404 for nonexistent repo, got %d", respNoRepo.StatusCode)
	}
	respNoRepo.Body.Close()

	// Not found
	resp3 := ghGet(t, "/api/v3/repos/nonexistent-owner/somerepo/installation", defaultToken)
	if resp3.StatusCode != 404 {
		resp3.Body.Close()
		t.Fatalf("expected 404 for nonexistent owner, got %d", resp3.StatusCode)
	}
	resp3.Body.Close()
}

func TestDeleteInstallationHTTP(t *testing.T) {
	appData := createGitHubAppViaManifest(t, "Delete Inst App", nil, nil)
	appID := int(appData["id"].(float64))
	pemKey := appData["pem"].(string)

	instData := installGitHubAppViaBrowser(t, appData["slug"].(string), "admin", "all")
	instID := int(instData["id"].(float64))

	// Delete via a JSON Web Token.
	jwt, _ := signAppJWT(pemKey, appID, time.Now())
	req, _ := http.NewRequest("DELETE", fmt.Sprintf("%s/api/v3/app/installations/%d", testBaseURL, instID), nil)
	req.Header.Set("Authorization", "Bearer "+jwt)
	httpResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if httpResp.StatusCode != 204 {
		httpResp.Body.Close()
		t.Fatalf("expected 204, got %d", httpResp.StatusCode)
	}
	httpResp.Body.Close()

	// Verify gone
	req2, _ := http.NewRequest("GET", fmt.Sprintf("%s/api/v3/app/installations/%d", testBaseURL, instID), nil)
	req2.Header.Set("Authorization", "Bearer "+jwt)
	httpResp2, _ := http.DefaultClient.Do(req2)
	if httpResp2.StatusCode != 404 {
		httpResp2.Body.Close()
		t.Fatalf("expected 404 after delete, got %d", httpResp2.StatusCode)
	}
	httpResp2.Body.Close()
}

func TestExistingPersonalAccessTokenAuthUnaffected(t *testing.T) {
	// Verify personal access token authentication still works for existing endpoints.
	resp := ghGet(t, "/api/v3/user", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200 for personal access token authentication, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)
	if data["login"] != "admin" {
		t.Fatalf("expected login=admin, got %v", data["login"])
	}

	// Verify no-auth still returns 401
	resp2 := ghGet(t, "/api/v3/user", "")
	if resp2.StatusCode != 401 {
		resp2.Body.Close()
		t.Fatalf("expected 401 without token, got %d", resp2.StatusCode)
	}
	resp2.Body.Close()
}

// --- Helpers for JSON Web Token lifetime tests ---

func pemDecode(pemStr string) (*pem.Block, []byte) {
	return pem.Decode([]byte(pemStr))
}

func parseRSAKey(block *pem.Block) (*rsa.PrivateKey, error) {
	return x509.ParsePKCS1PrivateKey(block.Bytes)
}

func sha256Sum(data []byte) []byte {
	h := sha256.Sum256(data)
	return h[:]
}

func rsaSign(key *rsa.PrivateKey, hash []byte) []byte {
	sig, _ := rsa.SignPKCS1v15(nil, key, crypto.SHA256, hash)
	return sig
}

// TestJSONWebTokenAlgorithmRejection: only RS256 authenticates /app endpoints. An
// unsigned (alg=none) or HMAC-flavoured token must never resolve to an app,
// no matter how well-formed the claims are.
func TestJSONWebTokenAlgorithmRejection(t *testing.T) {
	st := NewStore()
	app := st.CreateApp(1, "Alg App", "", nil, nil)
	now := time.Now().Unix()
	claims := testBase64urlEncode([]byte(fmt.Sprintf(`{"iss":"%d","iat":%d,"exp":%d}`, app.ID, now, now+540)))

	for _, alg := range []string{"none", "HS256", "RS512"} {
		header := testBase64urlEncode([]byte(`{"alg":"` + alg + `","typ":"JWT"}`))
		jwt := header + "." + claims + "." + testBase64urlEncode([]byte("sig"))
		if _, err := st.parseAndVerifyAppJWT(jwt); err == nil {
			t.Errorf("alg=%s: expected rejection", alg)
		} else if !strings.Contains(err.Error(), "unsupported algorithm") {
			t.Errorf("alg=%s: expected 'unsupported algorithm', got: %v", alg, err)
		}
	}

	// Non-numeric iss never resolves.
	header := testBase64urlEncode([]byte(`{"alg":"RS256","typ":"JWT"}`))
	badIss := testBase64urlEncode([]byte(fmt.Sprintf(`{"iss":"not-a-number","iat":%d,"exp":%d}`, now, now+540)))
	if _, err := st.parseAndVerifyAppJWT(header + "." + badIss + "." + testBase64urlEncode([]byte("sig"))); err == nil {
		t.Error("non-numeric iss: expected rejection")
	}
}
