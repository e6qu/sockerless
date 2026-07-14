package bleephub

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	githubAdminTeam     = "e6qu-org-admins"
	githubDeveloperTeam = "e6qu-org-members"
)

type identityConfig struct {
	githubClientID     string
	githubClientSecret string
}

type identityState struct {
	provider  string
	returnTo  string
	expiresAt time.Time
}

func identityConfigFromEnv() identityConfig {
	return identityConfig{
		githubClientID:     strings.TrimSpace(os.Getenv("BLEEPHUB_GITHUB_OAUTH_CLIENT_ID")),
		githubClientSecret: strings.TrimSpace(os.Getenv("BLEEPHUB_GITHUB_OAUTH_CLIENT_SECRET")),
	}
}

func (s *Server) registerExternalIdentityRoutes() {
	s.route("GET /auth/providers", s.handleIdentityProviders)
	s.route("GET /auth/session", s.handleIdentitySession)
	s.route("GET /auth/github", s.handleGitHubLogin)
	s.route("GET /auth/github/callback", s.handleGitHubCallback)
	s.route("POST /auth/local", s.handleLocalLogin)
	s.route("POST /auth/logout", s.handleIdentityLogout)
	s.route("GET /control", s.handlePrivateControl)
}

func (s *Server) handleIdentityProviders(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{
		"github": s.identity.githubClientID != "" && s.identity.githubClientSecret != "",
		"entra":  false,
		"local":  true,
	})
}

func (s *Server) handleIdentitySession(w http.ResponseWriter, r *http.Request) {
	session := s.sessionFromRequest(r)
	if session == nil {
		writeGHError(w, http.StatusUnauthorized, "browser session is required")
		return
	}
	user := s.store.GetUserByID(session.UserID)
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "browser session user is unavailable")
		return
	}
	writeJSON(w, http.StatusOK, s.fullUserJSON(user))
}

func (s *Server) handleGitHubLogin(w http.ResponseWriter, r *http.Request) {
	if s.identity.githubClientID == "" || s.identity.githubClientSecret == "" {
		writeGHError(w, http.StatusServiceUnavailable, "GitHub sign-in is not configured")
		return
	}
	state, err := randomIdentityState()
	if err != nil {
		writeGHError(w, http.StatusInternalServerError, "could not start GitHub sign-in")
		return
	}
	returnTo := r.URL.Query().Get("return_to")
	if !strings.HasPrefix(returnTo, "/") || strings.HasPrefix(returnTo, "//") {
		returnTo = "/ui/"
	}
	s.identityStatesMu.Lock()
	s.identityStates[state] = identityState{provider: "github", returnTo: returnTo, expiresAt: time.Now().Add(10 * time.Minute)}
	s.identityStatesMu.Unlock()

	redirect := url.URL{Scheme: "https", Host: "github.com", Path: "/login/oauth/authorize"}
	q := redirect.Query()
	q.Set("client_id", s.identity.githubClientID)
	q.Set("redirect_uri", s.externalAuthCallback("github"))
	q.Set("scope", "read:org user:email")
	q.Set("state", state)
	redirect.RawQuery = q.Encode()
	http.Redirect(w, r, redirect.String(), http.StatusFound)
}

func (s *Server) handleGitHubCallback(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	s.identityStatesMu.Lock()
	pending, ok := s.identityStates[state]
	delete(s.identityStates, state)
	s.identityStatesMu.Unlock()
	if !ok || pending.provider != "github" || time.Now().After(pending.expiresAt) || r.URL.Query().Get("code") == "" {
		writeGHError(w, http.StatusBadRequest, "invalid or expired GitHub sign-in state")
		return
	}
	accessToken, err := s.exchangeGitHubCode(r.URL.Query().Get("code"))
	if err != nil {
		s.logger.Warn().Err(err).Msg("GitHub OAuth callback failed")
		writeGHError(w, http.StatusUnauthorized, "GitHub sign-in failed")
		return
	}
	profile, err := githubProfile(accessToken)
	if err != nil {
		writeGHError(w, http.StatusUnauthorized, "GitHub account lookup failed")
		return
	}
	admin, developer, err := githubTeamRoles(accessToken, profile.Login)
	if err != nil {
		writeGHError(w, http.StatusForbidden, "GitHub team membership could not be verified")
		return
	}
	if !admin && !developer {
		writeGHError(w, http.StatusForbidden, "GitHub account is not an e6qu-org Bleephub member")
		return
	}
	if profile.Email == "" {
		profile.Email, err = githubPrimaryEmail(accessToken)
		if err != nil {
			writeGHError(w, http.StatusUnauthorized, "GitHub email lookup failed")
			return
		}
	}
	user := s.upsertExternalUser(profile.Login, profile.Name, profile.Email, profile.AvatarURL, admin)
	s.createBrowserSession(w, r, user)
	http.Redirect(w, r, pending.returnTo, http.StatusFound)
}

