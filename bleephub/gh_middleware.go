package bleephub

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

type contextKey string

const ctxUser contextKey = "gh-user"
const ctxApp contextKey = "gh-app"
const ctxInstallation contextKey = "gh-installation"
const ctxInstallationToken contextKey = "gh-installation-token"
const ctxUserToServerToken contextKey = "gh-uts-token"
const ctxPersonalAccessToken contextKey = "gh-personal-access-token"
const ctxSuspendedInstallation contextKey = "gh-suspended-installation"
const ctxSuspendedUser contextKey = "gh-suspended-user"

// GitHub token prefixes. Each prefix selects a different lookup table and
// auth shape in authenticateRequest; using the named constants keeps the
// middleware, stores and handlers agreeing on the exact prefix bytes.
const (
	tokenPrefixInstallation = "ghs_" // installation access token
	tokenPrefixOAuthUser    = "gho_" // classic OAuth-App user token
	tokenPrefixAppUser      = "ghu_" // GitHub-App user-to-server token
	tokenPrefixRefresh      = "ghr_" // refresh token (never valid as auth)
)

// ghUserFromContext extracts the authenticated user from the request context.
func ghUserFromContext(ctx context.Context) *User {
	u, _ := ctx.Value(ctxUser).(*User)
	return u
}

// ghAppFromContext extracts the JWT-authenticated app from the request context.
func ghAppFromContext(ctx context.Context) *App {
	a, _ := ctx.Value(ctxApp).(*App)
	return a
}

// ghInstallationFromContext extracts the installation associated with the request,
// if authenticated by a ghs_ installation token. Returns nil for other auth shapes.
// Consumed by gh_apps_rest.go (installation introspection) and the permission
// decorator.
func ghInstallationFromContext(ctx context.Context) *Installation {
	i, _ := ctx.Value(ctxInstallation).(*Installation)
	return i
}

// ghInstallationTokenFromContext extracts the installation token used to authenticate
// the request, if any. Consumed by gh_apps_perms.go (permission decorator) and
// gh_apps_rest.go (introspection endpoints).
func ghInstallationTokenFromContext(ctx context.Context) *InstallationToken {
	t, _ := ctx.Value(ctxInstallationToken).(*InstallationToken)
	return t
}

// ghUserToServerTokenFromContext extracts the gho_/ghu_ token used to authenticate,
// if any. Consumed by gh_apps_perms.go (permission decorator's user-to-server path).
func ghUserToServerTokenFromContext(ctx context.Context) *UserToServerToken {
	t, _ := ctx.Value(ctxUserToServerToken).(*UserToServerToken)
	return t
}

func ghPersonalAccessTokenFromContext(ctx context.Context) *Token {
	t, _ := ctx.Value(ctxPersonalAccessToken).(*Token)
	return t
}

// ghHeadersMiddleware injects GitHub-compatible response headers on /api/ routes
// and sets the authenticated user in request context.
func (s *Server) ghHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Activate for the REST API plus the uploads-host and authenticated
		// CodeQL storage paths. The official CodeQL Action posts database
		// bundles to /repos/... on uploads.github.com rather than /api/v3/.
		// Runner protocol (/_apis/) remains unaffected.
		if !strings.HasPrefix(path, "/api/") && !strings.HasPrefix(path, "/repos/") && !strings.HasPrefix(path, "/code-scanning/") {
			next.ServeHTTP(w, r)
			return
		}

		ctx := s.authenticateRequest(r)
		r = r.WithContext(ctx)

		// A suspended installation's tokens are dead for the entire API
		// surface (real GitHub fails every request made with the app's
		// credentials while suspended), not just for minting new tokens.
		if susp, _ := ctx.Value(ctxSuspendedInstallation).(bool); susp {
			writeGHError(w, http.StatusForbidden, "This installation has been suspended")
			return
		}
		if susp, _ := ctx.Value(ctxSuspendedUser).(bool); susp {
			writeGHError(w, http.StatusForbidden, "This account has been suspended")
			return
		}

		// Parse token for rate-limit header info
		var token *Token
		if auth := r.Header.Get("Authorization"); auth != "" {
			scheme, cred := authScheme(auth)
			var tokenStr string
			if scheme == "token" || scheme == "bearer" {
				tokenStr = cred
			}
			if tokenStr != "" && !looksLikeJWT(tokenStr) && !strings.HasPrefix(tokenStr, tokenPrefixInstallation) && !strings.HasPrefix(tokenStr, tokenPrefixRefresh) {
				if strings.HasPrefix(tokenStr, tokenPrefixOAuthUser) || strings.HasPrefix(tokenStr, tokenPrefixAppUser) {
					if utsTok, _ := s.store.LookupUserToServerToken(tokenStr); utsTok != nil {
						// Materialize a transient classic token so the response
						// writer can emit X-OAuth-Scopes for OAuth/GitHub-App
						// user-to-server tokens, matching real GitHub.
						token = &Token{Value: utsTok.Token, UserID: utsTok.UserID, Scopes: utsTok.Scopes}
					}
				} else {
					token, _ = s.store.LookupToken(tokenStr)
				}
			} else if scheme == "basic" {
				decoded, err := base64.StdEncoding.DecodeString(cred)
				if err == nil {
					parts := strings.SplitN(string(decoded), ":", 2)
					if len(parts) == 2 && parts[1] != "" {
						token, _ = s.store.LookupToken(parts[1])
					}
				}
			}
		}

		// Wrap response writer to inject headers
		rw := &ghResponseWriter{
			ResponseWriter: w,
			token:          token,
			path:           path,
		}
		next.ServeHTTP(rw, r)
	})
}

