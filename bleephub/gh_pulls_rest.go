package bleephub

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) registerGHPullRoutes() {
	s.mux.HandleFunc("POST /api/v3/repos/{owner}/{repo}/pulls", s.handleCreatePullRequest)
	s.mux.HandleFunc("GET /api/v3/repos/{owner}/{repo}/pulls", s.handleListPullRequests)
	s.mux.HandleFunc("GET /api/v3/repos/{owner}/{repo}/pulls/{number}", s.handleGetPullRequest)
	s.mux.HandleFunc("PATCH /api/v3/repos/{owner}/{repo}/pulls/{number}", s.handleUpdatePullRequest)
	s.mux.HandleFunc("PUT /api/v3/repos/{owner}/{repo}/pulls/{number}/merge", s.handleMergePullRequest)
	s.mux.HandleFunc("POST /api/v3/repos/{owner}/{repo}/pulls/{number}/reviews", s.handleCreatePRReview)
	s.mux.HandleFunc("GET /api/v3/repos/{owner}/{repo}/pulls/{number}/reviews", s.handleListPRReviews)
	s.mux.HandleFunc("POST /api/v3/repos/{owner}/{repo}/pulls/{number}/requested_reviewers", s.handleRequestReviewers)
}

func (s *Server) handleCreatePullRequest(w http.ResponseWriter, r *http.Request) {
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
		Title string `json:"title"`
		Body  string `json:"body"`
		Head  string `json:"head"`
		Base  string `json:"base"`
		Draft bool   `json:"draft"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeGHError(w, http.StatusBadRequest, "Problems parsing JSON")
		return
	}
	if req.Head == "" {
		writeGHValidationError(w, "PullRequest", "head", "missing_field")
		return
	}

	pr := s.store.CreatePullRequest(repo.ID, user.ID, req.Title, req.Body, req.Head, req.Base, req.Draft, nil, nil, 0)
	if pr == nil {
		writeGHError(w, http.StatusUnprocessableEntity, "Pull request creation failed")
		return
	}

	writeJSON(w, http.StatusCreated, pullRequestToJSON(pr, s.store, s.baseURL(r), repo.FullName))
}

func (s *Server) handleListPullRequests(w http.ResponseWriter, r *http.Request) {
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

	prs := s.store.ListPullRequests(repo.ID, stateFilter)

	// Filter by head
	if head := r.URL.Query().Get("head"); head != "" {
		// head can be "owner:branch" or just "branch"
		branch := head
		if idx := strings.Index(head, ":"); idx >= 0 {
			branch = head[idx+1:]
		}
		var filtered []*PullRequest
		for _, pr := range prs {
			if pr.HeadRefName == branch {
				filtered = append(filtered, pr)
			}
		}
		prs = filtered
	}

	// Filter by base
	if base := r.URL.Query().Get("base"); base != "" {
		var filtered []*PullRequest
		for _, pr := range prs {
			if pr.BaseRefName == base {
				filtered = append(filtered, pr)
			}
		}
		prs = filtered
	}

	base := s.baseURL(r)
	result := make([]map[string]interface{}, 0, len(prs))
	for _, pr := range prs {
		result = append(result, pullRequestToJSON(pr, s.store, base, repo.FullName))
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, result))
}

func (s *Server) handleGetPullRequest(w http.ResponseWriter, r *http.Request) {
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

	pr := s.store.GetPullRequestByNumber(repo.ID, num)
	if pr == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	writeJSON(w, http.StatusOK, pullRequestToJSON(pr, s.store, s.baseURL(r), repo.FullName))
}

func (s *Server) handleUpdatePullRequest(w http.ResponseWriter, r *http.Request) {
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

	pr := s.store.GetPullRequestByNumber(repo.ID, num)
	if pr == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	var req map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeGHError(w, http.StatusBadRequest, "Problems parsing JSON")
		return
	}

	s.store.UpdatePullRequest(pr.ID, func(p *PullRequest) {
		if v, ok := req["title"].(string); ok {
			p.Title = v
		}
		if v, ok := req["body"].(string); ok {
			p.Body = v
		}
		if v, ok := req["base"].(string); ok {
			p.BaseRefName = v
		}
		if v, ok := req["state"].(string); ok {
			switch v {
			case "closed":
				if p.State == "OPEN" {
					p.State = "CLOSED"
					now := time.Now()
					p.ClosedAt = &now
				}
			case "open":
				if p.State == "CLOSED" {
					p.State = "OPEN"
					p.ClosedAt = nil
				}
			}
		}
	})

	updated := s.store.GetPullRequest(pr.ID)
	writeJSON(w, http.StatusOK, pullRequestToJSON(updated, s.store, s.baseURL(r), repo.FullName))
}

func (s *Server) handleMergePullRequest(w http.ResponseWriter, r *http.Request) {
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

	pr := s.store.GetPullRequestByNumber(repo.ID, num)
	if pr == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	if pr.State == "MERGED" {
		writeGHError(w, http.StatusMethodNotAllowed, "Pull Request is not mergeable")
		return
	}
	if pr.State == "CLOSED" {
		writeGHError(w, http.StatusUnprocessableEntity, "Pull Request is closed")
		return
	}

	s.store.UpdatePullRequest(pr.ID, func(p *PullRequest) {
		now := time.Now()
		p.State = "MERGED"
		p.MergedAt = &now
		p.ClosedAt = &now
		p.MergedByID = user.ID
	})

	sha := fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("merge-%d-%d", pr.ID, time.Now().UnixNano()))))[:40]
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"sha":     sha,
		"merged":  true,
		"message": "Pull Request successfully merged",
	})
}

func (s *Server) handleCreatePRReview(w http.ResponseWriter, r *http.Request) {
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

	pr := s.store.GetPullRequestByNumber(repo.ID, num)
	if pr == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	var req struct {
		Body  string `json:"body"`
		Event string `json:"event"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeGHError(w, http.StatusBadRequest, "Problems parsing JSON")
		return
	}

	state := "COMMENTED"
	switch strings.ToUpper(req.Event) {
	case "APPROVE":
		state = "APPROVED"
	case "REQUEST_CHANGES":
		state = "CHANGES_REQUESTED"
	case "COMMENT":
		state = "COMMENTED"
	}

	review := s.store.CreatePRReview(pr.ID, user.ID, state, req.Body)
	if review == nil {
		writeGHError(w, http.StatusUnprocessableEntity, "Review creation failed")
		return
	}

	writeJSON(w, http.StatusOK, reviewToJSON(review, s.store, s.baseURL(r), repo.FullName, pr.Number))
}

