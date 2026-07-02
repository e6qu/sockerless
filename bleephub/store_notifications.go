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

// ListNotifications builds notification threads for the given user.
func (st *Store) ListNotifications(user *User, baseURL string, opts NotificationListOptions) []*NotificationThread {
	st.mu.RLock()
	defer st.mu.RUnlock()

	state := st.notificationsStateFor(user.ID)
	var threads []*NotificationThread

	add := func(src notificationThreadSource) {
		repo := st.Repos[src.RepoID]
		if repo == nil {
			return
		}
		if !canReadRepo(st, user, repo) {
			return
		}
		if opts.RepoScope != "" && repo.FullName != opts.RepoScope {
			return
		}

		threadID := fmt.Sprintf("%d", src.ID)
		if state.DismissedThreadIDs[threadID] {
			return
		}

		reason := notificationReason(st, user, src)
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

		threads = append(threads, st.buildThread(src, repo, threadID, reason, unread, baseURL, state))
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

	sort.Slice(threads, func(i, j int) bool {
		return threads[i].UpdatedAt.After(threads[j].UpdatedAt)
	})

	return threads
}

func notificationReason(st *Store, user *User, src notificationThreadSource) string {
	if src.AuthorID == user.ID {
		return "author"
	}
	for _, aid := range src.AssigneeIDs {
		if aid == user.ID {
			return "assign"
		}
	}
	for _, c := range st.Comments {
		parentID := src.ID
		parentType := strings.ToLower(src.Type)
		if c.AuthorID == user.ID && c.IssueID == parentID && strings.ToLower(c.ParentType) == parentType {
			return "comment"
		}
	}
	return "subscribed"
}

func (st *Store) buildThread(src notificationThreadSource, repo *Repo, threadID, reason string, unread bool, baseURL string, state *UserNotificationsState) *NotificationThread {
	base := baseURL
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
	var latestComment *Comment
	for _, c := range st.Comments {
		parentType := strings.ToLower(src.Type)
		if c.ParentType == parentType && c.IssueID == src.ID {
			if latestComment == nil || c.CreatedAt.After(latestComment.CreatedAt) {
				latestComment = c
			}
		}
	}
	if latestComment != nil {
		latestCommentURL = fmt.Sprintf("%s/api/v3/repos/%s/issues/comments/%d", base, repo.FullName, latestComment.ID)
	}

	var lastReadAt *time.Time
	if readAt, ok := state.ReadThreadIDs[threadID]; ok {
		t := readAt
		lastReadAt = &t
	} else if !state.LastReadAt.IsZero() {
		t := state.LastReadAt
		lastReadAt = &t
	}

	return &NotificationThread{
		ID:               threadID,
		Repository:       repoToJSON(repo, st, base),
		SubjectTitle:     src.Title,
		SubjectURL:       subjectURL,
		SubjectType:      src.Type,
		LatestCommentURL: latestCommentURL,
		HTMLURL:          htmlURL,
		Reason:           reason,
		Unread:           unread,
		UpdatedAt:        src.UpdatedAt,
		LastReadAt:       lastReadAt,
		SubscriptionURL:  fmt.Sprintf("%s/notifications/threads/%s/subscription", base, threadID),
		URL:              fmt.Sprintf("%s/notifications/threads/%s", base, threadID),
	}
}

// GetNotificationThread returns a single synthetic thread by ID.
func (st *Store) GetNotificationThread(user *User, baseURL, threadID string) *NotificationThread {
	st.mu.RLock()
	defer st.mu.RUnlock()

	id, err := strconv.Atoi(threadID)
	if err != nil {
		return nil
	}
	for _, issue := range st.Issues {
		if issue.ID == id {
			state := st.notificationsStateFor(user.ID)
			repo := st.Repos[issue.RepoID]
			if repo == nil || !canReadRepo(st, user, repo) {
				return nil
			}
			reason := notificationReason(st, user, notificationThreadSource{
				Type:        "Issue",
				ID:          issue.ID,
				RepoID:      issue.RepoID,
				Number:      issue.Number,
				Title:       issue.Title,
				UpdatedAt:   issue.UpdatedAt,
				AuthorID:    issue.AuthorID,
				AssigneeIDs: issue.AssigneeIDs,
			})
			return st.buildThread(notificationThreadSource{
				Type:        "Issue",
				ID:          issue.ID,
				RepoID:      issue.RepoID,
				Number:      issue.Number,
				Title:       issue.Title,
				UpdatedAt:   issue.UpdatedAt,
				AuthorID:    issue.AuthorID,
				AssigneeIDs: issue.AssigneeIDs,
			}, repo, threadID, reason, true, baseURL, state)
		}
	}
	for _, pr := range st.PullRequests {
		if pr.ID == id {
			state := st.notificationsStateFor(user.ID)
			repo := st.Repos[pr.RepoID]
			if repo == nil || !canReadRepo(st, user, repo) {
				return nil
			}
			reason := notificationReason(st, user, notificationThreadSource{
				Type:        "PullRequest",
				ID:          pr.ID,
				RepoID:      pr.RepoID,
				Number:      pr.Number,
				Title:       pr.Title,
				UpdatedAt:   pr.UpdatedAt,
				AuthorID:    pr.AuthorID,
				AssigneeIDs: pr.AssigneeIDs,
			})
			return st.buildThread(notificationThreadSource{
				Type:        "PullRequest",
				ID:          pr.ID,
				RepoID:      pr.RepoID,
				Number:      pr.Number,
				Title:       pr.Title,
				UpdatedAt:   pr.UpdatedAt,
				AuthorID:    pr.AuthorID,
				AssigneeIDs: pr.AssigneeIDs,
			}, repo, threadID, reason, true, baseURL, state)
		}
	}
	return nil
}

// GetNotificationThreadByID returns a single synthetic thread by numeric ID.
func (st *Store) GetNotificationThreadByID(userID int, baseURL string, threadID int) *NotificationThread {
	user := st.GetUserByID(userID)
	if user == nil {
		return nil
	}
	return st.GetNotificationThread(user, baseURL, strconv.Itoa(threadID))
}

// MarkThreadReadByID records a thread as read for the user by numeric ID.
func (st *Store) MarkThreadReadByID(userID int, threadID int, at time.Time) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	state := st.notificationsStateFor(userID)
	if state.ReadThreadIDs == nil {
		state.ReadThreadIDs = map[string]time.Time{}
	}
	state.ReadThreadIDs[strconv.Itoa(threadID)] = at
	st.persistNotificationsState(userID, state)
	return true
}

