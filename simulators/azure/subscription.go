package main

import (
	"fmt"
	"net/http"

	sim "github.com/sockerless/simulator"
)

// resourceProviderNamespaces lists the Azure resource provider namespaces
// that the azurerm provider queries during initialization.
var resourceProviderNamespaces = []string{
	"Microsoft.Authorization",
	"Microsoft.Compute",
	"Microsoft.ContainerRegistry",
	"Microsoft.ContainerService",
	"Microsoft.Insights",
	"Microsoft.ManagedIdentity",
	"Microsoft.App",
	"Microsoft.Network",
	"Microsoft.OperationalInsights",
	"Microsoft.Resources",
	"Microsoft.Storage",
	"Microsoft.Web",
}

func registerSubscription(srv *sim.Server) {
	// GET - Get subscription (for data.azurerm_subscription)
	srv.HandleFunc("GET /subscriptions/{subscriptionId}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")

		sim.WriteJSON(w, http.StatusOK, map[string]any{
			"id":             "/subscriptions/" + sub,
			"subscriptionId": sub,
			"tenantId":       "00000000-0000-0000-0000-000000000000",
			"displayName":    "Simulator Subscription",
			"state":          "Enabled",
			"subscriptionPolicies": map[string]any{
				"locationPlacementId": "Internal_2014-09-01",
				"quotaId":             "Internal_2014-09-01",
				"spendingLimit":       "Off",
			},
		})
	})

	// GET - List resource providers (azurerm populates its provider cache on init)
	srv.HandleFunc("GET /subscriptions/{subscriptionId}/providers", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")

		providers := make([]map[string]any, 0, len(resourceProviderNamespaces))
		for _, ns := range resourceProviderNamespaces {
			providers = append(providers, map[string]any{
				"id":                fmt.Sprintf("/subscriptions/%s/providers/%s", sub, ns),
				"namespace":        ns,
				"registrationState": "Registered",
				"resourceTypes":    []any{},
			})
		}

		sim.WriteJSON(w, http.StatusOK, map[string]any{
			"value": providers,
		})
	})
}
