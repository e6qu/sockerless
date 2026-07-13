package bleephub

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func (s *Server) registerMgmtRoutes() {
	routes := []struct {
		pattern string
		handler http.HandlerFunc
	}{
		{"GET /internal/workflows", s.handleListWorkflows},
		{"GET /internal/workflows/{workflowId}", s.handleGetWorkflow},
		{"GET /internal/workflows/{workflowId}/logs", s.handleGetWorkflowLogs},
		{"GET /internal/workflow_files", s.handleListWorkflowFilesInternal},
		{"GET /internal/oauth/state", s.handleOAuthStateInternal},
		{"GET /internal/sessions", s.handleListSessions},
		{"GET /internal/repos", s.handleListRepos},
		{"POST /internal/agents/{agent_id}/refresh-message", s.handleAgentRefreshMessage},
		{"GET /internal/users", s.handleListUsersInternal},
		{"POST /internal/users", s.handleCreateUserInternal},
		{"GET /internal/users/{id}", s.handleGetUserInternal},
		{"PATCH /internal/users/{id}", s.handleUpdateUserInternal},
		{"DELETE /internal/users/{id}", s.handleDeleteUserInternal},
		{"GET /internal/orgs", s.handleListOrgsInternal},
		{"GET /internal/orgs/{id}", s.handleGetOrgInternal},
		{"PATCH /internal/orgs/{id}", s.handleUpdateOrgInternal},
		{"DELETE /internal/orgs/{id}", s.handleDeleteOrgInternal},
		{"GET /internal/teams", s.handleListTeamsInternal},
		{"GET /internal/teams/{id}", s.handleGetTeamInternal},
		{"PATCH /internal/teams/{id}", s.handleUpdateTeamInternal},
		{"DELETE /internal/teams/{id}", s.handleDeleteTeamInternal},
		{"GET /internal/audit-log", s.handleListAuditLogInternal},
		{"POST /internal/audit-log/events", s.handleCreateAuditLogEventInternal},
		{"POST /internal/packages/{owner_type}/{owner}/{package_type}/{package_name}/versions", s.handleInternalCreatePackageVersion},
		{"POST /internal/packages/{owner_type}/{owner}/{repo}/{package_type}/{package_name}/versions", s.handleInternalCreatePackageVersion},
	}
	for _, r := range routes {
		s.route(r.pattern, r.handler)
	}
}

type oauthStateView struct {
	DeviceCodes []deviceCodeView `json:"deviceCodes"`
	AuthCodes   []authCodeView   `json:"authCodes"`
}

type deviceCodeView struct {
	Code      string `json:"code"`
	UserCode  string `json:"userCode"`
	Scopes    string `json:"scopes"`
	UserID    int    `json:"userId"`
	ExpiresAt string `json:"expiresAt"`
}

type authCodeView struct {
	Code        string `json:"code"`
	ClientID    string `json:"clientId"`
	RedirectURI string `json:"redirectUri"`
	Scopes      string `json:"scopes"`
	State       string `json:"state"`
	UserID      int    `json:"userId"`
	CreatedAt   string `json:"createdAt"`
	ExpiresAt   string `json:"expiresAt"`
}

func (s *Server) handleOAuthStateInternal(w http.ResponseWriter, r *http.Request) {
	s.store.mu.RLock()
	dcs := make([]deviceCodeView, 0, len(s.store.DeviceCodes))
	for _, dc := range s.store.DeviceCodes {
		dcs = append(dcs, deviceCodeView{
			Code:      dc.Code,
			UserCode:  dc.UserCode,
			Scopes:    dc.Scopes,
			UserID:    dc.UserID,
			ExpiresAt: dc.ExpiresAt.Format("2006-01-02T15:04:05Z"),
		})
	}
	acs := make([]authCodeView, 0, len(s.store.AuthCodes))
	for _, ac := range s.store.AuthCodes {
		acs = append(acs, authCodeView{
			Code:        ac.Code,
			ClientID:    ac.ClientID,
			RedirectURI: ac.RedirectURI,
			Scopes:      ac.Scopes,
			State:       ac.State,
			UserID:      ac.UserID,
			CreatedAt:   ac.CreatedAt.Format("2006-01-02T15:04:05Z"),
			ExpiresAt:   ac.ExpiresAt.Format("2006-01-02T15:04:05Z"),
		})
	}
	s.store.mu.RUnlock()
	writeJSON(w, http.StatusOK, oauthStateView{DeviceCodes: dcs, AuthCodes: acs})
}

