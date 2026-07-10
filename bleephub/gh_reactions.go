package bleephub

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Reactions API.
// Real GitHub exposes reactions on issues, issue comments, PR review comments,
// commits/commit comments, releases, and discussions. Eight content values:
//   +1, -1, laugh, confused, heart, hooray, rocket, eyes.
//
// Endpoints (all live under /repos/{owner}/{repo}/...):
//   - issues/{number}/reactions               GET, POST, DELETE /{id}
//   - issues/comments/{comment_id}/reactions  GET, POST, DELETE /{id}
//   - pulls/comments/{comment_id}/reactions   GET, POST, DELETE /{id}
//   - comments/{comment_id}/reactions         GET, POST, DELETE /{id}  (commit comments)
//   - releases/{release_id}/reactions         GET, POST, DELETE /{id}
//
// Plus the user-level: DELETE /users/{username}/reactions/{id} (rarely used; skip).

// Reaction represents a single user reaction on some parent entity.
//
// ParentType/ParentID/UserID carry real json names so persistence
// round-trips the linkage (the reload path re-indexes byParent from them).
// Client responses never marshal this struct — reactionToJSON emits an
// explicit map.
type Reaction struct {
	ID         int       `json:"id"`
	ParentType string    `json:"parent_type"`
	ParentID   int       `json:"parent_id"`
	Content    string    `json:"content"`
	UserID     int       `json:"user_id"`
	CreatedAt  time.Time `json:"created_at"`
}

// ReactionStore holds reactions keyed by (parentType, parentID).
type ReactionStore struct {
	mu       sync.RWMutex
	byParent map[string][]*Reaction
	byID     map[int]*Reaction
	nextID   int
	persist  *Persistence
}

func newReactionStore(p *Persistence) *ReactionStore {
	return &ReactionStore{
		byParent: make(map[string][]*Reaction),
		byID:     make(map[int]*Reaction),
		nextID:   1,
		persist:  p,
	}
}

// validReactionContent is the canonical set real GitHub accepts.
var validReactionContent = map[string]bool{
	"+1":       true,
	"-1":       true,
	"laugh":    true,
	"confused": true,
	"heart":    true,
	"hooray":   true,
	"rocket":   true,
	"eyes":     true,
}

func reactionParentKey(parentType string, parentID int) string {
	return fmt.Sprintf("%s:%d", parentType, parentID)
}

// AddReaction creates or returns the existing (userID, content) reaction.
// Real GitHub returns the same id on repeat POST (idempotent).
func (rs *ReactionStore) AddReaction(parentType string, parentID int, userID int, content string) (*Reaction, bool, error) {
	if !validReactionContent[content] {
		return nil, false, fmt.Errorf("invalid reaction content: %s", content)
	}
	rs.mu.Lock()
	defer rs.mu.Unlock()
	key := reactionParentKey(parentType, parentID)
	for _, r := range rs.byParent[key] {
		if r.UserID == userID && r.Content == content {
			return r, true, nil // already exists
		}
	}
	r := &Reaction{
		ID:         rs.nextID,
		ParentType: parentType,
		ParentID:   parentID,
		Content:    content,
		UserID:     userID,
		CreatedAt:  time.Now().UTC(),
	}
	rs.nextID++
	rs.byParent[key] = append(rs.byParent[key], r)
	rs.byID[r.ID] = r
	if rs.persist != nil {
		rs.persist.MustPut("reactions", reactionParentKey(parentType, parentID), rs.byParent[key])
	}
	return r, false, nil
}

// ListReactions returns reactions on a parent, optionally filtered by content.
func (rs *ReactionStore) ListReactions(parentType string, parentID int, contentFilter string) []*Reaction {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	src := rs.byParent[reactionParentKey(parentType, parentID)]
	if contentFilter == "" {
		out := make([]*Reaction, len(src))
		copy(out, src)
		return out
	}
	out := []*Reaction{}
	for _, r := range src {
		if r.Content == contentFilter {
			out = append(out, r)
		}
	}
	return out
}

// DeleteReaction removes the reaction with the given id from its parent.
// Returns true if removed.
func (rs *ReactionStore) DeleteReaction(parentType string, parentID, reactionID int) bool {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	r := rs.byID[reactionID]
	if r == nil || r.ParentType != parentType || r.ParentID != parentID {
		return false
	}
	key := reactionParentKey(parentType, parentID)
	src := rs.byParent[key]
	for i, x := range src {
		if x.ID == reactionID {
			rs.byParent[key] = append(src[:i], src[i+1:]...)
			break
		}
	}
	delete(rs.byID, reactionID)
	if rs.persist != nil {
		if len(rs.byParent[key]) > 0 {
			rs.persist.MustPut("reactions", key, rs.byParent[key])
		} else {
			rs.persist.MustDelete("reactions", key)
		}
	}
	return true
}

