package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
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
// The access_token is issued as a real-shape JWT (header.payload.signature
// base64url segments) so SDKs that parse the token before using it
// (cloudbuild.NewRESTClient does) accept the response. The JWT's HMAC
// signature uses a per-process random key — the sim doesn't validate
// inbound tokens; the signature is real-shape but unverifiable by
// downstream consumers, which is fine because the sim's audience
// handlers (e.g. /v2-functions-invoke/, /v1/projects/.../builds) don't
// validate inbound tokens either. Real production routes through
// oauth2.googleapis.com whose tokens ARE validated by Google's
// audience services; the sim plays the same role from the SDK's
// perspective without the validation cost.
func registerOAuth2(srv *sim.Server) {
	// One signing key per simulator process. Real production uses
	// Google's signing infrastructure with rotated keys; the sim is
	// process-scoped because a sim restart issues fresh tokens
	// regardless.
	signKey := make([]byte, 32)
	_, _ = rand.Read(signKey)

	handler := func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		now := time.Now()
		expires := now.Add(1 * time.Hour)

		token := mintSimJWT(signKey, "sockerless-sim", "sockerless-sim", now, expires)
		idToken := mintSimJWT(signKey, "sockerless-sim", "sockerless-sim", now, expires)

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

// mintSimJWT produces a real-shape JWT (`header.payload.signature`)
// signed with HS256 against the sim's per-process key. Real Google
// JWTs use RS256 with rotated keys; HS256 is sufficient here because
// the sim doesn't verify inbound tokens — the only requirement is
// that SDKs that parse the token (the cloudbuild SDK does) accept
// the structure.
func mintSimJWT(signKey []byte, issuer, subject string, issuedAt, expiresAt time.Time) string {
	headerJSON, _ := json.Marshal(map[string]string{
		"alg": "HS256",
		"typ": "JWT",
	})
	payloadJSON, _ := json.Marshal(map[string]any{
		"iss":   issuer,
		"sub":   subject,
		"aud":   "https://oauth2.googleapis.com/token",
		"iat":   issuedAt.Unix(),
		"exp":   expiresAt.Unix(),
		"scope": "https://www.googleapis.com/auth/cloud-platform",
	})
	headerB64 := base64.RawURLEncoding.EncodeToString(headerJSON)
	payloadB64 := base64.RawURLEncoding.EncodeToString(payloadJSON)
	signingInput := headerB64 + "." + payloadB64
	mac := hmac.New(sha256.New, signKey)
	mac.Write([]byte(signingInput))
	sigB64 := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return signingInput + "." + sigB64
}