// workflowFileView is the operator-facing aggregate shape of every
// registered WorkflowFile across every repo. The bleephub UI's
// Workflows tab reads this; the per-repo GitHub-shape endpoints
// (`/api/v3/repos/{o}/{r}/actions/workflows`) are for the gh CLI +
// runner-dispatcher.
type workflowFileView struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	Path         string `json:"path"`
	State        string `json:"state"`
	RepoFullName string `json:"repoFullName"`
	Source       string `json:"source"`
	CreatedAt    string `json:"createdAt"`
	UpdatedAt    string `json:"updatedAt"`
}

func (s *Server) handleListWorkflowFilesInternal(w http.ResponseWriter, r *http.Request) {
	// Discover from every repo's git storage so files newly pushed
	// since last poll show up. Cheap — the discovery is idempotent.
	s.store.mu.RLock()
	repoNames := make([]string, 0, len(s.store.Repos))
	for _, repo := range s.store.Repos {
		repoNames = append(repoNames, repo.FullName)
	}
	s.store.mu.RUnlock()
	for _, name := range repoNames {
		s.store.DiscoverWorkflowFilesFromGit(name)
	}

	s.store.mu.RLock()
	files := make([]workflowFileView, 0, len(s.store.WorkflowFiles))
	for _, wf := range s.store.WorkflowFiles {
		files = append(files, workflowFileView{
			ID:           wf.ID,
			Name:         wf.Name,
			Path:         wf.Path,
			State:        wf.State,
			RepoFullName: wf.RepoFullName,
			Source:       wf.Source,
			CreatedAt:    wf.CreatedAt.Format("2006-01-02T15:04:05Z"),
			UpdatedAt:    wf.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		})
	}
	s.store.mu.RUnlock()

	sort.Slice(files, func(i, j int) bool {
		if files[i].RepoFullName != files[j].RepoFullName {
			return files[i].RepoFullName < files[j].RepoFullName
		}
		return files[i].Path < files[j].Path
	})

	writeJSON(w, http.StatusOK, files)
}

// workflowView is the JSON representation of a workflow for the management API.
type workflowView struct {
	ID           string                  `json:"id"`
	Name         string                  `json:"name"`
	RunID        int                     `json:"runId"`
	Status       string                  `json:"status"`
	Result       string                  `json:"result"`
	CreatedAt    string                  `json:"createdAt"`
	EventName    string                  `json:"eventName,omitempty"`
	RepoFullName string                  `json:"repoFullName,omitempty"`
	Jobs         map[string]*WorkflowJob `json:"jobs"`
}

func workflowToView(wf *Workflow) workflowView {
	return workflowView{
		ID:           wf.ID,
		Name:         wf.Name,
		RunID:        wf.RunID,
		Status:       string(wf.Status),
		Result:       string(wf.Result),
		CreatedAt:    wf.CreatedAt.Format("2006-01-02T15:04:05Z"),
		EventName:    wf.EventName,
		RepoFullName: wf.RepoFullName,
		Jobs:         wf.Jobs,
	}
}

