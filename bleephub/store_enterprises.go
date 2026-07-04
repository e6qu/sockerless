package bleephub

import (
	"sort"
	"strconv"
	"time"
)

// Enterprise-scoped state. bleephub plays the role of a single GitHub
// Enterprise Server instance, so exactly one enterprise exists; its slug is
// configuration (BLEEPHUB_ENTERPRISE_SLUG), not store state. Everything the
// enterprise REST surfaces mutate — enterprise teams, code security
// configurations, Dependabot repository access, GitHub Actions cache limits,
// Actions OIDC custom property inclusions, and the Copilot coding agent
// policy — lives here and persists.

// EnterpriseTeam is a team scoped to the enterprise rather than to one
// organization. Membership is direct (user IDs); organization assignments
// depend on OrganizationSelectionType: "disabled" assigns none, "all"
// assigns every organization on the instance, "selected" assigns exactly
// SelectedOrgLogins.
type EnterpriseTeam struct {
	ID                        int       `json:"id"`
	Name                      string    `json:"name"`
	Description               string    `json:"description"`
	Slug                      string    `json:"slug"`
	OrganizationSelectionType string    `json:"organization_selection_type"`
	GroupID                   *string   `json:"group_id"`
	NotificationSetting       string    `json:"notification_setting"`
	MemberIDs                 []int     `json:"member_ids"`
	SelectedOrgLogins         []string  `json:"selected_org_logins"`
	CreatedAt                 time.Time `json:"created_at"`
	UpdatedAt                 time.Time `json:"updated_at"`
}

// EnterpriseCodeSecurityConfiguration mirrors GitHub's
// code-security-configuration schema with target_type "enterprise". Feature
// fields hold the enabled/disabled/not_set enum values the API accepts.
type EnterpriseCodeSecurityConfiguration struct {
	ID                                    int       `json:"id"`
	Name                                  string    `json:"name"`
	Description                           string    `json:"description"`
	AdvancedSecurity                      string    `json:"advanced_security"`
	DependencyGraph                       string    `json:"dependency_graph"`
	DependencyGraphAutosubmitAction       string    `json:"dependency_graph_autosubmit_action"`
	DependencyGraphAutosubmitLabeled      bool      `json:"dependency_graph_autosubmit_labeled_runners"`
	DependabotAlerts                      string    `json:"dependabot_alerts"`
	DependabotSecurityUpdates             string    `json:"dependabot_security_updates"`
	CodeScanningAllowAdvanced             *bool     `json:"code_scanning_allow_advanced"`
	CodeScanningDefaultSetup              string    `json:"code_scanning_default_setup"`
	CodeScanningRunnerType                *string   `json:"code_scanning_runner_type"`
	CodeScanningRunnerLabel               *string   `json:"code_scanning_runner_label"`
	CodeScanningDelegatedAlertDismissal   string    `json:"code_scanning_delegated_alert_dismissal"`
	SecretScanning                        string    `json:"secret_scanning"`
	SecretScanningPushProtection          string    `json:"secret_scanning_push_protection"`
	SecretScanningValidityChecks          string    `json:"secret_scanning_validity_checks"`
	SecretScanningNonProviderPatterns     string    `json:"secret_scanning_non_provider_patterns"`
	SecretScanningGenericSecrets          string    `json:"secret_scanning_generic_secrets"`
	SecretScanningDelegatedAlertDismissal string    `json:"secret_scanning_delegated_alert_dismissal"`
	SecretScanningExtendedMetadata        string    `json:"secret_scanning_extended_metadata"`
	PrivateVulnerabilityReporting         string    `json:"private_vulnerability_reporting"`
	Enforcement                           string    `json:"enforcement"`
	DefaultForNewRepos                    string    `json:"default_for_new_repos"` // "none" unless set via the defaults endpoint
	CreatedAt                             time.Time `json:"created_at"`
	UpdatedAt                             time.Time `json:"updated_at"`
}

// EnterpriseCodeSecurityAttachment records one repository's attachment to an
// enterprise code security configuration. Persisted individually so the
// repoID→configID association survives reload; a repository has at most one
// attached configuration.
type EnterpriseCodeSecurityAttachment struct {
	RepoID   int `json:"repo_id"`
	ConfigID int `json:"config_id"`
}

