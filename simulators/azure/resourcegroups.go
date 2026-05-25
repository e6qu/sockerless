package main

import (
	"fmt"
	"net/http"
	"strings"

	sim "github.com/sockerless/simulator"
)

type ResourceGroup struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Type       string            `json:"type"`
	Location   string            `json:"location"`
	Tags       map[string]string `json:"tags,omitempty"`
	Properties struct {
		ProvisioningState string `json:"provisioningState"`
	} `json:"properties"`
}

func registerResourceGroups(srv *sim.Server) {
	resourceGroups := sim.MakeStore[ResourceGroup](srv.DB(), "resource_groups")

	// PUT - Create or update resource group
	srv.HandleFunc("PUT /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rgName := sim.PathParam(r, "resourceGroupName")

		var req struct {
			Location string            `json:"location"`
			Tags     map[string]string `json:"tags,omitempty"`
		}
		if err := sim.ReadJSON(r, &req); err != nil {
			sim.AzureError(w, "InvalidRequestContent", "Failed to parse request body: "+err.Error(), http.StatusBadRequest)
			return
		}

		resourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s", sub, rgName)

		_, exists := resourceGroups.Get(resourceID)

		rg := ResourceGroup{
			ID:       resourceID,
			Name:     rgName,
			Type:     "Microsoft.Resources/resourceGroups",
			Location: req.Location,
			Tags:     req.Tags,
		}
		rg.Properties.ProvisioningState = "Succeeded"
		resourceGroups.Put(resourceID, rg)

		if exists {
			sim.WriteJSON(w, http.StatusOK, rg)
		} else {
			sim.WriteJSON(w, http.StatusCreated, rg)
		}
	})

	// GET - Get resource group
	srv.HandleFunc("GET /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rgName := sim.PathParam(r, "resourceGroupName")
		resourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s", sub, rgName)

		rg, ok := resourceGroups.Get(resourceID)
		if !ok {
			sim.AzureErrorf(w, "ResourceGroupNotFound", http.StatusNotFound,
				"Resource group '%s' could not be found.", rgName)
			return
		}
		sim.WriteJSON(w, http.StatusOK, rg)
	})

	// DELETE - Delete resource group
	srv.HandleFunc("DELETE /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rgName := sim.PathParam(r, "resourceGroupName")
		resourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s", sub, rgName)

		resourceGroups.Delete(resourceID)
		w.WriteHeader(http.StatusOK)
	})

	// GET - List resources in resource group (used by azurerm provider during destroy)
	srv.HandleFunc("GET /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/resources", func(w http.ResponseWriter, r *http.Request) {
		// Return empty list — the simulator doesn't track resources globally,
		// each handler manages its own state. An empty list is sufficient for destroy.
		sim.WriteJSON(w, http.StatusOK, map[string]any{
			"value": []any{},
		})
	})

	// GET - List resources in subscription. terraform-provider-azurerm
	// uses this to populate per-subscription caches (e.g. resolving a
	// Key Vault URL → resource ID for azurerm_key_vault_secret on
	// every plan refresh). Real Azure supports a `$filter` query but
	// the sim returns every Key Vault in the subscription regardless
	// — terraform-provider-azurerm's KV-cache logic filters client-
	// side by `properties.vaultUri`, so the broader list is harmless.
	srv.HandleFunc("GET /subscriptions/{subscriptionId}/resources", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		prefix := fmt.Sprintf("/subscriptions/%s/", sub)
		vaults := keyVaults.Filter(func(v KeyVault) bool {
			return strings.HasPrefix(v.ID, prefix)
		})
		values := make([]any, 0, len(vaults))
		for _, v := range vaults {
			values = append(values, v)
		}
		sim.WriteJSON(w, http.StatusOK, map[string]any{"value": values})
	})

	// HEAD - Check resource group existence
	srv.HandleFunc("HEAD /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rgName := sim.PathParam(r, "resourceGroupName")
		resourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s", sub, rgName)

		if _, ok := resourceGroups.Get(resourceID); ok {
			w.WriteHeader(http.StatusNoContent)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	})
}
