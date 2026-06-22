package main

import (
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"

	sim "github.com/sockerless/simulator"
)

// guidRe matches a canonical (hyphenated) GUID. Real Azure requires the role
// assignment name to be a GUID and validates that a principalId is a GUID.
var guidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

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

var azureRoleAssignments sim.Store[RoleAssignment]

// parseRoleAssignmentPath extracts the scope and role assignment name from a path like
// subscriptions/{sub}/providers/Microsoft.Authorization/roleAssignments/{name}
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

// builtinRolePermission is the permission set the management-plane RBAC
// definition carries — the four ARM action lists.
type builtinRolePermission struct {
	Actions        []string
	NotActions     []string
	DataActions    []string
	NotDataActions []string
}

// builtinRoleDef is one built-in role definition with its real GUID name and
// its truthful permission set (as documented by Azure's built-in roles).
type builtinRoleDef struct {
	Name        string
	ID          string
	Description string
	Permissions builtinRolePermission
}

// builtinRoleDefs lists the built-in roles the sim serves with their REAL
// definition GUIDs and granular permissions (not a wildcard for every role).
// Permission sets mirror Azure's published built-in role definitions.
var builtinRoleDefs = []builtinRoleDef{
	{
		Name:        "Owner",
		ID:          "8e3af657-a8ff-443c-a75c-2fe8c4bcb635",
		Description: "Grants full access to manage all resources, including the ability to assign roles in Azure RBAC.",
		Permissions: builtinRolePermission{Actions: []string{"*"}},
	},
	{
		Name:        "Contributor",
		ID:          "b24988ac-6180-42a0-ab88-20f7382dd24c",
		Description: "Grants full access to manage all resources, but does not allow you to assign roles in Azure RBAC, manage assignments in Azure Blueprints, or share image galleries.",
		Permissions: builtinRolePermission{
			Actions: []string{"*"},
			NotActions: []string{
				"Microsoft.Authorization/*/Delete",
				"Microsoft.Authorization/*/Write",
				"Microsoft.Authorization/elevateAccess/Action",
				"Microsoft.Blueprint/blueprintAssignments/write",
				"Microsoft.Blueprint/blueprintAssignments/delete",
				"Microsoft.Compute/galleries/share/action",
				"Microsoft.Purview/consents/write",
				"Microsoft.Purview/consents/delete",
			},
		},
	},
	{
		Name:        "Reader",
		ID:          "acdd72a7-3385-48ef-bd42-f606fba81ae7",
		Description: "View all resources, but does not allow you to make any changes.",
		Permissions: builtinRolePermission{Actions: []string{"*/read"}},
	},
	{
		Name:        "User Access Administrator",
		ID:          "18d7d88d-d35e-4fb5-a5c3-7773c20a72d9",
		Description: "Lets you manage user access to Azure resources.",
		Permissions: builtinRolePermission{
			Actions: []string{
				"*/read",
				"Microsoft.Authorization/*",
				"Microsoft.Support/*",
			},
		},
	},
	{
		Name:        "AcrPull",
		ID:          "7f951dda-4ed3-4680-a7ca-43fe172d538d",
		Description: "acr pull",
		Permissions: builtinRolePermission{
			Actions:     []string{"Microsoft.ContainerRegistry/registries/metadata/read"},
			DataActions: []string{"Microsoft.ContainerRegistry/registries/pull/read"},
		},
	},
	{
		Name:        "AcrPush",
		ID:          "8311e382-0749-4cb8-b61a-304f252e45ec",
		Description: "acr push",
		Permissions: builtinRolePermission{
			Actions: []string{"Microsoft.ContainerRegistry/registries/metadata/read"},
			DataActions: []string{
				"Microsoft.ContainerRegistry/registries/pull/read",
				"Microsoft.ContainerRegistry/registries/push/write",
			},
		},
	},
	{
		Name:        "Storage Blob Data Contributor",
		ID:          "ba92f5b4-2d11-453d-a403-e96b0029c9fe",
		Description: "Allows for read, write and delete access to Azure Storage blob containers and data.",
		Permissions: builtinRolePermission{
			Actions: []string{
				"Microsoft.Storage/storageAccounts/blobServices/containers/delete",
				"Microsoft.Storage/storageAccounts/blobServices/containers/read",
				"Microsoft.Storage/storageAccounts/blobServices/containers/write",
				"Microsoft.Storage/storageAccounts/blobServices/generateUserDelegationKey/action",
			},
			DataActions: []string{
				"Microsoft.Storage/storageAccounts/blobServices/containers/blobs/delete",
				"Microsoft.Storage/storageAccounts/blobServices/containers/blobs/read",
				"Microsoft.Storage/storageAccounts/blobServices/containers/blobs/write",
				"Microsoft.Storage/storageAccounts/blobServices/containers/blobs/move/action",
				"Microsoft.Storage/storageAccounts/blobServices/containers/blobs/add/action",
			},
		},
	},
	{
		Name:        "Storage File Data SMB Share Contributor",
		ID:          "0c867c2a-1d8c-454a-a3db-ab2ea1bdc8bb",
		Description: "Allows for read, write, and delete access in Azure Storage file shares over SMB.",
		Permissions: builtinRolePermission{
			Actions: []string{"Microsoft.Storage/storageAccounts/fileServices/shares/read"},
			DataActions: []string{
				"Microsoft.Storage/storageAccounts/fileServices/fileshares/files/read",
				"Microsoft.Storage/storageAccounts/fileServices/fileshares/files/write",
				"Microsoft.Storage/storageAccounts/fileServices/fileshares/files/delete",
			},
		},
	},
	{
		Name:        "Monitoring Reader",
		ID:          "43d0d8ad-25c7-4714-9337-8ba259a9fe05",
		Description: "Can read all monitoring data.",
		Permissions: builtinRolePermission{
			Actions: []string{
				"*/read",
				"Microsoft.OperationalInsights/workspaces/search/action",
			},
		},
	},
	{
		Name:        "Log Analytics Contributor",
		ID:          "92aaf0da-9dab-42b6-94a3-d43ce8d16293",
		Description: "Log Analytics Contributor can read all monitoring data and edit monitoring settings.",
		Permissions: builtinRolePermission{
			Actions: []string{
				"*/read",
				"Microsoft.Automation/automationAccounts/*",
				"Microsoft.Insights/alertRules/*",
				"Microsoft.Insights/diagnosticSettings/*",
				"Microsoft.OperationalInsights/*",
				"Microsoft.Resources/deployments/*",
				"Microsoft.Resources/subscriptions/resourceGroups/read",
			},
		},
	},
}

