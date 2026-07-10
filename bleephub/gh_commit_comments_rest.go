package bleephub

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Commit Comments API.
// Real GH endpoints:
//   GET    /repos/{o}/{r}/comments                          list repo comments
//   GET    /repos/{o}/{r}/commits/{sha}/comments            list comments for commit
//   POST   /repos/{o}/{r}/commits/{sha}/comments            create
//   GET    /repos/{o}/{r}/comments/{id}                     get
//   PATCH  /repos/{o}/{r}/comments/{id}                     update
//   DELETE /repos/{o}/{r}/comments/{id}                     delete

// CommitComment is a comment on a commit.
type CommitComment struct {
	ID        int       `json:"id"`
	NodeID    string    `json:"node_id"`
	RepoID    int       `json:"repo_id"`
	CommitID  string    `json:"commit_id"`
	AuthorID  int       `json:"author_id"`
	Body      string    `json:"body"`
	Path      string    `json:"path,omitempty"`
	Position  *int      `json:"position,omitempty"`
	Line      *int      `json:"line,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CommitCommentStore holds commit comments keyed by id, repo, and commit.
type CommitCommentStore struct {
	mu       sync.RWMutex
	byID     map[int]*CommitComment
	byRepo   map[int][]*CommitComment
	byCommit map[string][]*CommitComment
	nextID   int
	persist  *Persistence
}

func newCommitCommentStore(p *Persistence) *CommitCommentStore {
	return &CommitCommentStore{
		byID:     map[int]*CommitComment{},
		byRepo:   map[int][]*CommitComment{},
		byCommit: map[string][]*CommitComment{},
		nextID:   1,
		persist:  p,
	}
}

func commitKey(repoID int, commitID string) string {
	return strconv.Itoa(repoID) + ":" + commitID
}

// Create adds a new commit comment.
func (s *CommitCommentStore) Create(repoID int, commitID string, authorID int, body, path string, position, line *int) *CommitComment {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.nextID
	s.nextID++
	now := time.Now().UTC()
	c := &CommitComment{
		ID:        id,
		NodeID:    fmt.Sprintf("CC_kgDO%08d", id),
		RepoID:    repoID,
		CommitID:  commitID,
		AuthorID:  authorID,
		Body:      body,
		Path:      path,
		Position:  position,
		Line:      line,
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.byID[id] = c
	s.byRepo[repoID] = append(s.byRepo[repoID], c)
	ck := commitKey(repoID, commitID)
	s.byCommit[ck] = append(s.byCommit[ck], c)
	s.persistComment(c)
	return c
}

// Get returns a commit comment by id.
func (s *CommitCommentStore) Get(id int) *CommitComment {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.byID[id]
}

// ListForRepo returns all commit comments for a repo sorted newest-first.
func (s *CommitCommentStore) ListForRepo(repoID int) []*CommitComment {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*CommitComment, len(s.byRepo[repoID]))
	copy(out, s.byRepo[repoID])
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out
}

// ListForCommit returns commit comments for a specific commit sorted newest-first.
func (s *CommitCommentStore) ListForCommit(repoID int, commitID string) []*CommitComment {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ck := commitKey(repoID, commitID)
	out := make([]*CommitComment, len(s.byCommit[ck]))
	copy(out, s.byCommit[ck])
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out
}

// Update modifies the body of a commit comment.
func (s *CommitCommentStore) Update(id int, body string) bool {
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

// Delete removes a commit comment.
func (s *CommitCommentStore) Delete(id int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.byID[id]
	if c == nil {
		return false
	}
	delete(s.byID, id)
	repoList := s.byRepo[c.RepoID]
	for i, x := range repoList {
		if x.ID == id {
			s.byRepo[c.RepoID] = append(repoList[:i], repoList[i+1:]...)
			break
		}
	}
	ck := commitKey(c.RepoID, c.CommitID)
	commitList := s.byCommit[ck]
	for i, x := range commitList {
		if x.ID == id {
			s.byCommit[ck] = append(commitList[:i], commitList[i+1:]...)
			break
		}
	}
	if s.persist != nil {
		s.persist.MustDelete("commit_comments", strconv.Itoa(id))
	}
	return true
}

func (s *CommitCommentStore) IDsForRepo(repoID int) map[int]bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make(map[int]bool, len(s.byRepo[repoID]))
	for _, c := range s.byRepo[repoID] {
		ids[c.ID] = true
	}
	return ids
}

func (s *CommitCommentStore) persistComment(c *CommitComment) {
	if s.persist == nil {
		return
	}
	s.persist.MustPut("commit_comments", strconv.Itoa(c.ID), c)
}

func (s *CommitCommentStore) deleteRepo(repoID int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, c := range s.byID {
		if c.RepoID != repoID {
			continue
		}
		delete(s.byID, id)
		if s.persist != nil {
			s.persist.MustDelete("commit_comments", strconv.Itoa(id))
		}
	}
	delete(s.byRepo, repoID)
	prefix := strconv.Itoa(repoID) + ":"
	for key := range s.byCommit {
		if strings.HasPrefix(key, prefix) {
			delete(s.byCommit, key)
		}
	}
}

func (s *Server) registerGHCommitCommentsRoutes() {
	s.route("GET /api/v3/repos/{owner}/{repo}/comments", s.handleListRepoCommitComments)
	s.route("GET /api/v3/repos/{owner}/{repo}/commits/{commit_sha}/comments", s.handleListCommitComments)
	s.route("POST /api/v3/repos/{owner}/{repo}/commits/{commit_sha}/comments",
		s.requirePerm(scopeContents, permWrite, s.handleCreateCommitComment))
	s.route("GET /api/v3/repos/{owner}/{repo}/comments/{comment_id}", s.handleGetCommitComment)
	s.route("PATCH /api/v3/repos/{owner}/{repo}/comments/{comment_id}", s.handleUpdateCommitComment)
	s.route("DELETE /api/v3/repos/{owner}/{repo}/comments/{comment_id}", s.handleDeleteCommitComment)
}

func (s *Server) handleListRepoCommitComments(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return
	}
	comments := s.store.CommitComments.ListForRepo(repo.ID)
	page := paginateAndLink(w, r, comments)
	out := make([]map[string]interface{}, 0, len(page))
	for _, c := range page {
		out = append(out, commitCommentToJSON(c, s.store, s.baseURL(r), repo))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleListCommitComments(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return
	}
	sha := r.PathValue("commit_sha")
	comments := s.store.CommitComments.ListForCommit(repo.ID, sha)
	page := paginateAndLink(w, r, comments)
	out := make([]map[string]interface{}, 0, len(page))
	for _, c := range page {
		out = append(out, commitCommentToJSON(c, s.store, s.baseURL(r), repo))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCreateCommitComment(w http.ResponseWriter, r *http.Request) {
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
	var req struct {
		Body     string `json:"body"`
		Path     string `json:"path"`
		Position *int   `json:"position"`
		Line     *int   `json:"line"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Body == "" {
		writeGHValidationError(w, "CommitComment", "body", "missing_field")
		return
	}
	sha := r.PathValue("commit_sha")
	c := s.store.CommitComments.Create(repo.ID, sha, user.ID, req.Body, req.Path, req.Position, req.Line)
	writeJSON(w, http.StatusCreated, commitCommentToJSON(c, s.store, s.baseURL(r), repo))
}

