package bleephub

import (
	"fmt"
	"time"
)

// IssueLabel represents a GitHub issue label (named IssueLabel to avoid
// collision with the agent Label type in store.go).
type IssueLabel struct {
	ID          int
	NodeID      string
	RepoID      int
	Name        string
	Description string
	Color       string // hex without #, e.g. "d73a4a"
	Default     bool
	CreatedAt   time.Time
}

// Milestone represents a GitHub milestone.
type Milestone struct {
	ID          int
	NodeID      string
	RepoID      int
	Number      int // per-repo sequential
	Title       string
	Description string
	State       string // "open", "closed"
	DueOn       *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Issue represents a GitHub issue.
type Issue struct {
	ID          int
	NodeID      string
	Number      int // per-repo sequential
	RepoID      int
	Title       string
	Body        string
	State       string // "OPEN", "CLOSED"
	StateReason string // "", "COMPLETED", "NOT_PLANNED"
	AuthorID    int
	AssigneeIDs []int
	LabelIDs    []int
	MilestoneID int // 0 = none
	Locked      bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
	ClosedAt    *time.Time
}

// Comment represents a comment on an issue.
type Comment struct {
	ID        int
	NodeID    string
	IssueID   int
	AuthorID  int
	Body      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// --- Label CRUD ---

// CreateLabel creates a new label in the given repository.
func (st *Store) CreateLabel(repoID int, name, description, color string) *IssueLabel {
	st.mu.Lock()
	defer st.mu.Unlock()

	// Check for duplicate name in repo
	for _, l := range st.Labels {
		if l.RepoID == repoID && l.Name == name {
			return nil
		}
	}

	now := time.Now()
	label := &IssueLabel{
		ID:          st.NextLabel,
		NodeID:      fmt.Sprintf("LA_kgDO%08d", st.NextLabel),
		RepoID:      repoID,
		Name:        name,
		Description: description,
		Color:       color,
		CreatedAt:   now,
	}
	st.NextLabel++
	st.Labels[label.ID] = label
	return label
}

// GetLabel returns a label by global ID.
func (st *Store) GetLabel(id int) *IssueLabel {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.Labels[id]
}

// GetLabelByName returns a label by repo and name.
func (st *Store) GetLabelByName(repoID int, name string) *IssueLabel {
	st.mu.RLock()
	defer st.mu.RUnlock()
	for _, l := range st.Labels {
		if l.RepoID == repoID && l.Name == name {
			return l
		}
	}
	return nil
}

// ListLabels returns all labels for a repository.
func (st *Store) ListLabels(repoID int) []*IssueLabel {
	st.mu.RLock()
	defer st.mu.RUnlock()
	var labels []*IssueLabel
	for _, l := range st.Labels {
		if l.RepoID == repoID {
			labels = append(labels, l)
		}
	}
	return labels
}

// UpdateLabel applies a mutation function to a label.
func (st *Store) UpdateLabel(id int, fn func(*IssueLabel)) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	l, ok := st.Labels[id]
	if !ok {
		return false
	}
	fn(l)
	return true
}

// DeleteLabel removes a label and detaches it from all issues.
func (st *Store) DeleteLabel(id int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	if _, ok := st.Labels[id]; !ok {
		return false
	}
	delete(st.Labels, id)
	// Remove from any issues
	for _, issue := range st.Issues {
		for i, lid := range issue.LabelIDs {
			if lid == id {
				issue.LabelIDs = append(issue.LabelIDs[:i], issue.LabelIDs[i+1:]...)
				break
			}
		}
	}
	return true
}

// --- Milestone CRUD ---

// CreateMilestone creates a new milestone in the given repository.
func (st *Store) CreateMilestone(repoID int, title, description, state string, dueOn *time.Time) *Milestone {
	st.mu.Lock()
	defer st.mu.Unlock()

	repo := st.Repos[repoID]
	if repo == nil {
		return nil
	}

	if state == "" {
		state = "open"
	}

	now := time.Now()
	ms := &Milestone{
		ID:          st.NextMilestone,
		NodeID:      fmt.Sprintf("MI_kgDO%08d", st.NextMilestone),
		RepoID:      repoID,
		Number:      repo.NextMilestoneNumber,
		Title:       title,
		Description: description,
		State:       state,
		DueOn:       dueOn,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	repo.NextMilestoneNumber++
	st.NextMilestone++
	st.Milestones[ms.ID] = ms
	return ms
}

// GetMilestone returns a milestone by global ID.
func (st *Store) GetMilestone(id int) *Milestone {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.Milestones[id]
}

// GetMilestoneByNumber returns a milestone by repo and number.
func (st *Store) GetMilestoneByNumber(repoID, number int) *Milestone {
	st.mu.RLock()
	defer st.mu.RUnlock()
	for _, ms := range st.Milestones {
		if ms.RepoID == repoID && ms.Number == number {
			return ms
		}
	}
	return nil
}

// ListMilestones returns milestones for a repository, optionally filtered by state.
func (st *Store) ListMilestones(repoID int, state string) []*Milestone {
	st.mu.RLock()
	defer st.mu.RUnlock()
	var milestones []*Milestone
	for _, ms := range st.Milestones {
		if ms.RepoID != repoID {
			continue
		}
		if state != "" && state != "all" && ms.State != state {
			continue
		}
		milestones = append(milestones, ms)
	}
	return milestones
}

// UpdateMilestone applies a mutation function to a milestone.
func (st *Store) UpdateMilestone(id int, fn func(*Milestone)) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	ms, ok := st.Milestones[id]
	if !ok {
		return false
	}
	fn(ms)
	ms.UpdatedAt = time.Now()
	return true
}

// DeleteMilestone removes a milestone and detaches it from all issues.
func (st *Store) DeleteMilestone(id int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	if _, ok := st.Milestones[id]; !ok {
		return false
	}
	delete(st.Milestones, id)
	// Detach from issues
	for _, issue := range st.Issues {
		if issue.MilestoneID == id {
			issue.MilestoneID = 0
		}
	}
	return true
}

// --- Issue CRUD ---

// CreateIssue creates a new issue in the given repository.
func (st *Store) CreateIssue(repoID, authorID int, title, body string, labelIDs, assigneeIDs []int, milestoneID int) *Issue {
	st.mu.Lock()
	defer st.mu.Unlock()

	repo := st.Repos[repoID]
	if repo == nil {
		return nil
	}

	if labelIDs == nil {
		labelIDs = []int{}
	}
	if assigneeIDs == nil {
		assigneeIDs = []int{}
	}

	now := time.Now()
	issue := &Issue{
		ID:          st.NextIssue,
		NodeID:      fmt.Sprintf("I_kgDO%08d", st.NextIssue),
		Number:      repo.NextIssueNumber,
		RepoID:      repoID,
		Title:       title,
		Body:        body,
		State:       "OPEN",
		AuthorID:    authorID,
		AssigneeIDs: assigneeIDs,
		LabelIDs:    labelIDs,
		MilestoneID: milestoneID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	repo.NextIssueNumber++
	st.NextIssue++
	st.Issues[issue.ID] = issue
	return issue
}

// GetIssue returns an issue by global ID.
func (st *Store) GetIssue(id int) *Issue {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.Issues[id]
}

// GetIssueByNumber returns an issue by repo ID and number.
func (st *Store) GetIssueByNumber(repoID, number int) *Issue {
	st.mu.RLock()
	defer st.mu.RUnlock()
	for _, issue := range st.Issues {
		if issue.RepoID == repoID && issue.Number == number {
			return issue
		}
	}
	return nil
}

// ListIssues returns issues for a repository, optionally filtered by state.
// State filter matches "OPEN"/"CLOSED"; empty or "all" returns all.
func (st *Store) ListIssues(repoID int, state string) []*Issue {
	st.mu.RLock()
	defer st.mu.RUnlock()
	var issues []*Issue
	for _, issue := range st.Issues {
		if issue.RepoID != repoID {
			continue
		}
		if state != "" && state != "all" {
			if issue.State != state {
				continue
			}
		}
		issues = append(issues, issue)
	}
	return issues
}

// UpdateIssue applies a mutation function to an issue.
func (st *Store) UpdateIssue(id int, fn func(*Issue)) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	issue, ok := st.Issues[id]
	if !ok {
		return false
	}
	fn(issue)
	issue.UpdatedAt = time.Now()
	return true
}

// --- Comment CRUD ---

// CreateComment creates a new comment on an issue.
func (st *Store) CreateComment(issueID, authorID int, body string) *Comment {
	st.mu.Lock()
	defer st.mu.Unlock()

	if _, ok := st.Issues[issueID]; !ok {
		return nil
	}

	now := time.Now()
	c := &Comment{
		ID:        st.NextComment,
		NodeID:    fmt.Sprintf("IC_kgDO%08d", st.NextComment),
		IssueID:   issueID,
		AuthorID:  authorID,
		Body:      body,
		CreatedAt: now,
		UpdatedAt: now,
	}
	st.NextComment++
	st.Comments[c.ID] = c
	return c
}

// ListComments returns all comments for an issue.
func (st *Store) ListComments(issueID int) []*Comment {
	st.mu.RLock()
	defer st.mu.RUnlock()
	var comments []*Comment
	for _, c := range st.Comments {
		if c.IssueID == issueID {
			comments = append(comments, c)
		}
	}
	return comments
}
