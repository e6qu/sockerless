package main

import (
	"crypto/rand"
	"fmt"
	"net/http"
	"strings"
	"time"

	sim "github.com/sockerless/simulator"
)

// Workspace represents an Azure Log Analytics Workspace.
type Workspace struct {
	ID         string              `json:"id"`
	Name       string              `json:"name"`
	Type       string              `json:"type"`
	Location   string              `json:"location"`
	Tags       map[string]string   `json:"tags,omitempty"`
	Properties WorkspaceProperties `json:"properties"`
}

// WorkspaceProperties holds the properties of a Log Analytics Workspace.
type WorkspaceProperties struct {
	CustomerID        string             `json:"customerId"`
	ProvisioningState string             `json:"provisioningState"`
	RetentionInDays   int                `json:"retentionInDays,omitempty"`
	Sku               *WorkspaceSku      `json:"sku,omitempty"`
	Features          *WorkspaceFeatures `json:"features,omitempty"`
}

// WorkspaceSku holds the SKU of a Log Analytics Workspace.
type WorkspaceSku struct {
	Name string `json:"name"`
}

// WorkspaceFeatures holds workspace feature flags.
// The azurerm provider (go-azure-sdk) dereferences this struct — it must not be nil.
type WorkspaceFeatures struct {
	EnableLogAccessUsingOnlyResourcePermissions *bool `json:"enableLogAccessUsingOnlyResourcePermissions,omitempty"`
	DisableLocalAuth                           *bool `json:"disableLocalAuth,omitempty"`
	EnableDataExport                           *bool `json:"enableDataExport,omitempty"`
	ImmediatePurgeDataOn30Days                 *bool `json:"immediatePurgeDataOn30Days,omitempty"`
}

// QueryRequest holds a KQL query request body.
type QueryRequest struct {
	Query    string `json:"query"`
	Timespan string `json:"timespan,omitempty"`
}

// QueryResponse holds the response for a KQL query.
type QueryResponse struct {
	Tables []Table `json:"tables"`
}

// Table holds a single result table from a KQL query.
type Table struct {
	Name    string   `json:"name"`
	Columns []Column `json:"columns"`
	Rows    [][]any  `json:"rows"`
}

// Column holds a column definition in a query result table.
type Column struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// LogEntry represents a stored log entry for the simulator.
type LogEntry struct {
	TimeGenerated      string `json:"TimeGenerated"`
	ContainerGroupName string `json:"ContainerGroupName_s,omitempty"`
	Log                string `json:"Log_s,omitempty"`
	Stream             string `json:"Stream_s,omitempty"`
}

