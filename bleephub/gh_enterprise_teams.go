package bleephub

import (
	"encoding/json"
	"io"
	"net/http"
	"time"
)

func (s *Server) registerGHEnterpriseTeamRoutes() {
	s.route("GET /api/v3/enterprises/{enterprise}/teams", s.requireEnterpriseMember(s.handleListEnterpriseTeams))
	s.route("POST /api/v3/enterprises/{enterprise}/teams", s.requireEnterpriseOwner(s.handleCreateEnterpriseTeam))
	s.route("GET /api/v3/enterprises/{enterprise}/teams/{team_slug}", s.requireEnterpriseMember(s.handleGetEnterpriseTeam))
	s.route("PATCH /api/v3/enterprises/{enterprise}/teams/{team_slug}", s.requireEnterpriseOwner(s.handleUpdateEnterpriseTeam))
	s.route("DELETE /api/v3/enterprises/{enterprise}/teams/{team_slug}", s.requireEnterpriseOwner(s.handleDeleteEnterpriseTeam))

	s.route("GET /api/v3/enterprises/{enterprise}/teams/{team_slug}/memberships", s.requireEnterpriseMember(s.handleListEnterpriseTeamMemberships))
	s.route("POST /api/v3/enterprises/{enterprise}/teams/{team_slug}/memberships/add", s.requireEnterpriseOwner(s.handleBulkAddEnterpriseTeamMemberships))
	s.route("POST /api/v3/enterprises/{enterprise}/teams/{team_slug}/memberships/remove", s.requireEnterpriseOwner(s.handleBulkRemoveEnterpriseTeamMemberships))
	s.route("GET /api/v3/enterprises/{enterprise}/teams/{team_slug}/memberships/{username}", s.requireEnterpriseMember(s.handleGetEnterpriseTeamMembership))
	s.route("PUT /api/v3/enterprises/{enterprise}/teams/{team_slug}/memberships/{username}", s.requireEnterpriseOwner(s.handleAddEnterpriseTeamMembership))
	s.route("DELETE /api/v3/enterprises/{enterprise}/teams/{team_slug}/memberships/{username}", s.requireEnterpriseOwner(s.handleRemoveEnterpriseTeamMembership))

	s.route("GET /api/v3/enterprises/{enterprise}/teams/{team_slug}/organizations", s.requireEnterpriseMember(s.handleListEnterpriseTeamOrgs))
	s.route("POST /api/v3/enterprises/{enterprise}/teams/{team_slug}/organizations/add", s.requireEnterpriseOwner(s.handleBulkAddEnterpriseTeamOrgs))
	s.route("POST /api/v3/enterprises/{enterprise}/teams/{team_slug}/organizations/remove", s.requireEnterpriseOwner(s.handleBulkRemoveEnterpriseTeamOrgs))
	s.route("GET /api/v3/enterprises/{enterprise}/teams/{team_slug}/organizations/{org}", s.requireEnterpriseMember(s.handleGetEnterpriseTeamOrg))
	s.route("PUT /api/v3/enterprises/{enterprise}/teams/{team_slug}/organizations/{org}", s.requireEnterpriseOwner(s.handleAddEnterpriseTeamOrg))
	s.route("DELETE /api/v3/enterprises/{enterprise}/teams/{team_slug}/organizations/{org}", s.requireEnterpriseOwner(s.handleRemoveEnterpriseTeamOrg))
}

// enterpriseTeamJSON renders the GitHub `enterprise-team` schema shape.
func (s *Server) enterpriseTeamJSON(t *EnterpriseTeam, baseURL string) map[string]interface{} {
	slug := s.enterpriseSlug()
	api := baseURL + "/api/v3/enterprises/" + slug + "/teams/" + t.Slug
	var groupID interface{}
	if t.GroupID != nil {
		groupID = *t.GroupID
	}
	return map[string]interface{}{
		"id":                          t.ID,
		"name":                        t.Name,
		"description":                 nullOrString(t.Description),
		"slug":                        t.Slug,
		"url":                         api,
		"organization_selection_type": t.OrganizationSelectionType,
		"group_id":                    groupID,
		"html_url":                    baseURL + "/enterprises/" + slug + "/teams/" + t.Slug,
		"members_url":                 api + "/members{/member}",
		"notification_setting":        t.NotificationSetting,
		"created_at":                  t.CreatedAt.Format(time.RFC3339),
		"updated_at":                  t.UpdatedAt.Format(time.RFC3339),
	}
}

