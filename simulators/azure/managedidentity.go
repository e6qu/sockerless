package main

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	sim "github.com/sockerless/simulator"
)

// systemAssignedIdentity is the stable identity the IMDS/MSI endpoint
// represents when a token request carries no user-assigned selector. Real IMDS
// always speaks for the compute resource's own (system-assigned) identity, so
// the sim mints tokens for a fixed identity rather than depending on a
// user-assigned one having been provisioned.
var systemAssignedIdentity = UserAssignedIdentity{
	Properties: IdentityProperties{
		TenantId:    simTenantID,
		PrincipalId: "22222222-2222-2222-2222-222222222222",
		ClientId:    "33333333-3333-3333-3333-333333333333",
	},
}

// resolveTokenIdentity selects the managed identity an IMDS/MSI token request
// targets. IMDS accepts client_id, object_id (the principalId), or mi_res_id
// (the ARM resource ID) to disambiguate a user-assigned identity; absent all
// three it speaks for the resource's system-assigned identity. It reports false
// only when a selector is given but matches no provisioned user-assigned
// identity.
func resolveTokenIdentity(identities sim.Store[UserAssignedIdentity], q url.Values) (UserAssignedIdentity, bool) {
	clientID := strings.TrimSpace(q.Get("client_id"))
	objectID := strings.TrimSpace(q.Get("object_id"))
	miResID := strings.TrimSpace(q.Get("mi_res_id"))

	if clientID != "" || objectID != "" || miResID != "" {
		matches := identities.Filter(func(i UserAssignedIdentity) bool {
			switch {
			case clientID != "":
				return strings.EqualFold(i.Properties.ClientId, clientID)
			case objectID != "":
				return strings.EqualFold(i.Properties.PrincipalId, objectID)
			default:
				return strings.EqualFold(i.ID, miResID)
			}
		})
		if len(matches) == 0 {
			return UserAssignedIdentity{}, false
		}
		return matches[0], true
	}

	return systemAssignedIdentity, true
}

// simTenantID is the single Entra tenant the simulator presents. Real Azure
// resources (managed identities, service principals, role-assignment principals)
// all live in one tenant; the all-zero GUID is reserved/invalid, so the sim uses
// a stable non-zero tenant GUID everywhere it surfaces a tenantId.
const simTenantID = "11111111-1111-1111-1111-111111111111"

// azureManagedIdentities is the package-level handle to the managed-identity
// store so the authorization slice can resolve a role-assignment principalId
// that refers to a created managed identity.
var azureManagedIdentities sim.Store[UserAssignedIdentity]

// managedIdentityPrincipalExists reports whether principalId belongs to a
// managed identity created in this sim.
func managedIdentityPrincipalExists(principalID string) bool {
	if azureManagedIdentities == nil || principalID == "" {
		return false
	}
	found := azureManagedIdentities.Filter(func(i UserAssignedIdentity) bool {
		return strings.EqualFold(i.Properties.PrincipalId, principalID)
	})
	return len(found) > 0
}

type UserAssignedIdentity struct {
	ID         string             `json:"id"`
	Name       string             `json:"name"`
	Type       string             `json:"type"`
	Location   string             `json:"location"`
	Tags       map[string]string  `json:"tags,omitempty"`
	Properties IdentityProperties `json:"properties"`
}

type IdentityProperties struct {
	TenantId          string `json:"tenantId"`
	PrincipalId       string `json:"principalId"`
	ClientId          string `json:"clientId"`
	ProvisioningState string `json:"provisioningState,omitempty"`
}

