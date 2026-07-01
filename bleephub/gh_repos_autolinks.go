package bleephub

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"time"
)

func (s *Server) registerGHRepoAutolinkRoutes() {
	s.route("GET /api/v3/repos/{owner}/{repo}/autolinks", s.handleListAutolinks)
	s.route("POST /api/v3/repos/{owner}/{repo}/autolinks", s.requirePerm(scopeAdministration, permWrite, s.handleCreateAutolink))
	s.route("GET /api/v3/repos/{owner}/{repo}/autolinks/{autolink_id}", s.handleGetAutolink)
	s.route("DELETE /api/v3/repos/{owner}/{repo}/autolinks/{autolink_id}", s.requirePerm(scopeAdministration, permWrite, s.handleDeleteAutolink))
}

func (s *Server) handleListAutolinks(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	name := r.PathValue("repo")
	repo := s.store.GetRepo(owner, name)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	user := ghUserFromContext(r.Context())
	if repo.Private && !canReadRepo(s.store, user, repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	autolinks := s.store.ListRepoAutolinks(repo.FullName)
	out := make([]map[string]interface{}, 0, len(autolinks))
	for _, a := range autolinks {
		out = append(out, autolinkJSON(a))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCreateAutolink(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	name := r.PathValue("repo")
	repo := s.store.GetRepo(owner, name)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	user := ghUserFromContext(r.Context())
	if !canAdminRepo(s.store, user, repo) {
		writeGHError(w, http.StatusForbidden, "Must have admin rights to Repository.")
		return
	}

	var req struct {
		KeyPrefix      string `json:"key_prefix"`
		URLTemplate    string `json:"url_template"`
		IsAlphanumeric *bool  `json:"is_alphanumeric"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.KeyPrefix == "" {
		writeGHValidationError(w, "Autolink", "key_prefix", "missing_field")
		return
	}
	if req.URLTemplate == "" {
		writeGHValidationError(w, "Autolink", "url_template", "missing_field")
		return
	}
	isAlpha := true
	if req.IsAlphanumeric != nil {
		isAlpha = *req.IsAlphanumeric
	}

	autolink := s.store.CreateRepoAutolink(repo.FullName, req.KeyPrefix, req.URLTemplate, isAlpha)
	writeJSON(w, http.StatusCreated, autolinkJSON(autolink))
}

func (s *Server) handleGetAutolink(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	name := r.PathValue("repo")
	repo := s.store.GetRepo(owner, name)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	user := ghUserFromContext(r.Context())
	if repo.Private && !canReadRepo(s.store, user, repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	id, err := strconv.Atoi(r.PathValue("autolink_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	autolink := s.store.GetRepoAutolink(repo.FullName, id)
	if autolink == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, autolinkJSON(autolink))
}

func (s *Server) handleDeleteAutolink(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	name := r.PathValue("repo")
	repo := s.store.GetRepo(owner, name)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	user := ghUserFromContext(r.Context())
	if !canAdminRepo(s.store, user, repo) {
		writeGHError(w, http.StatusForbidden, "Must have admin rights to Repository.")
		return
	}

	id, err := strconv.Atoi(r.PathValue("autolink_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if !s.store.DeleteRepoAutolink(repo.FullName, id) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func autolinkJSON(a *RepoAutolink) map[string]interface{} {
	return map[string]interface{}{
		"id":              a.ID,
		"node_id":         a.NodeID,
		"key_prefix":      a.KeyPrefix,
		"url_template":    a.URLTemplate,
		"is_alphanumeric": a.IsAlphanumeric,
		"created_at":      a.CreatedAt.Format(time.RFC3339),
	}
}

// CreateRepoAutolink creates a new autolink reference on the repository.
func (st *Store) CreateRepoAutolink(repoKey, keyPrefix, urlTemplate string, isAlphanumeric bool) *RepoAutolink {
	st.mu.Lock()
	defer st.mu.Unlock()

	autolink := &RepoAutolink{
		ID:             st.NextAutolinkID,
		NodeID:         fmt.Sprintf("AL_kgDO%08d", st.NextAutolinkID),
		RepoKey:        repoKey,
		KeyPrefix:      keyPrefix,
		URLTemplate:    urlTemplate,
		IsAlphanumeric: isAlphanumeric,
		CreatedAt:      time.Now().UTC(),
	}
	st.NextAutolinkID++
	if st.RepoAutolinks[repoKey] == nil {
		st.RepoAutolinks[repoKey] = map[int]*RepoAutolink{}
	}
	st.RepoAutolinks[repoKey][autolink.ID] = autolink
	if st.persist != nil {
		st.persist.MustPut("repo_autolinks", repoKey, st.RepoAutolinks[repoKey])
	}
	return autolink
}

// ListRepoAutolinks returns all autolinks for a repository, sorted by ID.
func (st *Store) ListRepoAutolinks(repoKey string) []*RepoAutolink {
	st.mu.RLock()
	defer st.mu.RUnlock()

	m := st.RepoAutolinks[repoKey]
	out := make([]*RepoAutolink, 0, len(m))
	for _, a := range m {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// GetRepoAutolink returns an autolink by ID, or nil.
func (st *Store) GetRepoAutolink(repoKey string, id int) *RepoAutolink {
	st.mu.RLock()
	defer st.mu.RUnlock()
	if st.RepoAutolinks[repoKey] == nil {
		return nil
	}
	return st.RepoAutolinks[repoKey][id]
}

// DeleteRepoAutolink removes an autolink by ID. Returns true if it existed.
func (st *Store) DeleteRepoAutolink(repoKey string, id int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	if st.RepoAutolinks[repoKey] == nil {
		return false
	}
	if _, ok := st.RepoAutolinks[repoKey][id]; !ok {
		return false
	}
	delete(st.RepoAutolinks[repoKey], id)
	if st.persist != nil {
		st.persist.MustPut("repo_autolinks", repoKey, st.RepoAutolinks[repoKey])
	}
	return true
}
