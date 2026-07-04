package bleephub

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// GitHub code security configurations: named bundles of security feature
// enablement an organization defines and attaches to repositories.

// CodeSecurityConfiguration is an organization code security configuration.
type CodeSecurityConfiguration struct {
	ID                                  int       `json:"id"`
	OrgLogin                            string    `json:"org_login"`
	Name                                string    `json:"name"`
	Description                         string    `json:"description"`
	TargetType                          string    `json:"target_type"`
	AdvancedSecurity                    string    `json:"advanced_security"`
	DependencyGraph                     string    `json:"dependency_graph"`
	DependencyGraphAutosubmitAction     string    `json:"dependency_graph_autosubmit_action"`
	DependencyGraphAutosubmitLabeled    *bool     `json:"dependency_graph_autosubmit_labeled"`
	DependabotAlerts                    string    `json:"dependabot_alerts"`
	DependabotSecurityUpdates           string    `json:"dependabot_security_updates"`
	DependabotDelegatedAlertDismissal   string    `json:"dependabot_delegated_alert_dismissal"`
	CodeScanningDefaultSetup            string    `json:"code_scanning_default_setup"`
	CodeScanningRunnerType              *string   `json:"code_scanning_runner_type"`
	CodeScanningRunnerLabel             *string   `json:"code_scanning_runner_label"`
	CodeScanningDelegatedAlertDismissal string    `json:"code_scanning_delegated_alert_dismissal"`
	SecretScanning                      string    `json:"secret_scanning"`
	SecretScanningPushProtection        string    `json:"secret_scanning_push_protection"`
	SecretScanningDelegatedBypass       string    `json:"secret_scanning_delegated_bypass"`
	SecretScanningValidityChecks        string    `json:"secret_scanning_validity_checks"`
	SecretScanningNonProviderPatterns   string    `json:"secret_scanning_non_provider_patterns"`
	SecretScanningGenericSecrets        string    `json:"secret_scanning_generic_secrets"`
	SecretScanningDelegatedDismissal    string    `json:"secret_scanning_delegated_alert_dismissal"`
	SecretScanningExtendedMetadata      string    `json:"secret_scanning_extended_metadata"`
	CodeScanningAllowAdvanced           *bool     `json:"code_scanning_allow_advanced"`
	PrivateVulnerabilityReporting       string    `json:"private_vulnerability_reporting"`
	Enforcement                         string    `json:"enforcement"`
	DefaultForNewRepos                  string    `json:"default_for_new_repos"`
	CreatedAt                           time.Time `json:"created_at"`
	UpdatedAt                           time.Time `json:"updated_at"`
}

func (s *Server) registerGHCodeSecurityConfigurationRoutes() {
	s.route("GET /api/v3/orgs/{org}/code-security/configurations",
		s.requirePerm(scopeOrgAdministration, permRead, s.orgGated(s.handleListCodeSecurityConfigurations)))
	s.route("POST /api/v3/orgs/{org}/code-security/configurations",
		s.requirePerm(scopeOrgAdministration, permWrite, s.orgGated(s.handleCreateCodeSecurityConfiguration)))
	s.route("GET /api/v3/orgs/{org}/code-security/configurations/defaults",
		s.requirePerm(scopeOrgAdministration, permRead, s.orgGated(s.handleGetDefaultCodeSecurityConfigurations)))
	s.route("DELETE /api/v3/orgs/{org}/code-security/configurations/detach",
		s.requirePerm(scopeOrgAdministration, permWrite, s.orgGated(s.handleDetachCodeSecurityConfiguration)))
	s.route("GET /api/v3/orgs/{org}/code-security/configurations/{configuration_id}",
		s.requirePerm(scopeOrgAdministration, permRead, s.orgGated(s.handleGetCodeSecurityConfiguration)))
	s.route("PATCH /api/v3/orgs/{org}/code-security/configurations/{configuration_id}",
		s.requirePerm(scopeOrgAdministration, permWrite, s.orgGated(s.handleUpdateCodeSecurityConfiguration)))
	s.route("DELETE /api/v3/orgs/{org}/code-security/configurations/{configuration_id}",
		s.requirePerm(scopeOrgAdministration, permWrite, s.orgGated(s.handleDeleteCodeSecurityConfiguration)))
	s.route("POST /api/v3/orgs/{org}/code-security/configurations/{configuration_id}/attach",
		s.requirePerm(scopeOrgAdministration, permWrite, s.orgGated(s.handleAttachCodeSecurityConfiguration)))
	s.route("PUT /api/v3/orgs/{org}/code-security/configurations/{configuration_id}/defaults",
		s.requirePerm(scopeOrgAdministration, permWrite, s.orgGated(s.handleSetCodeSecurityConfigurationAsDefault)))
	s.route("GET /api/v3/orgs/{org}/code-security/configurations/{configuration_id}/repositories",
		s.requirePerm(scopeOrgAdministration, permRead, s.orgGated(s.handleListCodeSecurityConfigurationRepositories)))
	s.route("GET /api/v3/repos/{owner}/{repo}/code-security-configuration", s.handleGetRepoCodeSecurityConfiguration)
}

