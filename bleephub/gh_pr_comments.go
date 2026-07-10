package bleephub

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// PR review comments (inline / file-line / range).
// Endpoints:
//   POST   /repos/{o}/{r}/pulls/{number}/comments
//   GET    /repos/{o}/{r}/pulls/{number}/comments
//   GET    /repos/{o}/{r}/pulls/comments/{id}
//   PATCH  /repos/{o}/{r}/pulls/comments/{id}
//   DELETE /repos/{o}/{r}/pulls/comments/{id}
//   POST   /repos/{o}/{r}/pulls/{number}/comments/{id}/replies
//
// gh CLI's `gh pr review --thread` / `gh pr comment` uses GraphQL mutations
// (resolveReviewThread / unresolveReviewThread); the REST surface here is
// what octokit + probot use.
//
// PR review comments distinct from issue comments (`/issues/{n}/comments`):
// review comments attach to a specific file path + line + commit SHA and
// participate in review threads.

type PRReviewComment struct {
	ID                int       `json:"id"`
	NodeID            string    `json:"node_id"`
	PullRequestID     int       `json:"-"`
	ReviewID          int       `json:"pull_request_review_id"`
	InReplyToID       int       `json:"in_reply_to_id,omitempty"`
	DiffHunk          string    `json:"diff_hunk"`
	Path              string    `json:"path"`
	Position          *int      `json:"position"`
	OriginalPosition  *int      `json:"original_position"`
	Line              *int      `json:"line"`
	OriginalLine      *int      `json:"original_line"`
	StartLine         *int      `json:"start_line"`
	OriginalStartLine *int      `json:"original_start_line"`
	Side              string    `json:"side"` // LEFT | RIGHT
	StartSide         string    `json:"start_side,omitempty"`
	CommitID          string    `json:"commit_id"`
	OriginalCommitID  string    `json:"original_commit_id"`
	Body              string    `json:"body"`
	AuthorID          int       `json:"-"`
	ThreadID          int       `json:"-"` // shared id for the thread root + replies
	Resolved          bool      `json:"-"` // thread-level resolved flag stored on the root
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// prReviewCommentRecord is the persistence DTO for PRReviewComment. The
// struct's json:"-" linkage fields (PR, author, thread, resolved) must
// survive a reload while the public REST shape omits them, so the record
// carries them explicitly.
type prReviewCommentRecord struct {
	*PRReviewComment
	PullRequestID int  `json:"pull_request_id"`
	AuthorID      int  `json:"author_id"`
	ThreadID      int  `json:"thread_id"`
	Resolved      bool `json:"resolved"`
}

func newPRReviewCommentRecord(c *PRReviewComment) prReviewCommentRecord {
	return prReviewCommentRecord{
		PRReviewComment: c,
		PullRequestID:   c.PullRequestID,
		AuthorID:        c.AuthorID,
		ThreadID:        c.ThreadID,
		Resolved:        c.Resolved,
	}
}

// restore copies the record's explicit fields back onto the wrapped comment
// after unmarshal (the wrapped fields are json:"-" so decode skips them).
func (r *prReviewCommentRecord) restore() *PRReviewComment {
	c := r.PRReviewComment
	c.PullRequestID = r.PullRequestID
	c.AuthorID = r.AuthorID
	c.ThreadID = r.ThreadID
	c.Resolved = r.Resolved
	return c
}

// PRReviewCommentStore — concurrency-safe storage.
type PRReviewCommentStore struct {
	mu          sync.RWMutex
	byID        map[int]*PRReviewComment
	byPR        map[int][]*PRReviewComment
	threadRoots map[int]int
	nextID      int
	persist     *Persistence
}

// persistComment writes the comment through to disk as its storage record.
// Caller must hold s.mu.
func (s *PRReviewCommentStore) persistComment(c *PRReviewComment) {
	if s.persist == nil {
		return
	}
	s.persist.MustPut("pr_review_comments", strconv.Itoa(c.ID), newPRReviewCommentRecord(c))
}

func newPRReviewCommentStore(p *Persistence) *PRReviewCommentStore {
	return &PRReviewCommentStore{
		byID:        map[int]*PRReviewComment{},
		byPR:        map[int][]*PRReviewComment{},
		threadRoots: map[int]int{},
		nextID:      1,
		persist:     p,
	}
}

// CreateRootComment is the top-level review comment.
func (s *PRReviewCommentStore) CreateRootComment(prID, authorID int, path, body, commitID, side string, line, startLine int) *PRReviewComment {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.nextID
	s.nextID++
	now := time.Now().UTC()
	c := &PRReviewComment{
		ID:               id,
		NodeID:           fmt.Sprintf("PRRC_kgDO%08d", id),
		PullRequestID:    prID,
		Path:             path,
		Body:             body,
		CommitID:         commitID,
		OriginalCommitID: commitID,
		Side:             coalesceStr(side, "RIGHT"),
		AuthorID:         authorID,
		ThreadID:         id,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if line > 0 {
		c.Line = &line
		c.OriginalLine = &line
		pos := line
		c.Position = &pos
		c.OriginalPosition = &pos
	}
	if startLine > 0 {
		c.StartLine = &startLine
		c.OriginalStartLine = &startLine
	}
	s.byID[id] = c
	s.byPR[prID] = append(s.byPR[prID], c)
	s.threadRoots[id] = id
	s.persistComment(c)
	return c
}

// Reply appends a reply to a root comment. Real GH's POST /pulls/{n}/comments/{id}/replies.
func (s *PRReviewCommentStore) Reply(prID, rootID, authorID int, body string) *PRReviewComment {
	s.mu.Lock()
	defer s.mu.Unlock()
	root, ok := s.byID[rootID]
	if !ok || root.PullRequestID != prID {
		return nil
	}
	// Walk to true thread root (replies-to-replies share the same thread).
	threadRoot := rootID
	if tr, ok := s.threadRoots[rootID]; ok {
		threadRoot = tr
	}
	id := s.nextID
	s.nextID++
	now := time.Now().UTC()
	c := &PRReviewComment{
		ID:                id,
		NodeID:            fmt.Sprintf("PRRC_kgDO%08d", id),
		PullRequestID:     prID,
		InReplyToID:       rootID,
		Path:              root.Path,
		Body:              body,
		CommitID:          root.CommitID,
		OriginalCommitID:  root.OriginalCommitID,
		Side:              root.Side,
		AuthorID:          authorID,
		Line:              root.Line,
		OriginalLine:      root.OriginalLine,
		StartLine:         root.StartLine,
		OriginalStartLine: root.OriginalStartLine,
		Position:          root.Position,
		OriginalPosition:  root.OriginalPosition,
		ThreadID:          threadRoot,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	s.byID[id] = c
	s.byPR[prID] = append(s.byPR[prID], c)
	s.threadRoots[id] = threadRoot
	s.persistComment(c)
	return c
}

// AttachToReview links a review comment to the pull request review that
// created it (the create-review API's comments array).
func (s *PRReviewCommentStore) AttachToReview(commentID, reviewID int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.byID[commentID]
	if c == nil {
		return false
	}
	c.ReviewID = reviewID
	s.persistComment(c)
	return true
}

func (s *PRReviewCommentStore) Get(id int) *PRReviewComment {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.byID[id]
}

func (s *PRReviewCommentStore) ListForPR(prID int) []*PRReviewComment {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*PRReviewComment, len(s.byPR[prID]))
	copy(out, s.byPR[prID])
	return out
}

func (s *PRReviewCommentStore) Update(id int, body string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.byID[id]
	if c == nil {
		return false
	}
	c.Body = body
	c.UpdatedAt = time.Now().UTC()
	s.persistComment(c)
	return true
}

func (s *PRReviewCommentStore) Delete(id int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.byID[id]
	if c == nil {
		return false
	}
	delete(s.byID, id)
	src := s.byPR[c.PullRequestID]
	for i, x := range src {
		if x.ID == id {
			s.byPR[c.PullRequestID] = append(src[:i], src[i+1:]...)
			break
		}
	}
	delete(s.threadRoots, id)
	if s.persist != nil {
		s.persist.MustDelete("pr_review_comments", strconv.Itoa(id))
	}
	return true
}

func (s *PRReviewCommentStore) IDsForPR(prID int) map[int]bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make(map[int]bool, len(s.byPR[prID]))
	for _, c := range s.byPR[prID] {
		ids[c.ID] = true
	}
	return ids
}

func (s *PRReviewCommentStore) DeleteForPR(prID int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.byPR[prID] {
		delete(s.byID, c.ID)
		delete(s.threadRoots, c.ID)
		if s.persist != nil {
			s.persist.MustDelete("pr_review_comments", strconv.Itoa(c.ID))
		}
	}
	delete(s.byPR, prID)
}

// ResolveThread flips the thread root's Resolved flag.
func (s *PRReviewCommentStore) ResolveThread(threadID int, resolved bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	root := s.byID[threadID]
	if root == nil {
		return false
	}
	root.Resolved = resolved
	root.UpdatedAt = time.Now().UTC()
	s.persistComment(root)
	return true
}

func (s *PRReviewCommentStore) GetThread(threadID int) *ReviewThread {
	s.mu.RLock()
	defer s.mu.RUnlock()
	root := s.byID[threadID]
	if root == nil {
		return nil
	}
	thread := &ReviewThread{ID: threadID, IsResolved: root.Resolved}
	for _, c := range s.byPR[root.PullRequestID] {
		if c.ID == threadID || c.ThreadID == threadID {
			thread.Comments = append(thread.Comments, c)
		}
	}
	return thread
}

// ListThreads groups PR review comments by thread root.
type ReviewThread struct {
	ID         int                `json:"id"`
	IsResolved bool               `json:"isResolved"`
	Comments   []*PRReviewComment `json:"comments"`
}

func (s *PRReviewCommentStore) ListThreads(prID int) []*ReviewThread {
	s.mu.RLock()
	defer s.mu.RUnlock()
	threads := map[int]*ReviewThread{}
	for _, c := range s.byPR[prID] {
		threadID := c.ThreadID
		if threadID == 0 {
			threadID = c.ID
		}
		t, ok := threads[threadID]
		if !ok {
			t = &ReviewThread{ID: threadID}
			threads[threadID] = t
			// Pick up resolved flag from root.
			if root := s.byID[threadID]; root != nil {
				t.IsResolved = root.Resolved
			}
		}
		t.Comments = append(t.Comments, c)
	}
	out := make([]*ReviewThread, 0, len(threads))
	for _, t := range threads {
		out = append(out, t)
	}
	return out
}

func (s *Server) registerGHPRCommentsRoutes() {
	// `/pulls/{number}/comments` (3 segments, literal "comments" at pos 3)
	s.route("POST /api/v3/repos/{owner}/{repo}/pulls/{number}/comments",
		s.requirePerm(scopePullRequests, permWrite, s.handleCreatePRComment))
	s.route("GET /api/v3/repos/{owner}/{repo}/pulls/{number}/comments",
		s.handleListPRComments)
	s.route("POST /api/v3/repos/{owner}/{repo}/pulls/{number}/comments/{comment_id}/replies",
		s.requirePerm(scopePullRequests, permWrite, s.handleReplyPRComment))

	// `/pulls/{number}/reviews/{review_id}/comments` (4 segments; no clash
	// with the 3-segment dispatch in gh_reactions.go)
	s.route("GET /api/v3/repos/{owner}/{repo}/pulls/{number}/reviews/{review_id}/comments",
		s.handleListPRReviewCommentsForReview)

	// `/pulls/comments/{comment_id}` — single-review-comment surface.
	// Go 1.22's mux can't register both `/pulls/comments/{cid}` and
	// `/pulls/{number}/comments` because /pulls/comments/comments would match
	// both. Resolve via a 2-segment dispatcher (literal-anchored at p1=="comments"
	// when comment subpath is intended). The existing /pulls/{number}/<literal>
	// routes are strictly more specific (literal at pos 3) and continue to win
	// for their URLs.
	s.route("GET /api/v3/repos/{owner}/{repo}/pulls/{p1}/{p2}",
		s.handlePRCommentTwoSegDispatch("GET"))
	s.route("PATCH /api/v3/repos/{owner}/{repo}/pulls/{p1}/{p2}",
		s.handlePRCommentTwoSegDispatch("PATCH"))
	s.route("DELETE /api/v3/repos/{owner}/{repo}/pulls/{p1}/{p2}",
		s.handlePRCommentTwoSegDispatch("DELETE"))
}

// handlePRCommentTwoSegDispatch routes `/pulls/{p1}/{p2}` when p1=="comments"
// (single-review-comment surface) or p2=="files" (the changed-file diff list).
// For any other shape it 404s — the existing literal routes (e.g.
// /pulls/{number}/merge) win on their own paths.
func (s *Server) handlePRCommentTwoSegDispatch(method string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p1 := r.PathValue("p1")
		if method == "GET" && r.PathValue("p2") == "files" && p1 != "comments" {
			r.SetPathValue("number", p1)
			s.handleListPullRequestFiles(w, r)
			return
		}
		if p1 != "comments" {
			writeGHError(w, http.StatusNotFound, "Not Found")
			return
		}
		r.SetPathValue("comment_id", r.PathValue("p2"))
		switch method {
		case "GET":
			s.handleGetPRComment(w, r)
		case "PATCH":
			s.requirePerm(scopePullRequests, permWrite, s.handleUpdatePRComment)(w, r)
		case "DELETE":
			s.requirePerm(scopePullRequests, permWrite, s.handleDeletePRComment)(w, r)
		}
	}
}

