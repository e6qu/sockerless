package bleephub

import (
	"net/http"
	"sort"
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
		Title       string   `json:"title"`
		Body        string   `json:"body"`
		Labels      []string `json:"labels"`
		Assignees   []string `json:"assignees"`
		Milestone   int      `json:"milestone"` // milestone number
		IssueTypeID *int     `json:"issue_type_id"`
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

	var issueTypeID int
	if req.IssueTypeID != nil && *req.IssueTypeID > 0 {
		it := s.store.GetAssignableIssueTypeForRepo(repo, *req.IssueTypeID)
		if it == nil {
			writeGHValidationError(w, "Issue", "issue_type_id", "invalid")
			return
		}
		issueTypeID = it.ID
	}

	issue := s.store.CreateIssue(repo.ID, user.ID, req.Title, req.Body, labelIDs, assigneeIDs, milestoneID)
	if issue == nil {
		writeGHError(w, http.StatusUnprocessableEntity, "Issue creation failed")
		return
	}
	if issueTypeID > 0 {
		s.store.UpdateIssue(issue.ID, func(i *Issue) {
			i.IssueTypeID = issueTypeID
		})
		issue = s.store.GetIssue(issue.ID)
	}
	repoKey := owner + "/" + name
	s.emitWebhookEvent(repoKey, "issues", "opened", buildIssuesPayload(s.store, repo, issue, user, "opened"))

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

	// Resolve milestone, labels, and assignees before taking the write
	// lock so an invalid milestone number 422s without mutating anything.
	// An explicit `"milestone": null` clears the milestone; an absent
	// member keeps it — the map lookup distinguishes the two.
	var milestoneID *int
	if v, present := req["milestone"]; present {
		switch mv := v.(type) {
		case nil:
			cleared := 0
			milestoneID = &cleared
		case float64:
			ms := s.store.GetMilestoneByNumber(repo.ID, int(mv))
			if ms == nil {
				writeGHValidationError(w, "Issue", "milestone", "invalid")
				return
			}
			milestoneID = &ms.ID
		default:
			writeGHValidationError(w, "Issue", "milestone", "invalid")
			return
		}
	}
	var labelIDs *[]int
	if v, present := req["labels"]; present {
		entries, ok := v.([]interface{})
		if !ok {
			writeGHValidationError(w, "Issue", "labels", "invalid")
			return
		}
		ids := make([]int, 0, len(entries))
		for _, entry := range entries {
			// The documented body allows bare strings or {"name": ...}
			// objects; unknown label names are dropped, matching the
			// add-labels endpoint's semantics.
			name, ok := entry.(string)
			if !ok {
				obj, isObj := entry.(map[string]interface{})
				if !isObj {
					writeGHValidationError(w, "Issue", "labels", "invalid")
					return
				}
				if name, ok = obj["name"].(string); !ok {
					writeGHValidationError(w, "Issue", "labels", "invalid")
					return
				}
			}
			if l := s.store.GetLabelByName(repo.ID, name); l != nil {
				ids = append(ids, l.ID)
			}
		}
		labelIDs = &ids
	}
	var assigneeIDs *[]int
	if v, present := req["assignees"]; present {
		entries, ok := v.([]interface{})
		if !ok {
			writeGHValidationError(w, "Issue", "assignees", "invalid")
			return
		}
		ids := make([]int, 0, len(entries))
		for _, entry := range entries {
			login, ok := entry.(string)
			if !ok {
				writeGHValidationError(w, "Issue", "assignees", "invalid")
				return
			}
			if u := s.store.LookupUserByLogin(login); u != nil {
				ids = append(ids, u.ID)
			}
		}
		assigneeIDs = &ids
	}
	var issueTypeID *int
	if v, present := req["issue_type_id"]; present {
		switch tv := v.(type) {
		case nil:
			cleared := 0
			issueTypeID = &cleared
		case float64:
			if tv <= 0 {
				cleared := 0
				issueTypeID = &cleared
				break
			}
			it := s.store.GetAssignableIssueTypeForRepo(repo, int(tv))
			if it == nil {
				writeGHValidationError(w, "Issue", "issue_type_id", "invalid")
				return
			}
			resolved := it.ID
			issueTypeID = &resolved
		default:
			writeGHValidationError(w, "Issue", "issue_type_id", "invalid")
			return
		}
	}

	s.store.UpdateIssue(issue.ID, func(i *Issue) {
		if v, ok := req["title"].(string); ok {
			i.Title = v
		}
		if v, ok := req["body"].(string); ok {
			i.Body = v
		}
		if milestoneID != nil {
			i.MilestoneID = *milestoneID
		}
		if labelIDs != nil {
			i.LabelIDs = *labelIDs
		}
		if assigneeIDs != nil {
			i.AssigneeIDs = *assigneeIDs
		}
		if issueTypeID != nil {
			i.IssueTypeID = *issueTypeID
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
		s.emitWebhookEvent(repoKey, "issues", action, buildIssuesPayload(s.store, repo, updated, user, action))
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
	// number — real GitHub treats PRs as issues for this endpoint. The
	// resolver reads the mutable Locked flag under the store lock.
	parentType, parentID, parentNumber, locked, found := s.store.ResolveCommentParent(repo.ID, num)
	if !found {
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

// labelIDsToJSON resolves a slice of label IDs into GitHub label JSON, in the
// stored order, skipping any that no longer resolve.
func (s *Server) labelIDsToJSON(labelIDs []int, base, repoFullName string) []map[string]interface{} {
	labels := make([]map[string]interface{}, 0, len(labelIDs))
	for _, lid := range labelIDs {
		if l := s.store.GetLabel(lid); l != nil {
			labels = append(labels, issueLabelToJSON(l, base, repoFullName))
		}
	}
	return labels
}

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
	pr := (*PullRequest)(nil)
	if issue == nil {
		if pr = s.store.GetPullRequestByNumber(repo.ID, num); pr == nil {
			writeGHError(w, http.StatusNotFound, "Not Found")
			return
		}
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

	base := s.baseURL(r)
	if pr != nil {
		// Pull requests carry labels through the same surface real GitHub
		// exposes; PRs share the issue number space.
		s.store.AddPullRequestLabels(repo.ID, pr.Number, newLabelIDs, user.ID)
		updated := s.store.GetPullRequestByNumber(repo.ID, pr.Number)
		writeJSON(w, http.StatusOK, s.labelIDsToJSON(updated.LabelIDs, base, repo.FullName))
		return
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
	writeJSON(w, http.StatusOK, s.labelIDsToJSON(updated.LabelIDs, base, repo.FullName))
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
	pr := (*PullRequest)(nil)
	if issue == nil {
		if pr = s.store.GetPullRequestByNumber(repo.ID, num); pr == nil {
			writeGHError(w, http.StatusNotFound, "Not Found")
			return
		}
	}

	label := s.store.GetLabelByName(repo.ID, labelName)
	if label == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	if pr != nil {
		s.store.RemovePullRequestLabel(repo.ID, pr.Number, label.ID, user.ID)
		w.WriteHeader(http.StatusNoContent)
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
	pr := (*PullRequest)(nil)
	if issue == nil {
		if pr = s.store.GetPullRequestByNumber(repo.ID, num); pr == nil {
			writeGHError(w, http.StatusNotFound, "Not Found")
			return
		}
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

	base := s.baseURL(r)
	if pr != nil {
		s.store.SetPullRequestLabels(repo.ID, pr.Number, labelIDs, user.ID)
		updated := s.store.GetPullRequestByNumber(repo.ID, pr.Number)
		writeJSON(w, http.StatusOK, s.labelIDsToJSON(updated.LabelIDs, base, repo.FullName))
		return
	}

	s.store.SetIssueLabels(repo.ID, issue.Number, labelIDs, user.ID)

	updated := s.store.GetIssue(issue.ID)
	writeJSON(w, http.StatusOK, s.labelIDsToJSON(updated.LabelIDs, base, repo.FullName))
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
		if pr := s.store.GetPullRequestByNumber(repo.ID, num); pr != nil {
			s.store.ClearPullRequestLabels(repo.ID, pr.Number, user.ID)
			w.WriteHeader(http.StatusNoContent)
			return
		}
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
	// Real GitHub responds 201 Created when adding assignees.
	writeJSON(w, http.StatusCreated, issueToJSON(s.store.GetIssue(issue.ID), s.store, s.baseURL(r), repo.FullName))
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

	// Pull requests share the issue number space and are timeline-capable
	// on real GitHub; resolve the number to whichever exists.
	if issue := s.store.GetIssueByNumber(repo.ID, num); issue != nil {
		timeline := s.store.BuildIssueTimeline(repo, issue.ID, s.baseURL(r))
		writeJSON(w, http.StatusOK, paginateAndLink(w, r, timeline))
		return
	}
	pr := s.store.GetPullRequestByNumber(repo.ID, num)
	if pr == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	timeline, err := s.buildPullRequestTimeline(repo, pr, s.baseURL(r))
	if err != nil {
		writeGHError(w, http.StatusInternalServerError, "timeline derivation failed")
		return
	}
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

	// Pull requests share the issue number space; their events serve
	// through this endpoint too, as on real GitHub.
	var events []*IssueEvent
	if issue := s.store.GetIssueByNumber(repo.ID, num); issue != nil {
		events = s.store.ListIssueEvents(repo.ID, issue.ID)
	} else if pr := s.store.GetPullRequestByNumber(repo.ID, num); pr != nil {
		events = s.store.ListPullRequestEvents(repo.ID, pr.ID)
	} else {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

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

// --- JSON converters ---

func issueToJSON(issue *Issue, st *Store, baseURL, repoFullName string) map[string]interface{} {
	// Every mutable field of *issue is read under the store read lock and
	// captured into locals here: UpdateIssue / SetIssueOrPRLock mutate these
	// fields under st.mu.Lock, so reading them after RUnlock (title, body,
	// state, lock flags, timestamps) would race a concurrent writer.
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
	// Count comments via the maintained index while holding the lock.
	commentCount := st.countCommentsForLocked("issue", issue.ID)

	// Snapshot the mutable scalar fields before releasing the lock.
	issueID := issue.ID
	issueNumber := issue.Number
	nodeID := issue.NodeID
	title := issue.Title
	body := issue.Body
	rawState := issue.State
	stateReason := issue.StateReason
	locked := issue.Locked
	activeLockReason := issue.ActiveLockReason
	createdAt := issue.CreatedAt
	updatedAt := issue.UpdatedAt
	var closedAtCopy *time.Time
	if issue.ClosedAt != nil {
		c := *issue.ClosedAt
		closedAtCopy = &c
	}
	st.mu.RUnlock()

	reactions := st.Reactions.SummarizeReactions("issue", issueID)
	reactions["url"] = baseURL + "/api/v3/repos/" + repoFullName + "/issues/" + strconv.Itoa(issueNumber) + "/reactions"

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
	state := strings.ToLower(rawState)

	var closedAt interface{}
	if closedAtCopy != nil {
		closedAt = closedAtCopy.Format(time.RFC3339)
	}

	var activeLockReasonJSON interface{}
	if locked {
		activeLockReasonJSON = activeLockReason
	}

	numStr := strconv.Itoa(issueNumber)
	api := baseURL + "/api/v3/repos/" + repoFullName + "/issues/" + numStr
	return map[string]interface{}{
		"id":                 issueID,
		"node_id":            nodeID,
		"url":                api,
		"html_url":           baseURL + "/" + repoFullName + "/issues/" + numStr,
		"repository_url":     baseURL + "/api/v3/repos/" + repoFullName,
		"comments_url":       api + "/comments",
		"events_url":         api + "/events",
		"labels_url":         api + "/labels{/name}",
		"number":             issueNumber,
		"title":              title,
		"body":               body,
		"state":              state,
		"state_reason":       stateReason,
		"user":               authorJSON,
		"labels":             labels,
		"assignee":           assignee,
		"assignees":          assignees,
		"milestone":          milestoneJSON,
		"locked":             locked,
		"active_lock_reason": activeLockReasonJSON,
		"comments":           commentCount,
		"created_at":         createdAt.Format(time.RFC3339),
		"updated_at":         updatedAt.Format(time.RFC3339),
		"closed_at":          closedAt,
		"reactions":          reactions,
	}
}

func commentToJSON(c *Comment, st *Store, baseURL, repoFullName string, issueNumber int) map[string]interface{} {
	var authorJSON map[string]interface{}
	st.mu.RLock()
	if u, ok := st.Users[c.AuthorID]; ok {
		authorJSON = userToJSON(u)
	}
	st.mu.RUnlock()

	reactions := st.Reactions.SummarizeReactions("issue_comment", c.ID)
	reactions["url"] = baseURL + "/api/v3/repos/" + repoFullName + "/issues/comments/" + strconv.Itoa(c.ID) + "/reactions"

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
		"reactions":  reactions,
	}
}

// timelineCommentToJSON renders a comment as it appears inside an issue
// timeline, including the "event": "commented" discriminator.
func timelineCommentToJSON(c *Comment, st *Store, baseURL, repoFullName string, issueNumber int, repo *Repo) map[string]interface{} {
	out := commentToJSON(c, st, baseURL, repoFullName, issueNumber)
	out["event"] = "commented"
	out["actor"] = out["user"]
	out["author_association"] = authorAssociation(st, c.AuthorID, repo)
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
	case "review_requested", "review_request_removed":
		st.mu.RLock()
		var requesterJSON, reviewerJSON interface{}
		if u, ok := st.Users[e.ActorID]; ok {
			requesterJSON = userToJSON(u)
		}
		if u, ok := st.Users[e.RequestedReviewerID]; ok {
			reviewerJSON = userToJSON(u)
		}
		st.mu.RUnlock()
		// GitHub's actor on review-request events is the requester.
		out["review_requester"] = requesterJSON
		out["requested_reviewer"] = reviewerJSON
	default:
		// opened, closed, reopened, merged, locked, unlocked, etc. map to
		// the locked-issue-event schema which only adds a nullable
		// lock_reason.
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
	case "review_requested", "review_request_removed":
		st.mu.RLock()
		var requesterJSON, reviewerJSON interface{}
		if u, ok := st.Users[e.ActorID]; ok {
			requesterJSON = userToJSON(u)
		}
		if u, ok := st.Users[e.RequestedReviewerID]; ok {
			reviewerJSON = userToJSON(u)
		}
		st.mu.RUnlock()
		// GitHub's actor on review-request events is the requester.
		out["review_requester"] = requesterJSON
		out["requested_reviewer"] = reviewerJSON
	default:
		// opened, closed, reopened, merged, etc. map to
		// state-change-issue-event.
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

// handleListOrgIssues implements GET /orgs/{org}/issues: issues across the
// organization's repositories that involve the authenticated user, selected
// by the `filter` parameter exactly as on real GitHub (assigned is the
// default; `repos`/`all` widen to every org-repo issue the user can see).
func (s *Server) handleListOrgIssues(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	org := s.store.GetOrg(r.PathValue("org"))
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	q := r.URL.Query()
	filter := q.Get("filter")
	if filter == "" {
		filter = "assigned"
	}
	stateFilter := q.Get("state")
	if stateFilter == "" {
		stateFilter = "open"
	}
	var since time.Time
	if v := q.Get("since"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeGHValidationError(w, "Issue", "since", "invalid")
			return
		}
		since = t
	}
	var labelNames []string
	if v := q.Get("labels"); v != "" {
		labelNames = strings.Split(v, ",")
	}

	// Gather the org's issues under the read lock, render outside it.
	s.store.mu.RLock()
	orgRepos := map[int]*Repo{}
	for _, repo := range s.store.Repos {
		if repo.OwnerType == "Organization" && repo.OwnerID == org.ID {
			orgRepos[repo.ID] = repo
		}
	}
	type issueRow struct {
		issue *Issue
		repo  *Repo
	}
	commentedIssueIDs := map[int]bool{}
	for _, c := range s.store.Comments {
		if c.ParentType == "issue" && c.AuthorID == user.ID {
			commentedIssueIDs[c.IssueID] = true
		}
	}
	var rows []issueRow
	for _, issue := range s.store.Issues {
		repo := orgRepos[issue.RepoID]
		if repo == nil {
			continue
		}
		switch stateFilter {
		case "open":
			if issue.State != "OPEN" {
				continue
			}
		case "closed":
			if issue.State != "CLOSED" {
				continue
			}
		}
		assigned := false
		for _, aid := range issue.AssigneeIDs {
			if aid == user.ID {
				assigned = true
				break
			}
		}
		switch filter {
		case "assigned":
			if !assigned {
				continue
			}
		case "created":
			if issue.AuthorID != user.ID {
				continue
			}
		case "mentioned":
			if !strings.Contains(issue.Body, "@"+user.Login) {
				continue
			}
		case "subscribed":
			// Participation auto-subscribes on real GitHub: authored,
			// assigned, or commented issues.
			if issue.AuthorID != user.ID && !assigned && !commentedIssueIDs[issue.ID] {
				continue
			}
		case "repos", "all":
			// Every issue across the org's repositories.
		default:
			continue
		}
		if !since.IsZero() && issue.UpdatedAt.Before(since) {
			continue
		}
		rows = append(rows, issueRow{issue: issue, repo: repo})
	}
	s.store.mu.RUnlock()

	if len(labelNames) > 0 {
		kept := rows[:0]
		for _, row := range rows {
			if issueHasAllLabels(s.store, row.issue, labelNames, row.repo.ID) {
				kept = append(kept, row)
			}
		}
		rows = kept
	}
	// Private repos the caller cannot read never surface.
	readable := rows[:0]
	for _, row := range rows {
		if canReadRepo(s.store, user, row.repo) {
			readable = append(readable, row)
		}
	}
	rows = readable

	sortKey := q.Get("sort")
	asc := q.Get("direction") == "asc"
	sort.SliceStable(rows, func(i, j int) bool {
		var before bool
		switch sortKey {
		case "updated":
			before = rows[i].issue.UpdatedAt.Before(rows[j].issue.UpdatedAt)
		case "comments":
			before = rows[i].issue.ID < rows[j].issue.ID
		default: // created
			before = rows[i].issue.CreatedAt.Before(rows[j].issue.CreatedAt)
		}
		if asc {
			return before
		}
		return !before
	})

	base := s.baseURL(r)
	out := make([]map[string]interface{}, 0, len(rows))
	for _, row := range rows {
		issueJSON := issueToJSON(row.issue, s.store, base, row.repo.FullName)
		issueJSON["repository"] = repoToJSON(row.repo, s.store, base)
		out = append(out, issueJSON)
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, out))
}
