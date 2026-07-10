package bleephub

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ThreadSubscription tracks a user's subscription to a notification thread.
type ThreadSubscription struct {
	Subscribed bool      `json:"subscribed"`
	Ignored    bool      `json:"ignored"`
	Reason     string    `json:"reason"`
	CreatedAt  time.Time `json:"created_at"`
}

// UserNotificationsState persists per-user notification read/subscription state.
type UserNotificationsState struct {
	LastReadAt         time.Time                      `json:"last_read_at,omitempty"`
	RepoLastReadAt     map[string]time.Time           `json:"repo_last_read_at,omitempty"`
	ReadThreadIDs      map[string]time.Time           `json:"read_thread_ids,omitempty"`
	DismissedThreadIDs map[string]bool                `json:"dismissed_thread_ids,omitempty"`
	Subscriptions      map[string]*ThreadSubscription `json:"subscriptions,omitempty"`
}

// notificationThreadSource is the underlying resource a notification thread points at.
type notificationThreadSource struct {
	Type        string
	ID          int
	RepoID      int
	Number      int
	Title       string
	UpdatedAt   time.Time
	AuthorID    int
	AssigneeIDs []int
}

// notificationThreadRow is one accepted thread source gathered under the
// read lock, carrying everything buildThread needs so rendering can happen
// after the lock is released.
type notificationThreadRow struct {
	src        notificationThreadSource
	repo       *Repo
	threadID   string
	reason     string
	unread     bool
	lastReadAt *time.Time
}

// ListNotifications builds notification threads for the given user.
// Thread sources are gathered under the read lock; rendering happens after
// release because buildThread embeds the repository via repoToJSON, which
// derives counters under the store lock itself.
// ListNotifications gathers, sorts and renders every accepted thread. It is a
// convenience wrapper over NotificationRows + BuildNotificationThreads; hot
// handlers should paginate the rows first and render only the page.
func (st *Store) ListNotifications(user *User, baseURL string, opts NotificationListOptions) []*NotificationThread {
	rows := st.NotificationRows(user, opts)
	return st.BuildNotificationThreads(rows, baseURL)
}

// BuildNotificationThreads renders a (typically already-paginated) slice of
// rows into notification threads. buildThread is expensive per row (it embeds
// repoToJSON and scans comments for the latest-comment URL), so callers should
// paginate the rows before calling this.
func (st *Store) BuildNotificationThreads(rows []notificationThreadRow, baseURL string) []*NotificationThread {
	threads := make([]*NotificationThread, len(rows))
	for i, row := range rows {
		threads[i] = st.buildThread(row, baseURL)
	}
	return threads
}