var codeSecurityEnablement = map[string]bool{"enabled": true, "disabled": true, "not_set": true}

// codeSecurityConfigurationRequest is the create/update wire shape. The
// code_security / secret_protection members are write-only granular toggles
// real GitHub folds into advanced_security.
type codeSecurityConfigurationRequest struct {
	Name                             *string `json:"name"`
	Description                      *string `json:"description"`
	AdvancedSecurity                 *string `json:"advanced_security"`
	CodeSecurity                     *string `json:"code_security"`
	SecretProtection                 *string `json:"secret_protection"`
	DependencyGraph                  *string `json:"dependency_graph"`
	DependencyGraphAutosubmitAction  *string `json:"dependency_graph_autosubmit_action"`
	DependencyGraphAutosubmitOptions *struct {
		LabeledRunners *bool `json:"labeled_runners"`
	} `json:"dependency_graph_autosubmit_action_options"`
	DependabotAlerts                  *string `json:"dependabot_alerts"`
	DependabotSecurityUpdates         *string `json:"dependabot_security_updates"`
	DependabotDelegatedAlertDismissal *string `json:"dependabot_delegated_alert_dismissal"`
	CodeScanningDefaultSetup          *string `json:"code_scanning_default_setup"`
	CodeScanningDefaultSetupOptions   *struct {
		RunnerType  *string `json:"runner_type"`
		RunnerLabel *string `json:"runner_label"`
	} `json:"code_scanning_default_setup_options"`
	CodeScanningDelegatedAlertDismissal *string `json:"code_scanning_delegated_alert_dismissal"`
	SecretScanning                      *string `json:"secret_scanning"`
	SecretScanningPushProtection        *string `json:"secret_scanning_push_protection"`
	SecretScanningDelegatedBypass       *string `json:"secret_scanning_delegated_bypass"`
	SecretScanningValidityChecks        *string `json:"secret_scanning_validity_checks"`
	SecretScanningNonProviderPatterns   *string `json:"secret_scanning_non_provider_patterns"`
	SecretScanningGenericSecrets        *string `json:"secret_scanning_generic_secrets"`
	SecretScanningDelegatedDismissal    *string `json:"secret_scanning_delegated_alert_dismissal"`
	SecretScanningExtendedMetadata      *string `json:"secret_scanning_extended_metadata"`
	CodeScanningOptions                 *struct {
		AllowAdvanced *bool `json:"allow_advanced"`
	} `json:"code_scanning_options"`
	PrivateVulnerabilityReporting *string `json:"private_vulnerability_reporting"`
	Enforcement                   *string `json:"enforcement"`
}