func registerAzureMonitor(srv *sim.Server) {
	workspaces := sim.NewStateStore[Workspace]()
	logs := sim.NewStateStore[[]LogEntry]()

	const armBase = "/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.OperationalInsights"

	// PUT - Create or update workspace
	srv.HandleFunc("PUT "+armBase+"/workspaces/{workspaceName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		name := sim.PathParam(r, "workspaceName")

		var req Workspace
		if err := sim.ReadJSON(r, &req); err != nil {
			sim.AzureError(w, "InvalidRequestContent", "Failed to parse request body: "+err.Error(), http.StatusBadRequest)
			return
		}

		if req.Location == "" {
			sim.AzureError(w, "InvalidRequestContent", "The 'location' property is required.", http.StatusBadRequest)
			return
		}

		resourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.OperationalInsights/workspaces/%s", sub, rg, name)
		customerID := generateUUID()

		boolTrue := true
		boolFalse := false
		ws := Workspace{
			ID:       resourceID,
			Name:     name,
			Type:     "Microsoft.OperationalInsights/workspaces",
			Location: req.Location,
			Tags:     req.Tags,
			Properties: WorkspaceProperties{
				CustomerID:        customerID,
				ProvisioningState: "Succeeded",
				RetentionInDays:   30,
				Sku:               &WorkspaceSku{Name: "PerGB2018"},
				Features: &WorkspaceFeatures{
					EnableLogAccessUsingOnlyResourcePermissions: &boolTrue,
					DisableLocalAuth:                           &boolFalse,
					EnableDataExport:                           &boolFalse,
					ImmediatePurgeDataOn30Days:                 &boolFalse,
				},
			},
		}

		if req.Properties.RetentionInDays > 0 {
			ws.Properties.RetentionInDays = req.Properties.RetentionInDays
		}

		workspaces.Put(resourceID, ws)

		// go-azure-sdk expects 200 for sync creates
		sim.WriteJSON(w, http.StatusOK, ws)
	})

	// PATCH - Update workspace (azurerm v3 provider sends PATCH after initial create)
	srv.HandleFunc("PATCH "+armBase+"/workspaces/{workspaceName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		name := sim.PathParam(r, "workspaceName")

		resourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.OperationalInsights/workspaces/%s", sub, rg, name)

		ws, ok := workspaces.Get(resourceID)
		if !ok {
			sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound,
				"The Resource 'Microsoft.OperationalInsights/workspaces/%s' under resource group '%s' was not found.", name, rg)
			return
		}

		// Apply partial update from request body
		var patch Workspace
		sim.ReadJSON(r, &patch)
		if patch.Tags != nil {
			ws.Tags = patch.Tags
		}
		if patch.Properties.RetentionInDays > 0 {
			ws.Properties.RetentionInDays = patch.Properties.RetentionInDays
		}
		ws.Properties.ProvisioningState = "Succeeded"
		workspaces.Put(resourceID, ws)

		sim.WriteJSON(w, http.StatusOK, ws)
	})

	// GET - Get workspace
	srv.HandleFunc("GET "+armBase+"/workspaces/{workspaceName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		name := sim.PathParam(r, "workspaceName")

		resourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.OperationalInsights/workspaces/%s", sub, rg, name)

		ws, ok := workspaces.Get(resourceID)
		if !ok {
			sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound,
				"The Resource 'Microsoft.OperationalInsights/workspaces/%s' under resource group '%s' was not found.", name, rg)
			return
		}

		sim.WriteJSON(w, http.StatusOK, ws)
	})

	// POST - Get shared keys (azurerm provider reads these when linking workspace to Container App Environment)
	srv.HandleFunc("POST "+armBase+"/workspaces/{workspaceName}/sharedKeys", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		name := sim.PathParam(r, "workspaceName")

		resourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.OperationalInsights/workspaces/%s", sub, rg, name)

		if _, ok := workspaces.Get(resourceID); !ok {
			sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound,
				"The Resource 'Microsoft.OperationalInsights/workspaces/%s' under resource group '%s' was not found.", name, rg)
			return
		}

		sim.WriteJSON(w, http.StatusOK, map[string]any{
			"primarySharedKey":   "dGVzdHByaW1hcnlrZXkK",
			"secondarySharedKey": "dGVzdHNlY29uZGFyeWtleQo=",
		})
	})

	// DELETE - Delete workspace
	srv.HandleFunc("DELETE "+armBase+"/workspaces/{workspaceName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		name := sim.PathParam(r, "workspaceName")

		resourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.OperationalInsights/workspaces/%s", sub, rg, name)

		if workspaces.Delete(resourceID) {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusNoContent)
		}
	})

	// POST - Execute KQL query (Log Analytics data-plane)
	srv.HandleFunc("POST /v1/workspaces/{workspaceId}/query", func(w http.ResponseWriter, r *http.Request) {
		workspaceID := sim.PathParam(r, "workspaceId")

		var req QueryRequest
		if err := sim.ReadJSON(r, &req); err != nil {
			sim.AzureError(w, "BadArgumentError", "Failed to parse request body: "+err.Error(), http.StatusBadRequest)
			return
		}

		if req.Query == "" {
			sim.AzureError(w, "BadArgumentError", "The 'query' property is required.", http.StatusBadRequest)
			return
		}

		// Look up log entries for this workspace and filter based on simple KQL parsing
		entries, _ := logs.Get(workspaceID)
		rows := queryLogEntries(entries, req.Query)

		resp := QueryResponse{
			Tables: []Table{
				{
					Name: "PrimaryResult",
					Columns: []Column{
						{Name: "TimeGenerated", Type: "datetime"},
						{Name: "ContainerGroupName_s", Type: "string"},
						{Name: "Log_s", Type: "string"},
						{Name: "Stream_s", Type: "string"},
					},
					Rows: rows,
				},
			},
		}

		sim.WriteJSON(w, http.StatusOK, resp)
	})

	// POST - Log ingestion endpoint (simplified)
	srv.HandleFunc("POST /dataCollectionRules/{dcrId}/streams/{streamName}", func(w http.ResponseWriter, r *http.Request) {
		var entries []LogEntry
		if err := sim.ReadJSON(r, &entries); err != nil {
			sim.AzureError(w, "BadArgumentError", "Failed to parse request body: "+err.Error(), http.StatusBadRequest)
			return
		}

		// Add timestamps to entries missing them
		for i := range entries {
			if entries[i].TimeGenerated == "" {
				entries[i].TimeGenerated = time.Now().UTC().Format(time.RFC3339)
			}
		}

		// Store logs keyed by a default workspace ID
		// In a real simulator we would map DCR to workspace; for simplicity store under "default"
		existing, _ := logs.Get("default")
		existing = append(existing, entries...)
		logs.Put("default", existing)

		w.WriteHeader(http.StatusNoContent)
	})
}

// queryLogEntries performs simple KQL-style filtering on log entries.
// It handles basic "where" and "take" clauses.
func queryLogEntries(entries []LogEntry, query string) [][]any {
	var rows [][]any

	// Simple filter: look for 'where ContainerGroupName_s == "value"'
	var filterField, filterValue string
	parts := strings.Split(query, "|")
	limit := -1

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "where ") {
			clause := strings.TrimPrefix(part, "where ")
			// Parse simple equality: field == 'value' or field == "value"
			if idx := strings.Index(clause, "=="); idx > 0 {
				filterField = strings.TrimSpace(clause[:idx])
				val := strings.TrimSpace(clause[idx+2:])
				val = strings.Trim(val, "'\"")
				filterValue = val
			}
		}
		if strings.HasPrefix(part, "take ") {
			fmt.Sscanf(strings.TrimPrefix(part, "take "), "%d", &limit)
		}
		if strings.HasPrefix(part, "limit ") {
			fmt.Sscanf(strings.TrimPrefix(part, "limit "), "%d", &limit)
		}
	}

	for _, e := range entries {
		if filterField != "" && filterValue != "" {
			match := false
			switch filterField {
			case "ContainerGroupName_s":
				match = e.ContainerGroupName == filterValue
			case "Log_s":
				match = e.Log == filterValue
			case "Stream_s":
				match = e.Stream == filterValue
			default:
				match = true // Unknown field, include all
			}
			if !match {
				continue
			}
		}

		rows = append(rows, []any{e.TimeGenerated, e.ContainerGroupName, e.Log, e.Stream})

		if limit > 0 && len(rows) >= limit {
			break
		}
	}

	return rows
}

// generateUUID generates a random UUID string.
func generateUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40 // Version 4
	b[8] = (b[8] & 0x3f) | 0x80 // Variant 1
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
