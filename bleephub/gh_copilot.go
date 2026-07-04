package bleephub

// GitHub Copilot organization REST surface: seat billing (selected
// users / selected teams), seat listing and per-member seat details,
// usage metrics, content exclusion, Copilot coding agent permissions,
// and the repository-scoped Copilot cloud agent configuration.

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

func (s *Server) registerGHCopilotRoutes() {
	s.route("GET /api/v3/orgs/{org}/copilot/billing", s.handleGetCopilotOrganizationDetails)
	s.route("GET /api/v3/orgs/{org}/copilot/billing/seats", s.handleListCopilotSeats)
	s.route("POST /api/v3/orgs/{org}/copilot/billing/selected_users", s.handleAddCopilotSeatsForUsers)
	s.route("DELETE /api/v3/orgs/{org}/copilot/billing/selected_users", s.handleCancelCopilotSeatsForUsers)
	s.route("POST /api/v3/orgs/{org}/copilot/billing/selected_teams", s.handleAddCopilotSeatsForTeams)
	s.route("DELETE /api/v3/orgs/{org}/copilot/billing/selected_teams", s.handleCancelCopilotSeatsForTeams)
	s.route("GET /api/v3/orgs/{org}/members/{username}/copilot", s.handleGetCopilotSeatDetailsForUser)
	s.route("GET /api/v3/orgs/{org}/copilot/metrics", s.handleCopilotMetricsForOrganization)
	s.route("GET /api/v3/orgs/{org}/team/{team_slug}/copilot/metrics", s.handleCopilotMetricsForTeam)
	s.route("GET /api/v3/orgs/{org}/copilot/metrics/reports/organization-1-day", s.handleCopilotOneDayReport)
	s.route("GET /api/v3/orgs/{org}/copilot/metrics/reports/users-1-day", s.handleCopilotOneDayReport)
	s.route("GET /api/v3/orgs/{org}/copilot/metrics/reports/user-teams-1-day", s.handleCopilotOneDayReport)
	s.route("GET /api/v3/orgs/{org}/copilot/metrics/reports/organization-28-day/latest", s.handleCopilotLatest28DayReport)
	s.route("GET /api/v3/orgs/{org}/copilot/metrics/reports/users-28-day/latest", s.handleCopilotLatest28DayReport)
	s.route("GET /api/v3/orgs/{org}/copilot/content_exclusion", s.handleGetCopilotContentExclusion)
	s.route("PUT /api/v3/orgs/{org}/copilot/content_exclusion", s.handleSetCopilotContentExclusion)
	s.route("GET /api/v3/orgs/{org}/copilot/coding-agent/permissions", s.handleGetCopilotCodingAgentPermissions)
	s.route("PUT /api/v3/orgs/{org}/copilot/coding-agent/permissions", s.handleSetCopilotCodingAgentPermissions)
	s.route("GET /api/v3/orgs/{org}/copilot/coding-agent/permissions/repositories", s.handleListCopilotCodingAgentRepos)
	s.route("PUT /api/v3/orgs/{org}/copilot/coding-agent/permissions/repositories", s.handleSetCopilotCodingAgentRepos)
	s.route("PUT /api/v3/orgs/{org}/copilot/coding-agent/permissions/repositories/{repository_id}", s.handleEnableCopilotCodingAgentRepo)
	s.route("DELETE /api/v3/orgs/{org}/copilot/coding-agent/permissions/repositories/{repository_id}", s.handleDisableCopilotCodingAgentRepo)
	s.route("GET /api/v3/repos/{owner}/{repo}/copilot/cloud-agent/configuration", s.handleGetCopilotCloudAgentConfiguration)
}

// copilotOrgAdmin resolves the {org} path parameter and enforces the
// caller is an authenticated organization owner — the audience real
// GitHub grants the Copilot billing, metrics, and policy surface to.
// Writes 401/404/403 and returns nil when the gate fails.
func (s *Server) copilotOrgAdmin(w http.ResponseWriter, r *http.Request) *Org {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return nil
	}
	org := s.store.GetOrg(r.PathValue("org"))
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil
	}
	if !canAdminOrg(s.store, user, org) {
		writeGHError(w, http.StatusForbidden, "Must be an organization owner.")
		return nil
	}
	return org
}