// validateEnums checks every provided enum member; returns false after
// writing the validation error.
func (req *codeSecurityConfigurationRequest) validateEnums(w http.ResponseWriter) bool {
	enums := map[string]*string{
		"code_security":                             req.CodeSecurity,
		"secret_protection":                         req.SecretProtection,
		"dependency_graph":                          req.DependencyGraph,
		"dependency_graph_autosubmit_action":        req.DependencyGraphAutosubmitAction,
		"dependabot_alerts":                         req.DependabotAlerts,
		"dependabot_security_updates":               req.DependabotSecurityUpdates,
		"dependabot_delegated_alert_dismissal":      req.DependabotDelegatedAlertDismissal,
		"code_scanning_default_setup":               req.CodeScanningDefaultSetup,
		"code_scanning_delegated_alert_dismissal":   req.CodeScanningDelegatedAlertDismissal,
		"secret_scanning":                           req.SecretScanning,
		"secret_scanning_push_protection":           req.SecretScanningPushProtection,
		"secret_scanning_delegated_bypass":          req.SecretScanningDelegatedBypass,
		"secret_scanning_validity_checks":           req.SecretScanningValidityChecks,
		"secret_scanning_non_provider_patterns":     req.SecretScanningNonProviderPatterns,
		"secret_scanning_generic_secrets":           req.SecretScanningGenericSecrets,
		"secret_scanning_delegated_alert_dismissal": req.SecretScanningDelegatedDismissal,
		"secret_scanning_extended_metadata":         req.SecretScanningExtendedMetadata,
		"private_vulnerability_reporting":           req.PrivateVulnerabilityReporting,
	}
	for field, v := range enums {
		if v != nil && !codeSecurityEnablement[*v] {
			writeGHValidationError(w, "CodeSecurityConfiguration", field, "invalid")
			return false
		}
	}
	if req.AdvancedSecurity != nil {
		switch *req.AdvancedSecurity {
		case "enabled", "disabled", "code_security", "secret_protection":
		default:
			writeGHValidationError(w, "CodeSecurityConfiguration", "advanced_security", "invalid")
			return false
		}
	}
	if req.Enforcement != nil && *req.Enforcement != "enforced" && *req.Enforcement != "unenforced" {
		writeGHValidationError(w, "CodeSecurityConfiguration", "enforcement", "invalid")
		return false
	}
	return true
}

// apply copies every provided member onto the configuration and reports
// whether anything changed.
func (req *codeSecurityConfigurationRequest) apply(c *CodeSecurityConfiguration) bool {
	changed := false
	setStr := func(dst *string, v *string) {
		if v != nil && *dst != *v {
			*dst = *v
			changed = true
		}
	}
	setStr(&c.Name, req.Name)
	setStr(&c.Description, req.Description)
	setStr(&c.AdvancedSecurity, req.AdvancedSecurity)
	// The granular code_security / secret_protection toggles fold into
	// advanced_security when it is not itself provided.
	if req.AdvancedSecurity == nil && (req.CodeSecurity != nil || req.SecretProtection != nil) {
		cs := req.CodeSecurity != nil && *req.CodeSecurity == "enabled"
		sp := req.SecretProtection != nil && *req.SecretProtection == "enabled"
		folded := "disabled"
		switch {
		case cs && sp:
			folded = "enabled"
		case cs:
			folded = "code_security"
		case sp:
			folded = "secret_protection"
		}
		if c.AdvancedSecurity != folded {
			c.AdvancedSecurity = folded
			changed = true
		}
	}
	setStr(&c.DependencyGraph, req.DependencyGraph)
	setStr(&c.DependencyGraphAutosubmitAction, req.DependencyGraphAutosubmitAction)
	if req.DependencyGraphAutosubmitOptions != nil && req.DependencyGraphAutosubmitOptions.LabeledRunners != nil {
		v := *req.DependencyGraphAutosubmitOptions.LabeledRunners
		if c.DependencyGraphAutosubmitLabeled == nil || *c.DependencyGraphAutosubmitLabeled != v {
			c.DependencyGraphAutosubmitLabeled = &v
			changed = true
		}
	}
	setStr(&c.DependabotAlerts, req.DependabotAlerts)
	setStr(&c.DependabotSecurityUpdates, req.DependabotSecurityUpdates)
	setStr(&c.DependabotDelegatedAlertDismissal, req.DependabotDelegatedAlertDismissal)
	setStr(&c.CodeScanningDefaultSetup, req.CodeScanningDefaultSetup)
	if req.CodeScanningDefaultSetupOptions != nil {
		if v := req.CodeScanningDefaultSetupOptions.RunnerType; v != nil {
			if c.CodeScanningRunnerType == nil || *c.CodeScanningRunnerType != *v {
				c.CodeScanningRunnerType = v
				changed = true
			}
		}
		if v := req.CodeScanningDefaultSetupOptions.RunnerLabel; v != nil {
			if c.CodeScanningRunnerLabel == nil || *c.CodeScanningRunnerLabel != *v {
				c.CodeScanningRunnerLabel = v
				changed = true
			}
		}
	}
	setStr(&c.CodeScanningDelegatedAlertDismissal, req.CodeScanningDelegatedAlertDismissal)
	setStr(&c.SecretScanning, req.SecretScanning)
	setStr(&c.SecretScanningPushProtection, req.SecretScanningPushProtection)
	setStr(&c.SecretScanningDelegatedBypass, req.SecretScanningDelegatedBypass)
	setStr(&c.SecretScanningValidityChecks, req.SecretScanningValidityChecks)
	setStr(&c.SecretScanningNonProviderPatterns, req.SecretScanningNonProviderPatterns)
	setStr(&c.SecretScanningGenericSecrets, req.SecretScanningGenericSecrets)
	setStr(&c.SecretScanningDelegatedDismissal, req.SecretScanningDelegatedDismissal)
	setStr(&c.SecretScanningExtendedMetadata, req.SecretScanningExtendedMetadata)
	if req.CodeScanningOptions != nil && req.CodeScanningOptions.AllowAdvanced != nil {
		v := *req.CodeScanningOptions.AllowAdvanced
		if c.CodeScanningAllowAdvanced == nil || *c.CodeScanningAllowAdvanced != v {
			c.CodeScanningAllowAdvanced = &v
			changed = true
		}
	}
	setStr(&c.PrivateVulnerabilityReporting, req.PrivateVulnerabilityReporting)
	setStr(&c.Enforcement, req.Enforcement)
	return changed
}

