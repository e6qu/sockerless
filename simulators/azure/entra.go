package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
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

// EntraGraphGroup is a standalone group created via POST /v1.0/groups.
type EntraGraphGroup struct {
	ID              string `json:"id"`
	DisplayName     string `json:"displayName"`
	Description     string `json:"description,omitempty"`
	MailNickname    string `json:"mailNickname"`
	SecurityEnabled bool   `json:"securityEnabled"`
	MailEnabled     bool   `json:"mailEnabled"`
}

// entraGroupMembership records one user being a member of one group.
type entraGroupMembership struct {
	GroupID string
	UserID  string
}

var (
	entraUsersStore = sim.NewStateStore[EntraUser]()

	entraActiveOIDMu sync.RWMutex
	entraActiveOID   = entraDefaultUser.OID

	entraGraphGroupStore      = sim.NewStateStore[EntraGraphGroup]()
	entraGroupMembershipStore = sim.NewStateStore[entraGroupMembership]()
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
	return entraDefaultUser
}

// newGraphID returns a random UUID-shaped object ID for Graph resources.
func newGraphID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
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
	// Sim-internal seed endpoints — kept for backward compatibility with tests
	// that predate standard Graph provisioning. Standard provisioning via
	// POST /v1.0/groups, POST /v1.0/users, POST /v1.0/groups/{id}/members/$ref
	// is the preferred path; these endpoints are not present on real Azure.
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

	// Microsoft Graph group management — standard provisioning surface.
	// Real URL base: https://graph.microsoft.com/v1.0
	srv.HandleFunc("POST /v1.0/groups", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			DisplayName     string `json:"displayName"`
			MailNickname    string `json:"mailNickname"`
			Description     string `json:"description,omitempty"`
			SecurityEnabled bool   `json:"securityEnabled"`
			MailEnabled     bool   `json:"mailEnabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sim.AzureError(w, "Request_BadRequest", err.Error(), http.StatusBadRequest)
			return
		}
		if req.DisplayName == "" {
			sim.AzureError(w, "Request_BadRequest", "displayName is required", http.StatusBadRequest)
			return
		}
		grp := EntraGraphGroup{
			ID:              newGraphID(),
			DisplayName:     req.DisplayName,
			Description:     req.Description,
			MailNickname:    req.MailNickname,
			SecurityEnabled: req.SecurityEnabled,
			MailEnabled:     req.MailEnabled,
		}
		entraGraphGroupStore.Put(grp.ID, grp)
		sim.WriteJSON(w, http.StatusCreated, entraGraphGroupJSON(grp))
	})

	srv.HandleFunc("GET /v1.0/groups/{groupId}", func(w http.ResponseWriter, r *http.Request) {
		groupID := sim.PathParam(r, "groupId")
		grp, ok := entraGraphGroupStore.Get(groupID)
		if !ok {
			sim.AzureError(w, "Request_ResourceNotFound", "group not found", http.StatusNotFound)
			return
		}
		sim.WriteJSON(w, http.StatusOK, entraGraphGroupJSON(grp))
	})

	srv.HandleFunc("DELETE /v1.0/groups/{groupId}", func(w http.ResponseWriter, r *http.Request) {
		groupID := sim.PathParam(r, "groupId")
		if !entraGraphGroupStore.Delete(groupID) {
			sim.AzureError(w, "Request_ResourceNotFound", "group not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// Group membership management.
	srv.HandleFunc("POST /v1.0/groups/{groupId}/members/$ref", func(w http.ResponseWriter, r *http.Request) {
		groupID := sim.PathParam(r, "groupId")
		if _, ok := entraGraphGroupStore.Get(groupID); !ok {
			sim.AzureError(w, "Request_ResourceNotFound", "group not found", http.StatusNotFound)
			return
		}
		var req struct {
			ODataID string `json:"@odata.id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sim.AzureError(w, "Request_BadRequest", err.Error(), http.StatusBadRequest)
			return
		}
		// Extract user ID from the @odata.id URL (final path segment).
		parts := strings.Split(strings.TrimRight(req.ODataID, "/"), "/")
		userID := parts[len(parts)-1]
		key := groupID + "/" + userID
		entraGroupMembershipStore.Put(key, entraGroupMembership{GroupID: groupID, UserID: userID})
		w.WriteHeader(http.StatusNoContent)
	})

	srv.HandleFunc("GET /v1.0/groups/{groupId}/members", func(w http.ResponseWriter, r *http.Request) {
		groupID := sim.PathParam(r, "groupId")
		if _, ok := entraGraphGroupStore.Get(groupID); !ok {
			sim.AzureError(w, "Request_ResourceNotFound", "group not found", http.StatusNotFound)
			return
		}
		memberships := entraGroupMembershipStore.Filter(func(m entraGroupMembership) bool {
			return m.GroupID == groupID
		})
		values := make([]map[string]any, 0, len(memberships))
		for _, m := range memberships {
			u := getEntraSimUser(m.UserID)
			values = append(values, map[string]any{
				"@odata.type":       "#microsoft.graph.user",
				"id":                m.UserID,
				"displayName":       u.Name,
				"userPrincipalName": u.PreferredUsername,
			})
		}
		sim.WriteJSON(w, http.StatusOK, map[string]any{
			"@odata.context": "$metadata#directoryObjects",
			"value":          values,
		})
	})

	srv.HandleFunc("DELETE /v1.0/groups/{groupId}/members/{userId}/$ref", func(w http.ResponseWriter, r *http.Request) {
		groupID := sim.PathParam(r, "groupId")
		userID := sim.PathParam(r, "userId")
		key := groupID + "/" + userID
		if !entraGroupMembershipStore.Delete(key) {
			sim.AzureError(w, "Request_ResourceNotFound", "membership not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// Microsoft Graph user management.
	srv.HandleFunc("POST /v1.0/users", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			DisplayName       string `json:"displayName"`
			UserPrincipalName string `json:"userPrincipalName"`
			MailNickname      string `json:"mailNickname"`
			Mail              string `json:"mail,omitempty"`
			AccountEnabled    bool   `json:"accountEnabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sim.AzureError(w, "Request_BadRequest", err.Error(), http.StatusBadRequest)
			return
		}
		if req.DisplayName == "" || req.UserPrincipalName == "" {
			sim.AzureError(w, "Request_BadRequest", "displayName and userPrincipalName are required", http.StatusBadRequest)
			return
		}
		oid := newGraphID()
		user := EntraUser{
			OID:               oid,
			Sub:               oid,
			PreferredUsername: req.UserPrincipalName,
			Name:              req.DisplayName,
			Email:             req.Mail,
			Groups:            []EntraGroup{},
		}
		entraUsersStore.Put(oid, user)
		sim.WriteJSON(w, http.StatusCreated, entraGraphUserJSON(oid, req.DisplayName, req.UserPrincipalName, req.Mail, req.AccountEnabled))
	})

	srv.HandleFunc("GET /v1.0/users/{userId}", func(w http.ResponseWriter, r *http.Request) {
		userID := sim.PathParam(r, "userId")
		u, ok := entraUsersStore.Get(userID)
		if !ok {
			sim.AzureError(w, "Request_ResourceNotFound", "user not found", http.StatusNotFound)
			return
		}
		sim.WriteJSON(w, http.StatusOK, entraGraphUserJSON(u.OID, u.Name, u.PreferredUsername, u.Email, true))
	})

	srv.HandleFunc("DELETE /v1.0/users/{userId}", func(w http.ResponseWriter, r *http.Request) {
		userID := sim.PathParam(r, "userId")
		if !entraUsersStore.Delete(userID) {
			sim.AzureError(w, "Request_ResourceNotFound", "user not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// Microsoft Graph delegated read endpoints.
	// Real URL: https://graph.microsoft.com/v1.0/me/memberOf
	// The sim is configured as the graph endpoint in metadata.go, so requests
	// hit this process. We extract oid from the bearer token to look up the
	// user's group memberships from both the standard provisioning store and
	// the sim-seed path (for backward compatibility).
	srv.HandleFunc("GET /v1.0/me/memberOf", handleGraphMemberOf)
	srv.HandleFunc("GET /v1.0/me/transitiveMemberOf", handleGraphMemberOf)
}

func handleGraphMemberOf(w http.ResponseWriter, r *http.Request) {
	oid, ok := parseOIDFromBearer(r)
	if !ok {
		oid = entraDefaultUser.OID
	}

	seen := map[string]bool{}
	values := []map[string]any{}

	// Standard provisioning path: look up groups from the membership store.
	memberships := entraGroupMembershipStore.Filter(func(m entraGroupMembership) bool {
		return m.UserID == oid
	})
	for _, m := range memberships {
		if seen[m.GroupID] {
			continue
		}
		seen[m.GroupID] = true
		grp, ok := entraGraphGroupStore.Get(m.GroupID)
		if !ok {
			continue
		}
		values = append(values, map[string]any{
			"@odata.type": "#microsoft.graph.group",
			"id":          grp.ID,
			"displayName": grp.DisplayName,
		})
	}

	// Sim-seed path: inline groups on the EntraUser (backward compat).
	user := getEntraSimUser(oid)
	for _, g := range user.Groups {
		if seen[g.ID] {
			continue
		}
		seen[g.ID] = true
		values = append(values, map[string]any{
			"@odata.type": "#microsoft.graph.group",
			"id":          g.ID,
			"displayName": g.DisplayName,
		})
	}

	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"@odata.context": "$metadata#directoryObjects",
		"value":          values,
	})
}

func entraGraphGroupJSON(grp EntraGraphGroup) map[string]any {
	return map[string]any{
		"@odata.context":  "$metadata#groups/$entity",
		"id":              grp.ID,
		"displayName":     grp.DisplayName,
		"description":     grp.Description,
		"mailNickname":    grp.MailNickname,
		"securityEnabled": grp.SecurityEnabled,
		"mailEnabled":     grp.MailEnabled,
	}
}

func entraGraphUserJSON(id, displayName, userPrincipalName, mail string, accountEnabled bool) map[string]any {
	return map[string]any{
		"@odata.context":    "$metadata#users/$entity",
		"id":                id,
		"displayName":       displayName,
		"userPrincipalName": userPrincipalName,
		"mail":              mail,
		"accountEnabled":    accountEnabled,
	}
}