// validEnterpriseTeamEnums checks the enum-valued members a create/update
// body may carry ("" = absent, allowed).
func validEnterpriseTeamEnums(selectionType, notificationSetting string) (string, bool) {
	switch selectionType {
	case "", "disabled", "selected", "all":
	default:
		return "organization_selection_type", false
	}
	switch notificationSetting {
	case "", "notifications_enabled", "notifications_disabled":
	default:
		return "notification_setting", false
	}
	return "", true
}

func (s *Server) handleListEnterpriseTeams(w http.ResponseWriter, r *http.Request) {
	teams := s.store.ListEnterpriseTeams()
	page := paginateAndLink(w, r, teams)
	base := s.baseURL(r)
	out := make([]map[string]interface{}, 0, len(page))
	for _, t := range page {
		out = append(out, s.enterpriseTeamJSON(t, base))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCreateEnterpriseTeam(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name                      string  `json:"name"`
		Description               string  `json:"description"`
		OrganizationSelectionType string  `json:"organization_selection_type"`
		GroupID                   *string `json:"group_id"`
		NotificationSetting       string  `json:"notification_setting"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Name == "" {
		writeGHValidationError(w, "EnterpriseTeam", "name", "missing_field")
		return
	}
	if field, ok := validEnterpriseTeamEnums(req.OrganizationSelectionType, req.NotificationSetting); !ok {
		writeGHValidationError(w, "EnterpriseTeam", field, "invalid")
		return
	}

	team := s.store.CreateEnterpriseTeam(req.Name, req.Description, req.OrganizationSelectionType, req.GroupID, req.NotificationSetting)
	if team == nil {
		writeGHValidationError(w, "EnterpriseTeam", "name", "already_exists")
		return
	}
	writeJSON(w, http.StatusCreated, s.enterpriseTeamJSON(team, s.baseURL(r)))
}

// lookupEnterpriseTeam resolves {team_slug}, writing 404 when absent.
func (s *Server) lookupEnterpriseTeam(w http.ResponseWriter, r *http.Request) *EnterpriseTeam {
	team := s.store.GetEnterpriseTeam(r.PathValue("team_slug"))
	if team == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil
	}
	return team
}

func (s *Server) handleGetEnterpriseTeam(w http.ResponseWriter, r *http.Request) {
	team := s.lookupEnterpriseTeam(w, r)
	if team == nil {
		return
	}
	writeJSON(w, http.StatusOK, s.enterpriseTeamJSON(team, s.baseURL(r)))
}

func (s *Server) handleUpdateEnterpriseTeam(w http.ResponseWriter, r *http.Request) {
	team := s.lookupEnterpriseTeam(w, r)
	if team == nil {
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeGHError(w, http.StatusBadRequest, "Problems parsing JSON")
		return
	}
	var req struct {
		Name                      *string `json:"name"`
		Description               *string `json:"description"`
		OrganizationSelectionType *string `json:"organization_selection_type"`
		GroupID                   *string `json:"group_id"`
		NotificationSetting       *string `json:"notification_setting"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeGHError(w, http.StatusBadRequest, "Problems parsing JSON")
		return
	}
	// A raw view of the same (already-validated) bytes distinguishes an
	// explicit {"group_id": null} — which unlinks the IdP group — from an
	// absent member, which keeps it.
	var raw map[string]json.RawMessage
	_ = json.Unmarshal(body, &raw)
	selType, notif := "", ""
	if req.OrganizationSelectionType != nil {
		selType = *req.OrganizationSelectionType
	}
	if req.NotificationSetting != nil {
		notif = *req.NotificationSetting
	}
	if field, ok := validEnterpriseTeamEnums(selType, notif); !ok {
		writeGHValidationError(w, "EnterpriseTeam", field, "invalid")
		return
	}

	var groupID **string
	if _, present := raw["group_id"]; present {
		groupID = &req.GroupID
	}
	if !s.store.UpdateEnterpriseTeam(team, req.Name, req.Description, req.OrganizationSelectionType, req.NotificationSetting, groupID) {
		writeGHValidationError(w, "EnterpriseTeam", "name", "already_exists")
		return
	}
	writeJSON(w, http.StatusOK, s.enterpriseTeamJSON(team, s.baseURL(r)))
}

func (s *Server) handleDeleteEnterpriseTeam(w http.ResponseWriter, r *http.Request) {
	team := s.lookupEnterpriseTeam(w, r)
	if team == nil {
		return
	}
	s.store.DeleteEnterpriseTeam(team.Slug)
	w.WriteHeader(http.StatusNoContent)
}

// --- memberships ---

func (s *Server) handleListEnterpriseTeamMemberships(w http.ResponseWriter, r *http.Request) {
	team := s.lookupEnterpriseTeam(w, r)
	if team == nil {
		return
	}
	members := s.store.ListEnterpriseTeamMembers(team)
	page := paginateAndLink(w, r, members)
	out := make([]map[string]interface{}, 0, len(page))
	for _, u := range page {
		out = append(out, userToJSON(u))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleBulkAddEnterpriseTeamMemberships(w http.ResponseWriter, r *http.Request) {
	team := s.lookupEnterpriseTeam(w, r)
	if team == nil {
		return
	}
	var req struct {
		Usernames []string `json:"usernames"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if len(req.Usernames) == 0 {
		writeGHValidationError(w, "EnterpriseTeamMembership", "usernames", "missing_field")
		return
	}
	out := make([]map[string]interface{}, 0, len(req.Usernames))
	for _, login := range req.Usernames {
		u := s.store.LookupUserByLogin(login)
		if u == nil {
			writeGHValidationError(w, "EnterpriseTeamMembership", "usernames", "invalid")
			return
		}
		s.store.AddEnterpriseTeamMember(team, u.ID)
		out = append(out, userToJSON(u))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleBulkRemoveEnterpriseTeamMemberships(w http.ResponseWriter, r *http.Request) {
	team := s.lookupEnterpriseTeam(w, r)
	if team == nil {
		return
	}
	var req struct {
		Usernames []string `json:"usernames"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if len(req.Usernames) == 0 {
		writeGHValidationError(w, "EnterpriseTeamMembership", "usernames", "missing_field")
		return
	}
	out := make([]map[string]interface{}, 0, len(req.Usernames))
	for _, login := range req.Usernames {
		u := s.store.LookupUserByLogin(login)
		if u == nil {
			writeGHValidationError(w, "EnterpriseTeamMembership", "usernames", "invalid")
			return
		}
		if s.store.RemoveEnterpriseTeamMember(team, u.ID) {
			out = append(out, userToJSON(u))
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGetEnterpriseTeamMembership(w http.ResponseWriter, r *http.Request) {
	team := s.lookupEnterpriseTeam(w, r)
	if team == nil {
		return
	}
	u := s.store.LookupUserByLogin(r.PathValue("username"))
	if u == nil || !s.store.IsEnterpriseTeamMember(team, u.ID) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, userToJSON(u))
}

func (s *Server) handleAddEnterpriseTeamMembership(w http.ResponseWriter, r *http.Request) {
	team := s.lookupEnterpriseTeam(w, r)
	if team == nil {
		return
	}
	u := s.store.LookupUserByLogin(r.PathValue("username"))
	if u == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.store.AddEnterpriseTeamMember(team, u.ID)
	writeJSON(w, http.StatusCreated, userToJSON(u))
}

func (s *Server) handleRemoveEnterpriseTeamMembership(w http.ResponseWriter, r *http.Request) {
	team := s.lookupEnterpriseTeam(w, r)
	if team == nil {
		return
	}
	u := s.store.LookupUserByLogin(r.PathValue("username"))
	if u == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.store.RemoveEnterpriseTeamMember(team, u.ID)
	w.WriteHeader(http.StatusNoContent)
}

// --- organization assignments ---

// requireSelectedEnterpriseTeam 422s unless the team's organization selection
// type is "selected" — assignments can only be edited in that mode; "all" and
// "disabled" derive the assignment set from the selection type itself.
func requireSelectedEnterpriseTeam(w http.ResponseWriter, team *EnterpriseTeam) bool {
	if team.OrganizationSelectionType != "selected" {
		writeGHValidationError(w, "EnterpriseTeam", "organization_selection_type", "invalid")
		return false
	}
	return true
}

func (s *Server) handleListEnterpriseTeamOrgs(w http.ResponseWriter, r *http.Request) {
	team := s.lookupEnterpriseTeam(w, r)
	if team == nil {
		return
	}
	orgs := s.store.ListEnterpriseTeamOrgs(team)
	page := paginateAndLink(w, r, orgs)
	base := s.baseURL(r)
	out := make([]map[string]interface{}, 0, len(page))
	for _, o := range page {
		out = append(out, orgSimpleJSON(o, base))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleBulkAddEnterpriseTeamOrgs(w http.ResponseWriter, r *http.Request) {
	team := s.lookupEnterpriseTeam(w, r)
	if team == nil {
		return
	}
	if !requireSelectedEnterpriseTeam(w, team) {
		return
	}
	var req struct {
		OrganizationSlugs []string `json:"organization_slugs"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if len(req.OrganizationSlugs) == 0 {
		writeGHValidationError(w, "EnterpriseTeamOrganizations", "organization_slugs", "missing_field")
		return
	}
	base := s.baseURL(r)
	out := make([]map[string]interface{}, 0, len(req.OrganizationSlugs))
	for _, slug := range req.OrganizationSlugs {
		org := s.store.GetOrg(slug)
		if org == nil {
			writeGHValidationError(w, "EnterpriseTeamOrganizations", "organization_slugs", "invalid")
			return
		}
		s.store.AddEnterpriseTeamOrg(team, org.Login)
		out = append(out, orgSimpleJSON(org, base))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleBulkRemoveEnterpriseTeamOrgs(w http.ResponseWriter, r *http.Request) {
	team := s.lookupEnterpriseTeam(w, r)
	if team == nil {
		return
	}
	if !requireSelectedEnterpriseTeam(w, team) {
		return
	}
	var req struct {
		OrganizationSlugs []string `json:"organization_slugs"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if len(req.OrganizationSlugs) == 0 {
		writeGHValidationError(w, "EnterpriseTeamOrganizations", "organization_slugs", "missing_field")
		return
	}
	for _, slug := range req.OrganizationSlugs {
		if org := s.store.GetOrg(slug); org != nil {
			s.store.RemoveEnterpriseTeamOrg(team, org.Login)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGetEnterpriseTeamOrg(w http.ResponseWriter, r *http.Request) {
	team := s.lookupEnterpriseTeam(w, r)
	if team == nil {
		return
	}
	org := s.store.GetOrg(r.PathValue("org"))
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	for _, assigned := range s.store.ListEnterpriseTeamOrgs(team) {
		if assigned.ID == org.ID {
			writeJSON(w, http.StatusOK, orgSimpleJSON(org, s.baseURL(r)))
			return
		}
	}
	writeGHError(w, http.StatusNotFound, "Not Found")
}

func (s *Server) handleAddEnterpriseTeamOrg(w http.ResponseWriter, r *http.Request) {
	team := s.lookupEnterpriseTeam(w, r)
	if team == nil {
		return
	}
	org := s.store.GetOrg(r.PathValue("org"))
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if !requireSelectedEnterpriseTeam(w, team) {
		return
	}
	s.store.AddEnterpriseTeamOrg(team, org.Login)
	writeJSON(w, http.StatusCreated, orgSimpleJSON(org, s.baseURL(r)))
}

func (s *Server) handleRemoveEnterpriseTeamOrg(w http.ResponseWriter, r *http.Request) {
	team := s.lookupEnterpriseTeam(w, r)
	if team == nil {
		return
	}
	org := s.store.GetOrg(r.PathValue("org"))
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.store.RemoveEnterpriseTeamOrg(team, org.Login)
	w.WriteHeader(http.StatusNoContent)
}
