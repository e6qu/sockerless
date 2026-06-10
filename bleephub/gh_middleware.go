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

// ghHeadersMiddleware injects GitHub-compatible response headers on /api/ routes
// and sets the authenticated user in request context.
func (s *Server) ghHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Only activate for /api/ paths — runner protocol (/_apis/) is unaffected
		if !strings.HasPrefix(path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}

		ctx := s.authenticateRequest(r)
		r = r.WithContext(ctx)

		// Parse token for rate-limit header info
		var token *Token
		if auth := r.Header.Get("Authorization"); auth != "" {
			var tokenStr string
			if strings.HasPrefix(auth, "token ") {
				tokenStr = strings.TrimPrefix(auth, "token ")
			} else if strings.HasPrefix(auth, "Bearer ") {
				tokenStr = strings.TrimPrefix(auth, "Bearer ")
			}
			if tokenStr != "" && !looksLikeJWT(tokenStr) && !strings.HasPrefix(tokenStr, "ghs_") && !strings.HasPrefix(tokenStr, "gho_") && !strings.HasPrefix(tokenStr, "ghu_") && !strings.HasPrefix(tokenStr, "ghr_") {
				token, _ = s.store.LookupToken(tokenStr)
			} else if strings.HasPrefix(auth, "Basic ") {
				decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(auth, "Basic "))
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

// authenticateRequest parses the Authorization header and returns a context
// with the authenticated user/app/installation set. Used by both /api/
// middleware and git HTTP handlers.
func (s *Server) authenticateRequest(r *http.Request) context.Context {
	ctx := r.Context()
	var user *User
	if auth := r.Header.Get("Authorization"); auth != "" {
		var tokenStr string
		if strings.HasPrefix(auth, "token ") {
			tokenStr = strings.TrimPrefix(auth, "token ")
		} else if strings.HasPrefix(auth, "Bearer ") {
			tokenStr = strings.TrimPrefix(auth, "Bearer ")
		}
		if tokenStr != "" {
			switch {
			case looksLikeJWT(tokenStr):
				if app, err := s.store.parseAndVerifyAppJWT(tokenStr); err == nil {
					ctx = context.WithValue(ctx, ctxApp, app)
				}
			case strings.HasPrefix(tokenStr, "ghs_"):
				if instToken, inst := s.store.LookupInstallationToken(tokenStr); instToken != nil {
					ctx = context.WithValue(ctx, ctxInstallation, inst)
					ctx = context.WithValue(ctx, ctxInstallationToken, instToken)
					app := s.store.GetApp(instToken.AppID)
					if app != nil {
						botUser := &User{Login: app.Slug + "[bot]", Type: "Bot", ID: -app.ID}
						ctx = context.WithValue(ctx, ctxUser, botUser)
					}
				}
			case strings.HasPrefix(tokenStr, "gho_"), strings.HasPrefix(tokenStr, "ghu_"):
				if utsTok, u := s.store.LookupUserToServerToken(tokenStr); utsTok != nil {
					ctx = context.WithValue(ctx, ctxUserToServerToken, utsTok)
					if u != nil {
						ctx = context.WithValue(ctx, ctxUser, u)
						user = u
					}
				}
			case strings.HasPrefix(tokenStr, "ghr_"):
			default:
				_, user = s.store.LookupToken(tokenStr)
			}
		} else if strings.HasPrefix(auth, "Basic ") {
			decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(auth, "Basic "))
			if err == nil {
				parts := strings.SplitN(string(decoded), ":", 2)
				if len(parts) == 2 && parts[1] != "" {
					_, user = s.store.LookupToken(parts[1])
				}
			}
		}
	}
	if user != nil {
		ctx = context.WithValue(ctx, ctxUser, user)
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
