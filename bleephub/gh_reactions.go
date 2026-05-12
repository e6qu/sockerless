package bleephub

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Phase 154 (P154.1) — Reactions API.
//
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
type Reaction struct {
	ID         int       `json:"id"`
	ParentType string    `json:"-"`
	ParentID   int       `json:"-"`
	Content    string    `json:"content"`
	UserID     int       `json:"-"`
	CreatedAt  time.Time `json:"created_at"`
}

// ReactionStore holds reactions keyed by (parentType, parentID).
type ReactionStore struct {
	mu       sync.RWMutex
	byParent map[string][]*Reaction
	byID     map[int]*Reaction
	nextID   int
}

func newReactionStore() *ReactionStore {
	return &ReactionStore{
		byParent: make(map[string][]*Reaction),
		byID:     make(map[int]*Reaction),
		nextID:   1,
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
		CreatedAt:  time.Now(),
	}
	rs.nextID++
	rs.byParent[key] = append(rs.byParent[key], r)
	rs.byID[r.ID] = r
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
	return true
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
	// Issue reactions
	s.mux.HandleFunc("POST /api/v3/repos/{owner}/{repo}/issues/{number}/reactions",
		s.requirePerm("issues", permWrite, s.handleCreateReaction("issue", "number")))
	s.mux.HandleFunc("GET /api/v3/repos/{owner}/{repo}/issues/{number}/reactions",
		s.handleListReactions("issue", "number"))
	s.mux.HandleFunc("DELETE /api/v3/repos/{owner}/{repo}/issues/{number}/reactions/{reaction_id}",
		s.requirePerm("issues", permWrite, s.handleDeleteReaction("issue", "number")))

	// Issue comment reactions
	s.mux.HandleFunc("POST /api/v3/repos/{owner}/{repo}/issues/comments/{comment_id}/reactions",
		s.requirePerm("issues", permWrite, s.handleCreateReaction("issue_comment", "comment_id")))
	s.mux.HandleFunc("GET /api/v3/repos/{owner}/{repo}/issues/comments/{comment_id}/reactions",
		s.handleListReactions("issue_comment", "comment_id"))
	s.mux.HandleFunc("DELETE /api/v3/repos/{owner}/{repo}/issues/comments/{comment_id}/reactions/{reaction_id}",
		s.requirePerm("issues", permWrite, s.handleDeleteReaction("issue_comment", "comment_id")))

	// PR review-comment reactions
	s.mux.HandleFunc("POST /api/v3/repos/{owner}/{repo}/pulls/comments/{comment_id}/reactions",
		s.requirePerm("pull_requests", permWrite, s.handleCreateReaction("pull_request_review_comment", "comment_id")))
	s.mux.HandleFunc("GET /api/v3/repos/{owner}/{repo}/pulls/comments/{comment_id}/reactions",
		s.handleListReactions("pull_request_review_comment", "comment_id"))
	s.mux.HandleFunc("DELETE /api/v3/repos/{owner}/{repo}/pulls/comments/{comment_id}/reactions/{reaction_id}",
		s.requirePerm("pull_requests", permWrite, s.handleDeleteReaction("pull_request_review_comment", "comment_id")))

	// Commit comment reactions
	s.mux.HandleFunc("POST /api/v3/repos/{owner}/{repo}/comments/{comment_id}/reactions",
		s.requirePerm("contents", permWrite, s.handleCreateReaction("commit_comment", "comment_id")))
	s.mux.HandleFunc("GET /api/v3/repos/{owner}/{repo}/comments/{comment_id}/reactions",
		s.handleListReactions("commit_comment", "comment_id"))
	s.mux.HandleFunc("DELETE /api/v3/repos/{owner}/{repo}/comments/{comment_id}/reactions/{reaction_id}",
		s.requirePerm("contents", permWrite, s.handleDeleteReaction("commit_comment", "comment_id")))

	// Release reactions — register via the disambiguation dispatcher in
	// gh_releases.go because `/releases/tags/{tag}` and
	// `/releases/{release_id}/reactions` are ambiguous to Go 1.22's mux.
	// The dispatcher routes by segment-2 ("tags" vs numeric release_id).
}

func (s *Server) handleCreateReaction(parentType, pathParam string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := ghUserFromContext(r.Context())
		if user == nil {
			writeGHError(w, http.StatusUnauthorized, "Bad credentials")
			return
		}
		parentID, err := strconv.Atoi(r.PathValue(pathParam))
		if err != nil {
			writeGHError(w, http.StatusBadRequest, fmt.Sprintf("invalid %s", pathParam))
			return
		}
		var body struct {
			Content string `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Content == "" {
			writeGHValidationError(w, "Reaction", "content", "missing_field")
			return
		}
		reaction, alreadyExisted, err := s.store.Reactions.AddReaction(parentType, parentID, user.ID, body.Content)
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
		parentID, err := strconv.Atoi(r.PathValue(pathParam))
		if err != nil {
			writeGHError(w, http.StatusBadRequest, fmt.Sprintf("invalid %s", pathParam))
			return
		}
		contentFilter := r.URL.Query().Get("content")
		reactions := s.store.Reactions.ListReactions(parentType, parentID, contentFilter)
		page := paginateAndLink(w, r, reactions)
		out := make([]map[string]interface{}, 0, len(page))
		for _, rx := range page {
			user := s.store.Users[rx.UserID]
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
		parentID, err := strconv.Atoi(r.PathValue(pathParam))
		if err != nil {
			writeGHError(w, http.StatusBadRequest, fmt.Sprintf("invalid %s", pathParam))
			return
		}
		reactionID, err := strconv.Atoi(r.PathValue("reaction_id"))
		if err != nil {
			writeGHError(w, http.StatusBadRequest, "invalid reaction id")
			return
		}
		if !s.store.Reactions.DeleteReaction(parentType, parentID, reactionID) {
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
