package bleephub

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
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
// GitHub Classroom has no public create REST API. Its management and
// acceptance writes belong to the authenticated Classroom browser product;
// the public REST surface remains read-only and organization-admin scoped.

// Classroom is a GitHub Classroom classroom owned by an organization.
type Classroom struct {
	ID       int                `json:"id"`
	Name     string             `json:"name"`
	Archived bool               `json:"archived"`
	OrgID    int                `json:"org_id"`
	Roster   []ClassroomStudent `json:"roster,omitempty"`
}

// ClassroomAssignment is an assignment within a classroom.
type ClassroomAssignment struct {
	ID                          int                        `json:"id"`
	ClassroomID                 int                        `json:"classroom_id"`
	Title                       string                     `json:"title"`
	Type                        string                     `json:"type"` // "individual" or "group"
	Slug                        string                     `json:"slug"`
	InviteCode                  string                     `json:"invite_code"`
	InvitationsEnabled          bool                       `json:"invitations_enabled"`
	PublicRepo                  bool                       `json:"public_repo"`
	StudentsAreRepoAdmins       bool                       `json:"students_are_repo_admins"`
	FeedbackPullRequestsEnabled bool                       `json:"feedback_pull_requests_enabled"`
	MaxTeams                    *int                       `json:"max_teams"`
	MaxMembers                  *int                       `json:"max_members"`
	Editor                      string                     `json:"editor"`
	Language                    string                     `json:"language"`
	Deadline                    *time.Time                 `json:"deadline"`
	StarterCodeRepoID           int                        `json:"starter_code_repo_id"`
	AutogradingTests            []ClassroomAutogradingTest `json:"autograding_tests,omitempty"`
}

type ClassroomAutogradingTest struct {
	Name    string `json:"name"`
	Command string `json:"command"`
	Points  int    `json:"points"`
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
	Students     []ClassroomStudent `json:"students"`
	RepoID       int                `json:"repo_id"`
	GroupName    string             `json:"group_name"`
	AcceptedAt   time.Time          `json:"accepted_at"`
	BaselineSHA  string             `json:"baseline_sha"`
	SubmittedAt  time.Time          `json:"submitted_at,omitempty"` // reloads pre-transition Classroom records
}

func (s *Server) registerGHClassroomRoutes() {
	s.route("GET /api/v3/classrooms", s.classroomLocked(s.handleListClassrooms))
	s.route("GET /api/v3/classrooms/{classroom_id}", s.classroomLocked(s.handleGetClassroom))
	s.route("GET /api/v3/classrooms/{classroom_id}/assignments", s.classroomLocked(s.handleListClassroomAssignments))
	s.route("GET /api/v3/assignments/{assignment_id}", s.classroomLocked(s.handleGetClassroomAssignment))
	s.route("GET /api/v3/assignments/{assignment_id}/accepted_assignments", s.classroomLocked(s.handleListClassroomAcceptedAssignments))
	s.route("GET /api/v3/assignments/{assignment_id}/grades", s.classroomLocked(s.handleListClassroomAssignmentGrades))

}

func (s *Server) classroomLocked(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.classroomMu.Lock()
		defer s.classroomMu.Unlock()
		next(w, r)
	}
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

func (st *Store) UpdateClassroom(id int, update func(*Classroom)) *Classroom {
	st.mu.Lock()
	defer st.mu.Unlock()
	c := st.Classrooms[id]
	if c == nil {
		return nil
	}
	update(c)
	if st.persist != nil {
		st.persist.MustPut("classrooms", strconv.Itoa(c.ID), c)
	}
	return c
}

