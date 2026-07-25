package main

// Microsoft.Subscription (2021-10-01) — the ARM surface that creates
// subscriptions programmatically. An "alias" is the tenant-scoped creation
// request: PUT /providers/Microsoft.Subscription/aliases/{aliasName} either
// creates a new subscription under a billing scope or adopts an existing
// subscription id, and the long-running operation completes through
// provisioning-state polling of the alias resource itself (the documented
// model: the 201 body carries properties.provisioningState "Accepted" and a
// GET on the same URL reports "Succeeded"). Rename, cancel, and enable are
// the subscription-scoped POST actions the azurerm provider drives on
// update and destroy. Wire shapes mirror the vendored Swagger
// (specs/cloud-api/azure/subscription-arm-subscriptions-2021-10-01).

import (
	"encoding/json"
	"net/http"
	"sort"
	"time"

	sim "github.com/sockerless/simulator"
)

// SubscriptionAliasRecord mirrors SubscriptionAliasResponse in the
// Microsoft.Subscription 2021-10-01 Swagger (spec field spelling kept).
type SubscriptionAliasRecord struct {
	Name                 string
	SubscriptionID       string
	DisplayName          string
	BillingScope         string
	Workload             string
	ProvisioningState    string
	AcceptOwnershipState string
	CreatedTime          string
}

// AzureSubscriptionRecord backs GET /subscriptions[/{id}] for subscriptions
// created through the alias API or mutated through the Microsoft.Subscription
// rename/cancel/enable actions. Subscriptions without a record keep the
// simulator's implicit identity (display name "Simulator Subscription",
// state Enabled).
type AzureSubscriptionRecord struct {
	SubscriptionID string
	DisplayName    string
	State          string
}

var (
	azureSubscriptionAliases sim.Store[SubscriptionAliasRecord]
	azureSubscriptionRecords sim.Store[AzureSubscriptionRecord]
)

func registerSubscriptionAlias(srv *sim.Server) {
	azureSubscriptionAliases = sim.MakeStore[SubscriptionAliasRecord](srv.DB(), "azure_subscription_aliases")
	azureSubscriptionRecords = sim.MakeStore[AzureSubscriptionRecord](srv.DB(), "azure_subscription_records")

	// PUT - Alias_Create: create a subscription under a billing scope, or
	// adopt an existing subscription id under a new alias. Long-running:
	// 201 with provisioningState "Accepted", terminal via GET on the alias.
	srv.HandleFunc("PUT /providers/Microsoft.Subscription/aliases/{aliasName}", handleSubscriptionAliasCreate)

	// GET - Alias_Get (also the LRO poll target).
	srv.HandleFunc("GET /providers/Microsoft.Subscription/aliases/{aliasName}", func(w http.ResponseWriter, r *http.Request) {
		name := sim.PathParam(r, "aliasName")
		alias, ok := azureSubscriptionAliases.Get(name)
		if !ok {
			subscriptionRPError(w, "NotFound", "The alias '"+name+"' cannot be found.", http.StatusNotFound)
			return
		}
		if alias.ProvisioningState == "Accepted" {
			// Real ARM advertises a poll interval on in-progress operations;
			// a short one keeps SDK pollers from their 30s default.
			w.Header().Set("Retry-After", "1")
		}
		sim.WriteJSON(w, http.StatusOK, subscriptionAliasResponse(alias))
	})

	// DELETE - Alias_Delete: removes the alias only; the subscription it
	// references keeps existing (cancellation is a separate action).
	srv.HandleFunc("DELETE /providers/Microsoft.Subscription/aliases/{aliasName}", func(w http.ResponseWriter, r *http.Request) {
		name := sim.PathParam(r, "aliasName")
		if !azureSubscriptionAliases.Delete(name) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	// GET - Alias_List.
	srv.HandleFunc("GET /providers/Microsoft.Subscription/aliases", func(w http.ResponseWriter, _ *http.Request) {
		aliases := azureSubscriptionAliases.List()
		sort.Slice(aliases, func(i, j int) bool { return aliases[i].Name < aliases[j].Name })
		out := make([]map[string]any, 0, len(aliases))
		for _, alias := range aliases {
			out = append(out, subscriptionAliasResponse(alias))
		}
		sim.WriteJSON(w, http.StatusOK, map[string]any{"value": out})
	})

	// POST - Subscription_Cancel: a cancelled subscription is Disabled, not
	// deleted — it stays enumerable and can be re-enabled.
	srv.HandleFunc("POST /subscriptions/{subscriptionId}/providers/Microsoft.Subscription/cancel", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		setAzureSubscriptionState(sub, "Disabled")
		sim.WriteJSON(w, http.StatusOK, map[string]any{"subscriptionId": sub})
	})

	// POST - Subscription_Enable.
	srv.HandleFunc("POST /subscriptions/{subscriptionId}/providers/Microsoft.Subscription/enable", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		setAzureSubscriptionState(sub, "Enabled")
		sim.WriteJSON(w, http.StatusOK, map[string]any{"subscriptionId": sub})
	})

	// POST - Subscription_Rename.
	srv.HandleFunc("POST /subscriptions/{subscriptionId}/providers/Microsoft.Subscription/rename", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		var req struct {
			SubscriptionName string `json:"subscriptionName"`
		}
		if err := sim.ReadJSON(r, &req); err != nil {
			subscriptionRPError(w, "BadRequest", "Failed to parse request body: "+err.Error(), http.StatusBadRequest)
			return
		}
		if req.SubscriptionName == "" {
			subscriptionRPError(w, "BadRequest", "The request body must carry a non-empty 'subscriptionName'.", http.StatusBadRequest)
			return
		}
		ensureAzureSubscriptionRecord(sub)
		azureSubscriptionRecords.Update(sub, func(rec *AzureSubscriptionRecord) {
			rec.DisplayName = req.SubscriptionName
		})
		sim.WriteJSON(w, http.StatusOK, map[string]any{"subscriptionId": sub})
	})
}

