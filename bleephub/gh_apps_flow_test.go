package bleephub

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// Integration flows over the shared test server for the GitHub Apps
// surface: the manifest flow end-to-end, installation-token permission
// downscoping + repository scoping, suspension blocking live tokens, and
// the org-installations listing.

// noRedirectClient returns redirect responses instead of following them —
// the manifest flow's 302 carries the one-time code we need to read.
var noRedirectClient = &http.Client{
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

func createGitHubAppViaManifest(t *testing.T, name string, perms map[string]string, events []string) map[string]interface{} {
	t.Helper()
	manifest := map[string]interface{}{
		"name":                name,
		"url":                 "https://example.test/app",
		"redirect_url":        "https://example.test/callback",
		"default_permissions": perms,
		"default_events":      events,
	}
	manifestJSON, _ := json.Marshal(manifest)
	form := url.Values{"manifest": {string(manifestJSON)}}
	req, _ := http.NewRequest("POST", testBaseURL+"/settings/apps/new", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "token "+defaultToken)
	resp, err := noRedirectClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("manifest submission: got %d, want 302", resp.StatusCode)
	}
	loc, err := url.Parse(resp.Header.Get("Location"))
	if err != nil {
		t.Fatalf("parse manifest redirect: %v", err)
	}
	code := loc.Query().Get("code")
	if code == "" {
		t.Fatal("manifest redirect carries no code")
	}
	convResp := ghPost(t, "/api/v3/app-manifests/"+code+"/conversions", "", nil)
	if convResp.StatusCode != http.StatusCreated {
		convResp.Body.Close()
		t.Fatalf("manifest conversion: got %d, want 201", convResp.StatusCode)
	}
	return decodeJSON(t, convResp)
}

func installGitHubAppViaBrowser(t *testing.T, appSlug, targetLogin, selection string, repoIDs ...int) map[string]interface{} {
	t.Helper()
	form := url.Values{}
	if targetLogin != "" {
		form.Set("target_login", targetLogin)
	}
	if selection != "" {
		form.Set("repository_selection", selection)
	}
	for _, id := range repoIDs {
		form.Add("repository_ids", fmt.Sprint(id))
	}
	req, _ := http.NewRequest("POST", testBaseURL+"/apps/"+appSlug+"/installations/new", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "token "+defaultToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusCreated {
		resp.Body.Close()
		t.Fatalf("GitHub App browser installation: got %d, want 201", resp.StatusCode)
	}
	return decodeJSON(t, resp)
}

func TestGitHubAppBrowserSettingsListAndManageInstallation(t *testing.T) {
	created := createGitHubAppViaManifest(t, "Settings Managed App", map[string]string{"contents": "read"}, []string{"push"})
	appSlug := created["slug"].(string)

	req, _ := http.NewRequest("GET", testBaseURL+"/settings/apps", nil)
	req.Header.Set("Authorization", "token "+defaultToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /settings/apps status = %d", resp.StatusCode)
	}
	var apps []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&apps); err != nil {
		t.Fatal(err)
	}
	if len(apps) == 0 {
		t.Fatal("settings app list returned no apps")
	}
	found := false
	for _, app := range apps {
		if app["slug"] == appSlug {
			found = true
			if _, leaked := app["pem"]; leaked {
				t.Fatal("settings app list leaked private key")
			}
		}
	}
	if !found {
		t.Fatalf("settings apps = %+v, want created app slug %q", apps, appSlug)
	}

	inst := installGitHubAppViaBrowser(t, appSlug, "admin", "all")
	instID := int(inst["id"].(float64))

	req, _ = http.NewRequest("POST", fmt.Sprintf("%s/settings/installations/%d/suspend", testBaseURL, instID), nil)
	req.Header.Set("Authorization", "token "+defaultToken)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("settings suspend status = %d", resp.StatusCode)
	}
	if got := testServer.store.GetInstallation(instID); got == nil || got.SuspendedAt == nil {
		t.Fatalf("installation %d was not suspended through settings route", instID)
	}

	req, _ = http.NewRequest("POST", fmt.Sprintf("%s/settings/installations/%d/unsuspend", testBaseURL, instID), nil)
	req.Header.Set("Authorization", "token "+defaultToken)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("settings unsuspend status = %d", resp.StatusCode)
	}
	if got := testServer.store.GetInstallation(instID); got == nil || got.SuspendedAt != nil {
		t.Fatalf("installation %d was not unsuspended through settings route", instID)
	}

	req, _ = http.NewRequest("DELETE", fmt.Sprintf("%s/settings/installations/%d", testBaseURL, instID), nil)
	req.Header.Set("Authorization", "token "+defaultToken)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("settings delete status = %d", resp.StatusCode)
	}
	if got := testServer.store.GetInstallation(instID); got != nil {
		t.Fatalf("installation %d still exists after settings delete", instID)
	}
}

