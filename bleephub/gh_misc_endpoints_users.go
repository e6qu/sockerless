package bleephub

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// User-scoped extras that live under /users/{username} and /user.

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	users := s.store.ListUsers()
	out := make([]map[string]interface{}, 0, len(users))
	for _, u := range users {
		out = append(out, userToJSON(u))
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, out))
}

func normalizeGitHubLogin(login string) string {
	login = strings.ToLower(strings.TrimSpace(login))
	var b strings.Builder
	lastHyphen := false
	for _, r := range login {
		isAlphaNum := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if isAlphaNum {
			b.WriteRune(r)
			lastHyphen = false
			continue
		}
		if !lastHyphen && b.Len() > 0 {
			b.WriteByte('-')
			lastHyphen = true
		}
	}
	out := strings.Trim(b.String(), "-")
	return out
}

func (s *Server) handleAdminCreateUser(w http.ResponseWriter, r *http.Request) {
	if s.requireSiteAdmin(w, r) == nil {
		return
	}
	var req struct {
		Login     string `json:"login"`
		Email     string `json:"email"`
		Suspended bool   `json:"suspended"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	login := normalizeGitHubLogin(req.Login)
	if login == "" {
		writeGHValidationError(w, "User", "login", "missing_field")
		return
	}
	s.store.mu.Lock()
	if _, exists := s.store.UsersByLogin[login]; exists {
		s.store.mu.Unlock()
		writeGHValidationError(w, "User", "login", "already_exists")
		return
	}
	if req.Email != "" {
		for _, existing := range s.store.Users {
			if strings.EqualFold(existing.Email, req.Email) {
				s.store.mu.Unlock()
				writeGHValidationError(w, "User", "email", "already_exists")
				return
			}
		}
	}
	now := time.Now().UTC()
	u := &User{
		ID:           s.store.NextUser,
		NodeID:       fmt.Sprintf("U_kgDO%08d", s.store.NextUser),
		Login:        login,
		Email:        req.Email,
		AvatarURL:    "",
		Type:         "User",
		SiteAdmin:    false,
		Suspended:    req.Suspended,
		StarredRepos: map[string]bool{},
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	s.store.NextUser++
	s.store.Users[u.ID] = u
	s.store.UsersByLogin[u.Login] = u
	if s.store.persist != nil {
		s.store.persist.MustPut("users", strconv.Itoa(u.ID), u)
	}
	s.store.mu.Unlock()
	writeJSON(w, http.StatusCreated, userToJSON(u))
}

func (s *Server) handleAdminRenameUser(w http.ResponseWriter, r *http.Request) {
	if s.requireSiteAdmin(w, r) == nil {
		return
	}
	username := r.PathValue("username")
	var req struct {
		Login string `json:"login"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	nextLogin := normalizeGitHubLogin(req.Login)
	if nextLogin == "" {
		writeGHValidationError(w, "User", "login", "missing_field")
		return
	}
	s.store.mu.Lock()
	u := s.store.UsersByLogin[username]
	if u == nil {
		s.store.mu.Unlock()
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if existing := s.store.UsersByLogin[nextLogin]; existing != nil && existing.ID != u.ID {
		s.store.mu.Unlock()
		writeGHValidationError(w, "User", "login", "already_exists")
		return
	}
	delete(s.store.UsersByLogin, u.Login)
	u.Login = nextLogin
	u.UpdatedAt = time.Now().UTC()
	s.store.UsersByLogin[u.Login] = u
	if s.store.persist != nil {
		s.store.persist.MustPut("users", strconv.Itoa(u.ID), u)
	}
	s.store.mu.Unlock()
	writeJSON(w, http.StatusAccepted, map[string]string{
		"message": "Job queued to rename user. It may take a few minutes to complete.",
		"url":     fmt.Sprintf("/user/%d", u.ID),
	})
}

func (s *Server) handleAdminDeleteUser(w http.ResponseWriter, r *http.Request) {
	admin := s.requireSiteAdmin(w, r)
	if admin == nil {
		return
	}
	username := r.PathValue("username")
	s.store.mu.Lock()
	u := s.store.UsersByLogin[username]
	if u == nil {
		s.store.mu.Unlock()
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if u.ID == admin.ID {
		s.store.mu.Unlock()
		writeGHError(w, http.StatusForbidden, "You cannot delete your own account.")
		return
	}
	delete(s.store.Users, u.ID)
	delete(s.store.UsersByLogin, u.Login)
	for val, t := range s.store.Tokens {
		if t.UserID == u.ID {
			delete(s.store.Tokens, val)
			if s.store.persist != nil {
				s.store.persist.MustDelete("tokens", val)
			}
		}
	}
	for cookie, sess := range s.store.LoginSessions {
		if sess.UserID == u.ID {
			delete(s.store.LoginSessions, cookie)
		}
	}
	if s.store.persist != nil {
		s.store.persist.MustDelete("users", strconv.Itoa(u.ID))
	}
	s.store.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAdminPromoteUser(w http.ResponseWriter, r *http.Request) {
	s.handleAdminSetUserFlags(w, r, adminUserFlagUpdate{siteAdmin: adminUserBoolPtr(true)})
}

func (s *Server) handleAdminDemoteUser(w http.ResponseWriter, r *http.Request) {
	admin := s.requireSiteAdmin(w, r)
	if admin == nil {
		return
	}
	if admin.Login == r.PathValue("username") {
		writeGHError(w, http.StatusForbidden, "You cannot demote your own account.")
		return
	}
	s.setUserFlags(w, r.PathValue("username"), adminUserFlagUpdate{siteAdmin: adminUserBoolPtr(false)})
}

func (s *Server) handleAdminSuspendUser(w http.ResponseWriter, r *http.Request) {
	admin := s.requireSiteAdmin(w, r)
	if admin == nil {
		return
	}
	if admin.Login == r.PathValue("username") {
		writeGHError(w, http.StatusForbidden, "You cannot suspend your own account.")
		return
	}
	s.setUserFlags(w, r.PathValue("username"), adminUserFlagUpdate{suspended: adminUserBoolPtr(true)})
}

func (s *Server) handleAdminUnsuspendUser(w http.ResponseWriter, r *http.Request) {
	s.handleAdminSetUserFlags(w, r, adminUserFlagUpdate{suspended: adminUserBoolPtr(false)})
}

type adminUserFlagUpdate struct {
	siteAdmin *bool
	suspended *bool
}

func (s *Server) handleAdminSetUserFlags(w http.ResponseWriter, r *http.Request, update adminUserFlagUpdate) {
	if s.requireSiteAdmin(w, r) == nil {
		return
	}
	s.setUserFlags(w, r.PathValue("username"), update)
}

func (s *Server) setUserFlags(w http.ResponseWriter, username string, update adminUserFlagUpdate) {
	s.store.mu.Lock()
	u := s.store.UsersByLogin[username]
	if u == nil {
		s.store.mu.Unlock()
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if update.siteAdmin != nil {
		u.SiteAdmin = *update.siteAdmin
	}
	if update.suspended != nil {
		u.Suspended = *update.suspended
	}
	u.UpdatedAt = time.Now().UTC()
	if s.store.persist != nil {
		s.store.persist.MustPut("users", strconv.Itoa(u.ID), u)
	}
	s.store.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func adminUserBoolPtr(v bool) *bool {
	return &v
}

func (s *Server) handleListUserGists(w http.ResponseWriter, r *http.Request) {
	user := s.store.LookupUserByLogin(r.PathValue("username"))
	if user == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	since := parseSinceTime(r)
	gists := s.store.ListGistsForUser(user.ID, since)
	out := make([]map[string]interface{}, 0, len(gists))
	for _, g := range gists {
		out = append(out, s.gistToJSON(g, r, false))
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, out))
}

func (s *Server) handleListUserEvents(w http.ResponseWriter, r *http.Request) {
	user := s.store.LookupUserByLogin(r.PathValue("username"))
	if user == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	events := s.deriveActivityEvents(s.baseURL(r), s.publicReposByID(), nil)
	own := events[:0]
	for _, ev := range events {
		if ev.actorID == user.ID {
			own = append(own, ev)
		}
	}
	sortActivityEvents(own)
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, activityEventsJSON(own)))
}

func (s *Server) handleListUserEventsPublic(w http.ResponseWriter, r *http.Request) {
	s.handleListUserEvents(w, r)
}

func (s *Server) handleListUserEventsForOrg(w http.ResponseWriter, r *http.Request) {
	user := s.store.LookupUserByLogin(r.PathValue("username"))
	if user == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	org := r.PathValue("org")
	repos := s.publicReposByID()
	for id, repo := range repos {
		if !strings.HasPrefix(repo.FullName, org+"/") {
			delete(repos, id)
		}
	}
	events := s.deriveActivityEvents(s.baseURL(r), repos, nil)
	own := events[:0]
	for _, ev := range events {
		if ev.actorID == user.ID {
			own = append(own, ev)
		}
	}
	sortActivityEvents(own)
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, activityEventsJSON(own)))
}

func (s *Server) handleListUserReceivedEvents(w http.ResponseWriter, r *http.Request) {
	user := s.store.LookupUserByLogin(r.PathValue("username"))
	if user == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	// Received events are other users' activity on the user's own public
	// repositories.
	s.store.mu.RLock()
	repos := map[int]*Repo{}
	for _, repo := range s.store.Repos {
		if repo.OwnerType == "User" && repo.OwnerID == user.ID && !repo.Private {
			repos[repo.ID] = repo
		}
	}
	s.store.mu.RUnlock()
	events := s.deriveActivityEvents(s.baseURL(r), repos, nil)
	received := events[:0]
	for _, ev := range events {
		if ev.actorID != user.ID {
			received = append(received, ev)
		}
	}
	sortActivityEvents(received)
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, activityEventsJSON(received)))
}