// copilotSeatJSON renders one seat in the copilot-seat-details shape.
func (s *Server) copilotSeatJSON(seat *CopilotSeat, org *Org, baseURL string) map[string]interface{} {
	var assignee interface{}
	if u := s.store.GetUserByID(seat.UserID); u != nil {
		assignee = userToJSON(u)
	}
	var assigningTeam interface{}
	if seat.AssigningTeamSlug != "" {
		if team := s.store.GetTeam(org.Login, seat.AssigningTeamSlug); team != nil {
			assigningTeam = teamSimpleJSON(team, org, s.store, baseURL)
		}
	}
	var pendingCancellation interface{}
	if seat.PendingCancellationDate != "" {
		pendingCancellation = seat.PendingCancellationDate
	}
	return map[string]interface{}{
		"assignee":                  assignee,
		"organization":              orgSimpleJSON(org, baseURL),
		"assigning_team":            assigningTeam,
		"pending_cancellation_date": pendingCancellation,
		// bleephub records no Copilot editor telemetry, so the activity
		// members are honestly null rather than fabricated timestamps.
		"last_activity_at":     nil,
		"last_activity_editor": nil,
		"created_at":           seat.CreatedAt.Format(time.RFC3339),
		"updated_at":           seat.UpdatedAt.Format(time.RFC3339),
		"plan_type":            "business",
	}
}

func (s *Server) handleGetCopilotOrganizationDetails(w http.ResponseWriter, r *http.Request) {
	org := s.copilotOrgAdmin(w, r)
	if org == nil {
		return
	}
	seats := s.store.ListCopilotSeats(org.Login)
	now := time.Now().UTC()
	total := len(seats)
	pendingCancellation := 0
	addedThisCycle := 0
	pendingInvitation := 0
	for _, seat := range seats {
		if seat.PendingCancellationDate != "" {
			pendingCancellation++
		}
		if seat.CreatedAt.Year() == now.Year() && seat.CreatedAt.Month() == now.Month() {
			addedThisCycle++
		}
		if m := s.store.GetMembership(org.Login, seat.UserID); m != nil && m.State == MembershipStatePending {
			pendingInvitation++
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"seat_breakdown": map[string]interface{}{
			"total":                total,
			"added_this_cycle":     addedThisCycle,
			"pending_cancellation": pendingCancellation,
			"pending_invitation":   pendingInvitation,
			// No Copilot usage telemetry is recorded, so no seat has been
			// "active this cycle"; every billed seat is honestly inactive.
			"active_this_cycle":   0,
			"inactive_this_cycle": total,
		},
		// Every bleephub organization is provisioned with Copilot Business
		// and permissive feature policies: real GitHub configures these
		// through the organization settings UI only (no REST write surface),
		// and the seat-management endpoints below require a configured
		// subscription with per-user seat assignment to be usable at all.
		"public_code_suggestions": "allow",
		"ide_chat":                "enabled",
		"platform_chat":           "enabled",
		"cli":                     "enabled",
		"seat_management_setting": "assign_selected",
		"plan_type":               "business",
	})
}

func (s *Server) handleListCopilotSeats(w http.ResponseWriter, r *http.Request) {
	org := s.copilotOrgAdmin(w, r)
	if org == nil {
		return
	}
	seats := s.store.ListCopilotSeats(org.Login)
	total := len(seats)
	page := paginateAndLink(w, r, seats)
	base := s.baseURL(r)
	out := make([]map[string]interface{}, 0, len(page))
	for _, seat := range page {
		out = append(out, s.copilotSeatJSON(seat, org, base))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"total_seats": total,
		"seats":       out,
	})
}

