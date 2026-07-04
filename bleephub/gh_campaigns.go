package bleephub

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// GitHub security campaigns: organization-scoped remediation drives over
// linked code scanning alerts, with managers, an end date, and alert stats.

// Campaign is an organization security campaign.
type Campaign struct {
	Number             int           `json:"number"`
	OrgLogin           string        `json:"org_login"`
	Name               string        `json:"name"`
	Description        string        `json:"description"`
	ManagerLogins      []string      `json:"manager_logins"`
	TeamManagerSlugs   []string      `json:"team_manager_slugs"`
	EndsAt             time.Time     `json:"ends_at"`
	ContactLink        *string       `json:"contact_link"`
	State              string        `json:"state"`
	PublishedAt        time.Time     `json:"published_at"`
	ClosedAt           *time.Time    `json:"closed_at"`
	CreatedAt          time.Time     `json:"created_at"`
	UpdatedAt          time.Time     `json:"updated_at"`
	CodeScanningAlerts map[int][]int `json:"code_scanning_alerts"` // repo ID → alert numbers
}

func (s *Server) registerGHCampaignRoutes() {
	s.route("GET /api/v3/orgs/{org}/campaigns",
		s.requirePerm(scopeSecurityEvents, permRead, s.orgGated(s.handleListOrgCampaigns)))
	s.route("POST /api/v3/orgs/{org}/campaigns",
		s.requirePerm(scopeSecurityEvents, permWrite, s.orgGated(s.handleCreateOrgCampaign)))
	s.route("GET /api/v3/orgs/{org}/campaigns/{campaign_number}",
		s.requirePerm(scopeSecurityEvents, permRead, s.orgGated(s.handleGetOrgCampaign)))
	s.route("PATCH /api/v3/orgs/{org}/campaigns/{campaign_number}",
		s.requirePerm(scopeSecurityEvents, permWrite, s.orgGated(s.handleUpdateOrgCampaign)))
	s.route("DELETE /api/v3/orgs/{org}/campaigns/{campaign_number}",
		s.requirePerm(scopeSecurityEvents, permWrite, s.orgGated(s.handleDeleteOrgCampaign)))
}

func (s *Server) handleListOrgCampaigns(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	q := r.URL.Query()
	campaigns := s.store.ListCampaigns(org)

	if state := q.Get("state"); state != "" {
		filtered := campaigns[:0]
		for _, c := range campaigns {
			if c.State == state {
				filtered = append(filtered, c)
			}
		}
		campaigns = filtered
	}
	sortKey := q.Get("sort")
	if sortKey == "" {
		sortKey = "created"
	}
	sort.SliceStable(campaigns, func(i, j int) bool {
		a, b := campaigns[i], campaigns[j]
		switch sortKey {
		case "updated":
			return a.UpdatedAt.Before(b.UpdatedAt)
		case "ends_at":
			return a.EndsAt.Before(b.EndsAt)
		case "published":
			return a.PublishedAt.Before(b.PublishedAt)
		default:
			return a.CreatedAt.Before(b.CreatedAt)
		}
	})
	if q.Get("direction") != "asc" {
		for i, j := 0, len(campaigns)-1; i < j; i, j = i+1, j-1 {
			campaigns[i], campaigns[j] = campaigns[j], campaigns[i]
		}
	}
	campaigns = paginateAndLink(w, r, campaigns)
	out := make([]map[string]interface{}, 0, len(campaigns))
	for _, c := range campaigns {
		out = append(out, s.campaignJSON(c, r))
	}
	writeJSON(w, http.StatusOK, out)
}

type campaignAlertsRequest struct {
	RepositoryID *int  `json:"repository_id"`
	AlertNumbers []int `json:"alert_numbers"`
}

