package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	sim "github.com/sockerless/simulator"
)

type RoleAssignment struct {
	ID         string                   `json:"id"`
	Name       string                   `json:"name"`
	Type       string                   `json:"type"`
	Properties RoleAssignmentProperties `json:"properties"`
}

type RoleAssignmentProperties struct {
	RoleDefinitionId string `json:"roleDefinitionId"`
	PrincipalId      string `json:"principalId"`
	PrincipalType    string `json:"principalType,omitempty"`
	Scope            string `json:"scope"`
	CreatedOn        string `json:"createdOn,omitempty"`
	UpdatedOn        string `json:"updatedOn,omitempty"`
	CreatedBy        string `json:"createdBy,omitempty"`
}

// parseRoleAssignmentPath extracts the scope and role assignment name from a path like
// /subscriptions/{sub}/providers/Microsoft.Authorization/roleAssignments/{name}
func parseRoleAssignmentPath(path string) (scope, raName string, ok bool) {
	lowerPath := strings.ToLower(path)
	const marker = "/providers/microsoft.authorization/roleassignments/"
	idx := strings.Index(lowerPath, marker)
	if idx < 0 {
		return "", "", false
	}
	scope = path[:idx]
	raName = path[idx+len(marker):]
	if raName == "" || scope == "" {
		return "", "", false
	}
	// Strip query params or trailing slashes from raName
	if i := strings.IndexByte(raName, '?'); i >= 0 {
		raName = raName[:i]
	}
	raName = strings.TrimSuffix(raName, "/")
	return scope, raName, true
}

// builtinRoleDefinitions maps role names to UUIDs for the azurerm provider.
// When role_definition_name is used, the provider looks up the role definition ID.
var builtinRoleDefinitions = map[string]string{
	"Contributor":                              "b24988ac-6180-42a0-ab88-20f7382dd24c",
	"Reader":                                   "acdd72a7-3385-48ef-bd42-f606fba81ae7",
	"Owner":                                    "8e3af657-a8ff-443c-a75c-2fe8c4bcb635",
	"AcrPull":                                  "7f951dda-4ed3-4680-a7ca-43fe172d538d",
	"AcrPush":                                  "8311e382-0749-4cb8-b61a-304f252e45ec",
	"Storage Blob Data Contributor":            "ba92f5b4-2d11-453d-a403-e96b0029c9fe",
	"Storage File Data SMB Share Contributor":  "0c867c2a-1d8c-454a-a3db-ab2ea1bdc8bb",
	"Monitoring Reader":                        "43d0d8ad-25c7-4714-9337-8ba259a9fe05",
	"Log Analytics Contributor":                "92aaf0da-9dab-42b6-94a3-d43ce8d16293",
}

// extractSubscriptionFromPath extracts the subscription ID from an ARM path.
func extractSubscriptionFromPath(path string) string {
	lowerPath := strings.ToLower(path)
	const prefix = "/subscriptions/"
	idx := strings.Index(lowerPath, prefix)
	if idx < 0 {
		return "00000000-0000-0000-0000-000000000000"
	}
	rest := path[idx+len(prefix):]
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		return rest[:i]
	}
	return rest
}

// parseRoleNameFilter extracts the role name from an OData filter like "roleName eq 'Monitoring Reader'".
// Returns empty string if filter is empty or doesn't match the expected pattern.
func parseRoleNameFilter(filter string) string {
	// Expected format: roleName eq 'SomeRole'
	if filter == "" {
		return ""
	}
	// Try single quotes: roleName eq 'Monitoring Reader'
	if idx := strings.Index(filter, "'"); idx >= 0 {
		end := strings.LastIndex(filter, "'")
		if end > idx {
			return filter[idx+1 : end]
		}
	}
	return ""
}