// EnterpriseSettings holds the singleton enterprise-level settings the REST
// surfaces mutate. Persisted as one row under the "enterprise_settings"
// bucket; zero-value fields fall back to defaultEnterpriseSettings values on
// first access paths that seed them in NewStore.
type EnterpriseSettings struct {
	// Dependabot repository access across organizations.
	DependabotAccessibleRepoIDs []int  `json:"dependabot_accessible_repo_ids"`
	DependabotDefaultLevel      string `json:"dependabot_default_level"` // "" = never set (null), else public|internal

	// GitHub Actions cache policy. GitHub Enterprise Server ships with a
	// 14-day retention limit and a 10 GB per-repository storage limit.
	ActionsCacheRetentionDays int `json:"actions_cache_retention_days"`
	ActionsCacheSizeGB        int `json:"actions_cache_size_gb"`

	// GitHub Actions OIDC custom property inclusions (repository custom
	// properties included in OIDC token claims), in insertion order.
	OIDCCustomProperties []string `json:"oidc_custom_properties"`

	// Copilot coding agent policy. "" = never set.
	CopilotCodingAgentPolicy string   `json:"copilot_coding_agent_policy"`
	CopilotCodingAgentOrgs   []string `json:"copilot_coding_agent_orgs"`
}

func defaultEnterpriseSettings() *EnterpriseSettings {
	return &EnterpriseSettings{
		ActionsCacheRetentionDays: 14,
		ActionsCacheSizeGB:        10,
	}
}

func (st *Store) persistEnterpriseSettings() {
	if st.persist != nil {
		st.persist.MustPut("enterprise_settings", "enterprise", st.EnterpriseSettings)
	}
}

func (st *Store) persistEnterpriseTeam(t *EnterpriseTeam) {
	if st.persist != nil {
		st.persist.MustPut("enterprise_teams", strconv.Itoa(t.ID), t)
	}
}

func (st *Store) persistEnterpriseCodeSecurityConfig(c *EnterpriseCodeSecurityConfiguration) {
	if st.persist != nil {
		st.persist.MustPut("enterprise_code_security_configs", strconv.Itoa(c.ID), c)
	}
}

// --- enterprise teams ---

// CreateEnterpriseTeam creates an enterprise team. Returns nil when a team
// with the same slug already exists.
func (st *Store) CreateEnterpriseTeam(name, description, selectionType string, groupID *string, notificationSetting string) *EnterpriseTeam {
	st.mu.Lock()
	defer st.mu.Unlock()

	slug := slugify(name)
	if _, exists := st.EnterpriseTeamsBySlug[slug]; exists {
		return nil
	}
	if selectionType == "" {
		selectionType = "disabled"
	}
	if notificationSetting == "" {
		notificationSetting = "notifications_enabled"
	}
	now := time.Now().UTC()
	t := &EnterpriseTeam{
		ID:                        st.NextEnterpriseTeamID,
		Name:                      name,
		Description:               description,
		Slug:                      slug,
		OrganizationSelectionType: selectionType,
		GroupID:                   groupID,
		NotificationSetting:       notificationSetting,
		MemberIDs:                 []int{},
		SelectedOrgLogins:         []string{},
		CreatedAt:                 now,
		UpdatedAt:                 now,
	}
	st.NextEnterpriseTeamID++
	st.EnterpriseTeams[t.ID] = t
	st.EnterpriseTeamsBySlug[t.Slug] = t
	st.persistEnterpriseTeam(t)
	return t
}

// GetEnterpriseTeam returns an enterprise team by slug, or nil.
func (st *Store) GetEnterpriseTeam(slug string) *EnterpriseTeam {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.EnterpriseTeamsBySlug[slug]
}

