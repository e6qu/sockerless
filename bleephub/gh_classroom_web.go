package bleephub

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
)

// registerGHClassroomWebRoutes exposes the authenticated write and transition
// surface that GitHub Classroom historically served from classroom.github.com.
// These are deliberately outside /api/v3: GitHub's public Classroom REST API
// is read-only, while the browser product owns classroom management and
// assignment acceptance.
func (s *Server) registerGHClassroomWebRoutes() {
	s.route("GET /a/{invite_code}", s.classroomLocked(s.handleClassroomInviteRedirect))
	s.route("GET /classroom-data", s.classroomWebAuthenticated(s.handleClassroomDashboard))
	s.route("POST /classroom-data/classrooms", s.classroomWebAuthenticated(s.handleCreateClassroom))
	s.route("PATCH /classroom-data/classrooms/{classroom_id}", s.classroomWebAuthenticated(s.handleUpdateClassroom))
	s.route("DELETE /classroom-data/classrooms/{classroom_id}", s.classroomWebAuthenticated(s.handleDeleteClassroom))
	s.route("PUT /classroom-data/classrooms/{classroom_id}/roster", s.classroomWebAuthenticated(s.handleReplaceClassroomRoster))
	s.route("POST /classroom-data/classrooms/{classroom_id}/assignments", s.classroomWebAuthenticated(s.handleCreateClassroomAssignment))
	s.route("PATCH /classroom-data/assignments/{assignment_id}", s.classroomWebAuthenticated(s.handleUpdateClassroomAssignment))
	s.route("DELETE /classroom-data/assignments/{assignment_id}", s.classroomWebAuthenticated(s.handleDeleteClassroomAssignment))
	s.route("GET /classroom-data/invitations/{invite_code}", s.classroomWebAuthenticated(s.handleGetClassroomInvitation))
	s.route("POST /classroom-data/invitations/{invite_code}/accept", s.classroomWebAuthenticated(s.handleAcceptClassroomInvitation))
	s.route("GET /classroom-data/export", s.classroomWebAuthenticated(s.handleExportClassrooms))
	s.route("POST /classroom-data/import", s.classroomWebAuthenticated(s.handleImportClassrooms))
}

func (s *Server) handleClassroomInviteRedirect(w http.ResponseWriter, r *http.Request) {
	a := s.assignmentByInvite(r.PathValue("invite_code"))
	if a == nil || !a.InvitationsEnabled || s.classroomArchived(a.ClassroomID) {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, "/ui/classrooms/accept/"+a.InviteCode, http.StatusFound)
}

func (s *Server) classroomWebAuthenticated(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := s.authenticateRequest(r)
		if ghUserFromContext(ctx) == nil {
			writeGHError(w, http.StatusUnauthorized, "Requires authentication")
			return
		}
		if suspended, _ := ctx.Value(ctxSuspendedUser).(bool); suspended {
			writeGHError(w, http.StatusForbidden, "This account has been suspended")
			return
		}
		s.classroomMu.Lock()
		defer s.classroomMu.Unlock()
		next(w, r.WithContext(ctx))
	}
}

func (s *Server) handleClassroomDashboard(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	classrooms := make([]map[string]interface{}, 0)
	for _, c := range s.classroomsAdministeredBy(user) {
		classrooms = append(classrooms, s.classroomWebJSON(c, s.baseURL(r)))
	}
	orgs := make([]map[string]interface{}, 0)
	for _, org := range s.store.ListOrgsAll(0) {
		if user.SiteAdmin || canAdminOrg(s.store, user, org) {
			orgs = append(orgs, map[string]interface{}{"id": org.ID, "login": org.Login, "name": org.Name, "avatar_url": org.AvatarURL})
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"classrooms": classrooms, "organizations": orgs})
}

func (s *Server) classroomsAdministeredBy(user *User) []*Classroom {
	s.store.mu.RLock()
	all := make([]*Classroom, 0, len(s.store.Classrooms))
	for _, c := range s.store.Classrooms {
		all = append(all, c)
	}
	s.store.mu.RUnlock()
	sortClassrooms(all)
	out := all[:0]
	for _, c := range all {
		org := s.store.GetOrgByID(c.OrgID)
		if org != nil && (user.SiteAdmin || canAdminOrg(s.store, user, org)) {
			out = append(out, c)
		}
	}
	return out
}

func sortClassrooms(classrooms []*Classroom) {
	for i := 1; i < len(classrooms); i++ {
		for j := i; j > 0 && classrooms[j-1].ID > classrooms[j].ID; j-- {
			classrooms[j-1], classrooms[j] = classrooms[j], classrooms[j-1]
		}
	}
}

