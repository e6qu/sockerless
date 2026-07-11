package sdktests

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"net/http"
	"testing"
	"time"

	github "github.com/google/go-github/v88/github"
)

// signAppJWT mints the RS256 JSON Web Token a real GitHub App client sends,
// from the Privacy Enhanced Mail key Bleephub returned at app creation.
func signAppJWT(t *testing.T, privateKeyPEM string, appID int64) string {
	t.Helper()
	block, _ := pem.Decode([]byte(privateKeyPEM))
	if block == nil {
		t.Fatal("failed to decode app PEM")
	}
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		t.Fatalf("parse app key: %v", err)
	}
	b64 := func(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }
	now := time.Now().Unix()
	// iat backdated 60s for clock drift — the convention real app clients use.
	header := b64([]byte(`{"alg":"RS256","typ":"JWT"}`))
	payload := b64([]byte(fmt.Sprintf(`{"iss":"%d","iat":%d,"exp":%d}`, appID, now-60, now+540)))
	hash := sha256.Sum256([]byte(header + "." + payload))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hash[:])
	if err != nil {
		t.Fatalf("sign app JSON Web Token: %v", err)
	}
	return header + "." + payload + "." + b64(sig)
}

// ghClient builds a go-github client against Bleephub authenticated with the
// given bearer credential.
func ghClient(t *testing.T, credential string) *github.Client {
	t.Helper()
	c, err := github.NewClient(
		github.WithAuthToken(credential),
		github.WithEnterpriseURLs(baseURL+"/", baseURL+"/"),
	)
	if err != nil {
		t.Fatalf("new go-github client: %v", err)
	}
	return c
}

// TestAppsInstallationTokenFlow drives the app-auth lifecycle with the typed
// software development kit: JSON Web Token-authenticated installation listing,
// token minting with permission downscoping and repository scoping, using the
// minted installation token through the software development kit, and the 422
// on permission escalation.
func TestAppsInstallationTokenFlow(t *testing.T) {
	org := uniqueName("appflow")
	if code := createOrganizationViaAdminAPI(t, org, "", nil); code != http.StatusCreated {
		t.Fatalf("admin organization create status = %d, want 201", code)
	}
	repo, _, err := client.Repositories.Create(ctx(), org, &github.Repository{Name: github.Ptr("flow-repo")})
	if err != nil {
		t.Fatalf("Repositories.Create: %v", err)
	}

	var created struct {
		ID   int64  `json:"id"`
		Slug string `json:"slug"`
		PEM  string `json:"pem"`
	}
	if code := createGitHubAppViaManifest(t, uniqueName("flow-app"),
		map[string]string{"contents": "read", "issues": "write"}, &created); code != http.StatusCreated {
		t.Fatalf("GitHub App manifest conversion status = %d, want 201", code)
	}
	var inst struct {
		ID int64 `json:"id"`
	}
	if code := installGitHubAppViaBrowser(t, created.Slug, org, "all", nil, &inst); code != http.StatusCreated {
		t.Fatalf("GitHub App browser installation status = %d", code)
	}

	appClient := ghClient(t, signAppJWT(t, created.PEM, created.ID))

	// JSON Web Token-authenticated installation listing sees the installation.
	installations, _, err := appClient.Apps.ListInstallations(ctx(), nil)
	if err != nil {
		t.Fatalf("Apps.ListInstallations: %v", err)
	}
	if len(installations) != 1 || installations[0].GetID() != inst.ID {
		t.Fatalf("ListInstallations = %v, want the one installation id=%d", installations, inst.ID)
	}

	// Escalation beyond the grant is a 422 the software development kit surfaces as ErrorResponse.
	_, resp, err := appClient.Apps.CreateInstallationToken(ctx(), inst.ID, &github.InstallationTokenOptions{
		Permissions: &github.InstallationPermissions{Contents: github.Ptr("write")},
	})
	if err == nil || resp == nil || resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("escalating CreateInstallationToken: err=%v status=%v, want 422", err, resp)
	}

	// Downscoped mint with a repository subset.
	tok, _, err := appClient.Apps.CreateInstallationToken(ctx(), inst.ID, &github.InstallationTokenOptions{
		Permissions:  &github.InstallationPermissions{Issues: github.Ptr("read")},
		Repositories: []string{repo.GetName()},
	})
	if err != nil {
		t.Fatalf("CreateInstallationToken: %v", err)
	}
	if tok.GetToken() == "" || tok.GetExpiresAt().IsZero() {
		t.Fatalf("installation token missing token/expires_at: %+v", tok)
	}
	if got := time.Until(tok.GetExpiresAt().Time); got < 55*time.Minute || got > 65*time.Minute {
		t.Errorf("token TTL = %v, want ~1h", got)
	}
	if len(tok.GetRepositories()) != 1 || tok.GetRepositories()[0].GetName() != "flow-repo" {
		t.Errorf("token repositories = %+v, want exactly flow-repo", tok.GetRepositories())
	}

	// The installation token works through the software development kit and
	// sees exactly the scoped repository.
	instClient := ghClient(t, tok.GetToken())
	repos, _, err := instClient.Apps.ListRepos(ctx(), nil)
	if err != nil {
		t.Fatalf("Apps.ListRepos with installation token: %v", err)
	}
	if repos.GetTotalCount() != 1 || repos.Repositories[0].GetName() != "flow-repo" {
		t.Fatalf("installation repos = %+v, want exactly flow-repo", repos)
	}

	// Org-side installation listing through the typed client.
	orgInsts, _, err := client.Organizations.ListInstallations(ctx(), org, nil)
	if err != nil {
		t.Fatalf("Organizations.ListInstallations: %v", err)
	}
	if orgInsts.GetTotalCount() != 1 || orgInsts.Installations[0].GetID() != inst.ID {
		t.Fatalf("org installations = %+v, want the one installation", orgInsts)
	}
}