func (s *Server) handleListWorkflows(w http.ResponseWriter, r *http.Request) {
	s.store.mu.RLock()
	workflows := make([]workflowView, 0, len(s.store.Workflows))
	for _, wf := range s.store.Workflows {
		workflows = append(workflows, workflowToView(wf))
	}
	s.store.mu.RUnlock()

	// Sort by CreatedAt descending
	sort.Slice(workflows, func(i, j int) bool {
		return workflows[i].CreatedAt > workflows[j].CreatedAt
	})

	writeJSON(w, http.StatusOK, workflows)
}

func (s *Server) handleGetWorkflow(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("workflowId")

	s.store.mu.RLock()
	wf, ok := s.store.Workflows[id]
	if !ok {
		s.store.mu.RUnlock()
		http.Error(w, "workflow not found", http.StatusNotFound)
		return
	}
	view := workflowToView(wf)
	s.store.mu.RUnlock()

	writeJSON(w, http.StatusOK, view)
}

func (s *Server) handleGetWorkflowLogs(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("workflowId")

	s.store.mu.RLock()
	_, ok := s.store.Workflows[id]
	if !ok {
		s.store.mu.RUnlock()
		http.Error(w, "workflow not found", http.StatusNotFound)
		return
	}

	// Collect log lines for all planIDs associated with this workflow's jobs
	logs := make(map[string][]string)
	for _, wf := range s.store.Workflows {
		if wf.ID != id {
			continue
		}
		for _, job := range wf.Jobs {
			if lines, exists := s.store.LogLines[job.JobID]; exists {
				logs[job.JobID] = lines
			}
		}
	}
	s.store.mu.RUnlock()

	writeJSON(w, http.StatusOK, logs)
}

// sessionView is the JSON representation of a session for the management API.
type sessionView struct {
	SessionID       string `json:"sessionId"`
	OwnerName       string `json:"ownerName"`
	Agent           *Agent `json:"agent"`
	PendingMessages int    `json:"pendingMessages"`
}

func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	s.store.mu.RLock()
	sessions := make([]sessionView, 0, len(s.store.Sessions))
	for _, sess := range s.store.Sessions {
		pending := 0
		if sess.MsgCh != nil {
			pending = len(sess.MsgCh)
		}
		sessions = append(sessions, sessionView{
			SessionID:       sess.SessionID,
			OwnerName:       sess.OwnerName,
			Agent:           sess.Agent,
			PendingMessages: pending,
		})
	}
	s.store.mu.RUnlock()

	writeJSON(w, http.StatusOK, sessions)
}

// repoView is the JSON representation of a repo for the management API.
type repoView struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	FullName      string `json:"full_name"`
	Description   string `json:"description"`
	DefaultBranch string `json:"default_branch"`
	Visibility    string `json:"visibility"`
	Private       bool   `json:"private"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

func (s *Server) handleListRepos(w http.ResponseWriter, r *http.Request) {
	s.store.mu.RLock()
	repos := make([]repoView, 0, len(s.store.Repos))
	for _, repo := range s.store.Repos {
		repos = append(repos, repoView{
			ID:            repo.ID,
			Name:          repo.Name,
			FullName:      repo.FullName,
			Description:   repo.Description,
			DefaultBranch: repo.DefaultBranch,
			Visibility:    repo.Visibility,
			Private:       repo.Private,
			CreatedAt:     repo.CreatedAt.Format("2006-01-02T15:04:05Z"),
			UpdatedAt:     repo.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		})
	}
	s.store.mu.RUnlock()

	writeJSON(w, http.StatusOK, repos)
}

// handleAgentRefreshMessage — POST /internal/agents/{agent_id}/refresh-message
//
// Sim-control endpoint that tells bleephub to deliver an AgentRefreshMessage
// to the named agent's open session(s). Real GitHub pushes this message when
// a newer runner package is available; bleephub has no update feed, so the
// operator/test harness triggers it explicitly. Requires site-admin token.
func (s *Server) handleAgentRefreshMessage(w http.ResponseWriter, r *http.Request) {
	caller := ghUserFromContext(r.Context())
	if caller == nil || !caller.SiteAdmin {
		writeGHError(w, http.StatusForbidden, "Must be a site administrator.")
		return
	}

	agentID, err := strconv.Atoi(r.PathValue("agent_id"))
	if err != nil || agentID == 0 {
		writeGHError(w, http.StatusBadRequest, "invalid agent id")
		return
	}

	var req struct {
		TargetVersion string `json:"targetVersion"`
		Timeout       string `json:"timeout"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeGHError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.TargetVersion == "" {
		writeGHValidationError(w, "AgentRefreshMessage", "targetVersion", "missing_field")
		return
	}

	timeout := 5 * time.Minute
	if req.Timeout != "" {
		if d, err := time.ParseDuration(req.Timeout); err == nil {
			timeout = d
		}
	}

	s.store.mu.RLock()
	_, exists := s.store.Agents[agentID]
	s.store.mu.RUnlock()
	if !exists {
		writeGHError(w, http.StatusNotFound, "agent not found")
		return
	}

	s.sendAgentRefreshMessage(agentID, req.TargetVersion, timeout)
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) requireSiteAdmin(w http.ResponseWriter, r *http.Request) *User {
	user := ghUserFromContext(r.Context())
	if user == nil || !user.SiteAdmin {
		writeGHError(w, http.StatusForbidden, "Must be a site administrator.")
		return nil
	}
	return user
}