func TestAppManifestFlowEndToEnd(t *testing.T) {
	// 1. Submit the manifest form (the github.com/settings/apps/new POST).
	manifest := map[string]interface{}{
		"name":         "manifest-flow-app",
		"url":          "https://example.test/app",
		"redirect_url": "https://example.test/callback",
		"hook_attributes": map[string]interface{}{
			"url": "https://example.test/webhook",
		},
		"default_permissions": map[string]string{"contents": "read", "issues": "write"},
		"default_events":      []string{"push"},
	}
	manifestJSON, _ := json.Marshal(manifest)
	form := url.Values{"manifest": {string(manifestJSON)}}
	req, _ := http.NewRequest("POST", testBaseURL+"/settings/apps/new?state=abc123", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "token "+defaultToken)
	resp, err := noRedirectClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("manifest submission: got %d, want 302", resp.StatusCode)
	}
	loc, err := url.Parse(resp.Header.Get("Location"))
	if err != nil || loc.Host != "example.test" {
		t.Fatalf("redirect location = %q", resp.Header.Get("Location"))
	}
	if loc.Query().Get("state") != "abc123" {
		t.Errorf("state not echoed on redirect: %q", loc.String())
	}
	code := loc.Query().Get("code")
	if code == "" {
		t.Fatal("redirect carries no code")
	}

	// 2. Redeem the code for credentials.
	convResp := ghPost(t, "/api/v3/app-manifests/"+code+"/conversions", "", nil)
	if convResp.StatusCode != http.StatusCreated {
		convResp.Body.Close()
		t.Fatalf("conversion: got %d, want 201", convResp.StatusCode)
	}
	appData := decodeJSON(t, convResp)
	pemKey, _ := appData["pem"].(string)
	if pemKey == "" || appData["client_secret"] == "" || appData["webhook_secret"] == "" {
		t.Fatal("conversion response missing credentials")
	}
	appID := int(appData["id"].(float64))
	if appData["permissions"].(map[string]interface{})["issues"] != "write" {
		t.Errorf("manifest default_permissions not applied: %v", appData["permissions"])
	}

	// One-time code: a second redemption 404s.
	convAgain := ghPost(t, "/api/v3/app-manifests/"+code+"/conversions", "", nil)
	convAgain.Body.Close()
	if convAgain.StatusCode != http.StatusNotFound {
		t.Fatalf("code reuse: got %d, want 404", convAgain.StatusCode)
	}

	// 3. The returned Privacy Enhanced Mail key signs a JSON Web Token that
	// authenticates GET /app.
	jwt, err := signAppJWT(pemKey, appID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	appResp := ghGet(t, "/api/v3/app", jwt)
	if appResp.StatusCode != http.StatusOK {
		appResp.Body.Close()
		t.Fatalf("GET /app with manifest JSON Web Token: got %d, want 200", appResp.StatusCode)
	}
	got := decodeJSON(t, appResp)
	if got["slug"] != "manifest-flow-app" {
		t.Errorf("GET /app slug = %v", got["slug"])
	}
	if got["external_url"] != "https://example.test/app" {
		t.Errorf("external_url = %v, want the manifest url", got["external_url"])
	}

	// 4. The manifest's hook_attributes landed on the app webhook config.
	cfgResp := ghGet(t, "/api/v3/app/hook/config", jwt)
	if cfgResp.StatusCode != http.StatusOK {
		cfgResp.Body.Close()
		t.Fatalf("GET /app/hook/config: got %d", cfgResp.StatusCode)
	}
	cfg := decodeJSON(t, cfgResp)
	if cfg["url"] != "https://example.test/webhook" {
		t.Errorf("hook config url = %v", cfg["url"])
	}
}