func (s *Server) handleListUserReceivedEventsPublic(w http.ResponseWriter, r *http.Request) {
	s.handleListUserReceivedEvents(w, r)
}

func (s *Server) handleCheckUserFollowing(w http.ResponseWriter, r *http.Request) {
	user := s.store.LookupUserByLogin(r.PathValue("username"))
	if user == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	target := r.PathValue("target_user")
	s.store.Misc.mu.RLock()
	following := s.store.Misc.follows[user.Login] != nil && s.store.Misc.follows[user.Login][target]
	s.store.Misc.mu.RUnlock()
	if following {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeGHError(w, http.StatusNotFound, "Not Found")
}

func (s *Server) handleListUserSocialAccounts(w http.ResponseWriter, r *http.Request) {
	user := s.store.LookupUserByLogin(r.PathValue("username"))
	if user == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, s.store.ListUserSocialAccounts(user.ID))
}

func (s *Server) handleListUserSSHSigningKeys(w http.ResponseWriter, r *http.Request) {
	user := s.store.LookupUserByLogin(r.PathValue("username"))
	if user == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, s.store.ListUserSSHSigningKeys(user.ID))
}

func (s *Server) handleListUserSubscriptions(w http.ResponseWriter, r *http.Request) {
	user := s.store.LookupUserByLogin(r.PathValue("username"))
	if user == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	repos := s.store.ListRepoSubscriptionsForUser(user.ID)
	base := s.baseURL(r)
	out := make([]map[string]interface{}, 0, len(repos))
	for _, repo := range repos {
		out = append(out, repoToJSON(repo, s.store, base))
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, out))
}