// DeleteParent removes every reaction attached to one parent entity.
func (rs *ReactionStore) DeleteParent(parentType string, parentID int) {
	rs.DeleteParents(parentType, map[int]bool{parentID: true})
}

// DeleteParents removes every reaction attached to the given parent entities.
func (rs *ReactionStore) DeleteParents(parentType string, parentIDs map[int]bool) {
	if len(parentIDs) == 0 {
		return
	}
	rs.mu.Lock()
	defer rs.mu.Unlock()
	for parentID := range parentIDs {
		key := reactionParentKey(parentType, parentID)
		for _, r := range rs.byParent[key] {
			delete(rs.byID, r.ID)
		}
		delete(rs.byParent, key)
		if rs.persist != nil {
			rs.persist.MustDelete("reactions", key)
		}
	}
}

// SummarizeReactions computes the per-content counts + total used by
// real GitHub's reactions{url, total_count, +1, ...} block embedded in
// issue / comment / release JSON.
func (rs *ReactionStore) SummarizeReactions(parentType string, parentID int) map[string]interface{} {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	counts := map[string]int{
		"+1": 0, "-1": 0, "laugh": 0, "confused": 0,
		"heart": 0, "hooray": 0, "rocket": 0, "eyes": 0,
	}
	total := 0
	for _, r := range rs.byParent[reactionParentKey(parentType, parentID)] {
		counts[r.Content]++
		total++
	}
	return map[string]interface{}{
		"url":         "", // caller fills in the absolute URL
		"total_count": total,
		"+1":          counts["+1"],
		"-1":          counts["-1"],
		"laugh":       counts["laugh"],
		"confused":    counts["confused"],
		"heart":       counts["heart"],
		"hooray":      counts["hooray"],
		"rocket":      counts["rocket"],
		"eyes":        counts["eyes"],
	}
}

// --- HTTP surface ---

func (s *Server) registerGHReactionsRoutes() {
	// Issue reactions. GET /issues/{number}/reactions,
	// GET /issues/comments/{comment_id}/reactions, and the corresponding
	// DELETE /.../reactions/{reaction_id} paths are dispatched from
	// registerGHIssueRoutes because Go's mux cannot disambiguate literal
	// segments (comments) from wildcard segments (number) at the same depth.
	s.route("POST /api/v3/repos/{owner}/{repo}/issues/{number}/reactions",
		s.requirePerm(scopeIssues, permWrite, s.handleCreateReaction("issue", "number")))

	// Issue comment reactions. The DELETE path has four segments after
	// /issues, so it does not collide with the issue three-segment dispatch.
	s.route("POST /api/v3/repos/{owner}/{repo}/issues/comments/{comment_id}/reactions",
		s.requirePerm(scopeIssues, permWrite, s.handleCreateReaction("issue_comment", "comment_id")))
	s.route("DELETE /api/v3/repos/{owner}/{repo}/issues/comments/{comment_id}/reactions/{reaction_id}",
		s.requirePerm(scopeIssues, permWrite, s.handleDeleteReaction("issue_comment", "comment_id")))

	// PR review-comment reaction deletion. Four segments after /pulls, so it
	// does not collide with the pulls three-segment dispatch below.
	s.route("DELETE /api/v3/repos/{owner}/{repo}/pulls/comments/{comment_id}/reactions/{reaction_id}",
		s.requirePerm(scopePullRequests, permWrite, s.handleDeleteReaction("pull_request_review_comment", "comment_id")))

	// PR review-comment reactions. The 3-segment GET/POST routes
	// (/pulls/comments/{comment_id}/reactions) conflict with the PR review
	// routes (/pulls/{number}/reviews/{review_id}) under Go 1.22's mux, so
	// they are dispatched via handlePullsThreeSegDispatch.
	s.route("GET /api/v3/repos/{owner}/{repo}/pulls/{p1}/{p2}/{p3}", s.handlePullsThreeSegDispatch("GET"))
	s.route("POST /api/v3/repos/{owner}/{repo}/pulls/{p1}/{p2}/{p3}", s.requirePerm(scopePullRequests, permWrite, s.handlePullsThreeSegDispatch("POST")))
	s.route("PUT /api/v3/repos/{owner}/{repo}/pulls/{p1}/{p2}/{p3}", s.requirePerm(scopePullRequests, permWrite, s.handlePullsThreeSegDispatch("PUT")))
	s.route("DELETE /api/v3/repos/{owner}/{repo}/pulls/{p1}/{p2}/{p3}", s.requirePerm(scopePullRequests, permWrite, s.handlePullsThreeSegDispatch("DELETE")))

	// Commit comment reactions
	s.route("POST /api/v3/repos/{owner}/{repo}/comments/{comment_id}/reactions",
		s.requirePerm(scopeContents, permWrite, s.handleCreateReaction("commit_comment", "comment_id")))
	s.route("GET /api/v3/repos/{owner}/{repo}/comments/{comment_id}/reactions",
		s.handleListReactions("commit_comment", "comment_id"))
	s.route("DELETE /api/v3/repos/{owner}/{repo}/comments/{comment_id}/reactions/{reaction_id}",
		s.requirePerm(scopeContents, permWrite, s.handleDeleteReaction("commit_comment", "comment_id")))

	// Release reactions — register via the disambiguation dispatcher in
	// gh_releases.go because `/releases/tags/{tag}` and
	// `/releases/{release_id}/reactions` are ambiguous to Go 1.22's mux.
	// The dispatcher routes by segment-2 ("tags" vs numeric release_id).
}

