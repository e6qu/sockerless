package bleephub

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

func newInviteCodeE() (string, error) {
	code, err := randomHex(6)
	if err != nil {
		return "", fmt.Errorf("generate Classroom invite code: %w", err)
	}
	return code, nil
}

// GitHub Classroom REST surface (GET /classrooms, /classrooms/{classroom_id},
// /classrooms/{classroom_id}/assignments, /assignments/{assignment_id},
// /assignments/{assignment_id}/accepted_assignments, and
// /assignments/{assignment_id}/grades).
//
// GitHub Classroom has no public create API — classrooms and assignments are
// managed on classroom.github.com — so bleephub provides internal seeding
// endpoints (the same pattern as Dependabot alert seeding) and serves the
// public read surface from the seeded, persisted entities. Assignment
// acceptance counters and grade rows are derived live from the accepted
// assignments, and repository/organization/student references resolve
// against the real repo, org, and user stores.

// Classroom is a GitHub Classroom classroom owned by an organization.
type Classroom struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Archived bool   `json:"archived"`
	OrgID    int    `json:"org_id"`
}

// ClassroomAssignment is an assignment within a classroom.
type ClassroomAssignment struct {
	ID                          int        `json:"id"`
	ClassroomID                 int        `json:"classroom_id"`
	Title                       string     `json:"title"`
	Type                        string     `json:"type"` // "individual" or "group"
	Slug                        string     `json:"slug"`
	InviteCode                  string     `json:"invite_code"`
	InvitationsEnabled          bool       `json:"invitations_enabled"`
	PublicRepo                  bool       `json:"public_repo"`
	StudentsAreRepoAdmins       bool       `json:"students_are_repo_admins"`
	FeedbackPullRequestsEnabled bool       `json:"feedback_pull_requests_enabled"`
	MaxTeams                    *int       `json:"max_teams"`
	MaxMembers                  *int       `json:"max_members"`
	Editor                      string     `json:"editor"`
	Language                    string     `json:"language"`
	Deadline                    *time.Time `json:"deadline"`
	StarterCodeRepoID           int        `json:"starter_code_repo_id"`
}

// ClassroomStudent links an accepted assignment to a student user with the
// classroom roster identifier.
type ClassroomStudent struct {
	UserID           int    `json:"user_id"`
	RosterIdentifier string `json:"roster_identifier"`
}

// ClassroomAcceptedAssignment records a student's (or team's) acceptance of
// an assignment, backed by the real repository the acceptance created.
type ClassroomAcceptedAssignment struct {
	ID           int                `json:"id"`
	AssignmentID int                `json:"assignment_id"`
	Submitted    bool               `json:"submitted"`
	Passing      bool               `json:"passing"`
	CommitCount  int                `json:"commit_count"`
	Grade        string             `json:"grade"` // "awarded/available", e.g. "8/10"
	Students     []ClassroomStudent `json:"students"`
	RepoID       int                `json:"repo_id"`
	GroupName    string             `json:"group_name"`
	SubmittedAt  time.Time          `json:"submitted_at"`
}

func (s *Server) registerGHClassroomRoutes() {
	s.route("GET /api/v3/classrooms", s.handleListClassrooms)
	s.route("GET /api/v3/classrooms/{classroom_id}", s.handleGetClassroom)
	s.route("GET /api/v3/classrooms/{classroom_id}/assignments", s.handleListClassroomAssignments)
	s.route("GET /api/v3/assignments/{assignment_id}", s.handleGetClassroomAssignment)
	s.route("GET /api/v3/assignments/{assignment_id}/accepted_assignments", s.handleListClassroomAcceptedAssignments)
	s.route("GET /api/v3/assignments/{assignment_id}/grades", s.handleListClassroomAssignmentGrades)

	// Internal seeding endpoints — GitHub Classroom's write surface lives on
	// classroom.github.com, not the REST API.
	s.route("POST /internal/classrooms", s.handleSeedClassroom)
	s.route("POST /internal/classrooms/{classroom_id}/assignments", s.handleSeedClassroomAssignment)
	s.route("POST /internal/assignments/{assignment_id}/accepted_assignments", s.handleSeedClassroomAcceptedAssignment)
}

