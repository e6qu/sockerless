package bleephub

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestMigrations_UserCRUD(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	r1 := testServer.store.CreateRepo(admin, "migration-repo-1", "first migration repo", false)
	r2 := testServer.store.CreateRepo(admin, "migration-repo-2", "second migration repo", false)

	// Start migration
	resp := ghPost(t, "/api/v3/user/migrations", defaultToken, map[string]any{
		"repositories":      []string{r1.FullName, r2.FullName},
		"lock_repositories": true,
	})
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("create user migration: %d %s", resp.StatusCode, b)
	}
	created := decodeJSON(t, resp)
	if created["state"] != "exported" {
		t.Fatalf("expected state exported, got %v", created["state"])
	}
	if len(created["repositories"].([]any)) != 2 {
		t.Fatalf("expected 2 repositories, got %v", created["repositories"])
	}
	migrationID := int(created["id"].(float64))

	// List
	resp = ghGet(t, "/api/v3/user/migrations", defaultToken)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("list user migrations: %d %s", resp.StatusCode, b)
	}
	var list []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	resp.Body.Close()
	found := false
	for _, m := range list {
		if int(m["id"].(float64)) == migrationID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("created migration %d not in list: %v", migrationID, list)
	}

	// Get
	resp = ghGet(t, "/api/v3/user/migrations/"+itoa(migrationID), defaultToken)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("get user migration: %d %s", resp.StatusCode, b)
	}
	got := decodeJSON(t, resp)
	if got["lock_repositories"] != true {
		t.Fatalf("expected lock_repositories true")
	}

	// Download archive
	resp = ghGet(t, "/api/v3/user/migrations/"+itoa(migrationID)+"/archive", defaultToken)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("download archive: %d %s", resp.StatusCode, b)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/gzip" {
		t.Fatalf("expected Content-Type application/gzip, got %s", ct)
	}
	if disp := resp.Header.Get("Content-Disposition"); !strings.Contains(disp, "migration-"+itoa(migrationID)+".tar.gz") {
		t.Fatalf("unexpected Content-Disposition: %s", disp)
	}
	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	tr := tar.NewReader(gz)
	foundMeta := false
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar next: %v", err)
		}
		if h.Name == "metadata.json" {
			foundMeta = true
		}
	}
	resp.Body.Close()
	if !foundMeta {
		t.Fatal("archive missing metadata.json")
	}

	// Delete archive
	resp = ghDelete(t, "/api/v3/user/migrations/"+itoa(migrationID)+"/archive", defaultToken)
	if resp.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("delete archive: %d %s", resp.StatusCode, b)
	}
	resp.Body.Close()

	// Download after delete returns 404
	resp = ghGet(t, "/api/v3/user/migrations/"+itoa(migrationID)+"/archive", defaultToken)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 after archive delete, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Unlock repo
	resp = ghDelete(t, "/api/v3/user/migrations/"+itoa(migrationID)+"/repos/"+r1.Name+"/lock", defaultToken)
	if resp.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("unlock repo: %d %s", resp.StatusCode, b)
	}
	resp.Body.Close()

	// Unlock non-locked repo returns 404
	resp = ghDelete(t, "/api/v3/user/migrations/"+itoa(migrationID)+"/repos/"+r1.Name+"/lock", defaultToken)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 re-unlock, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestMigrations_OrgCRUD(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	org := testServer.store.CreateOrg(admin, "migration-org", "Migration Org", "")
	r1 := testServer.store.CreateOrgRepo(org, admin, "org-repo", "org migration repo", false)

	// Start org migration
	resp := ghPost(t, "/api/v3/orgs/"+org.Login+"/migrations", defaultToken, map[string]any{
		"repositories":      []string{r1.FullName},
		"lock_repositories": true,
	})
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("create org migration: %d %s", resp.StatusCode, b)
	}
	created := decodeJSON(t, resp)
	migrationID := int(created["id"].(float64))

	// List
	resp = ghGet(t, "/api/v3/orgs/"+org.Login+"/migrations", defaultToken)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("list org migrations: %d %s", resp.StatusCode, b)
	}
	var list []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	resp.Body.Close()
	found := false
	for _, m := range list {
		if int(m["id"].(float64)) == migrationID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("created org migration %d not in list: %v", migrationID, list)
	}

	// Get
	resp = ghGet(t, "/api/v3/orgs/"+org.Login+"/migrations/"+itoa(migrationID), defaultToken)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("get org migration: %d %s", resp.StatusCode, b)
	}
	got := decodeJSON(t, resp)
	if got["state"] != "exported" {
		t.Fatalf("expected exported, got %v", got["state"])
	}

	// Get lock status
	resp = ghGet(t, "/api/v3/orgs/"+org.Login+"/migrations/"+itoa(migrationID)+"/repos/"+r1.Name+"/lock", defaultToken)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("get lock status: %d %s", resp.StatusCode, b)
	}
	lockStatus := decodeJSON(t, resp)
	if lockStatus["locked"] != true {
		t.Fatalf("expected locked true, got %v", lockStatus)
	}

	// Unlock
	resp = ghDelete(t, "/api/v3/orgs/"+org.Login+"/migrations/"+itoa(migrationID)+"/repos/"+r1.Name+"/lock", defaultToken)
	if resp.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("unlock org repo: %d %s", resp.StatusCode, b)
	}
	resp.Body.Close()

	// Lock status after unlock returns 404
	resp = ghGet(t, "/api/v3/orgs/"+org.Login+"/migrations/"+itoa(migrationID)+"/repos/"+r1.Name+"/lock", defaultToken)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 after unlock, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestMigrations_404s(t *testing.T) {
	// Missing user migration
	resp := ghGet(t, "/api/v3/user/migrations/999999", defaultToken)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for missing user migration, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Missing org
	resp = ghGet(t, "/api/v3/orgs/nonexistent/migrations", defaultToken)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for missing org, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Missing org migration
	resp = ghGet(t, "/api/v3/orgs/nonexistent/migrations/999999", defaultToken)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for missing org migration, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Unlock on missing migration
	resp = ghDelete(t, "/api/v3/user/migrations/999999/repos/foo/lock", defaultToken)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 unlock missing migration, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestMigrations_StartRequiresAuth(t *testing.T) {
	resp := ghPost(t, "/api/v3/user/migrations", "", map[string]any{"repositories": []string{"a/b"}})
	if resp.StatusCode != http.StatusUnauthorized {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("expected 401 without token, got %d %s", resp.StatusCode, b)
	}
	resp.Body.Close()
}

func TestMigrations_StartValidation(t *testing.T) {
	// Missing repositories
	resp := ghPost(t, "/api/v3/user/migrations", defaultToken, map[string]any{})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("expected 422 missing repos, got %d %s", resp.StatusCode, b)
	}
	resp.Body.Close()

	// Invalid repository
	resp = ghPost(t, "/api/v3/user/migrations", defaultToken, map[string]any{
		"repositories": []string{"does/not-exist"},
	})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("expected 422 invalid repo, got %d %s", resp.StatusCode, b)
	}
	resp.Body.Close()
}