// resolveCopilotSeatUsers maps usernames to active organization members,
// writing a 422 and returning nil when any username does not resolve —
// GitHub validates the whole batch before assigning any seat.
func (s *Server) resolveCopilotSeatUsers(w http.ResponseWriter, org *Org, usernames []string) []int {
	if len(usernames) == 0 {
		writeGHValidationError(w, "CopilotSeat", "selected_usernames", "missing_field")
		return nil
	}
	ids := make([]int, 0, len(usernames))
	var invalid []string
	for _, login := range usernames {
		u := s.store.LookupUserByLogin(login)
		if u == nil {
			invalid = append(invalid, login)
			continue
		}
		m := s.store.GetMembership(org.Login, u.ID)
		if m == nil || m.State != MembershipStateActive {
			invalid = append(invalid, login)
			continue
		}
		ids = append(ids, u.ID)
	}
	if len(invalid) > 0 {
		writeGHError(w, http.StatusUnprocessableEntity,
			"Copilot seats cannot be managed for users that are not active members of this organization: "+strings.Join(invalid, ", "))
		return nil
	}
	return ids
}

func (s *Server) handleAddCopilotSeatsForUsers(w http.ResponseWriter, r *http.Request) {
	org := s.copilotOrgAdmin(w, r)
	if org == nil {
		return
	}
	var req struct {
		SelectedUsernames []string `json:"selected_usernames"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	ids := s.resolveCopilotSeatUsers(w, org, req.SelectedUsernames)
	if ids == nil {
		return
	}
	created := s.store.AddCopilotSeats(org.Login, ids, "")
	writeJSON(w, http.StatusCreated, map[string]interface{}{"seats_created": created})
}

func (s *Server) handleCancelCopilotSeatsForUsers(w http.ResponseWriter, r *http.Request) {
	org := s.copilotOrgAdmin(w, r)
	if org == nil {
		return
	}
	var req struct {
		SelectedUsernames []string `json:"selected_usernames"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	ids := s.resolveCopilotSeatUsers(w, org, req.SelectedUsernames)
	if ids == nil {
		return
	}
	cancelled, teamAssigned := s.store.CancelCopilotSeatsForUsers(org.Login, ids)
	if len(teamAssigned) > 0 {
		logins := make([]string, 0, len(teamAssigned))
		for _, id := range teamAssigned {
			if u := s.store.GetUserByID(id); u != nil {
				logins = append(logins, u.Login)
			}
		}
		writeGHError(w, http.StatusUnprocessableEntity,
			"Copilot seats assigned via a team cannot be cancelled individually: "+strings.Join(logins, ", "))
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"seats_cancelled": cancelled})
}

// resolveCopilotSeatTeams maps team names (or slugs) to teams of the
// organization, writing a 422 and returning nil when any does not resolve.
func (s *Server) resolveCopilotSeatTeams(w http.ResponseWriter, org *Org, names []string) []*Team {
	if len(names) == 0 {
		writeGHValidationError(w, "CopilotSeat", "selected_teams", "missing_field")
		return nil
	}
	teams := make([]*Team, 0, len(names))
	var invalid []string
	for _, name := range names {
		team := s.store.GetTeam(org.Login, name)
		if team == nil {
			team = s.store.GetTeam(org.Login, slugify(name))
		}
		if team == nil {
			invalid = append(invalid, name)
			continue
		}
		teams = append(teams, team)
	}
	if len(invalid) > 0 {
		writeGHError(w, http.StatusUnprocessableEntity,
			"Copilot seats cannot be managed for teams that do not belong to this organization: "+strings.Join(invalid, ", "))
		return nil
	}
	return teams
}

func (s *Server) handleAddCopilotSeatsForTeams(w http.ResponseWriter, r *http.Request) {
	org := s.copilotOrgAdmin(w, r)
	if org == nil {
		return
	}
	var req struct {
		SelectedTeams []string `json:"selected_teams"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	teams := s.resolveCopilotSeatTeams(w, org, req.SelectedTeams)
	if teams == nil {
		return
	}
	created := 0
	for _, team := range teams {
		members := s.store.ListTeamMembers(org.Login, team.Slug)
		ids := make([]int, 0, len(members))
		for _, m := range members {
			ids = append(ids, m.ID)
		}
		created += s.store.AddCopilotSeats(org.Login, ids, team.Slug)
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"seats_created": created})
}

func (s *Server) handleCancelCopilotSeatsForTeams(w http.ResponseWriter, r *http.Request) {
	org := s.copilotOrgAdmin(w, r)
	if org == nil {
		return
	}
	var req struct {
		SelectedTeams []string `json:"selected_teams"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	teams := s.resolveCopilotSeatTeams(w, org, req.SelectedTeams)
	if teams == nil {
		return
	}
	cancelled := 0
	for _, team := range teams {
		cancelled += s.store.CancelCopilotSeatsForTeam(org.Login, team.Slug)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"seats_cancelled": cancelled})
}