// resolveReactionParent converts the reaction path parameter into the
// store-level (parentType, parentID) pair. Issue reactions arrive keyed by
// issue *number*, which is only unique within one repository — they resolve
// through the repository to the issue's global ID so reactions never leak
// between repositories that happen to share issue numbers. Pull requests
// share the issue number space and are reactable on real GitHub via the same
// /issues/{number}/reactions surface, so a number that resolves to a PR is
// keyed under the "pull_request" parent type (issue and PR IDs come from
// independent counters and would otherwise collide). Writes the error
// response and returns false when the repository or parent does not exist.
func (s *Server) resolveReactionParent(w http.ResponseWriter, r *http.Request, parentType, pathParam string) (string, int, bool) {
	parentID, err := strconv.Atoi(r.PathValue(pathParam))
	if err != nil {
		writeGHError(w, http.StatusBadRequest, fmt.Sprintf("invalid %s", pathParam))
		return "", 0, false
	}
	if parentType != "issue" {
		return parentType, parentID, true
	}
	repo := s.store.GetRepo(r.PathValue("owner"), r.PathValue("repo"))
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return "", 0, false
	}
	if issue := s.store.GetIssueByNumber(repo.ID, parentID); issue != nil {
		return "issue", issue.ID, true
	}
	if pr := s.store.GetPullRequestByNumber(repo.ID, parentID); pr != nil {
		return "pull_request", pr.ID, true
	}
	writeGHError(w, http.StatusNotFound, "Not Found")
	return "", 0, false
}

func (s *Server) handleCreateReaction(parentType, pathParam string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := ghUserFromContext(r.Context())
		if user == nil {
			writeGHError(w, http.StatusUnauthorized, "Bad credentials")
			return
		}
		effType, parentID, ok := s.resolveReactionParent(w, r, parentType, pathParam)
		if !ok {
			return
		}
		var body struct {
			Content string `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Content == "" {
			writeGHValidationError(w, "Reaction", "content", "missing_field")
			return
		}
		reaction, alreadyExisted, err := s.store.Reactions.AddReaction(effType, parentID, user.ID, body.Content)
		if err != nil {
			writeGHValidationError(w, "Reaction", "content", "invalid")
			return
		}
		status := http.StatusCreated
		if alreadyExisted {
			status = http.StatusOK
		}
		writeJSON(w, status, reactionToJSON(reaction, user))
	}
}

func (s *Server) handleListReactions(parentType, pathParam string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		effType, parentID, ok := s.resolveReactionParent(w, r, parentType, pathParam)
		if !ok {
			return
		}
		contentFilter := r.URL.Query().Get("content")
		reactions := s.store.Reactions.ListReactions(effType, parentID, contentFilter)
		page := paginateAndLink(w, r, reactions)
		out := make([]map[string]interface{}, 0, len(page))
		for _, rx := range page {
			user := s.store.GetUserByID(rx.UserID)
			out = append(out, reactionToJSON(rx, user))
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func (s *Server) handleDeleteReaction(parentType, pathParam string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := ghUserFromContext(r.Context())
		if user == nil {
			writeGHError(w, http.StatusUnauthorized, "Bad credentials")
			return
		}
		effType, parentID, ok := s.resolveReactionParent(w, r, parentType, pathParam)
		if !ok {
			return
		}
		reactionID, err := strconv.Atoi(r.PathValue("reaction_id"))
		if err != nil {
			writeGHError(w, http.StatusBadRequest, "invalid reaction id")
			return
		}
		if !s.store.Reactions.DeleteReaction(effType, parentID, reactionID) {
			writeGHError(w, http.StatusNotFound, "Not Found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func reactionToJSON(r *Reaction, user *User) map[string]interface{} {
	var userJSON map[string]interface{}
	if user != nil {
		userJSON = userToJSON(user)
	}
	return map[string]interface{}{
		"id":         r.ID,
		"node_id":    fmt.Sprintf("RE_kgDO%08d", r.ID),
		"content":    r.Content,
		"created_at": r.CreatedAt.UTC().Format(time.RFC3339),
		"user":       userJSON,
	}
}
