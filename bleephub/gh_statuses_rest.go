package bleephub

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// Commit Statuses API.
// Real GH endpoints:
//   GET  /repos/{o}/{r}/commits/{ref}/status        combined status
//   GET  /repos/{o}/{r}/commits/{ref}/statuses      list statuses
//   POST /repos/{o}/{r}/statuses/{sha}              create status
//
// Statuses are repo+ref scoped; the combined status endpoint derives the
// worst state across the latest status per context.

// CommitStatus is a single commit status context.
type CommitStatus struct {
	ID          int       `json:"id"`
	NodeID      string    `json:"node_id"`
	State       string    `json:"state"`
	TargetURL   string    `json:"target_url"`
	Description string    `json:"description"`
	Context     string    `json:"context"`
	CreatorID   int       `json:"creator_id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// maxCommitStatusesPerRef bounds the number of statuses retained per repo+sha.
// Real GitHub rejects more than 1000 statuses on a single sha; matching that
// keeps a spammed ref from growing the store without limit.
const maxCommitStatusesPerRef = 1000

// CommitStatusStore holds commit statuses keyed by repo+ref.
type CommitStatusStore struct {
	mu      sync.RWMutex
	byKey   map[string][]*CommitStatus
	nextID  int
	persist *Persistence
}

func newCommitStatusStore(p *Persistence) *CommitStatusStore {
	return &CommitStatusStore{
		byKey:   map[string][]*CommitStatus{},
		persist: p,
	}
}

func statusKey(repoKey, ref string) string {
	return repoKey + ":" + ref
}

// Create appends a new status for the given repo+sha.
func (s *CommitStatusStore) Create(repoKey, sha string, creatorID int, state, targetURL, description, context string) *CommitStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := statusKey(repoKey, sha)
	if s.nextID < 1 {
		s.nextID = 1
	}
	id := s.nextID
	s.nextID++
	now := time.Now().UTC()
	st := &CommitStatus{
		ID:          id,
		NodeID:      fmt.Sprintf("CS_kgDO%08d", id),
		State:       normalizeStatusState(state),
		TargetURL:   targetURL,
		Description: description,
		Context:     coalesceStr(context, "default"),
		CreatorID:   creatorID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	list := append(s.byKey[key], st)
	if len(list) > maxCommitStatusesPerRef {
		list = list[len(list)-maxCommitStatusesPerRef:]
	}
	s.byKey[key] = list
	s.persistStatuses(key)
	return st
}

func normalizeStatusState(state string) string {
	switch strings.ToLower(state) {
	case "success":
		return "success"
	case "failure":
		return "failure"
	case "pending":
		return "pending"
	case "error":
		return "error"
	default:
		return "pending"
	}
}

// List returns statuses for a repo+ref sorted newest-first.
func (s *CommitStatusStore) List(repoKey, ref string) []*CommitStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	key := statusKey(repoKey, ref)
	src := s.byKey[key]
	out := make([]*CommitStatus, len(src))
	copy(out, src)
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out
}

// Combined returns the combined status state plus the latest status per context.
func (s *CommitStatusStore) Combined(repoKey, ref string) (state string, total int, statuses []*CommitStatus) {
	all := s.List(repoKey, ref)
	// latest per context (all is already newest-first)
	latestByCtx := map[string]*CommitStatus{}
	for _, st := range all {
		if _, ok := latestByCtx[st.Context]; !ok {
			latestByCtx[st.Context] = st
		}
	}
	statuses = make([]*CommitStatus, 0, len(latestByCtx))
	for _, st := range latestByCtx {
		statuses = append(statuses, st)
	}
	sort.Slice(statuses, func(i, j int) bool {
		return statuses[i].CreatedAt.After(statuses[j].CreatedAt)
	})
	total = len(statuses)
	state = computeCombinedState(statuses)
	return
}

func computeCombinedState(statuses []*CommitStatus) string {
	for _, st := range statuses {
		if st.State == "error" {
			return "error"
		}
	}
	for _, st := range statuses {
		if st.State == "failure" {
			return "failure"
		}
	}
	for _, st := range statuses {
		if st.State == "pending" {
			return "pending"
		}
	}
	return "success"
}

func (s *CommitStatusStore) persistStatuses(key string) {
	if s.persist == nil {
		return
	}
	s.persist.MustPut("commit_statuses", key, s.byKey[key])
}

func (s *CommitStatusStore) moveRepoKey(oldFull, newFull string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	prefix := oldFull + ":"
	for key, statuses := range s.byKey {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		newKey := newFull + strings.TrimPrefix(key, oldFull)
		s.byKey[newKey] = statuses
		delete(s.byKey, key)
		if s.persist != nil {
			s.persist.MustPut("commit_statuses", newKey, statuses)
			s.persist.MustDelete("commit_statuses", key)
		}
	}
}

func (s *CommitStatusStore) deleteRepoKey(fullName string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	prefix := fullName + ":"
	for key := range s.byKey {
		if strings.HasPrefix(key, prefix) {
			delete(s.byKey, key)
			if s.persist != nil {
				s.persist.MustDelete("commit_statuses", key)
			}
		}
	}
}

func (s *Server) registerGHStatusesRoutes() {
	s.route("GET /api/v3/repos/{owner}/{repo}/commits/{ref}/status", s.handleGetCombinedStatus)
	s.route("GET /api/v3/repos/{owner}/{repo}/commits/{ref}/statuses", s.handleListCommitStatuses)
	s.route("POST /api/v3/repos/{owner}/{repo}/statuses/{sha}", s.requirePerm(scopeContents, permWrite, s.handleCreateCommitStatus))
}

func (s *Server) handleGetCombinedStatus(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return
	}
	ref := r.PathValue("ref")
	state, total, statuses := s.store.CommitStatuses.Combined(repo.FullName, ref)
	out := make([]map[string]interface{}, 0, len(statuses))
	for _, st := range statuses {
		out = append(out, commitStatusToJSON(st, s.store, s.baseURL(r), repo.FullName))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"state":       state,
		"sha":         ref,
		"total_count": total,
		"statuses":    out,
		"repository":  repoToJSON(repo, s.store, s.baseURL(r)),
	})
}

func (s *Server) handleListCommitStatuses(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return
	}
	ref := r.PathValue("ref")
	statuses := s.store.CommitStatuses.List(repo.FullName, ref)
	page := paginateAndLink(w, r, statuses)
	out := make([]map[string]interface{}, 0, len(page))
	for _, st := range page {
		out = append(out, commitStatusToJSON(st, s.store, s.baseURL(r), repo.FullName))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCreateCommitStatus(w http.ResponseWriter, r *http.Request) {
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
		State       string `json:"state"`
		TargetURL   string `json:"target_url"`
		Description string `json:"description"`
		Context     string `json:"context"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.State == "" {
		writeGHValidationError(w, "Status", "state", "missing_field")
		return
	}
	sha := r.PathValue("sha")
	st := s.store.CommitStatuses.Create(repo.FullName, sha, user.ID, req.State, req.TargetURL, req.Description, req.Context)
	writeJSON(w, http.StatusCreated, commitStatusToJSON(st, s.store, s.baseURL(r), repo.FullName))
}

func commitStatusToJSON(st *CommitStatus, stStore *Store, baseURL, repoKey string) map[string]interface{} {
	if st == nil {
		return nil
	}
	var creator map[string]interface{}
	stStore.mu.RLock()
	if u := stStore.Users[st.CreatorID]; u != nil {
		creator = userToJSON(u)
	}
	stStore.mu.RUnlock()
	return map[string]interface{}{
		"id":          st.ID,
		"node_id":     st.NodeID,
		"state":       st.State,
		"description": st.Description,
		"target_url":  st.TargetURL,
		"context":     st.Context,
		"avatar_url":  "",
		"creator":     creator,
		"created_at":  st.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":  st.UpdatedAt.UTC().Format(time.RFC3339),
		"url":         fmt.Sprintf("%s/api/v3/repos/%s/statuses/%d", baseURL, repoKey, st.ID),
	}
}
