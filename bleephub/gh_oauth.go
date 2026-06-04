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
	// Session login (required before the web-flow authorize step).
	s.mux.HandleFunc("GET /login", s.handleLoginPage)
	s.mux.HandleFunc("POST /login", s.handleLoginPost)
	// OAuth web flow.
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

// handleLoginPage renders a simple login form. Real GitHub also has this page;
// the authorize step redirects here when no session cookie is present.
func (s *Server) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	returnTo := r.URL.Query().Get("return_to")
	page := fmt.Sprintf(`<!DOCTYPE html><html><head><title>Sign in</title></head>
<body style="font-family:system-ui,sans-serif;max-width:340px;margin:48px auto">
<h1>Sign in to bleephub</h1>
<form method="POST" action="/login">
  <input type="hidden" name="return_to" value="%s"/>
  <label>Username<br><input type="text" name="login" autofocus style="width:100%%"/></label><br><br>
  <label>Password<br><input type="password" name="password" style="width:100%%"/></label><br><br>
  <button type="submit" style="padding:8px 16px;background:#2da44e;color:white;border:0;border-radius:6px;font-size:14px;cursor:pointer">Sign in</button>
</form>
</body></html>`,
		html.EscapeString(returnTo),
	)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(page))
}

// handleLoginPost authenticates a user by login name (password not checked —
// sim policy) and sets a _gh_sess session cookie. Mirrors the POST /login
// endpoint on real GitHub / GHES.
func (s *Server) handleLoginPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeGHError(w, http.StatusBadRequest, "Problems parsing form")
		return
	}
	login := r.FormValue("login")
	returnTo := r.FormValue("return_to")

	if login == "" {
		writeGHError(w, http.StatusUnprocessableEntity, "login is required")
		return
	}

	s.store.mu.RLock()
	user := s.store.UsersByLogin[login]
	s.store.mu.RUnlock()

	if user == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`<!DOCTYPE html><html><body><p>Incorrect username or password.</p></body></html>`))
		return
	}

	sessionID := uuid.New().String()
	csrf := uuid.New().String()
	sess := &LoginSession{
		UserID:    user.ID,
		CSRFToken: csrf,
		ExpiresAt: time.Now().Add(time.Hour),
	}
	s.store.mu.Lock()
	s.store.LoginSessions[sessionID] = sess
	s.store.mu.Unlock()

	http.SetCookie(w, &http.Cookie{
		Name:     "_gh_sess",
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  sess.ExpiresAt,
	})

	if returnTo != "" {
		parsed, err := url.Parse(returnTo)
		if err == nil && parsed.Path != "" {
			http.Redirect(w, r, returnTo, http.StatusFound)
			return
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<!DOCTYPE html><html><body><p>Signed in successfully.</p></body></html>`))
}

// handleDeviceCode initiates the device authorization flow.
func (s *Server) handleDeviceCode(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeGHError(w, http.StatusBadRequest, "Problems parsing form")
		return
	}
	scope := r.FormValue("scope")

	s.store.mu.Lock()
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

// handleOAuthAccessToken handles BOTH OAuth flows on the same shared endpoint,
// mirroring real GitHub. The grant is identified by which fields the form carries:
//
//   - device_code → device flow (existing behaviour, auto-approved)
//   - code        → web flow with authorization code grant
//
// Both return `{access_token, token_type, scope}` on success and
// `{error: ...}` on failure (200 OK with an error body, matching real GitHub).
func (s *Server) handleOAuthAccessToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeGHError(w, http.StatusBadRequest, "Problems parsing form")
		return
	}
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

// handleDeviceTokenForm — device-flow leg. Auto-approved (sim policy: device
// codes mint a token on the first poll instead of requiring out-of-band confirmation).
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

// handleWebFlowTokenForm — web-flow leg. Exchanges a one-time-use authorization
// code (issued by /login/oauth/authorize) for an access token. Real GitHub
// validates client_id + client_secret; the sim doesn't gate on the secret but
// does enforce one-time-use.
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
	_, _ = w.Write([]byte(`<!DOCTYPE html><html><body><h1>Auto-approved by bleephub</h1><p>You can close this page.</p></body></html>`))
}

// handleOAuthAuthorize — GET /login/oauth/authorize.
//
// Real GitHub: requires an existing browser session. If no session is present,
// redirects to /login?return_to=<authorize_url>. Once authenticated, renders a
// consent form with an authenticity_token (CSRF) that the POST must echo.
//
// bleephub: same behaviour. Establish a session first via POST /login.
// ?auto=1 is a non-standard bleephub shortcut that skips the form and redirects
// immediately; it uses the session user if a cookie is present, or the seed admin
// otherwise (for test tooling backwards compatibility only — not present on real
// GitHub).
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

	sess := s.sessionFromRequest(r)

	if q.Get("auto") == "1" {
		// Non-standard fast path: use session user if present, seed admin otherwise.
		var user *User
		if sess != nil {
			s.store.mu.RLock()
			user = s.store.Users[sess.UserID]
			s.store.mu.RUnlock()
		}
		if user == nil {
			s.store.mu.RLock()
			user = s.store.Users[1]
			s.store.mu.RUnlock()
		}
		if user == nil {
			writeGHError(w, http.StatusInternalServerError, "no user available")
			return
		}
		s.completeAuthorize(w, r, user, clientID, redirectURI, scopes, state)
		return
	}

	if sess == nil {
		returnTo := r.URL.RequestURI()
		http.Redirect(w, r, "/login?return_to="+url.QueryEscape(returnTo), http.StatusFound)
		return
	}

	s.store.mu.RLock()
	user := s.store.Users[sess.UserID]
	csrf := sess.CSRFToken
	s.store.mu.RUnlock()
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "session user not found")
		return
	}

	page := fmt.Sprintf(`<!DOCTYPE html><html><head><title>Authorize bleephub</title></head>
<body style="font-family:system-ui,sans-serif;max-width:480px;margin:48px auto">
<h1>Authorize app</h1>
<p>Signed in as <strong>%s</strong>. The app <code>%s</code> is requesting access with scopes <code>%s</code>.</p>
<form method="POST" action="/login/oauth/authorize">
  <input type="hidden" name="authenticity_token" value="%s"/>
  <input type="hidden" name="client_id" value="%s"/>
  <input type="hidden" name="redirect_uri" value="%s"/>
  <input type="hidden" name="scope" value="%s"/>
  <input type="hidden" name="state" value="%s"/>
  <button type="submit" style="padding:8px 16px;background:#2da44e;color:white;border:0;border-radius:6px;font-size:14px;cursor:pointer">Authorize</button>
</form>
</body></html>`,
		html.EscapeString(user.Login),
		html.EscapeString(clientID),
		html.EscapeString(scopes),
		html.EscapeString(csrf),
		html.EscapeString(clientID),
		html.EscapeString(redirectURI),
		html.EscapeString(scopes),
		html.EscapeString(state),
	)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(page))
}

// handleOAuthAuthorizeApprove handles the POST that the authorize consent form
// submits. Validates the session cookie and the authenticity_token (CSRF), then
// issues the auth code and 302s to redirect_uri.
func (s *Server) handleOAuthAuthorizeApprove(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeGHError(w, http.StatusBadRequest, "Problems parsing form")
		return
	}

	sess := s.sessionFromRequest(r)
	if sess == nil {
		writeGHError(w, http.StatusUnauthorized, "session required — POST /login first")
		return
	}

	provided := r.FormValue("authenticity_token")
	s.store.mu.RLock()
	expected := sess.CSRFToken
	user := s.store.Users[sess.UserID]
	s.store.mu.RUnlock()

	if provided == "" || provided != expected {
		writeGHError(w, http.StatusUnprocessableEntity, "Invalid authenticity_token")
		return
	}
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "session user not found")
		return
	}

	clientID := r.FormValue("client_id")
	redirectURI := r.FormValue("redirect_uri")
	scopes := r.FormValue("scope")
	state := r.FormValue("state")
	if clientID == "" || redirectURI == "" {
		writeGHError(w, http.StatusBadRequest, "client_id and redirect_uri are required")
		return
	}
	s.completeAuthorize(w, r, user, clientID, redirectURI, scopes, state)
}

// completeAuthorize mints a one-time-use auth code bound to user, stores it,
// and 302s back to redirect_uri with code + state.
func (s *Server) completeAuthorize(w http.ResponseWriter, r *http.Request, user *User, clientID, redirectURI, scopes, state string) {
	s.store.mu.Lock()
	if s.store.AuthCodes == nil {
		s.store.AuthCodes = map[string]*authCode{}
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
		UserID:      user.ID,
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

// sessionFromRequest reads the _gh_sess cookie and returns the corresponding
// LoginSession, or nil if absent / expired / unknown.
func (s *Server) sessionFromRequest(r *http.Request) *LoginSession {
	cookie, err := r.Cookie("_gh_sess")
	if err != nil {
		return nil
	}
	s.store.mu.RLock()
	sess := s.store.LoginSessions[cookie.Value]
	s.store.mu.RUnlock()
	if sess == nil || time.Now().After(sess.ExpiresAt) {
		return nil
	}
	return sess
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
	if st.persist != nil {
		st.persist.MustPut("tokens", t.Value, t)
	}
	return t
}
