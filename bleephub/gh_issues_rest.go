package bleephub

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// --- Issue handlers ---

func (s *Server) handleCreateIssue(w http.ResponseWriter, r *http.Request) {
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
		Title     string   `json:"title"`
		Body      string   `json:"body"`
		Labels    []string `json:"labels"`
		Assignees []string `json:"assignees"`
		Milestone int      `json:"milestone"` // milestone number
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeGHError(w, http.StatusBadRequest, "Problems parsing JSON")
		return
	}
	if req.Title == "" {
		writeGHValidationError(w, "Issue", "title", "missing_field")
		return
	}

	// Resolve label names to IDs
	var labelIDs []int
	for _, name := range req.Labels {
		l := s.store.GetLabelByName(repo.ID, name)
		if l != nil {
			labelIDs = append(labelIDs, l.ID)
		}
	}

	// Resolve assignee logins to IDs
	var assigneeIDs []int
	for _, login := range req.Assignees {
		u := s.store.LookupUserByLogin(login)
		if u != nil {
			assigneeIDs = append(assigneeIDs, u.ID)
		}
	}

	// Resolve milestone number to ID
	var milestoneID int
	if req.Milestone > 0 {
		ms := s.store.GetMilestoneByNumber(repo.ID, req.Milestone)
		if ms != nil {
			milestoneID = ms.ID
		}
	}

	issue := s.store.CreateIssue(repo.ID, user.ID, req.Title, req.Body, labelIDs, assigneeIDs, milestoneID)
	if issue == nil {
		writeGHError(w, http.StatusUnprocessableEntity, "Issue creation failed")
		return
	}

	repoKey := owner + "/" + name
	s.emitWebhookEvent(repoKey, "issues", "opened", buildIssuesPayload(repo, issue, user, "opened"))

	writeJSON(w, http.StatusCreated, issueToJSON(issue, s.store, s.baseURL(r), repo.FullName))
}

func (s *Server) handleListIssues(w http.ResponseWriter, r *http.Request) {
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

	// Map REST state to internal state
	stateFilter := ""
	switch state {
	case "open":
		stateFilter = "OPEN"
	case "closed":
		stateFilter = "CLOSED"
	case "all":
		stateFilter = "all"
	default:
		stateFilter = "OPEN"
	}

	issues := s.store.ListIssues(repo.ID, stateFilter)

	// Filter by labels
	if labelsParam := r.URL.Query().Get("labels"); labelsParam != "" {
		labelNames := strings.Split(labelsParam, ",")
		var filtered []*Issue
		for _, issue := range issues {
			if issueHasAllLabels(s.store, issue, labelNames, repo.ID) {
				filtered = append(filtered, issue)
			}
		}
		issues = filtered
	}

	// Filter by assignee
	if assignee := r.URL.Query().Get("assignee"); assignee != "" {
		u := s.store.LookupUserByLogin(assignee)
		if u != nil {
			var filtered []*Issue
			for _, issue := range issues {
				for _, aid := range issue.AssigneeIDs {
					if aid == u.ID {
						filtered = append(filtered, issue)
						break
					}
				}
			}
			issues = filtered
		}
	}

	base := s.baseURL(r)
	result := make([]map[string]interface{}, 0, len(issues))
	for _, issue := range issues {
		result = append(result, issueToJSON(issue, s.store, base, repo.FullName))
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, result))
}

func (s *Server) handleGetIssue(w http.ResponseWriter, r *http.Request) {
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

	issue := s.store.GetIssueByNumber(repo.ID, num)
	if issue == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	writeJSON(w, http.StatusOK, issueToJSON(issue, s.store, s.baseURL(r), repo.FullName))
}

