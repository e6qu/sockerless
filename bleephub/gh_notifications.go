package bleephub

import (
	"net/http"
	"time"
)

func (s *Server) registerGHNotificationsRoutes() {
	s.route("GET /api/v3/notifications", s.handleListNotifications)
	s.route("PUT /api/v3/notifications", s.handleMarkNotificationsRead)
	s.route("GET /api/v3/repos/{owner}/{repo}/notifications", s.handleListRepoNotifications)
	s.route("PUT /api/v3/repos/{owner}/{repo}/notifications", s.handleMarkRepoNotificationsRead)
	s.route("GET /api/v3/notifications/threads/{thread_id}", s.handleGetThread)
	s.route("PATCH /api/v3/notifications/threads/{thread_id}", s.handlePatchThread)
	s.route("DELETE /api/v3/notifications/threads/{thread_id}", s.handleDeleteThread)
	s.route("GET /api/v3/notifications/threads/{thread_id}/subscription", s.handleGetThreadSubscription)
	s.route("PUT /api/v3/notifications/threads/{thread_id}/subscription", s.handleSetThreadSubscription)
	s.route("DELETE /api/v3/notifications/threads/{thread_id}/subscription", s.handleDeleteThreadSubscription)
}

func parseNotificationListOptions(r *http.Request) NotificationListOptions {
	opts := NotificationListOptions{}
	q := r.URL.Query()
	if q.Get("all") == "true" {
		opts.All = true
	}
	if q.Get("participating") == "true" {
		opts.Participating = true
	}
	if v := q.Get("since"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			opts.Since = t
		}
	}
	if v := q.Get("before"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			opts.Before = t
		}
	}
	return opts
}

func (s *Server) handleListNotifications(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}

	opts := parseNotificationListOptions(r)
	rows := s.store.NotificationRows(user, opts)
	rows = paginateAndLink(w, r, rows)
	threads := s.store.BuildNotificationThreads(rows, s.baseURL(r))
	out := make([]map[string]interface{}, len(threads))
	for i, t := range threads {
		out[i] = threadToJSON(t)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleMarkNotificationsRead(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}

	at := time.Now().UTC()
	var body struct {
		LastReadAt string `json:"last_read_at"`
	}
	if decodeJSONBodyOptional(w, r, &body) && body.LastReadAt != "" {
		if t, err := time.Parse(time.RFC3339, body.LastReadAt); err == nil {
			at = t
		}
	}
	s.store.MarkNotificationsRead(user.ID, at, "")
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) handleListRepoNotifications(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}

	owner, repoName := r.PathValue("owner"), r.PathValue("repo")
	repo := s.store.ReposByName[owner+"/"+repoName]
	if repo == nil || !canReadRepo(s.store, user, repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	opts := parseNotificationListOptions(r)
	opts.RepoScope = repo.FullName
	rows := s.store.NotificationRows(user, opts)
	rows = paginateAndLink(w, r, rows)
	threads := s.store.BuildNotificationThreads(rows, s.baseURL(r))
	out := make([]map[string]interface{}, len(threads))
	for i, t := range threads {
		out[i] = threadToJSON(t)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleMarkRepoNotificationsRead(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}

	owner, repoName := r.PathValue("owner"), r.PathValue("repo")
	repo := s.store.ReposByName[owner+"/"+repoName]
	if repo == nil || !canReadRepo(s.store, user, repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	at := time.Now().UTC()
	var body struct {
		LastReadAt string `json:"last_read_at"`
	}
	if decodeJSONBodyOptional(w, r, &body) && body.LastReadAt != "" {
		if t, err := time.Parse(time.RFC3339, body.LastReadAt); err == nil {
			at = t
		}
	}
	s.store.MarkNotificationsRead(user.ID, at, repo.FullName)
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) handleGetThread(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}

	thread := s.store.GetNotificationThread(user, s.baseURL(r), r.PathValue("thread_id"))
	if thread == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, threadToJSON(thread))
}

func (s *Server) handlePatchThread(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}

	threadID := r.PathValue("thread_id")
	thread := s.store.GetNotificationThread(user, s.baseURL(r), threadID)
	if thread == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	var body struct {
		Ignored string `json:"ignored"`
	}
	if !decodeJSONBodyOptional(w, r, &body) {
		return
	}

	if body.Ignored == "true" {
		s.store.MarkThreadDone(user.ID, threadID)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	s.store.MarkThreadRead(user.ID, threadID, time.Now().UTC())
	w.WriteHeader(http.StatusResetContent)
}

func (s *Server) handleGetThreadSubscription(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}

	threadID := r.PathValue("thread_id")
	thread := s.store.GetNotificationThread(user, s.baseURL(r), threadID)
	if thread == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	sub := s.store.GetThreadSubscription(user.ID, threadID)
	if sub == nil {
		// GitHub returns 404 when no explicit subscription exists.
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, threadSubscriptionToJSON(sub, thread.SubscriptionURL))
}

func (s *Server) handleSetThreadSubscription(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}

	threadID := r.PathValue("thread_id")
	thread := s.store.GetNotificationThread(user, s.baseURL(r), threadID)
	if thread == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	var body struct {
		Subscribed bool `json:"subscribed"`
		Ignored    bool `json:"ignored"`
	}
	if !decodeJSONBodyOptional(w, r, &body) {
		return
	}

	sub := &ThreadSubscription{
		Subscribed: body.Subscribed,
		Ignored:    body.Ignored,
		Reason:     thread.Reason,
		CreatedAt:  time.Now().UTC(),
	}
	s.store.SetThreadSubscription(user.ID, threadID, sub)
	writeJSON(w, http.StatusOK, threadSubscriptionToJSON(sub, thread.SubscriptionURL))
}

func (s *Server) handleDeleteThreadSubscription(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}

	threadID := r.PathValue("thread_id")
	if s.store.GetNotificationThread(user, s.baseURL(r), threadID) == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	s.store.SetThreadSubscription(user.ID, threadID, nil)
	w.WriteHeader(http.StatusNoContent)
}

func threadToJSON(t *NotificationThread) map[string]interface{} {
	m := map[string]interface{}{
		"id":         t.ID,
		"repository": t.Repository,
		"subject": map[string]interface{}{
			"title":              t.SubjectTitle,
			"url":                t.SubjectURL,
			"latest_comment_url": t.LatestCommentURL,
			"type":               t.SubjectType,
		},
		"reason":           t.Reason,
		"unread":           t.Unread,
		"updated_at":       t.UpdatedAt.UTC().Format(time.RFC3339),
		"last_read_at":     nil,
		"subscription_url": t.SubscriptionURL,
		"url":              t.URL,
	}
	if t.LastReadAt != nil {
		m["last_read_at"] = t.LastReadAt.UTC().Format(time.RFC3339)
	}
	return m
}

func threadSubscriptionToJSON(sub *ThreadSubscription, url string) map[string]interface{} {
	return map[string]interface{}{
		"subscribed": sub.Subscribed,
		"ignored":    sub.Ignored,
		"reason":     sub.Reason,
		"created_at": sub.CreatedAt.UTC().Format(time.RFC3339),
		"url":        url,
		"thread_url": url,
	}
}

// handleDeleteThread implements DELETE /notifications/threads/{thread_id}
// ("Mark a thread as done"): the thread is dismissed from the user's
// notification list.
func (s *Server) handleDeleteThread(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}

	threadID := r.PathValue("thread_id")
	thread := s.store.GetNotificationThread(user, s.baseURL(r), threadID)
	if thread == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.store.MarkThreadDone(user.ID, threadID)
	w.WriteHeader(http.StatusNoContent)
}