func (s *Server) classroomWebJSON(c *Classroom, base string) map[string]interface{} {
	out := s.classroomJSON(c, base)
	if out == nil {
		return map[string]interface{}{"id": c.ID, "name": c.Name, "archived": c.Archived}
	}
	roster := make([]map[string]interface{}, 0, len(c.Roster))
	for _, entry := range c.Roster {
		item := map[string]interface{}{"id": 0, "login": "", "avatar_url": "", "roster_identifier": entry.RosterIdentifier}
		if user := s.store.GetUserByID(entry.UserID); user != nil {
			item["id"], item["login"], item["avatar_url"] = user.ID, user.Login, user.AvatarURL
		}
		roster = append(roster, item)
	}
	assignments := make([]map[string]interface{}, 0)
	s.store.mu.RLock()
	classroomAssignments := make([]*ClassroomAssignment, 0)
	for _, assignment := range s.store.ClassroomAssignments {
		if assignment.ClassroomID == c.ID {
			classroomAssignments = append(classroomAssignments, assignment)
		}
	}
	s.store.mu.RUnlock()
	for _, assignment := range classroomAssignments {
		item := s.classroomAssignmentJSON(assignment, base, true)
		item["autograding_tests"] = assignment.AutogradingTests
		assignments = append(assignments, item)
	}
	out["roster"] = roster
	out["assignments"] = assignments
	return out
}