func (s *Server) handleUpdateIssue(w http.ResponseWriter, r *http.Request) {
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

	issue := s.store.GetIssueByNumber(repo.ID, num)
	if issue == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	var req map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeGHError(w, http.StatusBadRequest, "Problems parsing JSON")
		return
	}

	s.store.UpdateIssue(issue.ID, func(i *Issue) {
		if v, ok := req["title"].(string); ok {
			i.Title = v
		}
		if v, ok := req["body"].(string); ok {
			i.Body = v
		}
		if v, ok := req["state"].(string); ok {
			switch v {
			case "closed":
				i.State = "CLOSED"
				now := time.Now()
				i.ClosedAt = &now
				if i.StateReason == "" {
					i.StateReason = "COMPLETED"
				}
			case "open":
				i.State = "OPEN"
				i.ClosedAt = nil
				i.StateReason = ""
			}
		}
		if v, ok := req["state_reason"].(string); ok {
			i.StateReason = strings.ToUpper(v)
		}
	})

	updated := s.store.GetIssue(issue.ID)

	if v, ok := req["state"].(string); ok {
		action := "edited"
		if v == "closed" {
			action = "closed"
		} else if v == "open" {
			action = "reopened"
		}
		repoKey := owner + "/" + repoName
		s.emitWebhookEvent(repoKey, "issues", action, buildIssuesPayload(repo, updated, user, action))
	}

	writeJSON(w, http.StatusOK, issueToJSON(updated, s.store, s.baseURL(r), repo.FullName))
}

// --- Comment handlers ---

func (s *Server) handleCreateIssueComment(w http.ResponseWriter, r *http.Request) {
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

	issue := s.store.GetIssueByNumber(repo.ID, num)
	if issue == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	var req struct {
		Body string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeGHError(w, http.StatusBadRequest, "Problems parsing JSON")
		return
	}
	if req.Body == "" {
		writeGHError(w, http.StatusUnprocessableEntity, "Validation Failed")
		return
	}

	comment := s.store.CreateComment(issue.ID, user.ID, req.Body)
	if comment == nil {
		writeGHError(w, http.StatusUnprocessableEntity, "Comment creation failed")
		return
	}

	writeJSON(w, http.StatusCreated, commentToJSON(comment, s.store, s.baseURL(r), repo.FullName, issue.Number))
}

func (s *Server) handleListIssueComments(w http.ResponseWriter, r *http.Request) {
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

	issue := s.store.GetIssueByNumber(repo.ID, num)
	if issue == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	comments := s.store.ListComments(issue.ID)
	base := s.baseURL(r)
	result := make([]map[string]interface{}, 0, len(comments))
	for _, c := range comments {
		result = append(result, commentToJSON(c, s.store, base, repo.FullName, issue.Number))
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, result))
}

// --- Issue label management handlers ---