// TestOrgProfileTeamsAndMembershipSurfaces drives the org profile PATCH, the
// global org list, public-membership toggles, team hierarchy + maintainer
// role, and team repositories through the typed software development kit.
func TestOrgProfileTeamsAndMembershipSurfaces(t *testing.T) {
	org := uniqueName("orgsurf")
	if code := createOrganizationViaAdminAPI(t, org, "", nil); code != http.StatusCreated {
		t.Fatalf("admin organization create status = %d, want 201", code)
	}

	// Profile fields round-trip through Organizations.Edit.
	edited, _, err := client.Organizations.Edit(ctx(), org, &github.Organization{
		Company:               github.Ptr("ACME"),
		Location:              github.Ptr("Utrecht"),
		BillingEmail:          github.Ptr("bill@example.test"),
		DefaultRepoPermission: github.Ptr("write"),
	})
	if err != nil {
		t.Fatalf("Organizations.Edit: %v", err)
	}
	if edited.GetCompany() != "ACME" || edited.GetLocation() != "Utrecht" ||
		edited.GetBillingEmail() != "bill@example.test" || edited.GetDefaultRepoPermission() != "write" {
		t.Errorf("edited org = company %q location %q billing %q perm %q",
			edited.GetCompany(), edited.GetLocation(), edited.GetBillingEmail(), edited.GetDefaultRepoPermission())
	}

	// Global list includes the org.
	all, _, err := client.Organizations.ListAll(ctx(), nil)
	if err != nil {
		t.Fatalf("Organizations.ListAll: %v", err)
	}
	found := false
	for _, o := range all {
		if o.GetLogin() == org {
			found = true
		}
	}
	if !found {
		t.Error("ListAll missing the created org")
	}

	// Membership checks + public-member toggles for the admin creator.
	if member, _, err := client.Organizations.IsMember(ctx(), org, "admin"); err != nil || !member {
		t.Fatalf("IsMember(admin) = %v, %v; want true", member, err)
	}
	if pub, _, err := client.Organizations.IsPublicMember(ctx(), org, "admin"); err != nil || pub {
		t.Fatalf("IsPublicMember before publicize = %v, %v; want false", pub, err)
	}
	if _, err := client.Organizations.PublicizeMembership(ctx(), org, "admin"); err != nil {
		t.Fatalf("PublicizeMembership: %v", err)
	}
	if pub, _, err := client.Organizations.IsPublicMember(ctx(), org, "admin"); err != nil || !pub {
		t.Fatalf("IsPublicMember after publicize = %v, %v; want true", pub, err)
	}
	if _, err := client.Organizations.ConcealMembership(ctx(), org, "admin"); err != nil {
		t.Fatalf("ConcealMembership: %v", err)
	}

	// Team hierarchy: parent + child via ParentTeamID, child listing.
	parent, _, err := client.Teams.CreateTeam(ctx(), org, github.NewTeam{Name: "platform", Permission: github.Ptr("push")})
	if err != nil {
		t.Fatalf("CreateTeam(parent): %v", err)
	}
	child, _, err := client.Teams.CreateTeam(ctx(), org, github.NewTeam{
		Name:         "platform-core",
		ParentTeamID: parent.ID,
	})
	if err != nil {
		t.Fatalf("CreateTeam(child): %v", err)
	}
	if child.GetParent().GetID() != parent.GetID() {
		t.Errorf("child parent = %v, want %d", child.GetParent(), parent.GetID())
	}
	children, _, err := client.Teams.ListChildTeamsByParentSlug(ctx(), org, "platform", nil)
	if err != nil {
		t.Fatalf("ListChildTeamsByParentSlug: %v", err)
	}
	if len(children) != 1 || children[0].GetSlug() != "platform-core" {
		t.Errorf("child teams = %v", children)
	}

	// Maintainer role round-trips through team-membership endpoints.
	tm, _, err := client.Teams.AddTeamMembershipBySlug(ctx(), org, "platform", "admin",
		&github.TeamAddTeamMembershipOptions{Role: "maintainer"})
	if err != nil {
		t.Fatalf("AddTeamMembershipBySlug: %v", err)
	}
	if tm.GetRole() != "maintainer" || tm.GetState() != "active" {
		t.Errorf("team membership = role %q state %q, want maintainer/active", tm.GetRole(), tm.GetState())
	}
	got, _, err := client.Teams.GetTeamMembershipBySlug(ctx(), org, "platform", "admin")
	if err != nil {
		t.Fatalf("GetTeamMembershipBySlug: %v", err)
	}
	if got.GetRole() != "maintainer" {
		t.Errorf("read-back team role = %q, want maintainer", got.GetRole())
	}

	// Team repos: add, list (role_name reflects the team permission), check
	// via the repository media type, remove.
	if _, _, err := client.Repositories.Create(ctx(), org, &github.Repository{Name: github.Ptr("team-managed")}); err != nil {
		t.Fatalf("Repositories.Create: %v", err)
	}
	if _, err := client.Teams.AddTeamRepoBySlug(ctx(), org, "platform", org, "team-managed", nil); err != nil {
		t.Fatalf("AddTeamRepoBySlug: %v", err)
	}
	teamRepos, _, err := client.Teams.ListTeamReposBySlug(ctx(), org, "platform", nil)
	if err != nil {
		t.Fatalf("ListTeamReposBySlug: %v", err)
	}
	if len(teamRepos) != 1 || teamRepos[0].GetName() != "team-managed" {
		t.Fatalf("team repos = %v", teamRepos)
	}
	if teamRepos[0].GetRoleName() != "write" {
		t.Errorf("role_name = %q, want write", teamRepos[0].GetRoleName())
	}
	managed, _, err := client.Teams.IsTeamRepoBySlug(ctx(), org, "platform", org, "team-managed")
	if err != nil {
		t.Fatalf("IsTeamRepoBySlug: %v", err)
	}
	if managed.GetFullName() != org+"/team-managed" {
		t.Errorf("IsTeamRepoBySlug repo = %q", managed.GetFullName())
	}
	if _, err := client.Teams.RemoveTeamRepoBySlug(ctx(), org, "platform", org, "team-managed"); err != nil {
		t.Fatalf("RemoveTeamRepoBySlug: %v", err)
	}
}

