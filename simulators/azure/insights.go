package main

import (
	"fmt"
	"net/http"

	sim "github.com/sockerless/simulator"
)

// AppInsightsComponent represents an Azure Application Insights component.
type AppInsightsComponent struct {
	ID         string                         `json:"id"`
	Name       string                         `json:"name"`
	Type       string                         `json:"type"`
	Location   string                         `json:"location"`
	Kind       string                         `json:"kind,omitempty"`
	Tags       map[string]string              `json:"tags,omitempty"`
	Properties AppInsightsComponentProperties `json:"properties"`
}

// AppInsightsComponentProperties holds the properties of an Application Insights component.
// App Insights uses PascalCase for some property names (InstrumentationKey, ConnectionString)
// unlike most ARM APIs which use camelCase — the SDK serde reflects this.
type AppInsightsComponentProperties struct {
	ApplicationID      string `json:"ApplicationId,omitempty"`
	InstrumentationKey string `json:"InstrumentationKey,omitempty"`
	ConnectionString   string `json:"ConnectionString,omitempty"`
	ProvisioningState  string `json:"provisioningState"`
}

var azureAppInsightsComponents sim.Store[AppInsightsComponent]

func registerApplicationInsights(srv *sim.Server) {
	components := sim.MakeStore[AppInsightsComponent](srv.DB(), "insights_components")
	azureAppInsightsComponents = components

	const armBase = "/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.Insights"

	// PUT - Create or update component
	srv.HandleFunc("PUT "+armBase+"/components/{componentName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		name := sim.PathParam(r, "componentName")

		var req AppInsightsComponent
		if err := sim.ReadJSON(r, &req); err != nil {
			sim.AzureError(w, "InvalidRequestContent", "Failed to parse request body: "+err.Error(), http.StatusBadRequest)
			return
		}

		if req.Location == "" {
			sim.AzureError(w, "InvalidRequestContent", "The 'location' property is required.", http.StatusBadRequest)
			return
		}

		resourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Insights/components/%s", sub, rg, name)

		kind := req.Kind
		if kind == "" {
			kind = "web"
		}

		// Preserve stable IDs across upserts (real App Insights keeps the same instrumentation key).
		appID := generateUUID()
		instrumentationKey := generateUUID()
		if existing, exists := components.Get(resourceID); exists {
			appID = existing.Properties.ApplicationID
			instrumentationKey = existing.Properties.InstrumentationKey
		}

		comp := AppInsightsComponent{
			ID:       resourceID,
			Name:     name,
			Type:     "Microsoft.Insights/components",
			Location: req.Location,
			Kind:     kind,
			Tags:     req.Tags,
			Properties: AppInsightsComponentProperties{
				ApplicationID:      appID,
				InstrumentationKey: instrumentationKey,
				ConnectionString: fmt.Sprintf(
					"InstrumentationKey=%s;IngestionEndpoint=https://eastus-0.in.applicationinsights.azure.com/;LiveEndpoint=https://eastus.livediagnostics.monitor.azure.com/;ApplicationId=%s",
					instrumentationKey, appID),
				ProvisioningState: "Succeeded",
			},
		}

		components.Put(resourceID, comp)
		// go-azure-sdk expects 200 for sync creates
		sim.WriteJSON(w, http.StatusOK, comp)
	})

	// GET - Get component
	srv.HandleFunc("GET "+armBase+"/components/{componentName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		name := sim.PathParam(r, "componentName")

		resourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Insights/components/%s", sub, rg, name)

		comp, ok := components.Get(resourceID)
		if !ok {
			sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound,
				"The Resource 'Microsoft.Insights/components/%s' under resource group '%s' was not found.", name, rg)
			return
		}

		sim.WriteJSON(w, http.StatusOK, comp)
	})

	// DELETE - Delete component
	srv.HandleFunc("DELETE "+armBase+"/components/{componentName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		name := sim.PathParam(r, "componentName")

		resourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Insights/components/%s", sub, rg, name)

		if components.Delete(resourceID) {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusNoContent)
		}
	})

	// GET/PUT - Billing features (azurerm provider reads then updates after creating a component)
	billingResponse := map[string]any{
		"DataVolumeCap": map[string]any{
			"Cap":                                  100,
			"ResetTime":                            0,
			"StopSendNotificationWhenHitCap":       false,
			"StopSendNotificationWhenHitThreshold": false,
			"WarningThreshold":                     90,
			"MaxHistoryCap":                        500,
		},
		"CurrentBillingFeatures": []string{"Basic"},
	}
	srv.HandleFunc("GET "+armBase+"/components/{componentName}/currentbillingfeatures", func(w http.ResponseWriter, r *http.Request) {
		sim.WriteJSON(w, http.StatusOK, billingResponse)
	})
	srv.HandleFunc("PUT "+armBase+"/components/{componentName}/currentbillingfeatures", func(w http.ResponseWriter, r *http.Request) {
		sim.WriteJSON(w, http.StatusOK, billingResponse)
	})

	// POST - Query Application Insights (data-plane)
	srv.HandleFunc("POST /v1/apps/{appId}/query", func(w http.ResponseWriter, r *http.Request) {
		var req QueryRequest
		if err := sim.ReadJSON(r, &req); err != nil {
			sim.AzureError(w, "BadArgumentError", "Failed to parse request body: "+err.Error(), http.StatusBadRequest)
			return
		}

		if req.Query == "" {
			sim.AzureError(w, "BadArgumentError", "The 'query' property is required.", http.StatusBadRequest)
			return
		}

		// Return empty result set matching the query pattern
		resp := QueryResponse{
			Tables: []Table{
				{
					Name: "PrimaryResult",
					Columns: []Column{
						{Name: "TimeGenerated", Type: "datetime"},
						{Name: "Message", Type: "string"},
					},
					Rows: [][]any{},
				},
			},
		}

		sim.WriteJSON(w, http.StatusOK, resp)
	})
}
