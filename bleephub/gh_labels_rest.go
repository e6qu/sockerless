package bleephub

import (
	"net/http"
	"sort"
	"strconv"
	"time"
)

func (s *Server) registerGHIssueRoutes() {
	// Labels — issues:write covers labels (real GH conflates the two; admin
	// would be required for organization-level changes which bleephub doesn't model).
	s.route("POST /api/v3/repos/{owner}/{repo}/labels", s.requirePerm(scopeIssues, permWrite, s.handleCreateLabel))
	s.route("GET /api/v3/repos/{owner}/{repo}/labels", s.handleListLabels)
	s.route("GET /api/v3/repos/{owner}/{repo}/labels/{name}", s.handleGetLabel)
	s.route("PATCH /api/v3/repos/{owner}/{repo}/labels/{name}", s.requirePerm(scopeIssues, permWrite, s.handleUpdateLabel))
	s.route("DELETE /api/v3/repos/{owner}/{repo}/labels/{name}", s.requirePerm(scopeIssues, permWrite, s.handleDeleteLabel))

	// Milestones
	s.route("POST /api/v3/repos/{owner}/{repo}/milestones", s.requirePerm(scopeIssues, permWrite, s.handleCreateMilestone))
	s.route("GET /api/v3/repos/{owner}/{repo}/milestones", s.handleListMilestones)
	s.route("GET /api/v3/repos/{owner}/{repo}/milestones/{number}/labels", s.handleListMilestoneLabels)
	s.route("GET /api/v3/repos/{owner}/{repo}/milestones/{number}", s.handleGetMilestone)
	s.route("PATCH /api/v3/repos/{owner}/{repo}/milestones/{number}", s.requirePerm(scopeIssues, permWrite, s.handleUpdateMilestone))
	s.route("DELETE /api/v3/repos/{owner}/{repo}/milestones/{number}", s.requirePerm(scopeIssues, permWrite, s.handleDeleteMilestone))

	// Issues
	s.route("POST /api/v3/repos/{owner}/{repo}/issues", s.requirePerm(scopeIssues, permWrite, s.handleCreateIssue))
	s.route("GET /api/v3/repos/{owner}/{repo}/issues", s.handleListIssues)
	s.route("GET /api/v3/orgs/{org}/issues", s.handleListOrgIssues)
	s.route("GET /api/v3/repos/{owner}/{repo}/issues/{number}", s.handleGetIssue)
	s.route("PATCH /api/v3/repos/{owner}/{repo}/issues/{number}", s.requirePerm(scopeIssues, permWrite, s.handleUpdateIssue))

	// Issue comments. GET /issues/comments/{id} conflicts with
	// GET /issues/{number}/reactions (and GET /issues/events/{id}) under
	// Go 1.22's mux, so all two-segment issue GET paths dispatch via
	// handleIssuesTwoSegGetDispatch.
	s.route("GET /api/v3/repos/{owner}/{repo}/issues/{p1}/{p2}", s.handleIssuesTwoSegGetDispatch)

	s.route("POST /api/v3/repos/{owner}/{repo}/issues/{number}/comments", s.requirePerm(scopeIssues, permWrite, s.handleCreateIssueComment))
	s.route("PATCH /api/v3/repos/{owner}/{repo}/issues/comments/{comment_id}", s.requirePerm(scopeIssues, permWrite, s.handleUpdateIssueComment))

	// Issue + PR moderation — comment-by-id delete + lock/unlock collide at
	// `/issues/{p1}/{p2}` because Go 1.22's mux can't disambiguate
	// `/issues/comments/{id}` from `/issues/{n}/lock`. Dispatch via a
	// single 2-segment handler at delete time.
	s.route("DELETE /api/v3/repos/{owner}/{repo}/issues/{p1}/{p2}", s.requirePerm(scopeIssues, permWrite, s.handleIssuesDeleteDispatch))
	s.route("PUT /api/v3/repos/{owner}/{repo}/issues/{number}/lock", s.requirePerm(scopeIssues, permWrite, s.handleLockIssue))

	// Issue label management
	s.route("POST /api/v3/repos/{owner}/{repo}/issues/{number}/labels", s.requirePerm(scopeIssues, permWrite, s.handleAddIssueLabels))
	s.route("PUT /api/v3/repos/{owner}/{repo}/issues/{number}/labels", s.requirePerm(scopeIssues, permWrite, s.handleSetIssueLabels))
	s.route("DELETE /api/v3/repos/{owner}/{repo}/issues/{number}/labels", s.requirePerm(scopeIssues, permWrite, s.handleClearIssueLabels))

	// Issue comments (repo-level)
	s.route("GET /api/v3/repos/{owner}/{repo}/issues/comments", s.handleListRepoIssueComments)
	s.route("PUT /api/v3/repos/{owner}/{repo}/issues/comments/{comment_id}/pin", s.requirePerm(scopeIssues, permWrite, s.handlePinIssueComment))

	// Issue assignees
	s.route("POST /api/v3/repos/{owner}/{repo}/issues/{number}/assignees", s.requirePerm(scopeIssues, permWrite, s.handleAddIssueAssignees))
	s.route("DELETE /api/v3/repos/{owner}/{repo}/issues/{number}/assignees", s.requirePerm(scopeIssues, permWrite, s.handleRemoveIssueAssignees))

	// Issue timeline + events
	s.route("GET /api/v3/repos/{owner}/{repo}/issues/events", s.handleListRepoIssueEvents)

	// Sub-issues + issue dependencies (gh_sub_issues.go); issue-field-values
	// POST/PUT live in gh_issue_fields.go. List GETs and the sub-issue
	// removal dispatch through the shared two-/three-segment wildcard
	// handlers below.
	s.route("POST /api/v3/repos/{owner}/{repo}/issues/{number}/sub_issues", s.requirePerm(scopeIssues, permWrite, s.handleCreateSubIssue))
	s.route("PATCH /api/v3/repos/{owner}/{repo}/issues/{number}/sub_issues/priority", s.requirePerm(scopeIssues, permWrite, s.handleReprioritizeSubIssue))
	s.route("POST /api/v3/repos/{owner}/{repo}/issues/{number}/dependencies/blocked_by", s.requirePerm(scopeIssues, permWrite, s.handleAddIssueDependencyBlockedBy))
	s.route("DELETE /api/v3/repos/{owner}/{repo}/issues/{number}/dependencies/blocked_by/{issue_id}", s.requirePerm(scopeIssues, permWrite, s.handleRemoveIssueDependencyBlockedBy))

	// Go's mux cannot disambiguate 3-segment issue DELETE paths (e.g.
	// /issues/{n}/labels/{name} vs /issues/{n}/reactions/{id}), so they
	// dispatch from one handler. Direct routes for labels/sub-issues are more
	// specific and take precedence.
	s.route("DELETE /api/v3/repos/{owner}/{repo}/issues/{p1}/{p2}/{p3}", s.requirePerm(scopeIssues, permWrite, s.handleIssuesThreeSegDeleteDispatch))

	// 3-segment issue GET paths (e.g. /issues/comments/{id}/reactions vs
	// /issues/{n}/dependencies/blocked_by) also dispatch from one handler.
	s.route("GET /api/v3/repos/{owner}/{repo}/issues/{p1}/{p2}/{p3}", s.handleIssuesThreeSegGetDispatch)
}