// installAppOnOrg provisions an app, an organization owned by admin, an
// organization repository, and an installation carrying the given permissions;
// it returns appID, slug, pem, and instID.
func installAppOnOrg(t *testing.T, orgLogin, repoName string, perms map[string]string) (int, string, string, int) {
	t.Helper()
	createOrgViaAdminAPI(t, orgLogin)
	ghPost(t, "/api/v3/orgs/"+orgLogin+"/repos", defaultToken, map[string]interface{}{"name": repoName}).Body.Close()

	appData := createGitHubAppViaManifest(t, orgLogin+"-app", perms, nil)
	appID := int(appData["id"].(float64))
	pemKey := appData["pem"].(string)
	appSlug := appData["slug"].(string)
	instData := installGitHubAppViaBrowser(t, appSlug, orgLogin, "all")
	return appID, appSlug, pemKey, int(instData["id"].(float64))
}

func TestInstallationTokenDownscoping(t *testing.T) {
	appID, _, pemKey, instID := installAppOnOrg(t, "downscope-org", "scoped-repo",
		map[string]string{"contents": "read", "issues": "write"})
	jwt, err := signAppJWT(pemKey, appID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	tokenPath := fmt.Sprintf("/api/v3/app/installations/%d/access_tokens", instID)

	// Escalation beyond the installation grant → 422.
	esc := ghPost(t, tokenPath, jwt, map[string]interface{}{
		"permissions": map[string]string{"contents": "write"},
	})
	esc.Body.Close()
	if esc.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("escalation: got %d, want 422", esc.StatusCode)
	}

	// Scope the installation doesn't have at all → 422.
	unknown := ghPost(t, tokenPath, jwt, map[string]interface{}{
		"permissions": map[string]string{"deployments": "read"},
	})
	unknown.Body.Close()
	if unknown.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("ungranted scope: got %d, want 422", unknown.StatusCode)
	}

	// Invalid permission level value → 422.
	badLevel := ghPost(t, tokenPath, jwt, map[string]interface{}{
		"permissions": map[string]string{"contents": "sudo"},
	})
	badLevel.Body.Close()
	if badLevel.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("bad level: got %d, want 422", badLevel.StatusCode)
	}

	// Malformed JSON body → 400, not a silently full-permission token.
	malReq, _ := http.NewRequest("POST", testBaseURL+tokenPath, strings.NewReader("{not json"))
	malReq.Header.Set("Authorization", "Bearer "+jwt)
	malResp, err := http.DefaultClient.Do(malReq)
	if err != nil {
		t.Fatal(err)
	}
	malResp.Body.Close()
	if malResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("malformed body: got %d, want 400", malResp.StatusCode)
	}

	// repository_ids outside the installation → 422.
	badRepo := ghPost(t, tokenPath, jwt, map[string]interface{}{
		"repository_ids": []int{99999999},
	})
	badRepo.Body.Close()
	if badRepo.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("foreign repository_ids: got %d, want 422", badRepo.StatusCode)
	}

	// Unknown repository name → 422.
	badName := ghPost(t, tokenPath, jwt, map[string]interface{}{
		"repositories": []string{"no-such-repo"},
	})
	badName.Body.Close()
	if badName.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("unknown repository name: got %d, want 422", badName.StatusCode)
	}

	// Valid downscope (read ≤ write on issues) with a repo subset → 201,
	// repository_selection=selected, and the token's permissions reflect
	// the downscoped set.
	ok := ghPost(t, tokenPath, jwt, map[string]interface{}{
		"permissions":  map[string]string{"issues": "read"},
		"repositories": []string{"scoped-repo"},
	})
	if ok.StatusCode != http.StatusCreated {
		ok.Body.Close()
		t.Fatalf("valid downscope: got %d, want 201", ok.StatusCode)
	}
	tokData := decodeJSON(t, ok)
	if tokData["repository_selection"] != "selected" {
		t.Errorf("repository_selection = %v, want selected", tokData["repository_selection"])
	}
	tokenStr, _ := tokData["token"].(string)
	if !strings.HasPrefix(tokenStr, "ghs_") {
		t.Fatalf("token = %q, want ghs_ prefix", tokenStr)
	}
	perms := tokData["permissions"].(map[string]interface{})
	if perms["issues"] != "read" || len(perms) != 1 {
		t.Errorf("token permissions = %v, want exactly issues:read", perms)
	}

	// The downscoped token cannot write where the level is read-only:
	// requirePerm(scopeIssues, permWrite) gates issue creation.
	issueResp := ghPost(t, "/api/v3/repos/downscope-org/scoped-repo/issues", tokenStr,
		map[string]interface{}{"title": "blocked"})
	issueResp.Body.Close()
	if issueResp.StatusCode != http.StatusForbidden {
		t.Fatalf("downscoped token issue write: got %d, want 403", issueResp.StatusCode)
	}

	// A token carrying the full installation grant (issues:write) passes
	// the same gate — the decorator enforces the TOKEN's permissions, not
	// just the installation's.
	full := ghPost(t, tokenPath, jwt, nil)
	if full.StatusCode != http.StatusCreated {
		full.Body.Close()
		t.Fatalf("full-grant mint: got %d, want 201", full.StatusCode)
	}
	fullToken := decodeJSON(t, full)["token"].(string)
	allowed := ghPost(t, "/api/v3/repos/downscope-org/scoped-repo/issues", fullToken,
		map[string]interface{}{"title": "allowed"})
	allowed.Body.Close()
	if allowed.StatusCode != http.StatusCreated {
		t.Fatalf("full-grant token issue write: got %d, want 201", allowed.StatusCode)
	}
}