// --- Store operations ---

func (st *Store) CreateClassroom(name string, orgID int, archived bool) *Classroom {
	st.mu.Lock()
	defer st.mu.Unlock()
	c := &Classroom{ID: st.NextClassroomID, Name: name, Archived: archived, OrgID: orgID}
	st.NextClassroomID++
	st.Classrooms[c.ID] = c
	if st.persist != nil {
		st.persist.MustPut("classrooms", strconv.Itoa(c.ID), c)
	}
	return c
}

func (st *Store) CreateClassroomAssignment(a *ClassroomAssignment) *ClassroomAssignment {
	st.mu.Lock()
	defer st.mu.Unlock()
	a.ID = st.NextClassroomAssignmentID
	st.NextClassroomAssignmentID++
	st.ClassroomAssignments[a.ID] = a
	if st.persist != nil {
		st.persist.MustPut("classroom_assignments", strconv.Itoa(a.ID), a)
	}
	return a
}

func (st *Store) CreateClassroomAcceptedAssignment(a *ClassroomAcceptedAssignment) *ClassroomAcceptedAssignment {
	st.mu.Lock()
	defer st.mu.Unlock()
	a.ID = st.NextClassroomAcceptedID
	st.NextClassroomAcceptedID++
	st.ClassroomAcceptedAssignments[a.ID] = a
	if st.persist != nil {
		st.persist.MustPut("classroom_accepted_assignments", strconv.Itoa(a.ID), a)
	}
	return a
}