func registerAuthorization(srv *sim.Server) {
	roleAssignments := sim.NewStateStore[RoleAssignment]()

	// Middleware to handle authorization requests at ANY scope level.
	// Go 1.22 mux doesn't support variable-length wildcards in the middle of
	// patterns, but the azurerm provider looks up role definitions and creates
	// role assignments at resource-scoped paths (e.g., on ACR, Storage, etc.).
	// This middleware intercepts all /providers/Microsoft.Authorization/ paths.
	// Path matching is case-insensitive to handle varying SDK casing.
	srv.WrapHandler(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path
			// Normalize double slashes (this middleware runs before CleanPathMiddleware)
			for strings.Contains(path, "//") {
				path = strings.ReplaceAll(path, "//", "/")
			}

			lowerPath := strings.ToLower(path)

			// Role definitions: GET {scope}/providers/Microsoft.Authorization/roleDefinitions[/{id}]
			if r.Method == http.MethodGet && strings.Contains(lowerPath, "/providers/microsoft.authorization/roledefinitions") {
				sub := extractSubscriptionFromPath(path)

				// Extract what comes after "roleDefinitions" — could be empty (list) or "/{id}" (get by ID)
				const roleDefMarker = "/providers/microsoft.authorization/roledefinitions"
				markerIdx := strings.Index(lowerPath, roleDefMarker)
				afterMarker := path[markerIdx+len(roleDefMarker):]
				afterMarker = strings.TrimPrefix(afterMarker, "/")
				if i := strings.IndexByte(afterMarker, '?'); i >= 0 {
					afterMarker = afterMarker[:i]
				}
				afterMarker = strings.TrimSuffix(afterMarker, "/")

				if afterMarker != "" {
					// GET by ID: return single role definition
					roleDefID := afterMarker
					for name, id := range builtinRoleDefinitions {
						if strings.EqualFold(id, roleDefID) {
							fmt.Fprintf(os.Stderr, "AUTHZ: roleDefinition GET-by-ID path=%s id=%s role=%s\n", path, roleDefID, name)
							sim.WriteJSON(w, http.StatusOK, map[string]any{
								"id":   fmt.Sprintf("/subscriptions/%s/providers/Microsoft.Authorization/roleDefinitions/%s", sub, id),
								"name": id,
								"type": "Microsoft.Authorization/roleDefinitions",
								"properties": map[string]any{
									"roleName":         name,
									"type":             "BuiltInRole",
									"description":      name + " role",
									"assignableScopes": []string{"/"},
									"permissions":      []map[string]any{{"actions": []string{"*"}}},
								},
							})
							return
						}
					}
					// Not found
					fmt.Fprintf(os.Stderr, "AUTHZ: roleDefinition GET-by-ID NOT FOUND path=%s id=%s\n", path, roleDefID)
					sim.AzureErrorf(w, "RoleDefinitionNotFound", http.StatusNotFound,
						"Role definition '%s' not found.", roleDefID)
					return
				}

				// LIST: return all matching role definitions
				filter := r.URL.Query().Get("$filter")
				// Parse OData filter: "roleName eq 'Monitoring Reader'" → exact match on "Monitoring Reader"
				filterRoleName := parseRoleNameFilter(filter)

				var defs []map[string]any
				for name, id := range builtinRoleDefinitions {
					if filterRoleName != "" && name != filterRoleName {
						continue
					}
					defs = append(defs, map[string]any{
						"id":   fmt.Sprintf("/subscriptions/%s/providers/Microsoft.Authorization/roleDefinitions/%s", sub, id),
						"name": id,
						"type": "Microsoft.Authorization/roleDefinitions",
						"properties": map[string]any{
							"roleName":         name,
							"type":             "BuiltInRole",
							"description":      name + " role",
							"assignableScopes": []string{"/"},
							"permissions":      []map[string]any{{"actions": []string{"*"}}},
						},
					})
				}

				fmt.Fprintf(os.Stderr, "AUTHZ: roleDefinitions LIST path=%s filter=%q filterRole=%q numDefs=%d\n", path, filter, filterRoleName, len(defs))

				sim.WriteJSON(w, http.StatusOK, map[string]any{
					"value": defs,
				})
				return
			}

			// Role assignments: PUT/GET/DELETE {scope}/providers/Microsoft.Authorization/roleAssignments/{name}
			if strings.Contains(lowerPath, "/providers/microsoft.authorization/roleassignments/") {
				scope, raName, ok := parseRoleAssignmentPath(path)
				if !ok {
					next.ServeHTTP(w, r)
					return
				}

				resourceID := fmt.Sprintf("%s/providers/Microsoft.Authorization/roleAssignments/%s", scope, raName)

				switch r.Method {
				case http.MethodPut:
					var req struct {
						Properties struct {
							RoleDefinitionId string `json:"roleDefinitionId"`
							PrincipalId      string `json:"principalId"`
							PrincipalType    string `json:"principalType"`
						} `json:"properties"`
					}
					if err := sim.ReadJSON(r, &req); err != nil {
						sim.AzureError(w, "InvalidRequestContent", "Failed to parse request body: "+err.Error(), http.StatusBadRequest)
						return
					}

					_, exists := roleAssignments.Get(resourceID)

					ra := RoleAssignment{
						ID:   resourceID,
						Name: raName,
						Type: "Microsoft.Authorization/roleAssignments",
						Properties: RoleAssignmentProperties{
							RoleDefinitionId: req.Properties.RoleDefinitionId,
							PrincipalId:      req.Properties.PrincipalId,
							PrincipalType:    req.Properties.PrincipalType,
							Scope:            scope,
						},
					}
					roleAssignments.Put(resourceID, ra)

					fmt.Fprintf(os.Stderr, "AUTHZ: roleAssignment PUT scope=%s name=%s exists=%v\n", scope, raName, exists)

					if exists {
						sim.WriteJSON(w, http.StatusOK, ra)
					} else {
						sim.WriteJSON(w, http.StatusCreated, ra)
					}

				case http.MethodGet:
					ra, ok := roleAssignments.Get(resourceID)
					if !ok {
						sim.AzureErrorf(w, "RoleAssignmentNotFound", http.StatusNotFound,
							"Role assignment '%s' not found.", raName)
						return
					}
					sim.WriteJSON(w, http.StatusOK, ra)

				case http.MethodDelete:
					ra, ok := roleAssignments.Get(resourceID)
					if !ok {
						sim.AzureErrorf(w, "RoleAssignmentNotFound", http.StatusNotFound,
							"Role assignment '%s' not found.", raName)
						return
					}
					roleAssignments.Delete(resourceID)
					sim.WriteJSON(w, http.StatusOK, ra)

				default:
					next.ServeHTTP(w, r)
				}
				return
			}

			next.ServeHTTP(w, r)
		})
	})
}
