package bleephub

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// writeOAuthTokenResponse renders a POST /login/oauth/access_token response in
// the format the client negotiated via Accept, matching real GitHub: JSON when
// the client sends `Accept: application/json`, otherwise
// application/x-www-form-urlencoded (GitHub's default for this endpoint).
// Applies to both success and error bodies, and to both the web and device
// flows that share the endpoint.
func writeOAuthTokenResponse(w http.ResponseWriter, r *http.Request, fields map[string]string) {
	if strings.Contains(r.Header.Get("Accept"), "application/json") {
		obj := make(map[string]any, len(fields))
		for k, v := range fields {
			obj[k] = v
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(obj)
		return
	}
	form := url.Values{}
	for k, v := range fields {
		form.Set(k, v)
	}
	w.Header().Set("Content-Type", "application/x-www-form-urlencoded; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(form.Encode()))
}

func (s *Server) registerGHOAuthRoutes() {
	s.route("POST /login/device/code", s.handleDeviceCode)
	s.route("POST /login/oauth/access_token", s.handleOAuthAccessToken)
	s.route("GET /login/device", s.handleDevicePage)
	s.route("POST /login/device", s.handleDeviceApprove)
	// Session login (required before the web-flow authorize step).
	s.route("GET /login", s.handleLoginPage)
	s.route("POST /login", s.handleLoginPost)
	// OAuth web flow.
	s.route("GET /login/oauth/authorize", s.handleOAuthAuthorize)
	s.route("POST /login/oauth/authorize", s.handleOAuthAuthorizeApprove)
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
  <label>Personal access token<br><input type="password" name="password" style="width:100%%"/></label><br><br>
  <button type="submit" style="padding:8px 16px;background:#2da44e;color:white;border:0;border-radius:6px;font-size:14px;cursor:pointer">Sign in</button>
</form>
</body></html>`,
		html.EscapeString(returnTo),
	)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(page))
}

// handleLoginPost authenticates a user with a stored personal access token and
// sets a _gh_sess session cookie. Bleephub does not implement password
// verification (`/api/v3/meta` advertises that), so browser sessions must be
// backed by the same real credential source as API requests.
func (s *Server) handleLoginPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeGHError(w, http.StatusBadRequest, "Problems parsing form")
		return
	}
	login := r.FormValue("login")
	credential := r.FormValue("password")
	returnTo := r.FormValue("return_to")

	if login == "" {
		writeGHError(w, http.StatusUnprocessableEntity, "login is required")
		return
	}
	if credential == "" {
		writeGHError(w, http.StatusUnprocessableEntity, "personal access token is required")
		return
	}

	user := s.browserLoginUser(login, credential)
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

func (s *Server) browserLoginUser(login, credential string) *User {
	_, user := s.store.LookupToken(credential)
	if user != nil && user.Login == login && !user.Suspended {
		return user
	}
	user = s.store.LookupUserByLogin(login)
	if user == nil || user.Suspended || user.PasswordHash == "" || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(credential)) != nil {
		return nil
	}
	return user
}

// oauthClientKind resolves a registered OAuth client_id to the token-minting
// parameters CreateUserToServerToken expects: (appID, oauthClientID). A GitHub
// App client_id yields a non-zero appID (mints ghu_); a registered OAuth App
// client_id yields oauthClientID (mints gho_). Unknown client IDs are invalid.
func (s *Server) oauthClientKind(clientID string) (appID int, oauthClientID string, isGitHubApp bool, ok bool) {
	if clientID != "" {
		if app := s.store.GetAppByClientID(clientID); app != nil {
			return app.ID, "", true, true
		}
		if app := s.store.GetOAuthApp(clientID); app != nil {
			return 0, app.ClientID, false, true
		}
	}
	return 0, "", false, false
}

func (s *Server) verifyOAuthClientSecret(clientID, clientSecret string) (appID int, oauthClientID string, isGitHubApp bool, ok bool) {
	if clientID == "" || clientSecret == "" {
		return 0, "", false, false
	}
	if app := s.store.VerifyAppClientSecret(clientID, clientSecret); app != nil {
		return app.ID, "", true, true
	}
	if app := s.store.VerifyOAuthAppSecret(clientID, clientSecret); app != nil {
		return 0, app.ClientID, false, true
	}
	return 0, "", false, false
}

func newDeviceUserCode() string {
	raw := strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", ""))
	return raw[:4] + "-" + raw[4:8]
}

func normalizeDeviceUserCode(code string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(code), "-", ""))
}

// handleDeviceCode initiates the device authorization flow.
func (s *Server) handleDeviceCode(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeGHError(w, http.StatusBadRequest, "Problems parsing form")
		return
	}
	scope := r.FormValue("scope")
	clientID := r.FormValue("client_id")

	appID, oauthClientID, _, ok := s.oauthClientKind(clientID)
	if !ok {
		writeOAuthTokenResponse(w, r, map[string]string{"error": "incorrect_client_credentials"})
		return
	}

	s.store.mu.Lock()
	dc := &DeviceCode{
		Code:          uuid.New().String(),
		UserCode:      newDeviceUserCode(),
		ClientID:      clientID,
		Scopes:        scope,
		AppID:         appID,
		OAuthClientID: oauthClientID,
		ExpiresAt:     time.Now().Add(15 * time.Minute),
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
//   - device_code → device flow
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

// handleDeviceTokenForm — device-flow polling leg.
func (s *Server) handleDeviceTokenForm(w http.ResponseWriter, r *http.Request) {
	deviceCode := r.FormValue("device_code")
	clientID := r.FormValue("client_id")
	s.store.mu.Lock()
	dc, ok := s.store.DeviceCodes[deviceCode]

	if !ok {
		s.store.mu.Unlock()
		writeOAuthTokenResponse(w, r, map[string]string{"error": "bad_verification_code"})
		return
	}
	if clientID == "" || clientID != dc.ClientID {
		s.store.mu.Unlock()
		writeOAuthTokenResponse(w, r, map[string]string{"error": "incorrect_client_credentials"})
		return
	}
	if time.Now().After(dc.ExpiresAt) {
		delete(s.store.DeviceCodes, deviceCode)
		s.store.mu.Unlock()
		writeOAuthTokenResponse(w, r, map[string]string{"error": "expired_token"})
		return
	}
	if dc.Token == "" {
		s.store.mu.Unlock()
		writeOAuthTokenResponse(w, r, map[string]string{"error": "authorization_pending"})
		return
	}
	token := dc.Token
	scopes := dc.Scopes
	delete(s.store.DeviceCodes, deviceCode)
	s.store.mu.Unlock()

	s.logger.Info().Str("device_code", deviceCode).Msg("device token granted")

	writeOAuthTokenResponse(w, r, map[string]string{
		"access_token": token,
		"token_type":   "bearer",
		"scope":        scopes,
	})
}

// handleWebFlowTokenForm — web-flow leg. Exchanges a one-time-use authorization
// code (issued by /login/oauth/authorize) for an access token. GitHub requires
// the registered OAuth App or GitHub App client_id + client_secret here.
func (s *Server) handleWebFlowTokenForm(w http.ResponseWriter, r *http.Request) {
	code := r.FormValue("code")
	clientID := r.FormValue("client_id")
	clientSecret := r.FormValue("client_secret")
	redirectURI := r.FormValue("redirect_uri")

	s.store.mu.Lock()
	ac, ok := s.store.AuthCodes[code]
	if ok {
		delete(s.store.AuthCodes, code)
	}
	s.store.mu.Unlock()

	if !ok || time.Now().After(ac.ExpiresAt) {
		writeOAuthTokenResponse(w, r, map[string]string{"error": "bad_verification_code"})
		return
	}
	if clientID == "" || ac.ClientID == "" || clientID != ac.ClientID {
		writeOAuthTokenResponse(w, r, map[string]string{"error": "incorrect_client_credentials"})
		return
	}
	if redirectURI != "" && ac.RedirectURI != "" && redirectURI != ac.RedirectURI {
		writeOAuthTokenResponse(w, r, map[string]string{"error": "redirect_uri_mismatch"})
		return
	}
	appID, oauthClientID, _, ok := s.verifyOAuthClientSecret(clientID, clientSecret)
	if !ok {
		writeOAuthTokenResponse(w, r, map[string]string{"error": "incorrect_client_credentials"})
		return
	}

	s.store.mu.Lock()
	user := s.store.Users[ac.UserID]
	if user == nil {
		s.store.mu.Unlock()
		writeOAuthTokenResponse(w, r, map[string]string{"error": "server_error"})
		return
	}
	tok, _, err := s.store.createUserToServerTokenLocked(user.ID, appID, oauthClientID, ac.Scopes, 8*time.Hour, false)
	if err != nil {
		s.store.mu.Unlock()
		writeOAuthTokenResponse(w, r, map[string]string{"error": "server_error"})
		return
	}
	s.store.mu.Unlock()

	s.logger.Info().Str("auth_code", code).Int("user_id", user.ID).Msg("web flow token granted")
	writeOAuthTokenResponse(w, r, map[string]string{
		"access_token": tok.Token,
		"token_type":   "bearer",
		"scope":        ac.Scopes,
	})
}

// handleDevicePage renders the browser confirmation form for a device code.
func (s *Server) handleDevicePage(w http.ResponseWriter, r *http.Request) {
	sess := s.sessionFromRequest(r)
	if sess == nil {
		http.Redirect(w, r, "/login?return_to="+url.QueryEscape(r.URL.RequestURI()), http.StatusFound)
		return
	}
	userCode := r.URL.Query().Get("user_code")
	page := fmt.Sprintf(`<!DOCTYPE html><html><head><title>Device activation</title></head>
<body style="font-family:system-ui,sans-serif;max-width:420px;margin:48px auto">
<h1>Device activation</h1>
<form method="POST" action="/login/device">
  <label>Code<br><input type="text" name="user_code" value="%s" autofocus style="width:100%%;text-transform:uppercase"/></label><br><br>
  <button type="submit" style="padding:8px 16px;background:#2da44e;color:white;border:0;border-radius:6px;font-size:14px;cursor:pointer">Authorize</button>
</form>
</body></html>`, html.EscapeString(userCode))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(page))
}

// handleDeviceApprove binds a pending device code to the signed-in browser user.
func (s *Server) handleDeviceApprove(w http.ResponseWriter, r *http.Request) {
	sess := s.sessionFromRequest(r)
	if sess == nil {
		http.Redirect(w, r, "/login?return_to="+url.QueryEscape(r.URL.RequestURI()), http.StatusFound)
		return
	}
	if err := r.ParseForm(); err != nil {
		writeGHError(w, http.StatusBadRequest, "Problems parsing form")
		return
	}
	userCode := normalizeDeviceUserCode(r.FormValue("user_code"))
	if userCode == "" {
		writeGHError(w, http.StatusUnprocessableEntity, "user_code is required")
		return
	}

	s.store.mu.Lock()
	var dc *DeviceCode
	for _, candidate := range s.store.DeviceCodes {
		if normalizeDeviceUserCode(candidate.UserCode) == userCode {
			dc = candidate
			break
		}
	}
	if dc == nil {
		s.store.mu.Unlock()
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if time.Now().After(dc.ExpiresAt) {
		delete(s.store.DeviceCodes, dc.Code)
		s.store.mu.Unlock()
		writeGHError(w, http.StatusGone, "device code expired")
		return
	}
	user := s.store.Users[sess.UserID]
	if user == nil {
		s.store.mu.Unlock()
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	tok, _, err := s.store.createUserToServerTokenLocked(user.ID, dc.AppID, dc.OAuthClientID, dc.Scopes, 8*time.Hour, false)
	if err != nil {
		s.store.mu.Unlock()
		writeGHError(w, http.StatusInternalServerError, err.Error())
		return
	}
	dc.Token = tok.Token
	dc.UserID = user.ID
	dc.ApprovedAt = time.Now().UTC()
	s.store.mu.Unlock()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<!DOCTYPE html><html><body><p>Device authorized.</p></body></html>`))
}

