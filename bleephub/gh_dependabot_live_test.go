package bleephub

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

func TestLiveDependabot_AlertsAndSecrets(t *testing.T) {
	repo := createRepoWriteRepo(t, false)
	created := seedDependabotAlert(t, "admin", repo, map[string]any{
		"package_name":             "live-dependabot-lodash",
		"package_ecosystem":        "npm",
		"manifest_path":            "package-lock.json",
		"severity":                 "high",
		"summary":                  "Live test advisory",
		"description":              "A live test vulnerability.",
		"vulnerable_version_range": "< 4.17.21",
		"first_patched_version":    "4.17.21",
	})
	number := int(created["number"].(float64))

	resp := authedGet(t, "/api/v3/repos/admin/"+repo+"/dependabot/alerts")
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("list alerts: %d body=%s", resp.StatusCode, body)
	}
	resp.Body.Close()

	resp = authedGet(t, "/api/v3/repos/admin/"+repo+"/dependabot/alerts/"+itoa(number))
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("get alert: %d body=%s", resp.StatusCode, body)
	}
	resp.Body.Close()

	patch, _ := json.Marshal(map[string]any{"state": "dismissed", "dismissed_reason": "fix_started"})
	req, _ := http.NewRequest("PATCH", testBaseURL+"/api/v3/repos/admin/"+repo+"/dependabot/alerts/"+itoa(number), bytes.NewReader(patch))
	req.Header.Set("Authorization", "Bearer "+defaultToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("patch alert: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("patch alert: %d body=%s", resp.StatusCode, body)
	}
	resp.Body.Close()

	// Repo secret public-key + create
	resp = authedGet(t, "/api/v3/repos/admin/"+repo+"/dependabot/secrets/public-key")
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("repo public-key: %d body=%s", resp.StatusCode, body)
	}
	var pk map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&pk); err != nil {
		t.Fatalf("decode public key: %v", err)
	}
	resp.Body.Close()
	keyID := pk["key_id"].(string)

	put, _ := json.Marshal(map[string]any{
		"encrypted_value": base64.StdEncoding.EncodeToString([]byte("live-encrypted")),
		"key_id":          keyID,
	})
	req, _ = http.NewRequest("PUT", testBaseURL+"/api/v3/repos/admin/"+repo+"/dependabot/secrets/LIVE_SECRET", bytes.NewReader(put))
	req.Header.Set("Authorization", "Bearer "+defaultToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create repo secret: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("create repo secret: %d body=%s", resp.StatusCode, body)
	}
	resp.Body.Close()

	resp = authedGet(t, "/api/v3/repos/admin/"+repo+"/dependabot/secrets")
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("list repo secrets: %d body=%s", resp.StatusCode, body)
	}
	resp.Body.Close()

	// Org secret
	admin := testServer.store.UsersByLogin["admin"]
	org := testServer.store.CreateOrg(admin, "live-dependabot-org", "Live Dependabot Org", "")
	if org == nil {
		t.Fatal("create org failed")
	}
	orgRepo := testServer.store.CreateOrgRepo(org, admin, "live-dependabot-org-repo", "", false)

	resp = authedGet(t, "/api/v3/orgs/live-dependabot-org/dependabot/secrets/public-key")
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("org public-key: %d body=%s", resp.StatusCode, body)
	}
	resp.Body.Close()

	putOrg, _ := json.Marshal(map[string]any{
		"encrypted_value":         base64.StdEncoding.EncodeToString([]byte("live-org-encrypted")),
		"key_id":                  keyID,
		"visibility":              "selected",
		"selected_repository_ids": []int{orgRepo.ID},
	})
	req, _ = http.NewRequest("PUT", testBaseURL+"/api/v3/orgs/live-dependabot-org/dependabot/secrets/LIVE_ORG_SECRET", bytes.NewReader(putOrg))
	req.Header.Set("Authorization", "Bearer "+defaultToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create org secret: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("create org secret: %d body=%s", resp.StatusCode, body)
	}
	resp.Body.Close()

	resp = authedGet(t, "/api/v3/orgs/live-dependabot-org/dependabot/secrets/LIVE_ORG_SECRET/repositories")
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("list selected repos: %d body=%s", resp.StatusCode, body)
	}
	resp.Body.Close()
}
