package bleephub

import (
	"errors"
	"net/http"
	"strconv"
)

// Sub-issues and issue dependencies.
// Endpoints:
//
//	GET    /repos/{o}/{r}/issues/{n}/sub_issues            (two-seg GET dispatch)
//	POST   /repos/{o}/{r}/issues/{n}/sub_issues
//	PATCH  /repos/{o}/{r}/issues/{n}/sub_issues/priority
//	DELETE /repos/{o}/{r}/issues/{n}/sub_issue             (two-seg DELETE dispatch)
//	GET    /repos/{o}/{r}/issues/{n}/dependencies/blocked_by (three-seg GET dispatch)
//	POST   /repos/{o}/{r}/issues/{n}/dependencies/blocked_by
//	DELETE /repos/{o}/{r}/issues/{n}/dependencies/blocked_by/{issue_id}
//
// Both features are real bidirectional links in the issues store: a
// sub-issue knows its parent, and an issue blocked by another shows up as
// blocking on the other side.

var (
	errSubIssueSelf      = errors.New("an issue may not be its own sub-issue")
	errSubIssueHasParent = errors.New("the issue is already a sub-issue of another issue")
	errSubIssueCycle     = errors.New("the sub-issue relationship would create a cycle")
	errSubIssueDuplicate = errors.New("the issue is already a sub-issue of this issue")
	errSubIssueNotLinked = errors.New("the issue is not a sub-issue of this issue")
)

// --- Store: sub-issue links ---

// AddSubIssue links child under parent. replaceParent detaches the child
// from a previous parent first.
func (st *Store) AddSubIssue(parentID, childID int, replaceParent bool) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	if parentID == childID {
		return errSubIssueSelf
	}
	if cur, ok := st.SubIssueParent[childID]; ok {
		if cur == parentID {
			return errSubIssueDuplicate
		}
		if !replaceParent {
			return errSubIssueHasParent
		}
	}
	// Reject cycles: the parent (or any ancestor) may not be a descendant
	// of the child.
	for ancestor := parentID; ; {
		next, ok := st.SubIssueParent[ancestor]
		if !ok {
			break
		}
		if next == childID {
			return errSubIssueCycle
		}
		ancestor = next
	}
	if cur, ok := st.SubIssueParent[childID]; ok {
		st.removeSubIssueLocked(cur, childID)
		st.persistSubIssuesLocked(cur)
	}
	st.SubIssueLists[parentID] = append(st.SubIssueLists[parentID], childID)
	st.SubIssueParent[childID] = parentID
	st.persistSubIssuesLocked(parentID)
	return nil
}

// RemoveSubIssue unlinks child from parent.
func (st *Store) RemoveSubIssue(parentID, childID int) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.SubIssueParent[childID] != parentID {
		return errSubIssueNotLinked
	}
	st.removeSubIssueLocked(parentID, childID)
	st.persistSubIssuesLocked(parentID)
	return nil
}

func (st *Store) removeSubIssueLocked(parentID, childID int) {
	children := st.SubIssueLists[parentID]
	for i, id := range children {
		if id == childID {
			st.SubIssueLists[parentID] = append(children[:i], children[i+1:]...)
			break
		}
	}
	if len(st.SubIssueLists[parentID]) == 0 {
		delete(st.SubIssueLists, parentID)
	}
	delete(st.SubIssueParent, childID)
}

// ReprioritizeSubIssue moves child within parent's ordered sub-issue list,
// placing it immediately after afterID or immediately before beforeID.
func (st *Store) ReprioritizeSubIssue(parentID, childID int, afterID, beforeID *int) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.SubIssueParent[childID] != parentID {
		return errSubIssueNotLinked
	}
	children := st.SubIssueLists[parentID]
	without := make([]int, 0, len(children))
	for _, id := range children {
		if id != childID {
			without = append(without, id)
		}
	}
	pos := len(without)
	switch {
	case afterID != nil:
		found := false
		for i, id := range without {
			if id == *afterID {
				pos = i + 1
				found = true
				break
			}
		}
		if !found {
			return errSubIssueNotLinked
		}
	case beforeID != nil:
		found := false
		for i, id := range without {
			if id == *beforeID {
				pos = i
				found = true
				break
			}
		}
		if !found {
			return errSubIssueNotLinked
		}
	}
	reordered := make([]int, 0, len(children))
	reordered = append(reordered, without[:pos]...)
	reordered = append(reordered, childID)
	reordered = append(reordered, without[pos:]...)
	st.SubIssueLists[parentID] = reordered
	st.persistSubIssuesLocked(parentID)
	return nil
}