func (s *Server) handleCreateOrgCampaign(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	var req struct {
		Name               *string                 `json:"name"`
		Description        *string                 `json:"description"`
		Managers           []string                `json:"managers"`
		TeamManagers       []string                `json:"team_managers"`
		EndsAt             *time.Time              `json:"ends_at"`
		ContactLink        *string                 `json:"contact_link"`
		CodeScanningAlerts []campaignAlertsRequest `json:"code_scanning_alerts"`
		GenerateIssues     bool                    `json:"generate_issues"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Name == nil || *req.Name == "" || len(*req.Name) > 50 {
		writeGHError(w, http.StatusUnprocessableEntity, "Validation failed: name must be between 1 and 50 characters")
		return
	}
	if req.Description == nil || *req.Description == "" || len(*req.Description) > 255 {
		writeGHError(w, http.StatusUnprocessableEntity, "Validation failed: description must be between 1 and 255 characters")
		return
	}
	if req.EndsAt == nil {
		writeGHError(w, http.StatusUnprocessableEntity, "Validation failed: ends_at is required")
		return
	}
	if !req.EndsAt.After(time.Now()) {
		writeGHError(w, http.StatusUnprocessableEntity, "Validation failed: ends_at must be in the future")
		return
	}
	if len(req.CodeScanningAlerts) == 0 {
		writeGHError(w, http.StatusUnprocessableEntity, "Validation failed: code_scanning_alerts is required")
		return
	}
	if len(req.Managers) > 10 || len(req.TeamManagers) > 10 {
		writeGHError(w, http.StatusUnprocessableEntity, "Validation failed: a campaign supports at most 10 managers")
		return
	}
	for _, login := range req.Managers {
		if !isActiveOrgMember(s.store, s.store.LookupUserByLogin(login), org) {
			writeGHError(w, http.StatusUnprocessableEntity, "Validation failed: manager "+login+" is not an organization member")
			return
		}
	}
	for _, slug := range req.TeamManagers {
		if s.store.GetTeam(org, slug) == nil {
			writeGHError(w, http.StatusUnprocessableEntity, "Validation failed: team "+slug+" was not found in the organization")
			return
		}
	}
	alerts := map[int][]int{}
	for _, group := range req.CodeScanningAlerts {
		if group.RepositoryID == nil || len(group.AlertNumbers) == 0 {
			writeGHError(w, http.StatusBadRequest, "code_scanning_alerts entries require repository_id and alert_numbers")
			return
		}
		repo := s.store.GetRepoByID(*group.RepositoryID)
		if repo == nil || !s.store.RepoBelongsToOrg(repo, org) {
			writeGHError(w, http.StatusNotFound, "Not Found")
			return
		}
		for _, number := range group.AlertNumbers {
			if s.store.GetCodeScanningAlertForCampaign(repo.FullName, number) == nil {
				writeGHError(w, http.StatusNotFound, "Not Found")
				return
			}
		}
		alerts[*group.RepositoryID] = append(alerts[*group.RepositoryID], group.AlertNumbers...)
	}
	managers := req.Managers
	if len(managers) == 0 {
		if user := ghUserFromContext(r.Context()); user != nil {
			managers = []string{user.Login}
		}
	}
	c := s.store.CreateCampaign(org, *req.Name, *req.Description, managers, req.TeamManagers, *req.EndsAt, req.ContactLink, alerts)
	writeJSON(w, http.StatusOK, s.campaignJSON(c, r))
}

// resolveCampaign parses {campaign_number} and loads the campaign, writing a
// 404 on failure.
func (s *Server) resolveCampaign(w http.ResponseWriter, r *http.Request) *Campaign {
	number, err := strconv.Atoi(r.PathValue("campaign_number"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil
	}
	c := s.store.GetCampaign(r.PathValue("org"), number)
	if c == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil
	}
	return c
}

func (s *Server) handleGetOrgCampaign(w http.ResponseWriter, r *http.Request) {
	c := s.resolveCampaign(w, r)
	if c == nil {
		return
	}
	writeJSON(w, http.StatusOK, s.campaignJSON(c, r))
}

func (s *Server) handleUpdateOrgCampaign(w http.ResponseWriter, r *http.Request) {
	c := s.resolveCampaign(w, r)
	if c == nil {
		return
	}
	org := r.PathValue("org")
	var req struct {
		Name         *string    `json:"name"`
		Description  *string    `json:"description"`
		Managers     []string   `json:"managers"`
		TeamManagers []string   `json:"team_managers"`
		EndsAt       *time.Time `json:"ends_at"`
		ContactLink  *string    `json:"contact_link"`
		State        *string    `json:"state"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Name != nil && (*req.Name == "" || len(*req.Name) > 50) {
		writeGHError(w, http.StatusUnprocessableEntity, "Validation failed: name must be between 1 and 50 characters")
		return
	}
	if req.Description != nil && (*req.Description == "" || len(*req.Description) > 255) {
		writeGHError(w, http.StatusUnprocessableEntity, "Validation failed: description must be between 1 and 255 characters")
		return
	}
	if req.State != nil && *req.State != "open" && *req.State != "closed" {
		writeGHError(w, http.StatusUnprocessableEntity, "Validation failed: state must be open or closed")
		return
	}
	if len(req.Managers) > 10 || len(req.TeamManagers) > 10 {
		writeGHError(w, http.StatusUnprocessableEntity, "Validation failed: a campaign supports at most 10 managers")
		return
	}
	for _, login := range req.Managers {
		if !isActiveOrgMember(s.store, s.store.LookupUserByLogin(login), org) {
			writeGHError(w, http.StatusUnprocessableEntity, "Validation failed: manager "+login+" is not an organization member")
			return
		}
	}
	for _, slug := range req.TeamManagers {
		if s.store.GetTeam(org, slug) == nil {
			writeGHError(w, http.StatusUnprocessableEntity, "Validation failed: team "+slug+" was not found in the organization")
			return
		}
	}
	updated := s.store.UpdateCampaign(org, c.Number, func(c *Campaign) {
		if req.Name != nil {
			c.Name = *req.Name
		}
		if req.Description != nil {
			c.Description = *req.Description
		}
		if req.Managers != nil {
			c.ManagerLogins = req.Managers
		}
		if req.TeamManagers != nil {
			c.TeamManagerSlugs = req.TeamManagers
		}
		if req.EndsAt != nil {
			c.EndsAt = *req.EndsAt
		}
		if req.ContactLink != nil {
			c.ContactLink = req.ContactLink
		}
		if req.State != nil && *req.State != c.State {
			c.State = *req.State
			if c.State == "closed" {
				now := time.Now().UTC()
				c.ClosedAt = &now
			} else {
				c.ClosedAt = nil
			}
		}
	})
	writeJSON(w, http.StatusOK, s.campaignJSON(updated, r))
}

func (s *Server) handleDeleteOrgCampaign(w http.ResponseWriter, r *http.Request) {
	c := s.resolveCampaign(w, r)
	if c == nil {
		return
	}
	s.store.DeleteCampaign(r.PathValue("org"), c.Number)
	w.WriteHeader(http.StatusNoContent)
}

// campaignJSON renders the campaign-summary shape. Alert stats are derived
// live from the linked code scanning alerts' current states.
func (s *Server) campaignJSON(c *Campaign, r *http.Request) map[string]interface{} {
	base := s.baseURL(r)
	managers := make([]map[string]interface{}, 0, len(c.ManagerLogins))
	for _, login := range c.ManagerLogins {
		if u := s.store.LookupUserByLogin(login); u != nil {
			managers = append(managers, userToJSON(u))
		}
	}
	var contactLink interface{}
	if c.ContactLink != nil {
		contactLink = *c.ContactLink
	}
	var closedAt interface{}
	if c.ClosedAt != nil {
		closedAt = c.ClosedAt.Format(time.RFC3339)
	}
	open, closed := s.store.CampaignAlertCounts(c)
	out := map[string]interface{}{
		"number":       c.Number,
		"created_at":   c.CreatedAt.Format(time.RFC3339),
		"updated_at":   c.UpdatedAt.Format(time.RFC3339),
		"name":         c.Name,
		"description":  c.Description,
		"managers":     managers,
		"published_at": c.PublishedAt.Format(time.RFC3339),
		"ends_at":      c.EndsAt.Format(time.RFC3339),
		"closed_at":    closedAt,
		"state":        c.State,
		"contact_link": contactLink,
		"alert_stats": map[string]interface{}{
			"open_count":        open,
			"closed_count":      closed,
			"in_progress_count": 0,
		},
	}
	if len(c.TeamManagerSlugs) > 0 {
		org := s.store.GetOrg(c.OrgLogin)
		teams := make([]map[string]interface{}, 0, len(c.TeamManagerSlugs))
		for _, slug := range c.TeamManagerSlugs {
			if team := s.store.GetTeam(c.OrgLogin, slug); team != nil {
				teams = append(teams, teamToJSON(team, org, s.store, base))
			}
		}
		out["team_managers"] = teams
	}
	return out
}

// --- store ---

// ListCampaigns returns the org's campaigns sorted by number.
func (st *Store) ListCampaigns(orgLogin string) []*Campaign {
	st.mu.RLock()
	defer st.mu.RUnlock()
	m := st.OrgCampaigns[orgLogin]
	out := make([]*Campaign, 0, len(m))
	for _, c := range m {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Number < out[j].Number })
	return out
}

