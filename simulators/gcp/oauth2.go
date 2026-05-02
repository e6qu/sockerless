package main

import (
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
// We don't validate the JWT signature — the sim isn't an emulator and
// doesn't enforce GCP's auth gate; it just responds with the wire shape
// the SDK expects so the SDK proceeds to the actual API call. Real
// production deployments hit oauth2.googleapis.com which DOES validate;
// the sim plays the same role from the SDK's perspective without the
// validation cost.
//
// Operators (or tests) point the SA JSON's `token_uri` at this endpoint
// when their backend's GCP SDKs should mint tokens against the sim
// instead of Google's real token service.
func registerOAuth2(srv *sim.Server) {
	// POST /token — the bare path the SA JSON's token_uri resolves to.
	// The Google client lib also accepts /oauth2/v4/token historically.
	handler := func(w http.ResponseWriter, r *http.Request) {
		// Body parsing intentionally permissive: real SDKs send
		// form-encoded JWT-bearer requests; service-to-service flows
		// also use grant_type=client_credentials. We don't enforce —
		// just respond with the standard token envelope.
		_ = r.ParseForm()
		sim.WriteJSON(w, http.StatusOK, map[string]any{
			"access_token": "sim-access-" + generateUUID(),
			"expires_in":   int((1 * time.Hour).Seconds()),
			"token_type":   "Bearer",
			// The id_token is what idtoken.NewClient flows expect —
			// it's a JWT but the SDK doesn't verify it locally before
			// using as the Authorization header. Returning a non-empty
			// string is sufficient. Production tokens are real signed
			// JWTs the audience verifies; the sim's audience handler
			// (e.g. /v2-functions-invoke/) doesn't validate, so any
			// non-empty bearer works.
			"id_token": "sim-id-" + generateUUID(),
		})
	}
	srv.HandleFunc("POST /token", handler)
	srv.HandleFunc("POST /oauth2/v4/token", handler)
}
