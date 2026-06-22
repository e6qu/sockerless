package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

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

// EntraApplication is an Entra (Azure AD) application registration created via
// POST /v1.0/applications. The application's appId is the client identifier a
// service principal references.
type EntraApplication struct {
	ID             string `json:"id"`
	AppID          string `json:"appId"`
	DisplayName    string `json:"displayName"`
	SignInAudience string `json:"signInAudience,omitempty"`
}

// EntraServicePrincipal is the directory object that materializes an
// application (or managed identity) into a tenant. Its id is the principal's
// object ID (OID) — the value RBAC role assignments reference.
type EntraServicePrincipal struct {
	ID                   string                    `json:"id"`
	AppID                string                    `json:"appId"`
	DisplayName          string                    `json:"displayName"`
	ServicePrincipalType string                    `json:"servicePrincipalType"`
	PasswordCredentials  []EntraPasswordCredential `json:"passwordCredentials,omitempty"`
}

// EntraPasswordCredential is one client secret minted via addPassword. Real
// Graph returns the secretText only on creation; subsequent reads omit it.
type EntraPasswordCredential struct {
	KeyID         string `json:"keyId"`
	DisplayName   string `json:"displayName,omitempty"`
	SecretText    string `json:"secretText,omitempty"`
	StartDateTime string `json:"startDateTime,omitempty"`
	EndDateTime   string `json:"endDateTime,omitempty"`
}

var (
	entraUsersStore = sim.NewStateStore[EntraUser]()

	entraActiveOIDMu sync.RWMutex
	entraActiveOID   = entraDefaultUser.OID

	entraGraphGroupStore      = sim.NewStateStore[EntraGraphGroup]()
	entraGroupMembershipStore = sim.NewStateStore[entraGroupMembership]()

	entraApplicationStore      = sim.NewStateStore[EntraApplication]()
	entraServicePrincipalStore = sim.NewStateStore[EntraServicePrincipal]()
)

// entraRegisterServicePrincipal records a service principal in the directory.
// It is the single registration point for both application-backed principals
// (POST /v1.0/servicePrincipals) and managed-identity-backed principals
// (managedidentity.go calls this when a user-assigned identity is created), so
// GET /v1.0/servicePrincipals/{id} resolves either by the principal's object ID.
func entraRegisterServicePrincipal(id, appID, displayName, spType string) {
	entraServicePrincipalStore.Put(id, EntraServicePrincipal{
		ID:                   id,
		AppID:                appID,
		DisplayName:          displayName,
		ServicePrincipalType: spType,
	})
}

// entraUnregisterServicePrincipal removes a service principal — used when a
// managed identity backing one is deleted.
func entraUnregisterServicePrincipal(id string) {
	entraServicePrincipalStore.Delete(id)
}

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