func (s *Server) handleGetCopilotSeatDetailsForUser(w http.ResponseWriter, r *http.Request) {
	caller := ghUserFromContext(r.Context())
	if caller == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	org := s.store.GetOrg(r.PathValue("org"))
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	username := r.PathValue("username")
	if !canAdminOrg(s.store, caller, org) && !strings.EqualFold(caller.Login, username) {
		writeGHError(w, http.StatusForbidden, "Must be an organization owner.")
		return
	}
	user := s.store.LookupUserByLogin(username)
	if user == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	m := s.store.GetMembership(org.Login, user.ID)
	if m == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if m.State == MembershipStatePending {
		writeGHError(w, http.StatusUnprocessableEntity, "User has a pending organization invitation.")
		return
	}
	seat := s.store.GetCopilotSeat(org.Login, user.ID)
	if seat == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, s.copilotSeatJSON(seat, org, s.baseURL(r)))
}

// copilotMetricsWindow validates the optional since/until query
// parameters. Returns false after writing a 422 on a malformed value.
func copilotMetricsWindow(w http.ResponseWriter, r *http.Request) bool {
	for _, name := range []string{"since", "until"} {
		if v := r.URL.Query().Get(name); v != "" {
			if _, err := time.Parse(time.RFC3339, v); err != nil {
				writeGHError(w, http.StatusUnprocessableEntity,
					fmt.Sprintf("Invalid %s parameter. Expected an ISO 8601 timestamp.", name))
				return false
			}
		}
	}
	return true
}

func (s *Server) handleCopilotMetricsForOrganization(w http.ResponseWriter, r *http.Request) {
	org := s.copilotOrgAdmin(w, r)
	if org == nil {
		return
	}
	if !copilotMetricsWindow(w, r) {
		return
	}
	// bleephub records no Copilot editor or chat telemetry, so there are
	// no days with aggregated usage: the documented response with no
	// activity is an empty array, never fabricated numbers.
	writeJSON(w, http.StatusOK, []map[string]interface{}{})
}

func (s *Server) handleCopilotMetricsForTeam(w http.ResponseWriter, r *http.Request) {
	org := s.copilotOrgAdmin(w, r)
	if org == nil {
		return
	}
	if s.store.GetTeam(org.Login, r.PathValue("team_slug")) == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if !copilotMetricsWindow(w, r) {
		return
	}
	writeJSON(w, http.StatusOK, []map[string]interface{}{})
}

// handleCopilotOneDayReport serves the three organization single-day
// report endpoints. The day query parameter is required; with no Copilot
// activity ever recorded no daily report is generated, so a valid
// request gets the documented 204 no-report response.
func (s *Server) handleCopilotOneDayReport(w http.ResponseWriter, r *http.Request) {
	org := s.copilotOrgAdmin(w, r)
	if org == nil {
		return
	}
	day := r.URL.Query().Get("day")
	if day == "" {
		writeGHValidationError(w, "CopilotMetricsReport", "day", "missing_field")
		return
	}
	if _, err := time.Parse("2006-01-02", day); err != nil {
		writeGHValidationError(w, "CopilotMetricsReport", "day", "invalid")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleCopilotLatest28DayReport serves the two latest-28-day report
// endpoints. The report period is the latest complete 28-day window
// (ending yesterday, UTC); with no Copilot activity recorded there is
// nothing to download, so download_links is honestly empty.
func (s *Server) handleCopilotLatest28DayReport(w http.ResponseWriter, r *http.Request) {
	org := s.copilotOrgAdmin(w, r)
	if org == nil {
		return
	}
	end := time.Now().UTC().AddDate(0, 0, -1)
	start := end.AddDate(0, 0, -27)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"download_links":   []string{},
		"report_start_day": start.Format("2006-01-02"),
		"report_end_day":   end.Format("2006-01-02"),
	})
}

