package bleephub

import (
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
	if !decodeJSONBody(w, r, &req) {
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

	s.recordAuditEvent("issues.create", user.Login, "", map[string]interface{}{"repo": repoKey, "issue_id": issue.ID, "title": issue.Title})
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
	if repo.Private && !canReadRepo(s.store, ghUserFromContext(r.Context()), repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	state := r.URL.Query().Get("state")
	if state == "" {
		state = "open"
	}

	// Map REST state to internal state
	var stateFilter string
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
	if repo.Private && !canReadRepo(s.store, ghUserFromContext(r.Context()), repo) {
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
	if !decodeJSONBody(w, r, &req) {
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

	// /issues/{n}/comments routes resolve to either an Issue or a PR by
	// number — real GitHub treats PRs as issues for this endpoint.
	parentType := "issue"
	var parentID, parentNumber int
	var locked bool
	if issue := s.store.GetIssueByNumber(repo.ID, num); issue != nil {
		parentID, parentNumber, locked = issue.ID, issue.Number, issue.Locked
	} else if pr := s.store.GetPullRequestByNumber(repo.ID, num); pr != nil {
		parentType = "pull_request"
		parentID, parentNumber, locked = pr.ID, pr.Number, pr.Locked
	} else {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if locked {
		writeGHError(w, http.StatusForbidden, "Conversation is locked.")
		return
	}

	var req struct {
		Body string `json:"body"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Body == "" {
		writeGHError(w, http.StatusUnprocessableEntity, "Validation Failed")
		return
	}

	comment := s.store.CreateCommentFor(parentType, parentID, user.ID, req.Body)
	if comment == nil {
		writeGHError(w, http.StatusUnprocessableEntity, "Comment creation failed")
		return
	}

	writeJSON(w, http.StatusCreated, commentToJSON(comment, s.store, s.baseURL(r), repo.FullName, parentNumber))
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
	if repo.Private && !canReadRepo(s.store, ghUserFromContext(r.Context()), repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	num, err := strconv.Atoi(numStr)
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	parentType := "issue"
	var parentID, parentNumber int
	if issue := s.store.GetIssueByNumber(repo.ID, num); issue != nil {
		parentID, parentNumber = issue.ID, issue.Number
	} else if pr := s.store.GetPullRequestByNumber(repo.ID, num); pr != nil {
		parentType = "pull_request"
		parentID, parentNumber = pr.ID, pr.Number
	} else {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	comments := s.store.ListCommentsFor(parentType, parentID)
	base := s.baseURL(r)
	result := make([]map[string]interface{}, 0, len(comments))
	for _, c := range comments {
		result = append(result, commentToJSON(c, s.store, base, repo.FullName, parentNumber))
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

	labelNames, ok := decodeIssueLabelsBody(w, r)
	if !ok {
		return
	}

	// Resolve label names to IDs before taking write lock
	var newLabelIDs []int
	for _, name := range labelNames {
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

// --- Repo-level comment handlers ---

func (s *Server) handleListRepoIssueComments(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	repoName := r.PathValue("repo")
	repo := s.store.GetRepo(owner, repoName)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if repo.Private && !canReadRepo(s.store, ghUserFromContext(r.Context()), repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	comments := s.store.ListRepoIssueComments(repo.ID)
	base := s.baseURL(r)
	result := make([]map[string]interface{}, 0, len(comments))
	for _, c := range comments {
		parentNumber := 0
		if issue := s.store.GetIssue(c.IssueID); issue != nil {
			parentNumber = issue.Number
		}
		result = append(result, commentToJSON(c, s.store, base, repo.FullName, parentNumber))
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, result))
}

func (s *Server) handleGetIssueComment(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	repoName := r.PathValue("repo")
	idStr := r.PathValue("comment_id")
	repo := s.store.GetRepo(owner, repoName)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if repo.Private && !canReadRepo(s.store, ghUserFromContext(r.Context()), repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	comment := s.store.GetIssueComment(id)
	if comment == nil || comment.ParentType != "issue" || s.store.GetIssue(comment.IssueID) == nil || s.store.GetIssue(comment.IssueID).RepoID != repo.ID {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	parentNumber := commentParentNumber(s.store, comment)
	writeJSON(w, http.StatusOK, commentToJSON(comment, s.store, s.baseURL(r), repo.FullName, parentNumber))
}

// --- Issue label set/clear handlers ---

func (s *Server) handleSetIssueLabels(w http.ResponseWriter, r *http.Request) {
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

	labelNames, ok := decodeIssueLabelsBody(w, r)
	if !ok {
		return
	}

	var labelIDs []int
	for _, name := range labelNames {
		if l := s.store.GetLabelByName(repo.ID, name); l != nil {
			labelIDs = append(labelIDs, l.ID)
		}
	}

	s.store.SetIssueLabels(repo.ID, issue.Number, labelIDs, user.ID)

	updated := s.store.GetIssue(issue.ID)
	base := s.baseURL(r)
	labels := make([]map[string]interface{}, 0, len(updated.LabelIDs))
	for _, lid := range updated.LabelIDs {
		if l := s.store.GetLabel(lid); l != nil {
			labels = append(labels, issueLabelToJSON(l, base, repo.FullName))
		}
	}
	writeJSON(w, http.StatusOK, labels)
}

func (s *Server) handleClearIssueLabels(w http.ResponseWriter, r *http.Request) {
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

	s.store.ClearIssueLabels(repo.ID, issue.Number, user.ID)
	w.WriteHeader(http.StatusNoContent)
}

// --- Issue assignee handlers ---

func (s *Server) handleAddIssueAssignees(w http.ResponseWriter, r *http.Request) {
	repo, issue, ok := s.resolveRepoIssue(w, r)
	if !ok {
		return
	}
	user := ghUserFromContext(r.Context())

	var req struct {
		Assignees []string `json:"assignees"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}

	assigneeIDs := resolveUserIDs(s.store, req.Assignees)
	s.store.AddIssueAssignees(repo.ID, issue.Number, assigneeIDs, user.ID)
	writeJSON(w, http.StatusOK, issueToJSON(s.store.GetIssue(issue.ID), s.store, s.baseURL(r), repo.FullName))
}

func (s *Server) handleRemoveIssueAssignees(w http.ResponseWriter, r *http.Request) {
	repo, issue, ok := s.resolveRepoIssue(w, r)
	if !ok {
		return
	}
	user := ghUserFromContext(r.Context())

	var req struct {
		Assignees []string `json:"assignees"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}

	assigneeIDs := resolveUserIDs(s.store, req.Assignees)
	s.store.RemoveIssueAssignees(repo.ID, issue.Number, assigneeIDs, user.ID)
	writeJSON(w, http.StatusOK, issueToJSON(s.store.GetIssue(issue.ID), s.store, s.baseURL(r), repo.FullName))
}

// --- Comment pin handlers ---

func (s *Server) handlePinIssueComment(w http.ResponseWriter, r *http.Request) {
	repo, comment, ok := s.resolveRepoIssueComment(w, r)
	if !ok {
		return
	}

	s.store.PinIssueComment(comment.ID)
	parentNumber := commentParentNumber(s.store, comment)
	writeJSON(w, http.StatusOK, commentToJSON(s.store.GetIssueComment(comment.ID), s.store, s.baseURL(r), repo.FullName, parentNumber))
}

func (s *Server) handleUnpinIssueComment(w http.ResponseWriter, r *http.Request) {
	repo, comment, ok := s.resolveRepoIssueComment(w, r)
	if !ok {
		return
	}

	s.store.UnpinIssueComment(comment.ID)
	parentNumber := commentParentNumber(s.store, comment)
	writeJSON(w, http.StatusOK, commentToJSON(s.store.GetIssueComment(comment.ID), s.store, s.baseURL(r), repo.FullName, parentNumber))
}

// resolveRepoIssue resolves owner/repo/{number} and returns the repo + issue,
// writing the appropriate error response on failure.
func (s *Server) resolveRepoIssue(w http.ResponseWriter, r *http.Request) (*Repo, *Issue, bool) {
	owner := r.PathValue("owner")
	repoName := r.PathValue("repo")
	numStr := r.PathValue("number")
	repo := s.store.GetRepo(owner, repoName)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, nil, false
	}

	num, err := strconv.Atoi(numStr)
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, nil, false
	}

	issue := s.store.GetIssueByNumber(repo.ID, num)
	if issue == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, nil, false
	}
	return repo, issue, true
}

// resolveRepoIssueComment resolves owner/repo/{comment_id} and returns the repo
// + issue comment, writing the appropriate error response on failure.
func (s *Server) resolveRepoIssueComment(w http.ResponseWriter, r *http.Request) (*Repo, *Comment, bool) {
	owner := r.PathValue("owner")
	repoName := r.PathValue("repo")
	idStr := r.PathValue("comment_id")
	repo := s.store.GetRepo(owner, repoName)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, nil, false
	}

	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, nil, false
	}

	comment := s.store.GetIssueComment(id)
	if comment == nil || comment.ParentType != "issue" {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, nil, false
	}
	issue := s.store.GetIssue(comment.IssueID)
	if issue == nil || issue.RepoID != repo.ID {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, nil, false
	}
	return repo, comment, true
}

func resolveUserIDs(st *Store, logins []string) []int {
	var ids []int
	for _, login := range logins {
		if u := st.LookupUserByLogin(login); u != nil {
			ids = append(ids, u.ID)
		}
	}
	return ids
}

// --- Issue timeline + events handlers ---

func (s *Server) handleListIssueTimeline(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	repoName := r.PathValue("repo")
	numStr := r.PathValue("number")
	repo := s.store.GetRepo(owner, repoName)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if repo.Private && !canReadRepo(s.store, ghUserFromContext(r.Context()), repo) {
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

	timeline := s.store.BuildIssueTimeline(repo, issue.ID, s.baseURL(r))
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, timeline))
}

func (s *Server) handleListIssueEvents(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	repoName := r.PathValue("repo")
	numStr := r.PathValue("number")
	repo := s.store.GetRepo(owner, repoName)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if repo.Private && !canReadRepo(s.store, ghUserFromContext(r.Context()), repo) {
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

	events := s.store.ListIssueEvents(repo.ID, issue.ID)
	base := s.baseURL(r)
	result := make([]map[string]interface{}, 0, len(events))
	for _, e := range events {
		result = append(result, issueEventForIssueToJSON(e, s.store, base, repo.FullName))
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, result))
}

func (s *Server) handleListRepoIssueEvents(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	repoName := r.PathValue("repo")
	repo := s.store.GetRepo(owner, repoName)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if repo.Private && !canReadRepo(s.store, ghUserFromContext(r.Context()), repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	events := s.store.ListRepoIssueEvents(repo.ID)
	base := s.baseURL(r)
	result := make([]map[string]interface{}, 0, len(events))
	for _, e := range events {
		result = append(result, issueEventToJSON(e, s.store, base, repo.FullName))
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, result))
}

func (s *Server) handleGetIssueEvent(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	repoName := r.PathValue("repo")
	idStr := r.PathValue("event_id")
	repo := s.store.GetRepo(owner, repoName)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if repo.Private && !canReadRepo(s.store, ghUserFromContext(r.Context()), repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	event := s.store.GetIssueEvent(id)
	if event == nil || event.RepoID != repo.ID {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	writeJSON(w, http.StatusOK, issueEventToJSON(event, s.store, s.baseURL(r), repo.FullName))
}

// --- Sub-issues / dependencies / issue-field-values stubs ---

func (s *Server) handleListSubIssues(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, []map[string]interface{}{})
}

func (s *Server) handleCreateSubIssue(w http.ResponseWriter, r *http.Request) {
	writeGHError(w, http.StatusUnprocessableEntity, "Sub-issues are not enabled for this repository")
}

func (s *Server) handleRemoveSubIssue(w http.ResponseWriter, r *http.Request) {
	writeGHError(w, http.StatusUnprocessableEntity, "Sub-issues are not enabled for this repository")
}

func (s *Server) handleListIssueDependenciesBlockedBy(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, []map[string]interface{}{})
}

func (s *Server) handleListIssueFieldValues(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, []map[string]interface{}{})
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

	// Grab the milestone pointer; conversion happens after unlock because
	// milestoneToJSON derives issue counts under its own lock.
	var milestone *Milestone
	if issue.MilestoneID > 0 {
		milestone = st.Milestones[issue.MilestoneID]
	}

	// Count comments while holding the lock
	commentCount := 0
	for _, c := range st.Comments {
		if c.ParentType == "issue" && c.IssueID == issue.ID {
			commentCount++
		}
	}
	st.mu.RUnlock()

	var milestoneJSON interface{}
	if milestone != nil {
		milestoneJSON = milestoneToJSON(milestone, st, baseURL, repoFullName)
	}

	// GitHub's assignee is the first assignee, null when unassigned.
	var assignee interface{}
	if len(assignees) > 0 {
		assignee = assignees[0]
	}

	// REST uses lowercase state
	state := strings.ToLower(issue.State)

	var closedAt interface{}
	if issue.ClosedAt != nil {
		closedAt = issue.ClosedAt.Format(time.RFC3339)
	}

	var activeLockReason interface{}
	if issue.Locked {
		activeLockReason = issue.ActiveLockReason
	}

	numStr := strconv.Itoa(issue.Number)
	api := baseURL + "/api/v3/repos/" + repoFullName + "/issues/" + numStr
	return map[string]interface{}{
		"id":                 issue.ID,
		"node_id":            issue.NodeID,
		"url":                api,
		"html_url":           baseURL + "/" + repoFullName + "/issues/" + numStr,
		"repository_url":     baseURL + "/api/v3/repos/" + repoFullName,
		"comments_url":       api + "/comments",
		"events_url":         api + "/events",
		"labels_url":         api + "/labels{/name}",
		"number":             issue.Number,
		"title":              issue.Title,
		"body":               issue.Body,
		"state":              state,
		"state_reason":       issue.StateReason,
		"user":               authorJSON,
		"labels":             labels,
		"assignee":           assignee,
		"assignees":          assignees,
		"milestone":          milestoneJSON,
		"locked":             issue.Locked,
		"active_lock_reason": activeLockReason,
		"comments":           commentCount,
		"created_at":         issue.CreatedAt.Format(time.RFC3339),
		"updated_at":         issue.UpdatedAt.Format(time.RFC3339),
		"closed_at":          closedAt,
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

// timelineCommentToJSON renders a comment as it appears inside an issue
// timeline, including the "event": "commented" discriminator.
func timelineCommentToJSON(c *Comment, st *Store, baseURL, repoFullName string, issueNumber int, repo *Repo) map[string]interface{} {
	out := commentToJSON(c, st, baseURL, repoFullName, issueNumber)
	out["event"] = "commented"
	out["actor"] = out["user"]
	out["author_association"] = authorAssociationForComment(st, c, repo)
	out["performed_via_github_app"] = nil
	return out
}

// issueEventBase returns the common fields shared by every issue-event
// response shape.
func issueEventBase(e *IssueEvent, st *Store, baseURL, repoFullName string) map[string]interface{} {
	st.mu.RLock()
	var actorJSON map[string]interface{}
	if u, ok := st.Users[e.ActorID]; ok {
		actorJSON = userToJSON(u)
	}
	st.mu.RUnlock()

	var commitID interface{}
	if e.CommitID != "" {
		commitID = e.CommitID
	}
	var commitURL interface{}
	if e.CommitURL != "" {
		commitURL = e.CommitURL
	} else if e.CommitID != "" {
		commitURL = baseURL + "/api/v3/repos/" + repoFullName + "/commits/" + e.CommitID
	}

	return map[string]interface{}{
		"id":         e.ID,
		"node_id":    e.NodeID,
		"url":        baseURL + "/api/v3/repos/" + repoFullName + "/issues/events/" + strconv.Itoa(e.ID),
		"actor":      actorJSON,
		"event":      e.Event,
		"commit_id":  commitID,
		"commit_url": commitURL,
		"created_at": e.CreatedAt.Format(time.RFC3339),
	}
}

// issueEventLabelToJSON returns the slim label shape used inside issue
// events (name + color only).
func issueEventLabelToJSON(l *IssueLabel) map[string]interface{} {
	return map[string]interface{}{
		"name":  l.Name,
		"color": l.Color,
	}
}

// issueEventMilestoneToJSON returns the slim milestone shape used inside
// issue events (title only).
func issueEventMilestoneToJSON(ms *Milestone) map[string]interface{} {
	return map[string]interface{}{
		"title": ms.Title,
	}
}

// issueEventToJSON renders an IssueEvent to the repo-level GitHub
// issue-event shape.
func issueEventToJSON(e *IssueEvent, st *Store, baseURL, repoFullName string) map[string]interface{} {
	st.mu.RLock()
	var labelJSON interface{}
	if l, ok := st.Labels[e.LabelID]; ok {
		labelJSON = issueEventLabelToJSON(l)
	}
	var assigneeJSON interface{}
	if u, ok := st.Users[e.AssigneeID]; ok {
		assigneeJSON = userToJSON(u)
	}
	var assignerJSON interface{}
	if u, ok := st.Users[e.AssignerID]; ok {
		assignerJSON = userToJSON(u)
	}
	var milestoneJSON interface{}
	if ms, ok := st.Milestones[e.MilestoneID]; ok {
		milestoneJSON = issueEventMilestoneToJSON(ms)
	}
	st.mu.RUnlock()

	out := issueEventBase(e, st, baseURL, repoFullName)
	out["performed_via_github_app"] = nil
	out["label"] = labelJSON
	out["assignee"] = assigneeJSON
	out["assigner"] = assignerJSON
	out["milestone"] = milestoneJSON
	return out
}

// issueEventForIssueToJSON renders an IssueEvent to the per-issue
// issue-event-for-issue shape, which is a discriminated union of specific
// event schemas rather than a generic object.
func issueEventForIssueToJSON(e *IssueEvent, st *Store, baseURL, repoFullName string) map[string]interface{} {
	out := issueEventBase(e, st, baseURL, repoFullName)
	out["performed_via_github_app"] = nil

	switch e.Event {
	case "labeled", "unlabeled":
		st.mu.RLock()
		var labelJSON interface{}
		if l, ok := st.Labels[e.LabelID]; ok {
			labelJSON = issueEventLabelToJSON(l)
		}
		st.mu.RUnlock()
		out["label"] = labelJSON
	case "assigned", "unassigned":
		st.mu.RLock()
		var assigneeJSON, assignerJSON interface{}
		if u, ok := st.Users[e.AssigneeID]; ok {
			assigneeJSON = userToJSON(u)
		}
		if u, ok := st.Users[e.AssignerID]; ok {
			assignerJSON = userToJSON(u)
		}
		st.mu.RUnlock()
		out["assignee"] = assigneeJSON
		out["assigner"] = assignerJSON
	case "milestoned", "demilestoned":
		st.mu.RLock()
		var milestoneJSON interface{}
		if ms, ok := st.Milestones[e.MilestoneID]; ok {
			milestoneJSON = issueEventMilestoneToJSON(ms)
		}
		st.mu.RUnlock()
		out["milestone"] = milestoneJSON
	case "renamed":
		out["rename"] = map[string]interface{}{
			"from": e.RenameFrom,
			"to":   e.RenameTo,
		}
	default:
		// opened, closed, reopened, locked, unlocked, etc. map to the
		// locked-issue-event schema which only adds a nullable lock_reason.
		lockReason := interface{}(nil)
		if e.Event == "locked" && e.LockReason != "" {
			lockReason = e.LockReason
		}
		out["lock_reason"] = lockReason
	}
	return out
}

// issueEventForTimelineToJSON renders an IssueEvent to the timeline-event
// shape (timeline-issue-events union).
func issueEventForTimelineToJSON(e *IssueEvent, st *Store, baseURL, repoFullName string) map[string]interface{} {
	out := issueEventBase(e, st, baseURL, repoFullName)
	out["performed_via_github_app"] = nil

	switch e.Event {
	case "labeled", "unlabeled":
		st.mu.RLock()
		var labelJSON interface{}
		if l, ok := st.Labels[e.LabelID]; ok {
			labelJSON = issueEventLabelToJSON(l)
		}
		st.mu.RUnlock()
		out["label"] = labelJSON
	case "assigned", "unassigned":
		st.mu.RLock()
		var assigneeJSON interface{}
		if u, ok := st.Users[e.AssigneeID]; ok {
			assigneeJSON = userToJSON(u)
		}
		st.mu.RUnlock()
		out["assignee"] = assigneeJSON
	case "milestoned", "demilestoned":
		st.mu.RLock()
		var milestoneJSON interface{}
		if ms, ok := st.Milestones[e.MilestoneID]; ok {
			milestoneJSON = issueEventMilestoneToJSON(ms)
		}
		st.mu.RUnlock()
		out["milestone"] = milestoneJSON
	case "renamed":
		out["rename"] = map[string]interface{}{
			"from": e.RenameFrom,
			"to":   e.RenameTo,
		}
	case "locked", "unlocked":
		lockReason := interface{}(nil)
		if e.Event == "locked" && e.LockReason != "" {
			lockReason = e.LockReason
		}
		out["lock_reason"] = lockReason
	default:
		// opened, closed, reopened, etc. map to state-change-issue-event.
		out["state_reason"] = nil
	}
	return out
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