func (s *Server) handleCreateClassroom(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	var req struct {
		Name string `json:"name"`
		Org  string `json:"organization"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeGHValidationError(w, "Classroom", "name", "missing_field")
		return
	}
	org := s.store.GetOrg(req.Org)
	if org == nil || (!user.SiteAdmin && !canAdminOrg(s.store, user, org)) {
		writeGHError(w, http.StatusForbidden, "You must administer the organization to create its classroom.")
		return
	}
	c := s.store.CreateClassroom(strings.TrimSpace(req.Name), org.ID, false)
	writeJSON(w, http.StatusCreated, s.classroomWebJSON(c, s.baseURL(r)))
}

func (s *Server) handleUpdateClassroom(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.PathValue("classroom_id"))
	if s.classroomForAdmin(w, r, id) == nil {
		return
	}
	var req struct {
		Name     *string `json:"name"`
		Archived *bool   `json:"archived"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	c := s.store.UpdateClassroom(id, func(c *Classroom) {
		if req.Name != nil && strings.TrimSpace(*req.Name) != "" {
			c.Name = strings.TrimSpace(*req.Name)
		}
		if req.Archived != nil {
			c.Archived = *req.Archived
		}
	})
	writeJSON(w, http.StatusOK, s.classroomWebJSON(c, s.baseURL(r)))
}

func (s *Server) handleDeleteClassroom(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.PathValue("classroom_id"))
	if s.classroomForAdmin(w, r, id) == nil {
		return
	}
	if !s.store.DeleteClassroom(id) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleReplaceClassroomRoster(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.PathValue("classroom_id"))
	if s.classroomForAdmin(w, r, id) == nil {
		return
	}
	var req struct {
		Students []struct {
			Login      string `json:"login"`
			Identifier string `json:"roster_identifier"`
		} `json:"students"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	roster := make([]ClassroomStudent, 0, len(req.Students))
	seenUsers := map[int]bool{}
	seenIdentifiers := map[string]bool{}
	for _, entry := range req.Students {
		identifier := strings.TrimSpace(entry.Identifier)
		var user *User
		if strings.TrimSpace(entry.Login) != "" {
			user = s.store.LookupUserByLogin(entry.Login)
		}
		if strings.TrimSpace(entry.Login) != "" && user == nil {
			writeGHError(w, http.StatusUnprocessableEntity, "Student not found: "+entry.Login)
			return
		}
		userID := 0
		if user != nil {
			userID = user.ID
		}
		if identifier == "" || (userID != 0 && seenUsers[userID]) || seenIdentifiers[identifier] {
			writeGHValidationError(w, "ClassroomRoster", "students", "invalid")
			return
		}
		if userID != 0 {
			seenUsers[userID] = true
		}
		seenIdentifiers[identifier] = true
		roster = append(roster, ClassroomStudent{UserID: userID, RosterIdentifier: identifier})
	}
	c := s.store.UpdateClassroom(id, func(c *Classroom) { c.Roster = roster })
	writeJSON(w, http.StatusOK, s.classroomWebJSON(c, s.baseURL(r)))
}

type classroomAssignmentRequest struct {
	Title                       *string                    `json:"title"`
	Type                        *string                    `json:"type"`
	StarterCodeRepository       *string                    `json:"starter_code_repository"`
	PublicRepo                  *bool                      `json:"public_repo"`
	InvitationsEnabled          *bool                      `json:"invitations_enabled"`
	StudentsAreRepoAdmins       *bool                      `json:"students_are_repo_admins"`
	FeedbackPullRequestsEnabled *bool                      `json:"feedback_pull_requests_enabled"`
	MaxTeams                    *int                       `json:"max_teams"`
	MaxMembers                  *int                       `json:"max_members"`
	Editor                      *string                    `json:"editor"`
	Language                    *string                    `json:"language"`
	Deadline                    *time.Time                 `json:"deadline"`
	AutogradingTests            []ClassroomAutogradingTest `json:"autograding_tests"`
}

func (s *Server) handleCreateClassroomAssignment(w http.ResponseWriter, r *http.Request) {
	classroomID, _ := strconv.Atoi(r.PathValue("classroom_id"))
	classroom := s.classroomForAdmin(w, r, classroomID)
	if classroom == nil {
		return
	}
	if classroom.Archived {
		writeGHError(w, http.StatusUnprocessableEntity, "Archived classrooms cannot create assignments.")
		return
	}
	var req classroomAssignmentRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Title == nil || strings.TrimSpace(*req.Title) == "" || req.Type == nil || (*req.Type != "individual" && *req.Type != "group") || req.StarterCodeRepository == nil {
		writeGHValidationError(w, "ClassroomAssignment", "assignment", "invalid")
		return
	}
	if !validAutogradingTests(req.AutogradingTests) {
		writeGHValidationError(w, "ClassroomAssignment", "autograding_tests", "invalid")
		return
	}
	owner, name, found := strings.Cut(*req.StarterCodeRepository, "/")
	starter := s.store.GetRepo(owner, name)
	if !found || starter == nil || !canReadRepo(s.store, ghUserFromContext(r.Context()), starter) {
		writeGHError(w, http.StatusUnprocessableEntity, "Starter code repository not found")
		return
	}
	invite, err := newInviteCodeE()
	if err != nil {
		writeGHError(w, http.StatusInternalServerError, err.Error())
		return
	}
	a := &ClassroomAssignment{ClassroomID: classroom.ID, Title: strings.TrimSpace(*req.Title), Type: *req.Type, Slug: slugify(*req.Title), InviteCode: invite, InvitationsEnabled: true, StarterCodeRepoID: starter.ID}
	applyClassroomAssignmentRequest(a, &req)
	a = s.store.CreateClassroomAssignment(a)
	out := s.classroomAssignmentJSON(a, s.baseURL(r), true)
	out["autograding_tests"] = a.AutogradingTests
	writeJSON(w, http.StatusCreated, out)
}

func applyClassroomAssignmentRequest(a *ClassroomAssignment, req *classroomAssignmentRequest) {
	if req.Title != nil && strings.TrimSpace(*req.Title) != "" {
		a.Title, a.Slug = strings.TrimSpace(*req.Title), slugify(*req.Title)
	}
	if req.Type != nil {
		a.Type = *req.Type
	}
	if req.PublicRepo != nil {
		a.PublicRepo = *req.PublicRepo
	}
	if req.InvitationsEnabled != nil {
		a.InvitationsEnabled = *req.InvitationsEnabled
	}
	if req.StudentsAreRepoAdmins != nil {
		a.StudentsAreRepoAdmins = *req.StudentsAreRepoAdmins
	}
	if req.FeedbackPullRequestsEnabled != nil {
		a.FeedbackPullRequestsEnabled = *req.FeedbackPullRequestsEnabled
	}
	if req.MaxTeams != nil {
		a.MaxTeams = req.MaxTeams
	}
	if req.MaxMembers != nil {
		a.MaxMembers = req.MaxMembers
	}
	if req.Editor != nil {
		a.Editor = *req.Editor
	}
	if req.Language != nil {
		a.Language = *req.Language
	}
	if req.Deadline != nil {
		a.Deadline = req.Deadline
	}
	if req.AutogradingTests != nil {
		a.AutogradingTests = append([]ClassroomAutogradingTest(nil), req.AutogradingTests...)
	}
}

func validAutogradingTests(tests []ClassroomAutogradingTest) bool {
	for _, test := range tests {
		if strings.TrimSpace(test.Name) == "" || strings.TrimSpace(test.Command) == "" || test.Points <= 0 {
			return false
		}
	}
	return true
}

func (s *Server) handleUpdateClassroomAssignment(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.PathValue("assignment_id"))
	existing := s.classroomAssignmentForAdmin(w, r, id)
	if existing == nil {
		return
	}
	if classroom := s.store.getClassroom(existing.ClassroomID); classroom != nil && classroom.Archived {
		writeGHError(w, http.StatusUnprocessableEntity, "Archived classrooms cannot edit assignments.")
		return
	}
	var req classroomAssignmentRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Type != nil && *req.Type != "individual" && *req.Type != "group" {
		writeGHValidationError(w, "ClassroomAssignment", "type", "invalid")
		return
	}
	if !validAutogradingTests(req.AutogradingTests) {
		writeGHValidationError(w, "ClassroomAssignment", "autograding_tests", "invalid")
		return
	}
	a := s.store.UpdateClassroomAssignment(id, func(a *ClassroomAssignment) { applyClassroomAssignmentRequest(a, &req) })
	out := s.classroomAssignmentJSON(a, s.baseURL(r), true)
	out["autograding_tests"] = a.AutogradingTests
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleDeleteClassroomAssignment(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.PathValue("assignment_id"))
	if s.classroomAssignmentForAdmin(w, r, id) == nil {
		return
	}
	if !s.store.DeleteClassroomAssignment(id) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) assignmentByInvite(code string) *ClassroomAssignment {
	s.store.mu.RLock()
	defer s.store.mu.RUnlock()
	for _, a := range s.store.ClassroomAssignments {
		if a.InviteCode == code {
			return a
		}
	}
	return nil
}

func (s *Server) handleGetClassroomInvitation(w http.ResponseWriter, r *http.Request) {
	if ghUserFromContext(r.Context()) == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	a := s.assignmentByInvite(r.PathValue("invite_code"))
	if a == nil || !a.InvitationsEnabled || s.classroomArchived(a.ClassroomID) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	out := s.classroomAssignmentJSON(a, s.baseURL(r), true)
	out["autograding_tests"] = a.AutogradingTests
	user := ghUserFromContext(r.Context())
	classroom := s.store.getClassroom(a.ClassroomID)
	out["roster_identifier_required"] = classroomRosterIdentifier(classroom, user.ID) == "" && len(classroom.Roster) > 0
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleAcceptClassroomInvitation(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	a := s.assignmentByInvite(r.PathValue("invite_code"))
	if a == nil || !a.InvitationsEnabled || s.classroomArchived(a.ClassroomID) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	var req struct {
		GroupName        string `json:"group_name"`
		RosterIdentifier string `json:"roster_identifier"`
	}
	if r.ContentLength != 0 && !decodeJSONBody(w, r, &req) {
		return
	}
	req.GroupName = strings.TrimSpace(req.GroupName)
	if a.Type == "group" && req.GroupName == "" {
		writeGHValidationError(w, "ClassroomAssignment", "group_name", "missing_field")
		return
	}
	c := s.store.getClassroom(a.ClassroomID)
	existingAcceptances := s.store.classroomAcceptedFor(a.ID)
	var groupAcceptance *ClassroomAcceptedAssignment
	for _, existing := range existingAcceptances {
		for _, student := range existing.Students {
			if student.UserID == user.ID {
				repo := s.store.GetRepoByID(existing.RepoID)
				if repo == nil {
					writeGHError(w, http.StatusInternalServerError, "Classroom acceptance references a missing repository")
					return
				}
				writeJSON(w, http.StatusOK, map[string]interface{}{"id": existing.ID, "repository": simpleClassroomRepositoryJSON(repo, s.baseURL(r))})
				return
			}
		}
		if a.Type == "group" && strings.EqualFold(existing.GroupName, req.GroupName) {
			groupAcceptance = existing
		}
	}
	if groupAcceptance != nil && a.MaxMembers != nil && len(groupAcceptance.Students) >= *a.MaxMembers {
		writeGHValidationError(w, "ClassroomAssignment", "group_name", "team_full")
		return
	}
	if a.Type == "group" && groupAcceptance == nil && a.MaxTeams != nil && len(existingAcceptances) >= *a.MaxTeams {
		writeGHValidationError(w, "ClassroomAssignment", "group_name", "maximum_teams_reached")
		return
	}
	rosterID, err := s.claimClassroomRosterIdentifier(c, user.ID, strings.TrimSpace(req.RosterIdentifier))
	if err != nil {
		writeGHValidationError(w, "ClassroomRoster", "roster_identifier", err.Error())
		return
	}
	if groupAcceptance != nil {
		repo := s.store.GetRepoByID(groupAcceptance.RepoID)
		if repo == nil {
			writeGHError(w, http.StatusInternalServerError, "Classroom acceptance references a missing repository")
			return
		}
		permission := "push"
		if a.StudentsAreRepoAdmins {
			permission = "admin"
		}
		owner, _, _ := splitRepoFullName(repo.FullName)
		if !s.store.AddRepoCollaborator(owner, repo.Name, user.Login, permission) {
			writeGHError(w, http.StatusInternalServerError, "Could not grant student repository access")
			return
		}
		s.store.UpdateClassroomAcceptedAssignment(groupAcceptance.ID, func(accepted *ClassroomAcceptedAssignment) {
			accepted.Students = append(accepted.Students, ClassroomStudent{UserID: user.ID, RosterIdentifier: rosterID})
		})
		writeJSON(w, http.StatusOK, map[string]interface{}{"id": groupAcceptance.ID, "repository": simpleClassroomRepositoryJSON(repo, s.baseURL(r))})
		return
	}
	org := s.store.GetOrgByID(c.OrgID)
	starter := s.store.GetRepoByID(a.StarterCodeRepoID)
	if org == nil || starter == nil {
		writeGHError(w, http.StatusInternalServerError, "Classroom assignment references missing durable state")
		return
	}
	repoSuffix := user.Login
	if a.Type == "group" {
		repoSuffix = req.GroupName
	}
	repoName := a.Slug + "-" + slugify(repoSuffix)
	repo := s.store.CreateOrgRepo(org, user, repoName, "GitHub Classroom assignment: "+a.Title, !a.PublicRepo)
	if repo == nil {
		writeGHValidationError(w, "Repository", "name", "already_exists")
		return
	}
	rollback := func(cause error) {
		if _, err := s.store.DeleteRepo(org.Login, repo.Name); err != nil {
			writeGHError(w, http.StatusInternalServerError, fmt.Sprintf("%v; repository rollback failed: %v", cause, err))
			return
		}
		writeGHError(w, http.StatusUnprocessableEntity, cause.Error())
	}
	starterOwner, _, _ := splitRepoFullName(starter.FullName)
	sig := repoSignature(coalesceStr(user.Name, user.Login), coalesceStr(user.Email, user.Login+"@bleephub.local"))
	if err := generateFromTemplateStorage(s.store.GetGitStorage(starterOwner, starter.Name), s.store.GetGitStorage(org.Login, repo.Name), starter.DefaultBranch, false, sig); err != nil {
		rollback(fmt.Errorf("could not generate assignment repository: %w", err))
		return
	}
	s.store.UpdateRepo(org.Login, repo.Name, func(rp *Repo) {
		rp.DefaultBranch = starter.DefaultBranch
		rp.TemplateRepoID = starter.ID
		rp.PushedAt = time.Now().UTC()
	})
	if len(a.AutogradingTests) > 0 {
		if _, err := createFileCommit(
			s.store.GetGitStorage(org.Login, repo.Name),
			repo.DefaultBranch,
			".github/workflows/classroom.yml",
			classroomAutogradingWorkflow(a.AutogradingTests),
			"Configure GitHub Classroom autograding",
			sig,
		); err != nil {
			rollback(fmt.Errorf("could not configure autograding workflow: %w", err))
			return
		}
	}
	permission := "push"
	if a.StudentsAreRepoAdmins {
		permission = "admin"
	}
	if !s.store.AddRepoCollaborator(org.Login, repo.Name, user.Login, permission) {
		rollback(fmt.Errorf("could not grant student repository access"))
		return
	}
	if a.FeedbackPullRequestsEnabled {
		stor := s.store.GetGitStorage(org.Login, repo.Name)
		baseRef, err := stor.Reference(plumbing.NewBranchReferenceName(repo.DefaultBranch))
		if err != nil {
			rollback(fmt.Errorf("could not create feedback pull request: %w", err))
			return
		}
		if err := stor.SetReference(plumbing.NewHashReference(plumbing.NewBranchReferenceName("feedback"), baseRef.Hash())); err != nil {
			rollback(fmt.Errorf("could not create feedback branch: %w", err))
			return
		}
		s.store.CreatePullRequest(repo.ID, user.ID, "Feedback", "Use this pull request for instructor feedback.", "feedback", repo.DefaultBranch, false, nil, nil, 0)
	}
	baselineRef, err := s.store.GetGitStorage(org.Login, repo.Name).Reference(plumbing.NewBranchReferenceName(repo.DefaultBranch))
	if err != nil {
		rollback(fmt.Errorf("could not resolve generated assignment branch: %w", err))
		return
	}
	aa := s.store.CreateClassroomAcceptedAssignment(&ClassroomAcceptedAssignment{
		AssignmentID: a.ID, Students: []ClassroomStudent{{UserID: user.ID, RosterIdentifier: rosterID}}, RepoID: repo.ID, GroupName: req.GroupName, AcceptedAt: time.Now().UTC(), BaselineSHA: baselineRef.Hash().String(),
	})
	writeJSON(w, http.StatusCreated, map[string]interface{}{"id": aa.ID, "repository": simpleClassroomRepositoryJSON(s.store.GetRepoByID(repo.ID), s.baseURL(r))})
}

func classroomRosterIdentifier(classroom *Classroom, userID int) string {
	if classroom == nil {
		return ""
	}
	for _, entry := range classroom.Roster {
		if entry.UserID == userID {
			return entry.RosterIdentifier
		}
	}
	return ""
}

func (s *Server) classroomArchived(id int) bool {
	c := s.store.getClassroom(id)
	return c == nil || c.Archived
}

func (s *Server) claimClassroomRosterIdentifier(classroom *Classroom, userID int, requested string) (string, error) {
	if classroom == nil {
		return "", fmt.Errorf("classroom_missing")
	}
	if linked := classroomRosterIdentifier(classroom, userID); linked != "" {
		return linked, nil
	}
	if len(classroom.Roster) == 0 {
		return "", nil
	}
	if requested == "" {
		return "", fmt.Errorf("missing_field")
	}
	claimed := false
	conflict := false
	s.store.UpdateClassroom(classroom.ID, func(current *Classroom) {
		for index := range current.Roster {
			entry := &current.Roster[index]
			if entry.RosterIdentifier != requested {
				continue
			}
			if entry.UserID != 0 && entry.UserID != userID {
				conflict = true
				return
			}
			entry.UserID = userID
			claimed = true
			return
		}
	})
	if conflict {
		return "", fmt.Errorf("already_claimed")
	}
	if !claimed {
		return "", fmt.Errorf("invalid")
	}
	return requested, nil
}

func classroomAutogradingWorkflow(tests []ClassroomAutogradingTest) string {
	var body strings.Builder
	body.WriteString("name: GitHub Classroom Workflow\non:\n  push:\n  workflow_dispatch:\njobs:\n")
	for index, test := range tests {
		fmt.Fprintf(&body, "  autograding-%d:\n    name: %s\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@v4\n      - name: Run autograding test\n        run: %s\n", index+1, strconv.Quote(test.Name), strconv.Quote(test.Command))
	}
	return body.String()
}

type classroomTransitionBundle struct {
	Format     string                      `json:"format"`
	ExportedAt time.Time                   `json:"exported_at"`
	Classrooms []classroomTransitionCourse `json:"classrooms"`
}

type classroomTransitionCourse struct {
	Name         string                          `json:"name"`
	Archived     bool                            `json:"archived"`
	Organization string                          `json:"organization"`
	Roster       []classroomTransitionStudent    `json:"roster"`
	Assignments  []classroomTransitionAssignment `json:"assignments"`
}

type classroomTransitionStudent struct {
	Login      string `json:"login,omitempty"`
	Identifier string `json:"roster_identifier"`
}

type classroomTransitionAssignment struct {
	Title                       string                        `json:"title"`
	Type                        string                        `json:"type"`
	StarterCodeRepository       string                        `json:"starter_code_repository"`
	PublicRepo                  bool                          `json:"public_repo"`
	InvitationsEnabled          bool                          `json:"invitations_enabled"`
	StudentsAreRepoAdmins       bool                          `json:"students_are_repo_admins"`
	FeedbackPullRequestsEnabled bool                          `json:"feedback_pull_requests_enabled"`
	MaxTeams                    *int                          `json:"max_teams,omitempty"`
	MaxMembers                  *int                          `json:"max_members,omitempty"`
	Editor                      string                        `json:"editor,omitempty"`
	Language                    string                        `json:"language,omitempty"`
	Deadline                    *time.Time                    `json:"deadline,omitempty"`
	AutogradingTests            []ClassroomAutogradingTest    `json:"autograding_tests,omitempty"`
	AcceptedRepositories        []classroomTransitionAccepted `json:"accepted_repositories,omitempty"`
}

type classroomTransitionAccepted struct {
	Repository  string                       `json:"repository"`
	GroupName   string                       `json:"group_name,omitempty"`
	Students    []classroomTransitionStudent `json:"students"`
	AcceptedAt  time.Time                    `json:"accepted_at"`
	BaselineSHA string                       `json:"baseline_sha,omitempty"`
}

func (s *Server) handleExportClassrooms(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	bundle := classroomTransitionBundle{Format: "bleephub-classroom-transition-v1", ExportedAt: time.Now().UTC()}
	for _, classroom := range s.classroomsAdministeredBy(user) {
		org := s.store.GetOrgByID(classroom.OrgID)
		course := classroomTransitionCourse{Name: classroom.Name, Archived: classroom.Archived, Organization: org.Login}
		for _, roster := range classroom.Roster {
			login := ""
			if rosterUser := s.store.GetUserByID(roster.UserID); rosterUser != nil {
				login = rosterUser.Login
			}
			course.Roster = append(course.Roster, classroomTransitionStudent{Login: login, Identifier: roster.RosterIdentifier})
		}
		s.store.mu.RLock()
		assignments := make([]*ClassroomAssignment, 0)
		for _, assignment := range s.store.ClassroomAssignments {
			if assignment.ClassroomID == classroom.ID {
				assignments = append(assignments, assignment)
			}
		}
		s.store.mu.RUnlock()
		for _, assignment := range assignments {
			starter := s.store.GetRepoByID(assignment.StarterCodeRepoID)
			if starter == nil {
				writeGHError(w, http.StatusInternalServerError, "Classroom assignment references a missing starter repository")
				return
			}
			item := classroomTransitionAssignment{
				Title: assignment.Title, Type: assignment.Type, StarterCodeRepository: starter.FullName,
				PublicRepo: assignment.PublicRepo, InvitationsEnabled: assignment.InvitationsEnabled,
				StudentsAreRepoAdmins: assignment.StudentsAreRepoAdmins, FeedbackPullRequestsEnabled: assignment.FeedbackPullRequestsEnabled,
				MaxTeams: assignment.MaxTeams, MaxMembers: assignment.MaxMembers, Editor: assignment.Editor, Language: assignment.Language,
				Deadline: assignment.Deadline, AutogradingTests: assignment.AutogradingTests,
			}
			for _, accepted := range s.store.classroomAcceptedFor(assignment.ID) {
				repo := s.store.GetRepoByID(accepted.RepoID)
				if repo == nil {
					writeGHError(w, http.StatusInternalServerError, "Classroom acceptance references a missing repository")
					return
				}
				acceptance := classroomTransitionAccepted{Repository: repo.FullName, GroupName: accepted.GroupName, AcceptedAt: classroomAcceptedAt(accepted), BaselineSHA: accepted.BaselineSHA}
				for _, student := range accepted.Students {
					login := ""
					if acceptedUser := s.store.GetUserByID(student.UserID); acceptedUser != nil {
						login = acceptedUser.Login
					}
					acceptance.Students = append(acceptance.Students, classroomTransitionStudent{Login: login, Identifier: student.RosterIdentifier})
				}
				item.AcceptedRepositories = append(item.AcceptedRepositories, acceptance)
			}
			course.Assignments = append(course.Assignments, item)
		}
		bundle.Classrooms = append(bundle.Classrooms, course)
	}
	w.Header().Set("Content-Disposition", `attachment; filename="bleephub-classroom-transition.json"`)
	writeJSON(w, http.StatusOK, bundle)
}

func (s *Server) handleImportClassrooms(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	var bundle classroomTransitionBundle
	if !decodeJSONBody(w, r, &bundle) {
		return
	}
	if bundle.Format != "bleephub-classroom-transition-v1" || len(bundle.Classrooms) == 0 {
		writeGHValidationError(w, "ClassroomTransition", "format", "invalid")
		return
	}
	inviteCodes := make([][]string, len(bundle.Classrooms))
	for courseIndex, course := range bundle.Classrooms {
		org := s.store.GetOrg(course.Organization)
		if org == nil || (!user.SiteAdmin && !canAdminOrg(s.store, user, org)) {
			writeGHError(w, http.StatusForbidden, "You must create and administer every destination organization before importing Classroom data.")
			return
		}
		if strings.TrimSpace(course.Name) == "" {
			writeGHValidationError(w, "ClassroomTransition", "name", "invalid")
			return
		}
		seenRoster := map[string]bool{}
		for _, roster := range course.Roster {
			if roster.Identifier == "" || seenRoster[roster.Identifier] {
				writeGHValidationError(w, "ClassroomTransition", "roster", "invalid")
				return
			}
			seenRoster[roster.Identifier] = true
			if roster.Login != "" && s.store.LookupUserByLogin(roster.Login) == nil {
				writeGHError(w, http.StatusUnprocessableEntity, "Import references a missing user: "+roster.Login)
				return
			}
		}
		inviteCodes[courseIndex] = make([]string, len(course.Assignments))
		for assignmentIndex, assignment := range course.Assignments {
			if assignment.Type != "individual" && assignment.Type != "group" {
				writeGHValidationError(w, "ClassroomTransition", "assignment.type", "invalid")
				return
			}
			owner, name, found := strings.Cut(assignment.StarterCodeRepository, "/")
			if !found || s.store.GetRepo(owner, name) == nil {
				writeGHError(w, http.StatusUnprocessableEntity, "Migrate the starter repository before importing Classroom data: "+assignment.StarterCodeRepository)
				return
			}
			if !validAutogradingTests(assignment.AutogradingTests) {
				writeGHValidationError(w, "ClassroomTransition", "autograding_tests", "invalid")
				return
			}
			for _, accepted := range assignment.AcceptedRepositories {
				repoOwner, repoName, ok := strings.Cut(accepted.Repository, "/")
				if !ok || s.store.GetRepo(repoOwner, repoName) == nil {
					writeGHError(w, http.StatusUnprocessableEntity, "Migrate the accepted assignment repository before importing Classroom data: "+accepted.Repository)
					return
				}
				for _, student := range accepted.Students {
					if student.Login != "" && s.store.LookupUserByLogin(student.Login) == nil {
						writeGHError(w, http.StatusUnprocessableEntity, "Import references a missing user: "+student.Login)
						return
					}
				}
			}
			code, err := newInviteCodeE()
			if err != nil {
				writeGHError(w, http.StatusInternalServerError, err.Error())
				return
			}
			inviteCodes[courseIndex][assignmentIndex] = code
		}
	}

	created := make([]map[string]interface{}, 0, len(bundle.Classrooms))
	for courseIndex, course := range bundle.Classrooms {
		org := s.store.GetOrg(course.Organization)
		classroom := s.store.CreateClassroom(course.Name, org.ID, course.Archived)
		roster := make([]ClassroomStudent, 0, len(course.Roster))
		for _, entry := range course.Roster {
			userID := 0
			if entry.Login != "" {
				userID = s.store.LookupUserByLogin(entry.Login).ID
			}
			roster = append(roster, ClassroomStudent{UserID: userID, RosterIdentifier: entry.Identifier})
		}
		s.store.UpdateClassroom(classroom.ID, func(current *Classroom) { current.Roster = roster })
		for assignmentIndex, imported := range course.Assignments {
			owner, name, _ := strings.Cut(imported.StarterCodeRepository, "/")
			starter := s.store.GetRepo(owner, name)
			assignment := s.store.CreateClassroomAssignment(&ClassroomAssignment{
				ClassroomID: classroom.ID, Title: imported.Title, Type: imported.Type, Slug: slugify(imported.Title), InviteCode: inviteCodes[courseIndex][assignmentIndex],
				InvitationsEnabled: imported.InvitationsEnabled, PublicRepo: imported.PublicRepo, StudentsAreRepoAdmins: imported.StudentsAreRepoAdmins,
				FeedbackPullRequestsEnabled: imported.FeedbackPullRequestsEnabled, MaxTeams: imported.MaxTeams, MaxMembers: imported.MaxMembers,
				Editor: imported.Editor, Language: imported.Language, Deadline: imported.Deadline, StarterCodeRepoID: starter.ID, AutogradingTests: imported.AutogradingTests,
			})
			for _, importedAccepted := range imported.AcceptedRepositories {
				repoOwner, repoName, _ := strings.Cut(importedAccepted.Repository, "/")
				repo := s.store.GetRepo(repoOwner, repoName)
				students := make([]ClassroomStudent, 0, len(importedAccepted.Students))
				for _, importedStudent := range importedAccepted.Students {
					userID := 0
					if importedStudent.Login != "" {
						userID = s.store.LookupUserByLogin(importedStudent.Login).ID
					}
					students = append(students, ClassroomStudent{UserID: userID, RosterIdentifier: importedStudent.Identifier})
				}
				acceptedAt := importedAccepted.AcceptedAt
				if acceptedAt.IsZero() {
					acceptedAt = time.Now().UTC()
				}
				s.store.CreateClassroomAcceptedAssignment(&ClassroomAcceptedAssignment{AssignmentID: assignment.ID, Students: students, RepoID: repo.ID, GroupName: importedAccepted.GroupName, AcceptedAt: acceptedAt, BaselineSHA: importedAccepted.BaselineSHA})
			}
		}
		created = append(created, s.classroomWebJSON(classroom, s.baseURL(r)))
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"classrooms": created})
}
