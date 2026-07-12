package bleephub

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Fine-grained personal access token administration for organizations
// (/orgs/{org}/personal-access-token-requests and
// /orgs/{org}/personal-access-tokens).
//
// A fine-grained PAT targeting an organization starts life as a pending
// grant request; an org admin approves it (the request becomes an active
// grant) or denies it (the request is removed). Approved grants can later
// be revoked, individually or in bulk. The authenticated browser settings
// flow mints the live `github_pat_` credential and displays it once; the
// public organization REST surface remains GitHub App-only.

// OrgPATPermissions groups the permissions a fine-grained PAT requested,
// mirroring the organization-programmatic-access-grant permissions shape.
type OrgPATPermissions struct {
	Organization map[string]string `json:"organization,omitempty"`
	Repository   map[string]string `json:"repository,omitempty"`
	Other        map[string]string `json:"other,omitempty"`
}

// OrgPATGrantRequest is a pending request for organization access via a
// fine-grained personal access token.
type OrgPATGrantRequest struct {
	ID                  int               `json:"id"`
	OrgLogin            string            `json:"org_login"`
	OwnerUserID         int               `json:"owner_user_id"`
	TokenID             int               `json:"token_id"`
	TokenName           string            `json:"token_name"`
	TokenValue          string            `json:"token_value,omitempty"`
	Reason              *string           `json:"reason"`
	RepositorySelection string            `json:"repository_selection"` // none | all | subset
	RepositoryIDs       []int             `json:"repository_ids,omitempty"`
	Permissions         OrgPATPermissions `json:"permissions"`
	TokenExpiresAt      *time.Time        `json:"token_expires_at"`
	CreatedAt           time.Time         `json:"created_at"`
}

// OrgPATGrant is an approved fine-grained personal access token grant.
type OrgPATGrant struct {
	ID                  int               `json:"id"`
	OrgLogin            string            `json:"org_login"`
	OwnerUserID         int               `json:"owner_user_id"`
	TokenID             int               `json:"token_id"`
	TokenName           string            `json:"token_name"`
	TokenValue          string            `json:"token_value,omitempty"`
	RepositorySelection string            `json:"repository_selection"`
	RepositoryIDs       []int             `json:"repository_ids,omitempty"`
	Permissions         OrgPATPermissions `json:"permissions"`
	TokenExpiresAt      *time.Time        `json:"token_expires_at"`
	AccessGrantedAt     time.Time         `json:"access_granted_at"`
}

func (s *Server) registerGHOrgPATAdminRoutes() {
	s.route("GET /api/v3/orgs/{org}/personal-access-token-requests", s.requireOrgPATApp(scopePATRequests, permRead, s.handleListOrgPATGrantRequests))
	s.route("POST /api/v3/orgs/{org}/personal-access-token-requests", s.requireOrgPATApp(scopePATRequests, permWrite, s.handleReviewOrgPATGrantRequestsInBulk))
	s.route("POST /api/v3/orgs/{org}/personal-access-token-requests/{pat_request_id}", s.requireOrgPATApp(scopePATRequests, permWrite, s.handleReviewOrgPATGrantRequest))
	s.route("GET /api/v3/orgs/{org}/personal-access-token-requests/{pat_request_id}/repositories", s.requireOrgPATApp(scopePATRequests, permRead, s.handleListOrgPATGrantRequestRepositories))
	s.route("GET /api/v3/orgs/{org}/personal-access-tokens", s.requireOrgPATApp(scopePATs, permRead, s.handleListOrgPATGrants))
	s.route("POST /api/v3/orgs/{org}/personal-access-tokens", s.requireOrgPATApp(scopePATs, permWrite, s.handleUpdateOrgPATAccesses))
	s.route("POST /api/v3/orgs/{org}/personal-access-tokens/{pat_id}", s.requireOrgPATApp(scopePATs, permWrite, s.handleUpdateOrgPATAccess))
	s.route("GET /api/v3/orgs/{org}/personal-access-tokens/{pat_id}/repositories", s.requireOrgPATApp(scopePATs, permRead, s.handleListOrgPATGrantRepositories))

	s.route("GET /settings/personal-access-tokens", s.handleListPersonalAccessTokensWeb)
	s.route("POST /settings/personal-access-tokens", s.handleCreatePersonalAccessTokenWeb)
	s.route("DELETE /settings/personal-access-tokens/{token_id}", s.handleDeletePersonalAccessTokenWeb)
	s.route("POST /settings/organizations/{org}/personal-access-token-requests/{pat_request_id}", s.handleReviewPersonalAccessTokenWeb)
}