// --- Label handlers ---

func (s *Server) handleCreateLabel(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}

	owner := r.PathValue("owner")
	name := r.PathValue("repo")
	repo := s.store.GetRepo(owner, name)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	var req struct {
		Name        string `json:"name"`
		Color       string `json:"color"`
		Description string `json:"description"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Name == "" {
		writeGHValidationError(w, "Label", "name", "missing_field")
		return
	}

	label := s.store.CreateLabel(repo.ID, req.Name, req.Description, req.Color)
	if label == nil {
		writeGHValidationError(w, "Label", "name", "already_exists")
		return
	}

	repoKey := owner + "/" + name
	s.recordAuditEvent("label.create", user.Login, "", map[string]interface{}{"repo": repoKey, "label_id": label.ID, "name": label.Name})
	writeJSON(w, http.StatusCreated, issueLabelToJSON(label, s.baseURL(r), repo.FullName))
}

func (s *Server) handleListLabels(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	name := r.PathValue("repo")
	repo := s.store.GetRepo(owner, name)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	labels := s.store.ListLabels(repo.ID)
	base := s.baseURL(r)
	result := make([]map[string]interface{}, 0, len(labels))
	for _, l := range labels {
		result = append(result, issueLabelToJSON(l, base, repo.FullName))
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, result))
}

func (s *Server) handleGetLabel(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	repoName := r.PathValue("repo")
	labelName := r.PathValue("name")
	repo := s.store.GetRepo(owner, repoName)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	label := s.store.GetLabelByName(repo.ID, labelName)
	if label == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	writeJSON(w, http.StatusOK, issueLabelToJSON(label, s.baseURL(r), repo.FullName))
}

func (s *Server) handleUpdateLabel(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}

	owner := r.PathValue("owner")
	repoName := r.PathValue("repo")
	labelName := r.PathValue("name")
	repo := s.store.GetRepo(owner, repoName)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	label := s.store.GetLabelByName(repo.ID, labelName)
	if label == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	var req map[string]interface{}
	if !decodeJSONBody(w, r, &req) {
		return
	}

	s.store.UpdateLabel(label.ID, func(l *IssueLabel) {
		if v, ok := req["new_name"].(string); ok {
			l.Name = v
		}
		if v, ok := req["color"].(string); ok {
			l.Color = v
		}
		if v, ok := req["description"].(string); ok {
			l.Description = v
		}
	})

	updated := s.store.GetLabel(label.ID)
	writeJSON(w, http.StatusOK, issueLabelToJSON(updated, s.baseURL(r), repo.FullName))
}

func (s *Server) handleDeleteLabel(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}

	owner := r.PathValue("owner")
	repoName := r.PathValue("repo")
	labelName := r.PathValue("name")
	repo := s.store.GetRepo(owner, repoName)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	label := s.store.GetLabelByName(repo.ID, labelName)
	if label == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	repoKey := owner + "/" + repoName
	s.store.DeleteLabel(label.ID)
	s.recordAuditEvent("label.delete", user.Login, "", map[string]interface{}{"repo": repoKey, "label_id": label.ID})
	w.WriteHeader(http.StatusNoContent)
}

// --- Milestone handlers ---

func (s *Server) handleCreateMilestone(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}

	owner := r.PathValue("owner")
	name := r.PathValue("repo")
	repo := s.store.GetRepo(owner, name)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	var req struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		State       string `json:"state"`
		DueOn       string `json:"due_on"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Title == "" {
		writeGHError(w, http.StatusUnprocessableEntity, "Validation Failed")
		return
	}

	var dueOn *time.Time
	if req.DueOn != "" {
		t, err := time.Parse(time.RFC3339, req.DueOn)
		if err == nil {
			dueOn = &t
		}
	}

	ms := s.store.CreateMilestone(repo.ID, user.ID, req.Title, req.Description, req.State, dueOn)
	if ms == nil {
		writeGHError(w, http.StatusUnprocessableEntity, "Validation Failed")
		return
	}

	repoKey := owner + "/" + name
	s.recordAuditEvent("milestone.create", user.Login, "", map[string]interface{}{"repo": repoKey, "milestone_id": ms.ID, "title": ms.Title})
	writeJSON(w, http.StatusCreated, milestoneToJSON(ms, s.store, s.baseURL(r), repo.FullName))
}

