package bleephub

import (
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) registerGHPullRoutes() {
	s.route("POST /api/v3/repos/{owner}/{repo}/pulls", s.requirePerm(scopePullRequests, permWrite, s.handleCreatePullRequest))
	s.route("GET /api/v3/repos/{owner}/{repo}/pulls", s.handleListPullRequests)
	s.route("GET /api/v3/repos/{owner}/{repo}/pulls/{number}", s.handleGetPullRequest)
	s.route("PATCH /api/v3/repos/{owner}/{repo}/pulls/{number}", s.requirePerm(scopePullRequests, permWrite, s.handleUpdatePullRequest))
	s.route("PUT /api/v3/repos/{owner}/{repo}/pulls/{number}/merge", s.requirePerm(scopeContents, permWrite, s.handleMergePullRequest))

	// PR reviews. The 3-segment GET/PUT/DELETE paths conflict with PR
	// review-comment reaction routes under Go 1.22's mux, so they are
	// dispatched via handlePullsThreeSegDispatch in gh_reactions.go.
	s.route("GET /api/v3/repos/{owner}/{repo}/pulls/{number}/reviews", s.requirePerm(scopePullRequests, permRead, s.handleListPRReviews))
	s.route("POST /api/v3/repos/{owner}/{repo}/pulls/{number}/reviews", s.requirePerm(scopePullRequests, permWrite, s.handleCreatePRReview))
	s.route("POST /api/v3/repos/{owner}/{repo}/pulls/{number}/reviews/{review_id}/events", s.requirePerm(scopePullRequests, permWrite, s.handleSubmitPRReview))
	s.route("PUT /api/v3/repos/{owner}/{repo}/pulls/{number}/reviews/{review_id}/dismissals", s.requirePerm(scopePullRequests, permWrite, s.handleDismissPRReview))

	// Requested reviewers
	s.route("POST /api/v3/repos/{owner}/{repo}/pulls/{number}/requested_reviewers", s.requirePerm(scopePullRequests, permWrite, s.handleRequestReviewers))
	s.route("DELETE /api/v3/repos/{owner}/{repo}/pulls/{number}/requested_reviewers", s.requirePerm(scopePullRequests, permWrite, s.handleRemoveRequestedReviewers))

	// Update branch (merge base into PR head)
	s.route("PUT /api/v3/repos/{owner}/{repo}/pulls/{number}/update-branch", s.requirePerm(scopePullRequests, permWrite, s.handleUpdateBranch))
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
		Title string   `json:"title"`
		Body  string   `json:"body"`
		Head  string   `json:"head"`
		Base  string   `json:"base"`
		Draft flexBool `json:"draft"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Head == "" {
		writeGHValidationError(w, "PullRequest", "head", "missing_field")
		return
	}

	pr := s.store.CreatePullRequest(repo.ID, user.ID, req.Title, req.Body, req.Head, req.Base, bool(req.Draft), nil, nil, 0)
	if pr == nil {
		writeGHError(w, http.StatusUnprocessableEntity, "Pull request creation failed")
		return
	}

	repoKey := owner + "/" + name
	openedPayload := buildPullRequestPayload(repo, pr, user, "opened")
	s.emitWebhookEvent(repoKey, "pull_request", "opened", openedPayload)
	go s.triggerWorkflowsForEvent(repoKey, "pull_request", "opened", "refs/heads/"+pr.HeadRefName, openedPayload)

	s.recordAuditEvent("pull_request.create", user.Login, "", map[string]interface{}{"repo": repoKey, "pr_id": pr.ID})
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
	if repo.Private && !canReadRepo(s.store, ghUserFromContext(r.Context()), repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	state := r.URL.Query().Get("state")
	if state == "" {
		state = "open"
	}

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
		result = append(result, pullRequestSimpleJSON(pr, s.store, base, repo.FullName))
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
	if repo.Private && !canReadRepo(s.store, ghUserFromContext(r.Context()), repo) {
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

	out := pullRequestToJSON(pr, s.store, s.baseURL(r), repo.FullName)
	s.applyChecksToMergeability(out, repo, pr)
	writeJSON(w, http.StatusOK, out)
}

// applyChecksToMergeability folds the head commit's check runs into
// mergeable_state the way real GitHub does: unmet REQUIRED status
// checks (branch protection on the base branch) block the merge;
// failing or still-running non-required checks mark it unstable.
func (s *Server) applyChecksToMergeability(out map[string]interface{}, repo *Repo, pr *PullRequest) {
	if pr.State != "OPEN" || out["mergeable_state"] != "clean" {
		return
	}
	headSha := s.prHeadSha(repo, pr)
	if headSha == "" {
		return
	}
	st := s.evaluateChecksForMerge(repo, pr.BaseRefName, headSha)
	switch {
	case len(st.MissingRequired) > 0:
		out["mergeable_state"] = "blocked"
	case st.AnyFailing, st.AnyPending:
		out["mergeable_state"] = "unstable"
	}
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
	if !decodeJSONBody(w, r, &req) {
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

	if v, ok := req["state"].(string); ok {
		action := "edited"
		if v == "closed" {
			action = "closed"
		} else if v == "open" {
			action = "reopened"
		}
		repoKey := owner + "/" + repoName
		payload := buildPullRequestPayload(repo, updated, user, action)
		s.emitWebhookEvent(repoKey, "pull_request", action, payload)
		go s.triggerWorkflowsForEvent(repoKey, "pull_request", action, "refs/heads/"+updated.HeadRefName, payload)
	}

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

	// Branch protection: required status checks must be green on the
	// head commit before the merge API succeeds (405, real GitHub).
	if headSha := s.prHeadSha(repo, pr); headSha != "" {
		if st := s.evaluateChecksForMerge(repo, pr.BaseRefName, headSha); len(st.MissingRequired) > 0 {
			writeGHError(w, http.StatusMethodNotAllowed,
				fmt.Sprintf("Required status check %q is expected.", st.MissingRequired[0]))
			return
		}
	}

	if ok, msg := s.canMergePullRequest(repo, pr, user); !ok {
		status := http.StatusMethodNotAllowed
		if msg == "" {
			msg = "Pull Request is not mergeable"
		}
		writeGHError(w, status, msg)
		return
	}

	s.store.UpdatePullRequest(pr.ID, func(p *PullRequest) {
		now := time.Now()
		p.State = "MERGED"
		p.MergedAt = &now
		p.ClosedAt = &now
		p.MergedByID = user.ID
	})

	merged := s.store.GetPullRequest(pr.ID)
	repoKey := owner + "/" + repoName
	mergedPayload := buildPullRequestPayload(repo, merged, user, "closed")
	s.emitWebhookEvent(repoKey, "pull_request", "closed", mergedPayload)
	go s.triggerWorkflowsForEvent(repoKey, "pull_request", "closed", "refs/heads/"+merged.HeadRefName, mergedPayload)

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

	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	num, err := strconv.Atoi(r.PathValue("number"))
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
	if !decodeJSONBody(w, r, &req) {
		return
	}

	state := "PENDING"
	switch strings.ToUpper(req.Event) {
	case "APPROVE":
		state = "APPROVED"
	case "REQUEST_CHANGES":
		state = "CHANGES_REQUESTED"
	case "COMMENT":
		state = "COMMENTED"
	}

	review := s.store.CreatePullRequestReview(repo.FullName, pr.Number, user.ID, req.Body, state)
	if review == nil {
		writeGHError(w, http.StatusUnprocessableEntity, "Review creation failed")
		return
	}

	writeJSON(w, http.StatusOK, reviewToJSON(review, s.store, s.baseURL(r), repo.FullName, pr.Number))
}

func (s *Server) handleListPRReviews(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return
	}

	num, err := strconv.Atoi(r.PathValue("number"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	pr := s.store.GetPullRequestByNumber(repo.ID, num)
	if pr == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	reviews := s.store.ListPullRequestReviews(repo.FullName, pr.Number)
	base := s.baseURL(r)
	result := make([]map[string]interface{}, 0, len(reviews))
	for _, review := range reviews {
		result = append(result, reviewToJSON(review, s.store, base, repo.FullName, pr.Number))
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, result))
}

// handlePullsThreeSegDispatch routes the ambiguous 3-segment paths under
// /repos/{owner}/{repo}/pulls/{a}/{b}/{c}. The PR review surface
// (/pulls/{number}/reviews/{review_id}) and PR review-comment reactions
// (/pulls/comments/{comment_id}/reactions) both occupy three path segments
// after /pulls and cannot both be registered directly with Go 1.22's mux.
// The dispatcher inspects the literal segments and sets the correct path
// values for the delegated handler. It is registered from gh_reactions.go
// so that reaction-only test servers also provide the dispatch surface.
func (s *Server) handlePullsThreeSegDispatch(method string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p1 := r.PathValue("p1")
		p2 := r.PathValue("p2")
		p3 := r.PathValue("p3")

		// PR review: /pulls/{number}/reviews/{review_id}
		if p2 == "reviews" {
			r.SetPathValue("number", p1)
			r.SetPathValue("review_id", p3)
			switch method {
			case "GET":
				s.handleGetPRReview(w, r)
			case "PUT":
				s.handleUpdatePRReview(w, r)
			case "DELETE":
				s.handleDeletePRReview(w, r)
			default:
				writeGHError(w, http.StatusNotFound, "Not Found")
			}
			return
		}

		// PR review-comment reactions: /pulls/comments/{comment_id}/reactions
		if p1 == "comments" && p3 == "reactions" {
			r.SetPathValue("comment_id", p2)
			switch method {
			case "GET":
				s.handleListReactions("pull_request_review_comment", "comment_id")(w, r)
			case "POST":
				s.handleCreateReaction("pull_request_review_comment", "comment_id")(w, r)
			default:
				writeGHError(w, http.StatusNotFound, "Not Found")
			}
			return
		}

		writeGHError(w, http.StatusNotFound, "Not Found")
	}
}

func (s *Server) handleGetPRReview(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return
	}

	num, err := strconv.Atoi(r.PathValue("number"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	pr := s.store.GetPullRequestByNumber(repo.ID, num)
	if pr == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	reviewID, err := strconv.Atoi(r.PathValue("review_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	review := s.store.GetPullRequestReview(reviewID)
	if review == nil || review.PRID != pr.ID {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	writeJSON(w, http.StatusOK, reviewToJSON(review, s.store, s.baseURL(r), repo.FullName, pr.Number))
}

func (s *Server) handleUpdatePRReview(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	num, err := strconv.Atoi(r.PathValue("number"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	pr := s.store.GetPullRequestByNumber(repo.ID, num)
	if pr == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	reviewID, err := strconv.Atoi(r.PathValue("review_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	var req struct {
		Body string `json:"body"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}

	if !s.store.UpdatePullRequestReview(reviewID, req.Body) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	review := s.store.GetPullRequestReview(reviewID)
	if review == nil || review.PRID != pr.ID {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	writeJSON(w, http.StatusOK, reviewToJSON(review, s.store, s.baseURL(r), repo.FullName, pr.Number))
}