// authScheme splits an Authorization header into its lower-cased scheme and
// the credential. HTTP auth schemes are case-insensitive (RFC 7235), and
// GitHub accepts "token"/"Bearer"/"Basic" in any case (octokit sends "bearer"),
// so match case-insensitively rather than on an exact-case prefix.
func authScheme(auth string) (scheme, credential string) {
	s, cred, found := strings.Cut(auth, " ")
	if !found {
		return "", ""
	}
	return strings.ToLower(s), cred
}

// authenticateRequest parses the Authorization header and returns a context
// with the authenticated user/app/installation set. Used by both /api/
// middleware and git HTTP handlers.
func (s *Server) authenticateRequest(r *http.Request) context.Context {
	ctx := r.Context()
	var user *User
	if auth := r.Header.Get("Authorization"); auth != "" {
		scheme, cred := authScheme(auth)
		var tokenStr string
		if scheme == "token" || scheme == "bearer" {
			tokenStr = cred
		}
		if tokenStr != "" {
			switch {
			case looksLikeJWT(tokenStr):
				if app, err := s.store.parseAndVerifyAppJWT(tokenStr); err == nil {
					ctx = context.WithValue(ctx, ctxApp, app)
				}
			case strings.HasPrefix(tokenStr, tokenPrefixInstallation):
				if instToken, inst := s.store.LookupInstallationToken(tokenStr); instToken != nil {
					if inst != nil && inst.SuspendedAt != nil {
						ctx = context.WithValue(ctx, ctxSuspendedInstallation, true)
						break
					}
					ctx = context.WithValue(ctx, ctxInstallation, inst)
					ctx = context.WithValue(ctx, ctxInstallationToken, instToken)
					app := s.store.GetApp(instToken.AppID)
					if app != nil {
						botUser := &User{Login: app.Slug + "[bot]", Type: "Bot", ID: -app.ID}
						ctx = context.WithValue(ctx, ctxUser, botUser)
					}
				}
			case strings.HasPrefix(tokenStr, tokenPrefixOAuthUser), strings.HasPrefix(tokenStr, tokenPrefixAppUser):
				if utsTok, u := s.store.LookupUserToServerToken(tokenStr); utsTok != nil {
					ctx = context.WithValue(ctx, ctxUserToServerToken, utsTok)
					if u != nil {
						ctx = context.WithValue(ctx, ctxUser, u)
						user = u
					}
				}
			case strings.HasPrefix(tokenStr, tokenPrefixRefresh):
			default:
				if token, resolved := s.store.LookupToken(tokenStr); token != nil {
					user = resolved
					ctx = context.WithValue(ctx, ctxPersonalAccessToken, token)
				}
			}
		} else if scheme == "basic" {
			decoded, err := base64.StdEncoding.DecodeString(cred)
			if err == nil {
				parts := strings.SplitN(string(decoded), ":", 2)
				if len(parts) == 2 && parts[1] != "" {
					if token, resolved := s.store.LookupToken(parts[1]); token != nil {
						user = resolved
						ctx = context.WithValue(ctx, ctxPersonalAccessToken, token)
					}
				}
			}
		}
	}
	if user == nil {
		if session := s.sessionFromRequest(r); session != nil {
			user = s.store.GetUserByID(session.UserID)
		}
	}
	if user != nil {
		if user.Suspended {
			ctx = context.WithValue(ctx, ctxSuspendedUser, true)
		} else {
			ctx = context.WithValue(ctx, ctxUser, user)
		}
	}
	return ctx
}

// ghResponseWriter injects GitHub API headers before the first write.
type ghResponseWriter struct {
	http.ResponseWriter
	token       *Token
	path        string
	wroteHeader bool
}

func (rw *ghResponseWriter) WriteHeader(code int) {
	if !rw.wroteHeader {
		rw.wroteHeader = true
		h := rw.Header()

		// Upgrade Content-Type to include charset
		if ct := h.Get("Content-Type"); ct == "application/json" {
			h.Set("Content-Type", "application/json; charset=utf-8")
		}

		if rw.token != nil {
			h.Set("X-OAuth-Scopes", rw.token.Scopes)
		}
		h.Set("X-Accepted-OAuth-Scopes", "")

		now := time.Now()
		h.Set("X-RateLimit-Limit", "5000")
		h.Set("X-RateLimit-Remaining", "4999")
		h.Set("X-RateLimit-Used", "1")
		h.Set("X-RateLimit-Reset", fmt.Sprintf("%d", now.Unix()+3600))

		resource := "core"
		if strings.HasPrefix(rw.path, "/api/graphql") {
			resource = "graphql"
		}
		h.Set("X-RateLimit-Resource", resource)
		h.Set("X-GitHub-Request-Id", uuid.New().String())
		h.Set("X-GitHub-Api-Version", "2022-11-28")
	}
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *ghResponseWriter) Write(b []byte) (int, error) {
	if !rw.wroteHeader {
		rw.WriteHeader(http.StatusOK)
	}
	return rw.ResponseWriter.Write(b)
}