func (s *Server) handleListMilestones(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	name := r.PathValue("repo")
	repo := s.store.GetRepo(owner, name)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	state := r.URL.Query().Get("state")
	if state == "" {
		state = "open"
	}

	milestones := s.store.ListMilestones(repo.ID, state)
	base := s.baseURL(r)
	result := make([]map[string]interface{}, 0, len(milestones))
	for _, ms := range milestones {
		result = append(result, milestoneToJSON(ms, s.store, base, repo.FullName))
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, result))
}

func (s *Server) handleGetMilestone(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	repoName := r.PathValue("repo")
	numStr := r.PathValue("number")
	repo := s.store.GetRepo(owner, repoName)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	num, err := strconv.Atoi(numStr)
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	ms := s.store.GetMilestoneByNumber(repo.ID, num)
	if ms == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	writeJSON(w, http.StatusOK, milestoneToJSON(ms, s.store, s.baseURL(r), repo.FullName))
}

func (s *Server) handleUpdateMilestone(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}

	owner := r.PathValue("owner")
	repoName := r.PathValue("repo")
	numStr := r.PathValue("number")
	repo := s.store.GetRepo(owner, repoName)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	num, err := strconv.Atoi(numStr)
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	ms := s.store.GetMilestoneByNumber(repo.ID, num)
	if ms == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	var req map[string]interface{}
	if !decodeJSONBody(w, r, &req) {
		return
	}

	s.store.UpdateMilestone(ms.ID, func(m *Milestone) {
		if v, ok := req["title"].(string); ok {
			m.Title = v
		}
		if v, ok := req["description"].(string); ok {
			m.Description = v
		}
		if v, ok := req["state"].(string); ok {
			if v == "closed" && m.State != "closed" {
				now := time.Now().UTC()
				m.ClosedAt = &now
			} else if v == "open" {
				m.ClosedAt = nil
			}
			m.State = v
		}
	})

	updated := s.store.GetMilestone(ms.ID)
	writeJSON(w, http.StatusOK, milestoneToJSON(updated, s.store, s.baseURL(r), repo.FullName))
}

