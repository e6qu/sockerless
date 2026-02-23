package bleephub

import (
	"context"
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

		// Parse Authorization header: "token {pat}", "Bearer {pat}", JWT, or ghs_ token
		var token *Token
		var user *User
		ctx := r.Context()
		if auth := r.Header.Get("Authorization"); auth != "" {
			var tokenStr string
			if strings.HasPrefix(auth, "token ") {
				tokenStr = strings.TrimPrefix(auth, "token ")
			} else if strings.HasPrefix(auth, "Bearer ") {
				tokenStr = strings.TrimPrefix(auth, "Bearer ")
			}
			if tokenStr != "" {
				if looksLikeJWT(tokenStr) {
					if app, err := s.store.parseAndVerifyAppJWT(tokenStr); err == nil {
						ctx = context.WithValue(ctx, ctxApp, app)
					}
				} else if strings.HasPrefix(tokenStr, "ghs_") {
					if instToken, inst := s.store.LookupInstallationToken(tokenStr); instToken != nil {
						ctx = context.WithValue(ctx, ctxInstallation, inst)
						app := s.store.GetApp(instToken.AppID)
						if app != nil {
							botUser := &User{Login: app.Slug + "[bot]", Type: "Bot", ID: -app.ID}
							ctx = context.WithValue(ctx, ctxUser, botUser)
						}
					}
				} else {
					token, user = s.store.LookupToken(tokenStr)
				}
			}
		}

		// Store user in context
		if user != nil {
			ctx = context.WithValue(ctx, ctxUser, user)
		}
		r = r.WithContext(ctx)

		// Wrap response writer to inject headers
		rw := &ghResponseWriter{
			ResponseWriter: w,
			token:          token,
			path:           path,
		}
		next.ServeHTTP(rw, r)
	})
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