func (s *Server) handleCreatePRComment(w http.ResponseWriter, r *http.Request) {
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
		Body      string  `json:"body"`
		CommitID  string  `json:"commit_id"`
		Path      string  `json:"path"`
		Line      flexInt `json:"line"`
		StartLine flexInt `json:"start_line"`
		Side      string  `json:"side"`
		InReplyTo flexInt `json:"in_reply_to"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Body == "" {
		writeGHValidationError(w, "PullRequestReviewComment", "body", "missing_field")
		return
	}
	var c *PRReviewComment
	if int(req.InReplyTo) > 0 {
		c = s.store.PRReviewComments.Reply(pr.ID, int(req.InReplyTo), user.ID, req.Body)
		if c == nil {
			writeGHError(w, http.StatusNotFound, "Reply target not found")
			return
		}
	} else {
		if req.Path == "" {
			writeGHValidationError(w, "PullRequestReviewComment", "path", "missing_field")
			return
		}
		c = s.store.PRReviewComments.CreateRootComment(pr.ID, user.ID, req.Path, req.Body, req.CommitID, req.Side, int(req.Line), int(req.StartLine))
	}
	s.emitWebhookEvent(repo.FullName, "pull_request_review_comment", "created",
		buildPRReviewCommentEventPayload(repo, pr, c, user, "created"))
	writeJSON(w, http.StatusCreated, prReviewCommentToJSON(c, s.store, s.baseURL(r), repo, pr))
}

func (s *Server) handleListPRComments(w http.ResponseWriter, r *http.Request) {
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
	comments := s.store.PRReviewComments.ListForPR(pr.ID)
	page := paginateAndLink(w, r, comments)
	out := make([]map[string]interface{}, 0, len(page))
	for _, c := range page {
		out = append(out, prReviewCommentToJSON(c, s.store, s.baseURL(r), repo, pr))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGetPRComment(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return
	}
	id, err := strconv.Atoi(r.PathValue("comment_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	c := s.store.PRReviewComments.Get(id)
	if c == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	pr := s.store.GetPullRequest(c.PullRequestID)
	writeJSON(w, http.StatusOK, prReviewCommentToJSON(c, s.store, s.baseURL(r), repo, pr))
}

func (s *Server) handleUpdatePRComment(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	id, err := strconv.Atoi(r.PathValue("comment_id"))
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
	if !s.store.PRReviewComments.Update(id, req.Body) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	c := s.store.PRReviewComments.Get(id)
	pr := s.store.GetPullRequest(c.PullRequestID)
	writeJSON(w, http.StatusOK, prReviewCommentToJSON(c, s.store, s.baseURL(r), repo, pr))
}

func (s *Server) handleDeletePRComment(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("comment_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if !s.store.PRReviewComments.Delete(id) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.store.Reactions.DeleteParent("pull_request_comment", id)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleReplyPRComment(w http.ResponseWriter, r *http.Request) {
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
	rootID, err := strconv.Atoi(r.PathValue("comment_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	var req struct {
		Body string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Body == "" {
		writeGHValidationError(w, "PullRequestReviewComment", "body", "missing_field")
		return
	}
	c := s.store.PRReviewComments.Reply(pr.ID, rootID, user.ID, req.Body)
	if c == nil {
		writeGHError(w, http.StatusNotFound, "Reply target not found")
		return
	}
	writeJSON(w, http.StatusCreated, prReviewCommentToJSON(c, s.store, s.baseURL(r), repo, pr))
}

// handleListPRReviewCommentsForReview serves
// GET /repos/{owner}/{repo}/pulls/{number}/reviews/{review_id}/comments —
// the review comments that belong to one submitted pull request review.
func (s *Server) handleListPRReviewCommentsForReview(w http.ResponseWriter, r *http.Request) {
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
	out := []map[string]interface{}{}
	for _, c := range s.store.PRReviewComments.ListForPR(pr.ID) {
		if c.ReviewID != reviewID {
			continue
		}
		out = append(out, prReviewCommentToJSON(c, s.store, s.baseURL(r), repo, pr))
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, out))
}

func prReviewCommentToJSON(c *PRReviewComment, st *Store, baseURL string, repo *Repo, pr *PullRequest) map[string]interface{} {
	if c == nil {
		return nil
	}
	var author map[string]interface{}
	st.mu.RLock()
	if u := st.Users[c.AuthorID]; u != nil {
		author = userToJSON(u)
	}
	st.mu.RUnlock()
	reactions := st.Reactions.SummarizeReactions("pull_request_review_comment", c.ID)
	reactions["url"] = fmt.Sprintf("%s/api/v3/repos/%s/pulls/comments/%d/reactions", baseURL, repo.FullName, c.ID)
	out := map[string]interface{}{
		"id":                     c.ID,
		"node_id":                c.NodeID,
		"url":                    fmt.Sprintf("%s/api/v3/repos/%s/pulls/comments/%d", baseURL, repo.FullName, c.ID),
		"pull_request_review_id": c.ReviewID,
		"diff_hunk":              c.DiffHunk,
		"path":                   c.Path,
		"position":               c.Position,
		"original_position":      c.OriginalPosition,
		"line":                   c.Line,
		"original_line":          c.OriginalLine,
		"start_line":             c.StartLine,
		"original_start_line":    c.OriginalStartLine,
		"side":                   c.Side,
		"start_side":             c.StartSide,
		"commit_id":              c.CommitID,
		"original_commit_id":     c.OriginalCommitID,
		"body":                   c.Body,
		"user":                   author,
		"created_at":             c.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":             c.UpdatedAt.UTC().Format(time.RFC3339),
		"reactions":              reactions,
		"author_association":     "OWNER",
	}
	if c.InReplyToID > 0 {
		out["in_reply_to_id"] = c.InReplyToID
	}
	if pr != nil {
		out["pull_request_url"] = fmt.Sprintf("%s/api/v3/repos/%s/pulls/%d", baseURL, repo.FullName, pr.Number)
		out["html_url"] = fmt.Sprintf("%s/%s/pull/%d#discussion_r%d", baseURL, repo.FullName, pr.Number, c.ID)
		out["_links"] = map[string]interface{}{
			"self":         map[string]interface{}{"href": out["url"]},
			"html":         map[string]interface{}{"href": out["html_url"]},
			"pull_request": map[string]interface{}{"href": out["pull_request_url"]},
		}
	}
	return out
}

func buildPRReviewCommentEventPayload(repo *Repo, pr *PullRequest, c *PRReviewComment, sender *User, action string) map[string]interface{} {
	return attachInstallationBlock(map[string]interface{}{
		"action":  action,
		"comment": map[string]interface{}{"id": c.ID, "body": c.Body, "path": c.Path},
		"pull_request": map[string]interface{}{
			"number": pr.Number,
			"title":  pr.Title,
			"state":  pr.State,
		},
		"repository": repoPayload(repo),
		"sender":     senderPayload(sender),
	}, nil)
}