func (s *Server) handleGetCommitComment(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return
	}
	id, err := strconv.Atoi(r.PathValue("comment_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	c := s.store.CommitComments.Get(id)
	if c == nil || c.RepoID != repo.ID {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, commitCommentToJSON(c, s.store, s.baseURL(r), repo))
}

func (s *Server) handleUpdateCommitComment(w http.ResponseWriter, r *http.Request) {
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
	c := s.store.CommitComments.Get(id)
	if c == nil || c.RepoID != repo.ID {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if c.AuthorID != user.ID && !canAdminRepo(s.store, user, repo) {
		writeGHError(w, http.StatusForbidden, "Must have push access")
		return
	}
	if !s.store.CommitComments.Update(id, req.Body) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	c = s.store.CommitComments.Get(id)
	writeJSON(w, http.StatusOK, commitCommentToJSON(c, s.store, s.baseURL(r), repo))
}

func (s *Server) handleDeleteCommitComment(w http.ResponseWriter, r *http.Request) {
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
	id, err := strconv.Atoi(r.PathValue("comment_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	c := s.store.CommitComments.Get(id)
	if c == nil || c.RepoID != repo.ID {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if c.AuthorID != user.ID && !canAdminRepo(s.store, user, repo) {
		writeGHError(w, http.StatusForbidden, "Must have push access")
		return
	}
	if !s.store.CommitComments.Delete(id) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.store.Reactions.DeleteParent("commit_comment", id)
	w.WriteHeader(http.StatusNoContent)
}

func commitCommentToJSON(c *CommitComment, st *Store, baseURL string, repo *Repo) map[string]interface{} {
	if c == nil {
		return nil
	}
	var author map[string]interface{}
	st.mu.RLock()
	if u := st.Users[c.AuthorID]; u != nil {
		author = userToJSON(u)
	}
	st.mu.RUnlock()
	out := map[string]interface{}{
		"id":                 c.ID,
		"node_id":            c.NodeID,
		"body":               c.Body,
		"commit_id":          c.CommitID,
		"user":               author,
		"created_at":         c.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":         c.UpdatedAt.UTC().Format(time.RFC3339),
		"url":                fmt.Sprintf("%s/api/v3/repos/%s/comments/%d", baseURL, repo.FullName, c.ID),
		"html_url":           fmt.Sprintf("%s/%s/commit/%s#commitcomment-%d", baseURL, repo.FullName, c.CommitID, c.ID),
		"author_association": "OWNER",
	}
	if c.Path != "" {
		out["path"] = c.Path
	}
	if c.Position != nil {
		out["position"] = *c.Position
	}
	if c.Line != nil {
		out["line"] = *c.Line
	}
	return out
}