func TestInstallationTokenCreatesOrganizationRepositoryWithAdministrationPermission(t *testing.T) {
	orgLogin := "app-create-repo-org"
	createOrgViaAdminAPI(t, orgLogin)

	appData := createGitHubAppViaManifest(t, "Organization Repo Creator App", map[string]string{
		"administration": "write",
		"metadata":       "read",
	}, nil)
	appID := int(appData["id"].(float64))
	pemKey := appData["pem"].(string)
	appSlug := appData["slug"].(string)
	instData := installGitHubAppViaBrowser(t, appSlug, orgLogin, "all")
	instID := int(instData["id"].(float64))
	jwt, err := signAppJWT(pemKey, appID, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	tokenResp := ghPost(t, fmt.Sprintf("/api/v3/app/installations/%d/access_tokens", instID), jwt, nil)
	if tokenResp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(tokenResp.Body)
		tokenResp.Body.Close()
		t.Fatalf("create installation token: got %d body=%s", tokenResp.StatusCode, body)
	}
	tokenBody := decodeJSON(t, tokenResp)
	token, _ := tokenBody["token"].(string)
	if token == "" {
		t.Fatalf("installation token response missing token: %v", tokenBody)
	}

	createResp := ghPost(t, "/api/v3/orgs/"+orgLogin+"/repos", token, map[string]interface{}{
		"name":        "created-by-installation",
		"description": "created through a GitHub App installation token",
	})
	if createResp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(createResp.Body)
		createResp.Body.Close()
		t.Fatalf("create org repo with installation token: got %d body=%s", createResp.StatusCode, body)
	}
	repoBody := decodeJSON(t, createResp)
	if repoBody["full_name"] != orgLogin+"/created-by-installation" {
		t.Fatalf("created repository full_name = %v", repoBody["full_name"])
	}
}

func TestInstallationTokenCreateOrganizationRepositoryRequiresAdministrationWrite(t *testing.T) {
	orgLogin := "app-create-repo-denied-org"
	createOrgViaAdminAPI(t, orgLogin)

	appData := createGitHubAppViaManifest(t, "Organization Repo Metadata App", map[string]string{
		"metadata": "read",
	}, nil)
	appID := int(appData["id"].(float64))
	pemKey := appData["pem"].(string)
	appSlug := appData["slug"].(string)
	instData := installGitHubAppViaBrowser(t, appSlug, orgLogin, "all")
	instID := int(instData["id"].(float64))
	jwt, err := signAppJWT(pemKey, appID, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	tokenResp := ghPost(t, fmt.Sprintf("/api/v3/app/installations/%d/access_tokens", instID), jwt, nil)
	if tokenResp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(tokenResp.Body)
		tokenResp.Body.Close()
		t.Fatalf("create installation token: got %d body=%s", tokenResp.StatusCode, body)
	}
	tokenBody := decodeJSON(t, tokenResp)
	token, _ := tokenBody["token"].(string)
	if token == "" {
		t.Fatalf("installation token response missing token: %v", tokenBody)
	}

	createResp := ghPost(t, "/api/v3/orgs/"+orgLogin+"/repos", token, map[string]interface{}{
		"name": "created-without-administration",
	})
	if createResp.StatusCode != http.StatusForbidden {
		body, _ := io.ReadAll(createResp.Body)
		createResp.Body.Close()
		t.Fatalf("create org repo without administration: got %d body=%s", createResp.StatusCode, body)
	}
	body := decodeJSON(t, createResp)
	if body["message"] != "Resource not accessible by integration" {
		t.Fatalf("forbidden message = %v", body["message"])
	}
}

