package bleephub

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

// permission enforcement decorator.
// `requirePerm` wraps an http.HandlerFunc, returning 403 if the request's
// auth shape lacks the required permission/level.
//
// Authentication shapes handled:
//
//   - Classic PAT (Tokens map, no installation context)
//     Classic PATs retain their broad scope behavior.
//   - Fine-grained PAT (Tokens map + ctxPersonalAccessToken)
//     Checked against approval, expiration, resource owner, selected
//     repositories, and the requested permission map.
//   - GitHub App JWT (ghAppFromContext)
//     App auth is meta-level (manages installations); bypass for app-meta
//     endpoints which use this gate at most for read; gate by caller.
//   - Installation token (ghs_, ctxInstallation + ctxInstallationToken)
//     Checked against InstallationToken.Permissions[scope] >= required level.
//   - User-to-server (ghu_/gho_, ctxUserToServerToken)
//     For ghu_ (AppID > 0): looked up via Installation.Permissions of the
//     installation tied to the user's authorization for this app.
//     For gho_ (OAuthAppClientID set): mapped from classic Scopes string
//     ("repo" → contents:write, "read:org" → members:read, etc.).
//
// Level ordering: read < write < admin. "admin" implies write; "write" implies read.

type permLevel int

const (
	permRead permLevel = iota
	permWrite
	permAdmin
)

// permScope is a GitHub fine-grained permission name. The values are the
// exact keys used in an installation token's Permissions map and in the
// App API, so they must not change — but call sites reference the named
// constants, making a mistyped scope a compile error rather than a silent
// always-deny gate.
type permScope string

const (
	scopeMetadata          permScope = "metadata"
	scopeContents          permScope = "contents"
	scopeIssues            permScope = "issues"
	scopePullRequests      permScope = "pull_requests"
	scopeActions           permScope = "actions"
	scopeChecks            permScope = "checks"
	scopeSecrets           permScope = "secrets"
	scopeDeployments       permScope = "deployments"
	scopeAdministration    permScope = "administration"
	scopeMembers           permScope = "members"
	scopeOrgAdministration permScope = "organization_administration"
	scopeSecurityEvents    permScope = "security_events"
	scopeDependabotSecrets permScope = "dependabot_secrets"
	scopeCodespaces        permScope = "codespaces"
	scopeReactions         permScope = "reactions"
	scopeProjects          permScope = "projects"
	scopePages             permScope = "pages"
	scopePATRequests       permScope = "organization_personal_access_token_requests"
	scopePATs              permScope = "organization_personal_access_tokens"
)

func parsePermLevel(s string) permLevel {
	switch strings.ToLower(s) {
	case "admin":
		return permAdmin
	case "write":
		return permWrite
	case "read", "":
		return permRead
	}
	return permRead
}

// requirePerm returns a wrapper that enforces (scope, level) on the request's auth.
//
// Usage:
//
//	s.route("PATCH /api/v3/repos/{owner}/{repo}", s.requirePerm(scopeContents, permWrite, s.handleUpdateRepo))
func (s *Server) requirePerm(scope permScope, level permLevel, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Personal access token path. Fine-grained credentials are checked
		// below; classic credentials retain their broad scope behavior.
		// Detected by: user present, no installation token, no user-to-server token,
		// no JWT-app. The PAT itself sits in the auth header; middleware already
		// resolved it into ctxUser.
		instTok := ghInstallationTokenFromContext(r.Context())
		utsTok := ghUserToServerTokenFromContext(r.Context())
		jwtApp := ghAppFromContext(r.Context())
		user := ghUserFromContext(r.Context())

		switch {
		case instTok != nil:
			if !hasPerm(instTok.Permissions, scope, level) {
				writeGHError(w, http.StatusForbidden, "Resource not accessible by integration")
				return
			}
		case utsTok != nil:
			if !userToServerHasPerm(utsTok, scope, level, s.store) {
				writeGHError(w, http.StatusForbidden, "Resource not accessible by integration")
				return
			}
		case jwtApp != nil:
			// JWT auth is for app-meta endpoints only; reject on resource-level gates.
			writeGHError(w, http.StatusForbidden, "JWT can only be used for app-meta endpoints")
			return
		case user != nil:
			if token := ghPersonalAccessTokenFromContext(r.Context()); token != nil && token.FineGrained {
				if !s.fineGrainedPATAllows(r, token, scope, level) {
					writeGHError(w, http.StatusForbidden, "Resource not accessible by personal access token")
					return
				}
			}
		default:
			writeGHError(w, http.StatusUnauthorized, "Bad credentials")
			return
		}

		next(w, r)
	}
}