// builtinRoleDefByID returns the role definition for a definition GUID.
func builtinRoleDefByID(id string) (builtinRoleDef, bool) {
	for _, d := range builtinRoleDefs {
		if strings.EqualFold(d.ID, id) {
			return d, true
		}
	}
	return builtinRoleDef{}, false
}

// roleDefinitionGUID extracts the trailing GUID from a roleDefinitionId, which
// is either a bare GUID or a full ARM path
// (.../providers/Microsoft.Authorization/roleDefinitions/{guid}).
func roleDefinitionGUID(roleDefinitionID string) string {
	id := strings.TrimSuffix(roleDefinitionID, "/")
	if i := strings.LastIndex(id, "/"); i >= 0 {
		id = id[i+1:]
	}
	return id
}

// permissionsJSON renders a built-in role's permission set in the ARM
// roleDefinition `properties.permissions` shape.
func permissionsJSON(p builtinRolePermission) []map[string]any {
	return []map[string]any{{
		"actions":        nonNil(p.Actions),
		"notActions":     nonNil(p.NotActions),
		"dataActions":    nonNil(p.DataActions),
		"notDataActions": nonNil(p.NotDataActions),
	}}
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// roleAssignmentJSON renders a stored role assignment in the ARM response shape.
func roleAssignmentJSON(ra RoleAssignment) map[string]any {
	return map[string]any{
		"id":   ra.ID,
		"name": ra.Name,
		"type": ra.Type,
		"properties": map[string]any{
			"roleDefinitionId": ra.Properties.RoleDefinitionId,
			"principalId":      ra.Properties.PrincipalId,
			"principalType":    ra.Properties.PrincipalType,
			"scope":            ra.Properties.Scope,
		},
	}
}

// roleDefinitionJSON renders a built-in role definition in the ARM
// roleDefinitions response shape.
func roleDefinitionJSON(sub string, d builtinRoleDef) map[string]any {
	return map[string]any{
		"id":   fmt.Sprintf("/subscriptions/%s/providers/Microsoft.Authorization/roleDefinitions/%s", sub, d.ID),
		"name": d.ID,
		"type": "Microsoft.Authorization/roleDefinitions",
		"properties": map[string]any{
			"roleName":         d.Name,
			"type":             "BuiltInRole",
			"description":      d.Description,
			"assignableScopes": []string{"/"},
			"permissions":      permissionsJSON(d.Permissions),
		},
	}
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

// parsePrincipalIdFilter extracts the principal ID from an OData filter like
// "principalId eq 'GUID'". Returns "" if the filter doesn't target principalId.
func parsePrincipalIdFilter(filter string) string {
	if !strings.Contains(strings.ToLower(filter), "principalid") {
		return ""
	}
	if idx := strings.Index(filter, "'"); idx >= 0 {
		if end := strings.IndexByte(filter[idx+1:], '\''); end >= 0 {
			return filter[idx+1 : idx+1+end]
		}
	}
	return ""
}

func registerAuthorization(srv *sim.Server) {
	roleAssignments := sim.MakeStore[RoleAssignment](srv.DB(), "role_assignments")
	azureRoleAssignments = roleAssignments

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
					if d, ok := builtinRoleDefByID(roleDefID); ok {
						fmt.Fprintf(os.Stderr, "AUTHZ: roleDefinition GET-by-ID path=%s id=%s role=%s\n", path, roleDefID, d.Name)
						sim.WriteJSON(w, http.StatusOK, roleDefinitionJSON(sub, d))
						return
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
				for _, d := range builtinRoleDefs {
					if filterRoleName != "" && d.Name != filterRoleName {
						continue
					}
					defs = append(defs, roleDefinitionJSON(sub, d))
				}

				fmt.Fprintf(os.Stderr, "AUTHZ: roleDefinitions LIST path=%s filter=%q filterRole=%q numDefs=%d\n", path, filter, filterRoleName, len(defs))

				sim.WriteJSON(w, http.StatusOK, map[string]any{
					"value": defs,
				})
				return
			}

			// Role assignment LIST: GET {scope}/providers/Microsoft.Authorization/roleAssignments
			// (collection — no trailing /{name}). Honors $filter=principalId eq 'X'
			// and $filter=atScope().
			if r.Method == http.MethodGet {
				const listMarker = "/providers/microsoft.authorization/roleassignments"
				if idx := strings.Index(lowerPath, listMarker); idx >= 0 {
					after := strings.TrimPrefix(path[idx+len(listMarker):], "/")
					if i := strings.IndexByte(after, '?'); i >= 0 {
						after = after[:i]
					}
					after = strings.TrimSuffix(after, "/")
					if after == "" {
						scope := path[:idx]
						filter := r.URL.Query().Get("$filter")
						principalFilter := parsePrincipalIdFilter(filter)
						atScope := strings.Contains(strings.ToLower(filter), "atscope()")

						var values []map[string]any
						for _, ra := range roleAssignments.List() {
							if principalFilter != "" && !strings.EqualFold(ra.Properties.PrincipalId, principalFilter) {
								continue
							}
							if atScope && !strings.EqualFold(ra.Properties.Scope, scope) {
								continue
							}
							values = append(values, roleAssignmentJSON(ra))
						}
						if values == nil {
							values = []map[string]any{}
						}
						fmt.Fprintf(os.Stderr, "AUTHZ: roleAssignments LIST scope=%s filter=%q num=%d\n", scope, filter, len(values))
						sim.WriteJSON(w, http.StatusOK, map[string]any{"value": values})
						return
					}
				}
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
					// Real Azure requires the role assignment name to be a GUID.
					if !guidRe.MatchString(raName) {
						sim.AzureErrorf(w, "InvalidRoleAssignmentName", http.StatusBadRequest,
							"The role assignment name '%s' is not a valid GUID.", raName)
						return
					}

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

					// Validate the role definition exists. Real Azure returns 400
					// RoleDefinitionDoesNotExist for an unknown roleDefinitionId.
					roleDefID := roleDefinitionGUID(req.Properties.RoleDefinitionId)
					if roleDefID == "" {
						sim.AzureErrorf(w, "InvalidRoleDefinitionId", http.StatusBadRequest,
							"The role definition ID '%s' is malformed.", req.Properties.RoleDefinitionId)
						return
					}
					if _, ok := builtinRoleDefByID(roleDefID); !ok {
						sim.AzureErrorf(w, "RoleDefinitionDoesNotExist", http.StatusBadRequest,
							"The specified role definition with ID '%s' does not exist.", roleDefID)
						return
					}

					// Validate the principal is a well-formed GUID. Real Azure
					// rejects a malformed principalId; it cannot, however, be the
					// authority on every valid external principal (users, groups,
					// service principals, managed identities across the tenant),
					// so the sim does not hard-fail a syntactically-valid GUID it
					// hasn't itself provisioned — that would reject legitimate
					// assignments to principals created out of band. principalExists
					// (Entra object or managed identity) is used to set the
					// principalType when the sim does know the principal.
					if !guidRe.MatchString(req.Properties.PrincipalId) {
						sim.AzureErrorf(w, "InvalidPrincipalId", http.StatusBadRequest,
							"The principal ID '%s' is not a valid GUID.", req.Properties.PrincipalId)
						return
					}

					principalType := req.Properties.PrincipalType
					if principalType == "" && managedIdentityPrincipalExists(req.Properties.PrincipalId) {
						principalType = "ServicePrincipal"
					}

					_, exists := roleAssignments.Get(resourceID)

					ra := RoleAssignment{
						ID:   resourceID,
						Name: raName,
						Type: "Microsoft.Authorization/roleAssignments",
						Properties: RoleAssignmentProperties{
							RoleDefinitionId: req.Properties.RoleDefinitionId,
							PrincipalId:      req.Properties.PrincipalId,
							PrincipalType:    principalType,
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