// TestOrgWebhooksSDK drives organization webhook create, read, update, and
// delete through the typed software development kit.
func TestOrgWebhooksSDK(t *testing.T) {
	org := uniqueName("orghook")
	if code := createOrganizationViaAdminAPI(t, org, "", nil); code != http.StatusCreated {
		t.Fatalf("admin organization create status = %d, want 201", code)
	}

	hook, _, err := client.Organizations.CreateHook(ctx(), org, &github.Hook{
		Name:   github.Ptr("web"),
		Events: []string{"push", "organization"},
		Config: &github.HookConfig{
			URL:         github.Ptr("https://hooks.example.test/org"),
			ContentType: github.Ptr("json"),
		},
	})
	if err != nil {
		t.Fatalf("Organizations.CreateHook: %v", err)
	}
	if hook.GetType() != "Organization" {
		t.Errorf("hook type = %q, want Organization", hook.GetType())
	}

	got, _, err := client.Organizations.GetHook(ctx(), org, hook.GetID())
	if err != nil {
		t.Fatalf("Organizations.GetHook: %v", err)
	}
	if got.GetConfig().GetURL() != "https://hooks.example.test/org" {
		t.Errorf("hook config url = %q", got.GetConfig().GetURL())
	}

	edited, _, err := client.Organizations.EditHook(ctx(), org, hook.GetID(), &github.Hook{
		Events: []string{"push"},
	})
	if err != nil {
		t.Fatalf("Organizations.EditHook: %v", err)
	}
	if len(edited.Events) != 1 || edited.Events[0] != "push" {
		t.Errorf("edited events = %v, want [push]", edited.Events)
	}

	hooks, _, err := client.Organizations.ListHooks(ctx(), org, nil)
	if err != nil {
		t.Fatalf("Organizations.ListHooks: %v", err)
	}
	if len(hooks) != 1 {
		t.Errorf("hooks = %d, want 1", len(hooks))
	}

	if _, err := client.Organizations.PingHook(ctx(), org, hook.GetID()); err != nil {
		t.Fatalf("Organizations.PingHook: %v", err)
	}
	if _, err := client.Organizations.DeleteHook(ctx(), org, hook.GetID()); err != nil {
		t.Fatalf("Organizations.DeleteHook: %v", err)
	}
	if _, resp, err := client.Organizations.GetHook(ctx(), org, hook.GetID()); err == nil || resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GetHook after delete: err=%v status=%v, want 404", err, resp)
	}
}
