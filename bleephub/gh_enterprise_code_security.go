package bleephub

import (
	"net/http"
	"strconv"
	"time"
)

func (s *Server) registerGHEnterpriseCodeSecurityRoutes() {
	s.route("GET /api/v3/enterprises/{enterprise}/code-security/configurations", s.requireEnterpriseOwner(s.handleListEnterpriseCodeSecurityConfigs))
	s.route("POST /api/v3/enterprises/{enterprise}/code-security/configurations", s.requireEnterpriseOwner(s.handleCreateEnterpriseCodeSecurityConfig))
	s.route("GET /api/v3/enterprises/{enterprise}/code-security/configurations/defaults", s.requireEnterpriseOwner(s.handleGetEnterpriseCodeSecurityDefaults))
	s.route("GET /api/v3/enterprises/{enterprise}/code-security/configurations/{configuration_id}", s.requireEnterpriseOwner(s.handleGetEnterpriseCodeSecurityConfig))
	s.route("PATCH /api/v3/enterprises/{enterprise}/code-security/configurations/{configuration_id}", s.requireEnterpriseOwner(s.handleUpdateEnterpriseCodeSecurityConfig))
	s.route("DELETE /api/v3/enterprises/{enterprise}/code-security/configurations/{configuration_id}", s.requireEnterpriseOwner(s.handleDeleteEnterpriseCodeSecurityConfig))
	s.route("POST /api/v3/enterprises/{enterprise}/code-security/configurations/{configuration_id}/attach", s.requireEnterpriseOwner(s.handleAttachEnterpriseCodeSecurityConfig))
	s.route("PUT /api/v3/enterprises/{enterprise}/code-security/configurations/{configuration_id}/defaults", s.requireEnterpriseOwner(s.handleSetEnterpriseCodeSecurityConfigDefault))
	s.route("GET /api/v3/enterprises/{enterprise}/code-security/configurations/{configuration_id}/repositories", s.requireEnterpriseOwner(s.handleListEnterpriseCodeSecurityConfigRepos))
}

// enterpriseCodeSecurityConfigRequest carries the members GitHub's
// enterprise code security configuration create/update bodies accept.
// Pointer members distinguish absent from explicit values so PATCH can apply
// only what the caller sent.
type enterpriseCodeSecurityConfigRequest struct {
	Name                                   *string `json:"name"`
	Description                            *string `json:"description"`
	AdvancedSecurity                       *string `json:"advanced_security"`
	CodeSecurity                           *string `json:"code_security"`
	SecretProtection                       *string `json:"secret_protection"`
	DependencyGraph                        *string `json:"dependency_graph"`
	DependencyGraphAutosubmitAction        *string `json:"dependency_graph_autosubmit_action"`
	DependencyGraphAutosubmitActionOptions *struct {
		LabeledRunners *bool `json:"labeled_runners"`
	} `json:"dependency_graph_autosubmit_action_options"`
	DependabotAlerts          *string `json:"dependabot_alerts"`
	DependabotSecurityUpdates *string `json:"dependabot_security_updates"`
	CodeScanningOptions       *struct {
		AllowAdvanced *bool `json:"allow_advanced"`
	} `json:"code_scanning_options"`
	CodeScanningDefaultSetup        *string `json:"code_scanning_default_setup"`
	CodeScanningDefaultSetupOptions *struct {
		RunnerType  *string `json:"runner_type"`
		RunnerLabel *string `json:"runner_label"`
	} `json:"code_scanning_default_setup_options"`
	CodeScanningDelegatedAlertDismissal   *string `json:"code_scanning_delegated_alert_dismissal"`
	SecretScanning                        *string `json:"secret_scanning"`
	SecretScanningPushProtection          *string `json:"secret_scanning_push_protection"`
	SecretScanningValidityChecks          *string `json:"secret_scanning_validity_checks"`
	SecretScanningNonProviderPatterns     *string `json:"secret_scanning_non_provider_patterns"`
	SecretScanningGenericSecrets          *string `json:"secret_scanning_generic_secrets"`
	SecretScanningDelegatedAlertDismissal *string `json:"secret_scanning_delegated_alert_dismissal"`
	SecretScanningExtendedMetadata        *string `json:"secret_scanning_extended_metadata"`
	PrivateVulnerabilityReporting         *string `json:"private_vulnerability_reporting"`
	Enforcement                           *string `json:"enforcement"`
}