// requireOrgPATApp matches GitHub's organization token-administration
// contract: only GitHub App installation and user access tokens may call the
// REST endpoints. An organization owner's personal access token is not an
// alternate authentication shape for this API.
func (s *Server) requireOrgPATApp(scope permScope, level permLevel, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		org := s.store.GetOrg(r.PathValue("org"))
		if org == nil {
			writeGHError(w, http.StatusNotFound, "Not Found")
			return
		}
		if token := ghInstallationTokenFromContext(r.Context()); token != nil {
			installation := ghInstallationFromContext(r.Context())
			if installation == nil || installation.TargetType != "Organization" || !strings.EqualFold(installation.TargetLogin, org.Login) || !hasPerm(token.Permissions, scope, level) {
				writeGHError(w, http.StatusForbidden, "Resource not accessible by integration")
				return
			}
			next(w, r)
			return
		}
		if token := ghUserToServerTokenFromContext(r.Context()); token != nil && token.AppID > 0 && s.userAccessTokenCanAdminPATs(token, org.Login, scope, level) {
			next(w, r)
			return
		}
		writeGHError(w, http.StatusForbidden, "Resource not accessible by integration")
	}
}

func (s *Server) userAccessTokenCanAdminPATs(token *UserToServerToken, orgLogin string, scope permScope, level permLevel) bool {
	allowed := map[int]bool{}
	for _, id := range token.InstallationIDs {
		allowed[id] = true
	}
	s.store.mu.RLock()
	defer s.store.mu.RUnlock()
	for id, installation := range s.store.Installations {
		if installation.AppID != token.AppID || installation.TargetType != "Organization" || !strings.EqualFold(installation.TargetLogin, orgLogin) {
			continue
		}
		if len(allowed) > 0 && !allowed[id] {
			continue
		}
		if hasPerm(installation.Permissions, scope, level) {
			return true
		}
	}
	return false
}

// ─── store methods ───────────────────────────────────────────────────────

func (st *Store) persistOrgPATGrantRequestsLocked(orgLogin string) {
	if st.persist == nil {
		return
	}
	if m := st.OrgPATGrantRequests[orgLogin]; len(m) > 0 {
		st.persist.MustPut("org_pat_grant_requests", orgLogin, m)
	} else {
		st.persist.MustDelete("org_pat_grant_requests", orgLogin)
	}
}

func (st *Store) persistOrgPATGrantsLocked(orgLogin string) {
	if st.persist == nil {
		return
	}
	if m := st.OrgPATGrants[orgLogin]; len(m) > 0 {
		st.persist.MustPut("org_pat_grants", orgLogin, m)
	} else {
		st.persist.MustDelete("org_pat_grants", orgLogin)
	}
}

// CreateOrgPATGrantRequest mints a real fine-grained token for the user and
// files the pending grant request that references it.
func (st *Store) CreateOrgPATGrantRequest(orgLogin string, ownerUserID int, tokenName string, reason *string, repositorySelection string, repositoryIDs []int, perms OrgPATPermissions, expiresAt *time.Time) (*OrgPATGrantRequest, error) {
	return st.createOrgPATGrantRequestWithRandom(orgLogin, ownerUserID, tokenName, reason, repositorySelection, repositoryIDs, perms, expiresAt, rand.Reader)
}

