package bleephub

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNotifications_ListAndRead(t *testing.T) {
	s := newTestServer()
	s.registerGHNotificationsRoutes()

	admin := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(admin, "notif-repo", "", false)
	issue := s.store.CreateIssue(repo.ID, admin.ID, "bug", "body", nil, nil, 0)
	_ = issue

	do := func(method, path string, body []byte) *httptest.ResponseRecorder {
		var req *http.Request
		if body != nil {
			req = httptest.NewRequest(method, path, bytes.NewReader(body))
		} else {
			req = httptest.NewRequest(method, path, nil)
		}
		req.Header.Set("Authorization", "Bearer bleephub-admin-token-00000000000000000000")
		w := httptest.NewRecorder()
		s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
		return w
	}

	w := do("GET", "/api/v3/notifications", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list: %d body=%s", w.Code, w.Body.String())
	}
	var threads []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &threads); err != nil {
		t.Fatalf("unmarshal threads: %v", err)
	}
	if len(threads) != 1 {
		t.Fatalf("expected 1 thread, got %d", len(threads))
	}
	thread := threads[0]
	if thread["reason"] != "author" {
		t.Errorf("reason = %v", thread["reason"])
	}
	if thread["unread"] != true {
		t.Errorf("expected unread true, got %v", thread["unread"])
	}
	subject, ok := thread["subject"].(map[string]any)
	if !ok || subject["type"] != "Issue" {
		t.Errorf("subject type = %v", subject["type"])
	}

	threadID := thread["id"].(string)

	// Mark thread read returns 205.
	w = do("PATCH", "/api/v3/notifications/threads/"+threadID, nil)
	if w.Code != http.StatusResetContent {
		t.Errorf("mark read: %d", w.Code)
	}

	// Re-fetch with all=true: thread is now read.
	w = do("GET", "/api/v3/notifications?all=true", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list after read: %d", w.Code)
	}
	threads = nil
	json.Unmarshal(w.Body.Bytes(), &threads)
	if len(threads) != 1 {
		t.Fatalf("expected 1 thread, got %d", len(threads))
	}
	if threads[0]["unread"] != false {
		t.Errorf("expected unread false, got %v", threads[0]["unread"])
	}

	// Mark all notifications read.
	w = do("PUT", "/api/v3/notifications", nil)
	if w.Code != http.StatusAccepted {
		t.Errorf("mark all read: %d", w.Code)
	}
}