type githubIdentity struct{ Login, Name, Email, AvatarURL string }

func (s *Server) exchangeGitHubCode(code string) (string, error) {
	form := url.Values{"client_id": {s.identity.githubClientID}, "client_secret": {s.identity.githubClientSecret}, "code": {code}, "redirect_uri": {s.externalAuthCallback("github")}}
	req, err := http.NewRequest(http.MethodPost, "https://github.com/login/oauth/access_token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := &http.Client{Timeout: 10 * time.Second}
	response, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	var body struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		return "", err
	}
	if response.StatusCode != http.StatusOK || body.AccessToken == "" {
		return "", fmt.Errorf("GitHub token exchange: %s", body.Error)
	}
	return body.AccessToken, nil
}

func githubProfile(token string) (githubIdentity, error) {
	req, _ := http.NewRequest(http.MethodGet, "https://api.github.com/user", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return githubIdentity{}, err
	}
	defer response.Body.Close()
	var profile struct {
		Login     string `json:"login"`
		Name      string `json:"name"`
		Email     string `json:"email"`
		AvatarURL string `json:"avatar_url"`
	}
	if err := json.NewDecoder(response.Body).Decode(&profile); err != nil {
		return githubIdentity{}, err
	}
	if response.StatusCode != http.StatusOK || profile.Login == "" {
		return githubIdentity{}, fmt.Errorf("GitHub user lookup status %d", response.StatusCode)
	}
	return githubIdentity{profile.Login, profile.Name, profile.Email, profile.AvatarURL}, nil
}

func githubPrimaryEmail(token string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, "https://api.github.com/user/emails", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := json.NewDecoder(response.Body).Decode(&emails); err != nil {
		return "", err
	}
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub email lookup status %d", response.StatusCode)
	}
	for _, email := range emails {
		if email.Primary && email.Verified && email.Email != "" {
			return email.Email, nil
		}
	}
	return "", fmt.Errorf("GitHub account has no verified primary email")
}

func githubTeamRoles(token, login string) (admin, developer bool, err error) {
	for _, team := range []struct {
		slug  string
		admin bool
	}{{githubAdminTeam, true}, {githubDeveloperTeam, false}} {
		req, _ := http.NewRequest(http.MethodGet, "https://api.github.com/orgs/e6qu-org/teams/"+team.slug+"/memberships/"+url.PathEscape(login), nil)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/vnd.github+json")
		response, requestErr := (&http.Client{Timeout: 10 * time.Second}).Do(req)
		if requestErr != nil {
			return false, false, requestErr
		}
		if response.StatusCode == http.StatusNoContent || response.StatusCode == http.StatusOK {
			if team.admin {
				admin = true
			} else {
				developer = true
			}
		} else if response.StatusCode != http.StatusNotFound {
			response.Body.Close()
			return false, false, fmt.Errorf("GitHub team membership lookup for %s status %d", team.slug, response.StatusCode)
		}
		response.Body.Close()
	}
	return admin, developer, nil
}

func (s *Server) handleLocalLogin(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Login    string `json:"login"`
		Password string `json:"password"`
	}
	if !decodeJSONBody(w, r, &request) {
		return
	}
	user := s.browserLoginUser(strings.TrimSpace(request.Login), request.Password)
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "invalid local credentials")
		return
	}
	s.createBrowserSession(w, r, user)
	writeJSON(w, http.StatusOK, s.fullUserJSON(user))
}

func (s *Server) upsertExternalUser(login, name, email, avatarURL string, siteAdmin bool) *User {
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	if user := s.store.UsersByLogin[login]; user != nil {
		user.Name, user.Email, user.AvatarURL, user.SiteAdmin, user.UpdatedAt = name, email, avatarURL, siteAdmin, time.Now().UTC()
		if s.store.persist != nil {
			s.store.persist.MustPut("users", fmt.Sprint(user.ID), user)
		}
		return user
	}
	now := time.Now().UTC()
	user := &User{ID: s.store.NextUser, NodeID: "U_bleephub_" + login, Login: login, Name: name, Email: email, AvatarURL: avatarURL, Type: "User", SiteAdmin: siteAdmin, StarredRepos: map[string]bool{}, CreatedAt: now, UpdatedAt: now}
	s.store.NextUser++
	s.store.Users[user.ID], s.store.UsersByLogin[user.Login] = user, user
	if s.store.persist != nil {
		s.store.persist.MustPut("users", fmt.Sprint(user.ID), user)
	}
	return user
}

