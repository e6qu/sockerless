package bleephub

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"
)

var secretScanningSeedCounter uint64

func seedSecretAlert(t *testing.T, owner, repo, secretType string) map[string]any {
	t.Helper()
	n := atomic.AddUint64(&secretScanningSeedCounter, 1)
	path := fmt.Sprintf("config/secrets-%d.txt", n)
	content := fmt.Sprintf("detected=%s\n", secretScanningSeedValue(secretType))
	resp := ghPut(t, "/api/v3/repos/"+owner+"/"+repo+"/contents/"+path, defaultToken, map[string]any{
		"message": "add detected secret",
		"content": base64.StdEncoding.EncodeToString([]byte(content)),
	})
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("put detected secret: %d body=%s", resp.StatusCode, b)
	}
	resp.Body.Close()

	list := decodeJSONArray(t, ghGet(t, "/api/v3/repos/"+owner+"/"+repo+"/secret-scanning/alerts?secret_type="+secretType, defaultToken))
	if len(list) == 0 {
		t.Fatalf("secret scanning did not create %s alert for %s/%s", secretType, owner, repo)
	}
	return list[0]
}

func secretScanningSeedValue(secretType string) string {
	switch secretType {
	case "github_personal_access_token":
		return "ghp_abcdefghijklmnopqrstuvwxyzABCDEFGHIJ"
	case "aws_access_key_id":
		return "AKIA0123456789ABCDEF"
	case "google_api_key":
		return "AIza0123456789abcdefghijklmnopqrstuvwxy"
	case "slack_incoming_webhook_url":
		return "https://hooks.slack.com/" + "services/T00000000/B00000000/XXXXXXXXXXXXXXXXXXXXXXXX"
	default:
		return "ghp_abcdefghijklmnopqrstuvwxyzABCDEFGHIJ"
	}
}

func TestSecretScanningAlertTestsUseCommittedContent(t *testing.T) {
	sources := map[string]string{
		"gh_secret_scanning_test.go": `authedPost("` + `/internal/repos/` + `"+owner+"/"+repo+"/secret-scanning/alerts"`,
		"gh_secret_scanning.go":      `POST /internal/repos/{owner}/{repo}/secret-scanning/alerts`,
	}
	for path, needle := range sources {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), needle) {
			t.Fatalf("%s must not create or register secret scanning alerts through the internal operator route", path)
		}
	}
	body, err := os.ReadFile("gh_secret_scanning_test.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(body)
	if !strings.Contains(source, `"/contents/"+path`) {
		t.Fatal("secret scanning public alert tests must exercise the public contents API ingestion path")
	}
	placeholderNeedle := `authedPost("` + `/internal/repos/` + `"+owner+"/"+repo+"/secret-scanning/push-protection-placeholders"`
	if strings.Contains(source, placeholderNeedle) {
		t.Fatal("secret scanning push-protection tests must create placeholders from protected public writes, not the internal operator route")
	}
}

func TestSecretScanningDetectsGeneratedFineGrainedPersonalAccessToken(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	repo := testServer.store.CreateRepo(admin, "ss-fine-grained-pat", "", true)
	token, err := testServer.store.CreateUserFineGrainedPAT(admin.ID, createPersonalAccessTokenWebRequest{
		Name: "secret scanning live token", ResourceOwner: admin.Login, RepositorySelection: "none",
	})
	if err != nil {
		t.Fatalf("create fine-grained personal access token: %v", err)
	}
	resp := ghPut(t, "/api/v3/repos/admin/"+repo.Name+"/contents/credential.txt", defaultToken, map[string]any{
		"message": "commit fine-grained credential",
		"content": base64.StdEncoding.EncodeToString([]byte("token=" + token.Value + "\n")),
	})
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("commit fine-grained credential: %d body=%s", resp.StatusCode, body)
	}
	resp.Body.Close()
	alerts := decodeJSONArray(t, ghGet(t, "/api/v3/repos/admin/"+repo.Name+"/secret-scanning/alerts?secret_type=github_personal_access_token", defaultToken))
	if len(alerts) != 1 {
		t.Fatalf("generated fine-grained credential alerts = %v, want one", alerts)
	}
}