func (s *Server) handleAddIssueLabels(w http.ResponseWriter, r *http.Request) {
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

	issue := s.store.GetIssueByNumber(repo.ID, num)
	if issue == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	var req struct {
		Labels []string `json:"labels"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeGHError(w, http.StatusBadRequest, "Problems parsing JSON")
		return
	}

	// Resolve label names to IDs before taking write lock
	var newLabelIDs []int
	for _, name := range req.Labels {
		l := s.store.GetLabelByName(repo.ID, name)
		if l != nil {
			newLabelIDs = append(newLabelIDs, l.ID)
		}
	}

	s.store.UpdateIssue(issue.ID, func(i *Issue) {
		for _, lid := range newLabelIDs {
			found := false
			for _, existing := range i.LabelIDs {
				if existing == lid {
					found = true
					break
				}
			}
			if !found {
				i.LabelIDs = append(i.LabelIDs, lid)
			}
		}
	})

	// Return current labels
	updated := s.store.GetIssue(issue.ID)
	base := s.baseURL(r)
	labels := make([]map[string]interface{}, 0)
	for _, lid := range updated.LabelIDs {
		l := s.store.GetLabel(lid)
		if l != nil {
			labels = append(labels, issueLabelToJSON(l, base, repo.FullName))
		}
	}
	writeJSON(w, http.StatusOK, labels)
}

func (s *Server) handleRemoveIssueLabel(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}

	owner := r.PathValue("owner")
	repoName := r.PathValue("repo")
	numStr := r.PathValue("number")
	labelName := r.PathValue("name")
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

	issue := s.store.GetIssueByNumber(repo.ID, num)
	if issue == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	label := s.store.GetLabelByName(repo.ID, labelName)
	if label == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	s.store.UpdateIssue(issue.ID, func(i *Issue) {
		for idx, lid := range i.LabelIDs {
			if lid == label.ID {
				i.LabelIDs = append(i.LabelIDs[:idx], i.LabelIDs[idx+1:]...)
				break
			}
		}
	})

	w.WriteHeader(http.StatusNoContent)
}

// --- JSON converters ---

func issueToJSON(issue *Issue, st *Store, baseURL, repoFullName string) map[string]interface{} {
	// Resolve author
	var authorJSON map[string]interface{}
	st.mu.RLock()
	if u, ok := st.Users[issue.AuthorID]; ok {
		authorJSON = userToJSON(u)
	}

	// Resolve labels
	labels := make([]map[string]interface{}, 0)
	for _, lid := range issue.LabelIDs {
		if l, ok := st.Labels[lid]; ok {
			labels = append(labels, issueLabelToJSON(l, baseURL, repoFullName))
		}
	}

	// Resolve assignees
	assignees := make([]map[string]interface{}, 0)
	for _, aid := range issue.AssigneeIDs {
		if u, ok := st.Users[aid]; ok {
			assignees = append(assignees, userToJSON(u))
		}
	}

	// Resolve milestone
	var milestoneJSON interface{}
	if issue.MilestoneID > 0 {
		if ms, ok := st.Milestones[issue.MilestoneID]; ok {
			milestoneJSON = milestoneToJSON(ms, baseURL, repoFullName)
		}
	}

	// Count comments while holding the lock
	commentCount := 0
	for _, c := range st.Comments {
		if c.IssueID == issue.ID {
			commentCount++
		}
	}
	st.mu.RUnlock()

	// REST uses lowercase state
	state := strings.ToLower(issue.State)

	var closedAt interface{}
	if issue.ClosedAt != nil {
		closedAt = issue.ClosedAt.Format(time.RFC3339)
	}

	numStr := strconv.Itoa(issue.Number)
	return map[string]interface{}{
		"id":               issue.ID,
		"node_id":          issue.NodeID,
		"url":              baseURL + "/api/v3/repos/" + repoFullName + "/issues/" + numStr,
		"html_url":         baseURL + "/" + repoFullName + "/issues/" + numStr,
		"repository_url":   baseURL + "/api/v3/repos/" + repoFullName,
		"number":           issue.Number,
		"title":            issue.Title,
		"body":             issue.Body,
		"state":            state,
		"state_reason":     issue.StateReason,
		"user":             authorJSON,
		"labels":           labels,
		"assignees":        assignees,
		"milestone":        milestoneJSON,
		"locked":           issue.Locked,
		"comments":         commentCount,
		"created_at":       issue.CreatedAt.Format(time.RFC3339),
		"updated_at":       issue.UpdatedAt.Format(time.RFC3339),
		"closed_at":        closedAt,
	}
}

func commentToJSON(c *Comment, st *Store, baseURL, repoFullName string, issueNumber int) map[string]interface{} {
	var authorJSON map[string]interface{}
	st.mu.RLock()
	if u, ok := st.Users[c.AuthorID]; ok {
		authorJSON = userToJSON(u)
	}
	st.mu.RUnlock()

	return map[string]interface{}{
		"id":         c.ID,
		"node_id":    c.NodeID,
		"url":        baseURL + "/api/v3/repos/" + repoFullName + "/issues/comments/" + strconv.Itoa(c.ID),
		"html_url":   baseURL + "/" + repoFullName + "/issues/" + strconv.Itoa(issueNumber) + "#issuecomment-" + strconv.Itoa(c.ID),
		"issue_url":  baseURL + "/api/v3/repos/" + repoFullName + "/issues/" + strconv.Itoa(issueNumber),
		"body":       c.Body,
		"user":       authorJSON,
		"created_at": c.CreatedAt.Format(time.RFC3339),
		"updated_at": c.UpdatedAt.Format(time.RFC3339),
	}
}

// issueHasAllLabels checks if an issue has all the given label names.
func issueHasAllLabels(st *Store, issue *Issue, labelNames []string, repoID int) bool {
	for _, name := range labelNames {
		found := false
		for _, lid := range issue.LabelIDs {
			l := st.GetLabel(lid)
			if l != nil && l.Name == strings.TrimSpace(name) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