func (st *Store) createOrgPATGrantRequestWithRandom(orgLogin string, ownerUserID int, tokenName string, reason *string, repositorySelection string, repositoryIDs []int, perms OrgPATPermissions, expiresAt *time.Time, random io.Reader) (*OrgPATGrantRequest, error) {
	st.mu.Lock()
	defer st.mu.Unlock()

	value, err := newFineGrainedPATTokenFromReader(random)
	if err != nil {
		return nil, fmt.Errorf("generate fine-grained token: %w", err)
	}
	now := time.Now().UTC()
	tok := &Token{
		Value: value, UserID: ownerUserID, CreatedAt: now, FineGrained: true,
		FineGrainedID: st.NextPATTokenID, Name: tokenName, ResourceOwner: orgLogin,
		RepositorySelection: repositorySelection, RepositoryIDs: append([]int(nil), repositoryIDs...),
		Permissions: perms, ExpiresAt: expiresAt,
	}
	st.Tokens[value] = tok
	if st.persist != nil {
		st.persist.MustPut("tokens", tok.Value, tok)
	}

	req := &OrgPATGrantRequest{
		ID:                  st.NextPATRequestID,
		OrgLogin:            orgLogin,
		OwnerUserID:         ownerUserID,
		TokenID:             st.NextPATTokenID,
		TokenName:           tokenName,
		TokenValue:          value,
		Reason:              reason,
		RepositorySelection: repositorySelection,
		RepositoryIDs:       repositoryIDs,
		Permissions:         perms,
		TokenExpiresAt:      expiresAt,
		CreatedAt:           now,
	}
	st.NextPATRequestID++
	st.NextPATTokenID++
	if st.OrgPATGrantRequests[orgLogin] == nil {
		st.OrgPATGrantRequests[orgLogin] = map[int]*OrgPATGrantRequest{}
	}
	st.OrgPATGrantRequests[orgLogin][req.ID] = req
	st.persistOrgPATGrantRequestsLocked(orgLogin)
	return req, nil
}

func newFineGrainedPATTokenFromReader(random io.Reader) (string, error) {
	buf := make([]byte, 20)
	if _, err := io.ReadFull(random, buf); err != nil {
		return "", fmt.Errorf("fine-grained personal access token: %w", err)
	}
	return "github_pat_" + hex.EncodeToString(buf), nil
}

// ReviewOrgPATGrantRequest resolves a pending request: approve converts it
// into an active grant, deny removes it. Returns false when the request
// does not exist.
func (st *Store) ReviewOrgPATGrantRequest(orgLogin string, requestID int, approve bool) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	req := st.OrgPATGrantRequests[orgLogin][requestID]
	if req == nil {
		return false
	}
	delete(st.OrgPATGrantRequests[orgLogin], requestID)
	st.persistOrgPATGrantRequestsLocked(orgLogin)

	if approve {
		grant := &OrgPATGrant{
			ID:                  st.NextPATGrantID,
			OrgLogin:            orgLogin,
			OwnerUserID:         req.OwnerUserID,
			TokenID:             req.TokenID,
			TokenName:           req.TokenName,
			TokenValue:          req.TokenValue,
			RepositorySelection: req.RepositorySelection,
			RepositoryIDs:       req.RepositoryIDs,
			Permissions:         req.Permissions,
			TokenExpiresAt:      req.TokenExpiresAt,
			AccessGrantedAt:     time.Now().UTC(),
		}
		st.NextPATGrantID++
		if st.OrgPATGrants[orgLogin] == nil {
			st.OrgPATGrants[orgLogin] = map[int]*OrgPATGrant{}
		}
		st.OrgPATGrants[orgLogin][grant.ID] = grant
		st.persistOrgPATGrantsLocked(orgLogin)
	}
	return true
}

// RevokeOrgPATGrant removes an active grant. Returns false when it does not
// exist.
func (st *Store) RevokeOrgPATGrant(orgLogin string, grantID int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.OrgPATGrants[orgLogin][grantID] == nil {
		return false
	}
	delete(st.OrgPATGrants[orgLogin], grantID)
	st.persistOrgPATGrantsLocked(orgLogin)
	return true
}

// ─── rendering ───────────────────────────────────────────────────────────

