package main

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	sim "github.com/sockerless/simulator"
)

// EntraGroup is a single Microsoft Entra (Azure AD) security group.
type EntraGroup struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
}

// EntraUser is the seedable identity the sim mints tokens for.
type EntraUser struct {
	OID               string       `json:"oid"`
	Sub               string       `json:"sub"`
	PreferredUsername string       `json:"preferredUsername"`
	Name              string       `json:"name"`
	Email             string       `json:"email,omitempty"`
	Groups            []EntraGroup `json:"groups"`
}

var (
	entraUsersStore = sim.NewStateStore[EntraUser]()

	entraActiveOIDMu sync.RWMutex
	entraActiveOID   = entraDefaultUser.OID
)

// entraDefaultUser is the identity returned when no seed has been applied.
var entraDefaultUser = EntraUser{
	OID:               "test-oid",
	Sub:               "test-sub",
	PreferredUsername: "sockerless-test@example.com",
	Name:              "Sockerless Test User",
	Groups:            []EntraGroup{},
}

// getEntraSimActiveUser returns the currently active sim user, seeded or default.
func getEntraSimActiveUser() EntraUser {
	entraActiveOIDMu.RLock()
	oid := entraActiveOID
	entraActiveOIDMu.RUnlock()
	return getEntraSimUser(oid)
}

// getEntraSimUser looks up a seeded user by oid, falling back to defaults.
func getEntraSimUser(oid string) EntraUser {
	u, ok := entraUsersStore.Get(oid)
	if ok {
		return u
	}
	if oid == entraDefaultUser.OID {
		return entraDefaultUser
	}
	return entraDefaultUser
}

// parseOIDFromBearer decodes the oid claim from a Bearer JWT without signature
// verification — the sim trusts its own tokens internally.
func parseOIDFromBearer(r *http.Request) (string, bool) {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return "", false
	}
	token := strings.TrimPrefix(auth, "Bearer ")
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", false
	}
	var claims struct {
		OID string `json:"oid"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil || claims.OID == "" {
		return "", false
	}
	return claims.OID, true
}

func registerEntra(srv *sim.Server) {
	// Sim-internal seed endpoints — tests call these to configure identity and
	// group membership before exercising the auth flow or Graph endpoints.
	srv.HandleFunc("PUT /sim/v1/entra/users/{oid}", func(w http.ResponseWriter, r *http.Request) {
		oid := sim.PathParam(r, "oid")
		var user EntraUser
		if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
			sim.AzureError(w, "InvalidInput", err.Error(), http.StatusBadRequest)
			return
		}
		user.OID = oid
		if user.Groups == nil {
			user.Groups = []EntraGroup{}
		}
		entraUsersStore.Put(oid, user)
		entraActiveOIDMu.Lock()
		entraActiveOID = oid
		entraActiveOIDMu.Unlock()
		sim.WriteJSON(w, http.StatusOK, user)
	})

	srv.HandleFunc("GET /sim/v1/entra/users/{oid}", func(w http.ResponseWriter, r *http.Request) {
		oid := sim.PathParam(r, "oid")
		sim.WriteJSON(w, http.StatusOK, getEntraSimUser(oid))
	})

	srv.HandleFunc("DELETE /sim/v1/entra/users/{oid}", func(w http.ResponseWriter, r *http.Request) {
		oid := sim.PathParam(r, "oid")
		entraUsersStore.Delete(oid)
		entraActiveOIDMu.Lock()
		if entraActiveOID == oid {
			entraActiveOID = entraDefaultUser.OID
		}
		entraActiveOIDMu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})

	// Microsoft Graph delegated endpoints.
	// Real URL: https://graph.microsoft.com/v1.0/me/memberOf
	// The sim is configured as the graph endpoint in metadata.go, so requests
	// hit this process. We extract oid from the bearer token to look up the
	// seeded user's group memberships.
	srv.HandleFunc("GET /v1.0/me/memberOf", handleGraphMemberOf)
	srv.HandleFunc("GET /v1.0/me/transitiveMemberOf", handleGraphMemberOf)
}

func handleGraphMemberOf(w http.ResponseWriter, r *http.Request) {
	oid, ok := parseOIDFromBearer(r)
	if !ok {
		oid = entraDefaultUser.OID
	}
	user := getEntraSimUser(oid)
	values := make([]map[string]any, len(user.Groups))
	for i, g := range user.Groups {
		values[i] = map[string]any{
			"@odata.type": "#microsoft.graph.group",
			"id":          g.ID,
			"displayName": g.DisplayName,
		}
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"@odata.context": "$metadata#directoryObjects",
		"value":          values,
	})
}
