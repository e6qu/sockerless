package bleephub

import (
	"fmt"
	"html"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
)

func (s *Server) registerGHOAuthRoutes() {
	s.mux.HandleFunc("POST /login/device/code", s.handleDeviceCode)
	s.mux.HandleFunc("POST /login/oauth/access_token", s.handleOAuthAccessToken)
	s.mux.HandleFunc("GET /login/device", s.handleDevicePage)
	// Phase 132 — OAuth web flow (companion to the device flow above).
	s.mux.HandleFunc("GET /login/oauth/authorize", s.handleOAuthAuthorize)
	s.mux.HandleFunc("POST /login/oauth/authorize", s.handleOAuthAuthorizeApprove)
}

// authCode is a one-time-use OAuth authorization code keyed off a
// client_id + state pair. Used by the web-flow endpoints below.
type authCode struct {
	Code        string
	ClientID    string
	RedirectURI string
	Scopes      string
	State       string
	UserID      int
	CreatedAt   time.Time
	ExpiresAt   time.Time
}

// handleDeviceCode initiates the device authorization flow.
// Auto-creates a device code with a pre-generated token for the default admin user.
func (s *Server) handleDeviceCode(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	scope := r.FormValue("scope")

	s.store.mu.Lock()
	// Use admin user (ID=1)
	adminUser := s.store.Users[1]
	token := s.store.createTokenLocked(adminUser.ID, "repo, read:org, gist")

	dc := &DeviceCode{
		Code:      uuid.New().String(),
		UserCode:  "BLEE-PHUB",
		Scopes:    scope,
		Token:     token.Value,
		UserID:    adminUser.ID,
		ExpiresAt: time.Now().Add(15 * time.Minute),
	}
	s.store.DeviceCodes[dc.Code] = dc
	s.store.mu.Unlock()

	s.logger.Info().Str("device_code", dc.Code).Msg("device code issued")

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"device_code":      dc.Code,
		"user_code":        dc.UserCode,
		"verification_uri": "http://" + r.Host + "/login/device",
		"expires_in":       900,
		"interval":         1,
	})
}

// handleOAuthAccessToken handles BOTH OAuth flows on the same shared
// endpoint, mirroring real GitHub. The grant is identified by which
// fields the form carries:
//
//   - device_code  → device flow (existing behaviour, auto-approved)
//   - code         → web flow with authorization code grant (Phase 132)
//
// Both return `{access_token, token_type, scope}` on success and
// `{error: ...}` on failure (200 OK with an error body, matching real
// GitHub's OAuth endpoint shape).
func (s *Server) handleOAuthAccessToken(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	if r.FormValue("device_code") != "" {
		s.handleDeviceTokenForm(w, r)
		return
	}
	if r.FormValue("code") != "" {
		s.handleWebFlowTokenForm(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"error":"unsupported_grant_type"}`))
}

// handleDeviceTokenForm — device-flow leg, preserved verbatim from the
// pre-Phase-132 handler. Auto-approved.
func (s *Server) handleDeviceTokenForm(w http.ResponseWriter, r *http.Request) {
	deviceCode := r.FormValue("device_code")
	s.store.mu.RLock()
	dc, ok := s.store.DeviceCodes[deviceCode]
	s.store.mu.RUnlock()

	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"error":"bad_verification_code"}`))
		return
	}

	s.logger.Info().Str("device_code", deviceCode).Msg("device token granted")

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"access_token": dc.Token,
		"token_type":   "bearer",
		"scope":        "repo read:org gist",
	})
}

// handleWebFlowTokenForm — web-flow leg (Phase 132). Exchanges a
// one-time-use authorization code (issued by /login/oauth/authorize)
// for an access token. Real GitHub validates client_id + client_secret;
// the sim doesn't gate on the secret (which the dispatcher-generic
// rule does not require us to verify) but does enforce one-time-use.
func (s *Server) handleWebFlowTokenForm(w http.ResponseWriter, r *http.Request) {
	code := r.FormValue("code")
	clientID := r.FormValue("client_id")

	s.store.mu.Lock()
	ac, ok := s.store.AuthCodes[code]
	if ok {
		delete(s.store.AuthCodes, code)
	}
	s.store.mu.Unlock()

	if !ok || time.Now().After(ac.ExpiresAt) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"error":"bad_verification_code"}`))
		return
	}
	if clientID != "" && ac.ClientID != "" && clientID != ac.ClientID {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"error":"incorrect_client_credentials"}`))
		return
	}

	s.store.mu.Lock()
	user := s.store.Users[ac.UserID]
	if user == nil {
		s.store.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"error":"server_error"}`))
		return
	}
	tok := s.store.createTokenLocked(user.ID, ac.Scopes)
	s.store.mu.Unlock()

	s.logger.Info().Str("auth_code", code).Int("user_id", user.ID).Msg("web flow token granted")
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"access_token": tok.Value,
		"token_type":   "bearer",
		"scope":        ac.Scopes,
	})
}

// handleDevicePage renders a simple HTML confirmation page.
func (s *Server) handleDevicePage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(`<!DOCTYPE html><html><body><h1>Auto-approved by bleephub</h1><p>You can close this page.</p></body></html>`))
}

// handleOAuthAuthorize — GET /login/oauth/authorize.
// Real GitHub: renders an HTML "Authorize" page. After the user
// clicks Authorize, GitHub redirects to `redirect_uri?code=…&state=…`
// (302). Bleephub renders a minimal HTML form with an Authorize button
// that POSTs back to the same path; the POST handler issues the auth
// code and 302s. Operators who want a one-step auto-approve can pass
// `?auto=1` on the GET, which skips the form and 302s immediately —
// matches the device-flow auto-approval pattern.
func (s *Server) handleOAuthAuthorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	clientID := q.Get("client_id")
	redirectURI := q.Get("redirect_uri")
	scopes := q.Get("scope")
	state := q.Get("state")
	if clientID == "" || redirectURI == "" {
		writeGHError(w, http.StatusBadRequest, "client_id and redirect_uri are required")
		return
	}
	if q.Get("auto") == "1" {
		s.completeAuthorize(w, r, clientID, redirectURI, scopes, state)
		return
	}
	page := fmt.Sprintf(`<!DOCTYPE html><html><head><title>Authorize bleephub</title></head>