func patPermissionsJSON(p OrgPATPermissions) map[string]interface{} {
	out := map[string]interface{}{}
	if len(p.Organization) > 0 {
		out["organization"] = p.Organization
	}
	if len(p.Repository) > 0 {
		out["repository"] = p.Repository
	}
	if len(p.Other) > 0 {
		out["other"] = p.Other
	}
	return out
}

func patTokenExpiryJSON(expiresAt *time.Time) (expired bool, expiresJSON interface{}) {
	if expiresAt == nil {
		return false, nil
	}
	return expiresAt.Before(time.Now()), expiresAt.UTC().Format(time.RFC3339)
}

func (s *Server) patGrantRequestJSON(req *OrgPATGrantRequest, baseURL string) map[string]interface{} {
	owner := map[string]interface{}(nil)
	if u := s.store.GetUserByID(req.OwnerUserID); u != nil {
		owner = userToJSON(u)
	}
	expired, expiresJSON := patTokenExpiryJSON(req.TokenExpiresAt)
	var reason interface{}
	if req.Reason != nil {
		reason = *req.Reason
	}
	return map[string]interface{}{
		"id":                   req.ID,
		"reason":               reason,
		"owner":                owner,
		"repository_selection": req.RepositorySelection,
		"repositories_url":     baseURL + "/api/v3/orgs/" + req.OrgLogin + "/personal-access-token-requests/" + strconv.Itoa(req.ID) + "/repositories",
		"permissions":          patPermissionsJSON(req.Permissions),
		"created_at":           req.CreatedAt.UTC().Format(time.RFC3339),
		"token_id":             req.TokenID,
		"token_name":           req.TokenName,
		"token_expired":        expired,
		"token_expires_at":     expiresJSON,
		"token_last_used_at":   nil,
	}
}

func (s *Server) patGrantJSON(g *OrgPATGrant, baseURL string) map[string]interface{} {
	owner := map[string]interface{}(nil)
	if u := s.store.GetUserByID(g.OwnerUserID); u != nil {
		owner = userToJSON(u)
	}
	expired, expiresJSON := patTokenExpiryJSON(g.TokenExpiresAt)
	return map[string]interface{}{
		"id":                   g.ID,
		"owner":                owner,
		"repository_selection": g.RepositorySelection,
		"repositories_url":     baseURL + "/api/v3/orgs/" + g.OrgLogin + "/personal-access-tokens/" + strconv.Itoa(g.ID) + "/repositories",
		"permissions":          patPermissionsJSON(g.Permissions),
		"access_granted_at":    g.AccessGrantedAt.UTC().Format(time.RFC3339),
		"token_id":             g.TokenID,
		"token_name":           g.TokenName,
		"token_expired":        expired,
		"token_expires_at":     expiresJSON,
		"token_last_used_at":   nil,
	}
}

// patListFilters applies the shared owner/token_id filters and
// sort/direction ordering of the two PAT listing endpoints. rows must carry
// ownerLogin/tokenID/createdAt extraction closures.
type patListRow struct {
	ownerLogin string
	tokenID    int
	createdAt  time.Time
	json       map[string]interface{}
}

func filterAndSortPATRows(r *http.Request, rows []patListRow) []map[string]interface{} {
	q := r.URL.Query()
	if owners := q["owner"]; len(owners) > 0 {
		ownerSet := map[string]bool{}
		for _, o := range owners {
			ownerSet[strings.ToLower(o)] = true
		}
		kept := rows[:0]
		for _, row := range rows {
			if ownerSet[strings.ToLower(row.ownerLogin)] {
				kept = append(kept, row)
			}
		}
		rows = kept
	}
	if tokenIDs := q["token_id"]; len(tokenIDs) > 0 {
		idSet := map[int]bool{}
		for _, raw := range tokenIDs {
			for _, part := range strings.Split(raw, ",") {
				if id, err := strconv.Atoi(strings.TrimSpace(part)); err == nil {
					idSet[id] = true
				}
			}
		}
		kept := rows[:0]
		for _, row := range rows {
			if idSet[row.tokenID] {
				kept = append(kept, row)
			}
		}
		rows = kept
	}
	asc := q.Get("direction") == "asc"
	sort.SliceStable(rows, func(i, j int) bool {
		if asc {
			return rows[i].createdAt.Before(rows[j].createdAt)
		}
		return rows[j].createdAt.Before(rows[i].createdAt)
	})
	out := make([]map[string]interface{}, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.json)
	}
	return out
}