func (s *Server) handleGetCopilotContentExclusion(w http.ResponseWriter, r *http.Request) {
	org := s.copilotOrgAdmin(w, r)
	if org == nil {
		return
	}
	writeJSON(w, http.StatusOK, s.store.GetCopilotContentExclusion(org.Login))
}

func (s *Server) handleSetCopilotContentExclusion(w http.ResponseWriter, r *http.Request) {
	org := s.copilotOrgAdmin(w, r)
	if org == nil {
		return
	}
	var rules map[string][]interface{}
	if !decodeJSONBody(w, r, &rules) {
		return
	}
	if rules == nil {
		rules = map[string][]interface{}{}
	}
	for scope, entries := range rules {
		for _, entry := range entries {
			if !validContentExclusionRule(entry) {
				writeGHError(w, http.StatusUnprocessableEntity,
					fmt.Sprintf("Invalid content exclusion rule for %q: each rule must be a path string or an object with exactly one of ifAnyMatch / ifNoneMatch.", scope))
				return
			}
		}
	}
	s.store.SetCopilotContentExclusion(org.Login, rules)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Content exclusion settings updated.",
	})
}

// validContentExclusionRule accepts the documented rule forms: a path
// string, or an object with exactly one of ifAnyMatch / ifNoneMatch
// holding a list of strings.
func validContentExclusionRule(entry interface{}) bool {
	switch v := entry.(type) {
	case string:
		return true
	case map[string]interface{}:
		if len(v) != 1 {
			return false
		}
		for key, val := range v {
			if key != "ifAnyMatch" && key != "ifNoneMatch" {
				return false
			}
			items, ok := val.([]interface{})
			if !ok {
				return false
			}
			for _, item := range items {
				if _, ok := item.(string); !ok {
					return false
				}
			}
		}
		return true
	default:
		return false
	}
}