func (s *Server) handleListUserBlocks(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	logins := s.store.ListBlockedUsers(user.ID)
	out := make([]map[string]interface{}, 0, len(logins))
	for _, login := range logins {
		if u := s.store.LookupUserByLogin(login); u != nil {
			out = append(out, userToJSON(u))
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCheckUserBlocked(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	target := s.store.LookupUserByLogin(r.PathValue("username"))
	if target == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if s.store.IsUserBlocked(user.ID, target.ID) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeGHError(w, http.StatusNotFound, "Not Found")
}

func (s *Server) handleBlockUser(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	target := s.store.LookupUserByLogin(r.PathValue("username"))
	if target == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.store.BlockUser(user.ID, target.ID)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleUnblockUser(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	target := s.store.LookupUserByLogin(r.PathValue("username"))
	if target == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.store.UnblockUser(user.ID, target.ID)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleCheckMyFollowing(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	target := r.PathValue("username")
	s.store.Misc.mu.RLock()
	following := s.store.Misc.follows[user.Login] != nil && s.store.Misc.follows[user.Login][target]
	s.store.Misc.mu.RUnlock()
	if following {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeGHError(w, http.StatusNotFound, "Not Found")
}

func (s *Server) handleListMySocialAccounts(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	writeJSON(w, http.StatusOK, s.store.ListUserSocialAccounts(user.ID))
}

func (s *Server) handleCreateMySocialAccounts(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeGHValidationError(w, "accounts", "accounts", "missing_field")
		return
	}
	var urls []string
	if err := json.Unmarshal(body, &urls); err == nil && len(urls) > 0 {
		s.store.SetUserSocialAccounts(user.ID, urls)
		writeJSON(w, http.StatusOK, s.store.ListUserSocialAccounts(user.ID))
		return
	}
	var objects []map[string]interface{}
	if err := json.Unmarshal(body, &objects); err != nil {
		writeGHValidationError(w, "accounts", "accounts", "missing_field")
		return
	}
	urls = nil
	for _, o := range objects {
		if u, ok := o["url"].(string); ok {
			urls = append(urls, u)
		}
	}
	s.store.SetUserSocialAccounts(user.ID, urls)
	writeJSON(w, http.StatusOK, s.store.ListUserSocialAccounts(user.ID))
}

func (s *Server) handleDeleteMySocialAccounts(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		s.store.SetUserSocialAccounts(user.ID, nil)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	var urls []string
	if err := json.Unmarshal(body, &urls); err == nil && len(urls) > 0 {
		// fall through to filter
	} else {
		var req struct {
			AccountUrls []string `json:"account_urls"`
		}
		if err := json.Unmarshal(body, &req); err == nil {
			urls = req.AccountUrls
		}
	}
	if len(urls) == 0 {
		s.store.SetUserSocialAccounts(user.ID, nil)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	delSet := make(map[string]bool, len(urls))
	for _, u := range urls {
		delSet[u] = true
	}
	existing := s.store.ListUserSocialAccounts(user.ID)
	kept := make([]string, 0, len(existing))
	for _, acct := range existing {
		if url, _ := acct["url"].(string); url != "" && !delSet[url] {
			kept = append(kept, url)
		}
	}
	s.store.SetUserSocialAccounts(user.ID, kept)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListMySSHSigningKeys(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	writeJSON(w, http.StatusOK, s.store.ListUserSSHSigningKeys(user.ID))
}

func (s *Server) handleCreateMySSHSigningKey(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	var req struct {
		Key   string `json:"key"`
		Title string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Key == "" {
		writeGHValidationError(w, "Key", "key", "missing_field")
		return
	}
	entry := s.store.AddUserSSHSigningKey(user.ID, req.Key)
	if title, ok := entry["title"].(string); ok && title == "" && req.Title != "" {
		entry["title"] = req.Title
	}
	writeJSON(w, http.StatusCreated, entry)
}

func (s *Server) handleDeleteMySSHSigningKey(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	id, _ := strconv.Atoi(r.PathValue("ssh_signing_key_id"))
	if !s.store.DeleteUserSSHSigningKey(user.ID, id) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleCheckMyStarredRepo(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	owner := r.PathValue("owner")
	repo := r.PathValue("repo")
	if s.store.IsRepoStarredBy(user.ID, owner, repo) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeGHError(w, http.StatusNotFound, "Not Found")
}

func (s *Server) handleListMySubscriptions(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	repos := s.store.ListRepoSubscriptionsForUser(user.ID)
	base := s.baseURL(r)
	out := make([]map[string]interface{}, 0, len(repos))
	for _, repo := range repos {
		out = append(out, repoToJSON(repo, s.store, base))
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, out))
}

func parseSinceTime(r *http.Request) time.Time {
	v := r.URL.Query().Get("since")
	if v == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return time.Time{}
	}
	return t
}