// ─── handlers ────────────────────────────────────────────────────────────

func (s *Server) handleListOrgPATGrantRequests(w http.ResponseWriter, r *http.Request) {
	orgLogin := r.PathValue("org")
	base := s.baseURL(r)

	s.store.mu.RLock()
	requests := make([]*OrgPATGrantRequest, 0, len(s.store.OrgPATGrantRequests[orgLogin]))
	for _, req := range s.store.OrgPATGrantRequests[orgLogin] {
		requests = append(requests, req)
	}
	s.store.mu.RUnlock()

	rows := make([]patListRow, 0, len(requests))
	for _, req := range requests {
		ownerLogin := ""
		if u := s.store.GetUserByID(req.OwnerUserID); u != nil {
			ownerLogin = u.Login
		}
		rows = append(rows, patListRow{
			ownerLogin: ownerLogin,
			tokenID:    req.TokenID,
			createdAt:  req.CreatedAt,
			json:       s.patGrantRequestJSON(req, base),
		})
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, filterAndSortPATRows(r, rows)))
}

func (s *Server) handleListOrgPATGrants(w http.ResponseWriter, r *http.Request) {
	orgLogin := r.PathValue("org")
	base := s.baseURL(r)

	s.store.mu.RLock()
	grants := make([]*OrgPATGrant, 0, len(s.store.OrgPATGrants[orgLogin]))
	for _, g := range s.store.OrgPATGrants[orgLogin] {
		grants = append(grants, g)
	}
	s.store.mu.RUnlock()

	rows := make([]patListRow, 0, len(grants))
	for _, g := range grants {
		ownerLogin := ""
		if u := s.store.GetUserByID(g.OwnerUserID); u != nil {
			ownerLogin = u.Login
		}
		rows = append(rows, patListRow{
			ownerLogin: ownerLogin,
			tokenID:    g.TokenID,
			createdAt:  g.AccessGrantedAt,
			json:       s.patGrantJSON(g, base),
		})
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, filterAndSortPATRows(r, rows)))
}

func validPATReviewAction(action string) bool { return action == "approve" || action == "deny" }

