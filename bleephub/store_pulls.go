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
	HeadRepoID           int    // source repository; zero on legacy rows means RepoID
	BaseRefName          string // target branch name
	BaseSHA              string // base branch commit at PR creation ("" when the repo had no git objects)
	MergeCommitSHA       string // merge result commit ("" until merged, or when merged without git refs)
	MaintainerCanModify  bool
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

type PullRequestOptions struct {
	HeadRepoID          int
	MaintainerCanModify bool
}

// CreatePullRequest creates a new pull request in the given repository.
// Uses the shared NextIssueNumber counter for issue/PR numbering.
func (st *Store) CreatePullRequest(repoID, authorID int, title, body, headRefName, baseRefName string, isDraft bool, labelIDs, assigneeIDs []int, milestoneID int, opts ...PullRequestOptions) *PullRequest {
	st.mu.Lock()
	defer st.mu.Unlock()

	repo := st.Repos[repoID]
	if repo == nil {
		return nil
	}
	headRepoID := repoID
	maintainerCanModify := false
	if len(opts) > 0 {
		if opts[0].HeadRepoID != 0 {
			headRepoID = opts[0].HeadRepoID
		}
		maintainerCanModify = opts[0].MaintainerCanModify
	}
	headRepo := st.Repos[headRepoID]
	if headRepo == nil {
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
	headStor := st.GitStorages[headRepo.FullName]
	baseStor := st.GitStorages[repo.FullName]
	headSHA := resolveBranchSha(headStor, headRefName)
	baseSHA := resolveBranchSha(baseStor, baseRefName)
	if headSHA == "" || baseSHA == "" {
		return nil
	}

	now := time.Now().UTC()
	pr := &PullRequest{
		ID:                  st.NextPR,
		NodeID:              fmt.Sprintf("PR_kgDO%08d", st.NextPR),
		Number:              repo.NextIssueNumber, // shared counter
		RepoID:              repoID,
		Title:               title,
		Body:                body,
		State:               "OPEN",
		IsDraft:             isDraft,
		HeadRefName:         headRefName,
		HeadRepoID:          headRepoID,
		BaseRefName:         baseRefName,
		MaintainerCanModify: maintainerCanModify,
		// GitHub records the base commit at PR creation; the PR's commit
		// range stays anchored to it even after the base branch advances
		// (including past the PR's own merge commit).
		BaseSHA:     baseSHA,
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
	st.indexPullLocked(pr)
	if st.persist != nil {
		st.persist.MustPut("pull_requests", strconv.Itoa(pr.ID), pr)
	}
	return pr
}

// indexPullLocked records the PR in the per-repo secondary index so
// GetPullRequestByNumber and ListPullRequests resolve in O(PRs-in-repo)
// instead of a full scan of every PR in the store. Caller holds st.mu.
func (st *Store) indexPullLocked(pr *PullRequest) {
	m := st.PullsByRepo[pr.RepoID]
	if m == nil {
		m = make(map[int]*PullRequest)
		st.PullsByRepo[pr.RepoID] = m
	}
	m[pr.Number] = pr
}

// unindexPullLocked removes the PR from the per-repo secondary index.
// Caller holds st.mu.
func (st *Store) unindexPullLocked(pr *PullRequest) {
	if m := st.PullsByRepo[pr.RepoID]; m != nil {
		delete(m, pr.Number)
		if len(m) == 0 {
			delete(st.PullsByRepo, pr.RepoID)
		}
	}
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
	return st.PullsByRepo[repoID][number]
}

// ListPullRequests returns pull requests for a repository, optionally filtered by state.
// State filter: "OPEN", "CLOSED" (includes MERGED), "MERGED", "" or "all" returns all.
func (st *Store) ListPullRequests(repoID int, state string) []*PullRequest {
	st.mu.RLock()
	defer st.mu.RUnlock()
	var prs []*PullRequest
	for _, pr := range st.PullsByRepo[repoID] {
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

// recordPullRequestLabelEventLocked records a labeled/unlabeled event attached
// to a pull request so it surfaces in the PR timeline (ParentType
// "pull_request"), while st.mu is already held.
func (st *Store) recordPullRequestLabelEventLocked(repoID, prID, actorID, labelID int, event string) {
	e := st.recordIssueEventWithIDsLocked(repoID, prID, actorID, event, labelID, 0, 0, 0, 0)
	e.ParentType = "pull_request"
	if st.persist != nil {
		st.persist.MustPut("issue_events", strconv.Itoa(e.ID), e)
	}
}

// AddPullRequestLabels adds labels to a pull request, recording a labeled event
// for each new attachment. Pull requests carry labels through the same
// /issues/{number}/labels surface real GitHub exposes. Returns true when the PR
// exists; duplicate IDs are ignored.
func (st *Store) AddPullRequestLabels(repoID, prNumber int, labelIDs []int, actorID int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	pr := st.PullsByRepo[repoID][prNumber]
	if pr == nil {
		return false
	}
	added := false
	for _, lid := range labelIDs {
		found := false
		for _, existing := range pr.LabelIDs {
			if existing == lid {
				found = true
				break
			}
		}
		if !found {
			pr.LabelIDs = append(pr.LabelIDs, lid)
			st.recordPullRequestLabelEventLocked(repoID, pr.ID, actorID, lid, "labeled")
			added = true
		}
	}
	if added {
		pr.UpdatedAt = time.Now().UTC()
		if st.persist != nil {
			st.persist.MustPut("pull_requests", strconv.Itoa(pr.ID), pr)
		}
	}
	return true
}

// SetPullRequestLabels replaces all labels on a pull request, recording
// labeled/unlabeled events for the deltas. Returns true when the PR exists.
func (st *Store) SetPullRequestLabels(repoID, prNumber int, labelIDs []int, actorID int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	pr := st.PullsByRepo[repoID][prNumber]
	if pr == nil {
		return false
	}
	old := make(map[int]bool, len(pr.LabelIDs))
	for _, lid := range pr.LabelIDs {
		old[lid] = true
	}
	newSet := make(map[int]bool, len(labelIDs))
	for _, lid := range labelIDs {
		newSet[lid] = true
	}
	for _, lid := range pr.LabelIDs {
		if !newSet[lid] {
			st.recordPullRequestLabelEventLocked(repoID, pr.ID, actorID, lid, "unlabeled")
		}
	}
	for _, lid := range labelIDs {
		if !old[lid] {
			st.recordPullRequestLabelEventLocked(repoID, pr.ID, actorID, lid, "labeled")
		}
	}
	pr.LabelIDs = labelIDs
	pr.UpdatedAt = time.Now().UTC()
	if st.persist != nil {
		st.persist.MustPut("pull_requests", strconv.Itoa(pr.ID), pr)
	}
	return true
}

// ClearPullRequestLabels removes every label from a pull request, recording an
// unlabeled event for each previously-attached label. Returns true when the PR
// exists.
func (st *Store) ClearPullRequestLabels(repoID, prNumber, actorID int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	pr := st.PullsByRepo[repoID][prNumber]
	if pr == nil {
		return false
	}
	if len(pr.LabelIDs) == 0 {
		return true
	}
	for _, lid := range pr.LabelIDs {
		st.recordPullRequestLabelEventLocked(repoID, pr.ID, actorID, lid, "unlabeled")
	}
	pr.LabelIDs = nil
	pr.UpdatedAt = time.Now().UTC()
	if st.persist != nil {
		st.persist.MustPut("pull_requests", strconv.Itoa(pr.ID), pr)
	}
	return true
}

// RemovePullRequestLabel removes a single label from a pull request by id,
// recording an unlabeled event. Returns true when the PR exists (whether or not
// the label was attached), false when it does not.
func (st *Store) RemovePullRequestLabel(repoID, prNumber, labelID, actorID int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	pr := st.PullsByRepo[repoID][prNumber]
	if pr == nil {
		return false
	}
	for idx, lid := range pr.LabelIDs {
		if lid == labelID {
			pr.LabelIDs = append(pr.LabelIDs[:idx], pr.LabelIDs[idx+1:]...)
			pr.UpdatedAt = time.Now().UTC()
			if st.persist != nil {
				st.persist.MustPut("pull_requests", strconv.Itoa(pr.ID), pr)
			}
			st.recordPullRequestLabelEventLocked(repoID, pr.ID, actorID, labelID, "unlabeled")
			break
		}
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
	st.PRReviewsByPR[review.PRID] = append(st.PRReviewsByPR[review.PRID], review)
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
	pr := st.PullsByRepo[repo.ID][pullNumber]
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
	pr := st.PullsByRepo[repo.ID][pullNumber]
	if pr == nil {
		return nil
	}
	reviews := make([]*PullRequestReview, len(st.PRReviewsByPR[pr.ID]))
	copy(reviews, st.PRReviewsByPR[pr.ID])
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
	st.unindexPRReviewLocked(r)
	if st.persist != nil {
		st.persist.MustDelete("pr_reviews", strconv.Itoa(id))
	}
	return true
}

// unindexPRReviewLocked removes a review from the per-PR review index. Caller
// holds st.mu.
func (st *Store) unindexPRReviewLocked(r *PullRequestReview) {
	list := st.PRReviewsByPR[r.PRID]
	for i, x := range list {
		if x.ID == r.ID {
			st.PRReviewsByPR[r.PRID] = append(list[:i], list[i+1:]...)
			break
		}
	}
	if len(st.PRReviewsByPR[r.PRID]) == 0 {
		delete(st.PRReviewsByPR, r.PRID)
	}
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
	reviews := make([]*PullRequestReview, len(st.PRReviewsByPR[prID]))
	copy(reviews, st.PRReviewsByPR[prID])
	return reviews
}
