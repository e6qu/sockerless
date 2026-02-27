package main

import (
	"crypto/rand"
	"fmt"
	"net/http"
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

// LogEntry represents a stored log entry for the simulator (used for ingestion API).
type LogEntry struct {
	TimeGenerated      string `json:"TimeGenerated"`
	ContainerGroupName string `json:"ContainerGroupName_s,omitempty"`
	Log                string `json:"Log_s,omitempty"`
	Stream             string `json:"Stream_s,omitempty"`
	// AppTraces fields
	Message     string `json:"Message,omitempty"`
	AppRoleName string `json:"AppRoleName,omitempty"`
}

// monitorLogs stores rows keyed by "workspaceID:tableName".
// Package-level so other handlers (e.g., Container Apps) can inject log entries.
var monitorLogs = sim.NewStateStore[[]monitorLogRow]()

// injectContainerAppLog writes a log entry to the ContainerAppConsoleLogs_CL table.
func injectContainerAppLog(jobName, message string) {
	row := monitorLogRow{
		"TimeGenerated":        time.Now().UTC().Format(time.RFC3339),
		"ContainerGroupName_s": jobName,
		"Log_s":                message,
		"Stream_s":             "stdout",
	}
	storeKey := "default:ContainerAppConsoleLogs_CL"
	existing, _ := monitorLogs.Get(storeKey)
	existing = append(existing, row)
	monitorLogs.Put(storeKey, existing)
}

func registerAzureMonitor(srv *sim.Server) {
	workspaces := sim.NewStateStore[Workspace]()

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
		if err := sim.ReadJSON(r, &patch); err != nil {
			sim.AzureError(w, "InvalidRequestContent", "Failed to parse request body: "+err.Error(), http.StatusBadRequest)
			return
		}
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

		parsed := parseKQL(req.Query)

		// Look up the table schema
		columns, ok := kqlTableSchemas[parsed.Table]
		if !ok {
			// Default to ContainerAppConsoleLogs_CL schema
			columns = kqlTableSchemas["ContainerAppConsoleLogs_CL"]
		}

		// Look up log entries for this workspace + table
		storeKey := workspaceID + ":" + parsed.Table
		entries, _ := monitorLogs.Get(storeKey)
		// Also check "default" workspace for backward compat
		if len(entries) == 0 {
			entries, _ = monitorLogs.Get("default:" + parsed.Table)
		}

		var rows [][]any
		for _, row := range entries {
			if !row.matchesFilters(parsed.Filters) {
				continue
			}
			rows = append(rows, row.toRow(columns))
			if parsed.Limit > 0 && len(rows) >= parsed.Limit {
				break
			}
		}

		resp := QueryResponse{
			Tables: []Table{
				{
					Name:    "PrimaryResult",
					Columns: columns,
					Rows:    rows,
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

		now := time.Now().UTC().Format(time.RFC3339)
		for _, e := range entries {
			if e.TimeGenerated == "" {
				e.TimeGenerated = now
			}
			row := monitorLogRow{"TimeGenerated": e.TimeGenerated}
			// Detect table by which fields are populated
			tableName := "ContainerAppConsoleLogs_CL"
			if e.ContainerGroupName != "" {
				row["ContainerGroupName_s"] = e.ContainerGroupName
			}
			if e.Log != "" {
				row["Log_s"] = e.Log
			}
			if e.Stream != "" {
				row["Stream_s"] = e.Stream
			}
			if e.Message != "" || e.AppRoleName != "" {
				tableName = "AppTraces"
				row["Message"] = e.Message
				row["AppRoleName"] = e.AppRoleName
			}

			storeKey := "default:" + tableName
			existing, _ := monitorLogs.Get(storeKey)
			existing = append(existing, row)
			monitorLogs.Put(storeKey, existing)
		}

		w.WriteHeader(http.StatusNoContent)
	})
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