func createSecretScanningOrgRepoViaPublicAPI(t *testing.T, org, repo string) {
	t.Helper()
	createOrgViaAdminAPI(t, org)
	resp := ghPost(t, "/api/v3/orgs/"+org+"/repos", defaultToken, map[string]any{
		"name":    repo,
		"private": true,
	})
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("create org repo: %d body=%s", resp.StatusCode, b)
	}
	resp.Body.Close()
}

func enableSecretScanningPushProtectionPattern(t *testing.T, org, patternID string) {
	t.Helper()
	resp := ghPatch(t, "/api/v3/orgs/"+org+"/secret-scanning/pattern-configurations", defaultToken, map[string]any{
		"provider_pattern_settings": []map[string]any{
			{"token_type": patternID, "push_protection_setting": "enabled"},
		},
	})
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("enable push protection: %d body=%s", resp.StatusCode, b)
	}
	resp.Body.Close()
}

func secretScanningBlockedPlaceholder(t *testing.T, resp *http.Response) string {
	t.Helper()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("protected write: %d body=%s, want 422", resp.StatusCode, b)
	}
	body := decodeJSON(t, resp)
	placeholderID, _ := body["placeholder_id"].(string)
	if placeholderID == "" {
		t.Fatalf("protected write did not return placeholder_id: %v", body)
	}
	if body["token_type"] != "aws_access_key_id" {
		t.Fatalf("protected write token_type = %v, want aws_access_key_id", body["token_type"])
	}
	return placeholderID
}

func TestSecretScanning_ListAndFilter(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	repo := testServer.store.CreateRepo(admin, "ss-list", "", false)
	if repo == nil {
		t.Fatal("create repo failed")
	}

	seedSecretAlert(t, "admin", "ss-list", "github_personal_access_token")
	seedSecretAlert(t, "admin", "ss-list", "aws_access_key_id")

	resp := authedGet(t, "/api/v3/repos/admin/ss-list/secret-scanning/alerts")
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("list alerts: %d body=%s", resp.StatusCode, b)
	}
	var list []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	resp.Body.Close()
	if len(list) != 2 {
		t.Fatalf("expected 2 alerts, got %d", len(list))
	}

	resp = authedGet(t, "/api/v3/repos/admin/ss-list/secret-scanning/alerts?secret_type=aws_access_key_id")
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("filter by secret_type: %d body=%s", resp.StatusCode, b)
	}
	var filtered []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&filtered); err != nil {
		t.Fatalf("decode filtered: %v", err)
	}
	resp.Body.Close()
	if len(filtered) != 1 {
		t.Fatalf("expected 1 filtered alert, got %d", len(filtered))
	}
}

func TestSecretScanning_GetAndLocations(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	repo := testServer.store.CreateRepo(admin, "ss-get", "", false)
	if repo == nil {
		t.Fatal("create repo failed")
	}

	created := seedSecretAlert(t, "admin", "ss-get", "github_personal_access_token")
	number := int(created["number"].(float64))

	resp := authedGet(t, "/api/v3/repos/admin/ss-get/secret-scanning/alerts/"+itoa(number))
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("get alert: %d body=%s", resp.StatusCode, b)
	}
	var got map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode alert: %v", err)
	}
	resp.Body.Close()
	if got["number"].(float64) != float64(number) {
		t.Fatalf("expected number %d, got %v", number, got["number"])
	}
	if got["state"] != "open" {
		t.Fatalf("expected state open, got %v", got["state"])
	}

	resp = authedGet(t, "/api/v3/repos/admin/ss-get/secret-scanning/alerts/"+itoa(number)+"/locations")
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("get locations: %d body=%s", resp.StatusCode, b)
	}
	var locs []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&locs); err != nil {
		t.Fatalf("decode locations: %v", err)
	}
	resp.Body.Close()
	if len(locs) != 1 {
		t.Fatalf("expected 1 location, got %d", len(locs))
	}
	details, _ := locs[0]["details"].(map[string]any)
	if details["commit_sha"] == "" || details["blob_sha"] == "" {
		t.Fatalf("location did not use real git object identifiers: %v", details)
	}
	if blobURL, _ := details["blob_url"].(string); !strings.Contains(blobURL, "/api/v3/repos/admin/ss-get/git/blobs/") {
		t.Fatalf("location blob_url = %q, want public git blob API URL", blobURL)
	}
}

