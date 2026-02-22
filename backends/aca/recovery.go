package aca

import (
	"context"
	"time"

	core "github.com/sockerless/backend-core"
)

// ScanOrphanedResources discovers Sockerless-managed ACA Jobs.
func (s *Server) ScanOrphanedResources(ctx context.Context, instanceID string) ([]core.ResourceEntry, error) {
	var orphans []core.ResourceEntry

	pager := s.azure.Jobs.NewListByResourceGroupPager(s.config.ResourceGroup, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, job := range page.Value {
			if job.Tags == nil {
				continue
			}

			managed := false
			matchesInstance := false
			if v, ok := job.Tags["sockerless-managed"]; ok && v != nil && *v == "true" {
				managed = true
			}
			if v, ok := job.Tags["sockerless-instance"]; ok && v != nil && *v == instanceID {
				matchesInstance = true
			}

			if managed && matchesInstance && job.Name != nil {
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

// CleanupResource deletes an ACA Job.
func (s *Server) CleanupResource(ctx context.Context, entry core.ResourceEntry) error {
	poller, err := s.azure.Jobs.BeginDelete(ctx, s.config.ResourceGroup, entry.ResourceID, nil)
	if err != nil {
		return err
	}
	_, err = poller.PollUntilDone(ctx, nil)
	return err
}
