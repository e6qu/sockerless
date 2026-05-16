package bleephub

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// OAuth applications token management endpoints.
// Real GitHub exposes a parallel surface for OAuth Apps + GitHub Apps acting
// as OAuth clients. Authentication is HTTP Basic with client_id:client_secret.
// `client_id` resolves to either an OAuth App or a GitHub App (looked up in
// Store.OAuthApps then Store.AppsByClientID).
//
// Endpoints implemented:
//   POST   /applications/{client_id}/token         check token validity
//   PATCH  /applications/{client_id}/token         reset (rotate) token
//   DELETE /applications/{client_id}/token         revoke token
//   POST   /applications/{client_id}/token/scoped  scope user-to-server token
//   DELETE /applications/{client_id}/grant         revoke user grant

func (s *Server) registerGHAppsOAuthMgmtRoutes() {
	s.mux.HandleFunc("POST /api/v3/applications/{client_id}/token", s.handleCheckOAuthToken)
	s.mux.HandleFunc("PATCH /api/v3/applications/{client_id}/token", s.handleResetOAuthToken)
	s.mux.HandleFunc("DELETE /api/v3/applications/{client_id}/token", s.handleRevokeOAuthToken)
	s.mux.HandleFunc("POST /api/v3/applications/{client_id}/token/scoped", s.handleScopeOAuthToken)
	s.mux.HandleFunc("DELETE /api/v3/applications/{client_id}/grant", s.handleRevokeOAuthGrant)

	// OAuth App management (sim-only convenience for the UI / tests).
	s.mux.HandleFunc("POST /api/v3/bleephub/oauth-apps", s.handleCreateOAuthAppMgmt)
	s.mux.HandleFunc("GET /api/v3/bleephub/oauth-apps", s.handleListOAuthAppsMgmt)
}

// authenticateClientCreds reads + verifies HTTP Basic auth carrying
// client_id:client_secret against either OAuthApps or AppsByClientID.
// On match returns (clientID, isOAuthApp). On miss writes 401 + returns ("", false).
func (s *Server) authenticateClientCreds(w http.ResponseWriter, r *http.Request, pathClientID string) (string, bool) {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Basic ") {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return "", false
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(auth, "Basic "))
	if err != nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return "", false
	}
	parts := strings.SplitN(string(raw), ":", 2)
	if len(parts) != 2 {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return "", false
	}
	cid, secret := parts[0], parts[1]
	if cid != pathClientID {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return "", false
	}
	if oa := s.store.VerifyOAuthAppSecret(cid, secret); oa != nil {
		return cid, true
	}
	if a := s.store.VerifyAppClientSecret(cid, secret); a != nil {
		return cid, false
	}
	writeGHError(w, http.StatusUnauthorized, "Bad credentials")
	return "", false
}

func (s *Server) handleCheckOAuthToken(w http.ResponseWriter, r *http.Request) {
	clientID, _ := s.authenticateClientCreds(w, r, r.PathValue("client_id"))
	if clientID == "" {
		return
	}
	var body struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.AccessToken == "" {
		writeGHError(w, http.StatusUnprocessableEntity, "access_token required")
		return
	}
	tok, user := s.store.LookupUserToServerToken(body.AccessToken)
	if tok == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	// Token must belong to this client_id (either as OAuth App or as GitHub App's OAuth client).
	if !tokenMatchesClient(tok, clientID, s.store) {
		writeGHError(w, http.StatusUnprocessableEntity, "token does not match client_id")
		return
	}
	writeJSON(w, http.StatusOK, oauthTokenInspectionJSON(tok, user))
}

