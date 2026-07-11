package sdktests

import (
	"net/http"
	"testing"

	github "github.com/google/go-github/v88/github"
)

// TestOrganizations provisions an org via GitHub Enterprise Server's public
// site-admin organization API, then reads it back through the typed
// Organizations.Get method, and creates+lists a team.
func TestOrganizations(t *testing.T) {
	org := uniqueName("org")

	if code := createOrganizationViaAdminAPI(t, org, "Test Org", nil); code != http.StatusCreated {
		t.Fatalf("admin organization create status = %d, want 201", code)
	}

	got, _, err := client.Organizations.Get(ctx(), org)
	if err != nil {
		t.Fatalf("Organizations.Get: %v", err)
	}
	if got.GetLogin() != org {
		t.Errorf("org login = %q, want %q", got.GetLogin(), org)
	}
	if got.GetName() != "Test Org" {
		t.Errorf("org name = %q, want Test Org", got.GetName())
	}
	if got.GetType() != "Organization" {
		t.Errorf("org type = %q, want Organization", got.GetType())
	}

	// Teams
	team, _, err := client.Teams.CreateTeam(ctx(), org, github.NewTeam{
		Name:        "engineers",
		Description: github.Ptr("the eng team"),
	})
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	if team.GetName() != "engineers" {
		t.Errorf("team name = %q, want engineers", team.GetName())
	}
	if team.GetSlug() != "engineers" {
		t.Errorf("team slug = %q, want engineers", team.GetSlug())
	}

	teams, _, err := client.Teams.ListTeams(ctx(), org, nil)
	if err != nil {
		t.Fatalf("ListTeams: %v", err)
	}
	found := false
	for _, tm := range teams {
		if tm.GetSlug() == "engineers" {
			found = true
		}
	}
	if !found {
		t.Errorf("ListTeams missing 'engineers'")
	}
}

// TestUsers covers Users.Get("") (authenticated user), CreateKey and ListKeys.
func TestUsers(t *testing.T) {
	// Authenticated user.
	me, _, err := client.Users.Get(ctx(), "")
	if err != nil {
		t.Fatalf("Users.Get(\"\"): %v", err)
	}
	if me.GetLogin() != "admin" {
		t.Errorf("authenticated user login = %q, want admin", me.GetLogin())
	}
	if me.GetID() == 0 {
		t.Error("authenticated user ID is zero")
	}

	// Named user.
	named, _, err := client.Users.Get(ctx(), "admin")
	if err != nil {
		t.Fatalf("Users.Get(admin): %v", err)
	}
	if named.GetLogin() != "admin" {
		t.Errorf("named user login = %q, want admin", named.GetLogin())
	}

	// SSH keys: create + list.
	const pub = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAILq sdk-test-key"
	created, _, err := client.Users.CreateKey(ctx(), &github.Key{
		Title: github.Ptr("sdk-test"),
		Key:   github.Ptr(pub),
	})
	if err != nil {
		t.Fatalf("CreateKey: %v", err)
	}
	if created.GetID() == 0 {
		t.Error("created key ID is zero")
	}
	if created.GetKey() != pub {
		t.Errorf("created key = %q, want %q", created.GetKey(), pub)
	}
	t.Cleanup(func() { _, _ = client.Users.DeleteKey(ctx(), created.GetID()) })

	keys, _, err := client.Users.ListKeys(ctx(), "", nil)
	if err != nil {
		t.Fatalf("ListKeys: %v", err)
	}
	found := false
	for _, k := range keys {
		if k.GetID() == created.GetID() {
			found = true
		}
	}
	if !found {
		t.Error("ListKeys did not include the created key")
	}
}