// handleOAuthAuthorize — GET /login/oauth/authorize.
//
// Real GitHub: requires an existing browser session. If no session is present,
// redirects to /login?return_to=<authorize_url>. Once authenticated, renders a
// consent form with an authenticity_token (CSRF) that the POST must echo.
//
// bleephub: same behaviour. Establish a session first via POST /login.
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

	if sess == nil {
		returnTo := r.URL.RequestURI()
		http.Redirect(w, r, "/login?return_to="+url.QueryEscape(returnTo), http.StatusFound)
		return
	}
	if _, _, _, ok := s.oauthClientKind(clientID); !ok {
		writeGHError(w, http.StatusBadRequest, "incorrect_client_credentials")
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
	if _, _, _, ok := s.oauthClientKind(clientID); !ok {
		writeGHError(w, http.StatusBadRequest, "incorrect_client_credentials")
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
	var user *User
	if sess != nil {
		user = s.store.Users[sess.UserID]
	}
	s.store.mu.RUnlock()
	if sess == nil || user == nil || user.Suspended || time.Now().After(sess.ExpiresAt) {
		return nil
	}
	return sess
}

// createTokenLocked generates a new token (caller must hold st.mu write lock).
func (st *Store) createTokenLocked(userID int, scopes string) *Token {
	value, err := generateTokenValue()
	if err != nil {
		panic(err)
	}
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