func TestInstallationSuspensionBlocksTokens(t *testing.T) {
	appID, _, pemKey, instID := installAppOnOrg(t, "suspend-org", "suspend-repo",
		map[string]string{"contents": "read"})
	jwt, err := signAppJWT(pemKey, appID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	tokenPath := fmt.Sprintf("/api/v3/app/installations/%d/access_tokens", instID)

	mint := ghPost(t, tokenPath, jwt, nil)
	if mint.StatusCode != http.StatusCreated {
		mint.Body.Close()
		t.Fatalf("mint: got %d, want 201", mint.StatusCode)
	}
	tokenStr := decodeJSON(t, mint)["token"].(string)

	// Token works before suspension.
	before := ghGet(t, "/api/v3/installation/repositories", tokenStr)
	before.Body.Close()
	if before.StatusCode != http.StatusOK {
		t.Fatalf("pre-suspension: got %d, want 200", before.StatusCode)
	}

	// Suspend through the JSON Web Token-authenticated endpoint.
	susp := ghPut(t, fmt.Sprintf("/api/v3/app/installations/%d/suspended", instID), jwt, nil)
	susp.Body.Close()
	if susp.StatusCode != http.StatusNoContent {
		t.Fatalf("suspend: got %d, want 204", susp.StatusCode)
	}

	// Existing tokens die for the whole application programming interface
	// surface while suspended.
	during := ghGet(t, "/api/v3/installation/repositories", tokenStr)
	during.Body.Close()
	if during.StatusCode != http.StatusForbidden {
		t.Fatalf("suspended token: got %d, want 403", during.StatusCode)
	}
	// Minting fresh tokens is blocked too.
	mint2 := ghPost(t, tokenPath, jwt, nil)
	mint2.Body.Close()
	if mint2.StatusCode != http.StatusForbidden {
		t.Fatalf("mint while suspended: got %d, want 403", mint2.StatusCode)
	}

	// Unsuspend restores the (unexpired) token.
	unsusp := ghDelete(t, fmt.Sprintf("/api/v3/app/installations/%d/suspended", instID), jwt)
	unsusp.Body.Close()
	if unsusp.StatusCode != http.StatusNoContent {
		t.Fatalf("unsuspend: got %d, want 204", unsusp.StatusCode)
	}
	after := ghGet(t, "/api/v3/installation/repositories", tokenStr)
	after.Body.Close()
	if after.StatusCode != http.StatusOK {
		t.Fatalf("post-unsuspension: got %d, want 200", after.StatusCode)
	}

	// Revocation accepts the lowercase bearer scheme octokit sends.
	revReq, _ := http.NewRequest("DELETE", testBaseURL+"/api/v3/installation/token", nil)
	revReq.Header.Set("Authorization", "bearer "+tokenStr)
	revResp, err := http.DefaultClient.Do(revReq)
	if err != nil {
		t.Fatal(err)
	}
	revResp.Body.Close()
	if revResp.StatusCode != http.StatusNoContent {
		t.Fatalf("revoke: got %d, want 204", revResp.StatusCode)
	}
	gone := ghGet(t, "/api/v3/installation/repositories", tokenStr)
	gone.Body.Close()
	if gone.StatusCode != http.StatusUnauthorized {
		t.Fatalf("revoked token: got %d, want 401", gone.StatusCode)
	}
}

func TestOrgInstallationsList(t *testing.T) {
	appID, appSlug, _, instID := installAppOnOrg(t, "instlist-org", "instlist-repo",
		map[string]string{"contents": "read"})

	resp := ghGet(t, "/api/v3/orgs/instlist-org/installations", defaultToken)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("got %d, want 200", resp.StatusCode)
	}
	data := decodeJSON(t, resp)
	if int(data["total_count"].(float64)) != 1 {
		t.Fatalf("total_count = %v, want 1", data["total_count"])
	}
	insts := data["installations"].([]interface{})
	first := insts[0].(map[string]interface{})
	if int(first["id"].(float64)) != instID || int(first["app_id"].(float64)) != appID {
		t.Errorf("installation row = %v", first)
	}

	// Installing the same app twice on one target is rejected.
	form := url.Values{"target_login": {"instlist-org"}}
	req, _ := http.NewRequest("POST", testBaseURL+"/apps/"+appSlug+"/installations/new", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "token "+defaultToken)
	dup, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	dup.Body.Close()
	if dup.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("duplicate installation: got %d, want 422", dup.StatusCode)
	}
}