func (s *Server) handleListUsersInternal(w http.ResponseWriter, r *http.Request) {
	if s.requireSiteAdmin(w, r) == nil {
		return
	}
	s.store.mu.RLock()
	users := make([]*User, 0, len(s.store.Users))
	for _, u := range s.store.Users {
		users = append(users, u)
	}
	s.store.mu.RUnlock()
	sort.Slice(users, func(i, j int) bool { return users[i].ID < users[j].ID })

	out := make([]map[string]interface{}, len(users))
	for i, u := range users {
		out[i] = map[string]interface{}{
			"id":         u.ID,
			"login":      u.Login,
			"name":       u.Name,
			"email":      u.Email,
			"type":       u.Type,
			"site_admin": u.SiteAdmin,
			"created_at": u.CreatedAt.Format(time.RFC3339),
			"updated_at": u.UpdatedAt.Format(time.RFC3339),
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCreateUserInternal(w http.ResponseWriter, r *http.Request) {
	if s.requireSiteAdmin(w, r) == nil {
		return
	}
	var req struct {
		Login     string `json:"login"`
		Name      string `json:"name"`
		Email     string `json:"email"`
		Password  string `json:"password"`
		SiteAdmin *bool  `json:"site_admin"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Login == "" {
		writeGHValidationError(w, "User", "login", "missing_field")
		return
	}
	passwordHash := ""
	if req.Password != "" {
		var err error
		encoded, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			writeGHError(w, http.StatusInternalServerError, "could not secure local user password")
			return
		}
		passwordHash = string(encoded)
	}

	s.store.mu.Lock()
	if _, exists := s.store.UsersByLogin[req.Login]; exists {
		s.store.mu.Unlock()
		writeGHValidationError(w, "User", "login", "already_exists")
		return
	}

	now := time.Now().UTC()
	siteAdmin := false
	if req.SiteAdmin != nil {
		siteAdmin = *req.SiteAdmin
	}
	u := &User{
		ID:           s.store.NextUser,
		NodeID:       fmt.Sprintf("U_kgDO%08d", s.store.NextUser),
		Login:        req.Login,
		Name:         req.Name,
		Email:        req.Email,
		AvatarURL:    "",
		Bio:          "",
		Type:         "User",
		SiteAdmin:    siteAdmin,
		StarredRepos: map[string]bool{},
		CreatedAt:    now,
		UpdatedAt:    now,
		PasswordHash: passwordHash,
	}
	s.store.NextUser++
	s.store.Users[u.ID] = u
	s.store.UsersByLogin[u.Login] = u
	if s.store.persist != nil {
		s.store.persist.MustPut("users", strconv.Itoa(u.ID), u)
	}
	s.store.mu.Unlock()

	writeJSON(w, http.StatusCreated, s.fullUserJSON(u))
}

func (s *Server) handleGetUserInternal(w http.ResponseWriter, r *http.Request) {
	if s.requireSiteAdmin(w, r) == nil {
		return
	}
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id == 0 {
		writeGHError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	u := s.store.GetUserByID(id)
	if u == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, s.fullUserJSON(u))
}

func (s *Server) handleUpdateUserInternal(w http.ResponseWriter, r *http.Request) {
	if s.requireSiteAdmin(w, r) == nil {
		return
	}
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id == 0 {
		writeGHError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	var req struct {
		Name      *string `json:"name"`
		Email     *string `json:"email"`
		SiteAdmin *bool   `json:"site_admin"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}

	s.store.mu.Lock()
	u := s.store.Users[id]
	if u == nil {
		s.store.mu.Unlock()
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if req.Name != nil {
		u.Name = *req.Name
	}
	if req.Email != nil {
		u.Email = *req.Email
	}
	if req.SiteAdmin != nil {
		u.SiteAdmin = *req.SiteAdmin
	}
	u.UpdatedAt = time.Now().UTC()
	if s.store.persist != nil {
		s.store.persist.MustPut("users", strconv.Itoa(u.ID), u)
	}
	s.store.mu.Unlock()

	writeJSON(w, http.StatusOK, s.fullUserJSON(u))
}

func (s *Server) handleDeleteUserInternal(w http.ResponseWriter, r *http.Request) {
	if s.requireSiteAdmin(w, r) == nil {
		return
	}
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id == 0 {
		writeGHError(w, http.StatusBadRequest, "invalid user id")
		return
	}

	s.store.mu.Lock()
	u := s.store.Users[id]
	if u == nil {
		s.store.mu.Unlock()
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	delete(s.store.Users, u.ID)
	delete(s.store.UsersByLogin, u.Login)
	for val, t := range s.store.Tokens {
		if t.UserID == u.ID {
			delete(s.store.Tokens, val)
			if s.store.persist != nil {
				s.store.persist.MustDelete("tokens", val)
			}
		}
	}
	for cookie, sess := range s.store.LoginSessions {
		if sess.UserID == u.ID {
			delete(s.store.LoginSessions, cookie)
		}
	}
	if s.store.persist != nil {
		s.store.persist.MustDelete("users", strconv.Itoa(u.ID))
	}
	s.store.mu.Unlock()

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListOrgsInternal(w http.ResponseWriter, r *http.Request) {
	if s.requireSiteAdmin(w, r) == nil {
		return
	}
	s.store.mu.RLock()
	orgs := make([]*Org, 0, len(s.store.Orgs))
	for _, org := range s.store.Orgs {
		orgs = append(orgs, org)
	}
	s.store.mu.RUnlock()
	sort.Slice(orgs, func(i, j int) bool { return orgs[i].ID < orgs[j].ID })

	base := s.baseURL(r)
	out := make([]map[string]interface{}, len(orgs))
	for i, org := range orgs {
		out[i] = orgSimpleJSON(org, base)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGetOrgInternal(w http.ResponseWriter, r *http.Request) {
	if s.requireSiteAdmin(w, r) == nil {
		return
	}
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id == 0 {
		writeGHError(w, http.StatusBadRequest, "invalid org id")
		return
	}
	org := s.store.GetOrgByID(id)
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, orgToJSON(org, s.store, s.baseURL(r)))
}

func (s *Server) handleUpdateOrgInternal(w http.ResponseWriter, r *http.Request) {
	if s.requireSiteAdmin(w, r) == nil {
		return
	}
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id == 0 {
		writeGHError(w, http.StatusBadRequest, "invalid org id")
		return
	}
	var req struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}

	s.store.mu.Lock()
	org := s.store.Orgs[id]
	if org == nil {
		s.store.mu.Unlock()
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if req.Name != nil {
		org.Name = *req.Name
	}
	if req.Description != nil {
		org.Description = *req.Description
	}
	org.UpdatedAt = time.Now().UTC()
	if s.store.persist != nil {
		s.store.persist.MustPut("orgs", strconv.Itoa(org.ID), org)
	}
	s.store.mu.Unlock()

	writeJSON(w, http.StatusOK, orgToJSON(org, s.store, s.baseURL(r)))
}

func (s *Server) handleDeleteOrgInternal(w http.ResponseWriter, r *http.Request) {
	if s.requireSiteAdmin(w, r) == nil {
		return
	}
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id == 0 {
		writeGHError(w, http.StatusBadRequest, "invalid org id")
		return
	}
	org := s.store.GetOrgByID(id)
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	s.store.mu.RLock()
	var repoNames []string
	prefix := org.Login + "/"
	for fullName := range s.store.ReposByName {
		if strings.HasPrefix(fullName, prefix) {
			_, name, _ := strings.Cut(fullName, "/")
			repoNames = append(repoNames, name)
		}
	}
	s.store.mu.RUnlock()

	for _, name := range repoNames {
		if _, err := s.store.DeleteRepo(org.Login, name); err != nil {
			writeGHError(w, http.StatusInternalServerError, "repository delete failed: "+err.Error())
			return
		}
	}
	s.store.DeleteOrg(org.Login)

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListTeamsInternal(w http.ResponseWriter, r *http.Request) {
	if s.requireSiteAdmin(w, r) == nil {
		return
	}
	s.store.mu.RLock()
	teams := make([]*Team, 0, len(s.store.Teams))
	for _, team := range s.store.Teams {
		teams = append(teams, team)
	}
	s.store.mu.RUnlock()
	sort.Slice(teams, func(i, j int) bool { return teams[i].ID < teams[j].ID })

	out := make([]map[string]interface{}, 0, len(teams))
	for _, team := range teams {
		s.store.mu.RLock()
		org := s.store.Orgs[team.OrgID]
		s.store.mu.RUnlock()
		if org == nil {
			continue
		}
		out = append(out, map[string]interface{}{
			"id":          team.ID,
			"org":         org.Login,
			"slug":        team.Slug,
			"name":        team.Name,
			"description": team.Description,
			"privacy":     team.Privacy,
			"created_at":  team.CreatedAt.Format(time.RFC3339),
			"updated_at":  team.UpdatedAt.Format(time.RFC3339),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGetTeamInternal(w http.ResponseWriter, r *http.Request) {
	if s.requireSiteAdmin(w, r) == nil {
		return
	}
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id == 0 {
		writeGHError(w, http.StatusBadRequest, "invalid team id")
		return
	}
	team := s.store.GetTeamByID(id)
	if team == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	org := s.store.GetOrgByID(team.OrgID)
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, teamToJSON(team, org, s.store, s.baseURL(r)))
}

func (s *Server) handleUpdateTeamInternal(w http.ResponseWriter, r *http.Request) {
	if s.requireSiteAdmin(w, r) == nil {
		return
	}
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id == 0 {
		writeGHError(w, http.StatusBadRequest, "invalid team id")
		return
	}
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Privacy     string `json:"privacy"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}

	team := s.store.GetTeamByID(id)
	if team == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	org := s.store.GetOrgByID(team.OrgID)
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	ok := s.store.UpdateTeam(org.Login, team.Slug, func(t *Team) {
		if req.Name != "" {
			t.Name = req.Name
			t.Slug = slugify(req.Name)
		}
		if req.Description != "" {
			t.Description = req.Description
		}
		if req.Privacy != "" {
			t.Privacy = TeamPrivacy(req.Privacy)
		}
	})
	if !ok {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	updated := s.store.GetTeamByID(id)
	writeJSON(w, http.StatusOK, teamToJSON(updated, org, s.store, s.baseURL(r)))
}

func (s *Server) handleDeleteTeamInternal(w http.ResponseWriter, r *http.Request) {
	if s.requireSiteAdmin(w, r) == nil {
		return
	}
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id == 0 {
		writeGHError(w, http.StatusBadRequest, "invalid team id")
		return
	}
	team := s.store.GetTeamByID(id)
	if team == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	org := s.store.GetOrgByID(team.OrgID)
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.store.DeleteTeam(org.Login, team.Slug)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleCreateAuditLogEventInternal(w http.ResponseWriter, r *http.Request) {
	if s.requireSiteAdmin(w, r) == nil {
		return
	}
	var req struct {
		Actor      string                 `json:"actor"`
		Action     string                 `json:"action"`
		TargetType string                 `json:"target_type"`
		TargetID   string                 `json:"target_id"`
		Org        string                 `json:"org"`
		Details    map[string]interface{} `json:"details"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Action == "" {
		writeGHValidationError(w, "AuditLogEvent", "action", "missing_field")
		return
	}
	e := s.recordInternalAuditEvent(req.Actor, req.Action, req.TargetType, req.TargetID, req.Org, req.Details)
	writeJSON(w, http.StatusCreated, e)
}

func (s *Server) handleListAuditLogInternal(w http.ResponseWriter, r *http.Request) {
	if s.requireSiteAdmin(w, r) == nil {
		return
	}
	q := r.URL.Query()
	orgFilter := q.Get("org")
	actorFilter := q.Get("actor")
	actionFilter := q.Get("action")

	var fromTime, toTime time.Time
	if v := q.Get("from"); v != "" {
		fromTime, _ = time.Parse(time.RFC3339, v)
	}
	if v := q.Get("to"); v != "" {
		toTime, _ = time.Parse(time.RFC3339, v)
	}

	s.store.Misc.mu.RLock()
	entries := make([]*AuditLogEvent, 0, len(s.store.Misc.auditLogEvents))
	for _, e := range s.store.Misc.auditLogEvents {
		if orgFilter != "" && e.Org != orgFilter {
			continue
		}
		if actorFilter != "" && e.Actor != actorFilter {
			continue
		}
		if actionFilter != "" && e.Action != actionFilter {
			continue
		}
		if !fromTime.IsZero() && e.createdAt.Before(fromTime) {
			continue
		}
		if !toTime.IsZero() && e.createdAt.After(toTime) {
			continue
		}
		entries = append(entries, e)
	}
	s.store.Misc.mu.RUnlock()

	writeJSON(w, http.StatusOK, entries)
}

func (s *Server) recordInternalAuditEvent(actor, action, targetType, targetID, org string, details map[string]interface{}) *AuditLogEvent {
	s.store.Misc.mu.Lock()
	defer s.store.Misc.mu.Unlock()
	s.store.Misc.nextAdminAuditID++
	now := time.Now().UTC()
	e := &AuditLogEvent{
		ID:         s.store.Misc.nextAdminAuditID,
		Timestamp:  now.Format(time.RFC3339Nano),
		Actor:      actor,
		Action:     action,
		TargetType: targetType,
		TargetID:   targetID,
		Org:        org,
		Details:    details,
		createdAt:  now,
	}
	s.store.Misc.auditLogEvents = append([]*AuditLogEvent{e}, s.store.Misc.auditLogEvents...)
	if len(s.store.Misc.auditLogEvents) > maxAuditLogEntries {
		s.store.Misc.auditLogEvents = s.store.Misc.auditLogEvents[:maxAuditLogEntries]
	}
	if s.store.Misc.persist != nil {
		s.store.Misc.persist.MustPut("admin_audit_log", strconv.FormatInt(e.ID, 10), e)
	}
	return e
}