func (s *Server) handleListCodeSecurityConfigurations(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	configs := s.store.ListCodeSecurityConfigurations(org)
	if r.URL.Query().Get("target_type") == "global" {
		// bleephub defines no global (GitHub-managed) configurations.
		configs = nil
	}
	perPage := 30
	if v := r.URL.Query().Get("per_page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			perPage = n
		}
	}
	if len(configs) > perPage {
		configs = configs[:perPage]
	}
	base := s.baseURL(r)
	out := make([]map[string]interface{}, 0, len(configs))
	for _, c := range configs {
		out = append(out, codeSecurityConfigurationJSON(c, base))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCreateCodeSecurityConfiguration(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	var req codeSecurityConfigurationRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Name == nil || *req.Name == "" {
		writeGHValidationError(w, "CodeSecurityConfiguration", "name", "missing_field")
		return
	}
	if req.Description == nil || *req.Description == "" {
		writeGHValidationError(w, "CodeSecurityConfiguration", "description", "missing_field")
		return
	}
	if !req.validateEnums(w) {
		return
	}
	if s.store.GetCodeSecurityConfigurationByName(org, *req.Name) != nil {
		writeGHValidationError(w, "CodeSecurityConfiguration", "name", "already_exists")
		return
	}
	c := s.store.CreateCodeSecurityConfiguration(org, &req)
	writeJSON(w, http.StatusCreated, codeSecurityConfigurationJSON(c, s.baseURL(r)))
}

func (s *Server) handleGetDefaultCodeSecurityConfigurations(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	base := s.baseURL(r)
	out := []map[string]interface{}{}
	for _, c := range s.store.ListCodeSecurityConfigurations(org) {
		if c.DefaultForNewRepos == "" || c.DefaultForNewRepos == "none" {
			continue
		}
		out = append(out, map[string]interface{}{
			"default_for_new_repos": c.DefaultForNewRepos,
			"configuration":         codeSecurityConfigurationJSON(c, base),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// resolveCodeSecurityConfiguration parses {configuration_id} and loads the
// configuration, writing a 404 on failure.
func (s *Server) resolveCodeSecurityConfiguration(w http.ResponseWriter, r *http.Request) *CodeSecurityConfiguration {
	id, err := strconv.Atoi(r.PathValue("configuration_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil
	}
	c := s.store.GetCodeSecurityConfiguration(r.PathValue("org"), id)
	if c == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil
	}
	return c
}

func (s *Server) handleGetCodeSecurityConfiguration(w http.ResponseWriter, r *http.Request) {
	c := s.resolveCodeSecurityConfiguration(w, r)
	if c == nil {
		return
	}
	writeJSON(w, http.StatusOK, codeSecurityConfigurationJSON(c, s.baseURL(r)))
}

func (s *Server) handleUpdateCodeSecurityConfiguration(w http.ResponseWriter, r *http.Request) {
	c := s.resolveCodeSecurityConfiguration(w, r)
	if c == nil {
		return
	}
	var req codeSecurityConfigurationRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if !req.validateEnums(w) {
		return
	}
	updated, changed := s.store.UpdateCodeSecurityConfiguration(c.OrgLogin, c.ID, &req)
	if !changed {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, http.StatusOK, codeSecurityConfigurationJSON(updated, s.baseURL(r)))
}

func (s *Server) handleDeleteCodeSecurityConfiguration(w http.ResponseWriter, r *http.Request) {
	c := s.resolveCodeSecurityConfiguration(w, r)
	if c == nil {
		return
	}
	s.store.DeleteCodeSecurityConfiguration(c.OrgLogin, c.ID)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAttachCodeSecurityConfiguration(w http.ResponseWriter, r *http.Request) {
	c := s.resolveCodeSecurityConfiguration(w, r)
	if c == nil {
		return
	}
	var req struct {
		Scope                 *string `json:"scope"`
		SelectedRepositoryIDs []int   `json:"selected_repository_ids"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Scope == nil {
		writeGHValidationError(w, "CodeSecurityConfiguration", "scope", "missing_field")
		return
	}
	switch *req.Scope {
	case "all", "all_without_configurations", "public", "private_or_internal":
		if len(req.SelectedRepositoryIDs) > 0 {
			writeGHValidationError(w, "CodeSecurityConfiguration", "selected_repository_ids", "invalid")
			return
		}
	case "selected":
		if len(req.SelectedRepositoryIDs) == 0 {
			writeGHValidationError(w, "CodeSecurityConfiguration", "selected_repository_ids", "missing_field")
			return
		}
	default:
		writeGHValidationError(w, "CodeSecurityConfiguration", "scope", "invalid")
		return
	}
	if !s.store.AttachCodeSecurityConfiguration(c.OrgLogin, c.ID, *req.Scope, req.SelectedRepositoryIDs) {
		writeGHValidationError(w, "CodeSecurityConfiguration", "selected_repository_ids", "invalid")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]interface{}{})
}

func (s *Server) handleDetachCodeSecurityConfiguration(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	var req struct {
		SelectedRepositoryIDs []int `json:"selected_repository_ids"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if len(req.SelectedRepositoryIDs) == 0 || len(req.SelectedRepositoryIDs) > 250 {
		writeGHError(w, http.StatusBadRequest, "selected_repository_ids must contain between 1 and 250 repository IDs")
		return
	}
	s.store.DetachCodeSecurityConfigurations(org, req.SelectedRepositoryIDs)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSetCodeSecurityConfigurationAsDefault(w http.ResponseWriter, r *http.Request) {
	c := s.resolveCodeSecurityConfiguration(w, r)
	if c == nil {
		return
	}
	var req struct {
		DefaultForNewRepos *string `json:"default_for_new_repos"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.DefaultForNewRepos == nil {
		writeGHValidationError(w, "CodeSecurityConfiguration", "default_for_new_repos", "missing_field")
		return
	}
	switch *req.DefaultForNewRepos {
	case "all", "none", "private_and_internal", "public":
	default:
		writeGHValidationError(w, "CodeSecurityConfiguration", "default_for_new_repos", "invalid")
		return
	}
	updated := s.store.SetCodeSecurityConfigurationAsDefault(c.OrgLogin, c.ID, *req.DefaultForNewRepos)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"default_for_new_repos": updated.DefaultForNewRepos,
		"configuration":         codeSecurityConfigurationJSON(updated, s.baseURL(r)),
	})
}

func (s *Server) handleListCodeSecurityConfigurationRepositories(w http.ResponseWriter, r *http.Request) {
	c := s.resolveCodeSecurityConfiguration(w, r)
	if c == nil {
		return
	}
	statusFilter := r.URL.Query().Get("status")
	// Attachments apply synchronously, so "attached" is the only status a
	// repository can hold.
	if statusFilter != "" && statusFilter != "all" {
		matched := false
		for _, s := range strings.Split(statusFilter, ",") {
			if strings.TrimSpace(s) == "attached" {
				matched = true
				break
			}
		}
		if !matched {
			writeJSON(w, http.StatusOK, []map[string]interface{}{})
			return
		}
	}
	repos := s.store.ListCodeSecurityConfigurationRepos(c.OrgLogin, c.ID)
	base := s.baseURL(r)
	out := make([]map[string]interface{}, 0, len(repos))
	for _, repo := range repos {
		out = append(out, map[string]interface{}{
			"status":     "attached",
			"repository": simpleRepoJSON(repo, s.store, base),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGetRepoCodeSecurityConfiguration(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	name := r.PathValue("repo")
	repo := s.store.GetRepo(owner, name)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if !canReadRepo(s.store, ghUserFromContext(r.Context()), repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	c := s.store.GetRepoCodeSecurityConfiguration(owner, repo.ID)
	if c == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":        "attached",
		"configuration": codeSecurityConfigurationJSON(c, s.baseURL(r)),
	})
}

func codeSecurityConfigurationJSON(c *CodeSecurityConfiguration, baseURL string) map[string]interface{} {
	out := map[string]interface{}{
		"id":                                        c.ID,
		"name":                                      c.Name,
		"target_type":                               c.TargetType,
		"description":                               c.Description,
		"advanced_security":                         c.AdvancedSecurity,
		"dependency_graph":                          c.DependencyGraph,
		"dependency_graph_autosubmit_action":        c.DependencyGraphAutosubmitAction,
		"dependabot_alerts":                         c.DependabotAlerts,
		"dependabot_security_updates":               c.DependabotSecurityUpdates,
		"dependabot_delegated_alert_dismissal":      c.DependabotDelegatedAlertDismissal,
		"code_scanning_default_setup":               c.CodeScanningDefaultSetup,
		"code_scanning_delegated_alert_dismissal":   c.CodeScanningDelegatedAlertDismissal,
		"secret_scanning":                           c.SecretScanning,
		"secret_scanning_push_protection":           c.SecretScanningPushProtection,
		"secret_scanning_delegated_bypass":          c.SecretScanningDelegatedBypass,
		"secret_scanning_validity_checks":           c.SecretScanningValidityChecks,
		"secret_scanning_non_provider_patterns":     c.SecretScanningNonProviderPatterns,
		"secret_scanning_generic_secrets":           c.SecretScanningGenericSecrets,
		"secret_scanning_delegated_alert_dismissal": c.SecretScanningDelegatedDismissal,
		"private_vulnerability_reporting":           c.PrivateVulnerabilityReporting,
		"enforcement":                               c.Enforcement,
		"url":                                       baseURL + "/api/v3/orgs/" + c.OrgLogin + "/code-security/configurations/" + strconv.Itoa(c.ID),
		"html_url":                                  baseURL + "/organizations/" + c.OrgLogin + "/settings/security_products/configurations/view/" + strconv.Itoa(c.ID),
		"created_at":                                c.CreatedAt.Format(time.RFC3339),
		"updated_at":                                c.UpdatedAt.Format(time.RFC3339),
	}
	if c.SecretScanningExtendedMetadata != "" {
		out["secret_scanning_extended_metadata"] = c.SecretScanningExtendedMetadata
	}
	if c.CodeScanningAllowAdvanced != nil {
		out["code_scanning_options"] = map[string]interface{}{
			"allow_advanced": *c.CodeScanningAllowAdvanced,
		}
	}
	if c.DependencyGraphAutosubmitLabeled != nil {
		out["dependency_graph_autosubmit_action_options"] = map[string]interface{}{
			"labeled_runners": *c.DependencyGraphAutosubmitLabeled,
		}
	}
	if c.CodeScanningRunnerType != nil || c.CodeScanningRunnerLabel != nil {
		opts := map[string]interface{}{}
		if c.CodeScanningRunnerType != nil {
			opts["runner_type"] = *c.CodeScanningRunnerType
		}
		if c.CodeScanningRunnerLabel != nil {
			opts["runner_label"] = *c.CodeScanningRunnerLabel
		}
		out["code_scanning_default_setup_options"] = opts
	}
	return out
}

// --- store ---

// ListCodeSecurityConfigurations returns the org's configurations sorted by ID.
func (st *Store) ListCodeSecurityConfigurations(orgLogin string) []*CodeSecurityConfiguration {
	st.mu.RLock()
	defer st.mu.RUnlock()
	m := st.CodeSecurityConfigs[orgLogin]
	out := make([]*CodeSecurityConfiguration, 0, len(m))
	for _, c := range m {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// GetCodeSecurityConfiguration returns a configuration by org and ID, or nil.
func (st *Store) GetCodeSecurityConfiguration(orgLogin string, id int) *CodeSecurityConfiguration {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.CodeSecurityConfigs[orgLogin][id]
}

// GetCodeSecurityConfigurationByName returns a configuration by name, or nil.
func (st *Store) GetCodeSecurityConfigurationByName(orgLogin, name string) *CodeSecurityConfiguration {
	st.mu.RLock()
	defer st.mu.RUnlock()
	for _, c := range st.CodeSecurityConfigs[orgLogin] {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// CreateCodeSecurityConfiguration materializes a configuration with the
// documented per-field creation defaults, then applies the request.
func (st *Store) CreateCodeSecurityConfiguration(orgLogin string, req *codeSecurityConfigurationRequest) *CodeSecurityConfiguration {
	st.mu.Lock()
	defer st.mu.Unlock()
	now := time.Now().UTC()
	c := &CodeSecurityConfiguration{
		ID:                                  st.NextCodeSecurityConfigID,
		OrgLogin:                            orgLogin,
		TargetType:                          "organization",
		AdvancedSecurity:                    "disabled",
		DependencyGraph:                     "enabled",
		DependencyGraphAutosubmitAction:     "disabled",
		DependabotAlerts:                    "disabled",
		DependabotSecurityUpdates:           "disabled",
		DependabotDelegatedAlertDismissal:   "disabled",
		CodeScanningDefaultSetup:            "disabled",
		CodeScanningDelegatedAlertDismissal: "not_set",
		SecretScanning:                      "disabled",
		SecretScanningPushProtection:        "disabled",
		SecretScanningDelegatedBypass:       "disabled",
		SecretScanningValidityChecks:        "disabled",
		SecretScanningNonProviderPatterns:   "disabled",
		SecretScanningGenericSecrets:        "disabled",
		SecretScanningDelegatedDismissal:    "disabled",
		PrivateVulnerabilityReporting:       "disabled",
		Enforcement:                         "enforced",
		DefaultForNewRepos:                  "none",
		CreatedAt:                           now,
		UpdatedAt:                           now,
	}
	st.NextCodeSecurityConfigID++
	req.apply(c)
	if st.CodeSecurityConfigs[orgLogin] == nil {
		st.CodeSecurityConfigs[orgLogin] = map[int]*CodeSecurityConfiguration{}
	}
	st.CodeSecurityConfigs[orgLogin][c.ID] = c
	if st.persist != nil {
		st.persist.MustPut("code_security_configurations", orgLogin, st.CodeSecurityConfigs[orgLogin])
	}
	return c
}

// UpdateCodeSecurityConfiguration applies the request; the bool reports
// whether anything actually changed.
func (st *Store) UpdateCodeSecurityConfiguration(orgLogin string, id int, req *codeSecurityConfigurationRequest) (*CodeSecurityConfiguration, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	c := st.CodeSecurityConfigs[orgLogin][id]
	if c == nil {
		return nil, false
	}
	if !req.apply(c) {
		return c, false
	}
	c.UpdatedAt = time.Now().UTC()
	if st.persist != nil {
		st.persist.MustPut("code_security_configurations", orgLogin, st.CodeSecurityConfigs[orgLogin])
	}
	return c, true
}

// DeleteCodeSecurityConfiguration removes a configuration; repositories it
// was attached to retain their settings but lose the association.
func (st *Store) DeleteCodeSecurityConfiguration(orgLogin string, id int) {
	st.mu.Lock()
	defer st.mu.Unlock()
	delete(st.CodeSecurityConfigs[orgLogin], id)
	attachments := st.CodeSecurityRepoAttachments[orgLogin]
	for repoID, configID := range attachments {
		if configID == id {
			delete(attachments, repoID)
		}
	}
	if st.persist != nil {
		st.persist.MustPut("code_security_configurations", orgLogin, st.CodeSecurityConfigs[orgLogin])
		st.persist.MustPut("code_security_repo_attachments", orgLogin, attachments)
	}
}

// AttachCodeSecurityConfiguration applies the configuration to the repos the
// scope selects. Returns false when a selected repository ID is not an org
// repository.
func (st *Store) AttachCodeSecurityConfiguration(orgLogin string, id int, scope string, selectedIDs []int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	prefix := orgLogin + "/"
	orgRepos := map[int]*Repo{}
	for key, repo := range st.ReposByName {
		if strings.HasPrefix(key, prefix) {
			orgRepos[repo.ID] = repo
		}
	}
	if st.CodeSecurityRepoAttachments[orgLogin] == nil {
		st.CodeSecurityRepoAttachments[orgLogin] = map[int]int{}
	}
	attachments := st.CodeSecurityRepoAttachments[orgLogin]

	var targets []int
	switch scope {
	case "selected":
		for _, repoID := range selectedIDs {
			if orgRepos[repoID] == nil {
				return false
			}
			targets = append(targets, repoID)
		}
	default:
		for repoID, repo := range orgRepos {
			switch scope {
			case "all":
				targets = append(targets, repoID)
			case "all_without_configurations":
				if _, ok := attachments[repoID]; !ok {
					targets = append(targets, repoID)
				}
			case "public":
				if !repo.Private {
					targets = append(targets, repoID)
				}
			case "private_or_internal":
				if repo.Private {
					targets = append(targets, repoID)
				}
			}
		}
	}
	for _, repoID := range targets {
		attachments[repoID] = id
	}
	if st.persist != nil {
		st.persist.MustPut("code_security_repo_attachments", orgLogin, attachments)
	}
	return true
}

// DetachCodeSecurityConfigurations removes the configuration association
// from the given repositories.
func (st *Store) DetachCodeSecurityConfigurations(orgLogin string, repoIDs []int) {
	st.mu.Lock()
	defer st.mu.Unlock()
	attachments := st.CodeSecurityRepoAttachments[orgLogin]
	for _, repoID := range repoIDs {
		delete(attachments, repoID)
	}
	if st.persist != nil {
		st.persist.MustPut("code_security_repo_attachments", orgLogin, attachments)
	}
}

// SetCodeSecurityConfigurationAsDefault records the configuration's
// default-for-new-repositories policy, clearing overlapping defaults from
// other configurations.
func (st *Store) SetCodeSecurityConfigurationAsDefault(orgLogin string, id int, defaultFor string) *CodeSecurityConfiguration {
	st.mu.Lock()
	defer st.mu.Unlock()
	c := st.CodeSecurityConfigs[orgLogin][id]
	if defaultFor != "none" {
		for _, other := range st.CodeSecurityConfigs[orgLogin] {
			if other.ID == id || other.DefaultForNewRepos == "none" || other.DefaultForNewRepos == "" {
				continue
			}
			if defaultFor == "all" || other.DefaultForNewRepos == "all" || other.DefaultForNewRepos == defaultFor {
				other.DefaultForNewRepos = "none"
			}
		}
	}
	c.DefaultForNewRepos = defaultFor
	c.UpdatedAt = time.Now().UTC()
	if st.persist != nil {
		st.persist.MustPut("code_security_configurations", orgLogin, st.CodeSecurityConfigs[orgLogin])
	}
	return c
}

// ListCodeSecurityConfigurationRepos returns the repositories attached to
// the configuration, sorted by repo ID.
func (st *Store) ListCodeSecurityConfigurationRepos(orgLogin string, id int) []*Repo {
	st.mu.RLock()
	defer st.mu.RUnlock()
	out := []*Repo{}
	for repoID, configID := range st.CodeSecurityRepoAttachments[orgLogin] {
		if configID == id {
			if repo := st.Repos[repoID]; repo != nil {
				out = append(out, repo)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// GetRepoCodeSecurityConfiguration returns the configuration attached to the
// repository, or nil.
func (st *Store) GetRepoCodeSecurityConfiguration(orgLogin string, repoID int) *CodeSecurityConfiguration {
	st.mu.RLock()
	defer st.mu.RUnlock()
	configID, ok := st.CodeSecurityRepoAttachments[orgLogin][repoID]
	if !ok {
		return nil
	}
	return st.CodeSecurityConfigs[orgLogin][configID]
}
