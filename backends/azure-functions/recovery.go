package azf

import (
	"context"
	"time"

	core "github.com/sockerless/backend-core"
)

// ScanOrphanedResources discovers Sockerless-managed Azure Function Apps.
func (s *Server) ScanOrphanedResources(ctx context.Context, instanceID string) ([]core.ResourceEntry, error) {
	var orphans []core.ResourceEntry

	pager := s.azure.WebApps.NewListByResourceGroupPager(s.config.ResourceGroup, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, site := range page.Value {
			if site.Tags == nil {
				continue
			}

			managed := false
			matchesInstance := false
			if v, ok := site.Tags["sockerless-managed"]; ok && v != nil && *v == "true" {
				managed = true
			}
			if v, ok := site.Tags["sockerless-instance"]; ok && v != nil && *v == instanceID {
				matchesInstance = true
			}

			if managed && matchesInstance {
				resourceID := ""
				if site.ID != nil {
					resourceID = *site.ID
				}
				orphans = append(orphans, core.ResourceEntry{
					Backend:      "azf",
					ResourceType: "site",
					ResourceID:   resourceID,
					InstanceID:   instanceID,
					CreatedAt:    time.Now(),
				})
			}
		}
	}

	return orphans, nil
}

// CleanupResource deletes an Azure Function App.
func (s *Server) CleanupResource(ctx context.Context, entry core.ResourceEntry) error {
	// Extract function app name from resource ID
	// Resource IDs look like: /subscriptions/.../resourceGroups/.../providers/Microsoft.Web/sites/<name>
	name := ""
	if azfState, ok := findAZFByResourceID(s, entry.ResourceID); ok {
		name = azfState.FunctionAppName
	}
	if name == "" {
		return nil
	}
	_, err := s.azure.WebApps.Delete(ctx, s.config.ResourceGroup, name, nil)
	return err
}

func findAZFByResourceID(s *Server, resourceID string) (AZFState, bool) {
	for _, state := range s.AZF.List() {
		if state.ResourceID == resourceID {
			return state, true
		}
	}
	return AZFState{}, false
}