func (s *Server) handleReviewOrgPATGrantRequestsInBulk(w http.ResponseWriter, r *http.Request) {
	orgLogin := r.PathValue("org")
	var req struct {
		PATRequestIDs []int   `json:"pat_request_ids"`
		Action        string  `json:"action"`
		Reason        *string `json:"reason"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if !validPATReviewAction(req.Action) {
		writeGHValidationError(w, "OrganizationProgrammaticAccessGrantRequest", "action", "invalid")
		return
	}
	if len(req.PATRequestIDs) < 1 || len(req.PATRequestIDs) > 100 {
		writeGHValidationError(w, "OrganizationProgrammaticAccessGrantRequest", "pat_request_ids", "invalid")
		return
	}
	for _, id := range req.PATRequestIDs {
		if s.store.GetOrgPATGrantRequest(orgLogin, id) == nil {
			writeGHError(w, http.StatusNotFound, "Not Found")
			return
		}
	}
	for _, id := range req.PATRequestIDs {
		s.store.ReviewOrgPATGrantRequest(orgLogin, id, req.Action == "approve")
	}
	writeJSON(w, http.StatusAccepted, map[string]interface{}{})
}

func (s *Server) handleReviewOrgPATGrantRequest(w http.ResponseWriter, r *http.Request) {
	orgLogin := r.PathValue("org")
	requestID, err := strconv.Atoi(r.PathValue("pat_request_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	var req struct {
		Action string  `json:"action"`
		Reason *string `json:"reason"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if !validPATReviewAction(req.Action) {
		writeGHValidationError(w, "OrganizationProgrammaticAccessGrantRequest", "action", "invalid")
		return
	}
	if !s.store.ReviewOrgPATGrantRequest(orgLogin, requestID, req.Action == "approve") {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetOrgPATGrantRequest returns a pending grant request by ID, or nil.
func (st *Store) GetOrgPATGrantRequest(orgLogin string, id int) *OrgPATGrantRequest {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.OrgPATGrantRequests[orgLogin][id]
}

// GetOrgPATGrant returns an active grant by ID, or nil.
func (st *Store) GetOrgPATGrant(orgLogin string, id int) *OrgPATGrant {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.OrgPATGrants[orgLogin][id]
}

func (s *Server) handleUpdateOrgPATAccesses(w http.ResponseWriter, r *http.Request) {
	orgLogin := r.PathValue("org")
	var req struct {
		Action string `json:"action"`
		PATIDs []int  `json:"pat_ids"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Action != "revoke" {
		writeGHValidationError(w, "OrganizationProgrammaticAccessGrant", "action", "invalid")
		return
	}
	if len(req.PATIDs) < 1 || len(req.PATIDs) > 100 {
		writeGHValidationError(w, "OrganizationProgrammaticAccessGrant", "pat_ids", "invalid")
		return
	}
	for _, id := range req.PATIDs {
		if s.store.GetOrgPATGrant(orgLogin, id) == nil {
			writeGHError(w, http.StatusNotFound, "Not Found")
			return
		}
	}
	for _, id := range req.PATIDs {
		s.store.RevokeOrgPATGrant(orgLogin, id)
	}
	writeJSON(w, http.StatusAccepted, map[string]interface{}{})
}

func (s *Server) handleUpdateOrgPATAccess(w http.ResponseWriter, r *http.Request) {
	orgLogin := r.PathValue("org")
	grantID, err := strconv.Atoi(r.PathValue("pat_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	var req struct {
		Action string `json:"action"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Action != "revoke" {
		writeGHValidationError(w, "OrganizationProgrammaticAccessGrant", "action", "invalid")
		return
	}
	if !s.store.RevokeOrgPATGrant(orgLogin, grantID) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// writePATRepositoriesResponse renders the repositories a grant/request can
// access: every org repository for "all", the selected subset for "subset",
// none otherwise.
func (s *Server) writePATRepositoriesResponse(w http.ResponseWriter, r *http.Request, org *Org, selection string, repositoryIDs []int) {
	var repos []*Repo
	switch selection {
	case "all":
		s.store.mu.RLock()
		for _, repo := range s.store.Repos {
			if repo.OwnerType == "Organization" && repo.OwnerID == org.ID {
				repos = append(repos, repo)
			}
		}
		s.store.mu.RUnlock()
	case "subset":
		for _, id := range repositoryIDs {
			if repo := s.store.GetRepoByID(id); repo != nil {
				repos = append(repos, repo)
			}
		}
	}
	sort.Slice(repos, func(i, j int) bool { return repos[i].ID < repos[j].ID })

	base := s.baseURL(r)
	out := make([]map[string]interface{}, 0, len(repos))
	for _, repo := range repos {
		out = append(out, minimalRepoJSON(repo, s.store, base))
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, out))
}

func (s *Server) handleListOrgPATGrantRequestRepositories(w http.ResponseWriter, r *http.Request) {
	org := s.store.GetOrg(r.PathValue("org"))
	requestID, err := strconv.Atoi(r.PathValue("pat_request_id"))
	if err != nil || org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	req := s.store.GetOrgPATGrantRequest(org.Login, requestID)
	if req == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.writePATRepositoriesResponse(w, r, org, req.RepositorySelection, req.RepositoryIDs)
}

func (s *Server) handleListOrgPATGrantRepositories(w http.ResponseWriter, r *http.Request) {
	org := s.store.GetOrg(r.PathValue("org"))
	grantID, err := strconv.Atoi(r.PathValue("pat_id"))
	if err != nil || org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	g := s.store.GetOrgPATGrant(org.Login, grantID)
	if g == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.writePATRepositoriesResponse(w, r, org, g.RepositorySelection, g.RepositoryIDs)
}
