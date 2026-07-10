package bleephub

import (
	"testing"
	"time"
)

// TestPersistence_GovernanceSurfacesRoundTrip verifies every governance
// bucket (issue types, issue fields + values, custom properties + repo
// values, code security configurations + attachments, campaigns, private
// registries, network configurations + settings, immutable releases)
// reloads from persistence into a fresh store.
func TestPersistence_GovernanceSurfacesRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BLEEPHUB_PERSIST", "true")
	t.Setenv("BLEEPHUB_DATA_DIR", dir)

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
	org := st1.CreateOrg(admin, "gov-persist-org", "Gov Persist", "")
	repo := st1.CreateOrgRepo(org, admin, "gov-persist-repo", "", false)

	issueType := st1.CreateIssueType(org.Login, "Epic", nil, nil, true)
	field := st1.CreateIssueField(org.Login, "Priority", nil, "single_select", "all", []issueFieldOptionRequest{
		{Name: strPtr("High"), Color: strPtr("red")},
	})
	st1.SetIssueFieldValues(42, map[int]interface{}{field.ID: "High"})
	st1.UpsertCustomProperty(org.Login, &CustomProperty{
		PropertyName: "team", ValueType: "string", ValuesEditableBy: "org_actors",
	})
	st1.SetRepoCustomPropertyValues(repo.FullName, []customPropertyValuePayload{
		{PropertyName: "team", Value: "platform"},
	})
	cfg := st1.CreateCodeSecurityConfiguration(org.Login, &codeSecurityConfigurationRequest{
		Name: strPtr("persist-cfg"), Description: strPtr("d"),
	})
	if !st1.AttachCodeSecurityConfiguration(org.Login, cfg.ID, "selected", []int{repo.ID}) {
		t.Fatal("attach failed")
	}
	campaign := st1.CreateCampaign(org.Login, "persist campaign", "d", []string{admin.Login}, nil,
		time.Now().Add(24*time.Hour), nil, map[int][]int{})
	urlStr := "https://maven.example.com"
	st1.CreatePrivateRegistry(org.Login, &privateRegistryRequest{
		RegistryType: strPtr("maven_repository"), URL: &urlStr, Visibility: strPtr("all"),
	}, "token")
	settings, err := st1.CreateNetworkSettings(org.Login, "persist-subnet", "/subscriptions/x/subnets/s", "eastus")
	if err != nil {
		t.Fatalf("create network settings: %v", err)
	}
	netCfg, err := st1.CreateNetworkConfiguration(org.Login, &networkConfigurationRequest{
		Name: strPtr("persist-net"), NetworkSettingsIDs: []string{settings.ID},
	})
	if err != nil {
		t.Fatalf("create network configuration: %v", err)
	}
	st1.SetOrgImmutableReleasesSettings(org.Login, "selected", []int{repo.ID})
	st1.SetRepoImmutableReleases(repo.FullName, true)

	if err := p1.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	p2, err := NewPersistence()
	if err != nil {
		t.Fatalf("re-open: %v", err)
	}
	st2 := NewStore()
	if err := st2.SetPersistence(p2); err != nil {
		t.Fatalf("re-load SetPersistence: %v", err)
	}
	defer p2.Close()

	if got := st2.OrgIssueTypes[org.Login][issueType.ID]; got == nil || got.Name != "Epic" {
		t.Errorf("issue type did not persist: %v", got)
	}
	if st2.NextIssueTypeID <= issueType.ID {
		t.Errorf("NextIssueTypeID not recomputed: %d", st2.NextIssueTypeID)
	}
	gotField := st2.OrgIssueFields[org.Login][field.ID]
	if gotField == nil || gotField.DataType != "single_select" || len(gotField.Options) != 1 {
		t.Errorf("issue field did not persist: %v", gotField)
	}
	if got := st2.IssueFieldValues[42][field.ID]; got != "High" {
		t.Errorf("issue field value did not persist: %v", got)
	}
	if got := st2.OrgCustomProperties[org.Login]["team"]; got == nil || got.ValueType != "string" {
		t.Errorf("custom property did not persist: %v", got)
	}
	if got := st2.RepoCustomPropertyValues[repo.FullName]["team"]; got != "platform" {
		t.Errorf("repo custom property value did not persist: %v", got)
	}
	if got := st2.CodeSecurityConfigs[org.Login][cfg.ID]; got == nil || got.Name != "persist-cfg" {
		t.Errorf("code security configuration did not persist: %v", got)
	}
	if got := st2.CodeSecurityRepoAttachments[org.Login][repo.ID]; got != cfg.ID {
		t.Errorf("code security attachment did not persist: %v", got)
	}
	if got := st2.OrgCampaigns[org.Login][campaign.Number]; got == nil || got.Name != "persist campaign" {
		t.Errorf("campaign did not persist: %v", got)
	}
	if got := st2.OrgPrivateRegistries[org.Login]["MAVEN_REPOSITORY_SECRET"]; got == nil || got.URL != urlStr {
		t.Errorf("private registry did not persist: %v", got)
	}
	gotNet := st2.OrgNetworkConfigurations[org.Login][netCfg.ID]
	if gotNet == nil || gotNet.Name != "persist-net" {
		t.Errorf("network configuration did not persist: %v", gotNet)
	}
	gotSettings := st2.OrgNetworkSettings[org.Login][settings.ID]
	if gotSettings == nil || gotSettings.NetworkConfigurationID != netCfg.ID {
		t.Errorf("network settings did not persist: %v", gotSettings)
	}
	gotImm := st2.OrgImmutableReleases[org.Login]
	if gotImm == nil || gotImm.EnforcedRepositories != "selected" || len(gotImm.SelectedRepositoryIDs) != 1 {
		t.Errorf("org immutable releases settings did not persist: %v", gotImm)
	}
	if !st2.RepoImmutableReleases[repo.FullName] {
		t.Error("repo immutable releases toggle did not persist")
	}
}

func strPtr(s string) *string { return &s }