// ListSubIssues returns parent's sub-issue IDs in priority order.
func (st *Store) ListSubIssues(parentID int) []int {
	st.mu.RLock()
	defer st.mu.RUnlock()
	out := make([]int, len(st.SubIssueLists[parentID]))
	copy(out, st.SubIssueLists[parentID])
	return out
}

func (st *Store) persistSubIssuesLocked(parentID int) {
	if st.persist == nil {
		return
	}
	if children, ok := st.SubIssueLists[parentID]; ok {
		st.persist.MustPut("sub_issues", strconv.Itoa(parentID), children)
	} else {
		st.persist.MustDelete("sub_issues", strconv.Itoa(parentID))
	}
}

// --- Store: issue dependencies ---

// AddIssueBlockedBy records that issue is blocked by blocker. Returns false
// when the link already exists.
func (st *Store) AddIssueBlockedBy(issueID, blockerID int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	for _, id := range st.IssueBlockedBy[issueID] {
		if id == blockerID {
			return false
		}
	}
	st.IssueBlockedBy[issueID] = append(st.IssueBlockedBy[issueID], blockerID)
	st.persistBlockedByLocked(issueID)
	return true
}

// RemoveIssueBlockedBy removes a blocked-by link. Returns false when the
// link does not exist.
func (st *Store) RemoveIssueBlockedBy(issueID, blockerID int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	blockers := st.IssueBlockedBy[issueID]
	for i, id := range blockers {
		if id == blockerID {
			st.IssueBlockedBy[issueID] = append(blockers[:i], blockers[i+1:]...)
			if len(st.IssueBlockedBy[issueID]) == 0 {
				delete(st.IssueBlockedBy, issueID)
			}
			st.persistBlockedByLocked(issueID)
			return true
		}
	}
	return false
}

// ListIssueBlockedBy returns the IDs of the issues blocking issueID.
func (st *Store) ListIssueBlockedBy(issueID int) []int {
	st.mu.RLock()
	defer st.mu.RUnlock()
	out := make([]int, len(st.IssueBlockedBy[issueID]))
	copy(out, st.IssueBlockedBy[issueID])
	return out
}

// ListIssueBlocking returns the IDs of the issues that issueID blocks —
// the reverse view of the blocked-by links.
func (st *Store) ListIssueBlocking(issueID int) []int {
	st.mu.RLock()
	defer st.mu.RUnlock()
	var out []int
	for blocked, blockers := range st.IssueBlockedBy {
		for _, id := range blockers {
			if id == issueID {
				out = append(out, blocked)
				break
			}
		}
	}
	return out
}

func (st *Store) persistBlockedByLocked(issueID int) {
	if st.persist == nil {
		return
	}
	if blockers, ok := st.IssueBlockedBy[issueID]; ok {
		st.persist.MustPut("issue_blocked_by", strconv.Itoa(issueID), blockers)
	} else {
		st.persist.MustDelete("issue_blocked_by", strconv.Itoa(issueID))
	}
}

// --- Handlers ---

// issueFromNumberPath resolves {owner}/{repo} + the "number" path value to
// the repo and issue, writing a 404 when either is missing.
func (s *Server) issueFromNumberPath(w http.ResponseWriter, r *http.Request) (*Repo, *Issue) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, nil
	}
	number, err := strconv.Atoi(r.PathValue("number"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, nil
	}
	issue := s.store.GetIssueByNumber(repo.ID, number)
	if issue == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, nil
	}
	return repo, issue
}

func (s *Server) handleListSubIssues(w http.ResponseWriter, r *http.Request) {
	repo, issue := s.issueFromNumberPath(w, r)
	if issue == nil {
		return
	}
	ids := s.store.ListSubIssues(issue.ID)
	base := s.baseURL(r)
	out := make([]map[string]interface{}, 0, len(ids))
	for _, id := range ids {
		if child := s.store.GetIssue(id); child != nil {
			out = append(out, issueToJSON(child, s.store, base, repo.FullName))
		}
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, out))
}