func TestMigrations_OrgMigrationRepositories(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	org := testServer.store.CreateOrg(admin, "migration-repos-org", "Migration Repos Org", "")
	if org == nil {
		t.Fatal("create org failed")
	}
	r1 := testServer.store.CreateOrgRepo(org, admin, "migration-repos-1", "", false)
	r2 := testServer.store.CreateOrgRepo(org, admin, "migration-repos-2", "", false)
	if r1 == nil || r2 == nil {
		t.Fatal("create org repos failed")
	}

	resp := ghPost(t, "/api/v3/orgs/migration-repos-org/migrations", defaultToken, map[string]any{
		"repositories": []string{r1.FullName, r2.FullName},
	})
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("start org migration: %d %s", resp.StatusCode, b)
	}
	created := decodeJSON(t, resp)
	migrationID := int(created["id"].(float64))

	resp = ghGet(t, fmt.Sprintf("/api/v3/orgs/migration-repos-org/migrations/%d/repositories", migrationID), defaultToken)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("list migration repositories: %d", resp.StatusCode)
	}
	repos := decodeJSONArray(t, resp)
	if len(repos) != 2 {
		t.Fatalf("expected 2 migration repositories, got %v", repos)
	}
	names := map[string]bool{}
	for _, repo := range repos {
		fullName, _ := repo["full_name"].(string)
		names[fullName] = true
	}
	if !names[r1.FullName] || !names[r2.FullName] {
		t.Fatalf("migration repositories wrong: %v", names)
	}

	// Unknown migration.
	resp = ghGet(t, "/api/v3/orgs/migration-repos-org/migrations/999999/repositories", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown migration repositories: %d, want 404", resp.StatusCode)
	}
}
