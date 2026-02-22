package agent

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// authMiddleware validates Bearer token authentication.
func authMiddleware(token string, next http.Handler) http.Handler {
	if token == "" {
		return next
	}
	expected := []byte(token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth for health endpoint
		if r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}

		auth := r.Header.Get("Authorization")
		if auth == "" {
			// Also check query param for WebSocket connections
			auth = "Bearer " + r.URL.Query().Get("token")
		}

		if !strings.HasPrefix(auth, "Bearer ") {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		provided := []byte(strings.TrimPrefix(auth, "Bearer "))
		if subtle.ConstantTimeCompare(provided, expected) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}