func findEntraUserByUPN(upn string) (EntraUser, bool) {
	upn = strings.TrimSpace(upn)
	if upn == "" {
		return EntraUser{}, false
	}
	if strings.EqualFold(entraDefaultUser.PreferredUsername, upn) {
		return entraDefaultUser, true
	}
	users := entraUsersStore.Filter(func(u EntraUser) bool {
		return strings.EqualFold(u.PreferredUsername, upn)
	})
	if len(users) == 0 {
		return EntraUser{}, false
	}
	return users[0], true
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
		baseURL := azureAuthBaseURL(r)
		values := make([]map[string]any, 0, len(memberships))
		for _, m := range memberships {
			u := getEntraSimUser(m.UserID)
			values = append(values, map[string]any{
				"@odata.type":       "#microsoft.graph.user",
				"@odata.id":         fmt.Sprintf("%s/v1.0/directoryObjects/%s", baseURL, m.UserID),
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
		if _, exists := findEntraUserByUPN(req.UserPrincipalName); exists {
			sim.AzureError(w, "Request_BadRequest", fmt.Sprintf("Another object with the same value for property userPrincipalName already exists: %s", req.UserPrincipalName), http.StatusBadRequest)
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

	// PATCH applies an incremental update to a user. Real Graph PATCH only
	// touches the fields present in the body and returns 204 No Content.
	srv.HandleFunc("PATCH /v1.0/users/{userId}", func(w http.ResponseWriter, r *http.Request) {
		userID := sim.PathParam(r, "userId")
		var req map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sim.AzureError(w, "Request_BadRequest", err.Error(), http.StatusBadRequest)
			return
		}
		updated := entraUsersStore.Update(userID, func(u *EntraUser) {
			if raw, ok := req["displayName"]; ok {
				var v string
				if json.Unmarshal(raw, &v) == nil {
					u.Name = v
				}
			}
			if raw, ok := req["mail"]; ok {
				var v string
				if json.Unmarshal(raw, &v) == nil {
					u.Email = v
				}
			}
		})
		if !updated {
			sim.AzureError(w, "Request_ResourceNotFound", "user not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	srv.HandleFunc("DELETE /v1.0/users/{userId}", func(w http.ResponseWriter, r *http.Request) {
		userID := sim.PathParam(r, "userId")
		if !entraUsersStore.Delete(userID) {
			sim.AzureError(w, "Request_ResourceNotFound", "user not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	registerEntraApplications(srv)
	registerEntraServicePrincipals(srv)

	// Microsoft Graph delegated read endpoints.
	// Real URL: https://graph.microsoft.com/v1.0/me/memberOf
	// The sim is configured as the graph endpoint in metadata.go, so requests
	// hit this process. We extract oid from the bearer token to look up the
	// user's group memberships from both the standard provisioning store and
	// the sim-seed path (for backward compatibility).
	srv.HandleFunc("GET /v1.0/me/memberOf", handleGraphMemberOf)
	srv.HandleFunc("GET /v1.0/me/transitiveMemberOf", handleGraphMemberOf)
}

// registerEntraApplications mounts the Microsoft Graph application-registration
// CRUD surface. Real URL base: https://graph.microsoft.com/v1.0/applications
func registerEntraApplications(srv *sim.Server) {
	srv.HandleFunc("POST /v1.0/applications", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			DisplayName    string `json:"displayName"`
			SignInAudience string `json:"signInAudience"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sim.AzureError(w, "Request_BadRequest", err.Error(), http.StatusBadRequest)
			return
		}
		if req.DisplayName == "" {
			sim.AzureError(w, "Request_BadRequest", "displayName is required", http.StatusBadRequest)
			return
		}
		app := EntraApplication{
			ID:             newGraphID(),
			AppID:          newGraphID(),
			DisplayName:    req.DisplayName,
			SignInAudience: req.SignInAudience,
		}
		entraApplicationStore.Put(app.ID, app)
		sim.WriteJSON(w, http.StatusCreated, entraApplicationJSON(app))
	})

	srv.HandleFunc("GET /v1.0/applications", func(w http.ResponseWriter, r *http.Request) {
		apps := entraApplicationStore.List()
		values := make([]map[string]any, 0, len(apps))
		for _, a := range apps {
			values = append(values, entraApplicationJSON(a))
		}
		sim.WriteJSON(w, http.StatusOK, map[string]any{
			"@odata.context": "$metadata#applications",
			"value":          values,
		})
	})

	srv.HandleFunc("GET /v1.0/applications/{appObjectId}", func(w http.ResponseWriter, r *http.Request) {
		app, ok := entraApplicationStore.Get(sim.PathParam(r, "appObjectId"))
		if !ok {
			sim.AzureError(w, "Request_ResourceNotFound", "application not found", http.StatusNotFound)
			return
		}
		sim.WriteJSON(w, http.StatusOK, entraApplicationJSON(app))
	})

	srv.HandleFunc("PATCH /v1.0/applications/{appObjectId}", func(w http.ResponseWriter, r *http.Request) {
		id := sim.PathParam(r, "appObjectId")
		var req map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sim.AzureError(w, "Request_BadRequest", err.Error(), http.StatusBadRequest)
			return
		}
		updated := entraApplicationStore.Update(id, func(a *EntraApplication) {
			if raw, ok := req["displayName"]; ok {
				var v string
				if json.Unmarshal(raw, &v) == nil {
					a.DisplayName = v
				}
			}
			if raw, ok := req["signInAudience"]; ok {
				var v string
				if json.Unmarshal(raw, &v) == nil {
					a.SignInAudience = v
				}
			}
		})
		if !updated {
			sim.AzureError(w, "Request_ResourceNotFound", "application not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	srv.HandleFunc("DELETE /v1.0/applications/{appObjectId}", func(w http.ResponseWriter, r *http.Request) {
		if !entraApplicationStore.Delete(sim.PathParam(r, "appObjectId")) {
			sim.AzureError(w, "Request_ResourceNotFound", "application not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// registerEntraServicePrincipals mounts the Microsoft Graph service-principal
// CRUD + addPassword surface. Real URL base:
// https://graph.microsoft.com/v1.0/servicePrincipals
func registerEntraServicePrincipals(srv *sim.Server) {
	srv.HandleFunc("POST /v1.0/servicePrincipals", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			AppID       string `json:"appId"`
			DisplayName string `json:"displayName"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sim.AzureError(w, "Request_BadRequest", err.Error(), http.StatusBadRequest)
			return
		}
		if req.AppID == "" {
			sim.AzureError(w, "Request_BadRequest", "appId is required", http.StatusBadRequest)
			return
		}
		displayName := req.DisplayName
		// Graph resolves displayName from the backing application when omitted.
		if displayName == "" {
			apps := entraApplicationStore.Filter(func(a EntraApplication) bool {
				return strings.EqualFold(a.AppID, req.AppID)
			})
			if len(apps) > 0 {
				displayName = apps[0].DisplayName
			}
		}
		sp := EntraServicePrincipal{
			ID:                   newGraphID(),
			AppID:                req.AppID,
			DisplayName:          displayName,
			ServicePrincipalType: "Application",
		}
		entraServicePrincipalStore.Put(sp.ID, sp)
		sim.WriteJSON(w, http.StatusCreated, entraServicePrincipalJSON(sp))
	})

	srv.HandleFunc("GET /v1.0/servicePrincipals", func(w http.ResponseWriter, r *http.Request) {
		appIDFilter := parseGraphEqFilter(r.URL.Query().Get("$filter"), "appId")
		var sps []EntraServicePrincipal
		if appIDFilter != "" {
			sps = entraServicePrincipalStore.Filter(func(sp EntraServicePrincipal) bool {
				return strings.EqualFold(sp.AppID, appIDFilter)
			})
		} else {
			sps = entraServicePrincipalStore.List()
		}
		values := make([]map[string]any, 0, len(sps))
		for _, sp := range sps {
			values = append(values, entraServicePrincipalJSON(sp))
		}
		sim.WriteJSON(w, http.StatusOK, map[string]any{
			"@odata.context": "$metadata#servicePrincipals",
			"value":          values,
		})
	})

	srv.HandleFunc("GET /v1.0/servicePrincipals/{spId}", func(w http.ResponseWriter, r *http.Request) {
		sp, ok := entraServicePrincipalStore.Get(sim.PathParam(r, "spId"))
		if !ok {
			sim.AzureError(w, "Request_ResourceNotFound", "service principal not found", http.StatusNotFound)
			return
		}
		sim.WriteJSON(w, http.StatusOK, entraServicePrincipalJSON(sp))
	})

	srv.HandleFunc("PATCH /v1.0/servicePrincipals/{spId}", func(w http.ResponseWriter, r *http.Request) {
		id := sim.PathParam(r, "spId")
		var req map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sim.AzureError(w, "Request_BadRequest", err.Error(), http.StatusBadRequest)
			return
		}
		updated := entraServicePrincipalStore.Update(id, func(sp *EntraServicePrincipal) {
			if raw, ok := req["displayName"]; ok {
				var v string
				if json.Unmarshal(raw, &v) == nil {
					sp.DisplayName = v
				}
			}
		})
		if !updated {
			sim.AzureError(w, "Request_ResourceNotFound", "service principal not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	srv.HandleFunc("DELETE /v1.0/servicePrincipals/{spId}", func(w http.ResponseWriter, r *http.Request) {
		if !entraServicePrincipalStore.Delete(sim.PathParam(r, "spId")) {
			sim.AzureError(w, "Request_ResourceNotFound", "service principal not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	srv.HandleFunc("POST /v1.0/servicePrincipals/{spId}/addPassword", func(w http.ResponseWriter, r *http.Request) {
		id := sim.PathParam(r, "spId")
		var req struct {
			PasswordCredential struct {
				DisplayName string `json:"displayName"`
			} `json:"passwordCredential"`
		}
		// addPassword accepts an empty body (EOF); a non-empty malformed body is
		// a 400, not silently ignored.
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			sim.AzureError(w, "Request_BadRequest", "invalid request body", http.StatusBadRequest)
			return
		}

		now := time.Now().UTC()
		secret := make([]byte, 32)
		_, _ = rand.Read(secret)
		cred := EntraPasswordCredential{
			KeyID:         newGraphID(),
			DisplayName:   req.PasswordCredential.DisplayName,
			SecretText:    base64.RawURLEncoding.EncodeToString(secret),
			StartDateTime: now.Format(time.RFC3339),
			EndDateTime:   now.AddDate(2, 0, 0).Format(time.RFC3339),
		}
		// Persist the credential without its secretText — Graph only returns
		// secretText on the addPassword response, never on later reads.
		stored := cred
		stored.SecretText = ""
		updated := entraServicePrincipalStore.Update(id, func(sp *EntraServicePrincipal) {
			sp.PasswordCredentials = append(sp.PasswordCredentials, stored)
		})
		if !updated {
			sim.AzureError(w, "Request_ResourceNotFound", "service principal not found", http.StatusNotFound)
			return
		}
		sim.WriteJSON(w, http.StatusOK, map[string]any{
			"keyId":         cred.KeyID,
			"displayName":   cred.DisplayName,
			"secretText":    cred.SecretText,
			"startDateTime": cred.StartDateTime,
			"endDateTime":   cred.EndDateTime,
		})
	})
}

// parseGraphEqFilter extracts the value from a Graph OData filter of the form
// "<field> eq 'value'". Returns "" when the filter doesn't target field.
func parseGraphEqFilter(filter, field string) string {
	if !strings.Contains(strings.ToLower(filter), strings.ToLower(field)) {
		return ""
	}
	if idx := strings.Index(filter, "'"); idx >= 0 {
		if end := strings.IndexByte(filter[idx+1:], '\''); end >= 0 {
			return filter[idx+1 : idx+1+end]
		}
	}
	return ""
}

func entraApplicationJSON(a EntraApplication) map[string]any {
	return map[string]any{
		"@odata.context": "$metadata#applications/$entity",
		"id":             a.ID,
		"appId":          a.AppID,
		"displayName":    a.DisplayName,
		"signInAudience": a.SignInAudience,
	}
}

func entraServicePrincipalJSON(sp EntraServicePrincipal) map[string]any {
	creds := make([]map[string]any, 0, len(sp.PasswordCredentials))
	for _, c := range sp.PasswordCredentials {
		creds = append(creds, map[string]any{
			"keyId":         c.KeyID,
			"displayName":   c.DisplayName,
			"startDateTime": c.StartDateTime,
			"endDateTime":   c.EndDateTime,
		})
	}
	return map[string]any{
		"@odata.context":       "$metadata#servicePrincipals/$entity",
		"id":                   sp.ID,
		"appId":                sp.AppID,
		"displayName":          sp.DisplayName,
		"servicePrincipalType": sp.ServicePrincipalType,
		"passwordCredentials":  creds,
	}
}

func handleGraphMemberOf(w http.ResponseWriter, r *http.Request) {
	oid, ok := parseOIDFromBearer(r)
	if !ok {
		oid = entraDefaultUser.OID
	}

	baseURL := azureAuthBaseURL(r)
	seen := map[string]bool{}
	values := []map[string]any{}

	groupValue := func(id, displayName string) map[string]any {
		return map[string]any{
			"@odata.type": "#microsoft.graph.group",
			"@odata.id":   fmt.Sprintf("%s/v1.0/directoryObjects/%s", baseURL, id),
			"id":          id,
			"displayName": displayName,
		}
	}

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
		values = append(values, groupValue(grp.ID, grp.DisplayName))
	}

	// Sim-seed path: inline groups on the EntraUser (backward compat).
	user := getEntraSimUser(oid)
	for _, g := range user.Groups {
		if seen[g.ID] {
			continue
		}
		seen[g.ID] = true
		values = append(values, groupValue(g.ID, g.DisplayName))
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