func (s *Server) handleGetCopilotCodingAgentPermissions(w http.ResponseWriter, r *http.Request) {
	org := s.copilotOrgAdmin(w, r)
	if org == nil {
		return
	}
	p := s.store.GetCopilotCodingAgentPermissions(org.Login)
	out := map[string]interface{}{"enabled_repositories": p.EnabledRepositories}
	if p.EnabledRepositories == "selected" {
		out["selected_repositories_url"] = s.baseURL(r) + "/api/v3/orgs/" + org.Login + "/copilot/coding-agent/permissions/repositories"
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleSetCopilotCodingAgentPermissions(w http.ResponseWriter, r *http.Request) {
	org := s.copilotOrgAdmin(w, r)
	if org == nil {
		return
	}
	var req struct {
		EnabledRepositories string `json:"enabled_repositories"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	switch req.EnabledRepositories {
	case "all", "selected", "none":
	case "":
		writeGHValidationError(w, "CopilotCodingAgentPermissions", "enabled_repositories", "missing_field")
		return
	default:
		writeGHValidationError(w, "CopilotCodingAgentPermissions", "enabled_repositories", "invalid")
		return
	}
	s.store.SetCopilotCodingAgentPolicy(org.Login, req.EnabledRepositories)
	w.WriteHeader(http.StatusNoContent)
}

// copilotCodingAgentSelectedGate enforces the 409 the selected-repository
// sub-resource returns when the organization policy is not "selected".
func (s *Server) copilotCodingAgentSelectedGate(w http.ResponseWriter, org *Org) bool {
	p := s.store.GetCopilotCodingAgentPermissions(org.Login)
	if p.EnabledRepositories != "selected" {
		writeGHError(w, http.StatusConflict,
			"The organization's Copilot coding agent policy is not set to selected repositories.")
		return false
	}
	return true
}

func (s *Server) handleListCopilotCodingAgentRepos(w http.ResponseWriter, r *http.Request) {
	org := s.copilotOrgAdmin(w, r)
	if org == nil {
		return
	}
	if !s.copilotCodingAgentSelectedGate(w, org) {
		return
	}
	p := s.store.GetCopilotCodingAgentPermissions(org.Login)
	ids := make([]int, len(p.SelectedRepositoryIDs))
	copy(ids, p.SelectedRepositoryIDs)
	sort.Ints(ids)
	base := s.baseURL(r)
	repos := make([]map[string]interface{}, 0, len(ids))
	for _, id := range ids {
		s.store.mu.RLock()
		repo := s.store.Repos[id]
		s.store.mu.RUnlock()
		if repo != nil {
			repos = append(repos, repoToJSON(repo, s.store, base))
		}
	}
	total := len(repos)
	repos = paginateAndLink(w, r, repos)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"total_count":  total,
		"repositories": repos,
	})
}

// copilotOrgRepoIDs validates every ID references an existing repository
// owned by the organization, writing a 422 and returning false otherwise.
func (s *Server) copilotOrgRepoIDs(w http.ResponseWriter, org *Org, ids []int) bool {
	var invalid []string
	for _, id := range ids {
		s.store.mu.RLock()
		repo := s.store.Repos[id]
		s.store.mu.RUnlock()
		if repo == nil || !strings.HasPrefix(repo.FullName, org.Login+"/") {
			invalid = append(invalid, strconv.Itoa(id))
		}
	}
	if len(invalid) > 0 {
		writeGHError(w, http.StatusUnprocessableEntity,
			"The following repository IDs do not belong to this organization: "+strings.Join(invalid, ", "))
		return false
	}
	return true
}

func (s *Server) handleSetCopilotCodingAgentRepos(w http.ResponseWriter, r *http.Request) {
	org := s.copilotOrgAdmin(w, r)
	if org == nil {
		return
	}
	if !s.copilotCodingAgentSelectedGate(w, org) {
		return
	}
	var req struct {
		SelectedRepositoryIDs *[]int `json:"selected_repository_ids"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.SelectedRepositoryIDs == nil {
		writeGHValidationError(w, "CopilotCodingAgentPermissions", "selected_repository_ids", "missing_field")
		return
	}
	if !s.copilotOrgRepoIDs(w, org, *req.SelectedRepositoryIDs) {
		return
	}
	s.store.SetCopilotCodingAgentSelectedRepos(org.Login, *req.SelectedRepositoryIDs)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleEnableCopilotCodingAgentRepo(w http.ResponseWriter, r *http.Request) {
	org := s.copilotOrgAdmin(w, r)
	if org == nil {
		return
	}
	if !s.copilotCodingAgentSelectedGate(w, org) {
		return
	}
	id, err := strconv.Atoi(r.PathValue("repository_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.store.mu.RLock()
	repo := s.store.Repos[id]
	s.store.mu.RUnlock()
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if !s.copilotOrgRepoIDs(w, org, []int{id}) {
		return
	}
	s.store.AddCopilotCodingAgentSelectedRepo(org.Login, id)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDisableCopilotCodingAgentRepo(w http.ResponseWriter, r *http.Request) {
	org := s.copilotOrgAdmin(w, r)
	if org == nil {
		return
	}
	if !s.copilotCodingAgentSelectedGate(w, org) {
		return
	}
	id, err := strconv.Atoi(r.PathValue("repository_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.store.mu.RLock()
	repo := s.store.Repos[id]
	s.store.mu.RUnlock()
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.store.RemoveCopilotCodingAgentSelectedRepo(org.Login, id)
	w.WriteHeader(http.StatusNoContent)
}

// handleGetCopilotCloudAgentConfiguration serves the repository's Copilot
// cloud agent configuration. Real GitHub manages this configuration
// through the repository settings UI only — the REST surface is
// read-only — so every repository reports GitHub's defaults: firewall on
// with the recommended allowlist, Actions workflow approval required,
// the full review-tool suite enabled, and no MCP configuration.
func (s *Server) handleGetCopilotCloudAgentConfiguration(w http.ResponseWriter, r *http.Request) {
	if ghUserFromContext(r.Context()) == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"mcp_configuration": nil,
		"enabled_tools": map[string]interface{}{
			"codeql":                          true,
			"copilot_code_review":             true,
			"secret_scanning":                 true,
			"dependency_vulnerability_checks": true,
		},
		"require_actions_workflow_approval":         true,
		"is_firewall_enabled":                       true,
		"is_firewall_recommended_allowlist_enabled": true,
		"custom_allowlist":                          []string{},
	})
}
