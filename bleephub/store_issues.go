package bleephub

import (
	"fmt"
	"sort"
	"strconv"
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
	CreatorID   int    // user who created the milestone
	DueOn       *time.Time
	ClosedAt    *time.Time // set when state transitions to "closed"
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Issue represents a GitHub issue.
type Issue struct {
	ID               int
	NodeID           string
	Number           int // per-repo sequential
	RepoID           int
	Title            string
	Body             string
	State            string // "OPEN", "CLOSED"
	StateReason      string // "", "COMPLETED", "NOT_PLANNED"
	AuthorID         int
	AssigneeIDs      []int
	LabelIDs         []int
	MilestoneID      int // 0 = none
	IssueTypeID      int // 0 = none; organization issue type ID
	Locked           bool
	ActiveLockReason string // "", "off-topic", "too heated", "resolved", "spam"
	CreatedAt        time.Time
	UpdatedAt        time.Time
	ClosedAt         *time.Time
}

// Comment represents a conversation comment on an issue or PR. Real
// GitHub stores both in the same table because PRs are issues internally;
// bleephub mirrors that by discriminating via ParentType ("issue" or
// "pull_request"). The legacy field name IssueID is preserved for
// existing call sites and now holds the issue *or* PR database ID
// depending on ParentType.
type Comment struct {
	ID              int
	NodeID          string
	ParentType      string // "issue" or "pull_request"
	IssueID         int    // issue or PR database ID per ParentType
	AuthorID        int
	Body            string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	LastEditedAt    *time.Time // nil when never edited after creation
	EditorID        int        // user who performed the last edit; 0 when never edited
	MinimizedReason string     // "" when not minimized; otherwise OFF_TOPIC / OUTDATED / RESOLVED / DUPLICATE / SPAM / ABUSE
	MinimizedByID   int        // user who minimized; 0 when not minimized
	Pinned          bool       // pinned comments appear first in some GitHub UIs
}

// IssueEvent represents an event in an issue's or a pull request's
// timeline. The Event field matches the GitHub issue-event type names used
// by the REST API ("opened", "closed", "reopened", "locked", "unlocked",
// "commented", "labeled", "unlabeled", "assigned", "unassigned",
// "review_requested", "review_request_removed", "merged", ...).
// ParentType says which ID space IssueID refers to: "issue" (st.Issues) or
// "pull_request" (st.PullRequests) — issues and PRs share the per-repo
// number sequence but have independent global ID sequences.
type IssueEvent struct {
	ID                  int
	NodeID              string
	RepoID              int
	ParentType          string
	IssueID             int
	ActorID             int
	Event               string
	CommitID            string
	CommitURL           string
	CreatedAt           time.Time
	LabelID             int
	AssigneeID          int
	AssignerID          int
	MilestoneID         int
	CommentID           int
	RequestedReviewerID int
	LockReason          string
	RenameFrom          string
	RenameTo            string
}

// recordIssueEventLocked creates an IssueEvent while st.mu is already held.
func (st *Store) recordIssueEventLocked(repoID, issueID, actorID int, event string) *IssueEvent {
	e := &IssueEvent{
		ID:         st.NextIssueEventID,
		NodeID:     fmt.Sprintf("IE_kgDO%08d", st.NextIssueEventID),
		RepoID:     repoID,
		ParentType: "issue",
		IssueID:    issueID,
		ActorID:    actorID,
		Event:      event,
		CreatedAt:  time.Now().UTC(),
	}
	st.NextIssueEventID++
	st.IssueEvents[e.ID] = e
	if st.persist != nil {
		st.persist.MustPut("issue_events", strconv.Itoa(e.ID), e)
	}
	return e
}

// recordPullRequestEventLocked creates an IssueEvent attached to a pull
// request while st.mu is already held. commitID and requestedReviewerID are
// optional (zero-valued when the event type carries neither).
func (st *Store) recordPullRequestEventLocked(repoID, prID, actorID int, event, commitID string, requestedReviewerID int) *IssueEvent {
	e := st.recordIssueEventLocked(repoID, prID, actorID, event)
	e.ParentType = "pull_request"
	e.CommitID = commitID
	e.RequestedReviewerID = requestedReviewerID
	if st.persist != nil {
		st.persist.MustPut("issue_events", strconv.Itoa(e.ID), e)
	}
	return e
}

// RecordPullRequestEvent creates a public issue event attached to a pull
// request ("merged", "closed", "reopened", "review_requested", ...).
func (st *Store) RecordPullRequestEvent(repoID, prID, actorID int, event, commitID string, requestedReviewerID int) *IssueEvent {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.recordPullRequestEventLocked(repoID, prID, actorID, event, commitID, requestedReviewerID)
}

// recordIssueEventWithIDsLocked creates an IssueEvent with optional related IDs
// while st.mu is already held.
func (st *Store) recordIssueEventWithIDsLocked(repoID, issueID, actorID int, event string, labelID, assigneeID, assignerID, milestoneID, commentID int) *IssueEvent {
	e := st.recordIssueEventLocked(repoID, issueID, actorID, event)
	e.LabelID = labelID
	e.AssigneeID = assigneeID
	e.AssignerID = assignerID
	e.MilestoneID = milestoneID
	e.CommentID = commentID
	if st.persist != nil {
		st.persist.MustPut("issue_events", strconv.Itoa(e.ID), e)
	}
	return e
}

// RecordIssueEvent creates a public issue event. The payload map may contain
// optional related IDs using the same keys GitHub uses: label_id, assignee_id,
// assigner_id, milestone_id, comment_id, commit_id, commit_url.
func (st *Store) RecordIssueEvent(repoID, issueID, actorID int, event string, payload map[string]interface{}) *IssueEvent {
	st.mu.Lock()
	defer st.mu.Unlock()

	labelID := intFromPayload(payload, "label_id")
	assigneeID := intFromPayload(payload, "assignee_id")
	assignerID := intFromPayload(payload, "assigner_id")
	milestoneID := intFromPayload(payload, "milestone_id")
	commentID := intFromPayload(payload, "comment_id")

	e := st.recordIssueEventWithIDsLocked(repoID, issueID, actorID, event, labelID, assigneeID, assignerID, milestoneID, commentID)
	if v, ok := payload["commit_id"].(string); ok {
		e.CommitID = v
	}
	if v, ok := payload["commit_url"].(string); ok {
		e.CommitURL = v
	}
	if v, ok := payload["lock_reason"].(string); ok {
		e.LockReason = v
	}
	if v, ok := payload["rename_from"].(string); ok {
		e.RenameFrom = v
	}
	if v, ok := payload["rename_to"].(string); ok {
		e.RenameTo = v
	}
	if st.persist != nil {
		st.persist.MustPut("issue_events", strconv.Itoa(e.ID), e)
	}
	return e
}

// intFromPayload extracts an int from a payload map, tolerating float64
// (the default JSON number type) and int.
func intFromPayload(payload map[string]interface{}, key string) int {
	v, ok := payload[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case string:
		if i, err := strconv.Atoi(n); err == nil {
			return i
		}
	}
	return 0
}

// ListIssueEvents returns issue events for a repo, optionally filtered by
// issue ID (pass 0 to get all repo issue events, pull-request events
// included — GitHub's repo-level events listing spans both). Per-issue
// listings exclude pull-request events: a PR's global ID can collide with
// an issue's. Results are ordered by event ID so pagination is stable.
func (st *Store) ListIssueEvents(repoID, issueID int) []*IssueEvent {
	st.mu.RLock()
	defer st.mu.RUnlock()
	var events []*IssueEvent
	for _, e := range st.IssueEvents {
		if e.RepoID != repoID {
			continue
		}
		if issueID != 0 && (e.IssueID != issueID || e.ParentType == "pull_request") {
			continue
		}
		events = append(events, e)
	}
	sort.Slice(events, func(i, j int) bool { return events[i].ID < events[j].ID })
	return events
}

// ListPullRequestEvents returns the issue events attached to a pull
// request, ordered by event ID.
func (st *Store) ListPullRequestEvents(repoID, prID int) []*IssueEvent {
	st.mu.RLock()
	defer st.mu.RUnlock()
	var events []*IssueEvent
	for _, e := range st.IssueEvents {
		if e.RepoID != repoID || e.ParentType != "pull_request" || e.IssueID != prID {
			continue
		}
		events = append(events, e)
	}
	sort.Slice(events, func(i, j int) bool { return events[i].ID < events[j].ID })
	return events
}

// ListRepoIssueEvents returns all issue events for a repository.
func (st *Store) ListRepoIssueEvents(repoID int) []*IssueEvent {
	return st.ListIssueEvents(repoID, 0)
}

// GetIssueEvent returns an issue event by global ID.
func (st *Store) GetIssueEvent(id int) *IssueEvent {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.IssueEvents[id]
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

	now := time.Now().UTC()
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
	if st.persist != nil {
		st.persist.MustPut("labels", strconv.Itoa(label.ID), label)
	}
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
	if st.persist != nil {
		st.persist.MustPut("labels", strconv.Itoa(l.ID), l)
	}
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
	if st.persist != nil {
		st.persist.MustDelete("labels", strconv.Itoa(id))
	}
	// Remove from any issues
	for _, issue := range st.Issues {
		for i, lid := range issue.LabelIDs {
			if lid == id {
				issue.LabelIDs = append(issue.LabelIDs[:i], issue.LabelIDs[i+1:]...)
				if st.persist != nil {
					st.persist.MustPut("issues", strconv.Itoa(issue.ID), issue)
				}
				break
			}
		}
	}
	return true
}

// --- Milestone CRUD ---

// CreateMilestone creates a new milestone in the given repository on
// behalf of the given creator.
func (st *Store) CreateMilestone(repoID, creatorID int, title, description, state string, dueOn *time.Time) *Milestone {
	st.mu.Lock()
	defer st.mu.Unlock()

	repo := st.Repos[repoID]
	if repo == nil {
		return nil
	}

	if state == "" {
		state = "open"
	}

	now := time.Now().UTC()
	ms := &Milestone{
		ID:          st.NextMilestone,
		NodeID:      fmt.Sprintf("MI_kgDO%08d", st.NextMilestone),
		RepoID:      repoID,
		Number:      repo.NextMilestoneNumber,
		Title:       title,
		Description: description,
		State:       state,
		CreatorID:   creatorID,
		DueOn:       dueOn,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	repo.NextMilestoneNumber++
	st.NextMilestone++
	st.Milestones[ms.ID] = ms
	if st.persist != nil {
		st.persist.MustPut("milestones", strconv.Itoa(ms.ID), ms)
	}
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
	ms.UpdatedAt = time.Now().UTC()
	if st.persist != nil {
		st.persist.MustPut("milestones", strconv.Itoa(ms.ID), ms)
	}
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
	if st.persist != nil {
		st.persist.MustDelete("milestones", strconv.Itoa(id))
	}
	// Detach from issues
	for _, issue := range st.Issues {
		if issue.MilestoneID == id {
			issue.MilestoneID = 0
			if st.persist != nil {
				st.persist.MustPut("issues", strconv.Itoa(issue.ID), issue)
			}
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

	now := time.Now().UTC()
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
	st.indexIssueLocked(issue)
	if st.persist != nil {
		st.persist.MustPut("issues", strconv.Itoa(issue.ID), issue)
	}
	st.recordIssueEventLocked(repoID, issue.ID, authorID, "opened")
	return issue
}

// indexIssueLocked records the issue in the per-repo secondary index so
// GetIssueByNumber and ListIssues resolve in O(issues-in-repo) instead of a
// full scan of every issue in the store. Caller holds st.mu.
func (st *Store) indexIssueLocked(issue *Issue) {
	m := st.IssuesByRepo[issue.RepoID]
	if m == nil {
		m = make(map[int]*Issue)
		st.IssuesByRepo[issue.RepoID] = m
	}
	m[issue.Number] = issue
}

// unindexIssueLocked removes the issue from the per-repo secondary index.
// Caller holds st.mu.
func (st *Store) unindexIssueLocked(issue *Issue) {
	if m := st.IssuesByRepo[issue.RepoID]; m != nil {
		delete(m, issue.Number)
		if len(m) == 0 {
			delete(st.IssuesByRepo, issue.RepoID)
		}
	}
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
	return st.IssuesByRepo[repoID][number]
}

// ListIssues returns issues for a repository, optionally filtered by state.
// State filter matches "OPEN"/"CLOSED"; empty or "all" returns all.
func (st *Store) ListIssues(repoID int, state string) []*Issue {
	st.mu.RLock()
	defer st.mu.RUnlock()
	var issues []*Issue
	for _, issue := range st.IssuesByRepo[repoID] {
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
	issue.UpdatedAt = time.Now().UTC()
	if st.persist != nil {
		st.persist.MustPut("issues", strconv.Itoa(issue.ID), issue)
	}
	return true
}

// issueByRepoKeyAndNumber resolves an issue by its repo key and number while
// holding the store lock. Returns nil when not found.
func (st *Store) issueByRepoKeyAndNumber(repoKey string, number int) *Issue {
	repo := st.ReposByName[repoKey]
	if repo == nil {
		return nil
	}
	return st.IssuesByRepo[repo.ID][number]
}

// issueByRepoIDAndNumber resolves an issue by its repo ID and number while
// holding the store lock. Returns nil when not found.
func (st *Store) issueByRepoIDAndNumber(repoID, number int) *Issue {
	return st.IssuesByRepo[repoID][number]
}

// AddIssueAssignees adds assignees to an issue, returning true when the issue
// exists. Duplicate IDs are ignored; events are recorded for each addition.
func (st *Store) AddIssueAssignees(repoID int, issueNumber int, assigneeIDs []int, actorID int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	issue := st.issueByRepoIDAndNumber(repoID, issueNumber)
	if issue == nil {
		return false
	}
	added := false
	for _, uid := range assigneeIDs {
		found := false
		for _, existing := range issue.AssigneeIDs {
			if existing == uid {
				found = true
				break
			}
		}
		if !found {
			issue.AssigneeIDs = append(issue.AssigneeIDs, uid)
			st.recordIssueEventWithIDsLocked(repoID, issue.ID, actorID, "assigned", 0, uid, actorID, 0, 0)
			added = true
		}
	}
	if added {
		issue.UpdatedAt = time.Now().UTC()
		if st.persist != nil {
			st.persist.MustPut("issues", strconv.Itoa(issue.ID), issue)
		}
	}
	return true
}

// RemoveIssueAssignees removes assignees from an issue, returning true when
// the issue exists. Events are recorded for each removal.
func (st *Store) RemoveIssueAssignees(repoID int, issueNumber int, assigneeIDs []int, actorID int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	issue := st.issueByRepoIDAndNumber(repoID, issueNumber)
	if issue == nil {
		return false
	}
	removed := false
	for _, uid := range assigneeIDs {
		for idx, existing := range issue.AssigneeIDs {
			if existing == uid {
				issue.AssigneeIDs = append(issue.AssigneeIDs[:idx], issue.AssigneeIDs[idx+1:]...)
				st.recordIssueEventWithIDsLocked(repoID, issue.ID, actorID, "unassigned", 0, uid, actorID, 0, 0)
				removed = true
				break
			}
		}
	}
	if removed {
		issue.UpdatedAt = time.Now().UTC()
		if st.persist != nil {
			st.persist.MustPut("issues", strconv.Itoa(issue.ID), issue)
		}
	}
	return true
}

// SetIssueLabels replaces all labels on an issue, recording labeled/unlabeled
// events for the deltas. Returns true when the issue exists.
func (st *Store) SetIssueLabels(repoID int, issueNumber int, labelIDs []int, actorID int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	issue := st.issueByRepoIDAndNumber(repoID, issueNumber)
	if issue == nil {
		return false
	}
	old := make(map[int]bool, len(issue.LabelIDs))
	for _, lid := range issue.LabelIDs {
		old[lid] = true
	}
	newSet := make(map[int]bool, len(labelIDs))
	for _, lid := range labelIDs {
		newSet[lid] = true
	}
	for _, lid := range issue.LabelIDs {
		if !newSet[lid] {
			st.recordIssueEventWithIDsLocked(repoID, issue.ID, actorID, "unlabeled", lid, 0, 0, 0, 0)
		}
	}
	for _, lid := range labelIDs {
		if !old[lid] {
			st.recordIssueEventWithIDsLocked(repoID, issue.ID, actorID, "labeled", lid, 0, 0, 0, 0)
		}
	}
	issue.LabelIDs = labelIDs
	issue.UpdatedAt = time.Now().UTC()
	if st.persist != nil {
		st.persist.MustPut("issues", strconv.Itoa(issue.ID), issue)
	}
	return true
}

// ClearIssueLabels removes every label from an issue, recording an unlabeled
// event for each previously-attached label.
func (st *Store) ClearIssueLabels(repoID int, issueNumber int, actorID int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	issue := st.issueByRepoIDAndNumber(repoID, issueNumber)
	if issue == nil {
		return false
	}
	if len(issue.LabelIDs) == 0 {
		return true
	}
	for _, lid := range issue.LabelIDs {
		st.recordIssueEventWithIDsLocked(repoID, issue.ID, actorID, "unlabeled", lid, 0, 0, 0, 0)
	}
	issue.LabelIDs = nil
	issue.UpdatedAt = time.Now().UTC()
	if st.persist != nil {
		st.persist.MustPut("issues", strconv.Itoa(issue.ID), issue)
	}
	return true
}

// AddIssueLabels adds labels to an issue, returning true when the issue
// exists. Duplicate IDs are ignored; events are recorded for each addition.
func (st *Store) AddIssueLabels(repoKey string, issueNumber int, labelIDs []int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	issue := st.issueByRepoKeyAndNumber(repoKey, issueNumber)
	if issue == nil {
		return false
	}
	repo := st.ReposByName[repoKey]
	added := false
	for _, lid := range labelIDs {
		found := false
		for _, existing := range issue.LabelIDs {
			if existing == lid {
				found = true
				break
			}
		}
		if !found {
			issue.LabelIDs = append(issue.LabelIDs, lid)
			st.recordIssueEventWithIDsLocked(repo.ID, issue.ID, 0, "labeled", lid, 0, 0, 0, 0)
			added = true
		}
	}
	if added {
		issue.UpdatedAt = time.Now().UTC()
		if st.persist != nil {
			st.persist.MustPut("issues", strconv.Itoa(issue.ID), issue)
		}
	}
	return true
}

// RemoveIssueLabel removes a single label from an issue by name, returning
// true when the issue and label exist and the label was attached.
func (st *Store) RemoveIssueLabel(repoKey string, issueNumber int, labelName string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	issue := st.issueByRepoKeyAndNumber(repoKey, issueNumber)
	if issue == nil {
		return false
	}
	repo := st.ReposByName[repoKey]
	var label *IssueLabel
	for _, l := range st.Labels {
		if l.RepoID == repo.ID && l.Name == labelName {
			label = l
			break
		}
	}
	if label == nil {
		return false
	}
	for idx, lid := range issue.LabelIDs {
		if lid == label.ID {
			issue.LabelIDs = append(issue.LabelIDs[:idx], issue.LabelIDs[idx+1:]...)
			issue.UpdatedAt = time.Now().UTC()
			if st.persist != nil {
				st.persist.MustPut("issues", strconv.Itoa(issue.ID), issue)
			}
			st.recordIssueEventWithIDsLocked(repo.ID, issue.ID, 0, "unlabeled", label.ID, 0, 0, 0, 0)
			return true
		}
	}
	return true
}

// LockIssue locks an issue, optionally recording a lock reason. Returns true
// when the issue exists.
func (st *Store) LockIssue(repoKey string, issueNumber int, lockReason string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	issue := st.issueByRepoKeyAndNumber(repoKey, issueNumber)
	if issue == nil {
		return false
	}
	issue.Locked = true
	issue.ActiveLockReason = lockReason
	issue.UpdatedAt = time.Now().UTC()
	if st.persist != nil {
		st.persist.MustPut("issues", strconv.Itoa(issue.ID), issue)
	}
	st.recordIssueEventLocked(issue.RepoID, issue.ID, 0, "locked")
	return true
}

// UnlockIssue unlocks an issue. Returns true when the issue exists.
func (st *Store) UnlockIssue(repoKey string, issueNumber int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	issue := st.issueByRepoKeyAndNumber(repoKey, issueNumber)
	if issue == nil {
		return false
	}
	issue.Locked = false
	issue.ActiveLockReason = ""
	issue.UpdatedAt = time.Now().UTC()
	if st.persist != nil {
		st.persist.MustPut("issues", strconv.Itoa(issue.ID), issue)
	}
	st.recordIssueEventLocked(issue.RepoID, issue.ID, 0, "unlocked")
	return true
}

// ListIssueComments returns all conversation comments for the issue with the
// given repo key and number.
func (st *Store) ListIssueComments(repoKey string, issueNumber int) []*Comment {
	st.mu.RLock()
	defer st.mu.RUnlock()
	issue := st.issueByRepoKeyAndNumber(repoKey, issueNumber)
	if issue == nil {
		return nil
	}
	var comments []*Comment
	for _, c := range st.Comments {
		if c.ParentType == "issue" && c.IssueID == issue.ID {
			comments = append(comments, c)
		}
	}
	return comments
}

// GetIssueComment returns a comment by global ID.
func (st *Store) GetIssueComment(id int) *Comment {
	return st.GetComment(id)
}

// DeleteIssueComment removes a comment by id. Returns true if removed.
func (st *Store) DeleteIssueComment(id int) bool {
	return st.DeleteComment(id)
}

// ListRepoIssueComments returns all issue comments across the repo, oldest
// first.
func (st *Store) ListRepoIssueComments(repoID int) []*Comment {
	st.mu.RLock()
	defer st.mu.RUnlock()
	var comments []*Comment
	for _, c := range st.Comments {
		if c.ParentType == "issue" && st.Issues[c.IssueID] != nil && st.Issues[c.IssueID].RepoID == repoID {
			comments = append(comments, c)
		}
	}
	sort.Slice(comments, func(i, j int) bool { return comments[i].CreatedAt.Before(comments[j].CreatedAt) })
	return comments
}

// PinIssueComment marks a comment as pinned. Returns true when the comment
// exists.
func (st *Store) PinIssueComment(commentID int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	c, ok := st.Comments[commentID]
	if !ok {
		return false
	}
	c.Pinned = true
	c.UpdatedAt = time.Now().UTC()
	if st.persist != nil {
		st.persist.MustPut("comments", strconv.Itoa(c.ID), c)
	}
	return true
}

// UnpinIssueComment clears a comment's pinned flag. Returns true when the
// comment exists.
func (st *Store) UnpinIssueComment(commentID int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	c, ok := st.Comments[commentID]
	if !ok {
		return false
	}
	c.Pinned = false
	c.UpdatedAt = time.Now().UTC()
	if st.persist != nil {
		st.persist.MustPut("comments", strconv.Itoa(c.ID), c)
	}
	return true
}

// BuildIssueTimeline returns a synthesized timeline for an issue by
// interleaving issue events and issue comments ordered by created_at.
func (st *Store) BuildIssueTimeline(repo *Repo, issueID int, baseURL string) []map[string]interface{} {
	// ListIssueEvents and ListCommentsFor take st.mu.RLock themselves;
	// holding it across the calls would re-acquire the read lock and can
	// deadlock against a queued writer.
	events := st.ListIssueEvents(repo.ID, issueID)
	comments := st.ListCommentsFor("issue", issueID)

	items := make([]timelineItem, 0, len(events)+len(comments))
	for _, e := range events {
		items = append(items, timelineItem{createdAt: e.CreatedAt, kind: "event", event: e})
	}
	for _, c := range comments {
		items = append(items, timelineItem{createdAt: c.CreatedAt, kind: "comment", comment: c})
	}
	sort.Slice(items, func(i, j int) bool {
		if !items[i].createdAt.Equal(items[j].createdAt) {
			return items[i].createdAt.Before(items[j].createdAt)
		}
		// Events before comments at identical timestamps for stability.
		if items[i].kind != items[j].kind {
			return items[i].kind == "event"
		}
		return items[i].id() < items[j].id()
	})

	out := make([]map[string]interface{}, 0, len(items))
	for _, ti := range items {
		switch ti.kind {
		case "event":
			out = append(out, issueEventForTimelineToJSON(ti.event, st, baseURL, repo.FullName))
		case "comment":
			parentNumber := 0
			if issue := st.GetIssue(ti.comment.IssueID); issue != nil {
				parentNumber = issue.Number
			}
			out = append(out, timelineCommentToJSON(ti.comment, st, baseURL, repo.FullName, parentNumber, repo))
		}
	}
	return out
}

// timelineItem is a helper used only by BuildIssueTimeline.
type timelineItem struct {
	createdAt time.Time
	kind      string
	event     *IssueEvent
	comment   *Comment
}

func (ti timelineItem) id() int {
	switch ti.kind {
	case "event":
		if ti.event != nil {
			return ti.event.ID
		}
	case "comment":
		if ti.comment != nil {
			return ti.comment.ID
		}
	}
	return 0
}

// --- Comment CRUD ---

// CreateComment creates a new conversation comment on an issue. Use
// CreateCommentFor for PR conversation comments — real GitHub stores
// both in the same table; bleephub mirrors that via ParentType.
func (st *Store) CreateComment(issueID, authorID int, body string) *Comment {
	return st.CreateCommentFor("issue", issueID, authorID, body)
}

// CreateCommentFor creates a comment on an issue (parentType="issue") or
// pull request (parentType="pull_request"). The parent must already
// exist in the matching store.
func (st *Store) CreateCommentFor(parentType string, parentID, authorID int, body string) *Comment {
	st.mu.Lock()
	defer st.mu.Unlock()
	switch parentType {
	case "issue":
		if _, ok := st.Issues[parentID]; !ok {
			return nil
		}
	case "pull_request":
		if _, ok := st.PullRequests[parentID]; !ok {
			return nil
		}
	default:
		return nil
	}

	now := time.Now().UTC()
	c := &Comment{
		ID:         st.NextComment,
		NodeID:     fmt.Sprintf("IC_kgDO%08d", st.NextComment),
		ParentType: parentType,
		IssueID:    parentID,
		AuthorID:   authorID,
		Body:       body,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	st.NextComment++
	st.Comments[c.ID] = c
	st.CommentCounts[commentCountKey(parentType, parentID)]++
	if st.persist != nil {
		st.persist.MustPut("comments", strconv.Itoa(c.ID), c)
	}
	// Record a timeline event for issue comments (PR comments get their own
	// review-comment machinery elsewhere).
	if parentType == "issue" {
		if issue := st.Issues[parentID]; issue != nil {
			st.recordIssueEventWithIDsLocked(issue.RepoID, issue.ID, authorID, "commented", 0, 0, 0, 0, c.ID)
		}
	}
	return c
}

// ListComments returns all conversation comments for an issue.
func (st *Store) ListComments(issueID int) []*Comment {
	return st.ListCommentsFor("issue", issueID)
}

// GetComment returns a comment by global ID.
func (st *Store) GetComment(id int) *Comment {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.Comments[id]
}

// DeleteComment removes a comment by id. Returns true if removed.
func (st *Store) DeleteComment(id int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	c, ok := st.Comments[id]
	if !ok {
		return false
	}
	delete(st.Comments, id)
	st.Reactions.DeleteParent(c.ParentType+"_comment", id)
	key := commentCountKey(c.ParentType, c.IssueID)
	if st.CommentCounts[key] <= 1 {
		delete(st.CommentCounts, key)
	} else {
		st.CommentCounts[key]--
	}
	if st.persist != nil {
		st.persist.MustDelete("comments", strconv.Itoa(id))
	}
	return true
}

// commentCountKey builds the CommentCounts index key for a comment parent.
func commentCountKey(parentType string, parentID int) string {
	return parentType + "\x1f" + strconv.Itoa(parentID)
}

// CountCommentsFor returns the number of conversation comments on the given
// parent (parentType "issue" or "pull_request") via the maintained index,
// avoiding a full scan of every comment in the store. Caller must NOT hold
// st.mu.
func (st *Store) CountCommentsFor(parentType string, parentID int) int {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.CommentCounts[commentCountKey(parentType, parentID)]
}

// countCommentsForLocked is the lock-free variant for callers already holding
// st.mu (the JSON serializers gather under one lock).
func (st *Store) countCommentsForLocked(parentType string, parentID int) int {
	return st.CommentCounts[commentCountKey(parentType, parentID)]
}

// GetIssueOrPullRequestIDByNumber returns the global ID of the issue or
// pull request with the given repo + number, or 0 when neither exists.
// PRs share the issue number namespace in GitHub, so the same number can
// identify either kind of object.
func (st *Store) GetIssueOrPullRequestIDByNumber(repoID, number int) int {
	st.mu.RLock()
	defer st.mu.RUnlock()
	for _, i := range st.Issues {
		if i.RepoID == repoID && i.Number == number {
			return i.ID
		}
	}
	for _, pr := range st.PullRequests {
		if pr.RepoID == repoID && pr.Number == number {
			return pr.ID
		}
	}
	return 0
}

// ResolveCommentParent resolves the issue or pull request with the given
// repo + number and returns its kind, global ID, number, and locked flag in a
// single read-locked pass. Callers must not read the mutable Locked flag off a
// shared *Issue/*PullRequest pointer themselves — SetIssueOrPRLock mutates it
// under the write lock, so the read has to happen under st.mu here.
func (st *Store) ResolveCommentParent(repoID, number int) (parentType string, parentID, parentNumber int, locked, found bool) {
	st.mu.RLock()
	defer st.mu.RUnlock()
	for _, i := range st.Issues {
		if i.RepoID == repoID && i.Number == number {
			return "issue", i.ID, i.Number, i.Locked, true
		}
	}
	for _, pr := range st.PullRequests {
		if pr.RepoID == repoID && pr.Number == number {
			return "pull_request", pr.ID, pr.Number, pr.Locked, true
		}
	}
	return "", 0, 0, false, false
}

// SetIssueOrPRLock toggles the locked flag on the issue or PR with the
// given repo + number. Returns true if a target was found and updated;
// false when no issue or PR matches. The reason is recorded only when
// locked=true.
func (st *Store) SetIssueOrPRLock(repoID, number int, locked bool, reason string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	for _, i := range st.Issues {
		if i.RepoID == repoID && i.Number == number {
			i.Locked = locked
			if locked {
				i.ActiveLockReason = reason
			} else {
				i.ActiveLockReason = ""
			}
			if st.persist != nil {
				st.persist.MustPut("issues", strconv.Itoa(i.ID), i)
			}
			return true
		}
	}
	for _, pr := range st.PullRequests {
		if pr.RepoID == repoID && pr.Number == number {
			pr.Locked = locked
			if locked {
				pr.ActiveLockReason = reason
			} else {
				pr.ActiveLockReason = ""
			}
			if st.persist != nil {
				st.persist.MustPut("pull_requests", strconv.Itoa(pr.ID), pr)
			}
			return true
		}
	}
	return false
}

// UpdateCommentBody mutates a comment's body and records the edit metadata
// (LastEditedAt + EditorID). Returns the updated comment or nil if no
// comment matches the id.
func (st *Store) UpdateCommentBody(id, editorID int, body string) *Comment {
	st.mu.Lock()
	defer st.mu.Unlock()
	c, ok := st.Comments[id]
	if !ok {
		return nil
	}
	now := time.Now().UTC()
	c.Body = body
	c.UpdatedAt = now
	c.LastEditedAt = &now
	c.EditorID = editorID
	if st.persist != nil {
		st.persist.MustPut("comments", strconv.Itoa(c.ID), c)
	}
	return c
}

// LookupCommentByNodeID returns the comment with the given GraphQL node ID,
// or nil if not found. Used by minimize / unminimize mutations that target
// comments via their global node ID.
func (st *Store) LookupCommentByNodeID(nodeID string) *Comment {
	st.mu.RLock()
	defer st.mu.RUnlock()
	for _, c := range st.Comments {
		if c.NodeID == nodeID {
			return c
		}
	}
	return nil
}

// SetCommentMinimization sets or clears a comment's minimization state.
// reason is one of OFF_TOPIC / OUTDATED / RESOLVED / DUPLICATE / SPAM /
// ABUSE to minimize; pass an empty string to unminimize. minimizerID is
// the user who performed the action (ignored when clearing).
func (st *Store) SetCommentMinimization(id, minimizerID int, reason string) *Comment {
	st.mu.Lock()
	defer st.mu.Unlock()
	c, ok := st.Comments[id]
	if !ok {
		return nil
	}
	if reason == "" {
		c.MinimizedReason = ""
		c.MinimizedByID = 0
	} else {
		c.MinimizedReason = reason
		c.MinimizedByID = minimizerID
	}
	if st.persist != nil {
		st.persist.MustPut("comments", strconv.Itoa(c.ID), c)
	}
	return c
}

// ListCommentsFor returns all conversation comments for an issue
// (parentType="issue") or pull request (parentType="pull_request").
func (st *Store) ListCommentsFor(parentType string, parentID int) []*Comment {
	st.mu.RLock()
	defer st.mu.RUnlock()
	var comments []*Comment
	for _, c := range st.Comments {
		if c.ParentType == parentType && c.IssueID == parentID {
			comments = append(comments, c)
		}
	}
	return comments
}
