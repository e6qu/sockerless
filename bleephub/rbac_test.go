package bleephub

import (
	"testing"
	"time"
)

func TestSiteAdministratorCanAccessOrganizationRepository(t *testing.T) {
	store := NewStore()
	store.SeedDefaultUser()
	creator := store.LookupUserByLogin("admin")
	org := store.CreateOrg(creator, "site-admin-access", "Site admin access", "")
	if org == nil {
		t.Fatal("CreateOrg returned nil")
	}
	repo := store.CreateOrgRepo(org, creator, "private-repository", "", true)
	if repo == nil {
		t.Fatal("CreateOrgRepo returned nil")
	}

	store.mu.Lock()
	siteAdmin := &User{
		ID:           store.NextUser,
		Login:        "external-site-admin",
		Type:         "User",
		SiteAdmin:    true,
		StarredRepos: map[string]bool{},
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
	store.NextUser++
	store.Users[siteAdmin.ID] = siteAdmin
	store.UsersByLogin[siteAdmin.Login] = siteAdmin
	store.mu.Unlock()

	if !canReadRepo(store, siteAdmin, repo) {
		t.Fatal("site administrator could not read organization repository")
	}
	if !canPushRepo(store, siteAdmin, repo) {
		t.Fatal("site administrator could not push organization repository")
	}
	if !canAdminRepo(store, siteAdmin, repo) {
		t.Fatal("site administrator could not administer organization repository")
	}
}