func (s *Server) handleListPRReviews(w http.ResponseWriter, r *http.Request) {
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

	pr := s.store.GetPullRequestByNumber(repo.ID, num)
	if pr == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	reviews := s.store.ListPRReviews(pr.ID)
	base := s.baseURL(r)
	result := make([]map[string]interface{}, 0, len(reviews))
	for _, review := range reviews {
		result = append(result, reviewToJSON(review, s.store, base, repo.FullName, pr.Number))
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, result))
}

func (s *Server) handleRequestReviewers(w http.ResponseWriter, r *http.Request) {
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

	pr := s.store.GetPullRequestByNumber(repo.ID, num)
	if pr == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	// Just consume the body and return the PR
	var req map[string]interface{}
	json.NewDecoder(r.Body).Decode(&req)

	writeJSON(w, http.StatusCreated, pullRequestToJSON(pr, s.store, s.baseURL(r), repo.FullName))
}

// --- JSON converters ---

func pullRequestToJSON(pr *PullRequest, st *Store, baseURL, repoFullName string) map[string]interface{} {
	st.mu.RLock()

	// Resolve author
	var authorJSON map[string]interface{}
	if u, ok := st.Users[pr.AuthorID]; ok {
		authorJSON = userToJSON(u)
	}

	// Resolve labels
	labels := make([]map[string]interface{}, 0)
	for _, lid := range pr.LabelIDs {
		if l, ok := st.Labels[lid]; ok {
			labels = append(labels, issueLabelToJSON(l, baseURL, repoFullName))
		}
	}

	// Resolve assignees
	assignees := make([]map[string]interface{}, 0)
	for _, aid := range pr.AssigneeIDs {
		if u, ok := st.Users[aid]; ok {
			assignees = append(assignees, userToJSON(u))
		}
	}

	// Resolve milestone
	var milestoneJSON interface{}
	if pr.MilestoneID > 0 {
		if ms, ok := st.Milestones[pr.MilestoneID]; ok {
			milestoneJSON = milestoneToJSON(ms, baseURL, repoFullName)
		}
	}

	// Resolve merged_by
	var mergedByJSON interface{}
	if pr.MergedByID > 0 {
		if u, ok := st.Users[pr.MergedByID]; ok {
			mergedByJSON = userToJSON(u)
		}
	}

	// Count reviews inline to avoid deadlock
	reviewCount := 0
	for _, r := range st.PRReviews {
		if r.PRID == pr.ID {
			reviewCount++
		}
	}

	st.mu.RUnlock()

	// REST state: "MERGED" → state:"closed", merged:true
	state := strings.ToLower(pr.State)
	merged := pr.State == "MERGED"
	if merged {
		state = "closed"
	}

	var closedAt interface{}
	if pr.ClosedAt != nil {
		closedAt = pr.ClosedAt.Format(time.RFC3339)
	}
	var mergedAt interface{}
	if pr.MergedAt != nil {
		mergedAt = pr.MergedAt.Format(time.RFC3339)
	}

	sha := fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("head-%d", pr.ID))))[:40]

	numStr := strconv.Itoa(pr.Number)
	return map[string]interface{}{
		"id":        pr.ID,
		"node_id":   pr.NodeID,
		"url":       baseURL + "/api/v3/repos/" + repoFullName + "/pulls/" + numStr,
		"html_url":  baseURL + "/" + repoFullName + "/pull/" + numStr,
		"diff_url":  baseURL + "/" + repoFullName + "/pull/" + numStr + ".diff",
		"patch_url": baseURL + "/" + repoFullName + "/pull/" + numStr + ".patch",
		"number":    pr.Number,
		"title":     pr.Title,
		"body":      pr.Body,
		"state":     state,
		"draft":     pr.IsDraft,
		"user":      authorJSON,
		"head": map[string]interface{}{
			"ref":   pr.HeadRefName,
			"sha":   sha,
			"label": repoFullName + ":" + pr.HeadRefName,
		},
		"base": map[string]interface{}{
			"ref":   pr.BaseRefName,
			"sha":   fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("base-%d", pr.ID))))[:40],
			"label": repoFullName + ":" + pr.BaseRefName,
		},
		"labels":              labels,
		"assignees":           assignees,
		"milestone":           milestoneJSON,
		"requested_reviewers": []interface{}{},
		"merged":              merged,
		"mergeable":           pr.Mergeable == "MERGEABLE",
		"merged_at":           mergedAt,
		"merged_by":           mergedByJSON,
		"additions":           pr.Additions,
		"deletions":           pr.Deletions,
		"changed_files":       pr.ChangedFiles,
		"comments":            0,
		"review_comments":     reviewCount,
		"commits":             1,
		"created_at":          pr.CreatedAt.Format(time.RFC3339),
		"updated_at":          pr.UpdatedAt.Format(time.RFC3339),
		"closed_at":           closedAt,
	}
}

func reviewToJSON(review *PullRequestReview, st *Store, baseURL, repoFullName string, prNumber int) map[string]interface{} {
	var authorJSON map[string]interface{}
	st.mu.RLock()
	if u, ok := st.Users[review.AuthorID]; ok {
		authorJSON = userToJSON(u)
	}
	st.mu.RUnlock()

	return map[string]interface{}{
		"id":                   review.ID,
		"node_id":              review.NodeID,
		"user":                 authorJSON,
		"body":                 review.Body,
		"state":                review.State,
		"html_url":             baseURL + "/" + repoFullName + "/pull/" + strconv.Itoa(prNumber) + "#pullrequestreview-" + strconv.Itoa(review.ID),
		"pull_request_url":     baseURL + "/api/v3/repos/" + repoFullName + "/pulls/" + strconv.Itoa(prNumber),
		"author_association":   "OWNER",
		"submitted_at":         review.CreatedAt.Format(time.RFC3339),
	}
}