func TestSecretScanning_GitDatabaseRefCreatesAlert(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	repo := testServer.store.CreateRepo(admin, "ss-git-database", "", false)
	if repo == nil {
		t.Fatal("create repo failed")
	}

	resp := ghPost(t, "/api/v3/repos/admin/ss-git-database/git/blobs", defaultToken, map[string]any{
		"content": "token=" + secretScanningSeedValue("aws_access_key_id") + "\n",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create blob: %d", resp.StatusCode)
	}
	blob := decodeJSON(t, resp)
	resp = ghPost(t, "/api/v3/repos/admin/ss-git-database/git/trees", defaultToken, map[string]any{
		"tree": []map[string]any{
			{"path": "credentials.txt", "mode": "100644", "type": "blob", "sha": blob["sha"]},
		},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create tree: %d", resp.StatusCode)
	}
	tree := decodeJSON(t, resp)
	resp = ghPost(t, "/api/v3/repos/admin/ss-git-database/git/commits", defaultToken, map[string]any{
		"message": "add credentials",
		"tree":    tree["sha"],
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create commit: %d", resp.StatusCode)
	}
	commit := decodeJSON(t, resp)
	resp = ghPost(t, "/api/v3/repos/admin/ss-git-database/git/refs", defaultToken, map[string]any{
		"ref": "refs/heads/main",
		"sha": commit["sha"],
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create branch ref: %d", resp.StatusCode)
	}
	resp.Body.Close()

	alerts := decodeJSONArray(t, ghGet(t, "/api/v3/repos/admin/ss-git-database/secret-scanning/alerts?secret_type=aws_access_key_id", defaultToken))
	if len(alerts) != 1 {
		t.Fatalf("expected one Git Database-ingested alert, got %d: %v", len(alerts), alerts)
	}
	if alerts[0]["secret_type"] != "aws_access_key_id" {
		t.Fatalf("secret_type = %v, want aws_access_key_id", alerts[0]["secret_type"])
	}
}

func TestSecretScanning_PatchResolution(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	repo := testServer.store.CreateRepo(admin, "ss-patch", "", false)
	if repo == nil {
		t.Fatal("create repo failed")
	}

	created := seedSecretAlert(t, "admin", "ss-patch", "github_personal_access_token")
	number := int(created["number"].(float64))

	patch, _ := json.Marshal(map[string]any{"state": "resolved", "resolution": "false_positive"})
	req, _ := http.NewRequest("PATCH", testBaseURL+"/api/v3/repos/admin/ss-patch/secret-scanning/alerts/"+itoa(number), bytes.NewReader(patch))
	req.Header.Set("Authorization", "Bearer "+defaultToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("patch alert: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("patch alert: %d body=%s", resp.StatusCode, b)
	}
	var updated map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&updated); err != nil {
		t.Fatalf("decode patched: %v", err)
	}
	resp.Body.Close()
	if updated["state"] != "resolved" {
		t.Fatalf("expected resolved, got %v", updated["state"])
	}
	if updated["resolution"] != "false_positive" {
		t.Fatalf("expected false_positive, got %v", updated["resolution"])
	}
}

func TestSecretScanning_InvalidResolution(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	repo := testServer.store.CreateRepo(admin, "ss-invalid", "", false)
	if repo == nil {
		t.Fatal("create repo failed")
	}

	created := seedSecretAlert(t, "admin", "ss-invalid", "github_personal_access_token")
	number := int(created["number"].(float64))

	patch, _ := json.Marshal(map[string]any{"state": "resolved", "resolution": "not_a_real_resolution"})
	req, _ := http.NewRequest("PATCH", testBaseURL+"/api/v3/repos/admin/ss-invalid/secret-scanning/alerts/"+itoa(number), bytes.NewReader(patch))
	req.Header.Set("Authorization", "Bearer "+defaultToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("patch alert: %v", err)
	}
	if resp.StatusCode != http.StatusUnprocessableEntity {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("expected 422, got %d body=%s", resp.StatusCode, b)
	}
	resp.Body.Close()
}

func TestSecretScanning_404(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	repo := testServer.store.CreateRepo(admin, "ss-404", "", false)
	if repo == nil {
		t.Fatal("create repo failed")
	}

	resp := authedGet(t, "/api/v3/repos/admin/ss-404/secret-scanning/alerts/999")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = authedGet(t, "/api/v3/repos/admin/does-not-exist/secret-scanning/alerts")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for missing repo, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestSecretScanning_BulkUpdate(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	repo := testServer.store.CreateRepo(admin, "ss-bulk", "", false)
	if repo == nil {
		t.Fatal("create repo failed")
	}

	seedSecretAlert(t, "admin", "ss-bulk", "github_personal_access_token")
	seedSecretAlert(t, "admin", "ss-bulk", "github_personal_access_token")
	seedSecretAlert(t, "admin", "ss-bulk", "aws_access_key_id")

	patch, _ := json.Marshal(map[string]any{"state": "resolved", "resolution": "used_in_tests"})
	req, _ := http.NewRequest("PATCH", testBaseURL+"/api/v3/repos/admin/ss-bulk/secret-scanning/alerts?secret_type=github_personal_access_token", bytes.NewReader(patch))
	req.Header.Set("Authorization", "Bearer "+defaultToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("bulk patch: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("bulk patch: %d body=%s", resp.StatusCode, b)
	}
	var updated []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&updated); err != nil {
		t.Fatalf("decode bulk response: %v", err)
	}
	resp.Body.Close()
	if len(updated) != 2 {
		t.Fatalf("expected 2 updated alerts, got %d", len(updated))
	}
	for _, a := range updated {
		if a["state"] != "resolved" || a["resolution"] != "used_in_tests" {
			t.Fatalf("unexpected updated alert: %+v", a)
		}
	}
}

func TestSecretScanning_OrgAlerts(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	org := testServer.store.CreateOrg(admin, "ss-org-alerts", "SS Org Alerts", "")
	if org == nil {
		t.Fatal("create org failed")
	}
	repo1 := testServer.store.CreateOrgRepo(org, admin, "ss-org-repo1", "", false)
	repo2 := testServer.store.CreateOrgRepo(org, admin, "ss-org-repo2", "", false)
	if repo1 == nil || repo2 == nil {
		t.Fatal("create org repo failed")
	}
	userRepo := testServer.store.CreateRepo(admin, "ss-user-repo", "", false)
	if userRepo == nil {
		t.Fatal("create repo failed")
	}

	seedSecretAlert(t, org.Login, repo1.Name, "github_personal_access_token")
	seedSecretAlert(t, org.Login, repo2.Name, "aws_access_key_id")
	seedSecretAlert(t, "admin", userRepo.Name, "slack_incoming_webhook_url")

	resp := authedGet(t, "/api/v3/orgs/ss-org-alerts/secret-scanning/alerts")
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("list org alerts: %d body=%s", resp.StatusCode, b)
	}
	var list []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode org alerts: %v", err)
	}
	resp.Body.Close()
	if len(list) != 2 {
		t.Fatalf("expected 2 org alerts, got %d", len(list))
	}
}

func TestSecretScanning_PatternConfigurations(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	org := testServer.store.CreateOrg(admin, "ss-patterns", "SS Patterns", "")
	if org == nil {
		t.Fatal("create org failed")
	}

	resp := authedGet(t, "/api/v3/orgs/ss-patterns/secret-scanning/pattern-configurations")
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("list patterns: %d body=%s", resp.StatusCode, b)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode patterns: %v", err)
	}
	resp.Body.Close()
	overrides, ok := body["provider_pattern_overrides"].([]any)
	if !ok || len(overrides) == 0 {
		t.Fatalf("expected provider_pattern_overrides, got %+v", body)
	}
	found := false
	for _, po := range overrides {
		m, ok := po.(map[string]any)
		if !ok {
			continue
		}
		if m["token_type"] == "ghp" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected pattern ghp in %+v", overrides)
	}
}

func TestSecretScanning_PatternConfigurationsUpdate(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	org := testServer.store.CreateOrg(admin, "ss-pattern-org", "SS Pattern Org", "")
	if org == nil {
		t.Fatal("create org failed")
	}

	// Fresh org: no configuration version yet, every pattern not-set.
	resp := ghGet(t, "/api/v3/orgs/ss-pattern-org/secret-scanning/pattern-configurations", defaultToken)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("get pattern configurations: %d", resp.StatusCode)
	}
	initial := decodeJSON(t, resp)
	if initial["pattern_config_version"] != nil {
		t.Fatalf("fresh pattern_config_version = %v, want null", initial["pattern_config_version"])
	}

	// Update the aws pattern's push protection setting.
	resp = ghPatch(t, "/api/v3/orgs/ss-pattern-org/secret-scanning/pattern-configurations", defaultToken, map[string]any{
		"provider_pattern_settings": []map[string]any{
			{"token_type": "aws", "push_protection_setting": "enabled"},
		},
	})
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("patch pattern configurations: %d", resp.StatusCode)
	}
	updated := decodeJSON(t, resp)
	version, _ := updated["pattern_config_version"].(string)
	if version == "" {
		t.Fatalf("update did not return a pattern_config_version: %v", updated)
	}

	// The stored setting and version read back.
	resp = ghGet(t, "/api/v3/orgs/ss-pattern-org/secret-scanning/pattern-configurations", defaultToken)
	after := decodeJSON(t, resp)
	if after["pattern_config_version"] != version {
		t.Fatalf("pattern_config_version = %v, want %s", after["pattern_config_version"], version)
	}
	overrides, _ := after["provider_pattern_overrides"].([]any)
	var awsSetting string
	for _, o := range overrides {
		row, _ := o.(map[string]any)
		if row["token_type"] == "aws" {
			awsSetting, _ = row["setting"].(string)
		}
	}
	if awsSetting != "enabled" {
		t.Fatalf("aws pattern setting = %q, want enabled", awsSetting)
	}

	// Optimistic concurrency: a stale version conflicts, the current one wins.
	resp = ghPatch(t, "/api/v3/orgs/ss-pattern-org/secret-scanning/pattern-configurations", defaultToken, map[string]any{
		"pattern_config_version": "stale-version",
		"provider_pattern_settings": []map[string]any{
			{"token_type": "aws", "push_protection_setting": "disabled"},
		},
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("stale version patch: %d, want 409", resp.StatusCode)
	}
	resp = ghPatch(t, "/api/v3/orgs/ss-pattern-org/secret-scanning/pattern-configurations", defaultToken, map[string]any{
		"pattern_config_version": version,
		"provider_pattern_settings": []map[string]any{
			{"token_type": "aws", "push_protection_setting": "disabled"},
		},
	})
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("current version patch: %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Validation: unknown pattern, invalid setting.
	resp = ghPatch(t, "/api/v3/orgs/ss-pattern-org/secret-scanning/pattern-configurations", defaultToken, map[string]any{
		"provider_pattern_settings": []map[string]any{
			{"token_type": "made-up-pattern", "push_protection_setting": "enabled"},
		},
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("unknown pattern: %d, want 422", resp.StatusCode)
	}
	resp = ghPatch(t, "/api/v3/orgs/ss-pattern-org/secret-scanning/pattern-configurations", defaultToken, map[string]any{
		"provider_pattern_settings": []map[string]any{
			{"token_type": "aws", "push_protection_setting": "sometimes"},
		},
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("invalid setting: %d, want 422", resp.StatusCode)
	}
}

func TestSecretScanning_PushProtectionBypasses(t *testing.T) {
	org := "ss-bypass-org"
	repo := "ss-bypass-repo"
	createSecretScanningOrgRepoViaPublicAPI(t, org, repo)
	enableSecretScanningPushProtectionPattern(t, org, "aws")

	// Unknown placeholder → 404.
	resp := ghPost(t, "/api/v3/repos/"+org+"/"+repo+"/secret-scanning/push-protection-bypasses", defaultToken, map[string]any{
		"reason":         "false_positive",
		"placeholder_id": "no-such-placeholder",
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown placeholder: %d, want 404", resp.StatusCode)
	}

	// A protected public contents write mints a placeholder before it commits.
	resp = ghPut(t, "/api/v3/repos/"+org+"/"+repo+"/contents/config/secret.txt", defaultToken, map[string]any{
		"message": "add protected credential",
		"content": base64.StdEncoding.EncodeToString([]byte("token=" + secretScanningSeedValue("aws_access_key_id") + "\n")),
	})
	placeholderID := secretScanningBlockedPlaceholder(t, resp)

	resp = ghPost(t, "/api/v3/repos/"+org+"/"+repo+"/secret-scanning/push-protection-bypasses", defaultToken, map[string]any{
		"reason":         "used_in_tests",
		"placeholder_id": placeholderID,
	})
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("create bypass: %d", resp.StatusCode)
	}
	bypass := decodeJSON(t, resp)
	if bypass["reason"] != "used_in_tests" || bypass["token_type"] != "aws_access_key_id" {
		t.Fatalf("bypass fields wrong: %v", bypass)
	}
	if expireAt, _ := bypass["expire_at"].(string); expireAt == "" {
		t.Fatalf("bypass missing expire_at: %v", bypass)
	}

	resp = ghPut(t, "/api/v3/repos/"+org+"/"+repo+"/contents/config/secret.txt", defaultToken, map[string]any{
		"message": "add bypassed credential",
		"content": base64.StdEncoding.EncodeToString([]byte("token=" + secretScanningSeedValue("aws_access_key_id") + "\n")),
	})
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("bypassed contents write: %d body=%s", resp.StatusCode, b)
	}
	resp.Body.Close()

	alerts := decodeJSONArray(t, ghGet(t, "/api/v3/repos/"+org+"/"+repo+"/secret-scanning/alerts?secret_type=aws_access_key_id", defaultToken))
	if len(alerts) != 1 {
		t.Fatalf("bypassed contents write did not create alert: %v", alerts)
	}

	// The placeholder is consumed by the bypass endpoint.
	resp = ghPost(t, "/api/v3/repos/"+org+"/"+repo+"/secret-scanning/push-protection-bypasses", defaultToken, map[string]any{
		"reason":         "used_in_tests",
		"placeholder_id": placeholderID,
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("re-bypass consumed placeholder: %d, want 404", resp.StatusCode)
	}

	// Invalid reason.
	resp = ghPost(t, "/api/v3/repos/"+org+"/"+repo+"/secret-scanning/push-protection-bypasses", defaultToken, map[string]any{
		"reason":         "because",
		"placeholder_id": placeholderID,
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("invalid reason: %d, want 422", resp.StatusCode)
	}
}

func TestSecretScanning_PushProtectionBlocksGitDatabaseRefBeforeMutation(t *testing.T) {
	org := "ss-bypass-git-org"
	repo := "ss-bypass-git-repo"
	createSecretScanningOrgRepoViaPublicAPI(t, org, repo)
	enableSecretScanningPushProtectionPattern(t, org, "aws")

	resp := ghPost(t, "/api/v3/repos/"+org+"/"+repo+"/git/blobs", defaultToken, map[string]any{
		"content": "token=" + secretScanningSeedValue("aws_access_key_id") + "\n",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create blob: %d", resp.StatusCode)
	}
	blob := decodeJSON(t, resp)
	resp = ghPost(t, "/api/v3/repos/"+org+"/"+repo+"/git/trees", defaultToken, map[string]any{
		"tree": []map[string]any{
			{"path": "credentials.txt", "mode": "100644", "type": "blob", "sha": blob["sha"]},
		},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create tree: %d", resp.StatusCode)
	}
	tree := decodeJSON(t, resp)
	resp = ghPost(t, "/api/v3/repos/"+org+"/"+repo+"/git/commits", defaultToken, map[string]any{
		"message": "add credentials",
		"tree":    tree["sha"],
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create commit: %d", resp.StatusCode)
	}
	commit := decodeJSON(t, resp)

	refPath := "/api/v3/repos/" + org + "/" + repo + "/git/refs"
	resp = ghPost(t, refPath, defaultToken, map[string]any{
		"ref": "refs/heads/main",
		"sha": commit["sha"],
	})
	placeholderID := secretScanningBlockedPlaceholder(t, resp)

	resp = ghGet(t, "/api/v3/repos/"+org+"/"+repo+"/git/ref/heads/main", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("protected ref create mutated branch: %d, want 404", resp.StatusCode)
	}

	resp = ghPost(t, "/api/v3/repos/"+org+"/"+repo+"/secret-scanning/push-protection-bypasses", defaultToken, map[string]any{
		"reason":         "used_in_tests",
		"placeholder_id": placeholderID,
	})
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("create bypass: %d body=%s", resp.StatusCode, b)
	}
	resp.Body.Close()

	resp = ghPost(t, refPath, defaultToken, map[string]any{
		"ref": "refs/heads/main",
		"sha": commit["sha"],
	})
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("bypassed ref create: %d body=%s", resp.StatusCode, b)
	}
	resp.Body.Close()
}

func TestSecretScanning_ScanHistory(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	if testServer.store.CreateRepo(admin, "ss-history-repo", "", false) == nil {
		t.Fatal("create repo failed")
	}

	// A repository with no recorded scanning activity has an empty history.
	resp := ghGet(t, "/api/v3/repos/admin/ss-history-repo/secret-scanning/scan-history", defaultToken)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("scan history (empty): %d", resp.StatusCode)
	}
	empty := decodeJSON(t, resp)
	for _, key := range []string{"incremental_scans", "pattern_update_scans", "backfill_scans", "custom_pattern_backfill_scans", "generic_secrets_backfill_scans"} {
		scans, ok := empty[key].([]any)
		if !ok || len(scans) != 0 {
			t.Fatalf("expected empty %s, got %v", key, empty[key])
		}
	}

	// Recorded alerts imply completed scans.
	seedSecretAlert(t, "admin", "ss-history-repo", "aws_access_key_id")
	resp = ghGet(t, "/api/v3/repos/admin/ss-history-repo/secret-scanning/scan-history", defaultToken)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("scan history: %d", resp.StatusCode)
	}
	history := decodeJSON(t, resp)
	backfills, _ := history["backfill_scans"].([]any)
	if len(backfills) != 1 {
		t.Fatalf("expected 1 backfill scan, got %v", history["backfill_scans"])
	}
	incrementals, _ := history["incremental_scans"].([]any)
	if len(incrementals) == 0 {
		t.Fatalf("expected incremental scans, got %v", history["incremental_scans"])
	}
	first, _ := incrementals[0].(map[string]any)
	if first["type"] != "incremental" || first["status"] != "completed" {
		t.Fatalf("incremental scan row wrong: %v", first)
	}
	if ts, _ := first["completed_at"].(string); ts == "" {
		t.Fatalf("incremental scan missing completed_at: %v", first)
	}

	// Unknown repository.
	resp = ghGet(t, "/api/v3/repos/admin/no-such-history-repo/secret-scanning/scan-history", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown repo scan history: %d, want 404", resp.StatusCode)
	}
}
