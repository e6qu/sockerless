package bleephub

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	// jwtMaxLifetimeSeconds mirrors GitHub's 10-minute cap on app JWT expiry.
	jwtMaxLifetimeSeconds = 600
	// jwtClockDriftSeconds is the tolerance GitHub allows for client clock skew.
	jwtClockDriftSeconds = 60
)

// parseAndVerifyAppJWT validates an RS256 JWT against stored app keys.
func (st *Store) parseAndVerifyAppJWT(tokenStr string) (*App, error) {
	parts := strings.SplitN(tokenStr, ".", 3)
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid JWT: expected 3 parts")
	}

	headerBytes, err := base64urlDecode(parts[0])
	if err != nil {
		return nil, fmt.Errorf("invalid JWT header: %w", err)
	}
	var header struct {
		Alg string `json:"alg"`
	}
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, fmt.Errorf("invalid JWT header JSON: %w", err)
	}
	if header.Alg != "RS256" {
		return nil, fmt.Errorf("unsupported algorithm: %s", header.Alg)
	}

	payloadBytes, err := base64urlDecode(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid JWT payload: %w", err)
	}
	var payload struct {
		Iss string  `json:"iss"`
		Iat float64 `json:"iat"`
		Exp float64 `json:"exp"`
	}
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return nil, fmt.Errorf("invalid JWT payload JSON: %w", err)
	}

	iat := int64(payload.Iat)
	exp := int64(payload.Exp)

	// Real GitHub bounds the claims relative to ITS clock: exp at most 10
	// minutes ahead and iat in the past, each with ~60s drift tolerance.
	// It does not constrain exp-iat directly, so a client that backdates
	// iat for clock skew (ghinstallation sets iat=now-60) stays valid.
	now := time.Now().Unix()
	if exp <= now {
		return nil, fmt.Errorf("JWT expired")
	}
	if exp > now+jwtMaxLifetimeSeconds+jwtClockDriftSeconds {
		return nil, fmt.Errorf("JWT 'exp' claim is too far in the future (max %ds ahead)", jwtMaxLifetimeSeconds)
	}
	if iat > now+jwtClockDriftSeconds {
		return nil, fmt.Errorf("JWT iat is in the future")
	}

	appID, err := strconv.Atoi(payload.Iss)
	if err != nil {
		return nil, fmt.Errorf("invalid iss claim: %w", err)
	}

	st.mu.RLock()
	app := st.Apps[appID]
	st.mu.RUnlock()
	if app == nil {
		return nil, fmt.Errorf("app not found: %d", appID)
	}

	block, _ := pem.Decode([]byte(app.PEMPrivateKey))
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM for app %d", appID)
	}
	privKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	sigBytes, err := base64urlDecode(parts[2])
	if err != nil {
		return nil, fmt.Errorf("invalid JWT signature: %w", err)
	}

	signInput := parts[0] + "." + parts[1]
	hash := sha256.Sum256([]byte(signInput))
	if err := rsa.VerifyPKCS1v15(&privKey.PublicKey, crypto.SHA256, hash[:], sigBytes); err != nil {
		return nil, fmt.Errorf("invalid JWT signature: %w", err)
	}

	return app, nil
}

// base64urlDecode handles JWT's unpadded base64url encoding.
func base64urlDecode(s string) ([]byte, error) {
	// Add padding if needed
	switch len(s) % 4 {
	case 2:
		s += "=="
	case 3:
		s += "="
	}
	return base64.URLEncoding.DecodeString(s)
}

// looksLikeJWT returns true if the string has the structure of a JWT.
func looksLikeJWT(s string) bool {
	return strings.HasPrefix(s, "eyJ") && strings.Count(s, ".") == 2
}