func TestNotifications_ThreadIDsSeparateIssuesAndPullRequests(t *testing.T) {
	s := newTestServer()
	s.registerGHNotificationsRoutes()

	admin := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(admin, "notif-collisions", "", false)
	seedPullRequestBranches(t, s, repo, "feature")
	issue := s.store.CreateIssue(repo.ID, admin.ID, "issue notification", "body", nil, nil, 0)
	pr := s.store.CreatePullRequest(repo.ID, admin.ID, "pull request notification", "body", "feature", "main", false, nil, nil, 0)
	if issue == nil || pr == nil {
		t.Fatalf("failed to create notification sources: issue=%v pullRequest=%v", issue, pr)
	}
	if issue.ID != pr.ID {
		t.Fatalf("test setup expected colliding source IDs, issue=%d pullRequest=%d", issue.ID, pr.ID)
	}

	do := func(method, path string, body []byte) *httptest.ResponseRecorder {
		var req *http.Request
		if body != nil {
			req = httptest.NewRequest(method, path, bytes.NewReader(body))
		} else {
			req = httptest.NewRequest(method, path, nil)
		}
		req.Header.Set("Authorization", "Bearer bleephub-admin-token-00000000000000000000")
		w := httptest.NewRecorder()
		s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
		return w
	}

	w := do("GET", "/api/v3/notifications", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list: %d body=%s", w.Code, w.Body.String())
	}
	var threads []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &threads); err != nil {
		t.Fatalf("unmarshal threads: %v", err)
	}
	if len(threads) != 2 {
		t.Fatalf("threads len = %d, want 2: %+v", len(threads), threads)
	}

	var issueID, pullRequestID string
	for _, thread := range threads {
		subject := thread["subject"].(map[string]any)
		switch subject["type"] {
		case "Issue":
			issueID = thread["id"].(string)
		case "PullRequest":
			pullRequestID = thread["id"].(string)
		}
		for _, field := range []string{"url", "subscription_url"} {
			got, _ := thread[field].(string)
			if got == "" || !strings.Contains(got, "/api/v3/notifications/threads/") {
				t.Fatalf("%s = %q, want GitHub REST notification URL", field, got)
			}
		}
	}
	if issueID == "" || pullRequestID == "" {
		t.Fatalf("missing typed notification IDs: issue=%q pullRequest=%q threads=%+v", issueID, pullRequestID, threads)
	}
	if issueID == pullRequestID {
		t.Fatalf("thread IDs collided: issue=%q pullRequest=%q", issueID, pullRequestID)
	}

	w = do("PATCH", "/api/v3/notifications/threads/"+issueID, nil)
	if w.Code != http.StatusResetContent {
		t.Fatalf("mark issue read: %d body=%s", w.Code, w.Body.String())
	}

	w = do("GET", "/api/v3/notifications?all=true", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list all: %d body=%s", w.Code, w.Body.String())
	}
	threads = nil
	if err := json.Unmarshal(w.Body.Bytes(), &threads); err != nil {
		t.Fatalf("unmarshal all threads: %v", err)
	}
	unreadByID := map[string]bool{}
	for _, thread := range threads {
		unreadByID[thread["id"].(string)] = thread["unread"].(bool)
	}
	if unreadByID[issueID] {
		t.Fatalf("issue thread stayed unread after marking it read: %+v", unreadByID)
	}
	if !unreadByID[pullRequestID] {
		t.Fatalf("pull request thread inherited issue read state: %+v", unreadByID)
	}

	w = do("GET", "/api/v3/notifications/threads/"+pullRequestID, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("get pull request thread: %d body=%s", w.Code, w.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal pull request thread: %v", err)
	}
	subject := got["subject"].(map[string]any)
	if subject["type"] != "PullRequest" || subject["title"] != "pull request notification" {
		t.Fatalf("pull request thread subject = %+v", subject)
	}
}