// DeleteClassroom removes the Classroom product metadata and its assignments.
// Assignment repositories remain ordinary organization repositories, matching
// GitHub Classroom's promise that deleting Classroom data does not delete the
// repositories students worked in.
func (st *Store) DeleteClassroom(id int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.Classrooms[id] == nil {
		return false
	}
	delete(st.Classrooms, id)
	if st.persist != nil {
		st.persist.MustDelete("classrooms", strconv.Itoa(id))
	}
	for assignmentID, assignment := range st.ClassroomAssignments {
		if assignment.ClassroomID != id {
			continue
		}
		delete(st.ClassroomAssignments, assignmentID)
		if st.persist != nil {
			st.persist.MustDelete("classroom_assignments", strconv.Itoa(assignmentID))
		}
		for acceptedID, accepted := range st.ClassroomAcceptedAssignments {
			if accepted.AssignmentID == assignmentID {
				delete(st.ClassroomAcceptedAssignments, acceptedID)
				if st.persist != nil {
					st.persist.MustDelete("classroom_accepted_assignments", strconv.Itoa(acceptedID))
				}
			}
		}
	}
	return true
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

func (st *Store) UpdateClassroomAssignment(id int, update func(*ClassroomAssignment)) *ClassroomAssignment {
	st.mu.Lock()
	defer st.mu.Unlock()
	a := st.ClassroomAssignments[id]
	if a == nil {
		return nil
	}
	update(a)
	if st.persist != nil {
		st.persist.MustPut("classroom_assignments", strconv.Itoa(a.ID), a)
	}
	return a
}

func (st *Store) DeleteClassroomAssignment(id int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.ClassroomAssignments[id] == nil {
		return false
	}
	delete(st.ClassroomAssignments, id)
	if st.persist != nil {
		st.persist.MustDelete("classroom_assignments", strconv.Itoa(id))
	}
	for acceptedID, accepted := range st.ClassroomAcceptedAssignments {
		if accepted.AssignmentID == id {
			delete(st.ClassroomAcceptedAssignments, acceptedID)
			if st.persist != nil {
				st.persist.MustDelete("classroom_accepted_assignments", strconv.Itoa(acceptedID))
			}
		}
	}
	return true
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

func (st *Store) UpdateClassroomAcceptedAssignment(id int, update func(*ClassroomAcceptedAssignment)) *ClassroomAcceptedAssignment {
	st.mu.Lock()
	defer st.mu.Unlock()
	a := st.ClassroomAcceptedAssignments[id]
	if a == nil {
		return nil
	}
	update(a)
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
	return baseURL + "/ui/classrooms/" + strconv.Itoa(c.ID)
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
	if org == nil {
		return nil
	}
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
	if repo == nil {
		return nil
	}
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
	a := s.store.getClassroomAssignment(assignmentID)
	for _, aa := range s.store.classroomAcceptedFor(assignmentID) {
		accepted++
		state := s.classroomAcceptedState(a, aa)
		if state.submitted {
			submitted++
		}
		if state.passing {
			passing++
		}
	}
	return
}

type classroomAcceptedDerivedState struct {
	submitted   bool
	passing     bool
	commitCount int
	grade       string
	awarded     int
	available   int
	submittedAt time.Time
}

// classroomAcceptedState derives Classroom reporting exclusively from the
// generated repository and completed GitHub Actions jobs. The management
// product cannot assert that a student submitted, passed, or earned points.
func (s *Server) classroomAcceptedState(a *ClassroomAssignment, aa *ClassroomAcceptedAssignment) classroomAcceptedDerivedState {
	acceptedAt := classroomAcceptedAt(aa)
	state := classroomAcceptedDerivedState{submittedAt: acceptedAt}
	if a == nil {
		return state
	}
	for _, test := range a.AutogradingTests {
		state.available += test.Points
	}
	repo := s.store.GetRepoByID(aa.RepoID)
	if repo == nil {
		state.grade = fmt.Sprintf("%d/%d", state.awarded, state.available)
		return state
	}
	if commits, ok := s.defaultBranchCommits(repo); ok {
		for _, commit := range commits {
			if aa.BaselineSHA != "" && commit.Hash.String() == aa.BaselineSHA {
				break
			}
			state.commitCount++
			if commit.Committer.When.After(state.submittedAt) {
				state.submittedAt = commit.Committer.When
			}
		}
	}
	state.submitted = a.Deadline != nil && !time.Now().UTC().Before(*a.Deadline)

	jobResults := map[string]Result{}
	s.store.mu.RLock()
	var latest *Workflow
	for _, workflow := range s.store.Workflows {
		if workflow.RepoFullName == repo.FullName && workflow.Status == WorkflowStatusCompleted && (latest == nil || workflow.CreatedAt.After(latest.CreatedAt)) {
			latest = workflow
		}
	}
	if latest != nil {
		for key, job := range latest.Jobs {
			if job.Status == JobStatusCompleted {
				jobResults[key] = job.Result
			}
		}
	}
	s.store.mu.RUnlock()
	for index, test := range a.AutogradingTests {
		if jobResults[fmt.Sprintf("autograding-%d", index+1)] == ResultSuccess {
			state.awarded += test.Points
		}
	}
	state.passing = state.available > 0 && state.awarded == state.available
	state.grade = fmt.Sprintf("%d/%d", state.awarded, state.available)
	return state
}

func classroomAcceptedAt(accepted *ClassroomAcceptedAssignment) time.Time {
	if !accepted.AcceptedAt.IsZero() {
		return accepted.AcceptedAt
	}
	return accepted.SubmittedAt
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
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	s.store.mu.RLock()
	all := make([]*Classroom, 0, len(s.store.Classrooms))
	for _, c := range s.store.Classrooms {
		all = append(all, c)
	}
	s.store.mu.RUnlock()
	classrooms := make([]*Classroom, 0, len(all))
	for _, c := range all {
		org := s.store.GetOrgByID(c.OrgID)
		if org != nil && (user.SiteAdmin || canAdminOrg(s.store, user, org)) {
			classrooms = append(classrooms, c)
		}
	}
	sort.Slice(classrooms, func(i, j int) bool { return classrooms[i].ID < classrooms[j].ID })

	page := paginateAndLink(w, r, classrooms)
	base := s.baseURL(r)
	out := make([]map[string]interface{}, 0, len(page))
	for _, c := range page {
		out = append(out, simpleClassroomJSON(c, base))
	}
	writeJSON(w, http.StatusOK, out)
}

// classroomForAdmin resolves a Classroom resource without disclosing it to
// callers who do not administer its owning organization. GitHub's Classroom
// REST endpoints are organization-administrator endpoints, not public course
// catalogues.
func (s *Server) classroomForAdmin(w http.ResponseWriter, r *http.Request, id int) *Classroom {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return nil
	}
	c := s.store.getClassroom(id)
	if c == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil
	}
	org := s.store.GetOrgByID(c.OrgID)
	if org == nil || (!user.SiteAdmin && !canAdminOrg(s.store, user, org)) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil
	}
	return c
}