// GetThreadSubscriptionByID returns the user's subscription for a numeric thread ID.
func (st *Store) GetThreadSubscriptionByID(userID int, threadID int) *ThreadSubscription {
	st.mu.RLock()
	defer st.mu.RUnlock()
	state := st.notificationsStateFor(userID)
	return state.Subscriptions[strconv.Itoa(threadID)]
}

// SetThreadSubscriptionByID sets a thread subscription for the user by numeric ID.
func (st *Store) SetThreadSubscriptionByID(userID int, threadID int, subscribed bool) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	state := st.notificationsStateFor(userID)
	if state.Subscriptions == nil {
		state.Subscriptions = map[string]*ThreadSubscription{}
	}
	state.Subscriptions[strconv.Itoa(threadID)] = &ThreadSubscription{
		Subscribed: subscribed,
		CreatedAt:  time.Now().UTC(),
	}
	st.persistNotificationsState(userID, state)
	return true
}

// DeleteThreadSubscriptionByID removes a thread subscription for the user by numeric ID.
func (st *Store) DeleteThreadSubscriptionByID(userID int, threadID int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	state := st.notificationsStateFor(userID)
	if state.Subscriptions == nil {
		return false
	}
	id := strconv.Itoa(threadID)
	if state.Subscriptions[id] == nil {
		return false
	}
	delete(state.Subscriptions, id)
	st.persistNotificationsState(userID, state)
	return true
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
	state := st.notificationsStateFor(userID)
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
