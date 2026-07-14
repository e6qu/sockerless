package bleephub

import (
	"crypto/rand"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

func (s *Server) personalAccessTokenWebUser(w http.ResponseWriter, r *http.Request) (*User, *http.Request) {
	ctx := s.authenticateRequest(r)
	user := ghUserFromContext(ctx)
	// The settings UI is a browser-authenticated surface. A signed-in browser
	// must be able to create its first personal access token; requiring a
	// pre-existing PAT here would make that flow circular.
	if user == nil || (ghPersonalAccessTokenFromContext(ctx) == nil && s.sessionFromRequest(r) == nil) {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return nil, r
	}
	if suspended, _ := ctx.Value(ctxSuspendedUser).(bool); suspended {
		writeGHError(w, http.StatusForbidden, "This account has been suspended")
		return nil, r
	}
	return user, r.WithContext(ctx)
}

func (s *Server) fineGrainedPATStatus(token *Token) string {
	if token.ExpiresAt != nil && !token.ExpiresAt.After(time.Now()) {
		return "expired"
	}
	if s.store.GetOrg(token.ResourceOwner) == nil {
		return "active"
	}
	s.store.mu.RLock()
	defer s.store.mu.RUnlock()
	for _, grant := range s.store.OrgPATGrants[token.ResourceOwner] {
		if grant.TokenID == token.FineGrainedID {
			return "active"
		}
	}
	for _, request := range s.store.OrgPATGrantRequests[token.ResourceOwner] {
		if request.TokenID == token.FineGrainedID {
			return "pending"
		}
	}
	return "revoked"
}

func personalAccessTokenWebJSON(token *Token, status string) map[string]interface{} {
	var expiry interface{}
	if token.ExpiresAt != nil {
		expiry = token.ExpiresAt.UTC().Format(time.RFC3339)
	}
	return map[string]interface{}{
		"id": token.FineGrainedID, "name": token.Name, "resource_owner": token.ResourceOwner,
		"repository_selection": token.RepositorySelection, "repository_ids": token.RepositoryIDs,
		"permissions": token.Permissions, "created_at": token.CreatedAt.UTC().Format(time.RFC3339),
		"expires_at": expiry, "status": status,
	}
}

func (s *Server) handleListPersonalAccessTokensWeb(w http.ResponseWriter, r *http.Request) {
	user, _ := s.personalAccessTokenWebUser(w, r)
	if user == nil {
		return
	}
	tokens := make([]*Token, 0)
	s.store.mu.RLock()
	for _, token := range s.store.Tokens {
		if token.UserID == user.ID && token.FineGrained {
			copy := *token
			copy.RepositoryIDs = append([]int(nil), token.RepositoryIDs...)
			tokens = append(tokens, &copy)
		}
	}
	s.store.mu.RUnlock()
	sort.Slice(tokens, func(i, j int) bool { return tokens[i].FineGrainedID < tokens[j].FineGrainedID })
	rows := make([]map[string]interface{}, 0, len(tokens))
	for _, token := range tokens {
		rows = append(rows, personalAccessTokenWebJSON(token, s.fineGrainedPATStatus(token)))
	}

	owners := []map[string]interface{}{{"login": user.Login, "type": "User"}}
	repositories := map[string][]map[string]interface{}{user.Login: {}}
	pending := []map[string]interface{}{}
	for _, org := range s.store.ListOrgsByUser(user.ID) {
		owners = append(owners, map[string]interface{}{"login": org.Login, "type": "Organization"})
		repositories[org.Login] = s.personalAccessTokenRepositories(org.Login)
		if canAdminOrg(s.store, user, org) {
			s.store.mu.RLock()
			requests := make([]*OrgPATGrantRequest, 0, len(s.store.OrgPATGrantRequests[org.Login]))
			for _, request := range s.store.OrgPATGrantRequests[org.Login] {
				requests = append(requests, request)
			}
			s.store.mu.RUnlock()
			sort.Slice(requests, func(i, j int) bool { return requests[i].ID < requests[j].ID })
			for _, request := range requests {
				row := s.patGrantRequestJSON(request, s.baseURL(r))
				row["organization"] = org.Login
				pending = append(pending, row)
			}
		}
	}
	repositories[user.Login] = s.personalAccessTokenRepositories(user.Login)
	writeJSON(w, http.StatusOK, map[string]interface{}{"tokens": rows, "resource_owners": owners, "repositories": repositories, "pending_requests": pending})
}

func (s *Server) personalAccessTokenRepositories(owner string) []map[string]interface{} {
	type repositoryRow struct {
		id      int
		name    string
		private bool
	}
	typed := []repositoryRow{}
	s.store.mu.RLock()
	for _, repo := range s.store.ReposByName {
		if strings.HasPrefix(repo.FullName, owner+"/") {
			typed = append(typed, repositoryRow{id: repo.ID, name: repo.Name, private: repo.Private})
		}
	}
	s.store.mu.RUnlock()
	sort.Slice(typed, func(i, j int) bool { return typed[i].name < typed[j].name })
	rows := make([]map[string]interface{}, 0, len(typed))
	for _, repo := range typed {
		rows = append(rows, map[string]interface{}{"id": repo.id, "name": repo.name, "private": repo.private})
	}
	return rows
}

type createPersonalAccessTokenWebRequest struct {
	Name                string            `json:"name"`
	ResourceOwner       string            `json:"resource_owner"`
	RepositorySelection string            `json:"repository_selection"`
	RepositoryIDs       []int             `json:"repository_ids"`
	Permissions         OrgPATPermissions `json:"permissions"`
	ExpiresAt           *time.Time        `json:"expires_at"`
	Reason              *string           `json:"reason"`
}

func (s *Server) handleCreatePersonalAccessTokenWeb(w http.ResponseWriter, r *http.Request) {
	user, r := s.personalAccessTokenWebUser(w, r)
	if user == nil {
		return
	}
	var body createPersonalAccessTokenWebRequest
	if !decodeJSONBody(w, r, &body) {
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" || len(body.Name) > 40 {
		writeGHValidationError(w, "FineGrainedPersonalAccessToken", "name", "invalid")
		return
	}
	if body.ResourceOwner == "" {
		body.ResourceOwner = user.Login
	}
	if body.RepositorySelection == "selected" {
		body.RepositorySelection = "subset"
	}
	if body.RepositorySelection != "all" && body.RepositorySelection != "subset" && body.RepositorySelection != "none" {
		writeGHValidationError(w, "FineGrainedPersonalAccessToken", "repository_selection", "invalid")
		return
	}
	if body.ExpiresAt != nil && !body.ExpiresAt.After(time.Now()) {
		writeGHValidationError(w, "FineGrainedPersonalAccessToken", "expires_at", "invalid")
		return
	}
	if !validPATPermissions(body.Permissions) {
		writeGHValidationError(w, "FineGrainedPersonalAccessToken", "permissions", "invalid")
		return
	}
	org := s.store.GetOrg(body.ResourceOwner)
	if body.ResourceOwner != user.Login && (org == nil || !isActiveOrgMember(s.store, user, org.Login)) {
		writeGHValidationError(w, "FineGrainedPersonalAccessToken", "resource_owner", "invalid")
		return
	}
	if body.RepositorySelection != "subset" && len(body.RepositoryIDs) != 0 {
		writeGHValidationError(w, "FineGrainedPersonalAccessToken", "repository_ids", "invalid")
		return
	}
	for _, id := range body.RepositoryIDs {
		repo := s.store.GetRepoByID(id)
		if repo == nil || !strings.HasPrefix(repo.FullName, body.ResourceOwner+"/") || !canReadRepo(s.store, user, repo) {
			writeGHValidationError(w, "FineGrainedPersonalAccessToken", "repository_ids", "invalid")
			return
		}
	}
	if s.store.CountFineGrainedPATs(user.ID) >= 50 {
		writeGHError(w, http.StatusUnprocessableEntity, "Fine-grained personal access token limit reached")
		return
	}
	var token *Token
	var err error
	if org != nil {
		request, createErr := s.store.CreateOrgPATGrantRequest(org.Login, user.ID, body.Name, body.Reason, body.RepositorySelection, body.RepositoryIDs, body.Permissions, body.ExpiresAt)
		err = createErr
		if request != nil {
			token, _ = s.store.LookupToken(request.TokenValue)
		}
	} else {
		token, err = s.store.CreateUserFineGrainedPAT(user.ID, body)
	}
	if err != nil || token == nil {
		s.logger.Error().Err(err).Msg("create fine-grained personal access token")
		writeGHError(w, http.StatusInternalServerError, "Internal Server Error")
		return
	}
	out := personalAccessTokenWebJSON(token, s.fineGrainedPATStatus(token))
	out["token"] = token.Value
	writeJSON(w, http.StatusCreated, out)
}

func validPATPermissions(perms OrgPATPermissions) bool {
	for _, group := range []map[string]string{perms.Organization, perms.Repository, perms.Other} {
		for _, level := range group {
			if !validPermLevelString(level) {
				return false
			}
		}
	}
	return true
}

func (st *Store) CreateUserFineGrainedPAT(userID int, body createPersonalAccessTokenWebRequest) (*Token, error) {
	value, err := newFineGrainedPATTokenFromReader(rand.Reader)
	if err != nil {
		return nil, err
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	token := &Token{Value: value, UserID: userID, CreatedAt: time.Now().UTC(), FineGrained: true, FineGrainedID: st.NextPATTokenID, Name: body.Name, ResourceOwner: body.ResourceOwner, RepositorySelection: body.RepositorySelection, RepositoryIDs: append([]int(nil), body.RepositoryIDs...), Permissions: body.Permissions, ExpiresAt: body.ExpiresAt}
	st.NextPATTokenID++
	st.Tokens[value] = token
	if st.persist != nil {
		st.persist.MustPut("tokens", value, token)
	}
	return token, nil
}

func (st *Store) CountFineGrainedPATs(userID int) int {
	st.mu.RLock()
	defer st.mu.RUnlock()
	count := 0
	for _, token := range st.Tokens {
		if token.UserID == userID && token.FineGrained {
			count++
		}
	}
	return count
}

func (s *Server) handleDeletePersonalAccessTokenWeb(w http.ResponseWriter, r *http.Request) {
	user, _ := s.personalAccessTokenWebUser(w, r)
	if user == nil {
		return
	}
	id, err := strconv.Atoi(r.PathValue("token_id"))
	if err != nil || !s.store.DeleteFineGrainedPAT(user.ID, id) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (st *Store) DeleteFineGrainedPAT(userID, tokenID int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	value := ""
	for candidate, token := range st.Tokens {
		if token.UserID == userID && token.FineGrained && token.FineGrainedID == tokenID {
			value = candidate
			break
		}
	}
	if value == "" {
		return false
	}
	delete(st.Tokens, value)
	if st.persist != nil {
		st.persist.MustDelete("tokens", value)
	}
	for org, requests := range st.OrgPATGrantRequests {
		for id, request := range requests {
			if request.TokenID == tokenID {
				delete(requests, id)
				st.persistOrgPATGrantRequestsLocked(org)
			}
		}
	}
	for org, grants := range st.OrgPATGrants {
		for id, grant := range grants {
			if grant.TokenID == tokenID {
				delete(grants, id)
				st.persistOrgPATGrantsLocked(org)
			}
		}
	}
	return true
}

func (s *Server) handleReviewPersonalAccessTokenWeb(w http.ResponseWriter, r *http.Request) {
	user, _ := s.personalAccessTokenWebUser(w, r)
	org := s.store.GetOrg(r.PathValue("org"))
	if user == nil {
		return
	}
	if org == nil || !canAdminOrg(s.store, user, org) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	id, err := strconv.Atoi(r.PathValue("pat_request_id"))
	var body struct {
		Action string `json:"action"`
	}
	if err != nil || !decodeJSONBody(w, r, &body) {
		return
	}
	if !validPATReviewAction(body.Action) {
		writeGHValidationError(w, "OrganizationProgrammaticAccessGrantRequest", "action", "invalid")
		return
	}
	if !s.store.ReviewOrgPATGrantRequest(org.Login, id, body.Action == "approve") {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