func TestNotifications_ParticipatingFilter(t *testing.T) {
	s := newTestServer()
	s.registerGHNotificationsRoutes()

	admin := s.store.UsersByLogin["admin"]
	s.store.mu.Lock()
	other := &User{ID: s.store.NextUser, Login: "other", Type: "User"}
	s.store.NextUser++
	s.store.Users[other.ID] = other
	s.store.UsersByLogin[other.Login] = other
	s.store.mu.Unlock()
	repo := s.store.CreateRepo(admin, "part-repo", "", false)
	// Add other as collaborator with pull so they can read.
	s.store.AddRepoCollaborator(repo.Owner.Login, repo.Name, other.Login, "pull")
	issue := s.store.CreateIssue(repo.ID, admin.ID, "bug", "body", nil, nil, 0)
	_ = issue

	do := func(token, method, path string, body []byte) *httptest.ResponseRecorder {
		var req *http.Request
		if body != nil {
			req = httptest.NewRequest(method, path, bytes.NewReader(body))
		} else {
			req = httptest.NewRequest(method, path, nil)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
		return w
	}

	otherToken := s.store.CreateToken(other.ID, "repo").Value

	w := do(otherToken, "GET", "/api/v3/notifications?participating=true", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list participating: %d body=%s", w.Code, w.Body.String())
	}
	var threads []map[string]any
	json.Unmarshal(w.Body.Bytes(), &threads)
	if len(threads) != 0 {
		t.Errorf("expected 0 participating threads for non-participant, got %d", len(threads))
	}

	w = do(otherToken, "GET", "/api/v3/notifications", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list all: %d", w.Code)
	}
	threads = nil
	json.Unmarshal(w.Body.Bytes(), &threads)
	if len(threads) != 1 || threads[0]["reason"] != "subscribed" {
		t.Errorf("expected subscribed thread, got %+v", threads)
	}
}

func TestNotifications_RepoScoped(t *testing.T) {
	s := newTestServer()
	s.registerGHNotificationsRoutes()

	admin := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(admin, "repo-notif", "", false)
	issue := s.store.CreateIssue(repo.ID, admin.ID, "bug", "body", nil, nil, 0)
	_ = issue

	do := func(method, path string, body []byte) *httptest.ResponseRecorder {
		var req *http.Request
		if body != nil {
			req = httptest.NewRequest(method, path, bytes.NewReader(body))
		} else {
			req = httptest.NewRequest(method, path, nil)
		}
		req.Header.Set("Authorization", "Bearer bleephub-admin-token-00000000000000000000")
		w := httptest.NewRecorder()
		s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
		return w
	}

	w := do("GET", "/api/v3/repos/admin/repo-notif/notifications", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("repo notifications: %d body=%s", w.Code, w.Body.String())
	}
	var threads []map[string]any
	json.Unmarshal(w.Body.Bytes(), &threads)
	if len(threads) != 1 {
		t.Errorf("expected 1 repo thread, got %d", len(threads))
	}

	// Mark repo notifications read.
	w = do("PUT", "/api/v3/repos/admin/repo-notif/notifications", nil)
	if w.Code != http.StatusAccepted {
		t.Errorf("mark repo read: %d", w.Code)
	}
}

func TestNotifications_ThreadSubscription(t *testing.T) {
	s := newTestServer()
	s.registerGHNotificationsRoutes()

	admin := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(admin, "sub-repo", "", false)
	issue := s.store.CreateIssue(repo.ID, admin.ID, "bug", "body", nil, nil, 0)
	_ = issue

	do := func(method, path string, body []byte) *httptest.ResponseRecorder {
		var req *http.Request
		if body != nil {
			req = httptest.NewRequest(method, path, bytes.NewReader(body))
		} else {
			req = httptest.NewRequest(method, path, nil)
		}
		req.Header.Set("Authorization", "Bearer bleephub-admin-token-00000000000000000000")
		w := httptest.NewRecorder()
		s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
		return w
	}

	w := do("GET", "/api/v3/notifications", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list: %d", w.Code)
	}
	var threads []map[string]any
	json.Unmarshal(w.Body.Bytes(), &threads)
	threadID := threads[0]["id"].(string)

	// No subscription yet → 404.
	w = do("GET", "/api/v3/notifications/threads/"+threadID+"/subscription", nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("get missing subscription: %d", w.Code)
	}

	body, _ := json.Marshal(map[string]any{"subscribed": true, "ignored": false})
	w = do("PUT", "/api/v3/notifications/threads/"+threadID+"/subscription", body)
	if w.Code != http.StatusOK {
		t.Fatalf("set subscription: %d body=%s", w.Code, w.Body.String())
	}
	var sub map[string]any
	json.Unmarshal(w.Body.Bytes(), &sub)
	if sub["subscribed"] != true {
		t.Errorf("subscribed = %v", sub["subscribed"])
	}

	w = do("GET", "/api/v3/notifications/threads/"+threadID+"/subscription", nil)
	if w.Code != http.StatusOK {
		t.Errorf("get subscription: %d", w.Code)
	}

	w = do("DELETE", "/api/v3/notifications/threads/"+threadID+"/subscription", nil)
	if w.Code != http.StatusNoContent {
		t.Errorf("delete subscription: %d", w.Code)
	}
}

func TestNotifications_SinceAndBefore(t *testing.T) {
	s := newTestServer()
	s.registerGHNotificationsRoutes()

	admin := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(admin, "since-repo", "", false)
	oldIssue := s.store.CreateIssue(repo.ID, admin.ID, "old", "body", nil, nil, 0)
	oldIssue.UpdatedAt = time.Now().UTC().Add(-48 * time.Hour)
	newIssue := s.store.CreateIssue(repo.ID, admin.ID, "new", "body", nil, nil, 0)
	newIssue.UpdatedAt = time.Now().UTC().Add(-1 * time.Hour)

	do := func(method, path string, body []byte) *httptest.ResponseRecorder {
		var req *http.Request
		if body != nil {
			req = httptest.NewRequest(method, path, bytes.NewReader(body))
		} else {
			req = httptest.NewRequest(method, path, nil)
		}
		req.Header.Set("Authorization", "Bearer bleephub-admin-token-00000000000000000000")
		w := httptest.NewRecorder()
		s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
		return w
	}

	since := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)
	w := do("GET", "/api/v3/notifications?since="+since, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list since: %d body=%s", w.Code, w.Body.String())
	}
	var threads []map[string]any
	json.Unmarshal(w.Body.Bytes(), &threads)
	if len(threads) != 1 || threads[0]["subject"].(map[string]any)["title"] != "new" {
		t.Errorf("expected only 'new' thread, got %+v", threads)
	}
}
