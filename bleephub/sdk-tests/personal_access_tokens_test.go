package sdktests

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	github "github.com/google/go-github/v88/github"
)

// TestFineGrainedPersonalAccessTokenAdministration creates the credential
// through GitHub's browser settings-shaped producer workflow, then drives the
// GitHub App-only organization request and grant reads and approval through
// the official go-github software development kit.
func TestFineGrainedPersonalAccessTokenAdministration(t *testing.T) {
	org := uniqueName("pat-sdk")
	if code := createOrganizationViaAdminAPI(t, org, "Personal access token SDK", nil); code != http.StatusCreated {
		t.Fatalf("create organization status = %d", code)
	}
	body, _ := json.Marshal(map[string]interface{}{
		"name": "SDK release token", "resource_owner": org, "repository_selection": "none",
		"permissions": map[string]interface{}{"organization": map[string]string{"members": "read"}},
		"reason":      "official SDK lifecycle",
	})
	req, _ := http.NewRequest(http.MethodPost, baseURL+"/settings/personal-access-tokens", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := rawHTTP.Do(req)
	if err != nil {
		t.Fatalf("create fine-grained personal access token: %v", err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create fine-grained personal access token status = %d body=%s", resp.StatusCode, raw)
	}

	permissions := map[string]string{
		"organization_personal_access_token_requests": "write",
		"organization_personal_access_tokens":         "read",
	}
	var app struct {
		ID   int64  `json:"id"`
		Slug string `json:"slug"`
		PEM  string `json:"pem"`
	}
	if code := createGitHubAppViaManifest(t, uniqueName("pat-admin-app"), permissions, &app); code != http.StatusCreated {
		t.Fatalf("create GitHub App status = %d", code)
	}
	var installation struct {
		ID int64 `json:"id"`
	}
	if code := installGitHubAppViaBrowser(t, app.Slug, org, "all", nil, &installation); code != http.StatusCreated {
		t.Fatalf("install GitHub App status = %d", code)
	}
	appClient := ghClient(t, signAppJWT(t, app.PEM, app.ID))
	installationToken, _, err := appClient.Apps.CreateInstallationToken(ctx(), installation.ID, &github.InstallationTokenOptions{
		Permissions: &github.InstallationPermissions{
			OrganizationPersonalAccessTokenRequests: github.Ptr("write"),
			OrganizationPersonalAccessTokens:        github.Ptr("read"),
		},
	})
	if err != nil {
		t.Fatalf("Apps.CreateInstallationToken: %v", err)
	}
	orgClient := ghClient(t, installationToken.GetToken())
	requests, _, err := orgClient.Organizations.ListFineGrainedPersonalAccessTokenRequests(ctx(), org, &github.ListFineGrainedPATOptions{})
	if err != nil {
		t.Fatalf("Organizations.ListFineGrainedPersonalAccessTokenRequests: %v", err)
	}
	if len(requests) != 1 || requests[0].GetTokenName() != "SDK release token" || requests[0].GetReason() != "official SDK lifecycle" {
		t.Fatalf("pending requests = %+v", requests)
	}
	if _, err := orgClient.Organizations.ReviewPersonalAccessTokenRequest(ctx(), org, requests[0].GetID(), github.ReviewPersonalAccessTokenRequestOptions{Action: "approve"}); err != nil {
		t.Fatalf("Organizations.ReviewPersonalAccessTokenRequest: %v", err)
	}
	grants, _, err := orgClient.Organizations.ListFineGrainedPersonalAccessTokens(ctx(), org, &github.ListFineGrainedPATOptions{})
	if err != nil {
		t.Fatalf("Organizations.ListFineGrainedPersonalAccessTokens: %v", err)
	}
	if len(grants) != 1 || grants[0].GetTokenName() != "SDK release token" {
		t.Fatalf("approved grants = %+v", grants)
	}
}