// GetCampaign returns a campaign by org and number, or nil.
func (st *Store) GetCampaign(orgLogin string, number int) *Campaign {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.OrgCampaigns[orgLogin][number]
}

// CreateCampaign creates an open campaign with the next per-org number.
func (st *Store) CreateCampaign(orgLogin, name, description string, managers, teamManagers []string, endsAt time.Time, contactLink *string, alerts map[int][]int) *Campaign {
	st.mu.Lock()
	defer st.mu.Unlock()
	number := 1
	for n := range st.OrgCampaigns[orgLogin] {
		if n >= number {
			number = n + 1
		}
	}
	now := time.Now().UTC()
	c := &Campaign{
		Number:             number,
		OrgLogin:           orgLogin,
		Name:               name,
		Description:        description,
		ManagerLogins:      managers,
		TeamManagerSlugs:   teamManagers,
		EndsAt:             endsAt.UTC(),
		ContactLink:        contactLink,
		State:              "open",
		PublishedAt:        now,
		CreatedAt:          now,
		UpdatedAt:          now,
		CodeScanningAlerts: alerts,
	}
	if st.OrgCampaigns[orgLogin] == nil {
		st.OrgCampaigns[orgLogin] = map[int]*Campaign{}
	}
	st.OrgCampaigns[orgLogin][number] = c
	if st.persist != nil {
		st.persist.MustPut("org_campaigns", orgLogin, st.OrgCampaigns[orgLogin])
	}
	return c
}

