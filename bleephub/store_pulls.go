package bleephub

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// PullRequest represents a GitHub pull request.
type PullRequest struct {
	ID                   int
	NodeID               string
	Number               int // per-repo, SHARED with issues via NextIssueNumber
	RepoID               int
	Title                string
	Body                 string
	State                string // "OPEN", "CLOSED", "MERGED"
	IsDraft              bool
	HeadRefName          string // source branch name
	BaseRefName          string // target branch name
	BaseSHA              string // base branch commit at PR creation ("" when the repo had no git objects)
	MergeCommitSHA       string // merge result commit ("" until merged, or when merged without git refs)
	AuthorID             int
	AssigneeIDs          []int
	LabelIDs             []int
	RequestedReviewerIDs []int
	MilestoneID          int    // 0 = none
	Mergeable            string // "MERGEABLE", "CONFLICTING", "UNKNOWN"
	Additions            int
	Deletions            int
	ChangedFiles         int
	MergedByID           int // 0 = not merged
	Locked               bool
	ActiveLockReason     string // "", "off-topic", "too heated", "resolved", "spam"
	CreatedAt            time.Time
	UpdatedAt            time.Time
	ClosedAt             *time.Time
	MergedAt             *time.Time
}

// PullRequestReview represents a review on a pull request.
type PullRequestReview struct {
	ID               int
	NodeID           string
	PRID             int // PullRequest.ID
	AuthorID         int
	State            string // "APPROVED", "CHANGES_REQUESTED", "COMMENTED", "PENDING", "DISMISSED"
	Body             string
	SubmittedAt      *time.Time
	DismissedAt      *time.Time
	DismissalMessage string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// CreatePullRequest creates a new pull request in the given repository.
// Uses the shared NextIssueNumber counter for issue/PR numbering.
func (st *Store) CreatePullRequest(repoID, authorID int, title, body, headRefName, baseRefName string, isDraft bool, labelIDs, assigneeIDs []int, milestoneID int) *PullRequest {
	st.mu.Lock()
	defer st.mu.Unlock()

	repo := st.Repos[repoID]
	if repo == nil {
		return nil
	}

	if baseRefName == "" {
		baseRefName = repo.DefaultBranch
	}

	if labelIDs == nil {
		labelIDs = []int{}
	}
	if assigneeIDs == nil {
		assigneeIDs = []int{}
	}

	now := time.Now().UTC()
	pr := &PullRequest{
		ID:          st.NextPR,
		NodeID:      fmt.Sprintf("PR_kgDO%08d", st.NextPR),
		Number:      repo.NextIssueNumber, // shared counter
		RepoID:      repoID,
		Title:       title,
		Body:        body,
		State:       "OPEN",
		IsDraft:     isDraft,
		HeadRefName: headRefName,
		BaseRefName: baseRefName,
		// GitHub records the base commit at PR creation; the PR's commit
		// range stays anchored to it even after the base branch advances
		// (including past the PR's own merge commit).
		BaseSHA:     resolveBranchSha(st.GitStorages[repo.FullName], baseRefName),
		AuthorID:    authorID,
		AssigneeIDs: assigneeIDs,
		LabelIDs:    labelIDs,
		MilestoneID: milestoneID,
		Mergeable:   "MERGEABLE",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	repo.NextIssueNumber++
	st.NextPR++
	st.PullRequests[pr.ID] = pr
	if st.persist != nil {
		st.persist.MustPut("pull_requests", strconv.Itoa(pr.ID), pr)
	}
	return pr
}

// GetPullRequest returns a pull request by global ID.
func (st *Store) GetPullRequest(id int) *PullRequest {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.PullRequests[id]
}

// GetPullRequestByNumber returns a pull request by repo ID and number.
func (st *Store) GetPullRequestByNumber(repoID, number int) *PullRequest {
	st.mu.RLock()
	defer st.mu.RUnlock()
	for _, pr := range st.PullRequests {
		if pr.RepoID == repoID && pr.Number == number {
			return pr
		}
	}
	return nil
}

// ListPullRequests returns pull requests for a repository, optionally filtered by state.
// State filter: "OPEN", "CLOSED" (includes MERGED), "MERGED", "" or "all" returns all.
func (st *Store) ListPullRequests(repoID int, state string) []*PullRequest {
	st.mu.RLock()
	defer st.mu.RUnlock()
	var prs []*PullRequest
	for _, pr := range st.PullRequests {
		if pr.RepoID != repoID {
			continue
		}
		if state != "" && state != "all" {
			if state == "CLOSED" {
				// GitHub: "closed" includes merged
				if pr.State != "CLOSED" && pr.State != "MERGED" {
					continue
				}
			} else if pr.State != state {
				continue
			}
		}
		prs = append(prs, pr)
	}
	return prs
}

// UpdatePullRequest applies a mutation function to a pull request.
func (st *Store) UpdatePullRequest(id int, fn func(*PullRequest)) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	pr, ok := st.PullRequests[id]
	if !ok {
		return false
	}
	fn(pr)
	pr.UpdatedAt = time.Now().UTC()
	if st.persist != nil {
		st.persist.MustPut("pull_requests", strconv.Itoa(pr.ID), pr)
	}
	return true
}

// CreatePRReview creates a new review on a pull request (legacy prID-based API).
func (st *Store) CreatePRReview(prID, authorID int, state, body string) *PullRequestReview {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.createPRReviewLocked(prID, authorID, state, body)
}

// createPRReviewLocked creates a review while holding st.mu.
func (st *Store) createPRReviewLocked(prID, authorID int, state, body string) *PullRequestReview {
	if _, ok := st.PullRequests[prID]; !ok {
		return nil
	}

	now := time.Now().UTC()
	var submittedAt *time.Time
	if state != "PENDING" {
		submittedAt = &now
	}
	review := &PullRequestReview{
		ID:          st.NextPRReview,
		NodeID:      fmt.Sprintf("PRR_kgDO%08d", st.NextPRReview),
		PRID:        prID,
		AuthorID:    authorID,
		State:       state,
		Body:        body,
		SubmittedAt: submittedAt,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	st.NextPRReview++
	st.PRReviews[review.ID] = review
	if st.persist != nil {
		st.persist.MustPut("pr_reviews", strconv.Itoa(review.ID), review)
	}
	return review
}

// CreatePullRequestReview creates a review addressed by repo key and PR number.
func (st *Store) CreatePullRequestReview(repoKey string, pullNumber int, userID int, body string, state string) *PullRequestReview {
	st.mu.Lock()
	defer st.mu.Unlock()
	repo := st.ReposByName[repoKey]
	if repo == nil {
		return nil
	}
	var pr *PullRequest
	for _, p := range st.PullRequests {
		if p.RepoID == repo.ID && p.Number == pullNumber {
			pr = p
			break
		}
	}
	if pr == nil {
		return nil
	}
	return st.createPRReviewLocked(pr.ID, userID, state, body)
}

// GetPullRequestReview returns a review by global ID.
func (st *Store) GetPullRequestReview(id int) *PullRequestReview {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.PRReviews[id]
}

// ListPullRequestReviews returns all reviews for a repo/PR number.
func (st *Store) ListPullRequestReviews(repoKey string, pullNumber int) []*PullRequestReview {
	st.mu.RLock()
	defer st.mu.RUnlock()
	repo := st.ReposByName[repoKey]
	if repo == nil {
		return nil
	}
	var pr *PullRequest
	for _, p := range st.PullRequests {
		if p.RepoID == repo.ID && p.Number == pullNumber {
			pr = p
			break
		}
	}
	if pr == nil {
		return nil
	}
	var reviews []*PullRequestReview
	for _, r := range st.PRReviews {
		if r.PRID == pr.ID {
			reviews = append(reviews, r)
		}
	}
	return reviews
}

// UpdatePullRequestReview updates a review's body.
func (st *Store) UpdatePullRequestReview(id int, body string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	r, ok := st.PRReviews[id]
	if !ok {
		return false
	}
	r.Body = body
	r.UpdatedAt = time.Now().UTC()
	if st.persist != nil {
		st.persist.MustPut("pr_reviews", strconv.Itoa(r.ID), r)
	}
	return true
}

// DeletePullRequestReview deletes a pending review.
func (st *Store) DeletePullRequestReview(id int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	r, ok := st.PRReviews[id]
	if !ok {
		return false
	}
	if r.State != "PENDING" {
		return false
	}
	delete(st.PRReviews, id)
	if st.persist != nil {
		st.persist.MustDelete("pr_reviews", strconv.Itoa(id))
	}
	return true
}

// SubmitPullRequestReview transitions a pending review to an event state.
func (st *Store) SubmitPullRequestReview(id int, event string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	r, ok := st.PRReviews[id]
	if !ok {
		return false
	}
	if r.State != "PENDING" {
		return false
	}
	now := time.Now().UTC()
	switch strings.ToUpper(event) {
	case "APPROVE":
		r.State = "APPROVED"
	case "REQUEST_CHANGES":
		r.State = "CHANGES_REQUESTED"
	case "COMMENT":
		r.State = "COMMENTED"
	default:
		return false
	}
	r.SubmittedAt = &now
	r.UpdatedAt = now
	if st.persist != nil {
		st.persist.MustPut("pr_reviews", strconv.Itoa(r.ID), r)
	}
	return true
}

// DismissPullRequestReview marks a review as dismissed.
func (st *Store) DismissPullRequestReview(id int, message string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	r, ok := st.PRReviews[id]
	if !ok {
		return false
	}
	now := time.Now().UTC()
	r.State = "DISMISSED"
	r.DismissalMessage = message
	r.DismissedAt = &now
	r.UpdatedAt = now
	if st.persist != nil {
		st.persist.MustPut("pr_reviews", strconv.Itoa(r.ID), r)
	}
	return true
}

func (st *Store) findPRByRepoNumberLocked(repoKey string, pullNumber int) *PullRequest {
	repo := st.ReposByName[repoKey]
	if repo == nil {
		return nil
	}
	for _, p := range st.PullRequests {
		if p.RepoID == repo.ID && p.Number == pullNumber {
			return p
		}
	}
	return nil
}

// RequestReviewers adds reviewer IDs to a PR's requested reviewers list and
// records a review_requested issue event for each newly added reviewer,
// attributed to actorID (the review requester).
func (st *Store) RequestReviewers(repoKey string, pullNumber int, reviewerIDs []int, actorID int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	pr := st.findPRByRepoNumberLocked(repoKey, pullNumber)
	if pr == nil {
		return false
	}
	existing := map[int]struct{}{}
	for _, id := range pr.RequestedReviewerIDs {
		existing[id] = struct{}{}
	}
	for _, id := range reviewerIDs {
		if _, ok := existing[id]; !ok {
			pr.RequestedReviewerIDs = append(pr.RequestedReviewerIDs, id)
			existing[id] = struct{}{}
			st.recordPullRequestEventLocked(pr.RepoID, pr.ID, actorID, "review_requested", "", id)
		}
	}
	pr.UpdatedAt = time.Now().UTC()
	if st.persist != nil {
		st.persist.MustPut("pull_requests", strconv.Itoa(pr.ID), pr)
	}
	return true
}

// RemoveRequestedReviewers removes reviewer IDs from a PR's requested
// reviewers list and records a review_request_removed issue event for each
// reviewer actually removed, attributed to actorID.
func (st *Store) RemoveRequestedReviewers(repoKey string, pullNumber int, reviewerIDs []int, actorID int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	pr := st.findPRByRepoNumberLocked(repoKey, pullNumber)
	if pr == nil {
		return false
	}
	remove := map[int]struct{}{}
	for _, id := range reviewerIDs {
		remove[id] = struct{}{}
	}
	var kept []int
	for _, id := range pr.RequestedReviewerIDs {
		if _, ok := remove[id]; !ok {
			kept = append(kept, id)
			continue
		}
		st.recordPullRequestEventLocked(pr.RepoID, pr.ID, actorID, "review_request_removed", "", id)
	}
	pr.RequestedReviewerIDs = kept
	pr.UpdatedAt = time.Now().UTC()
	if st.persist != nil {
		st.persist.MustPut("pull_requests", strconv.Itoa(pr.ID), pr)
	}
	return true
}

// ListPRReviews returns all reviews for a pull request.
func (st *Store) ListPRReviews(prID int) []*PullRequestReview {
	st.mu.RLock()
	defer st.mu.RUnlock()
	var reviews []*PullRequestReview
	for _, r := range st.PRReviews {
		if r.PRID == prID {
			reviews = append(reviews, r)
		}
	}
	return reviews
}
