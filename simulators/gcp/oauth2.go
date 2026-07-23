package main

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	sim "github.com/sockerless/simulator"
)

// registerOAuth2 wires the minimal OAuth2 token endpoint that
// service-account JWT-bearer flows hit when minting access and ID tokens.
//
// Real flow: the SDK constructs a JWT signed with the SA's private key and
// POSTs it to `https://oauth2.googleapis.com/token` with
// `grant_type=urn:ietf:params:oauth:grant-type:jwt-bearer`. For an access
// token it gets back `{"access_token":"...","token_type":"Bearer",...}`; when
// the assertion carries a `target_audience` claim (the workload requesting an
// ID token for a downstream Cloud Run / Cloud Functions invoke) Google also
// returns an RS256 `id_token` whose `aud` is that target audience. The workload
// then presents whichever token as `Authorization: Bearer ...`.
//
// The sim's role: accept the POST and return a real-shape token response. Both
// the access token and the ID token are RS256 JWTs the simulator signs with its
// own access-token key (see signAccessToken / signIdentityToken); the
// data-plane bearer middleware verifies against that same key, so a token
// minted here is accepted on subsequent requests and an unverifiable one is
// rejected. Real production routes through oauth2.googleapis.com whose tokens
// Google validates internally; the sim plays the same role with tokens it both
// issues and verifies.
func registerOAuth2(srv *sim.Server) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		now := time.Now()
		expires := now.Add(1 * time.Hour)

		// The principal (service-account email) and requested ID-token
		// audience come from the JWT-bearer assertion. Test surfaces that
		// only need an access token POST a bare grant_type with no
		// assertion, in which case the sim mints an access token for the
		// default principal and returns no ID token — matching real Google,
		// which returns an id_token only when target_audience is requested.
		principal, targetAudience := serviceAccountAssertionClaims(r.Form.Get("assertion"))
		if principal == "" {
			principal = "sockerless-sim"
		}

		resp := map[string]any{
			"access_token": signAccessToken(principal, now, expires),
			"expires_in":   int(time.Until(expires).Seconds()),
			"token_type":   "Bearer",
		}
		if targetAudience != "" {
			resp["id_token"] = signInvokeIDToken(principal, now, expires)
		}
		sim.WriteJSON(w, http.StatusOK, resp)
	}
	srv.HandleFunc("POST /token", handler)
	srv.HandleFunc("POST /oauth2/v4/token", handler)
}

// serviceAccountAssertionClaims decodes the `iss` (service-account email) and
// `target_audience` claims from a service-account JWT-bearer assertion. The
// golang.org/x/oauth2 service-account ID-token flow signs an assertion whose
// payload carries `target_audience` = the URL the workload will invoke, and the
// token endpoint mints an ID token for that audience. Both returns are empty
// when the request carries no assertion (a plain access-token grant) or the
// assertion is not a decodable JWT, in which case the caller mints an access
// token for the default principal and no ID token.
func serviceAccountAssertionClaims(assertion string) (principal, targetAudience string) {
	if assertion == "" {
		return "", ""
	}
	parts := strings.Split(assertion, ".")
	if len(parts) != 3 {
		return "", ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", ""
	}
	var claims struct {
		Iss            string `json:"iss"`
		TargetAudience string `json:"target_audience"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", ""
	}
	return claims.Iss, claims.TargetAudience
}