// NotificationRows gathers and sorts (newest-first) the lightweight thread rows
// the user should see, without rendering any of them. Rendering each row is
// expensive, so callers paginate these rows and render only the page.
func (st *Store) NotificationRows(user *User, opts NotificationListOptions) []notificationThreadRow {
	st.mu.RLock()

	state := st.notificationsStateViewLocked(user.ID)
	var rows []notificationThreadRow

	// Precompute the set of (parentType, parentID) the viewer commented on in
	// a single pass over st.Comments, so notificationReason is an O(1) map
	// lookup per thread instead of an O(all-comments) scan per thread (which
	// made this handler O((issues+PRs) × comments)).
	commentedOn := make(map[string]struct{})
	for _, c := range st.Comments {
		if c.AuthorID == user.ID {
			commentedOn[strings.ToLower(c.ParentType)+"\x1f"+strconv.Itoa(c.IssueID)] = struct{}{}
		}
	}

	add := func(src notificationThreadSource) {
		repo := st.Repos[src.RepoID]
		if repo == nil {
			return
		}
		if !canReadRepoLocked(st, user, repo) {
			return
		}
		if opts.RepoScope != "" && repo.FullName != opts.RepoScope {
			return
		}

		threadID := notificationThreadID(src.Type, src.ID)
		if state.DismissedThreadIDs[threadID] {
			return
		}

		reason := notificationReasonWithComments(user, src, commentedOn)
		if opts.Participating && reason == "subscribed" {
			return
		}

		updated := src.UpdatedAt
		if !opts.Since.IsZero() && updated.Before(opts.Since) {
			return
		}
		if !opts.Before.IsZero() && updated.After(opts.Before) {
			return
		}

		lastRead := state.LastReadAt
		if opts.RepoScope != "" {
			if r, ok := state.RepoLastReadAt[opts.RepoScope]; ok {
				if lastRead.IsZero() || r.After(lastRead) {
					lastRead = r
				}
			}
		}
		unread := lastRead.IsZero() || updated.After(lastRead)
		if readAt, ok := state.ReadThreadIDs[threadID]; ok {
			if !updated.After(readAt) {
				unread = false
			}
		}
		if !opts.All && !unread {
			return
		}

		rows = append(rows, notificationThreadRow{src, repo, threadID, reason, unread, lastReadAtFor(state, threadID)})
	}

	for _, issue := range st.Issues {
		add(notificationThreadSource{
			Type:        "Issue",
			ID:          issue.ID,
			RepoID:      issue.RepoID,
			Number:      issue.Number,
			Title:       issue.Title,
			UpdatedAt:   issue.UpdatedAt,
			AuthorID:    issue.AuthorID,
			AssigneeIDs: issue.AssigneeIDs,
		})
	}
	for _, pr := range st.PullRequests {
		add(notificationThreadSource{
			Type:        "PullRequest",
			ID:          pr.ID,
			RepoID:      pr.RepoID,
			Number:      pr.Number,
			Title:       pr.Title,
			UpdatedAt:   pr.UpdatedAt,
			AuthorID:    pr.AuthorID,
			AssigneeIDs: pr.AssigneeIDs,
		})
	}

	st.mu.RUnlock()

	sort.Slice(rows, func(i, j int) bool {
		return rows[i].src.UpdatedAt.After(rows[j].src.UpdatedAt)
	})

	return rows
}

// notificationReason derives the thread reason for a single thread. Callers
// hold st.mu (it reads st.Comments directly). Used for one-off thread lookups;
// the list path uses notificationReasonWithComments with a precomputed set.
func notificationReason(st *Store, user *User, src notificationThreadSource) string {
	if src.AuthorID == user.ID {
		return "author"
	}
	for _, aid := range src.AssigneeIDs {
		if aid == user.ID {
			return "assign"
		}
	}
	parentType := strings.ToLower(src.Type)
	for _, c := range st.Comments {
		if c.AuthorID == user.ID && c.IssueID == src.ID && strings.ToLower(c.ParentType) == parentType {
			return "comment"
		}
	}
	return "subscribed"
}

// notificationReasonWithComments derives the thread reason for the user using
// a precomputed set of the (parentType, parentID) pairs the user commented on
// (keyed "type\x1fid"). This keeps the per-thread cost O(1) rather than
// rescanning every comment in the store.
func notificationReasonWithComments(user *User, src notificationThreadSource, commentedOn map[string]struct{}) string {
	if src.AuthorID == user.ID {
		return "author"
	}
	for _, aid := range src.AssigneeIDs {
		if aid == user.ID {
			return "assign"
		}
	}
	if _, ok := commentedOn[strings.ToLower(src.Type)+"\x1f"+strconv.Itoa(src.ID)]; ok {
		return "comment"
	}
	return "subscribed"
}

func notificationThreadID(sourceType string, sourceID int) string {
	switch strings.ToLower(sourceType) {
	case "issue":
		return fmt.Sprintf("issue-%d", sourceID)
	case "pullrequest", "pull_request", "pull-request":
		return fmt.Sprintf("pull-request-%d", sourceID)
	default:
		return fmt.Sprintf("%s-%d", strings.ToLower(sourceType), sourceID)
	}
}

func parseNotificationThreadID(threadID string) (string, int, bool) {
	switch {
	case strings.HasPrefix(threadID, "issue-"):
		id, err := strconv.Atoi(strings.TrimPrefix(threadID, "issue-"))
		return "Issue", id, err == nil
	case strings.HasPrefix(threadID, "pull-request-"):
		id, err := strconv.Atoi(strings.TrimPrefix(threadID, "pull-request-"))
		return "PullRequest", id, err == nil
	default:
		id, err := strconv.Atoi(threadID)
		if err == nil {
			return "", id, true
		}
		return "", 0, false
	}
}