func registerManagedIdentity(srv *sim.Server) {
	identities := sim.MakeStore[UserAssignedIdentity](srv.DB(), "managed_identities")
	azureManagedIdentities = identities

	// The system-assigned identity the IMDS/MSI endpoint speaks for is always a
	// directory service principal, so its principalId resolves via Graph.
	entraRegisterServicePrincipal(systemAssignedIdentity.Properties.PrincipalId,
		systemAssignedIdentity.Properties.ClientId, "system-assigned", "ManagedIdentity")

	const armBase = "/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.ManagedIdentity"

	// PUT - Create or update managed identity
	srv.HandleFunc("PUT "+armBase+"/userAssignedIdentities/{identityName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		identityName := sim.PathParam(r, "identityName")

		var req struct {
			Location string            `json:"location"`
			Tags     map[string]string `json:"tags,omitempty"`
		}
		if err := sim.ReadJSON(r, &req); err != nil {
			sim.AzureError(w, "InvalidRequestContent", "Failed to parse request body: "+err.Error(), http.StatusBadRequest)
			return
		}

		resourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.ManagedIdentity/userAssignedIdentities/%s",
			sub, rg, identityName)

		identity := UserAssignedIdentity{
			ID:       resourceID,
			Name:     identityName,
			Type:     "Microsoft.ManagedIdentity/userAssignedIdentities",
			Location: req.Location,
			Tags:     req.Tags,
			Properties: IdentityProperties{
				TenantId:    simTenantID,
				PrincipalId: generateUUID(),
				ClientId:    generateUUID(),
			},
		}

		// Preserve existing IDs on update; the createOrUpdate is atomic so a
		// concurrent PUT can't regenerate the principal/client IDs mid-flight.
		exists := identities.Update(resourceID, func(existing *UserAssignedIdentity) {
			identity.Properties.PrincipalId = existing.Properties.PrincipalId
			identity.Properties.ClientId = existing.Properties.ClientId
			*existing = identity
		})
		if !exists {
			identities.Put(resourceID, identity)
		}

		// A managed identity materializes a directory service principal whose
		// object ID equals the identity's principalId, so role assignments and
		// Graph servicePrincipal reads resolve it. Real Azure does this
		// automatically when the identity is created.
		entraRegisterServicePrincipal(identity.Properties.PrincipalId,
			identity.Properties.ClientId, identityName, "ManagedIdentity")

		// Real Azure returns 201 Created for a new identity and 200 OK for an
		// update of an existing one.
		status := http.StatusCreated
		if exists {
			status = http.StatusOK
		}
		sim.WriteJSON(w, status, identity)
	})

	// GET - Get managed identity
	srv.HandleFunc("GET "+armBase+"/userAssignedIdentities/{identityName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		identityName := sim.PathParam(r, "identityName")
		resourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.ManagedIdentity/userAssignedIdentities/%s",
			sub, rg, identityName)

		identity, ok := identities.Get(resourceID)
		if !ok {
			sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound,
				"Identity '%s' not found.", identityName)
			return
		}
		sim.WriteJSON(w, http.StatusOK, identity)
	})

	// IMDS metadata token endpoint. Real Azure exposes managed-identity
	// access tokens via two equivalent paths:
	//   - VMs: http://169.254.169.254/metadata/identity/oauth2/token
	//   - App Service / Container Apps: $IDENTITY_ENDPOINT (e.g.
	//     http://localhost:42356/msi/token), with $IDENTITY_HEADER as a
	//     simple shared-secret to gate the call.
	// Sockerless's runners and any Azure SDK that relies on
	// DefaultAzureCredential / ChainedTokenCredential will hit this
	// endpoint to mint scoped tokens. Backends point their managed
	// containers at <sim-base>/metadata/identity/oauth2/token by setting
	// IDENTITY_ENDPOINT in the function/app env.
	tokenHandler := func(w http.ResponseWriter, r *http.Request) {
		resource := r.URL.Query().Get("resource")
		if resource == "" {
			sim.AzureError(w, "InvalidRequestContent",
				"missing required 'resource' query parameter (audience the token is scoped to)",
				http.StatusBadRequest)
			return
		}

		// Resolve which managed identity is requesting the token. IMDS selects a
		// user-assigned identity by client_id, object_id (principalId), or
		// mi_res_id (the ARM resource ID); absent all three it uses the (single)
		// identity provisioned in this sim. The minted JWT carries that
		// identity's oid/clientId so resource servers can authorize it.
		identity, ok := resolveTokenIdentity(identities, r.URL.Query())
		if !ok {
			sim.AzureError(w, "invalid_request",
				"no managed identity matches the requested client_id/object_id/mi_res_id; create one first (PUT .../userAssignedIdentities/{name})",
				http.StatusBadRequest)
			return
		}

		now := time.Now()
		token, err := mintAzureSimSignedJWT(map[string]any{
			"aud":       resource,
			"iss":       fmt.Sprintf("https://sts.windows.net/%s/", identity.Properties.TenantId),
			"oid":       identity.Properties.PrincipalId,
			"sub":       identity.Properties.PrincipalId,
			"tid":       identity.Properties.TenantId,
			"appid":     identity.Properties.ClientId,
			"client_id": identity.Properties.ClientId,
			"iat":       now.Unix(),
			"nbf":       now.Unix(),
			"exp":       now.Add(time.Hour).Unix(),
			"ver":       "1.0",
		})
		if err != nil {
			sim.AzureError(w, "InternalServerError", err.Error(), http.StatusInternalServerError)
			return
		}

		sim.WriteJSON(w, http.StatusOK, map[string]any{
			"access_token": token,
			"expires_in":   "3600",
			"expires_on":   fmt.Sprintf("%d", now.Unix()+3600),
			"not_before":   fmt.Sprintf("%d", now.Unix()),
			"resource":     resource,
			"token_type":   "Bearer",
			"client_id":    identity.Properties.ClientId,
		})
	}
	srv.HandleFunc("GET /metadata/identity/oauth2/token", tokenHandler)
	// App-Service-style endpoint that container apps inject as
	// IDENTITY_ENDPOINT — same payload, different path.
	srv.HandleFunc("GET /msi/token", tokenHandler)

	// DELETE - Delete managed identity
	srv.HandleFunc("DELETE "+armBase+"/userAssignedIdentities/{identityName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		identityName := sim.PathParam(r, "identityName")
		resourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.ManagedIdentity/userAssignedIdentities/%s",
			sub, rg, identityName)

		if existing, ok := identities.Get(resourceID); ok {
			entraUnregisterServicePrincipal(existing.Properties.PrincipalId)
		}
		identities.Delete(resourceID)
		w.WriteHeader(http.StatusOK)
	})
}
