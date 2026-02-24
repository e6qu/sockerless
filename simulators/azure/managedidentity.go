package main

import (
	"fmt"
	"net/http"

	sim "github.com/sockerless/simulator"
)

type UserAssignedIdentity struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Type       string            `json:"type"`
	Location   string            `json:"location"`
	Tags       map[string]string `json:"tags,omitempty"`
	Properties IdentityProperties `json:"properties"`
}

type IdentityProperties struct {
	TenantId          string `json:"tenantId"`
	PrincipalId       string `json:"principalId"`
	ClientId          string `json:"clientId"`
	ProvisioningState string `json:"provisioningState,omitempty"`
}

func registerManagedIdentity(srv *sim.Server) {
	identities := sim.NewStateStore[UserAssignedIdentity]()

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

		existing, exists := identities.Get(resourceID)

		identity := UserAssignedIdentity{
			ID:       resourceID,
			Name:     identityName,
			Type:     "Microsoft.ManagedIdentity/userAssignedIdentities",
			Location: req.Location,
			Tags:     req.Tags,
			Properties: IdentityProperties{
				TenantId:    "00000000-0000-0000-0000-000000000000",
				PrincipalId: generateUUID(),
				ClientId:    generateUUID(),
			},
		}

		// Preserve existing IDs on update
		if exists {
			identity.Properties.PrincipalId = existing.Properties.PrincipalId
			identity.Properties.ClientId = existing.Properties.ClientId
		}

		identities.Put(resourceID, identity)

		// go-azure-sdk expects 200 for sync creates
		sim.WriteJSON(w, http.StatusOK, identity)
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

	// DELETE - Delete managed identity
	srv.HandleFunc("DELETE "+armBase+"/userAssignedIdentities/{identityName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		identityName := sim.PathParam(r, "identityName")
		resourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.ManagedIdentity/userAssignedIdentities/%s",
			sub, rg, identityName)

		identities.Delete(resourceID)
		w.WriteHeader(http.StatusOK)
	})
}