// classroomAcceptedFor returns the accepted assignments for an assignment,
// oldest first.
func (st *Store) classroomAcceptedFor(assignmentID int) []*ClassroomAcceptedAssignment {
	st.mu.RLock()
	defer st.mu.RUnlock()
	var out []*ClassroomAcceptedAssignment
	for _, a := range st.ClassroomAcceptedAssignments {
		if a.AssignmentID == assignmentID {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// --- JSON shapes ---

func classroomURL(baseURL string, c *Classroom) string {
	return baseURL + "/classrooms/" + strconv.Itoa(c.ID) + "-" + slugify(c.Name)
}

// simpleClassroomJSON renders the spec `simple-classroom` shape.
func simpleClassroomJSON(c *Classroom, baseURL string) map[string]interface{} {
	return map[string]interface{}{
		"id":       c.ID,
		"name":     c.Name,
		"archived": c.Archived,
		"url":      classroomURL(baseURL, c),
	}
}

// classroomJSON renders the spec `classroom` shape (simple-classroom plus
// the owning organization).
func (s *Server) classroomJSON(c *Classroom, baseURL string) map[string]interface{} {
	out := simpleClassroomJSON(c, baseURL)
	org := s.store.GetOrgByID(c.OrgID)
	out["organization"] = map[string]interface{}{
		"id":         org.ID,
		"login":      org.Login,
		"node_id":    org.NodeID,
		"html_url":   baseURL + "/" + org.Login,
		"name":       nullOrString(org.Name),
		"avatar_url": org.AvatarURL,
	}
	return out
}

// simpleClassroomRepositoryJSON renders the spec `simple-classroom-repository`
// shape from a real repository.
func simpleClassroomRepositoryJSON(repo *Repo, baseURL string) map[string]interface{} {
	return map[string]interface{}{
		"id":             repo.ID,
		"full_name":      repo.FullName,
		"html_url":       baseURL + "/" + repo.FullName,
		"node_id":        repo.NodeID,
		"private":        repo.Private,
		"default_branch": repo.DefaultBranch,
	}
}

// classroomAssignmentCounters derives accepted/submitted/passing from the
// accepted assignments.
func (s *Server) classroomAssignmentCounters(assignmentID int) (accepted, submitted, passing int) {
	for _, aa := range s.store.classroomAcceptedFor(assignmentID) {
		accepted++
		if aa.Submitted {
			submitted++
		}
		if aa.Passing {
			passing++
		}
	}
	return
}

// classroomAssignmentJSON renders the assignment shape; full=true renders
// the spec `classroom-assignment` (full classroom + starter code repo),
// full=false the `simple-classroom-assignment`.
func (s *Server) classroomAssignmentJSON(a *ClassroomAssignment, baseURL string, full bool) map[string]interface{} {
	accepted, submitted, passing := s.classroomAssignmentCounters(a.ID)
	var deadline interface{}
	if a.Deadline != nil {
		deadline = a.Deadline.UTC().Format(time.RFC3339)
	}
	var maxTeams, maxMembers interface{}
	if a.MaxTeams != nil {
		maxTeams = *a.MaxTeams
	}
	if a.MaxMembers != nil {
		maxMembers = *a.MaxMembers
	}
	classroom := s.store.getClassroom(a.ClassroomID)
	out := map[string]interface{}{
		"id":                             a.ID,
		"public_repo":                    a.PublicRepo,
		"title":                          a.Title,
		"type":                           a.Type,
		"invite_link":                    baseURL + "/a/" + a.InviteCode,
		"invitations_enabled":            a.InvitationsEnabled,
		"slug":                           a.Slug,
		"students_are_repo_admins":       a.StudentsAreRepoAdmins,
		"feedback_pull_requests_enabled": a.FeedbackPullRequestsEnabled,
		"max_teams":                      maxTeams,
		"max_members":                    maxMembers,
		"editor":                         a.Editor,
		"accepted":                       accepted,
		"submitted":                      submitted,
		"passing":                        passing,
		"language":                       a.Language,
		"deadline":                       deadline,
	}
	if full {
		out["classroom"] = s.classroomJSON(classroom, baseURL)
		out["starter_code_repository"] = simpleClassroomRepositoryJSON(s.store.GetRepoByID(a.StarterCodeRepoID), baseURL)
	} else {
		out["classroom"] = simpleClassroomJSON(classroom, baseURL)
	}
	return out
}

func (st *Store) getClassroom(id int) *Classroom {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.Classrooms[id]
}

func (st *Store) getClassroomAssignment(id int) *ClassroomAssignment {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.ClassroomAssignments[id]
}

// --- Read handlers ---

func (s *Server) handleListClassrooms(w http.ResponseWriter, r *http.Request) {
	if ghUserFromContext(r.Context()) == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	s.store.mu.RLock()
	classrooms := make([]*Classroom, 0, len(s.store.Classrooms))
	for _, c := range s.store.Classrooms {
		classrooms = append(classrooms, c)
	}
	s.store.mu.RUnlock()
	sort.Slice(classrooms, func(i, j int) bool { return classrooms[i].ID < classrooms[j].ID })

	page := paginateAndLink(w, r, classrooms)
	base := s.baseURL(r)
	out := make([]map[string]interface{}, 0, len(page))
	for _, c := range page {
		out = append(out, simpleClassroomJSON(c, base))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGetClassroom(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.PathValue("classroom_id"))
	c := s.store.getClassroom(id)
	if c == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, s.classroomJSON(c, s.baseURL(r)))
}

func (s *Server) handleListClassroomAssignments(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.PathValue("classroom_id"))
	c := s.store.getClassroom(id)
	if c == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.store.mu.RLock()
	assignments := make([]*ClassroomAssignment, 0)
	for _, a := range s.store.ClassroomAssignments {
		if a.ClassroomID == c.ID {
			assignments = append(assignments, a)
		}
	}
	s.store.mu.RUnlock()
	sort.Slice(assignments, func(i, j int) bool { return assignments[i].ID < assignments[j].ID })

	page := paginateAndLink(w, r, assignments)
	base := s.baseURL(r)
	out := make([]map[string]interface{}, 0, len(page))
	for _, a := range page {
		out = append(out, s.classroomAssignmentJSON(a, base, false))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGetClassroomAssignment(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.PathValue("assignment_id"))
	a := s.store.getClassroomAssignment(id)
	if a == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, s.classroomAssignmentJSON(a, s.baseURL(r), true))
}

func (s *Server) handleListClassroomAcceptedAssignments(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.PathValue("assignment_id"))
	a := s.store.getClassroomAssignment(id)
	if a == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	accepted := s.store.classroomAcceptedFor(a.ID)
	page := paginateAndLink(w, r, accepted)
	base := s.baseURL(r)
	out := make([]map[string]interface{}, 0, len(page))
	for _, aa := range page {
		students := make([]map[string]interface{}, 0, len(aa.Students))
		for _, cs := range aa.Students {
			u := s.store.GetUserByID(cs.UserID)
			students = append(students, map[string]interface{}{
				"id":         u.ID,
				"login":      u.Login,
				"avatar_url": u.AvatarURL,
				"html_url":   base + "/" + u.Login,
			})
		}
		out = append(out, map[string]interface{}{
			"id":           aa.ID,
			"submitted":    aa.Submitted,
			"passing":      aa.Passing,
			"commit_count": aa.CommitCount,
			"grade":        aa.Grade,
			"students":     students,
			"repository":   simpleClassroomRepositoryJSON(s.store.GetRepoByID(aa.RepoID), base),
			"assignment":   s.classroomAssignmentJSON(a, base, false),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleListClassroomAssignmentGrades(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.PathValue("assignment_id"))
	a := s.store.getClassroomAssignment(id)
	if a == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	base := s.baseURL(r)
	starter := s.store.GetRepoByID(a.StarterCodeRepoID)
	assignmentURL := base + "/a/" + a.InviteCode

	out := make([]map[string]interface{}, 0)
	for _, aa := range s.store.classroomAcceptedFor(a.ID) {
		repo := s.store.GetRepoByID(aa.RepoID)
		awarded, available := parseClassroomGrade(aa.Grade)
		for _, cs := range aa.Students {
			u := s.store.GetUserByID(cs.UserID)
			row := map[string]interface{}{
				"assignment_name":         a.Title,
				"assignment_url":          assignmentURL,
				"starter_code_url":        base + "/" + starter.FullName,
				"github_username":         u.Login,
				"roster_identifier":       cs.RosterIdentifier,
				"student_repository_name": repo.Name,
				"student_repository_url":  base + "/" + repo.FullName,
				"submission_timestamp":    aa.SubmittedAt.UTC().Format(time.RFC3339),
				"points_awarded":          awarded,
				"points_available":        available,
			}
			if a.Type == "group" {
				row["group_name"] = aa.GroupName
			}
			out = append(out, row)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// parseClassroomGrade splits an "awarded/available" grade string into its
// numeric parts.
func parseClassroomGrade(grade string) (awarded, available int) {
	a, b, found := strings.Cut(grade, "/")
	if !found {
		return 0, 0
	}
	awarded, _ = strconv.Atoi(strings.TrimSpace(a))
	available, _ = strconv.Atoi(strings.TrimSpace(b))
	return awarded, available
}

// --- Internal seeding handlers ---

func (s *Server) handleSeedClassroom(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name"`
		Org      string `json:"org"`
		Archived bool   `json:"archived"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeGHError(w, http.StatusBadRequest, "Problems parsing JSON")
		return
	}
	if req.Name == "" {
		writeGHValidationError(w, "Classroom", "name", "missing_field")
		return
	}
	org := s.store.GetOrg(req.Org)
	if org == nil {
		writeGHError(w, http.StatusUnprocessableEntity, "Organization not found: "+req.Org)
		return
	}
	c := s.store.CreateClassroom(req.Name, org.ID, req.Archived)
	writeJSON(w, http.StatusCreated, s.classroomJSON(c, s.baseURL(r)))
}

func (s *Server) handleSeedClassroomAssignment(w http.ResponseWriter, r *http.Request) {
	classroomID, _ := strconv.Atoi(r.PathValue("classroom_id"))
	classroom := s.store.getClassroom(classroomID)
	if classroom == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	var req struct {
		Title                       string     `json:"title"`
		Type                        string     `json:"type"`
		StarterCodeRepository       string     `json:"starter_code_repository"`
		PublicRepo                  bool       `json:"public_repo"`
		InvitationsEnabled          bool       `json:"invitations_enabled"`
		StudentsAreRepoAdmins       bool       `json:"students_are_repo_admins"`
		FeedbackPullRequestsEnabled bool       `json:"feedback_pull_requests_enabled"`
		MaxTeams                    *int       `json:"max_teams"`
		MaxMembers                  *int       `json:"max_members"`
		Editor                      string     `json:"editor"`
		Language                    string     `json:"language"`
		Deadline                    *time.Time `json:"deadline"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeGHError(w, http.StatusBadRequest, "Problems parsing JSON")
		return
	}
	if req.Title == "" {
		writeGHValidationError(w, "ClassroomAssignment", "title", "missing_field")
		return
	}
	if req.Type != "individual" && req.Type != "group" {
		writeGHValidationError(w, "ClassroomAssignment", "type", "invalid")
		return
	}
	owner, name, _ := strings.Cut(req.StarterCodeRepository, "/")
	starter := s.store.GetRepo(owner, name)
	if starter == nil {
		writeGHError(w, http.StatusUnprocessableEntity, "Starter code repository not found: "+req.StarterCodeRepository)
		return
	}
	inviteCode, err := newInviteCodeE()
	if err != nil {
		writeGHError(w, http.StatusInternalServerError, err.Error())
		return
	}
	a := s.store.CreateClassroomAssignment(&ClassroomAssignment{
		ClassroomID:                 classroom.ID,
		Title:                       req.Title,
		Type:                        req.Type,
		Slug:                        slugify(req.Title),
		InviteCode:                  inviteCode,
		InvitationsEnabled:          req.InvitationsEnabled,
		PublicRepo:                  req.PublicRepo,
		StudentsAreRepoAdmins:       req.StudentsAreRepoAdmins,
		FeedbackPullRequestsEnabled: req.FeedbackPullRequestsEnabled,
		MaxTeams:                    req.MaxTeams,
		MaxMembers:                  req.MaxMembers,
		Editor:                      req.Editor,
		Language:                    req.Language,
		Deadline:                    req.Deadline,
		StarterCodeRepoID:           starter.ID,
	})
	writeJSON(w, http.StatusCreated, s.classroomAssignmentJSON(a, s.baseURL(r), true))
}

func (s *Server) handleSeedClassroomAcceptedAssignment(w http.ResponseWriter, r *http.Request) {
	assignmentID, _ := strconv.Atoi(r.PathValue("assignment_id"))
	assignment := s.store.getClassroomAssignment(assignmentID)
	if assignment == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	var req struct {
		Students []struct {
			Login            string `json:"login"`
			RosterIdentifier string `json:"roster_identifier"`
		} `json:"students"`
		Repository  string     `json:"repository"`
		Submitted   bool       `json:"submitted"`
		Passing     bool       `json:"passing"`
		CommitCount int        `json:"commit_count"`
		Grade       string     `json:"grade"`
		GroupName   string     `json:"group_name"`
		SubmittedAt *time.Time `json:"submitted_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeGHError(w, http.StatusBadRequest, "Problems parsing JSON")
		return
	}
	if len(req.Students) == 0 {
		writeGHValidationError(w, "ClassroomAcceptedAssignment", "students", "missing_field")
		return
	}
	students := make([]ClassroomStudent, 0, len(req.Students))
	for _, st := range req.Students {
		u := s.store.LookupUserByLogin(st.Login)
		if u == nil {
			writeGHError(w, http.StatusUnprocessableEntity, "Student not found: "+st.Login)
			return
		}
		students = append(students, ClassroomStudent{UserID: u.ID, RosterIdentifier: st.RosterIdentifier})
	}
	owner, name, _ := strings.Cut(req.Repository, "/")
	repo := s.store.GetRepo(owner, name)
	if repo == nil {
		writeGHError(w, http.StatusUnprocessableEntity, "Repository not found: "+req.Repository)
		return
	}
	submittedAt := time.Now().UTC()
	if req.SubmittedAt != nil {
		submittedAt = *req.SubmittedAt
	}
	aa := s.store.CreateClassroomAcceptedAssignment(&ClassroomAcceptedAssignment{
		AssignmentID: assignment.ID,
		Submitted:    req.Submitted,
		Passing:      req.Passing,
		CommitCount:  req.CommitCount,
		Grade:        req.Grade,
		Students:     students,
		RepoID:       repo.ID,
		GroupName:    req.GroupName,
		SubmittedAt:  submittedAt,
	})
	writeJSON(w, http.StatusCreated, map[string]interface{}{"id": aa.ID})
}