func (s *Server) classroomAssignmentForAdmin(w http.ResponseWriter, r *http.Request, id int) *ClassroomAssignment {
	a := s.store.getClassroomAssignment(id)
	if a == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil
	}
	if s.classroomForAdmin(w, r, a.ClassroomID) == nil {
		return nil
	}
	return a
}

func (s *Server) handleGetClassroom(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.PathValue("classroom_id"))
	c := s.classroomForAdmin(w, r, id)
	if c == nil {
		return
	}
	writeJSON(w, http.StatusOK, s.classroomJSON(c, s.baseURL(r)))
}

func (s *Server) handleListClassroomAssignments(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.PathValue("classroom_id"))
	c := s.classroomForAdmin(w, r, id)
	if c == nil {
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
	a := s.classroomAssignmentForAdmin(w, r, id)
	if a == nil {
		return
	}
	writeJSON(w, http.StatusOK, s.classroomAssignmentJSON(a, s.baseURL(r), true))
}

func (s *Server) handleListClassroomAcceptedAssignments(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.PathValue("assignment_id"))
	a := s.classroomAssignmentForAdmin(w, r, id)
	if a == nil {
		return
	}
	accepted := s.store.classroomAcceptedFor(a.ID)
	page := paginateAndLink(w, r, accepted)
	base := s.baseURL(r)
	out := make([]map[string]interface{}, 0, len(page))
	for _, aa := range page {
		state := s.classroomAcceptedState(a, aa)
		students := make([]map[string]interface{}, 0, len(aa.Students))
		for _, cs := range aa.Students {
			u := s.store.GetUserByID(cs.UserID)
			if u == nil {
				writeGHError(w, http.StatusInternalServerError, "Classroom student record references a missing user")
				return
			}
			students = append(students, map[string]interface{}{
				"id":         u.ID,
				"login":      u.Login,
				"avatar_url": u.AvatarURL,
				"html_url":   base + "/" + u.Login,
			})
		}
		repo := s.store.GetRepoByID(aa.RepoID)
		if repo == nil {
			writeGHError(w, http.StatusInternalServerError, "Classroom acceptance references a missing repository")
			return
		}
		out = append(out, map[string]interface{}{
			"id":           aa.ID,
			"submitted":    state.submitted,
			"passing":      state.passing,
			"commit_count": state.commitCount,
			"grade":        state.grade,
			"students":     students,
			"repository":   simpleClassroomRepositoryJSON(repo, base),
			"assignment":   s.classroomAssignmentJSON(a, base, false),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleListClassroomAssignmentGrades(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.PathValue("assignment_id"))
	a := s.classroomAssignmentForAdmin(w, r, id)
	if a == nil {
		return
	}
	base := s.baseURL(r)
	starter := s.store.GetRepoByID(a.StarterCodeRepoID)
	if starter == nil {
		writeGHError(w, http.StatusInternalServerError, "Classroom assignment references a missing starter repository")
		return
	}
	assignmentURL := base + "/a/" + a.InviteCode

	out := make([]map[string]interface{}, 0)
	for _, aa := range s.store.classroomAcceptedFor(a.ID) {
		repo := s.store.GetRepoByID(aa.RepoID)
		if repo == nil {
			writeGHError(w, http.StatusInternalServerError, "Classroom acceptance references a missing repository")
			return
		}
		state := s.classroomAcceptedState(a, aa)
		for _, cs := range aa.Students {
			u := s.store.GetUserByID(cs.UserID)
			if u == nil {
				writeGHError(w, http.StatusInternalServerError, "Classroom acceptance references a missing user")
				return
			}
			row := map[string]interface{}{
				"assignment_name":         a.Title,
				"assignment_url":          assignmentURL,
				"starter_code_url":        base + "/" + starter.FullName,
				"github_username":         u.Login,
				"roster_identifier":       cs.RosterIdentifier,
				"student_repository_name": repo.Name,
				"student_repository_url":  base + "/" + repo.FullName,
				"submission_timestamp":    state.submittedAt.UTC().Format(time.RFC3339),
				"points_awarded":          state.awarded,
				"points_available":        state.available,
			}
			if a.Type == "group" {
				row["group_name"] = aa.GroupName
			}
			out = append(out, row)
		}
	}
	writeJSON(w, http.StatusOK, out)
}