func (s *Server) handleDeleteMilestone(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}

	owner := r.PathValue("owner")
	repoName := r.PathValue("repo")
	numStr := r.PathValue("number")
	repo := s.store.GetRepo(owner, repoName)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	num, err := strconv.Atoi(numStr)
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	ms := s.store.GetMilestoneByNumber(repo.ID, num)
	if ms == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	repoKey := owner + "/" + repoName
	s.store.DeleteMilestone(ms.ID)
	s.recordAuditEvent("milestone.delete", user.Login, "", map[string]interface{}{"repo": repoKey, "milestone_id": ms.ID})
	w.WriteHeader(http.StatusNoContent)
}

// handleListMilestoneLabels — GET /repos/{o}/{r}/milestones/{number}/labels.
// Lists the labels on every issue (and pull request — PRs are issues) in
// the milestone, each label once.
func (s *Server) handleListMilestoneLabels(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	repoName := r.PathValue("repo")
	repo := s.store.GetRepo(owner, repoName)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	num, err := strconv.Atoi(r.PathValue("number"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	ms := s.store.GetMilestoneByNumber(repo.ID, num)
	if ms == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	s.store.mu.RLock()
	seen := map[int]bool{}
	var labels []*IssueLabel
	collect := func(labelIDs []int) {
		for _, lid := range labelIDs {
			if seen[lid] {
				continue
			}
			if l, ok := s.store.Labels[lid]; ok {
				seen[lid] = true
				labels = append(labels, l)
			}
		}
	}
	for _, issue := range s.store.Issues {
		if issue.MilestoneID == ms.ID {
			collect(issue.LabelIDs)
		}
	}
	for _, pr := range s.store.PullRequests {
		if pr.MilestoneID == ms.ID {
			collect(pr.LabelIDs)
		}
	}
	s.store.mu.RUnlock()

	sort.Slice(labels, func(i, j int) bool { return labels[i].ID < labels[j].ID })
	base := s.baseURL(r)
	out := make([]map[string]interface{}, 0, len(labels))
	for _, l := range labels {
		out = append(out, issueLabelToJSON(l, base, repo.FullName))
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, out))
}

// --- JSON converters ---

func issueLabelToJSON(l *IssueLabel, baseURL, repoFullName string) map[string]interface{} {
	return map[string]interface{}{
		"id":          l.ID,
		"node_id":     l.NodeID,
		"url":         baseURL + "/api/v3/repos/" + repoFullName + "/labels/" + l.Name,
		"name":        l.Name,
		"description": l.Description,
		"color":       l.Color,
		"default":     l.Default,
	}
}

// milestoneToJSON converts a Milestone to the GitHub `milestone` shape.
// Open/closed issue counts are derived live from the issues and pull
// requests attached to the milestone (PRs count because they are issues
// internally on GitHub). Must not be called with st.mu held.
func milestoneToJSON(ms *Milestone, st *Store, baseURL, repoFullName string) map[string]interface{} {
	var dueOn interface{}
	if ms.DueOn != nil {
		dueOn = ms.DueOn.Format(time.RFC3339)
	}
	var closedAt interface{}
	if ms.ClosedAt != nil {
		closedAt = ms.ClosedAt.Format(time.RFC3339)
	}

	st.mu.RLock()
	var creatorJSON interface{}
	if u, ok := st.Users[ms.CreatorID]; ok {
		creatorJSON = userToJSON(u)
	}
	openIssues, closedIssues := 0, 0
	for _, issue := range st.Issues {
		if issue.MilestoneID != ms.ID {
			continue
		}
		if issue.State == "OPEN" {
			openIssues++
		} else {
			closedIssues++
		}
	}
	for _, pr := range st.PullRequests {
		if pr.MilestoneID != ms.ID {
			continue
		}
		if pr.State == "OPEN" {
			openIssues++
		} else {
			closedIssues++
		}
	}
	st.mu.RUnlock()

	return map[string]interface{}{
		"id":            ms.ID,
		"node_id":       ms.NodeID,
		"url":           baseURL + "/api/v3/repos/" + repoFullName + "/milestones/" + strconv.Itoa(ms.Number),
		"html_url":      baseURL + "/" + repoFullName + "/milestone/" + strconv.Itoa(ms.Number),
		"labels_url":    baseURL + "/api/v3/repos/" + repoFullName + "/milestones/" + strconv.Itoa(ms.Number) + "/labels",
		"number":        ms.Number,
		"title":         ms.Title,
		"description":   ms.Description,
		"state":         ms.State,
		"creator":       creatorJSON,
		"open_issues":   openIssues,
		"closed_issues": closedIssues,
		"due_on":        dueOn,
		"closed_at":     closedAt,
		"created_at":    ms.CreatedAt.Format(time.RFC3339),
		"updated_at":    ms.UpdatedAt.Format(time.RFC3339),
	}
}

// handleIssuesTwoSegDispatchGET resolves GET /repos/{}/issues/{p1}/{p2} to
// either an issue-comment lookup (/issues/comments/{id}), an issue-event
// lookup (/issues/events/{id}), or one of the per-issue sub-resources
// (comments, timeline, events, reactions, sub-issues, issue-field-values).
// Go 1.22's mux cannot disambiguate these literal-and-wildcard mixtures.
func (s *Server) handleIssuesTwoSegGetDispatch(w http.ResponseWriter, r *http.Request) {
	p1 := r.PathValue("p1")
	p2 := r.PathValue("p2")
	switch {
	case p1 == "comments":
		r.SetPathValue("comment_id", p2)
		s.handleGetIssueComment(w, r)
	case p1 == "events":
		r.SetPathValue("event_id", p2)
		s.handleGetIssueEvent(w, r)
	case p2 == "comments":
		r.SetPathValue("number", p1)
		s.handleListIssueComments(w, r)
	case p2 == "timeline":
		r.SetPathValue("number", p1)
		s.handleListIssueTimeline(w, r)
	case p2 == "events":
		r.SetPathValue("number", p1)
		s.handleListIssueEvents(w, r)
	case p2 == "reactions":
		r.SetPathValue("number", p1)
		s.handleListReactions("issue", "number")(w, r)
	case p2 == "sub_issues" || p2 == "sub_issue":
		r.SetPathValue("number", p1)
		s.handleListSubIssues(w, r)
	case p2 == "issue-field-values":
		r.SetPathValue("number", p1)
		s.handleListIssueFieldValues(w, r)
	default:
		writeGHError(w, http.StatusNotFound, "Not Found")
	}
}

// handleIssuesThreeSegDeleteDispatch resolves DELETE /repos/{}/issues/{p1}/{p2}/{p3}
// to the correct handler. Go 1.22's mux cannot disambiguate literal segments
// from wildcard segments at the same depth, so issue reaction deletes live here
// alongside the (more specific) direct routes for labels and sub-issues.
func (s *Server) handleIssuesThreeSegDeleteDispatch(w http.ResponseWriter, r *http.Request) {
	p1 := r.PathValue("p1")
	p2 := r.PathValue("p2")
	p3 := r.PathValue("p3")
	switch {
	case p1 == "comments" && p3 == "pin":
		r.SetPathValue("comment_id", p2)
		s.handleUnpinIssueComment(w, r)
	case p2 == "reactions":
		r.SetPathValue("number", p1)
		r.SetPathValue("reaction_id", p3)
		s.handleDeleteReaction("issue", "number")(w, r)
	case p2 == "labels":
		r.SetPathValue("number", p1)
		r.SetPathValue("name", p3)
		s.handleRemoveIssueLabel(w, r)
	default:
		writeGHError(w, http.StatusNotFound, "Not Found")
	}
}

// handleIssuesThreeSegGetDispatch resolves GET /repos/{}/issues/{p1}/{p2}/{p3}
// to the correct handler. Go 1.22's mux cannot disambiguate literal segments
// (comments) from wildcard segments (number) at the same depth.
func (s *Server) handleIssuesThreeSegGetDispatch(w http.ResponseWriter, r *http.Request) {
	p1 := r.PathValue("p1")
	p2 := r.PathValue("p2")
	p3 := r.PathValue("p3")
	switch {
	case p1 == "comments" && p3 == "reactions":
		r.SetPathValue("comment_id", p2)
		s.handleListReactions("issue_comment", "comment_id")(w, r)
	case p2 == "dependencies" && p3 == "blocked_by":
		r.SetPathValue("number", p1)
		s.handleListIssueDependenciesBlockedBy(w, r)
	default:
		writeGHError(w, http.StatusNotFound, "Not Found")
	}
}