// lastReadAtFor derives the thread's last-read timestamp from the user's
// notification state. Callers hold st.mu (state is store-owned).
func lastReadAtFor(state *UserNotificationsState, threadID string) *time.Time {
	if readAt, ok := state.ReadThreadIDs[threadID]; ok {
		t := readAt
		return &t
	}
	if !state.LastReadAt.IsZero() {
		t := state.LastReadAt
		return &t
	}
	return nil
}

// buildThread renders one gathered notification thread row. Must not be
// called with st.mu held: it scans comments under its own read lock and
// embeds the repository via repoToJSON, which derives counters under the
// store lock itself.
func (st *Store) buildThread(row notificationThreadRow, baseURL string) *NotificationThread {
	src, repo, threadID := row.src, row.repo, row.threadID
	base := baseURL
	apiBase := baseURL + "/api/v3"
	var subjectURL, latestCommentURL, htmlURL string
	if src.Type == "Issue" {
		subjectURL = fmt.Sprintf("%s/api/v3/repos/%s/issues/%d", base, repo.FullName, src.Number)
		latestCommentURL = subjectURL + "/comments"
		htmlURL = fmt.Sprintf("%s/%s/issues/%d", base, repo.FullName, src.Number)
	} else {
		subjectURL = fmt.Sprintf("%s/api/v3/repos/%s/pulls/%d", base, repo.FullName, src.Number)
		latestCommentURL = fmt.Sprintf("%s/api/v3/repos/%s/issues/%d/comments", base, repo.FullName, src.Number)
		htmlURL = fmt.Sprintf("%s/%s/pull/%d", base, repo.FullName, src.Number)
	}

	// Find the most recent comment to set latest_comment_url to a concrete comment when available.
	var latestCommentID int
	var latestCommentAt time.Time
	st.mu.RLock()
	for _, c := range st.Comments {
		parentType := strings.ToLower(src.Type)
		if c.ParentType == parentType && c.IssueID == src.ID {
			if latestCommentID == 0 || c.CreatedAt.After(latestCommentAt) {
				latestCommentID = c.ID
				latestCommentAt = c.CreatedAt
			}
		}
	}
	st.mu.RUnlock()
	if latestCommentID != 0 {
		latestCommentURL = fmt.Sprintf("%s/api/v3/repos/%s/issues/comments/%d", base, repo.FullName, latestCommentID)
	}

	return &NotificationThread{
		ID:               threadID,
		Repository:       repoToJSON(repo, st, base),
		SubjectTitle:     src.Title,
		SubjectURL:       subjectURL,
		SubjectType:      src.Type,
		LatestCommentURL: latestCommentURL,
		HTMLURL:          htmlURL,
		Reason:           row.reason,
		Unread:           row.unread,
		UpdatedAt:        src.UpdatedAt,
		LastReadAt:       row.lastReadAt,
		SubscriptionURL:  fmt.Sprintf("%s/notifications/threads/%s/subscription", apiBase, threadID),
		URL:              fmt.Sprintf("%s/notifications/threads/%s", apiBase, threadID),
	}
}

// GetNotificationThread returns a single notification thread by ID.
// The thread source is gathered under the read lock; rendering happens after
// release because buildThread embeds the repository via repoToJSON, which
// derives counters under the store lock itself.
func (st *Store) GetNotificationThread(user *User, baseURL, threadID string) *NotificationThread {
	sourceType, id, ok := parseNotificationThreadID(threadID)
	if !ok {
		return nil
	}

	var row *notificationThreadRow
	st.mu.RLock()
	if sourceType == "Issue" || sourceType == "" {
		if issue := st.Issues[id]; issue != nil {
			row = st.notificationIssueRowLocked(user, issue, notificationThreadID("Issue", issue.ID))
		}
	}
	if row == nil && (sourceType == "PullRequest" || sourceType == "") {
		if pr := st.PullRequests[id]; pr != nil {
			row = st.notificationPullRequestRowLocked(user, pr, notificationThreadID("PullRequest", pr.ID))
		}
	}
	st.mu.RUnlock()

	if row == nil {
		return nil
	}
	return st.buildThread(*row, baseURL)
}

