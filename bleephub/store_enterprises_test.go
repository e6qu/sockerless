package bleephub

import (
	"testing"
)

// TestEnterpriseStatePersistenceReload verifies every enterprise store
// surface — teams (with members and organization assignments), code security
// configurations (with attachments and next-ID counter), and the singleton
// enterprise settings — survives a persistence close/reopen cycle.
func TestEnterpriseStatePersistenceReload(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BLEEPHUB_PERSIST", "true")
	t.Setenv("BLEEPHUB_DATA_DIR", dir)

	// --- session 1: create enterprise state, then close ---
	p1, err := NewPersistence()
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	st1 := NewStore()
	if err := st1.SetPersistence(p1); err != nil {
		t.Fatalf("SetPersistence: %v", err)
	}
	st1.SeedDefaultUser()
	admin := st1.UsersByLogin["admin"]

	groupID := "62ab9291-fae2-468e-974b-7e45096d5021"
	team := st1.CreateEnterpriseTeam("Reload Crew", "survives restarts", "selected", &groupID, "notifications_disabled")
	if team == nil {
		t.Fatal("CreateEnterpriseTeam returned nil")
	}
	st1.AddEnterpriseTeamMember(team, admin.ID)
	st1.AddEnterpriseTeamOrg(team, "reload-org")

	cfg := st1.CreateEnterpriseCodeSecurityConfig(&EnterpriseCodeSecurityConfiguration{
		Name:            "reload-config",
		Description:     "reload coverage",
		SecretScanning:  "enabled",
		DependencyGraph: "enabled",
		Enforcement:     "enforced",
	})
	st1.SetEnterpriseCodeSecurityConfigDefault(cfg, "public")
	repo := st1.CreateRepo(admin, "ent-reload-repo", "", false)
	if repo == nil {
		t.Fatal("CreateRepo returned nil")
	}
	// Attach directly (the repo is user-owned; the association mechanism is
	// what reload must preserve).
	st1.mu.Lock()
	st1.EnterpriseCodeSecurityRepoConfigs[repo.ID] = cfg.ID
	st1.persist.MustPut("enterprise_code_security_attachments", "1",
		&EnterpriseCodeSecurityAttachment{RepoID: repo.ID, ConfigID: cfg.ID})
	st1.mu.Unlock()

	st1.SetEnterpriseDependabotRepoAccess([]int{repo.ID})
	st1.SetEnterpriseDependabotDefaultLevel("internal")
	st1.SetEnterpriseActionsCacheRetentionDays(21)
	st1.SetEnterpriseActionsCacheSizeGB(42)
	if !st1.AddEnterpriseOIDCCustomProperty("cost_center") {
		t.Fatal("AddEnterpriseOIDCCustomProperty returned false for a fresh property")
	}
	st1.SetEnterpriseCopilotCodingAgentPolicy("enabled_for_selected_orgs")
	st1.AddEnterpriseCopilotCodingAgentOrgs([]string{"reload-org"})

	if err := p1.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// --- session 2: reload and assert every surface came back ---
	p2, err := NewPersistence()
	if err != nil {
		t.Fatalf("re-open: %v", err)
	}
	st2 := NewStore()
	if err := st2.SetPersistence(p2); err != nil {
		t.Fatalf("re-load SetPersistence: %v", err)
	}
	defer p2.Close()

	got := st2.GetEnterpriseTeam("reload-crew")
	if got == nil {
		t.Fatal("enterprise team did not persist")
	}
	if got.Name != "Reload Crew" || got.OrganizationSelectionType != "selected" || got.NotificationSetting != "notifications_disabled" {
		t.Errorf("reloaded team = %+v", got)
	}
	if got.GroupID == nil || *got.GroupID != groupID {
		t.Errorf("reloaded team group_id = %v, want %s", got.GroupID, groupID)
	}
	if !st2.IsEnterpriseTeamMember(got, admin.ID) {
		t.Error("team membership did not persist")
	}
	if len(got.SelectedOrgLogins) != 1 || got.SelectedOrgLogins[0] != "reload-org" {
		t.Errorf("team org assignments = %v", got.SelectedOrgLogins)
	}
	// The team ID counter resumes past the loaded team.
	next := st2.CreateEnterpriseTeam("Another Crew", "", "", nil, "")
	if next == nil || next.ID <= got.ID {
		t.Errorf("post-reload team ID = %v, want > %d", next, got.ID)
	}

	cfg2 := st2.GetEnterpriseCodeSecurityConfig(cfg.ID)
	if cfg2 == nil {
		t.Fatal("code security configuration did not persist")
	}
	if cfg2.Name != "reload-config" || cfg2.SecretScanning != "enabled" || cfg2.DefaultForNewRepos != "public" {
		t.Errorf("reloaded configuration = %+v", cfg2)
	}
	if st2.EnterpriseCodeSecurityRepoConfigs[repo.ID] != cfg.ID {
		t.Error("code security attachment did not persist")
	}
	if st2.NextEnterpriseCodeSecurityConfigID <= cfg.ID {
		t.Errorf("configuration ID counter = %d, want > %d", st2.NextEnterpriseCodeSecurityConfigID, cfg.ID)
	}

	s := st2.EnterpriseSettings
	if len(s.DependabotAccessibleRepoIDs) != 1 || s.DependabotAccessibleRepoIDs[0] != repo.ID {
		t.Errorf("dependabot access = %v", s.DependabotAccessibleRepoIDs)
	}
	if s.DependabotDefaultLevel != "internal" {
		t.Errorf("dependabot default level = %q", s.DependabotDefaultLevel)
	}
	if s.ActionsCacheRetentionDays != 21 || s.ActionsCacheSizeGB != 42 {
		t.Errorf("actions cache limits = %d days / %d GB, want 21 / 42", s.ActionsCacheRetentionDays, s.ActionsCacheSizeGB)
	}
	if len(s.OIDCCustomProperties) != 1 || s.OIDCCustomProperties[0] != "cost_center" {
		t.Errorf("OIDC custom properties = %v", s.OIDCCustomProperties)
	}
	if s.CopilotCodingAgentPolicy != "enabled_for_selected_orgs" {
		t.Errorf("copilot policy = %q", s.CopilotCodingAgentPolicy)
	}
	if len(s.CopilotCodingAgentOrgs) != 1 || s.CopilotCodingAgentOrgs[0] != "reload-org" {
		t.Errorf("copilot orgs = %v", s.CopilotCodingAgentOrgs)
	}
}