// validateEnterpriseCodeSecurityEnums checks every enum-valued member of the
// request, returning the first offending field name.
func (req *enterpriseCodeSecurityConfigRequest) validate() (string, bool) {
	feature := func(v *string) bool {
		return v == nil || *v == "enabled" || *v == "disabled" || *v == "not_set"
	}
	if req.AdvancedSecurity != nil {
		switch *req.AdvancedSecurity {
		case "enabled", "disabled", "code_security", "secret_protection":
		default:
			return "advanced_security", false
		}
	}
	for field, v := range map[string]*string{
		"code_security":                             req.CodeSecurity,
		"secret_protection":                         req.SecretProtection,
		"dependency_graph":                          req.DependencyGraph,
		"dependency_graph_autosubmit_action":        req.DependencyGraphAutosubmitAction,
		"dependabot_alerts":                         req.DependabotAlerts,
		"dependabot_security_updates":               req.DependabotSecurityUpdates,
		"code_scanning_default_setup":               req.CodeScanningDefaultSetup,
		"code_scanning_delegated_alert_dismissal":   req.CodeScanningDelegatedAlertDismissal,
		"secret_scanning":                           req.SecretScanning,
		"secret_scanning_push_protection":           req.SecretScanningPushProtection,
		"secret_scanning_validity_checks":           req.SecretScanningValidityChecks,
		"secret_scanning_non_provider_patterns":     req.SecretScanningNonProviderPatterns,
		"secret_scanning_generic_secrets":           req.SecretScanningGenericSecrets,
		"secret_scanning_delegated_alert_dismissal": req.SecretScanningDelegatedAlertDismissal,
		"secret_scanning_extended_metadata":         req.SecretScanningExtendedMetadata,
		"private_vulnerability_reporting":           req.PrivateVulnerabilityReporting,
	} {
		if !feature(v) {
			return field, false
		}
	}
	if req.Enforcement != nil && *req.Enforcement != "enforced" && *req.Enforcement != "unenforced" {
		return "enforcement", false
	}
	return "", true
}

// apply copies the members present in the request onto the configuration.
// The request-only code_security / secret_protection aggregate toggles fold
// into advanced_security exactly as GitHub reports them back: both enabled →
// "enabled", one enabled → that product's value; an explicit
// advanced_security member wins.
func (req *enterpriseCodeSecurityConfigRequest) apply(c *EnterpriseCodeSecurityConfiguration) {
	setStr := func(dst *string, v *string) {
		if v != nil {
			*dst = *v
		}
	}
	setStr(&c.Name, req.Name)
	setStr(&c.Description, req.Description)
	setStr(&c.DependencyGraph, req.DependencyGraph)
	setStr(&c.DependencyGraphAutosubmitAction, req.DependencyGraphAutosubmitAction)
	if req.DependencyGraphAutosubmitActionOptions != nil && req.DependencyGraphAutosubmitActionOptions.LabeledRunners != nil {
		c.DependencyGraphAutosubmitLabeled = *req.DependencyGraphAutosubmitActionOptions.LabeledRunners
	}
	setStr(&c.DependabotAlerts, req.DependabotAlerts)
	setStr(&c.DependabotSecurityUpdates, req.DependabotSecurityUpdates)
	if req.CodeScanningOptions != nil {
		c.CodeScanningAllowAdvanced = req.CodeScanningOptions.AllowAdvanced
	}
	setStr(&c.CodeScanningDefaultSetup, req.CodeScanningDefaultSetup)
	if req.CodeScanningDefaultSetupOptions != nil {
		c.CodeScanningRunnerType = req.CodeScanningDefaultSetupOptions.RunnerType
		c.CodeScanningRunnerLabel = req.CodeScanningDefaultSetupOptions.RunnerLabel
	}
	setStr(&c.CodeScanningDelegatedAlertDismissal, req.CodeScanningDelegatedAlertDismissal)
	setStr(&c.SecretScanning, req.SecretScanning)
	setStr(&c.SecretScanningPushProtection, req.SecretScanningPushProtection)
	setStr(&c.SecretScanningValidityChecks, req.SecretScanningValidityChecks)
	setStr(&c.SecretScanningNonProviderPatterns, req.SecretScanningNonProviderPatterns)
	setStr(&c.SecretScanningGenericSecrets, req.SecretScanningGenericSecrets)
	setStr(&c.SecretScanningDelegatedAlertDismissal, req.SecretScanningDelegatedAlertDismissal)
	setStr(&c.SecretScanningExtendedMetadata, req.SecretScanningExtendedMetadata)
	setStr(&c.PrivateVulnerabilityReporting, req.PrivateVulnerabilityReporting)
	setStr(&c.Enforcement, req.Enforcement)

	switch {
	case req.AdvancedSecurity != nil:
		c.AdvancedSecurity = *req.AdvancedSecurity
	case req.CodeSecurity != nil || req.SecretProtection != nil:
		cs := req.CodeSecurity != nil && *req.CodeSecurity == "enabled"
		sp := req.SecretProtection != nil && *req.SecretProtection == "enabled"
		switch {
		case cs && sp:
			c.AdvancedSecurity = "enabled"
		case cs:
			c.AdvancedSecurity = "code_security"
		case sp:
			c.AdvancedSecurity = "secret_protection"
		default:
			c.AdvancedSecurity = "disabled"
		}
	}
}