func (st *Store) notificationIssueRowLocked(user *User, issue *Issue, threadID string) *notificationThreadRow {
	repo := st.Repos[issue.RepoID]
	if repo == nil || !canReadRepoLocked(st, user, repo) {
		return nil
	}
	src := notificationThreadSource{
		Type:        "Issue",
		ID:          issue.ID,
		RepoID:      issue.RepoID,
		Number:      issue.Number,
		Title:       issue.Title,
		UpdatedAt:   issue.UpdatedAt,
		AuthorID:    issue.AuthorID,
		AssigneeIDs: issue.AssigneeIDs,
	}
	state := st.notificationsStateViewLocked(user.ID)
	return &notificationThreadRow{src, repo, threadID, notificationReason(st, user, src), true, lastReadAtFor(state, threadID)}
}

func (st *Store) notificationPullRequestRowLocked(user *User, pr *PullRequest, threadID string) *notificationThreadRow {
	repo := st.Repos[pr.RepoID]
	if repo == nil || !canReadRepoLocked(st, user, repo) {
		return nil
	}
	src := notificationThreadSource{
		Type:        "PullRequest",
		ID:          pr.ID,
		RepoID:      pr.RepoID,
		Number:      pr.Number,
		Title:       pr.Title,
		UpdatedAt:   pr.UpdatedAt,
		AuthorID:    pr.AuthorID,
		AssigneeIDs: pr.AssigneeIDs,
	}
	state := st.notificationsStateViewLocked(user.ID)
	return &notificationThreadRow{src, repo, threadID, notificationReason(st, user, src), true, lastReadAtFor(state, threadID)}
}

// MarkNotificationsRead sets the global last-read timestamp for the user.
func (st *Store) MarkNotificationsRead(userID int, at time.Time, repoScope string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	state := st.notificationsStateFor(userID)
	if repoScope != "" {
		if state.RepoLastReadAt == nil {
			state.RepoLastReadAt = map[string]time.Time{}
		}
		state.RepoLastReadAt[repoScope] = at
	} else {
		state.LastReadAt = at
	}
	st.persistNotificationsState(userID, state)
}

// MarkThreadRead records a thread as read for the user.
func (st *Store) MarkThreadRead(userID int, threadID string, at time.Time) {
	st.mu.Lock()
	defer st.mu.Unlock()
	state := st.notificationsStateFor(userID)
	if state.ReadThreadIDs == nil {
		state.ReadThreadIDs = map[string]time.Time{}
	}
	state.ReadThreadIDs[threadID] = at
	st.persistNotificationsState(userID, state)
}

// MarkThreadDone dismisses a thread for the user.
func (st *Store) MarkThreadDone(userID int, threadID string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	state := st.notificationsStateFor(userID)
	if state.DismissedThreadIDs == nil {
		state.DismissedThreadIDs = map[string]bool{}
	}
	state.DismissedThreadIDs[threadID] = true
	st.persistNotificationsState(userID, state)
}

// GetThreadSubscription returns the user's subscription for a thread.
func (st *Store) GetThreadSubscription(userID int, threadID string) *ThreadSubscription {
	st.mu.RLock()
	defer st.mu.RUnlock()
	state := st.notificationsStateViewLocked(userID)
	return state.Subscriptions[threadID]
}

// SetThreadSubscription sets or clears a thread subscription for the user.
func (st *Store) SetThreadSubscription(userID int, threadID string, sub *ThreadSubscription) {
	st.mu.Lock()
	defer st.mu.Unlock()
	state := st.notificationsStateFor(userID)
	if state.Subscriptions == nil {
		state.Subscriptions = map[string]*ThreadSubscription{}
	}
	if sub == nil {
		delete(state.Subscriptions, threadID)
	} else {
		state.Subscriptions[threadID] = sub
	}
	st.persistNotificationsState(userID, state)
}