func (s *Server) handleCreateSubIssue(w http.ResponseWriter, r *http.Request) {
	repo, issue := s.issueFromNumberPath(w, r)
	if issue == nil {
		return
	}
	var req struct {
		SubIssueID    *int     `json:"sub_issue_id"`
		ReplaceParent flexBool `json:"replace_parent"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.SubIssueID == nil {
		writeGHValidationError(w, "SubIssue", "sub_issue_id", "missing_field")
		return
	}
	child := s.store.GetIssue(*req.SubIssueID)
	if child == nil || child.RepoID != repo.ID {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if err := s.store.AddSubIssue(issue.ID, child.ID, bool(req.ReplaceParent)); err != nil {
		writeGHError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, issueToJSON(s.store.GetIssue(issue.ID), s.store, s.baseURL(r), repo.FullName))
}

func (s *Server) handleRemoveSubIssue(w http.ResponseWriter, r *http.Request) {
	repo, issue := s.issueFromNumberPath(w, r)
	if issue == nil {
		return
	}
	var req struct {
		SubIssueID *int `json:"sub_issue_id"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.SubIssueID == nil {
		writeGHValidationError(w, "SubIssue", "sub_issue_id", "missing_field")
		return
	}
	child := s.store.GetIssue(*req.SubIssueID)
	if child == nil || child.RepoID != repo.ID {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if err := s.store.RemoveSubIssue(issue.ID, child.ID); err != nil {
		writeGHError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, issueToJSON(child, s.store, s.baseURL(r), repo.FullName))
}

func (s *Server) handleReprioritizeSubIssue(w http.ResponseWriter, r *http.Request) {
	repo, issue := s.issueFromNumberPath(w, r)
	if issue == nil {
		return
	}
	var req struct {
		SubIssueID *int `json:"sub_issue_id"`
		AfterID    *int `json:"after_id"`
		BeforeID   *int `json:"before_id"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.SubIssueID == nil {
		writeGHValidationError(w, "SubIssue", "sub_issue_id", "missing_field")
		return
	}
	if req.AfterID != nil && req.BeforeID != nil {
		writeGHValidationError(w, "SubIssue", "after_id", "invalid")
		return
	}
	if err := s.store.ReprioritizeSubIssue(issue.ID, *req.SubIssueID, req.AfterID, req.BeforeID); err != nil {
		writeGHError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, issueToJSON(s.store.GetIssue(issue.ID), s.store, s.baseURL(r), repo.FullName))
}

func (s *Server) handleListIssueDependenciesBlockedBy(w http.ResponseWriter, r *http.Request) {
	repo, issue := s.issueFromNumberPath(w, r)
	if issue == nil {
		return
	}
	ids := s.store.ListIssueBlockedBy(issue.ID)
	base := s.baseURL(r)
	out := make([]map[string]interface{}, 0, len(ids))
	for _, id := range ids {
		if blocker := s.store.GetIssue(id); blocker != nil {
			out = append(out, issueToJSON(blocker, s.store, base, repo.FullName))
		}
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, out))
}

func (s *Server) handleAddIssueDependencyBlockedBy(w http.ResponseWriter, r *http.Request) {
	repo, issue := s.issueFromNumberPath(w, r)
	if issue == nil {
		return
	}
	var req struct {
		IssueID *int `json:"issue_id"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.IssueID == nil {
		writeGHValidationError(w, "IssueDependency", "issue_id", "missing_field")
		return
	}
	blocker := s.store.GetIssue(*req.IssueID)
	if blocker == nil || blocker.RepoID != repo.ID {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if blocker.ID == issue.ID {
		writeGHError(w, http.StatusUnprocessableEntity, "An issue may not block itself")
		return
	}
	if !s.store.AddIssueBlockedBy(issue.ID, blocker.ID) {
		writeGHError(w, http.StatusUnprocessableEntity, "The issue dependency already exists")
		return
	}
	writeJSON(w, http.StatusCreated, issueToJSON(blocker, s.store, s.baseURL(r), repo.FullName))
}

func (s *Server) handleRemoveIssueDependencyBlockedBy(w http.ResponseWriter, r *http.Request) {
	repo, issue := s.issueFromNumberPath(w, r)
	if issue == nil {
		return
	}
	blockerID, err := strconv.Atoi(r.PathValue("issue_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	blocker := s.store.GetIssue(blockerID)
	if blocker == nil || blocker.RepoID != repo.ID {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if !s.store.RemoveIssueBlockedBy(issue.ID, blocker.ID) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, issueToJSON(blocker, s.store, s.baseURL(r), repo.FullName))
}