// enterpriseCodeSecurityConfigJSON renders the GitHub
// code-security-configuration schema shape with target_type "enterprise".
func (s *Server) enterpriseCodeSecurityConfigJSON(c *EnterpriseCodeSecurityConfiguration, baseURL string) map[string]interface{} {
	api := baseURL + "/api/v3/enterprises/" + s.enterpriseSlug() + "/code-security/configurations/" + strconv.Itoa(c.ID)
	var codeScanningOptions interface{}
	if c.CodeScanningAllowAdvanced != nil {
		codeScanningOptions = map[string]interface{}{"allow_advanced": *c.CodeScanningAllowAdvanced}
	}
	var defaultSetupOptions interface{}
	if c.CodeScanningRunnerType != nil || c.CodeScanningRunnerLabel != nil {
		opts := map[string]interface{}{}
		if c.CodeScanningRunnerType != nil {
			opts["runner_type"] = *c.CodeScanningRunnerType
		}
		if c.CodeScanningRunnerLabel != nil {
			opts["runner_label"] = *c.CodeScanningRunnerLabel
		}
		defaultSetupOptions = opts
	}
	return map[string]interface{}{
		"id":                                 c.ID,
		"name":                               c.Name,
		"target_type":                        "enterprise",
		"description":                        c.Description,
		"advanced_security":                  c.AdvancedSecurity,
		"dependency_graph":                   c.DependencyGraph,
		"dependency_graph_autosubmit_action": c.DependencyGraphAutosubmitAction,
		"dependency_graph_autosubmit_action_options": map[string]interface{}{
			"labeled_runners": c.DependencyGraphAutosubmitLabeled,
		},
		"dependabot_alerts":                         c.DependabotAlerts,
		"dependabot_security_updates":               c.DependabotSecurityUpdates,
		"code_scanning_options":                     codeScanningOptions,
		"code_scanning_default_setup":               c.CodeScanningDefaultSetup,
		"code_scanning_default_setup_options":       defaultSetupOptions,
		"code_scanning_delegated_alert_dismissal":   c.CodeScanningDelegatedAlertDismissal,
		"secret_scanning":                           c.SecretScanning,
		"secret_scanning_push_protection":           c.SecretScanningPushProtection,
		"secret_scanning_validity_checks":           c.SecretScanningValidityChecks,
		"secret_scanning_non_provider_patterns":     c.SecretScanningNonProviderPatterns,
		"secret_scanning_generic_secrets":           c.SecretScanningGenericSecrets,
		"secret_scanning_delegated_alert_dismissal": c.SecretScanningDelegatedAlertDismissal,
		"secret_scanning_extended_metadata":         c.SecretScanningExtendedMetadata,
		"private_vulnerability_reporting":           c.PrivateVulnerabilityReporting,
		"enforcement":                               c.Enforcement,
		"url":                                       api,
		"html_url":                                  baseURL + "/enterprises/" + s.enterpriseSlug() + "/settings/security_products/configurations/edit/" + strconv.Itoa(c.ID),
		"created_at":                                c.CreatedAt.Format(time.RFC3339),
		"updated_at":                                c.UpdatedAt.Format(time.RFC3339),
	}
}