func (s *Server) fineGrainedPATAllows(r *http.Request, token *Token, scope permScope, level permLevel) bool {
	if token.ExpiresAt != nil && !token.ExpiresAt.After(time.Now()) {
		return false
	}
	if repo := s.repoFromPATRequest(r); repo != nil && !repo.Private && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
		return true
	}
	if !s.fineGrainedPATResourceAllowed(r, token) {
		return false
	}
	permissions := token.Permissions.Repository
	if r.PathValue("org") != "" && r.PathValue("repo") == "" && r.PathValue("repository_id") == "" {
		permissions = token.Permissions.Organization
	}
	return hasPerm(permissions, scope, level)
}

func (s *Server) fineGrainedPATApproved(token *Token) bool {
	if s.store.GetOrg(token.ResourceOwner) == nil {
		return true
	}
	s.store.mu.RLock()
	defer s.store.mu.RUnlock()
	for _, grant := range s.store.OrgPATGrants[token.ResourceOwner] {
		if grant.TokenID == token.FineGrainedID {
			return true
		}
	}
	return false
}

func (s *Server) repoFromPATRequest(r *http.Request) *Repo {
	if owner, name := r.PathValue("owner"), r.PathValue("repo"); owner != "" && name != "" {
		return s.store.GetRepo(owner, name)
	}
	if raw := r.PathValue("repository_id"); raw != "" {
		id, err := strconv.Atoi(raw)
		if err == nil {
			return s.store.GetRepoByID(id)
		}
	}
	return nil
}

func (s *Server) fineGrainedPATResourceAllowed(r *http.Request, token *Token) bool {
	if !s.fineGrainedPATApproved(token) {
		return false
	}
	if org := r.PathValue("org"); org != "" && !strings.EqualFold(org, token.ResourceOwner) {
		return false
	}
	repo := s.repoFromPATRequest(r)
	if repo == nil {
		return true
	}
	owner, _, ok := strings.Cut(repo.FullName, "/")
	if !ok || !strings.EqualFold(owner, token.ResourceOwner) {
		return false
	}
	switch token.RepositorySelection {
	case "all":
		return true
	case "subset":
		for _, id := range token.RepositoryIDs {
			if id == repo.ID {
				return true
			}
		}
	}
	return false
}

// enforceFineGrainedPATResource covers public handlers that perform their own
// role checks instead of using requirePerm. It runs after ServeMux has filled
// path values, so a selected-repository token cannot inherit its owner's wider
// membership through those handlers.
func (s *Server) enforceFineGrainedPATResource(pattern string, next http.HandlerFunc) http.HandlerFunc {
	if !strings.Contains(pattern, " /api/") || (!strings.Contains(pattern, "{repo}") && !strings.Contains(pattern, "{org}") && !strings.Contains(pattern, "{repository_id}")) {
		return next
	}
	return func(w http.ResponseWriter, r *http.Request) {
		token := ghPersonalAccessTokenFromContext(r.Context())
		if token == nil || !token.FineGrained {
			next(w, r)
			return
		}
		if repo := s.repoFromPATRequest(r); repo != nil && !repo.Private && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
			next(w, r)
			return
		}
		if !s.fineGrainedPATResourceAllowed(r, token) {
			writeGHError(w, http.StatusForbidden, "Resource not accessible by personal access token")
			return
		}
		scope, level := fineGrainedPATPermissionForPattern(pattern, r.Method)
		permissions := token.Permissions.Repository
		if r.PathValue("org") != "" && s.repoFromPATRequest(r) == nil && !strings.Contains(pattern, "/orgs/{org}/repos") {
			permissions = token.Permissions.Organization
		}
		if !hasPerm(permissions, scope, level) {
			writeGHError(w, http.StatusForbidden, "Resource not accessible by personal access token")
			return
		}
		next(w, r)
	}
}

