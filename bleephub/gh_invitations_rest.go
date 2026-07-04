package bleephub

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

func (s *Server) registerGHRepoInvitationRoutes() {
	s.route("GET /api/v3/repos/{owner}/{repo}/invitations", s.requirePerm(scopeAdministration, permWrite, s.handleListRepoInvitations))
	s.route("PATCH /api/v3/repos/{owner}/{repo}/invitations/{invitation_id}", s.requirePerm(scopeAdministration, permWrite, s.handleUpdateRepoInvitation))
	s.route("DELETE /api/v3/repos/{owner}/{repo}/invitations/{invitation_id}", s.requirePerm(scopeAdministration, permWrite, s.handleCancelRepoInvitation))
	s.route("GET /api/v3/user/repository_invitations", s.handleListUserRepoInvitations)
	s.route("PATCH /api/v3/user/repository_invitations/{invitation_id}", s.handleAcceptRepoInvitation)
	s.route("DELETE /api/v3/user/repository_invitations/{invitation_id}", s.handleDeclineRepoInvitation)
}

func (s *Server) handleListRepoInvitations(w http.ResponseWriter, r *http.Request) {
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

	invitations := s.store.ListPendingRepoInvitations(repo.FullName)
	out := make([]map[string]interface{}, 0, len(invitations))
	base := s.baseURL(r)
	for _, inv := range invitations {
		out = append(out, invitationJSON(inv, repo, s.store, base))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleUpdateRepoInvitation(w http.ResponseWriter, r *http.Request) {
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

	id, err := strconv.Atoi(r.PathValue("invitation_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	var req struct {
		Permissions string `json:"permissions"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Permissions == "" {
		writeGHValidationError(w, "RepositoryInvitation", "permissions", "missing_field")
		return
	}

	updated := s.store.UpdateRepoInvitation(repo.FullName, id, req.Permissions)
	if updated == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, invitationJSON(updated, repo, s.store, s.baseURL(r)))
}

func (s *Server) handleCancelRepoInvitation(w http.ResponseWriter, r *http.Request) {
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

	id, err := strconv.Atoi(r.PathValue("invitation_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if !s.store.DeleteRepoInvitation(repo.FullName, id) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListUserRepoInvitations(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}

	invitations := s.store.ListUserRepoInvitations(user)
	out := make([]map[string]interface{}, 0, len(invitations))
	base := s.baseURL(r)
	for _, inv := range invitations {
		if repo := s.store.GetRepoByFullName(inv.RepoKey); repo != nil {
			out = append(out, invitationJSON(inv, repo, s.store, base))
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleAcceptRepoInvitation(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}

	id, err := strconv.Atoi(r.PathValue("invitation_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if !s.store.AcceptRepoInvitation(id, user) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeclineRepoInvitation(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}

	id, err := strconv.Atoi(r.PathValue("invitation_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if !s.store.DeclineRepoInvitation(id, user) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func invitationJSON(inv *RepoInvitation, repo *Repo, st *Store, baseURL string) map[string]interface{} {
	invitee := map[string]interface{}(nil)
	if inv.InviteeLogin != "" {
		if u := st.LookupUserByLogin(inv.InviteeLogin); u != nil {
			invitee = userToJSON(u)
		}
	}
	inviter := map[string]interface{}(nil)
	if u := st.GetUserByID(inv.InviterID); u != nil {
		inviter = userToJSON(u)
	}
	perm := inv.Permissions
	if perm == "" {
		perm = "pull"
	}
	roleName := githubRoleName(perm)

	return map[string]interface{}{
		"id":          inv.ID,
		"node_id":     inv.NodeID,
		"repository":  repoToJSON(repo, st, baseURL),
		"invitee":     invitee,
		"inviter":     inviter,
		"permissions": roleName,
		"created_at":  inv.CreatedAt.Format(time.RFC3339),
		"url":         baseURL + "/user/repository-invitations/" + strconv.Itoa(inv.ID),
		"html_url":    baseURL + "/" + inv.RepoKey + "/invitations",
		"expired":     false,
	}
}

// CreateRepoInvitation creates a pending invitation for a user to collaborate
// on the repository. Used by tests and by future admin-facing create endpoints.
func (st *Store) CreateRepoInvitation(repoKey, inviteeLogin, inviteeEmail string, inviterID int, permission string) *RepoInvitation {
	st.mu.Lock()
	defer st.mu.Unlock()

	perm := normalizeRepoPermission(permission)
	inv := &RepoInvitation{
		ID:           st.NextInvitationID,
		NodeID:       fmt.Sprintf("RI_kgDO%08d", st.NextInvitationID),
		RepoKey:      repoKey,
		InviteeLogin: inviteeLogin,
		InviteeEmail: inviteeEmail,
		InviterID:    inviterID,
		Permissions:  perm,
		CreatedAt:    time.Now().UTC(),
		Status:       "pending",
	}
	st.NextInvitationID++
	if st.RepoInvitations[repoKey] == nil {
		st.RepoInvitations[repoKey] = map[int]*RepoInvitation{}
	}
	st.RepoInvitations[repoKey][inv.ID] = inv
	if st.persist != nil {
		st.persist.MustPut("repo_invitations", repoKey, st.RepoInvitations[repoKey])
	}
	return inv
}

// ListPendingRepoInvitations returns pending invitations for a repository, sorted by ID.
func (st *Store) ListPendingRepoInvitations(repoKey string) []*RepoInvitation {
	st.mu.RLock()
	defer st.mu.RUnlock()

	m := st.RepoInvitations[repoKey]
	out := make([]*RepoInvitation, 0, len(m))
	for _, inv := range m {
		if inv.Status == "pending" {
			out = append(out, inv)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// GetRepoInvitation returns an invitation by repository key and ID, or nil.
func (st *Store) GetRepoInvitation(repoKey string, id int) *RepoInvitation {
	st.mu.RLock()
	defer st.mu.RUnlock()
	if st.RepoInvitations[repoKey] == nil {
		return nil
	}
	return st.RepoInvitations[repoKey][id]
}

// UpdateRepoInvitation updates the permission on a pending invitation.
// Returns the updated invitation, or nil if not found.
func (st *Store) UpdateRepoInvitation(repoKey string, id int, permission string) *RepoInvitation {
	st.mu.Lock()
	defer st.mu.Unlock()

	if st.RepoInvitations[repoKey] == nil {
		return nil
	}
	inv, ok := st.RepoInvitations[repoKey][id]
	if !ok || inv.Status != "pending" {
		return nil
	}
	inv.Permissions = normalizeRepoPermission(permission)
	if st.persist != nil {
		st.persist.MustPut("repo_invitations", repoKey, st.RepoInvitations[repoKey])
	}
	return inv
}

// DeleteRepoInvitation removes an invitation from a repository. Returns true if it existed.
func (st *Store) DeleteRepoInvitation(repoKey string, id int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	if st.RepoInvitations[repoKey] == nil {
		return false
	}
	if _, ok := st.RepoInvitations[repoKey][id]; !ok {
		return false
	}
	delete(st.RepoInvitations[repoKey], id)
	if st.persist != nil {
		st.persist.MustPut("repo_invitations", repoKey, st.RepoInvitations[repoKey])
	}
	return true
}

// ListUserRepoInvitations returns pending invitations addressed to the user.
func (st *Store) ListUserRepoInvitations(user *User) []*RepoInvitation {
	st.mu.RLock()
	defer st.mu.RUnlock()

	out := []*RepoInvitation{}
	for _, m := range st.RepoInvitations {
		for _, inv := range m {
			if inv.Status != "pending" {
				continue
			}
			if strings.EqualFold(inv.InviteeLogin, user.Login) || (inv.InviteeEmail != "" && strings.EqualFold(inv.InviteeEmail, user.Email)) {
				out = append(out, inv)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// AcceptRepoInvitation accepts an invitation for the user, adding them as a
// collaborator with the invitation's permission. Returns true on success.
func (st *Store) AcceptRepoInvitation(id int, user *User) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	var target *RepoInvitation
	for _, m := range st.RepoInvitations {
		if inv, ok := m[id]; ok {
			target = inv
			break
		}
	}
	if target == nil || target.Status != "pending" {
		return false
	}
	if !invitationMatchesUser(target, user) {
		return false
	}
	parts := strings.SplitN(target.RepoKey, "/", 2)
	if len(parts) != 2 {
		return false
	}
	repo, ok := st.ReposByName[target.RepoKey]
	if !ok {
		return false
	}
	if st.RepoCollaborators[target.RepoKey] == nil {
		st.RepoCollaborators[target.RepoKey] = map[string]string{}
	}
	st.RepoCollaborators[target.RepoKey][user.Login] = target.Permissions
	repo.UpdatedAt = time.Now().UTC()
	delete(st.RepoInvitations[target.RepoKey], id)
	if st.persist != nil {
		st.persist.MustPut("repo_invitations", target.RepoKey, st.RepoInvitations[target.RepoKey])
		st.persist.MustPut("repo_collaborators", target.RepoKey, st.RepoCollaborators[target.RepoKey])
		st.persist.MustPut("repos", strconv.Itoa(repo.ID), repo)
	}
	return true
}

// DeclineRepoInvitation removes an invitation addressed to the user.
// Returns true on success.
func (st *Store) DeclineRepoInvitation(id int, user *User) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	var target *RepoInvitation
	for _, m := range st.RepoInvitations {
		if inv, ok := m[id]; ok {
			target = inv
			break
		}
	}
	if target == nil || target.Status != "pending" {
		return false
	}
	if !invitationMatchesUser(target, user) {
		return false
	}
	delete(st.RepoInvitations[target.RepoKey], id)
	if st.persist != nil {
		st.persist.MustPut("repo_invitations", target.RepoKey, st.RepoInvitations[target.RepoKey])
	}
	return true
}

func invitationMatchesUser(inv *RepoInvitation, user *User) bool {
	if strings.EqualFold(inv.InviteeLogin, user.Login) {
		return true
	}
	if inv.InviteeEmail != "" && strings.EqualFold(inv.InviteeEmail, user.Email) {
		return true
	}
	return false
}

// GetRepoByFullName returns a repo by its "owner/name" key, or nil.
func (st *Store) GetRepoByFullName(fullName string) *Repo {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.ReposByName[fullName]
}
