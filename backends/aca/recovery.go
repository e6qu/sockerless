package aca

import (
	"context"
	"fmt"
	"time"

	core "github.com/sockerless/backend-core"
)

// ScanOrphanedResources discovers Sockerless-managed ACA resources for the
// given instance. In Job mode these are ACA Jobs; in UseApp mode containers
// are realized as ContainerApps, which must be scanned and cleaned up too or
// they leak (billable) across a backend restart.
func (s *Server) ScanOrphanedResources(ctx context.Context, instanceID string) ([]core.ResourceEntry, error) {
	var orphans []core.ResourceEntry

	if s.config.UseApp {
		if s.azure != nil && s.azure.ContainerApps != nil {
			pager := s.azure.ContainerApps.NewListByResourceGroupPager(s.config.ResourceGroup, nil)
			for pager.More() {
				page, err := pager.NextPage(ctx)
				if err != nil {
					return nil, err
				}
				for _, app := range page.Value {
					if app.Tags == nil || app.Name == nil {
						continue
					}
					if managedByInstance(app.Tags, instanceID) {
						orphans = append(orphans, core.ResourceEntry{
							Backend:      "aca",
							ResourceType: "app",
							ResourceID:   *app.Name,
							InstanceID:   instanceID,
							CreatedAt:    time.Now(),
						})
					}
				}
			}
		}
		return orphans, nil
	}

	pager := s.azure.Jobs.NewListByResourceGroupPager(s.config.ResourceGroup, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, job := range page.Value {
			if job.Tags == nil || job.Name == nil {
				continue
			}
			if managedByInstance(job.Tags, instanceID) {
				orphans = append(orphans, core.ResourceEntry{
					Backend:      "aca",
					ResourceType: "job",
					ResourceID:   *job.Name,
					InstanceID:   instanceID,
					CreatedAt:    time.Now(),
				})
			}
		}
	}

	return orphans, nil
}

// managedByInstance reports whether an Azure resource's tags mark it as a
// sockerless-managed resource belonging to the given backend instance.
func managedByInstance(tags map[string]*string, instanceID string) bool {
	managed := false
	if v, ok := tags["sockerless-managed"]; ok && v != nil && *v == "true" {
		managed = true
	}
	matchesInstance := false
	if v, ok := tags["sockerless-instance"]; ok && v != nil && *v == instanceID {
		matchesInstance = true
	}
	return managed && matchesInstance
}

// CleanupResource deletes an orphaned ACA resource — a ContainerApp in
// UseApp mode, otherwise a Job.
func (s *Server) CleanupResource(ctx context.Context, entry core.ResourceEntry) error {
	switch entry.ResourceType {
	case "app":
		if s.azure == nil || s.azure.ContainerApps == nil {
			return fmt.Errorf("cleanup ACA app %q: ContainerApps client not configured", entry.ResourceID)
		}
		poller, err := s.azure.ContainerApps.BeginDelete(ctx, s.config.ResourceGroup, entry.ResourceID, nil)
		if err != nil {
			return err
		}
		_, err = poller.PollUntilDone(ctx, nil)
		return err
	default:
		poller, err := s.azure.Jobs.BeginDelete(ctx, s.config.ResourceGroup, entry.ResourceID, nil)
		if err != nil {
			return err
		}
		_, err = poller.PollUntilDone(ctx, nil)
		return err
	}
}