func (st *Store) moveNotificationRepoKeyLocked(oldFull, newFull string) {
	for userID, state := range st.NotificationsState {
		if state == nil || state.RepoLastReadAt == nil {
			continue
		}
		if at, ok := state.RepoLastReadAt[oldFull]; ok {
			state.RepoLastReadAt[newFull] = at
			delete(state.RepoLastReadAt, oldFull)
			st.persistNotificationsState(userID, state)
		}
	}
}

func (st *Store) deleteNotificationRepoKeyLocked(fullName string) {
	for userID, state := range st.NotificationsState {
		if state == nil || state.RepoLastReadAt == nil {
			continue
		}
		if _, ok := state.RepoLastReadAt[fullName]; ok {
			delete(state.RepoLastReadAt, fullName)
			st.persistNotificationsState(userID, state)
		}
	}
}

func (st *Store) deleteNotificationThreadStateLocked(threadIDs []string) {
	if len(threadIDs) == 0 {
		return
	}
	for userID, state := range st.NotificationsState {
		if state == nil {
			continue
		}
		changed := false
		for _, threadID := range threadIDs {
			if state.ReadThreadIDs != nil {
				if _, ok := state.ReadThreadIDs[threadID]; ok {
					delete(state.ReadThreadIDs, threadID)
					changed = true
				}
			}
			if state.DismissedThreadIDs != nil {
				if _, ok := state.DismissedThreadIDs[threadID]; ok {
					delete(state.DismissedThreadIDs, threadID)
					changed = true
				}
			}
			if state.Subscriptions != nil {
				if _, ok := state.Subscriptions[threadID]; ok {
					delete(state.Subscriptions, threadID)
					changed = true
				}
			}
		}
		if changed {
			st.persistNotificationsState(userID, state)
		}
	}
}

// notificationsStateViewLocked returns the user's notification state for
// reading. Callers hold st.mu (read or write). Unlike notificationsStateFor
// it never mutates the store: a user with no recorded state gets a fresh
// zero-value view that is not inserted into the map (nil inner maps are safe
// to read).
func (st *Store) notificationsStateViewLocked(userID int) *UserNotificationsState {
	if state, ok := st.NotificationsState[userID]; ok {
		return state
	}
	return &UserNotificationsState{}
}

// notificationsStateFor returns the user's notification state, lazily
// creating and normalizing it. Callers hold st.mu for WRITING — it inserts
// into st.NotificationsState and repairs nil inner maps.
func (st *Store) notificationsStateFor(userID int) *UserNotificationsState {
	if st.NotificationsState == nil {
		st.NotificationsState = map[int]*UserNotificationsState{}
	}
	state, ok := st.NotificationsState[userID]
	if !ok {
		state = &UserNotificationsState{
			RepoLastReadAt:     map[string]time.Time{},
			ReadThreadIDs:      map[string]time.Time{},
			DismissedThreadIDs: map[string]bool{},
			Subscriptions:      map[string]*ThreadSubscription{},
		}
		st.NotificationsState[userID] = state
	}
	// Ensure maps are non-nil after loading from persistence.
	if state.RepoLastReadAt == nil {
		state.RepoLastReadAt = map[string]time.Time{}
	}
	if state.ReadThreadIDs == nil {
		state.ReadThreadIDs = map[string]time.Time{}
	}
	if state.DismissedThreadIDs == nil {
		state.DismissedThreadIDs = map[string]bool{}
	}
	if state.Subscriptions == nil {
		state.Subscriptions = map[string]*ThreadSubscription{}
	}
	return state
}

func (st *Store) persistNotificationsState(userID int, state *UserNotificationsState) {
	if st.persist != nil {
		st.persist.MustPut("notifications_state", strconv.Itoa(userID), state)
	}
}

// NotificationThread is the wire-shape of a GitHub notification thread.
type NotificationThread struct {
	ID               string
	Repository       map[string]interface{}
	SubjectTitle     string
	SubjectURL       string
	SubjectType      string
	LatestCommentURL string
	HTMLURL          string
	Reason           string
	Unread           bool
	UpdatedAt        time.Time
	LastReadAt       *time.Time
	SubscriptionURL  string
	URL              string
}

// NotificationListOptions controls filtering of ListNotifications.
type NotificationListOptions struct {
	All           bool
	Participating bool
	Since         time.Time
	Before        time.Time
	RepoScope     string
}
