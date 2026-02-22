package bleephub

import (
	"fmt"
	"time"
)

// PullRequest represents a GitHub pull request.
type PullRequest struct {
	ID           int
	NodeID       string
	Number       int // per-repo, SHARED with issues via NextIssueNumber
	RepoID       int
	Title        string
	Body         string
	State        string // "OPEN", "CLOSED", "MERGED"
	IsDraft      bool
	HeadRefName  string // source branch name
	BaseRefName  string // target branch name
	AuthorID     int
	AssigneeIDs  []int
	LabelIDs     []int
	MilestoneID  int // 0 = none
	Mergeable    string // "MERGEABLE", "CONFLICTING", "UNKNOWN"
	Additions    int
	Deletions    int
	ChangedFiles int
	MergedByID   int // 0 = not merged
	CreatedAt    time.Time
	UpdatedAt    time.Time
	ClosedAt     *time.Time
	MergedAt     *time.Time
}

// PullRequestReview represents a review on a pull request.
type PullRequestReview struct {
	ID        int
	NodeID    string
	PRID      int // PullRequest.ID
	AuthorID  int
	State     string // "APPROVED", "CHANGES_REQUESTED", "COMMENTED", "DISMISSED"
	Body      string
	CreatedAt time.Time
	UpdatedAt time.Time
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

	now := time.Now()
	pr := &PullRequest{
		ID:           st.NextPR,
		NodeID:       fmt.Sprintf("PR_kgDO%08d", st.NextPR),
		Number:       repo.NextIssueNumber, // shared counter
		RepoID:       repoID,
		Title:        title,
		Body:         body,
		State:        "OPEN",
		IsDraft:      isDraft,
		HeadRefName:  headRefName,
		BaseRefName:  baseRefName,
		AuthorID:     authorID,
		AssigneeIDs:  assigneeIDs,
		LabelIDs:     labelIDs,
		MilestoneID:  milestoneID,
		Mergeable:    "MERGEABLE",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	repo.NextIssueNumber++
	st.NextPR++
	st.PullRequests[pr.ID] = pr
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
	pr.UpdatedAt = time.Now()
	return true
}

// CreatePRReview creates a new review on a pull request.
func (st *Store) CreatePRReview(prID, authorID int, state, body string) *PullRequestReview {
	st.mu.Lock()
	defer st.mu.Unlock()

	if _, ok := st.PullRequests[prID]; !ok {
		return nil
	}

	now := time.Now()
	review := &PullRequestReview{
		ID:        st.NextPRReview,
		NodeID:    fmt.Sprintf("PRR_kgDO%08d", st.NextPRReview),
		PRID:      prID,
		AuthorID:  authorID,
		State:     state,
		Body:      body,
		CreatedAt: now,
		UpdatedAt: now,
	}
	st.NextPRReview++
	st.PRReviews[review.ID] = review
	return review
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