func handleSubscriptionAliasCreate(w http.ResponseWriter, r *http.Request) {
	name := sim.PathParam(r, "aliasName")
	var req struct {
		Properties *struct {
			DisplayName    string `json:"displayName"`
			Workload       string `json:"workload"`
			BillingScope   string `json:"billingScope"`
			SubscriptionID string `json:"subscriptionId"`
		} `json:"properties"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		subscriptionRPError(w, "BadRequest", "Failed to parse request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	if existing, ok := azureSubscriptionAliases.Get(name); ok {
		// PUT on an existing alias is idempotent; retargeting an alias at a
		// different subscription is a conflict.
		if req.Properties != nil && req.Properties.SubscriptionID != "" && req.Properties.SubscriptionID != existing.SubscriptionID {
			subscriptionRPError(w, "Conflict",
				"The alias '"+name+"' already references subscription '"+existing.SubscriptionID+"'.", http.StatusConflict)
			return
		}
		sim.WriteJSON(w, http.StatusOK, subscriptionAliasResponse(existing))
		return
	}

	if req.Properties == nil || (req.Properties.BillingScope == "" && req.Properties.SubscriptionID == "") {
		subscriptionRPError(w, "BadRequest",
			"Creating a subscription requires properties.billingScope; adopting an existing subscription requires properties.subscriptionId.",
			http.StatusBadRequest)
		return
	}
	workload := req.Properties.Workload
	switch workload {
	case "":
		workload = "Production"
	case "Production", "DevTest":
	default:
		subscriptionRPError(w, "BadRequest",
			"The workload '"+workload+"' is invalid. Supported values are 'Production' and 'DevTest'.", http.StatusBadRequest)
		return
	}

	adopting := req.Properties.SubscriptionID != ""
	subscriptionID := req.Properties.SubscriptionID
	displayName := req.Properties.DisplayName
	if adopting {
		// Adoption attaches an alias to a subscription that already exists;
		// its display name is the subscription's, not the request's (rename
		// is the API that changes it).
		displayName, _ = azureSubscriptionView(subscriptionID)
	} else {
		subscriptionID = generateUUID()
	}

	alias := SubscriptionAliasRecord{
		Name:                 name,
		SubscriptionID:       subscriptionID,
		DisplayName:          displayName,
		BillingScope:         req.Properties.BillingScope,
		Workload:             workload,
		ProvisioningState:    "Accepted",
		AcceptOwnershipState: "Pending",
		CreatedTime:          time.Now().UTC().Format(time.RFC3339Nano),
	}
	azureSubscriptionAliases.Put(name, alias)

	go func() {
		// The subscription materializes while the alias provisions — the
		// same async window real subscription creation has.
		time.Sleep(50 * time.Millisecond)
		if adopting {
			ensureAzureSubscriptionRecord(subscriptionID)
		} else {
			azureSubscriptionRecords.Put(subscriptionID, AzureSubscriptionRecord{
				SubscriptionID: subscriptionID,
				DisplayName:    displayName,
				State:          "Enabled",
			})
		}
		azureSubscriptionAliases.Update(name, func(rec *SubscriptionAliasRecord) {
			rec.ProvisioningState = "Succeeded"
			rec.AcceptOwnershipState = "Completed"
		})
	}()

	w.Header().Set("Retry-After", "1")
	sim.WriteJSON(w, http.StatusCreated, subscriptionAliasResponse(alias))
}

func subscriptionAliasResponse(alias SubscriptionAliasRecord) map[string]any {
	props := map[string]any{
		"subscriptionId":       alias.SubscriptionID,
		"displayName":          alias.DisplayName,
		"provisioningState":    alias.ProvisioningState,
		"acceptOwnershipState": alias.AcceptOwnershipState,
		"workload":             alias.Workload,
		"createdTime":          alias.CreatedTime,
	}
	if alias.BillingScope != "" {
		props["billingScope"] = alias.BillingScope
	}
	return map[string]any{
		"id":         "/providers/Microsoft.Subscription/aliases/" + alias.Name,
		"name":       alias.Name,
		"type":       "Microsoft.Subscription/aliases",
		"properties": props,
	}
}

// ensureAzureSubscriptionRecord materializes the stored row for a subscription
// the simulator has so far served implicitly, preserving its current identity.
func ensureAzureSubscriptionRecord(sub string) {
	if _, ok := azureSubscriptionRecords.Get(sub); ok {
		return
	}
	displayName, state := azureSubscriptionView(sub)
	azureSubscriptionRecords.Put(sub, AzureSubscriptionRecord{
		SubscriptionID: sub,
		DisplayName:    displayName,
		State:          state,
	})
}

func setAzureSubscriptionState(sub, state string) {
	ensureAzureSubscriptionRecord(sub)
	azureSubscriptionRecords.Update(sub, func(rec *AzureSubscriptionRecord) {
		rec.State = state
	})
}

// subscriptionRPError writes the Microsoft.Subscription resource provider's
// error envelope: the standard nested ARM error plus the top-level code and
// message its Swagger's ErrorResponseBody declares.
func subscriptionRPError(w http.ResponseWriter, code, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error":   map[string]string{"code": code, "message": message},
		"code":    code,
		"message": message,
	})
}
