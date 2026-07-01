package bleephub

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

// Live-server shape test for the migrations surface. Exercises the JSON
// endpoints through the shared TestMain server so the OpenAPI response-shape
// validator observes them.
func TestLiveMigrations_UserAndOrg(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	repo := testServer.store.CreateRepo(admin, "live-migration-repo", "live", false)
	org := testServer.store.CreateOrg(admin, "live-migration-org", "Live Org", "")
	orgRepo := testServer.store.CreateOrgRepo(org, admin, "live-org-repo", "live org", false)

	// Start user migration
	body, _ := json.Marshal(map[string]any{
		"repositories":      []string{repo.FullName},
		"lock_repositories": true,
	})
	resp, err := authedPost("/api/v3/user/migrations", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("create user migration: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("create user migration: %d %s", resp.StatusCode, b)
	}
	var userMig map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&userMig); err != nil {
		t.Fatalf("decode user migration: %v", err)
	}
	resp.Body.Close()
	userID := int(userMig["id"].(float64))

	// List user migrations
	resp = authedGet(t, "/api/v3/user/migrations")
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("list user migrations: %d %s", resp.StatusCode, b)
	}
	json.NewDecoder(resp.Body).Decode(&[]map[string]any{})
	resp.Body.Close()

	// Get user migration
	resp = authedGet(t, "/api/v3/user/migrations/"+itoa(userID))
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("get user migration: %d %s", resp.StatusCode, b)
	}
	resp.Body.Close()

	// Start org migration
	body, _ = json.Marshal(map[string]any{
		"repositories":      []string{orgRepo.FullName},
		"lock_repositories": true,
	})
	resp, err = authedPost("/api/v3/orgs/"+org.Login+"/migrations", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("create org migration: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("create org migration: %d %s", resp.StatusCode, b)
	}
	var orgMig map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&orgMig); err != nil {
		t.Fatalf("decode org migration: %v", err)
	}
	resp.Body.Close()
	orgMigID := int(orgMig["id"].(float64))

	// List org migrations
	resp = authedGet(t, "/api/v3/orgs/"+org.Login+"/migrations")
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("list org migrations: %d %s", resp.StatusCode, b)
	}
	json.NewDecoder(resp.Body).Decode(&[]map[string]any{})
	resp.Body.Close()

	// Get org migration
	resp = authedGet(t, "/api/v3/orgs/"+org.Login+"/migrations/"+itoa(orgMigID))
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("get org migration: %d %s", resp.StatusCode, b)
	}
	resp.Body.Close()

	// Get lock status
	resp = authedGet(t, "/api/v3/orgs/"+org.Login+"/migrations/"+itoa(orgMigID)+"/repos/"+orgRepo.Name+"/lock")
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("get org lock status: %d %s", resp.StatusCode, b)
	}
	resp.Body.Close()
}
