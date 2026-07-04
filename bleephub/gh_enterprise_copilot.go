package bleephub

import (
	"net/http"
	"time"
)

func (s *Server) registerGHEnterpriseCopilotRoutes() {
	s.route("PUT /api/v3/enterprises/{enterprise}/copilot/policies/coding_agent", s.requireEnterpriseOwner(s.handleSetEnterpriseCopilotCodingAgentPolicy))
	s.route("POST /api/v3/enterprises/{enterprise}/copilot/policies/coding_agent/organizations", s.requireEnterpriseOwner(s.handleAddEnterpriseCopilotCodingAgentOrgs))
	s.route("DELETE /api/v3/enterprises/{enterprise}/copilot/policies/coding_agent/organizations", s.requireEnterpriseOwner(s.handleRemoveEnterpriseCopilotCodingAgentOrgs))

	s.route("GET /api/v3/enterprises/{enterprise}/copilot/metrics/reports/enterprise-1-day", s.requireEnterpriseOwner(s.handleEnterpriseCopilotOneDayReport))
	s.route("GET /api/v3/enterprises/{enterprise}/copilot/metrics/reports/enterprise-28-day/latest", s.requireEnterpriseOwner(s.handleEnterpriseCopilotLatest28DayReport))
	s.route("GET /api/v3/enterprises/{enterprise}/copilot/metrics/reports/user-teams-1-day", s.requireEnterpriseOwner(s.handleEnterpriseCopilotOneDayReport))
	s.route("GET /api/v3/enterprises/{enterprise}/copilot/metrics/reports/users-1-day", s.requireEnterpriseOwner(s.handleEnterpriseCopilotOneDayReport))
	s.route("GET /api/v3/enterprises/{enterprise}/copilot/metrics/reports/users-28-day/latest", s.requireEnterpriseOwner(s.handleEnterpriseCopilotLatest28DayReport))
}

// --- Copilot coding agent policy ---

func (s *Server) handleSetEnterpriseCopilotCodingAgentPolicy(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PolicyState string `json:"policy_state"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	switch req.PolicyState {
	case "enabled_for_all_orgs", "disabled_for_all_orgs", "enabled_for_selected_orgs", "configured_by_org_admins":
	default:
		writeGHError(w, http.StatusBadRequest, "Invalid request. policy_state must be one of enabled_for_all_orgs, disabled_for_all_orgs, enabled_for_selected_orgs, configured_by_org_admins.")
		return
	}
	s.store.SetEnterpriseCopilotCodingAgentPolicy(req.PolicyState)
	w.WriteHeader(http.StatusNoContent)
}

// copilotCodingAgentOrgSelection carries the organization selection body the
// coding agent policy add/remove endpoints accept: explicit logins and/or
// custom property filters.
type copilotCodingAgentOrgSelection struct {
	Organizations    []string `json:"organizations"`
	CustomProperties []struct {
		PropertyName string   `json:"property_name"`
		Values       []string `json:"values"`
	} `json:"custom_properties"`
}

// resolveCopilotCodingAgentOrgs resolves the selection to organization logins
// that exist on the instance — GitHub only affects organizations that belong
// to the enterprise, silently skipping the rest. bleephub stores no
// organization custom property values, so property filters honestly match no
// organizations.
func (s *Server) resolveCopilotCodingAgentOrgs(sel copilotCodingAgentOrgSelection) []string {
	var out []string
	for _, login := range sel.Organizations {
		if org := s.store.GetOrg(login); org != nil {
			out = append(out, org.Login)
		}
	}
	return out
}

// requireSelectedOrgsPolicy 400s unless the coding agent policy is
// enabled_for_selected_orgs — the documented precondition for editing the
// organization selection.
func (s *Server) requireSelectedOrgsPolicy(w http.ResponseWriter) bool {
	s.store.mu.RLock()
	policy := s.store.EnterpriseSettings.CopilotCodingAgentPolicy
	s.store.mu.RUnlock()
	if policy != "enabled_for_selected_orgs" {
		writeGHError(w, http.StatusBadRequest, "The enterprise's coding agent policy must be set to enabled_for_selected_orgs before using this endpoint.")
		return false
	}
	return true
}

func (s *Server) handleAddEnterpriseCopilotCodingAgentOrgs(w http.ResponseWriter, r *http.Request) {
	var req copilotCodingAgentOrgSelection
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if !s.requireSelectedOrgsPolicy(w) {
		return
	}
	s.store.AddEnterpriseCopilotCodingAgentOrgs(s.resolveCopilotCodingAgentOrgs(req))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRemoveEnterpriseCopilotCodingAgentOrgs(w http.ResponseWriter, r *http.Request) {
	var req copilotCodingAgentOrgSelection
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if !s.requireSelectedOrgsPolicy(w) {
		return
	}
	s.store.RemoveEnterpriseCopilotCodingAgentOrgs(s.resolveCopilotCodingAgentOrgs(req))
	w.WriteHeader(http.StatusNoContent)
}

// --- Copilot usage metrics reports ---
//
// The report endpoints return links to downloadable NDJSON report artifacts
// derived from recorded Copilot usage activity. bleephub records no Copilot
// usage events (no Copilot client surface exists), so every report is
// honestly empty — the documented shape with no download artifacts — rather
// than fabricated usage numbers.

func (s *Server) handleEnterpriseCopilotOneDayReport(w http.ResponseWriter, r *http.Request) {
	day := r.URL.Query().Get("day")
	if day == "" {
		writeGHValidationError(w, "CopilotUsageMetricsReport", "day", "missing")
		return
	}
	parsed, err := time.Parse("2006-01-02", day)
	if err != nil {
		writeGHValidationError(w, "CopilotUsageMetricsReport", "day", "invalid")
		return
	}
	// A report can only exist for days that have already happened.
	if parsed.After(time.Now().UTC().Truncate(24 * time.Hour)) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"download_links": []string{},
		"report_day":     parsed.Format("2006-01-02"),
	})
}

func (s *Server) handleEnterpriseCopilotLatest28DayReport(w http.ResponseWriter, r *http.Request) {
	// The latest 28-day report covers the 28 full days ending yesterday.
	end := time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, -1)
	start := end.AddDate(0, 0, -27)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"download_links":   []string{},
		"report_start_day": start.Format("2006-01-02"),
		"report_end_day":   end.Format("2006-01-02"),
	})
}