func fineGrainedPATPermissionForPattern(pattern, method string) (permScope, permLevel) {
	level := permWrite
	if method == http.MethodGet || method == http.MethodHead {
		level = permRead
	}
	lower := strings.ToLower(pattern)
	if strings.Contains(lower, "/orgs/{org}/repos") {
		return scopeMetadata, level
	}
	if strings.Contains(lower, "{org}") && !strings.Contains(lower, "{repo}") && !strings.Contains(lower, "{repository_id}") {
		if strings.Contains(lower, "/members") || strings.Contains(lower, "/memberships") || strings.Contains(lower, "/teams") || strings.Contains(lower, "/invitations") || strings.Contains(lower, "/outside_collaborators") {
			return scopeMembers, level
		}
		return scopeOrgAdministration, level
	}
	for _, candidate := range []struct {
		fragment string
		scope    permScope
	}{
		{"/actions", scopeActions}, {"/issues", scopeIssues}, {"/milestones", scopeIssues}, {"/labels", scopeIssues},
		{"/pulls", scopePullRequests}, {"/checks", scopeChecks}, {"/deployments", scopeDeployments}, {"/environments", scopeDeployments},
		{"/pages", scopePages}, {"/codespaces", scopeCodespaces}, {"/secret-scanning", scopeSecurityEvents}, {"/code-scanning", scopeSecurityEvents},
		{"/dependabot", scopeDependabotSecrets}, {"/reactions", scopeReactions}, {"/projects", scopeProjects},
		{"/contents", scopeContents}, {"/git/", scopeContents}, {"/commits", scopeContents}, {"/branches", scopeContents}, {"/tags", scopeContents}, {"/releases", scopeContents},
		{"/collaborators", scopeAdministration}, {"/hooks", scopeAdministration}, {"/keys", scopeAdministration}, {"/rules", scopeAdministration},
	} {
		if strings.Contains(lower, candidate.fragment) {
			return candidate.scope, level
		}
	}
	if level == permRead {
		return scopeMetadata, level
	}
	return scopeAdministration, level
}

func (s *Server) filterReposForFineGrainedPAT(r *http.Request, repos []*Repo) []*Repo {
	token := ghPersonalAccessTokenFromContext(r.Context())
	if token == nil || !token.FineGrained {
		return repos
	}
	filtered := make([]*Repo, 0, len(repos))
	for _, repo := range repos {
		if !repo.Private {
			filtered = append(filtered, repo)
			continue
		}
		copy := r.Clone(r.Context())
		owner, name, ok := strings.Cut(repo.FullName, "/")
		if !ok {
			continue
		}
		copy.SetPathValue("owner", owner)
		copy.SetPathValue("repo", name)
		if s.fineGrainedPATResourceAllowed(copy, token) {
			filtered = append(filtered, repo)
		}
	}
	return filtered
}

// hasPerm checks an installation-token permissions map against (scope, level).
// Missing scope = no grant. Admin implies write, write implies read.
func hasPerm(perms map[string]string, scope permScope, level permLevel) bool {
	if perms == nil {
		return false
	}
	got, ok := perms[string(scope)]
	if !ok {
		// "metadata" is auto-granted on every installation per real GH; honour it
		// for readability checks.
		if scope == scopeMetadata && level == permRead {
			return true
		}
		return false
	}
	return parsePermLevel(got) >= level
}