func (s *Server) handleListEnterpriseCodeSecurityConfigs(w http.ResponseWriter, r *http.Request) {
	configs := s.store.ListEnterpriseCodeSecurityConfigs()
	base := s.baseURL(r)
	out := make([]map[string]interface{}, 0, len(configs))
	for _, c := range cursorPageByID(r, configs, func(c *EnterpriseCodeSecurityConfiguration) int { return c.ID }) {
		out = append(out, s.enterpriseCodeSecurityConfigJSON(c, base))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCreateEnterpriseCodeSecurityConfig(w http.ResponseWriter, r *http.Request) {
	var req enterpriseCodeSecurityConfigRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	// name and description are the schema's required members; GitHub
	// documents 400 (not 422) for a bad enterprise configuration request.
	if req.Name == nil || *req.Name == "" || req.Description == nil {
		writeGHError(w, http.StatusBadRequest, "Invalid request. name and description are required.")
		return
	}
	if field, ok := req.validate(); !ok {
		writeGHError(w, http.StatusBadRequest, "Invalid request. Invalid value for "+field+".")
		return
	}

	c := &EnterpriseCodeSecurityConfiguration{
		// Defaults per GitHub's create schema: dependency graph on, every
		// other feature off, enforcement on.
		AdvancedSecurity:                      "disabled",
		DependencyGraph:                       "enabled",
		DependencyGraphAutosubmitAction:       "disabled",
		DependabotAlerts:                      "disabled",
		DependabotSecurityUpdates:             "disabled",
		CodeScanningDefaultSetup:              "disabled",
		CodeScanningDelegatedAlertDismissal:   "disabled",
		SecretScanning:                        "disabled",
		SecretScanningPushProtection:          "disabled",
		SecretScanningValidityChecks:          "disabled",
		SecretScanningNonProviderPatterns:     "disabled",
		SecretScanningGenericSecrets:          "disabled",
		SecretScanningDelegatedAlertDismissal: "disabled",
		SecretScanningExtendedMetadata:        "disabled",
		PrivateVulnerabilityReporting:         "disabled",
		Enforcement:                           "enforced",
	}
	req.apply(c)
	s.store.CreateEnterpriseCodeSecurityConfig(c)
	writeJSON(w, http.StatusCreated, s.enterpriseCodeSecurityConfigJSON(c, s.baseURL(r)))
}

// lookupEnterpriseCodeSecurityConfig resolves {configuration_id}, writing 404
// when absent.
func (s *Server) lookupEnterpriseCodeSecurityConfig(w http.ResponseWriter, r *http.Request) *EnterpriseCodeSecurityConfiguration {
	id, err := strconv.Atoi(r.PathValue("configuration_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil
	}
	c := s.store.GetEnterpriseCodeSecurityConfig(id)
	if c == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil
	}
	return c
}

func (s *Server) handleGetEnterpriseCodeSecurityConfig(w http.ResponseWriter, r *http.Request) {
	c := s.lookupEnterpriseCodeSecurityConfig(w, r)
	if c == nil {
		return
	}
	writeJSON(w, http.StatusOK, s.enterpriseCodeSecurityConfigJSON(c, s.baseURL(r)))
}

func (s *Server) handleUpdateEnterpriseCodeSecurityConfig(w http.ResponseWriter, r *http.Request) {
	c := s.lookupEnterpriseCodeSecurityConfig(w, r)
	if c == nil {
		return
	}
	var req enterpriseCodeSecurityConfigRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if field, ok := req.validate(); !ok {
		writeGHValidationError(w, "CodeSecurityConfiguration", field, "invalid")
		return
	}
	s.store.TouchEnterpriseCodeSecurityConfig(c, func() { req.apply(c) })
	writeJSON(w, http.StatusOK, s.enterpriseCodeSecurityConfigJSON(c, s.baseURL(r)))
}

func (s *Server) handleDeleteEnterpriseCodeSecurityConfig(w http.ResponseWriter, r *http.Request) {
	c := s.lookupEnterpriseCodeSecurityConfig(w, r)
	if c == nil {
		return
	}
	deleted, conflict := s.store.DeleteEnterpriseCodeSecurityConfig(c.ID)
	if conflict {
		writeGHError(w, http.StatusConflict, "Cannot delete a configuration that is set as a default for new repositories.")
		return
	}
	if !deleted {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAttachEnterpriseCodeSecurityConfig(w http.ResponseWriter, r *http.Request) {
	c := s.lookupEnterpriseCodeSecurityConfig(w, r)
	if c == nil {
		return
	}
	var req struct {
		Scope string `json:"scope"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Scope != "all" && req.Scope != "all_without_configurations" {
		writeGHValidationError(w, "CodeSecurityConfiguration", "scope", "invalid")
		return
	}
	s.store.AttachEnterpriseCodeSecurityConfig(c, req.Scope)
	writeJSON(w, http.StatusAccepted, map[string]interface{}{})
}

func (s *Server) handleSetEnterpriseCodeSecurityConfigDefault(w http.ResponseWriter, r *http.Request) {
	c := s.lookupEnterpriseCodeSecurityConfig(w, r)
	if c == nil {
		return
	}
	var req struct {
		DefaultForNewRepos string `json:"default_for_new_repos"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	switch req.DefaultForNewRepos {
	case "all", "none", "private_and_internal", "public":
	default:
		writeGHValidationError(w, "CodeSecurityConfiguration", "default_for_new_repos", "invalid")
		return
	}
	s.store.SetEnterpriseCodeSecurityConfigDefault(c, req.DefaultForNewRepos)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"default_for_new_repos": c.DefaultForNewRepos,
		"configuration":         s.enterpriseCodeSecurityConfigJSON(c, s.baseURL(r)),
	})
}

func (s *Server) handleGetEnterpriseCodeSecurityDefaults(w http.ResponseWriter, r *http.Request) {
	base := s.baseURL(r)
	out := make([]map[string]interface{}, 0)
	for _, c := range s.store.ListEnterpriseCodeSecurityConfigs() {
		if c.DefaultForNewRepos == "none" || c.DefaultForNewRepos == "" {
			continue
		}
		out = append(out, map[string]interface{}{
			"default_for_new_repos": c.DefaultForNewRepos,
			"configuration":         s.enterpriseCodeSecurityConfigJSON(c, base),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleListEnterpriseCodeSecurityConfigRepos(w http.ResponseWriter, r *http.Request) {
	c := s.lookupEnterpriseCodeSecurityConfig(w, r)
	if c == nil {
		return
	}
	// Attachments in bleephub are synchronous, so every association is in
	// the terminal "attached" state; a status filter naming other states
	// yields nothing.
	status := r.URL.Query().Get("status")
	base := s.baseURL(r)
	out := make([]map[string]interface{}, 0)
	if status == "" || status == "all" || statusFilterContains(status, "attached") {
		repos := s.store.ListEnterpriseCodeSecurityConfigRepos(c.ID)
		for _, repo := range cursorPageByID(r, repos, func(rp *Repo) int { return rp.ID }) {
			out = append(out, map[string]interface{}{
				"status":     "attached",
				"repository": simpleRepoJSON(repo, s.store, base),
			})
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// statusFilterContains reports whether the comma-separated status filter
// names the given state.
func statusFilterContains(filter, state string) bool {
	for _, part := range splitCommaList(filter) {
		if part == state {
			return true
		}
	}
	return false
}

// cursorPageByID implements the cursor pagination the code security list
// endpoints use (per_page + before/after, where the cursor is the item ID):
// "after" returns items with IDs strictly greater, "before" strictly smaller
// (keeping the window closest to the cursor), and per_page caps the page.
func cursorPageByID[T any](r *http.Request, items []T, id func(T) int) []T {
	q := r.URL.Query()
	perPage := 30
	if v, err := strconv.Atoi(q.Get("per_page")); err == nil && v > 0 {
		perPage = v
		if perPage > 100 {
			perPage = 100
		}
	}
	if after, err := strconv.Atoi(q.Get("after")); err == nil {
		for len(items) > 0 && id(items[0]) <= after {
			items = items[1:]
		}
	}
	if before, err := strconv.Atoi(q.Get("before")); err == nil {
		for len(items) > 0 && id(items[len(items)-1]) >= before {
			items = items[:len(items)-1]
		}
		if len(items) > perPage {
			items = items[len(items)-perPage:]
		}
		return items
	}
	if len(items) > perPage {
		items = items[:perPage]
	}
	return items
}