func (s *Server) handleResetOAuthToken(w http.ResponseWriter, r *http.Request) {
	clientID, _ := s.authenticateClientCreds(w, r, r.PathValue("client_id"))
	if clientID == "" {
		return
	}
	var body struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.AccessToken == "" {
		writeGHError(w, http.StatusUnprocessableEntity, "access_token required")
		return
	}
	tok, _ := s.store.LookupUserToServerToken(body.AccessToken)
	if tok == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if !tokenMatchesClient(tok, clientID, s.store) {
		writeGHError(w, http.StatusUnprocessableEntity, "token does not match client_id")
		return
	}
	// Revoke old + mint fresh pair carrying same scopes + user.
	s.store.RevokeUserToServerToken(tok.Token)
	fresh, refresh := s.store.CreateUserToServerToken(tok.UserID, tok.AppID, tok.OAuthAppClientID, tok.Scopes, 8*time.Hour, tok.RefreshTokenValue != "")
	user, _ := s.store.LookupUserToServerToken(fresh.Token)
	_ = user
	resp := oauthTokenInspectionJSON(fresh, s.userByID(fresh.UserID))
	resp["token"] = fresh.Token
	if refresh != nil {
		resp["refresh_token"] = refresh.Token
		resp["refresh_token_expires_in"] = int(time.Until(refresh.ExpiresAt).Seconds())
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleRevokeOAuthToken(w http.ResponseWriter, r *http.Request) {
	clientID, _ := s.authenticateClientCreds(w, r, r.PathValue("client_id"))
	if clientID == "" {
		return
	}
	var body struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.AccessToken == "" {
		writeGHError(w, http.StatusUnprocessableEntity, "access_token required")
		return
	}
	tok, _ := s.store.LookupUserToServerToken(body.AccessToken)
	if tok == nil {
		w.WriteHeader(http.StatusNoContent) // idempotent
		return
	}
	if !tokenMatchesClient(tok, clientID, s.store) {
		writeGHError(w, http.StatusUnprocessableEntity, "token does not match client_id")
		return
	}
	s.store.RevokeUserToServerToken(tok.Token)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleScopeOAuthToken(w http.ResponseWriter, r *http.Request) {
	clientID, _ := s.authenticateClientCreds(w, r, r.PathValue("client_id"))
	if clientID == "" {
		return
	}
	var body struct {
		AccessToken   string            `json:"access_token"`
		Target        string            `json:"target"`
		TargetID      int               `json:"target_id"`
		Permissions   map[string]string `json:"permissions"`
		Repositories  []string          `json:"repositories"`
		RepositoryIDs []int             `json:"repository_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.AccessToken == "" {
		writeGHError(w, http.StatusUnprocessableEntity, "access_token required")
		return
	}
	tok, _ := s.store.LookupUserToServerToken(body.AccessToken)
	if tok == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if !tokenMatchesClient(tok, clientID, s.store) {
		writeGHError(w, http.StatusUnprocessableEntity, "token does not match client_id")
		return
	}
	// Bleephub's sim accepts target / target_id but doesn't enforce installation
	// targeting at the token level. Returns the same token unmodified so
	// SDK contract holds. Real GH would return a freshly-narrowed token.
	writeJSON(w, http.StatusOK, oauthTokenInspectionJSON(tok, s.userByID(tok.UserID)))
}

func (s *Server) handleRevokeOAuthGrant(w http.ResponseWriter, r *http.Request) {
	clientID, _ := s.authenticateClientCreds(w, r, r.PathValue("client_id"))
	if clientID == "" {
		return
	}
	var body struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.AccessToken == "" {
		writeGHError(w, http.StatusUnprocessableEntity, "access_token required")
		return
	}
	tok, _ := s.store.LookupUserToServerToken(body.AccessToken)
	if tok == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !tokenMatchesClient(tok, clientID, s.store) {
		writeGHError(w, http.StatusUnprocessableEntity, "token does not match client_id")
		return
	}
	s.store.RevokeUserGrant(clientID, tok.UserID)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleCreateOAuthAppMgmt(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		URL         string `json:"url"`
		CallbackURL string `json:"callback_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		writeGHValidationError(w, "OAuthApp", "name", "missing_field")
		return
	}
	app := s.store.CreateOAuthApp(user.ID, req.Name, req.Description, req.URL, req.CallbackURL)
	writeJSON(w, http.StatusCreated, oauthAppToJSON(app, true))
}

func (s *Server) handleListOAuthAppsMgmt(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	apps := s.store.ListOAuthApps()
	out := make([]map[string]interface{}, 0, len(apps))
	for _, a := range apps {
		out = append(out, oauthAppToJSON(a, false))
	}
	writeJSON(w, http.StatusOK, out)
}

func tokenMatchesClient(tok *UserToServerToken, clientID string, st *Store) bool {
	if tok.OAuthAppClientID != "" {
		return tok.OAuthAppClientID == clientID
	}
	if tok.AppID > 0 {
		if app := st.AppsByClientID[clientID]; app != nil && app.ID == tok.AppID {
			return true
		}
	}
	return false
}

func (s *Server) userByID(id int) *User {
	s.store.mu.RLock()
	defer s.store.mu.RUnlock()
	return s.store.Users[id]
}

func oauthTokenInspectionJSON(tok *UserToServerToken, user *User) map[string]interface{} {
	app := map[string]interface{}{
		"client_id": tok.OAuthAppClientID,
		"name":      "",
		"url":       "",
	}
	if tok.OAuthAppClientID == "" && tok.AppID > 0 {
		app["client_id"] = "(app)"
	}
	out := map[string]interface{}{
		"id":          0,
		"url":         "",
		"scopes":      splitScopes(tok.Scopes),
		"token":       tok.Token,
		"app":         app,
		"note":        nil,
		"note_url":    nil,
		"updated_at":  tok.CreatedAt.UTC().Format(time.RFC3339),
		"created_at":  tok.CreatedAt.UTC().Format(time.RFC3339),
		"fingerprint": nil,
		"expires_at":  tok.ExpiresAt.UTC().Format(time.RFC3339),
	}
	if user != nil {
		out["user"] = userToJSON(user)
	}
	return out
}

func splitScopes(s string) []string {
	if s == "" {
		return []string{}
	}
	out := []string{}
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func oauthAppToJSON(a *OAuthApp, includeSecret bool) map[string]interface{} {
	out := map[string]interface{}{
		"client_id":    a.ClientID,
		"name":         a.Name,
		"description":  a.Description,
		"url":          a.URL,
		"callback_url": a.CallbackURL,
		"owner_id":     a.OwnerID,
		"created_at":   a.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":   a.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if includeSecret {
		out["client_secret"] = a.ClientSecret
	}
	return out
}
