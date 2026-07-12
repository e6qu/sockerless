package bleephub

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	gitStorage "github.com/go-git/go-git/v5/storage"
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
	s.route("GET /api/v3/repos/{owner}/{repo}/pulls/{number}/requested_reviewers", s.requirePerm(scopePullRequests, permRead, s.handleListRequestedReviewers))
	s.route("POST /api/v3/repos/{owner}/{repo}/pulls/{number}/requested_reviewers", s.requirePerm(scopePullRequests, permWrite, s.handleRequestReviewers))
	s.route("DELETE /api/v3/repos/{owner}/{repo}/pulls/{number}/requested_reviewers", s.requirePerm(scopePullRequests, permWrite, s.handleRemoveRequestedReviewers))

	// PR commits (the commits_url every PR response advertises)
	s.route("GET /api/v3/repos/{owner}/{repo}/pulls/{number}/commits", s.handleListPullRequestCommits)

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
		Title               string   `json:"title"`
		Body                string   `json:"body"`
		Head                string   `json:"head"`
		Base                string   `json:"base"`
		Draft               flexBool `json:"draft"`
		MaintainerCanModify flexBool `json:"maintainer_can_modify"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Head == "" {
		writeGHValidationError(w, "PullRequest", "head", "missing_field")
		return
	}

	headRepo, headRef := resolvePullRequestHead(s.store, repo, req.Head)
	if headRepo == nil || headRef == "" {
		writeGHValidationError(w, "PullRequest", "head", "invalid")
		return
	}

	pr := s.store.CreatePullRequest(repo.ID, user.ID, req.Title, req.Body, headRef, req.Base, bool(req.Draft), nil, nil, 0, PullRequestOptions{
		HeadRepoID:          headRepo.ID,
		MaintainerCanModify: bool(req.MaintainerCanModify),
	})
	if pr == nil {
		writeGHError(w, http.StatusUnprocessableEntity, "Pull request creation failed")
		return
	}

	repoKey := owner + "/" + name
	openedPayload := buildPullRequestPayload(s.store, repo, pr, user, "opened")
	s.emitWebhookEvent(repoKey, "pull_request", "opened", openedPayload)
	s.triggerWorkflowsForEvent(repoKey, "pull_request", "opened", "refs/heads/"+pr.HeadRefName, openedPayload)

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
		headOwner := ""
		branch := head
		if idx := strings.Index(head, ":"); idx >= 0 {
			headOwner = head[:idx]
			branch = head[idx+1:]
		}
		var filtered []*PullRequest
		for _, pr := range prs {
			if pr.HeadRefName != branch {
				continue
			}
			if headOwner != "" {
				headRepo := pullRequestHeadRepo(s.store, pr)
				if headRepo == nil || headRepo.Owner == nil || headRepo.Owner.Login != headOwner {
					continue
				}
			}
			filtered = append(filtered, pr)
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

	priorState := pr.State
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

	switch {
	case priorState == "OPEN" && updated.State == "CLOSED":
		s.store.RecordPullRequestEvent(repo.ID, pr.ID, user.ID, "closed", "", 0)
	case priorState == "CLOSED" && updated.State == "OPEN":
		s.store.RecordPullRequestEvent(repo.ID, pr.ID, user.ID, "reopened", "", 0)
	}

	if v, ok := req["state"].(string); ok {
		action := "edited"
		if v == "closed" {
			action = "closed"
		} else if v == "open" {
			action = "reopened"
		}
		repoKey := owner + "/" + repoName
		payload := buildPullRequestPayload(s.store, repo, updated, user, action)
		s.emitWebhookEvent(repoKey, "pull_request", action, payload)
		s.triggerWorkflowsForEvent(repoKey, "pull_request", action, "refs/heads/"+updated.HeadRefName, payload)
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

	var req struct {
		CommitTitle   string `json:"commit_title"`
		CommitMessage string `json:"commit_message"`
		SHA           string `json:"sha"`
		MergeMethod   string `json:"merge_method"`
	}
	if raw, err := io.ReadAll(r.Body); err == nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, &req); err != nil {
			writeGHError(w, http.StatusBadRequest, "Problems parsing JSON")
			return
		}
	}
	switch req.MergeMethod {
	case "", "merge", "squash", "rebase":
	default:
		writeGHValidationError(w, "PullRequest", "merge_method", "invalid")
		return
	}
	// Expected-head guard: merging against a stale head SHA is a 409.
	if req.SHA != "" {
		if head := s.prHeadSha(repo, pr); head != "" && head != req.SHA {
			writeGHError(w, http.StatusConflict, "Head branch was modified. Review and try the merge again.")
			return
		}
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

	mergeSha, errMsg := s.completePullRequestMerge(repo, pr, user, req.MergeMethod, req.CommitTitle, req.CommitMessage)
	if errMsg != "" {
		writeGHError(w, http.StatusMethodNotAllowed, errMsg)
		return
	}

	merged := s.store.GetPullRequest(pr.ID)
	repoKey := owner + "/" + repoName
	mergedPayload := buildPullRequestPayload(s.store, repo, merged, user, "closed")
	s.emitWebhookEvent(repoKey, "pull_request", "closed", mergedPayload)
	s.triggerWorkflowsForEvent(repoKey, "pull_request", "closed", "refs/heads/"+merged.HeadRefName, mergedPayload)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"sha":     mergeSha,
		"merged":  true,
		"message": "Pull Request successfully merged",
	})
}

// completePullRequestMerge materialises the merge in the repository's git
// storage (merge commit, squash commit, or rebased commits per method), marks
// the PR merged, and records the merged and closed timeline events. It returns
// the resulting merge commit SHA or a non-empty error message when the merge
// cannot be performed (e.g. a merge conflict).
func (s *Server) completePullRequestMerge(repo *Repo, pr *PullRequest, user *User, method, commitTitle, commitMessage string) (string, string) {
	owner, name, _ := splitRepoFullName(repo.FullName)
	stor := s.store.GetGitStorage(owner, name)
	var mergeSha string
	if stor == nil {
		return "", "Pull Request is not mergeable"
	}
	headStor, headRepoFullName := pullRequestGitStorage(s.store, repo, pr)
	if headStor == nil {
		return "", "Pull Request is not mergeable"
	}
	headHash, headErr := resolveGitRef(headStor, pr.HeadRefName)
	if headErr == nil && headRepoFullName != repo.FullName {
		if err := copyGitObjects(headStor, stor); err != nil {
			return "", "Pull Request is not mergeable"
		}
	}
	baseRef := plumbing.NewBranchReferenceName(pr.BaseRefName)
	if _, baseErr := stor.Reference(baseRef); headErr != nil || baseErr != nil {
		return "", "Pull Request is not mergeable"
	}
	email := user.Email
	if email == "" {
		email = user.Login + "@users.noreply.bleephub.local"
	}
	author := repoSignature(user.Login, email)
	var hash plumbing.Hash
	var err error
	switch method {
	case "squash":
		message := commitTitle
		if message == "" {
			message = fmt.Sprintf("%s (#%d)", pr.Title, pr.Number)
		}
		if commitMessage != "" {
			message += "\n\n" + commitMessage
		}
		hash, err = performSquashMerge(stor, baseRef, headHash, message, author, author)
	case "rebase":
		hash, err = performRebaseMerge(stor, baseRef, headHash, author)
	default: // "merge"
		message := commitTitle
		if message == "" {
			headOwner := owner
			if headRepo := pullRequestHeadRepo(s.store, pr); headRepo != nil && headRepo.Owner != nil {
				headOwner = headRepo.Owner.Login
			}
			message = fmt.Sprintf("Merge pull request #%d from %s/%s", pr.Number, headOwner, pr.HeadRefName)
		}
		body := commitMessage
		if body == "" {
			body = pr.Title
		}
		hash, err = performMergeCommit(stor, baseRef, headHash, message+"\n\n"+body, author)
	}
	if err != nil {
		return "", "Pull Request is not mergeable"
	}
	mergeSha = hash.String()
	s.store.UpdateRepo(owner, name, func(r *Repo) {
		r.PushedAt = time.Now().UTC()
	})

	s.store.UpdatePullRequest(pr.ID, func(p *PullRequest) {
		now := time.Now()
		p.State = "MERGED"
		p.MergedAt = &now
		p.ClosedAt = &now
		p.MergedByID = user.ID
		p.MergeCommitSHA = mergeSha
	})
	// GitHub's timeline for a merged PR carries a merged event (with the
	// merge commit) immediately followed by a closed event.
	s.store.RecordPullRequestEvent(repo.ID, pr.ID, user.ID, "merged", mergeSha, 0)
	s.store.RecordPullRequestEvent(repo.ID, pr.ID, user.ID, "closed", "", 0)
	return mergeSha, ""
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
		Body     string `json:"body"`
		Event    string `json:"event"`
		CommitID string `json:"commit_id"`
		Comments []struct {
			Path      string  `json:"path"`
			Body      string  `json:"body"`
			Line      flexInt `json:"line"`
			StartLine flexInt `json:"start_line"`
			Side      string  `json:"side"`
			Position  flexInt `json:"position"`
		} `json:"comments"`
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

	// The create-review API attaches its draft comments to the review; they
	// surface through GET /pulls/{number}/reviews/{review_id}/comments and
	// the regular review-comment endpoints.
	for _, rc := range req.Comments {
		if rc.Path == "" || rc.Body == "" {
			writeGHValidationError(w, "PullRequestReviewComment", "comments", "invalid")
			return
		}
		line := int(rc.Line)
		if line == 0 {
			line = int(rc.Position)
		}
		c := s.store.PRReviewComments.CreateRootComment(pr.ID, user.ID, rc.Path, rc.Body, req.CommitID, rc.Side, line, int(rc.StartLine))
		s.store.PRReviewComments.AttachToReview(c.ID, review.ID)
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
		if !s.store.RequestReviewers(repo.FullName, pr.Number, reviewerIDs, user.ID) {
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
		if !s.store.RemoveRequestedReviewers(repo.FullName, pr.Number, reviewerIDs, user.ID) {
			writeGHError(w, http.StatusUnprocessableEntity, "Unable to remove reviewers")
			return
		}
	}

	updated := s.store.GetPullRequestByNumber(repo.ID, num)
	writeJSON(w, http.StatusOK, pullRequestSimpleJSON(updated, s.store, s.baseURL(r), repo.FullName))
}

// handleListRequestedReviewers serves
// GET /repos/{owner}/{repo}/pulls/{number}/requested_reviewers — the
// pull-request-review-request shape ({users, teams}). bleephub does not model
// team review requests, so teams is always empty.
func (s *Server) handleListRequestedReviewers(w http.ResponseWriter, r *http.Request) {
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

	users := make([]map[string]interface{}, 0)
	s.store.mu.RLock()
	for _, id := range pr.RequestedReviewerIDs {
		if u, ok := s.store.Users[id]; ok {
			users = append(users, userToJSON(u))
		}
	}
	s.store.mu.RUnlock()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"users": users,
		"teams": []interface{}{},
	})
}

// buildPullRequestTimeline assembles the issue-timeline union for a pull
// request: the PR's real git commits ("committed" items, interleaved by
// commit author date), conversation comments ("commented"), submitted
// reviews ("reviewed"), and recorded issue events (review_requested,
// review_request_removed, merged, closed, reopened), ordered by creation
// time. Pending reviews are not part of the public timeline on real GitHub
// and are excluded here too.
func (s *Server) buildPullRequestTimeline(repo *Repo, pr *PullRequest, baseURL string) ([]map[string]interface{}, error) {
	comments := s.store.ListCommentsFor("pull_request", pr.ID)
	reviews := s.store.ListPullRequestReviews(repo.FullName, pr.Number)
	events := s.store.ListPullRequestEvents(repo.ID, pr.ID)
	commits, err := pullRequestCommitObjects(s.store, repo, pr)
	if err != nil {
		return nil, err
	}

	type timelineEntry struct {
		at time.Time
		// Committed items sort before same-instant events/comments: the
		// commits exist before anything reacting to them.
		rank int
		id   int
		json map[string]interface{}
	}
	entries := make([]timelineEntry, 0, len(commits)+len(comments)+len(reviews)+len(events))
	for i, c := range commits {
		entries = append(entries, timelineEntry{
			at:   c.Author.When.UTC(),
			rank: 0,
			id:   i,
			json: timelineCommittedEventJSON(c, repo.FullName, baseURL),
		})
	}
	for _, c := range comments {
		entries = append(entries, timelineEntry{
			at:   c.CreatedAt,
			rank: 1,
			id:   c.ID,
			json: timelineCommentToJSON(c, s.store, baseURL, repo.FullName, pr.Number, repo),
		})
	}
	for _, review := range reviews {
		if review.State == "PENDING" {
			continue
		}
		j := reviewToJSON(review, s.store, baseURL, repo.FullName, pr.Number)
		j["event"] = "reviewed"
		at := review.CreatedAt
		if review.SubmittedAt != nil {
			at = *review.SubmittedAt
		}
		entries = append(entries, timelineEntry{at: at, rank: 1, id: review.ID, json: j})
	}
	for _, e := range events {
		entries = append(entries, timelineEntry{
			at:   e.CreatedAt,
			rank: 1,
			id:   e.ID,
			json: issueEventForTimelineToJSON(e, s.store, baseURL, repo.FullName),
		})
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if !entries[i].at.Equal(entries[j].at) {
			return entries[i].at.Before(entries[j].at)
		}
		if entries[i].rank != entries[j].rank {
			return entries[i].rank < entries[j].rank
		}
		return entries[i].id < entries[j].id
	})

	out := make([]map[string]interface{}, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.json)
	}
	return out, nil
}

// pullRequestCommitObjects derives a pull request's commits from the
// repository's real git history: the commits reachable from the head branch
// but not from the merge base against the PR's recorded creation-time base
// commit, oldest first — the same range GitHub's pulls/{n}/commits lists.
// The recorded base keeps the range stable after the base branch advances,
// including past the PR's own merge commit. The result is empty (with a nil
// error) when the repository holds no git objects for the PR's branches.
func pullRequestCommitObjects(st *Store, repo *Repo, pr *PullRequest) ([]*object.Commit, error) {
	stor, _ := pullRequestGitStorage(st, repo, pr)
	if stor == nil {
		return nil, nil
	}
	return pullRequestCommitObjectsFromStorage(stor, pr)
}

func pullRequestCommitObjectsFromStorage(stor gitStorage.Storer, pr *PullRequest) ([]*object.Commit, error) {
	headHash, err := resolveGitRef(stor, pr.HeadRefName)
	if err != nil {
		return nil, nil
	}
	var baseHash plumbing.Hash
	if pr.BaseSHA != "" {
		baseHash = plumbing.NewHash(pr.BaseSHA)
	} else {
		baseHash, err = resolveGitRef(stor, pr.BaseRefName)
		if err != nil {
			return nil, nil
		}
	}
	mergeBase, err := findMergeBase(stor, baseHash, headHash)
	if err != nil {
		return nil, err
	}
	if mergeBase.IsZero() {
		return nil, nil
	}
	if mergeBase == headHash {
		return nil, nil
	}
	commits, err := commitsBetween(stor, mergeBase, headHash)
	if err != nil {
		return nil, err
	}
	// commitsBetween is newest-first; the API lists oldest first.
	for i, j := 0, len(commits)-1; i < j; i, j = i+1, j-1 {
		commits[i], commits[j] = commits[j], commits[i]
	}
	return commits, nil
}

// timelineCommittedEventJSON renders a real git commit as the GitHub
// timeline-committed-event shape ("event": "committed").
func timelineCommittedEventJSON(c *object.Commit, repoFullName, baseURL string) map[string]interface{} {
	sha := c.Hash.String()
	parents := make([]map[string]interface{}, 0, len(c.ParentHashes))
	for _, h := range c.ParentHashes {
		parents = append(parents, map[string]interface{}{
			"sha":      h.String(),
			"url":      baseURL + "/api/v3/repos/" + repoFullName + "/git/commits/" + h.String(),
			"html_url": baseURL + "/" + repoFullName + "/commit/" + h.String(),
		})
	}
	return map[string]interface{}{
		"event":    "committed",
		"sha":      sha,
		"node_id":  encodeNodeID("Commit", 0, sha),
		"url":      baseURL + "/api/v3/repos/" + repoFullName + "/git/commits/" + sha,
		"html_url": baseURL + "/" + repoFullName + "/commit/" + sha,
		"author": map[string]interface{}{
			"name":  c.Author.Name,
			"email": c.Author.Email,
			"date":  c.Author.When.UTC().Format(time.RFC3339),
		},
		"committer": map[string]interface{}{
			"name":  c.Committer.Name,
			"email": c.Committer.Email,
			"date":  c.Committer.When.UTC().Format(time.RFC3339),
		},
		"message": c.Message,
		"tree": map[string]interface{}{
			"sha": c.TreeHash.String(),
			"url": baseURL + "/api/v3/repos/" + repoFullName + "/git/trees/" + c.TreeHash.String(),
		},
		"parents": parents,
		"verification": map[string]interface{}{
			"verified":    false,
			"reason":      "unsigned",
			"signature":   nil,
			"payload":     nil,
			"verified_at": nil,
		},
	}
}

// handleListPullRequestCommits serves GET
// /repos/{owner}/{repo}/pulls/{number}/commits — the PR's commits derived
// from the repository's real git history (the commits_url every PR
// response advertises).
func (s *Server) handleListPullRequestCommits(w http.ResponseWriter, r *http.Request) {
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

	commits, err := pullRequestCommitObjects(s.store, repo, pr)
	if err != nil {
		writeGHError(w, http.StatusInternalServerError, "commit lookup failed")
		return
	}
	base := s.baseURL(r)
	out := make([]map[string]interface{}, 0, len(commits))
	for _, c := range commits {
		out = append(out, commitToJSON(c, repo, base))
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, out))
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

func pullRequestHeadRepoID(pr *PullRequest) int {
	if pr == nil {
		return 0
	}
	if pr.HeadRepoID != 0 {
		return pr.HeadRepoID
	}
	return pr.RepoID
}

func pullRequestHeadRepo(st *Store, pr *PullRequest) *Repo {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return pullRequestHeadRepoLocked(st, pr)
}

func pullRequestHeadRepoLocked(st *Store, pr *PullRequest) *Repo {
	if pr == nil {
		return nil
	}
	return st.Repos[pullRequestHeadRepoID(pr)]
}

func resolvePullRequestHead(st *Store, baseRepo *Repo, head string) (*Repo, string) {
	if baseRepo == nil || strings.TrimSpace(head) == "" {
		return nil, ""
	}
	ownerLogin := ""
	branch := head
	if idx := strings.Index(head, ":"); idx >= 0 {
		ownerLogin = head[:idx]
		branch = head[idx+1:]
	}
	if branch == "" {
		return nil, ""
	}
	if ownerLogin == "" || (baseRepo.Owner != nil && ownerLogin == baseRepo.Owner.Login) {
		return baseRepo, branch
	}

	st.mu.RLock()
	defer st.mu.RUnlock()
	var matches []*Repo
	networkSourceID := baseRepo.ID
	if baseRepo.SourceID != 0 {
		networkSourceID = baseRepo.SourceID
	}
	for _, repo := range st.Repos {
		if repo == nil || repo.Owner == nil || repo.Owner.Login != ownerLogin {
			continue
		}
		if repo.ID == baseRepo.ID {
			matches = append(matches, repo)
			continue
		}
		sourceID := repo.SourceID
		if sourceID == 0 {
			sourceID = repo.ID
		}
		if repo.ParentID == baseRepo.ID || sourceID == networkSourceID {
			matches = append(matches, repo)
		}
	}
	if len(matches) == 1 {
		return matches[0], branch
	}
	for _, repo := range matches {
		if repo.Name == baseRepo.Name {
			return repo, branch
		}
	}
	return nil, ""
}

func pullRequestGitStorage(st *Store, repo *Repo, pr *PullRequest) (gitStorage.Storer, string) {
	if repo == nil || pr == nil {
		return nil, ""
	}
	headRepo := pullRequestHeadRepo(st, pr)
	if headRepo == nil {
		return nil, ""
	}
	owner, name, ok := splitRepoFullName(headRepo.FullName)
	if !ok {
		return nil, ""
	}
	return st.GetGitStorage(owner, name), headRepo.FullName
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

func pullRequestHeadSHA(pr *PullRequest, st *Store) string {
	if pr == nil {
		return ""
	}
	st.mu.RLock()
	defer st.mu.RUnlock()
	return pullRequestHeadSHALocked(pr, st)
}

func pullRequestHeadSHALocked(pr *PullRequest, st *Store) string {
	if pr == nil {
		return ""
	}
	repo := pullRequestHeadRepoLocked(st, pr)
	if repo == nil {
		return ""
	}
	return resolveBranchSha(st.GitStorages[repo.FullName], pr.HeadRefName)
}

func pullRequestReviewCommitSHA(review *PullRequestReview, st *Store) string {
	if review == nil {
		return ""
	}
	return pullRequestHeadSHA(st.GetPullRequest(review.PRID), st)
}

// pullRequestSimpleJSON converts a PullRequest to the GitHub
// `pull-request-simple` shape used by list responses — the full shape
// minus the merge/diff-stat members that exist only on `pull-request`.
// Must not be called with st.mu held.
func pullRequestSimpleJSON(pr *PullRequest, st *Store, baseURL, repoFullName string) map[string]interface{} {
	// Read the PR's mutable fields off a private snapshot: an UpdatePullRequest
	// / merge / close writer mutates title, body, state, merge shas, and
	// timestamps under st.mu.Lock, so the live pointer must not be read here.
	// The snapshot's RLock is released before the map-resolution RLock below —
	// they are sequential, never nested.
	pr = st.snapPR(pr)
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
	headRepo := pullRequestHeadRepoLocked(st, pr)
	headStor := gitStorage.Storer(nil)
	if headRepo != nil {
		headStor = st.GitStorages[headRepo.FullName]
	}

	st.mu.RUnlock()

	headSHA := resolveBranchSha(headStor, pr.HeadRefName)
	baseSHA := pr.BaseSHA

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
	var headRepoJSON interface{}
	var headRepoOwnerJSON interface{}
	headRepoFullName := repoFullName
	if headRepo != nil {
		headRepoFullName = headRepo.FullName
		headRepoJSON = repoToJSON(headRepo, st, baseURL)
		headRepoOwnerJSON = repoOwnerREST(headRepo, st, baseURL)
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
	var mergeCommitSHA interface{}
	if pr.MergeCommitSHA != "" {
		mergeCommitSHA = pr.MergeCommitSHA
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
		"statuses_url":        baseURL + "/api/v3/repos/" + repoFullName + "/statuses/" + headSHA,
		"number":              pr.Number,
		"title":               pr.Title,
		"body":                pr.Body,
		"state":               state,
		"locked":              pr.Locked,
		"draft":               pr.IsDraft,
		"user":                authorJSON,
		"head": map[string]interface{}{
			"ref":   pr.HeadRefName,
			"sha":   headSHA,
			"label": headRepoFullName + ":" + pr.HeadRefName,
			"repo":  headRepoJSON,
			"user":  headRepoOwnerJSON,
		},
		"base": map[string]interface{}{
			"ref":   pr.BaseRefName,
			"sha":   baseSHA,
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
			"statuses":        map[string]interface{}{"href": baseURL + "/api/v3/repos/" + repoFullName + "/statuses/" + headSHA},
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
		"merge_commit_sha":    mergeCommitSHA,
		"created_at":          pr.CreatedAt.Format(time.RFC3339),
		"updated_at":          pr.UpdatedAt.Format(time.RFC3339),
		"closed_at":           closedAt,
	}
}

// pullRequestToJSON converts a PullRequest to the full GitHub
// `pull-request` shape served by single-PR operations: the simple shape
// plus merge state, diff stats, and conversation counters. Must not be
// called with st.mu held.
func pullRequestToJSON(pr *PullRequest, st *Store, baseURL, repoFullName string) map[string]interface{} {
	out := pullRequestSimpleJSON(pr, st, baseURL, repoFullName)

	// Snapshot before reading the mutable merge/diff fields off the pointer.
	pr = st.snapPR(pr)
	st.mu.RLock()
	reviewCount := len(st.PRReviewsByPR[pr.ID])
	commentCount := st.countCommentsForLocked("pull_request", pr.ID)
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
	out["maintainer_can_modify"] = pr.MaintainerCanModify
	out["merged_by"] = mergedByJSON
	out["additions"] = pr.Additions
	out["deletions"] = pr.Deletions
	out["changed_files"] = pr.ChangedFiles
	out["comments"] = commentCount
	out["review_comments"] = reviewCount
	commitCount := 0
	if repo := st.GetRepoByID(pr.RepoID); repo != nil {
		if commits, err := pullRequestCommitObjects(st, repo, pr); err == nil {
			commitCount = len(commits)
		}
	}
	out["commits"] = commitCount
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
		"commit_id":          pullRequestReviewCommitSHA(review, st),
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

// handleListPullRequestFiles serves GET /repos/{owner}/{repo}/pulls/{number}/files,
// the changed-file list with per-file unified-diff patches real GitHub returns.
// It is reached through handlePRCommentTwoSegDispatch (p2 == "files") so it adds
// no new mux pattern. The diff is computed between the merge-base of the PR's
// base and head and the head tip — the same range GitHub reports.
func (s *Server) handleListPullRequestFiles(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if repo.Private && !canReadRepo(s.store, ghUserFromContext(r.Context()), repo) {
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
	files, err := pullRequestChangedFiles(s.store, repo, pr, s.baseURL(r))
	if err != nil {
		writeGHError(w, http.StatusInternalServerError, "diff derivation failed")
		return
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, files))
}

// pullRequestChangedFiles diffs the PR's merge-base against its head tip and
// returns the per-file JSON GitHub's pulls/{n}/files endpoint emits, including
// the unified-diff `patch` for text changes.
func pullRequestChangedFiles(st *Store, repo *Repo, pr *PullRequest, baseURL string) ([]map[string]interface{}, error) {
	stor, _ := pullRequestGitStorage(st, repo, pr)
	if stor == nil {
		return []map[string]interface{}{}, nil
	}
	headHash, err := resolveGitRef(stor, pr.HeadRefName)
	if err != nil {
		return []map[string]interface{}{}, nil
	}
	var baseHash plumbing.Hash
	if pr.BaseSHA != "" {
		baseHash = plumbing.NewHash(pr.BaseSHA)
	} else if baseHash, err = resolveGitRef(stor, pr.BaseRefName); err != nil {
		return []map[string]interface{}{}, nil
	}
	mergeBase, err := findMergeBase(stor, baseHash, headHash)
	if err != nil {
		return nil, err
	}
	headCommit, err := object.GetCommit(stor, headHash)
	if err != nil {
		return nil, err
	}
	headTree, err := headCommit.Tree()
	if err != nil {
		return nil, err
	}
	var baseTree *object.Tree
	if !mergeBase.IsZero() {
		if baseCommit, err := object.GetCommit(stor, mergeBase); err == nil {
			baseTree, _ = baseCommit.Tree()
		}
	}
	changes, err := object.DiffTree(baseTree, headTree)
	if err != nil {
		return nil, err
	}
	files := make([]map[string]interface{}, 0, len(changes))
	for _, ch := range changes {
		var status string
		switch {
		case ch.To.TreeEntry.Mode == 0:
			status = "removed"
		case ch.From.TreeEntry.Mode == 0:
			status = "added"
		case ch.From.TreeEntry.Hash == ch.To.TreeEntry.Hash:
			continue // unchanged
		default:
			status = "modified"
		}
		adds, dels, err := changeStats(ch)
		if err != nil {
			return nil, err
		}
		filename := ch.To.Name
		if filename == "" {
			filename = ch.From.Name
		}
		sha := ch.To.TreeEntry.Hash.String()
		if ch.To.TreeEntry.Mode == 0 {
			sha = ch.From.TreeEntry.Hash.String()
		}
		file := map[string]interface{}{
			"sha":          sha,
			"filename":     filename,
			"status":       status,
			"additions":    adds,
			"deletions":    dels,
			"changes":      adds + dels,
			"blob_url":     baseURL + "/" + repo.FullName + "/blob/" + headHash.String() + "/" + filename,
			"raw_url":      baseURL + "/" + repo.FullName + "/raw/" + headHash.String() + "/" + filename,
			"contents_url": baseURL + "/api/v3/repos/" + repo.FullName + "/contents/" + filename + "?ref=" + headHash.String(),
		}
		if patch := changeUnifiedPatch(ch); patch != "" {
			file["patch"] = patch
		}
		if status != "added" && status != "removed" && ch.From.Name != "" && ch.From.Name != filename {
			file["previous_filename"] = ch.From.Name
		}
		files = append(files, file)
	}
	sort.Slice(files, func(i, j int) bool {
		ni, _ := files[i]["filename"].(string)
		nj, _ := files[j]["filename"].(string)
		return ni < nj
	})
	return files, nil
}

// changeUnifiedPatch renders the hunk portion of a change's unified diff, matching
// GitHub's `patch` field (which starts at the first "@@" hunk header, without the
// "diff --git"/index/"---"/"+++" preamble). Returns "" for binary or empty diffs.
func changeUnifiedPatch(ch *object.Change) string {
	patch, err := ch.Patch()
	if err != nil {
		return ""
	}
	full := patch.String()
	if idx := strings.Index(full, "@@"); idx >= 0 {
		return strings.TrimRight(full[idx:], "\n")
	}
	return ""
}