func (s *Server) createBrowserSession(w http.ResponseWriter, r *http.Request, user *User) {
	id, _ := randomIdentityState()
	s.store.mu.Lock()
	s.store.LoginSessions[id] = &LoginSession{UserID: user.ID, CSRFToken: id, ExpiresAt: time.Now().Add(12 * time.Hour)}
	s.store.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: "_gh_sess", Value: id, Path: "/", HttpOnly: true, Secure: strings.HasPrefix(s.externalURL, "https://"), SameSite: http.SameSiteLaxMode, Expires: time.Now().Add(12 * time.Hour)})
}

func (s *Server) externalAuthCallback(provider string) string {
	return strings.TrimRight(s.externalURL, "/") + "/auth/" + provider + "/callback"
}
func (s *Server) handleIdentityLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie("_gh_sess"); err == nil {
		s.store.mu.Lock()
		delete(s.store.LoginSessions, cookie.Value)
		s.store.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{Name: "_gh_sess", Value: "", Path: "/", MaxAge: -1, HttpOnly: true})
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) handlePrivateControl(w http.ResponseWriter, r *http.Request) {
	if adminHost := strings.TrimSpace(os.Getenv("BLEEPHUB_ADMIN_HOST")); adminHost != "" && !strings.EqualFold(strings.Split(r.Host, ":")[0], adminHost) {
		http.NotFound(w, r)
		return
	}
	session := s.sessionFromRequest(r)
	if session == nil || !s.store.GetUserByID(session.UserID).SiteAdmin {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<!doctype html><html><head><meta name="viewport" content="width=device-width,initial-scale=1"><title>Bleephub control</title><style>body{margin:0;background:#111827;color:#f8fafc;font:16px system-ui}main{max-width:960px;margin:3rem auto;padding:2rem;background:#1e293b;border:1px solid #334155;border-radius:18px}h1{color:#67e8f9}table{width:100%;border-collapse:collapse}td,th{padding:.7rem;border-bottom:1px solid #334155;text-align:left}form{display:grid;grid-template-columns:1fr 1fr 1fr 1fr auto;gap:.6rem;margin:1.5rem 0}input,select,button{padding:.7rem;border-radius:8px;border:1px solid #475569}button{background:#a855f7;color:white;font-weight:700;cursor:pointer}.stats{display:grid;grid-template-columns:repeat(auto-fit,minmax(150px,1fr));gap:.7rem}.stat{padding:1rem;background:#0f172a;border:1px solid #334155;border-radius:10px}.stat b{display:block;color:#67e8f9;font-size:1.3rem}</style></head><body><main><h1>Bleephub private control</h1><p>Instance-only identity, storage, and runtime administration. This surface is intentionally separate from GitHub-compatible routes.</p><section><h2>Live service monitoring</h2><div class="stats" id="stats"><div class="stat">Loading…</div></div></section><form id="new-user"><input name="login" placeholder="login" required><input name="email" type="email" placeholder="email" required><input name="password" type="password" placeholder="temporary password" required><select name="role"><option value="developer">developer</option><option value="admin">admin</option></select><button>Create local user</button></form><table><thead><tr><th>Login</th><th>Email</th><th>Role</th></tr></thead><tbody id="users"></tbody></table></main><script>const users=document.querySelector('#users'),stats=document.querySelector('#stats');const esc=v=>String(v??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));async function load(){const [u,m,s,t]=await Promise.all([fetch('/internal/users'),fetch('/internal/metrics'),fetch('/internal/status'),fetch('/internal/storage')]);if(u.ok)users.innerHTML=(await u.json()).map(x=>'<tr><td>@'+esc(x.login)+'</td><td>'+esc(x.email)+'</td><td>'+ (x.site_admin?'admin':'developer')+'</td></tr>').join('');if(m.ok&&s.ok&&t.ok){const metrics=await m.json(),status=await s.json(),storage=await t.json();stats.innerHTML='<div class="stat">Workflows<b>'+esc(status.active_workflows)+'</b></div><div class="stat">Connected runners<b>'+esc(status.connected_runners)+'</b></div><div class="stat">Uptime<b>'+esc(status.uptime_seconds)+'s</b></div><div class="stat">Git storage<b>'+esc(storage.git)+'</b></div><div class="stat">Persistence<b>'+esc(storage.persistence)+'</b></div><div class="stat">Active sessions<b>'+esc(metrics.active_sessions)+'</b></div>'}}document.querySelector('#new-user').onsubmit=async e=>{e.preventDefault();const f=new FormData(e.target);const body=Object.fromEntries(f);body.site_admin=body.role==='admin';const r=await fetch('/internal/users',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)});if(!r.ok){alert('Could not create local user');return}e.target.reset();load()};load();setInterval(load,15000)</script></body></html>`))
}

func randomIdentityState() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}
