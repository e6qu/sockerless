package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	sim "github.com/sockerless/simulator"
)

// registerOAuth2 wires the minimal OAuth2 token endpoint that
// service-account JWT-bearer flows hit when minting access tokens.
//
// Real flow: SDK constructs a JWT signed with the SA's private key,
// POSTs it to `https://oauth2.googleapis.com/token` with
// `grant_type=urn:ietf:params:oauth:grant-type:jwt-bearer`, gets back
// `{"access_token":"...","expires_in":3600,"token_type":"Bearer"}`,
// then sends the access token as `Authorization: Bearer ...` on
// subsequent requests.
//
// The sim's role: accept the POST, return a real-shape token response.
// The access_token is a JWT the simulator signs with its own access-token
// key (see signAccessToken); the data-plane bearer middleware verifies
// against that same key, so a token minted here is accepted on subsequent
// requests and an unverifiable one is rejected. Real production routes
// through oauth2.googleapis.com whose opaque tokens Google validates
// internally; the sim plays the same role with a token it both issues and
// verifies.
func registerOAuth2(srv *sim.Server) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		now := time.Now()
		expires := now.Add(1 * time.Hour)

		token := signAccessToken("sockerless-sim", now, expires)
		idToken := mintSimIdToken(idTokenSignKey(), "sockerless-sim", "sockerless-sim", false, now, expires)

		sim.WriteJSON(w, http.StatusOK, map[string]any{
			"access_token": token,
			"expires_in":   int(time.Until(expires).Seconds()),
			"token_type":   "Bearer",
			"id_token":     idToken,
		})
	}
	srv.HandleFunc("POST /token", handler)
	srv.HandleFunc("POST /oauth2/v4/token", handler)
}

// idTokenKey is the per-process HMAC key shared by every ID token the
// sim mints (oauth2 endpoint id_token + iamcredentials.generateIdToken).
// Real Google rotates the signing key; the sim is process-scoped so a
// restart issues fresh tokens regardless.
var (
	idTokenKey     []byte
	idTokenKeyOnce sync.Once
)

func idTokenSignKey() []byte {
	idTokenKeyOnce.Do(func() {
		idTokenKey = make([]byte, 32)
		_, _ = rand.Read(idTokenKey)
	})
	return idTokenKey
}

// mintSimIdToken mints a real-shape Google ID token JWT for the given
// service-account email + audience. Used by iamcredentials.generateIdToken
// (`POST .../serviceAccounts/{email}:generateIdToken`). Audience is the
// target service URL the caller will invoke with this token. `email`
// claim is set when the caller requests includeEmail=true (matches real
// GCP behaviour). Signature is HS256 against the sim's per-process key
// — the sim's audience handlers don't validate, but SDKs that pre-decode
// the token expect a parseable structure.
func mintSimIdToken(signKey []byte, email, audience string, includeEmail bool, issuedAt, expiresAt time.Time) string {
	headerJSON, _ := json.Marshal(map[string]string{
		"alg": "HS256",
		"typ": "JWT",
	})
	claims := map[string]any{
		"iss": "https://accounts.google.com",
		"sub": email,
		"aud": audience,
		"iat": issuedAt.Unix(),
		"exp": expiresAt.Unix(),
		"azp": email,
	}
	if includeEmail {
		claims["email"] = email
		claims["email_verified"] = true
	}
	payloadJSON, _ := json.Marshal(claims)
	headerB64 := base64.RawURLEncoding.EncodeToString(headerJSON)
	payloadB64 := base64.RawURLEncoding.EncodeToString(payloadJSON)
	signingInput := headerB64 + "." + payloadB64
	mac := hmac.New(sha256.New, signKey)
	mac.Write([]byte(signingInput))
	sigB64 := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return signingInput + "." + sigB64
}
