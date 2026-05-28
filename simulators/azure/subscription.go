package main

import (
	"fmt"
	"net/http"

	sim "github.com/sockerless/simulator"
)

// resourceProviderNamespaces lists the Azure resource provider namespaces
// that the azurerm provider queries during initialization.
var resourceProviderNamespaces = []string{
	"Microsoft.ApiManagement",
	"Microsoft.Authorization",
	"Microsoft.Cache",
	"Microsoft.Compute",
	"Microsoft.ContainerRegistry",
	"Microsoft.ContainerService",
	"Microsoft.DBforPostgreSQL",
	"Microsoft.EventGrid",
	"Microsoft.EventHub",
	"Microsoft.Insights",
	"Microsoft.KeyVault",
	"Microsoft.ManagedIdentity",
	"Microsoft.App",
	"Microsoft.Network",
	"Microsoft.OperationalInsights",
	"Microsoft.Resources",
	"Microsoft.Storage",
	"Microsoft.Web",
}

var azureProviderResourceTypes = map[string][]string{
	"Microsoft.ApiManagement":       {"service"},
	"Microsoft.App":                 {"managedEnvironments", "containerApps", "jobs"},
	"Microsoft.Authorization":       {"roleAssignments"},
	"Microsoft.Cache":               {"Redis", "Redis/firewallRules"},
	"Microsoft.ContainerRegistry":   {"registries"},
	"Microsoft.DBforPostgreSQL":     {"flexibleServers", "flexibleServers/databases", "flexibleServers/firewallRules", "flexibleServers/configurations"},
	"Microsoft.EventGrid":           {"topics", "domains", "domains/topics", "systemTopics", "partnerTopics", "eventSubscriptions"},
	"Microsoft.EventHub":            {"namespaces", "namespaces/eventhubs", "namespaces/eventhubs/consumerGroups", "namespaces/authorizationRules"},
	"Microsoft.Insights":            {"components"},
	"Microsoft.KeyVault":            {"vaults", "vaults/accessPolicies", "deletedVaults"},
	"Microsoft.ManagedIdentity":     {"userAssignedIdentities"},
	"Microsoft.Network":             {"virtualNetworks", "virtualNetworks/subnets", "networkSecurityGroups", "networkSecurityGroups/securityRules", "privateDnsZones"},
	"Microsoft.OperationalInsights": {"workspaces"},
	"Microsoft.Resources":           {"resourceGroups", "resources", "providers"},
	"Microsoft.Storage":             {"storageAccounts", "storageAccounts/blobServices", "storageAccounts/blobServices/containers", "storageAccounts/fileServices", "storageAccounts/fileServices/shares"},
	"Microsoft.Web":                 {"serverfarms", "sites"},
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
				"namespace":         ns,
				"registrationState": "Registered",
				"resourceTypes":     azureProviderResourceTypeEntries(ns),
			})
		}

		sim.WriteJSON(w, http.StatusOK, map[string]any{
			"value": providers,
		})
	})
}

func azureProviderResourceTypeEntries(namespace string) []map[string]any {
	names := azureProviderResourceTypes[namespace]
	entries := make([]map[string]any, 0, len(names))
	for _, name := range names {
		entries = append(entries, map[string]any{
			"resourceType": name,
			"locations":    []string{"eastus", "westeurope"},
			"apiVersions":  []string{"2024-11-01", "2024-04-01-preview", "2023-05-01", "2021-04-01"},
			"capabilities": "SupportsTags",
		})
	}
	return entries
}
