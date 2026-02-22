package main

import (
	"fmt"
	"net/http"

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
	resourceGroups := sim.NewStateStore[ResourceGroup]()

	// PUT - Create or update resource group
	srv.HandleFunc("PUT /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rgName := sim.PathParam(r, "resourceGroupName")

		var req struct {
			Location string            `json:"location"`
			Tags     map[string]string `json:"tags,omitempty"`
		}
		sim.ReadJSON(r, &req)

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
