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

// ghUserFromContext extracts the authenticated user from the request context.
func ghUserFromContext(ctx context.Context) *User {
	u, _ := ctx.Value(ctxUser).(*User)
	return u
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

		// Parse Authorization header: "token {pat}" or "Bearer {pat}"
		var token *Token
		var user *User
		if auth := r.Header.Get("Authorization"); auth != "" {
			var tokenStr string
			if strings.HasPrefix(auth, "token ") {
				tokenStr = strings.TrimPrefix(auth, "token ")
			} else if strings.HasPrefix(auth, "Bearer ") {
				tokenStr = strings.TrimPrefix(auth, "Bearer ")
			}
			if tokenStr != "" {
				token, user = s.store.LookupToken(tokenStr)
			}
		}

		// Store user in context
		ctx := r.Context()
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