// ListEnterpriseTeams returns all enterprise teams sorted by ID.
func (st *Store) ListEnterpriseTeams() []*EnterpriseTeam {
	st.mu.RLock()
	defer st.mu.RUnlock()
	out := make([]*EnterpriseTeam, 0, len(st.EnterpriseTeams))
	for _, t := range st.EnterpriseTeams {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// UpdateEnterpriseTeam applies the non-nil fields. Renaming re-slugs the team
// exactly as GitHub does. Returns false when the new slug collides with a
// different existing team.
func (st *Store) UpdateEnterpriseTeam(t *EnterpriseTeam, name, description, selectionType, notificationSetting *string, groupID **string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	if name != nil && *name != "" && *name != t.Name {
		newSlug := slugify(*name)
		if other, exists := st.EnterpriseTeamsBySlug[newSlug]; exists && other != t {
			return false
		}
		delete(st.EnterpriseTeamsBySlug, t.Slug)
		t.Name = *name
		t.Slug = newSlug
		st.EnterpriseTeamsBySlug[t.Slug] = t
	}
	if description != nil {
		t.Description = *description
	}
	if selectionType != nil {
		t.OrganizationSelectionType = *selectionType
	}
	if notificationSetting != nil {
		t.NotificationSetting = *notificationSetting
	}
	if groupID != nil {
		t.GroupID = *groupID
	}
	t.UpdatedAt = time.Now().UTC()
	st.persistEnterpriseTeam(t)
	return true
}

// DeleteEnterpriseTeam removes an enterprise team by slug. Returns true if it
// existed.
func (st *Store) DeleteEnterpriseTeam(slug string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	t, ok := st.EnterpriseTeamsBySlug[slug]
	if !ok {
		return false
	}
	delete(st.EnterpriseTeamsBySlug, slug)
	delete(st.EnterpriseTeams, t.ID)
	if st.persist != nil {
		st.persist.MustDelete("enterprise_teams", strconv.Itoa(t.ID))
	}
	return true
}

// AddEnterpriseTeamMember adds a user to the team (idempotent).
func (st *Store) AddEnterpriseTeamMember(t *EnterpriseTeam, userID int) {
	st.mu.Lock()
	defer st.mu.Unlock()

	for _, id := range t.MemberIDs {
		if id == userID {
			return
		}
	}
	t.MemberIDs = append(t.MemberIDs, userID)
	t.UpdatedAt = time.Now().UTC()
	st.persistEnterpriseTeam(t)
}

// RemoveEnterpriseTeamMember removes a user from the team. Returns true if
// the user was a member.
func (st *Store) RemoveEnterpriseTeamMember(t *EnterpriseTeam, userID int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	for i, id := range t.MemberIDs {
		if id == userID {
			t.MemberIDs = append(t.MemberIDs[:i], t.MemberIDs[i+1:]...)
			t.UpdatedAt = time.Now().UTC()
			st.persistEnterpriseTeam(t)
			return true
		}
	}
	return false
}

// IsEnterpriseTeamMember reports whether the user belongs to the team.
func (st *Store) IsEnterpriseTeamMember(t *EnterpriseTeam, userID int) bool {
	st.mu.RLock()
	defer st.mu.RUnlock()
	for _, id := range t.MemberIDs {
		if id == userID {
			return true
		}
	}
	return false
}

// ListEnterpriseTeamMembers returns the team's members sorted by user ID.
func (st *Store) ListEnterpriseTeamMembers(t *EnterpriseTeam) []*User {
	st.mu.RLock()
	defer st.mu.RUnlock()
	out := make([]*User, 0, len(t.MemberIDs))
	for _, id := range t.MemberIDs {
		if u := st.Users[id]; u != nil {
			out = append(out, u)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// AddEnterpriseTeamOrg records an organization assignment (idempotent).
func (st *Store) AddEnterpriseTeamOrg(t *EnterpriseTeam, orgLogin string) {
	st.mu.Lock()
	defer st.mu.Unlock()

	for _, l := range t.SelectedOrgLogins {
		if l == orgLogin {
			return
		}
	}
	t.SelectedOrgLogins = append(t.SelectedOrgLogins, orgLogin)
	sort.Strings(t.SelectedOrgLogins)
	t.UpdatedAt = time.Now().UTC()
	st.persistEnterpriseTeam(t)
}

// RemoveEnterpriseTeamOrg removes an organization assignment. Returns true if
// it was assigned.
func (st *Store) RemoveEnterpriseTeamOrg(t *EnterpriseTeam, orgLogin string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	for i, l := range t.SelectedOrgLogins {
		if l == orgLogin {
			t.SelectedOrgLogins = append(t.SelectedOrgLogins[:i], t.SelectedOrgLogins[i+1:]...)
			t.UpdatedAt = time.Now().UTC()
			st.persistEnterpriseTeam(t)
			return true
		}
	}
	return false
}

// ListEnterpriseTeamOrgs resolves the team's organization assignments from
// its selection type: "all" assigns every organization on the instance,
// "selected" the recorded list, "disabled" none. Sorted by org ID.
func (st *Store) ListEnterpriseTeamOrgs(t *EnterpriseTeam) []*Org {
	st.mu.RLock()
	defer st.mu.RUnlock()

	var out []*Org
	switch t.OrganizationSelectionType {
	case "all":
		for _, o := range st.Orgs {
			out = append(out, o)
		}
	case "selected":
		for _, l := range t.SelectedOrgLogins {
			if o := st.OrgsByLogin[l]; o != nil {
				out = append(out, o)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// --- enterprise code security configurations ---

// CreateEnterpriseCodeSecurityConfig stores a new configuration.
func (st *Store) CreateEnterpriseCodeSecurityConfig(c *EnterpriseCodeSecurityConfiguration) *EnterpriseCodeSecurityConfiguration {
	st.mu.Lock()
	defer st.mu.Unlock()

	now := time.Now().UTC()
	c.ID = st.NextEnterpriseCodeSecurityConfigID
	st.NextEnterpriseCodeSecurityConfigID++
	c.CreatedAt = now
	c.UpdatedAt = now
	if c.DefaultForNewRepos == "" {
		c.DefaultForNewRepos = "none"
	}
	st.EnterpriseCodeSecurityConfigs[c.ID] = c
	st.persistEnterpriseCodeSecurityConfig(c)
	return c
}

// GetEnterpriseCodeSecurityConfig returns a configuration by ID, or nil.
func (st *Store) GetEnterpriseCodeSecurityConfig(id int) *EnterpriseCodeSecurityConfiguration {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.EnterpriseCodeSecurityConfigs[id]
}

// ListEnterpriseCodeSecurityConfigs returns all configurations sorted by ID.
func (st *Store) ListEnterpriseCodeSecurityConfigs() []*EnterpriseCodeSecurityConfiguration {
	st.mu.RLock()
	defer st.mu.RUnlock()
	out := make([]*EnterpriseCodeSecurityConfiguration, 0, len(st.EnterpriseCodeSecurityConfigs))
	for _, c := range st.EnterpriseCodeSecurityConfigs {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// TouchEnterpriseCodeSecurityConfig bumps updated_at and persists after a
// caller-applied field mutation. Callers mutate under this lock via mutate.
func (st *Store) TouchEnterpriseCodeSecurityConfig(c *EnterpriseCodeSecurityConfiguration, mutate func()) {
	st.mu.Lock()
	defer st.mu.Unlock()
	mutate()
	c.UpdatedAt = time.Now().UTC()
	st.persistEnterpriseCodeSecurityConfig(c)
}

// DeleteEnterpriseCodeSecurityConfig removes a configuration and detaches its
// repositories. Returns false when the configuration is a default for new
// repositories (GitHub refuses with 409 in that state).
func (st *Store) DeleteEnterpriseCodeSecurityConfig(id int) (deleted, conflict bool) {
	st.mu.Lock()
	defer st.mu.Unlock()

	c, ok := st.EnterpriseCodeSecurityConfigs[id]
	if !ok {
		return false, false
	}
	if c.DefaultForNewRepos != "none" {
		return false, true
	}
	delete(st.EnterpriseCodeSecurityConfigs, id)
	for repoID, cfgID := range st.EnterpriseCodeSecurityRepoConfigs {
		if cfgID == id {
			delete(st.EnterpriseCodeSecurityRepoConfigs, repoID)
			if st.persist != nil {
				st.persist.MustDelete("enterprise_code_security_attachments", strconv.Itoa(repoID))
			}
		}
	}
	if st.persist != nil {
		st.persist.MustDelete("enterprise_code_security_configs", strconv.Itoa(id))
	}
	return true, false
}

// AttachEnterpriseCodeSecurityConfig attaches the configuration to every
// organization-owned repository on the instance ("all"), or only to those
// without an attached configuration ("all_without_configurations").
func (st *Store) AttachEnterpriseCodeSecurityConfig(c *EnterpriseCodeSecurityConfiguration, scope string) {
	st.mu.Lock()
	defer st.mu.Unlock()

	for _, repo := range st.Repos {
		if repo.OwnerType != "Organization" {
			continue
		}
		if scope == "all_without_configurations" {
			if _, attached := st.EnterpriseCodeSecurityRepoConfigs[repo.ID]; attached {
				continue
			}
		}
		st.EnterpriseCodeSecurityRepoConfigs[repo.ID] = c.ID
		if st.persist != nil {
			st.persist.MustPut("enterprise_code_security_attachments", strconv.Itoa(repo.ID),
				&EnterpriseCodeSecurityAttachment{RepoID: repo.ID, ConfigID: c.ID})
		}
	}
}

// ListEnterpriseCodeSecurityConfigRepos returns the repositories attached to
// the configuration, sorted by repo ID.
func (st *Store) ListEnterpriseCodeSecurityConfigRepos(configID int) []*Repo {
	st.mu.RLock()
	defer st.mu.RUnlock()

	var out []*Repo
	for repoID, cfgID := range st.EnterpriseCodeSecurityRepoConfigs {
		if cfgID != configID {
			continue
		}
		if repo := st.Repos[repoID]; repo != nil {
			out = append(out, repo)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// SetEnterpriseCodeSecurityConfigDefault marks the configuration as the
// default for new repositories of the given visibility ("none" clears it).
func (st *Store) SetEnterpriseCodeSecurityConfigDefault(c *EnterpriseCodeSecurityConfiguration, defaultForNewRepos string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	c.DefaultForNewRepos = defaultForNewRepos
	c.UpdatedAt = time.Now().UTC()
	st.persistEnterpriseCodeSecurityConfig(c)
}

// --- enterprise settings mutators ---

// SetEnterpriseDependabotRepoAccess replaces the Dependabot accessible
// repository ID list.
func (st *Store) SetEnterpriseDependabotRepoAccess(ids []int) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.EnterpriseSettings.DependabotAccessibleRepoIDs = append([]int(nil), ids...)
	st.persistEnterpriseSettings()
}

// SetEnterpriseDependabotDefaultLevel sets the Dependabot default repository
// access level (public|internal).
func (st *Store) SetEnterpriseDependabotDefaultLevel(level string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.EnterpriseSettings.DependabotDefaultLevel = level
	st.persistEnterpriseSettings()
}

// SetEnterpriseActionsCacheRetentionDays sets the Actions cache retention
// limit in days.
func (st *Store) SetEnterpriseActionsCacheRetentionDays(days int) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.EnterpriseSettings.ActionsCacheRetentionDays = days
	st.persistEnterpriseSettings()
}

// SetEnterpriseActionsCacheSizeGB sets the Actions cache storage limit in GB.
func (st *Store) SetEnterpriseActionsCacheSizeGB(gb int) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.EnterpriseSettings.ActionsCacheSizeGB = gb
	st.persistEnterpriseSettings()
}

// AddEnterpriseOIDCCustomProperty records an OIDC custom property inclusion.
// Returns false when the property is already included.
func (st *Store) AddEnterpriseOIDCCustomProperty(name string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	for _, p := range st.EnterpriseSettings.OIDCCustomProperties {
		if p == name {
			return false
		}
	}
	st.EnterpriseSettings.OIDCCustomProperties = append(st.EnterpriseSettings.OIDCCustomProperties, name)
	st.persistEnterpriseSettings()
	return true
}

// RemoveEnterpriseOIDCCustomProperty removes an OIDC custom property
// inclusion. Returns true if it existed.
func (st *Store) RemoveEnterpriseOIDCCustomProperty(name string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	for i, p := range st.EnterpriseSettings.OIDCCustomProperties {
		if p == name {
			st.EnterpriseSettings.OIDCCustomProperties = append(
				st.EnterpriseSettings.OIDCCustomProperties[:i],
				st.EnterpriseSettings.OIDCCustomProperties[i+1:]...)
			st.persistEnterpriseSettings()
			return true
		}
	}
	return false
}

// SetEnterpriseCopilotCodingAgentPolicy sets the Copilot coding agent policy
// state.
func (st *Store) SetEnterpriseCopilotCodingAgentPolicy(policyState string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.EnterpriseSettings.CopilotCodingAgentPolicy = policyState
	st.persistEnterpriseSettings()
}

// AddEnterpriseCopilotCodingAgentOrgs enables the Copilot coding agent for
// the given organization logins (idempotent, sorted).
func (st *Store) AddEnterpriseCopilotCodingAgentOrgs(logins []string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	set := map[string]bool{}
	for _, l := range st.EnterpriseSettings.CopilotCodingAgentOrgs {
		set[l] = true
	}
	for _, l := range logins {
		set[l] = true
	}
	out := make([]string, 0, len(set))
	for l := range set {
		out = append(out, l)
	}
	sort.Strings(out)
	st.EnterpriseSettings.CopilotCodingAgentOrgs = out
	st.persistEnterpriseSettings()
}

// RemoveEnterpriseCopilotCodingAgentOrgs disables the Copilot coding agent
// for the given organization logins.
func (st *Store) RemoveEnterpriseCopilotCodingAgentOrgs(logins []string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	drop := map[string]bool{}
	for _, l := range logins {
		drop[l] = true
	}
	kept := st.EnterpriseSettings.CopilotCodingAgentOrgs[:0]
	for _, l := range st.EnterpriseSettings.CopilotCodingAgentOrgs {
		if !drop[l] {
			kept = append(kept, l)
		}
	}
	st.EnterpriseSettings.CopilotCodingAgentOrgs = kept
	st.persistEnterpriseSettings()
}