// UpdateCampaign applies fn to the campaign under the store lock.
func (st *Store) UpdateCampaign(orgLogin string, number int, fn func(*Campaign)) *Campaign {
	st.mu.Lock()
	defer st.mu.Unlock()
	c := st.OrgCampaigns[orgLogin][number]
	if c == nil {
		return nil
	}
	fn(c)
	c.UpdatedAt = time.Now().UTC()
	if st.persist != nil {
		st.persist.MustPut("org_campaigns", orgLogin, st.OrgCampaigns[orgLogin])
	}
	return c
}

// DeleteCampaign removes a campaign.
func (st *Store) DeleteCampaign(orgLogin string, number int) {
	st.mu.Lock()
	defer st.mu.Unlock()
	delete(st.OrgCampaigns[orgLogin], number)
	if st.persist != nil {
		st.persist.MustPut("org_campaigns", orgLogin, st.OrgCampaigns[orgLogin])
	}
}

// RepoBelongsToOrg reports whether the repository lives under the org's
// namespace.
func (st *Store) RepoBelongsToOrg(repo *Repo, orgLogin string) bool {
	owner, _, _ := strings.Cut(repo.FullName, "/")
	return owner == orgLogin
}

// GetCodeScanningAlertForCampaign returns the repo's code scanning alert by
// number, or nil.
func (st *Store) GetCodeScanningAlertForCampaign(repoKey string, number int) *CodeScanningAlert {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.CodeScanningAlertsByRepo[repoKey][number]
}

// CampaignAlertCounts derives open/closed counts from the current states of
// the campaign's linked code scanning alerts.
func (st *Store) CampaignAlertCounts(c *Campaign) (open, closed int) {
	st.mu.RLock()
	defer st.mu.RUnlock()
	for repoID, numbers := range c.CodeScanningAlerts {
		repo := st.Repos[repoID]
		if repo == nil {
			continue
		}
		for _, number := range numbers {
			alert := st.CodeScanningAlertsByRepo[repo.FullName][number]
			if alert == nil {
				continue
			}
			if alert.State == "open" {
				open++
			} else {
				closed++
			}
		}
	}
	return open, closed
}
