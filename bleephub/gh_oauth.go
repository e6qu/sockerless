package bleephub

import (
	"net/http"
	"time"

	"github.com/google/uuid"
)

func (s *Server) registerGHOAuthRoutes() {
	s.mux.HandleFunc("POST /login/device/code", s.handleDeviceCode)
	s.mux.HandleFunc("POST /login/oauth/access_token", s.handleDeviceToken)
	s.mux.HandleFunc("GET /login/device", s.handleDevicePage)
}

// handleDeviceCode initiates the device authorization flow.
// Auto-creates a device code with a pre-generated token for the default admin user.
func (s *Server) handleDeviceCode(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	scope := r.FormValue("scope")

	s.store.mu.Lock()
	// Use admin user (ID=1)
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

// handleDeviceToken polls for the access token. Auto-approved immediately.
func (s *Server) handleDeviceToken(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	grantType := r.FormValue("grant_type")
	deviceCode := r.FormValue("device_code")

	// Accept both the standard grant_type and legacy format
	_ = grantType

	s.store.mu.RLock()
	dc, ok := s.store.DeviceCodes[deviceCode]
	s.store.mu.RUnlock()

	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"error":"bad_verification_code"}`))
		return
	}

	s.logger.Info().Str("device_code", deviceCode).Msg("device token granted")

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"access_token": dc.Token,
		"token_type":   "bearer",
		"scope":        "repo read:org gist",
	})
}

// handleDevicePage renders a simple HTML confirmation page.
func (s *Server) handleDevicePage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(`<!DOCTYPE html><html><body><h1>Auto-approved by bleephub</h1><p>You can close this page.</p></body></html>`))
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
	return t
}