<body style="font-family:system-ui,sans-serif;max-width:480px;margin:48px auto">
<h1>Authorize app</h1>
<p>The app <code>%s</code> is requesting access to your bleephub account with scopes <code>%s</code>.</p>
<form method="POST" action="/login/oauth/authorize">
  <input type="hidden" name="client_id" value="%s"/>
  <input type="hidden" name="redirect_uri" value="%s"/>
  <input type="hidden" name="scope" value="%s"/>
  <input type="hidden" name="state" value="%s"/>
  <button type="submit" style="padding:8px 16px;background:#2da44e;color:white;border:0;border-radius:6px;font-size:14px;cursor:pointer">Authorize</button>
</form>
</body></html>`,
		html.EscapeString(clientID),
		html.EscapeString(scopes),
		html.EscapeString(clientID),
		html.EscapeString(redirectURI),
		html.EscapeString(scopes),
		html.EscapeString(state),
	)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(page))
}

// handleOAuthAuthorizeApprove handles the POST that the authorize
// page submits. Issues the auth code + 302s the user back to the
// caller's redirect_uri.
func (s *Server) handleOAuthAuthorizeApprove(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	clientID := r.FormValue("client_id")
	redirectURI := r.FormValue("redirect_uri")
	scopes := r.FormValue("scope")
	state := r.FormValue("state")
	if clientID == "" || redirectURI == "" {
		writeGHError(w, http.StatusBadRequest, "client_id and redirect_uri are required")
		return
	}
	s.completeAuthorize(w, r, clientID, redirectURI, scopes, state)
}

// completeAuthorize mints a one-time-use auth code, stores it, and
// 302s back to redirect_uri with code + state. Shared by the auto-
// approve GET path and the form POST path.
func (s *Server) completeAuthorize(w http.ResponseWriter, r *http.Request, clientID, redirectURI, scopes, state string) {
	s.store.mu.Lock()
	if s.store.AuthCodes == nil {
		s.store.AuthCodes = map[string]*authCode{}
	}
	adminUser := s.store.Users[1]
	if adminUser == nil {
		// Pick any user — sim is unscoped per existing oauth handler.
		for _, u := range s.store.Users {
			adminUser = u
			break
		}
	}
	if adminUser == nil {
		s.store.mu.Unlock()
		writeGHError(w, http.StatusInternalServerError, "no user available")
		return
	}
	code := uuid.New().String()
	if scopes == "" {
		scopes = "repo read:org gist"
	}
	s.store.AuthCodes[code] = &authCode{
		Code:        code,
		ClientID:    clientID,
		RedirectURI: redirectURI,
		Scopes:      scopes,
		State:       state,
		UserID:      adminUser.ID,
		CreatedAt:   time.Now(),
		ExpiresAt:   time.Now().Add(10 * time.Minute),
	}
	s.store.mu.Unlock()

	dest, err := url.Parse(redirectURI)
	if err != nil {
		writeGHError(w, http.StatusBadRequest, "invalid redirect_uri")
		return
	}
	q := dest.Query()
	q.Set("code", code)
	if state != "" {
		q.Set("state", state)
	}
	dest.RawQuery = q.Encode()
	http.Redirect(w, r, dest.String(), http.StatusFound)
}

// createTokenLocked generates a new token (caller must hold st.mu write lock).
func (st *Store) createTokenLocked(userID int, scopes string) *Token {
	value := generateTokenValue()
	t := &Token{
		Value:     value,
		UserID:    userID,
		Scopes:    scopes,
		CreatedAt: time.Now(),
	}
	st.Tokens[t.Value] = t
	return t
}