func (s *Server) handleDeletePRReview(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	num, err := strconv.Atoi(r.PathValue("number"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	pr := s.store.GetPullRequestByNumber(repo.ID, num)
	if pr == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	reviewID, err := strconv.Atoi(r.PathValue("review_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	review := s.store.GetPullRequestReview(reviewID)
	if review == nil || review.PRID != pr.ID {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if review.State != "PENDING" {
		writeGHError(w, http.StatusUnprocessableEntity, "Review must be pending to delete")
		return
	}

	if !s.store.DeletePullRequestReview(reviewID) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSubmitPRReview(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	num, err := strconv.Atoi(r.PathValue("number"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	pr := s.store.GetPullRequestByNumber(repo.ID, num)
	if pr == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	reviewID, err := strconv.Atoi(r.PathValue("review_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	var req struct {
		Event string `json:"event"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}

	review := s.store.GetPullRequestReview(reviewID)
	if review == nil || review.PRID != pr.ID {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if review.State != "PENDING" {
		writeGHError(w, http.StatusUnprocessableEntity, "Review must be pending to submit")
		return
	}

	if !s.store.SubmitPullRequestReview(reviewID, req.Event) {
		writeGHError(w, http.StatusUnprocessableEntity, "Invalid review event")
		return
	}

	review = s.store.GetPullRequestReview(reviewID)
	writeJSON(w, http.StatusOK, reviewToJSON(review, s.store, s.baseURL(r), repo.FullName, pr.Number))
}

func (s *Server) handleDismissPRReview(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	num, err := strconv.Atoi(r.PathValue("number"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	pr := s.store.GetPullRequestByNumber(repo.ID, num)
	if pr == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	reviewID, err := strconv.Atoi(r.PathValue("review_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	var req struct {
		Message string `json:"message"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}

	review := s.store.GetPullRequestReview(reviewID)
	if review == nil || review.PRID != pr.ID {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	if !s.store.DismissPullRequestReview(reviewID, req.Message) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	review = s.store.GetPullRequestReview(reviewID)
	writeJSON(w, http.StatusOK, reviewToJSON(review, s.store, s.baseURL(r), repo.FullName, pr.Number))
}

func (s *Server) handleRequestReviewers(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}

	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	num, err := strconv.Atoi(r.PathValue("number"))
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
		Reviewers     []interface{} `json:"reviewers"`
		TeamReviewers []string      `json:"team_reviewers"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}

	reviewerIDs := reviewerIDsFromRequest(s.store, req.Reviewers)
	if len(reviewerIDs) == 0 && len(req.TeamReviewers) == 0 {
		writeGHValidationError(w, "PullRequest", "reviewers", "missing_field")
		return
	}

	if len(reviewerIDs) > 0 {
		if !s.store.RequestReviewers(repo.FullName, pr.Number, reviewerIDs) {
			writeGHError(w, http.StatusUnprocessableEntity, "Unable to request reviewers")
			return
		}
	}

	updated := s.store.GetPullRequestByNumber(repo.ID, num)
	writeJSON(w, http.StatusCreated, pullRequestSimpleJSON(updated, s.store, s.baseURL(r), repo.FullName))
}

func (s *Server) handleRemoveRequestedReviewers(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}

	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	num, err := strconv.Atoi(r.PathValue("number"))
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
		Reviewers     []interface{} `json:"reviewers"`
		TeamReviewers []string      `json:"team_reviewers"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}

	reviewerIDs := reviewerIDsFromRequest(s.store, req.Reviewers)
	if len(reviewerIDs) > 0 {
		if !s.store.RemoveRequestedReviewers(repo.FullName, pr.Number, reviewerIDs) {
			writeGHError(w, http.StatusUnprocessableEntity, "Unable to remove reviewers")
			return
		}
	}

	updated := s.store.GetPullRequestByNumber(repo.ID, num)
	writeJSON(w, http.StatusOK, pullRequestSimpleJSON(updated, s.store, s.baseURL(r), repo.FullName))
}

func (s *Server) handleUpdateBranch(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}

	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	num, err := strconv.Atoi(r.PathValue("number"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	pr := s.store.GetPullRequestByNumber(repo.ID, num)
	if pr == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	// bleephub does not materialise branch merges; accept the request and
	// return the message and URL documented for the async 202 response.
	_, _ = io.Copy(io.Discard, r.Body)
	writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"message": "Updating pull request branch.",
		"url":     fmt.Sprintf("%s/api/v3/repos/%s/pulls/%d", s.baseURL(r), repo.FullName, pr.Number),
	})
}

// reviewerIDsFromRequest normalises the GitHub request reviewers field,
// which may be an array of logins (strings) or objects with an id/login key.
func reviewerIDsFromRequest(st *Store, reviewers []interface{}) []int {
	var ids []int
	for _, v := range reviewers {
		switch x := v.(type) {
		case string:
			if u := st.LookupUserByLogin(x); u != nil {
				ids = append(ids, u.ID)
			}
		case map[string]interface{}:
			if id, ok := x["id"].(float64); ok {
				ids = append(ids, int(id))
			} else if login, ok := x["login"].(string); ok {
				if u := st.LookupUserByLogin(login); u != nil {
					ids = append(ids, u.ID)
				}
			}
		}
	}
	return ids
}

// --- JSON converters ---

// prHeadSHA returns the deterministic synthetic head commit SHA for a
// pull request; reviews reference the same value as commit_id.
func prHeadSHA(prID int) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("head-%d", prID))))[:40]
}

// pullRequestSimpleJSON converts a PullRequest to the GitHub
// `pull-request-simple` shape used by list responses — the full shape
// minus the merge/diff-stat members that exist only on `pull-request`.
// Bleephub PRs are same-repository, so head and base both carry the
// repository and its owner. Must not be called with st.mu held.
func pullRequestSimpleJSON(pr *PullRequest, st *Store, baseURL, repoFullName string) map[string]interface{} {
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

	// Resolve requested reviewers
	requestedReviewers := make([]map[string]interface{}, 0)
	for _, rid := range pr.RequestedReviewerIDs {
		if u, ok := st.Users[rid]; ok {
			requestedReviewers = append(requestedReviewers, userToJSON(u))
		}
	}

	// Milestone and repo conversion happens after unlock: both derive
	// counts under their own locks.
	var milestone *Milestone
	if pr.MilestoneID > 0 {
		milestone = st.Milestones[pr.MilestoneID]
	}
	repo := st.ReposByName[repoFullName]

	st.mu.RUnlock()

	var milestoneJSON interface{}
	if milestone != nil {
		milestoneJSON = milestoneToJSON(milestone, st, baseURL, repoFullName)
	}

	var repoJSON interface{}
	var repoOwnerJSON interface{}
	if repo != nil {
		repoJSON = repoToJSON(repo, st, baseURL)
		repoOwnerJSON = repoOwnerREST(repo, st, baseURL)
	}

	// GitHub's assignee is the first assignee, null when unassigned.
	var assignee interface{}
	if len(assignees) > 0 {
		assignee = assignees[0]
	}

	// author_association relative to the repository: its owner authored
	// it or someone else did. Bleephub does not model commit-derived
	// CONTRIBUTOR status.
	authorAssociation := "NONE"
	if repo != nil && repo.Owner != nil && repo.Owner.ID == pr.AuthorID {
		authorAssociation = "OWNER"
	}

	// REST state: "MERGED" → state:"closed", merged:true
	state := strings.ToLower(pr.State)
	if pr.State == "MERGED" {
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

	numStr := strconv.Itoa(pr.Number)
	api := baseURL + "/api/v3/repos/" + repoFullName + "/pulls/" + numStr
	issueAPI := baseURL + "/api/v3/repos/" + repoFullName + "/issues/" + numStr
	htmlURL := baseURL + "/" + repoFullName + "/pull/" + numStr
	return map[string]interface{}{
		"id":                  pr.ID,
		"node_id":             pr.NodeID,
		"url":                 api,
		"html_url":            htmlURL,
		"diff_url":            htmlURL + ".diff",
		"patch_url":           htmlURL + ".patch",
		"issue_url":           issueAPI,
		"commits_url":         api + "/commits",
		"review_comments_url": api + "/comments",
		"review_comment_url":  baseURL + "/api/v3/repos/" + repoFullName + "/pulls/comments{/number}",
		"comments_url":        issueAPI + "/comments",
		"statuses_url":        baseURL + "/api/v3/repos/" + repoFullName + "/statuses/" + prHeadSHA(pr.ID),
		"number":              pr.Number,
		"title":               pr.Title,
		"body":                pr.Body,
		"state":               state,
		"locked":              pr.Locked,
		"draft":               pr.IsDraft,
		"user":                authorJSON,
		"head": map[string]interface{}{
			"ref":   pr.HeadRefName,
			"sha":   prHeadSHA(pr.ID),
			"label": repoFullName + ":" + pr.HeadRefName,
			"repo":  repoJSON,
			"user":  repoOwnerJSON,
		},
		"base": map[string]interface{}{
			"ref":   pr.BaseRefName,
			"sha":   fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("base-%d", pr.ID))))[:40],
			"label": repoFullName + ":" + pr.BaseRefName,
			"repo":  repoJSON,
			"user":  repoOwnerJSON,
		},
		"_links": map[string]interface{}{
			"self":            map[string]interface{}{"href": api},
			"html":            map[string]interface{}{"href": htmlURL},
			"issue":           map[string]interface{}{"href": issueAPI},
			"comments":        map[string]interface{}{"href": issueAPI + "/comments"},
			"review_comments": map[string]interface{}{"href": api + "/comments"},
			"review_comment":  map[string]interface{}{"href": baseURL + "/api/v3/repos/" + repoFullName + "/pulls/comments{/number}"},
			"commits":         map[string]interface{}{"href": api + "/commits"},
			"statuses":        map[string]interface{}{"href": baseURL + "/api/v3/repos/" + repoFullName + "/statuses/" + prHeadSHA(pr.ID)},
		},
		"labels":              labels,
		"assignee":            assignee,
		"assignees":           assignees,
		"milestone":           milestoneJSON,
		"requested_reviewers": requestedReviewers,
		"requested_teams":     []interface{}{},
		"author_association":  authorAssociation,
		"auto_merge":          nil,
		"merged_at":           mergedAt,
		"merge_commit_sha":    nil,
		"created_at":          pr.CreatedAt.Format(time.RFC3339),
		"updated_at":          pr.UpdatedAt.Format(time.RFC3339),
		"closed_at":           closedAt,
	}
}

// pullRequestToJSON converts a PullRequest to the full GitHub
// `pull-request` shape served by single-PR operations: the simple shape
// plus merge state, diff stats, and conversation counters. Bleephub
// does not materialise merges, so merge_commit_sha stays null (as on
// real GitHub before mergeability is computed). Must not be called with
// st.mu held.
func pullRequestToJSON(pr *PullRequest, st *Store, baseURL, repoFullName string) map[string]interface{} {
	out := pullRequestSimpleJSON(pr, st, baseURL, repoFullName)

	st.mu.RLock()
	reviewCount := 0
	for _, r := range st.PRReviews {
		if r.PRID == pr.ID {
			reviewCount++
		}
	}
	commentCount := 0
	for _, c := range st.Comments {
		if c.ParentType == "pull_request" && c.IssueID == pr.ID {
			commentCount++
		}
	}
	st.mu.RUnlock()

	merged := pr.State == "MERGED"
	mergeableState := "unknown"
	switch pr.Mergeable {
	case "MERGEABLE":
		mergeableState = "clean"
	case "CONFLICTING":
		mergeableState = "dirty"
	}

	var mergedByJSON interface{}
	if pr.MergedByID > 0 {
		st.mu.RLock()
		if u, ok := st.Users[pr.MergedByID]; ok {
			mergedByJSON = userToJSON(u)
		}
		st.mu.RUnlock()
	}

	out["merged"] = merged
	out["mergeable"] = pr.Mergeable == "MERGEABLE"
	out["mergeable_state"] = mergeableState
	out["maintainer_can_modify"] = false
	out["merged_by"] = mergedByJSON
	out["additions"] = pr.Additions
	out["deletions"] = pr.Deletions
	out["changed_files"] = pr.ChangedFiles
	out["comments"] = commentCount
	out["review_comments"] = reviewCount
	out["commits"] = 1
	return out
}

func reviewToJSON(review *PullRequestReview, st *Store, baseURL, repoFullName string, prNumber int) map[string]interface{} {
	var authorJSON map[string]interface{}
	var authorAssociation string
	st.mu.RLock()
	if u, ok := st.Users[review.AuthorID]; ok {
		authorJSON = userToJSON(u)
	}
	repo := st.ReposByName[repoFullName]
	if repo != nil && repo.Owner != nil && repo.Owner.ID == review.AuthorID {
		authorAssociation = "OWNER"
	} else {
		authorAssociation = "CONTRIBUTOR"
	}
	st.mu.RUnlock()

	htmlURL := baseURL + "/" + repoFullName + "/pull/" + strconv.Itoa(prNumber) + "#pullrequestreview-" + strconv.Itoa(review.ID)
	pullURL := baseURL + "/api/v3/repos/" + repoFullName + "/pulls/" + strconv.Itoa(prNumber)

	var submittedAt interface{}
	if review.SubmittedAt != nil {
		submittedAt = review.SubmittedAt.Format(time.RFC3339)
	}

	return map[string]interface{}{
		"id":                 review.ID,
		"node_id":            review.NodeID,
		"user":               authorJSON,
		"body":               review.Body,
		"state":              review.State,
		"commit_id":          prHeadSHA(review.PRID),
		"html_url":           htmlURL,
		"pull_request_url":   pullURL,
		"author_association": authorAssociation,
		"submitted_at":       submittedAt,
		"_links": map[string]interface{}{
			"html":         map[string]interface{}{"href": htmlURL},
			"pull_request": map[string]interface{}{"href": pullURL},
		},
	}
}