// userToServerHasPerm dispatches a user-to-server token to either the App
// installation permissions map (ghu_) or the classic OAuth scopes (gho_).
func userToServerHasPerm(tok *UserToServerToken, scope permScope, level permLevel, st *Store) bool {
	if tok.AppID > 0 {
		// ghu_: use the installation's permissions. A token scoped to
		// specific installations (POST /applications/{cid}/token/scoped)
		// must be checked against exactly those; an unscoped token checks
		// any installation of the app.
		st.mu.RLock()
		defer st.mu.RUnlock()
		if len(tok.InstallationIDs) > 0 {
			for _, id := range tok.InstallationIDs {
				if inst := st.Installations[id]; inst != nil && inst.AppID == tok.AppID {
					return hasPerm(inst.Permissions, scope, level)
				}
			}
			return false
		}
		for _, inst := range st.Installations {
			if inst.AppID == tok.AppID {
				return hasPerm(inst.Permissions, scope, level)
			}
		}
		return false
	}
	// gho_: classic OAuth scopes → perm mapping.
	return classicScopeCovers(tok.Scopes, scope, level)
}

// validPermLevelString reports whether s is one of the permission levels the
// App API accepts in request bodies.
func validPermLevelString(s string) bool {
	switch strings.ToLower(s) {
	case "read", "write", "admin":
		return true
	}
	return false
}

// validateRequestedPermissions checks a token-mint request's permissions map
// against the installation's granted permissions: every requested scope must
// be granted at >= the requested level (metadata:read is implicitly granted
// on every installation). Returns the first offending scope and false on
// escalation or an unknown level value.
func validateRequestedPermissions(requested, granted map[string]string) (string, bool) {
	for scope, level := range requested {
		if !validPermLevelString(level) {
			return scope, false
		}
		grantedLevel, ok := granted[scope]
		if !ok {
			if permScope(scope) == scopeMetadata && parsePermLevel(level) == permRead {
				continue
			}
			return scope, false
		}
		if parsePermLevel(level) > parsePermLevel(grantedLevel) {
			return scope, false
		}
	}
	return "", true
}

// classicScopeCovers approximates real GH's mapping of classic OAuth scopes
// (`repo`, `read:org`, `gist`, ...) onto the fine-grained permission model
// the App API expresses.
//
// This is intentionally conservative — only canonical mappings.
func classicScopeCovers(scopes string, scope permScope, level permLevel) bool {
	set := map[string]struct{}{}
	for _, s := range strings.Split(scopes, ",") {
		s = strings.TrimSpace(s)
		if s != "" {
			set[s] = struct{}{}
		}
	}
	has := func(s string) bool { _, ok := set[s]; return ok }

	switch scope {
	case scopeMetadata:
		return level == permRead || has("repo") || has("public_repo")
	case scopeContents, scopeIssues, scopePullRequests, scopePages:
		if has("repo") {
			return level <= permWrite
		}
		if has("public_repo") {
			return level <= permWrite
		}
		return false
	case scopeChecks:
		if has("repo") {
			return level <= permWrite
		}
		return false
	case scopeAdministration:
		return has("admin:repo_hook") && level <= permWrite
	case scopeMembers, scopeOrgAdministration:
		if has("admin:org") {
			return level <= permAdmin
		}
		if has("write:org") {
			return level <= permWrite
		}
		if has("read:org") {
			return level == permRead
		}
		return false
	case scopeSecrets, scopeSecurityEvents, scopeDependabotSecrets, scopeReactions:
		if has("repo") {
			return level <= permWrite
		}
		return false
	case scopeCodespaces:
		if has("codespace") || has("repo") {
			return level <= permWrite
		}
		return false
	case scopeProjects:
		if has("project") || has("repo") || has("public_repo") {
			return level <= permWrite
		}
		return false
	}
	return false
}
