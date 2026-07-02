package bleephub

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
)

// Issue + PR moderation: comment edit/delete, issue/PR locking.
// Real GitHub keeps these on the /issues path (PRs are issues internally).

// validLockReasons matches real GitHub's accepted lock reasons (lowercased
// kebab-case in REST; the GraphQL enum is uppercase elsewhere).
var validLockReasons = map[string]bool{
	"off-topic":  true,
	"too heated": true,
	"resolved":   true,
	"spam":       true,
}

// --- Comment edit / delete (Issue + PR conversation comments) ---

func (s *Server) handleUpdateIssueComment(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	owner := r.PathValue("owner")
	repoName := r.PathValue("repo")
	idStr := r.PathValue("comment_id")
	repo := s.store.GetRepo(owner, repoName)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	id, err := strconv.Atoi(idStr)
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
	if req.Body == "" {
		writeGHError(w, http.StatusUnprocessableEntity, "Validation Failed")
		return
	}

	updated := s.store.UpdateCommentBody(id, user.ID, req.Body)
	if updated == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	// Resolve the parent number for the URL portion of the JSON.
	parentNumber := commentParentNumber(s.store, updated)
	writeJSON(w, http.StatusOK, commentToJSON(updated, s.store, s.baseURL(r), repo.FullName, parentNumber))
}

func (s *Server) handleDeleteIssueComment(w http.ResponseWriter, r *http.Request, idStr string) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if !s.store.DeleteComment(id) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleIssuesDeleteDispatch resolves `DELETE /repos/{}/issues/{p1}/{p2}`
// to either the by-id comment delete (`/issues/comments/{id}`) or the
// unlock endpoint (`/issues/{n}/lock`).
func (s *Server) handleIssuesDeleteDispatch(w http.ResponseWriter, r *http.Request) {
	p1 := r.PathValue("p1")
	p2 := r.PathValue("p2")
	if p1 == "comments" {
		s.handleDeleteIssueComment(w, r, p2)
		return
	}
	if p2 == "lock" {
		s.handleUnlockIssue(w, r, p1)
		return
	}
	if p2 == "sub_issues" || p2 == "sub_issue" {
		r.SetPathValue("number", p1)
		s.handleRemoveSubIssue(w, r)
		return
	}
	writeGHError(w, http.StatusNotFound, "Not Found")
}

// commentParentNumber returns the issue or PR number that owns the comment,
// or 0 when neither parent can be found (caller renders an URL without the
// number segment).
func commentParentNumber(st *Store, c *Comment) int {
	st.mu.RLock()
	defer st.mu.RUnlock()
	switch c.ParentType {
	case "issue":
		if i, ok := st.Issues[c.IssueID]; ok {
			return i.Number
		}
	case "pull_request":
		if pr, ok := st.PullRequests[c.IssueID]; ok {
			return pr.Number
		}
	}
	return 0
}

// --- Issue / PR lock + unlock ---

func (s *Server) handleLockIssue(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	repo, numStr := r.PathValue("owner"), r.PathValue("number")
	repoObj := s.store.GetRepo(repo, r.PathValue("repo"))
	if repoObj == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	num, err := strconv.Atoi(numStr)
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	var req struct {
		LockReason string `json:"lock_reason"`
	}
	// Body is optional (empty = lock with no reason); a present-but-malformed
	// body must still be rejected so silent decode errors don't drop a real
	// lock_reason on the floor.
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeGHError(w, http.StatusBadRequest, "Problems parsing JSON")
		return
	}
	if req.LockReason != "" && !validLockReasons[req.LockReason] {
		writeGHError(w, http.StatusUnprocessableEntity, "Invalid lock reason")
		return
	}

	if !s.store.SetIssueOrPRLock(repoObj.ID, num, true, req.LockReason) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	// PRs are issues internally; resolve whichever one was locked so the
	// timeline event is recorded against the correct parent ID.
	parentID := s.store.GetIssueOrPullRequestIDByNumber(repoObj.ID, num)
	if parentID == 0 {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	// Record a discrete lock event with the actor so the timeline reflects
	// who performed the lock.
	s.store.RecordIssueEvent(repoObj.ID, parentID, user.ID, "locked", map[string]interface{}{"lock_reason": req.LockReason})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleUnlockIssue(w http.ResponseWriter, r *http.Request, numberStr string) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	repoObj := s.store.GetRepo(r.PathValue("owner"), r.PathValue("repo"))
	if repoObj == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	num, err := strconv.Atoi(numberStr)
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if !s.store.SetIssueOrPRLock(repoObj.ID, num, false, "") {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	parentID := s.store.GetIssueOrPullRequestIDByNumber(repoObj.ID, num)
	if parentID == 0 {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.store.RecordIssueEvent(repoObj.ID, parentID, user.ID, "unlocked", map[string]interface{}{})
	w.WriteHeader(http.StatusNoContent)
}
