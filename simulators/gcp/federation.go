package main

import (
	"net/http"

	sim "github.com/sockerless/simulator"
)

// registerFederationBroker mounts the console's cloud-credential broker on the
// operator-session boundary.
//
// A real Google Cloud console federates the signed-in operator into a
// short-lived cloud access token and calls the Google APIs with it. The
// simulator's console does the same: the browser asks the broker for a token,
// the broker reads the operator's Shauth assertion from the session it is
// already authenticated on, and exchanges it through Workforce Identity
// Federation. The assertion never reaches the browser — only the federated
// token does, exactly as a real console keeps the identity-provider assertion
// server-side.
func registerFederationBroker(srv *sim.Server) {
	srv.HandleUIFunc("GET /auth/cloud-token", func(w http.ResponseWriter, r *http.Request) {
		assertion, issuer, audience, ok := srv.OperatorAssertion(r)
		if !ok {
			sim.WriteJSON(w, http.StatusUnauthorized, map[string]string{
				"error":             "no_session",
				"error_description": "no signed-in operator to federate",
			})
			return
		}

		// The deployment's single sign-on provider is federated by a workforce
		// pool provider. The broker ensures the one for its own configured
		// provider exists — the same federation an administrator would set up —
		// then exchanges the operator's assertion against it.
		providerName := ensureConsoleWorkforceProvider(issuer, audience)

		token, expiresIn, code, err := federateWorkforceSubject(r.Context(), providerName, assertion)
		if err != nil {
			sim.WriteJSON(w, http.StatusBadGateway, map[string]string{
				"error":             code,
				"error_description": err.Error(),
			})
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		sim.WriteJSON(w, http.StatusOK, map[string]any{
			"access_token": token,
			"token_type":   "Bearer",
			"expires_in":   expiresIn,
		})
	})
}

const (
	consoleWorkforcePool     = "locations/global/workforcePools/sockerless-console"
	consoleWorkforceProvider = consoleWorkforcePool + "/providers/sso"
)

// ensureConsoleWorkforceProvider makes sure the workforce pool and OpenID
// Connect provider that federate the console's single sign-on provider exist,
// creating them in the same IAM store the Workforce Identity Federation API
// writes to, and returns the provider resource name. It is idempotent: the
// provider's issuer and client ID are kept in step with the console's
// configured identity coordinates.
func ensureConsoleWorkforceProvider(issuer, audience string) string {
	if _, ok := iamResources.Get(consoleWorkforcePool); !ok {
		iamResources.Put(consoleWorkforcePool, map[string]any{
			"name":        consoleWorkforcePool,
			"displayName": "Sockerless Console",
			"parent":      "organizations/sockerless",
			"state":       "ACTIVE",
		})
	}
	provider, ok := iamResources.Get(consoleWorkforceProvider)
	oidc, _ := provider["oidc"].(map[string]any)
	if !ok || oidc["issuerUri"] != issuer || oidc["clientId"] != audience {
		iamResources.Put(consoleWorkforceProvider, map[string]any{
			"name":        consoleWorkforceProvider,
			"displayName": "Single sign-on",
			"state":       "ACTIVE",
			"oidc": map[string]any{
				"issuerUri": issuer,
				"clientId":  audience,
			},
			"attributeMapping": map[string]any{
				"google.subject": "assertion.sub",
			},
		})
	}
	return consoleWorkforceProvider
}
