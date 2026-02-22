package cloudrun

import (
	"context"
	"time"

	runpb "cloud.google.com/go/run/apiv2/runpb"
	core "github.com/sockerless/backend-core"
	"google.golang.org/api/iterator"
)

// ScanOrphanedResources discovers Sockerless-managed Cloud Run Jobs.
func (s *Server) ScanOrphanedResources(ctx context.Context, instanceID string) ([]core.ResourceEntry, error) {
	var orphans []core.ResourceEntry

	it := s.gcp.Jobs.ListJobs(ctx, &runpb.ListJobsRequest{
		Parent: s.buildJobParent(),
	})

	for {
		job, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}

		managed := job.Labels["sockerless_managed"] == "true"
		matchesInstance := job.Labels["sockerless_instance"] == instanceID

		if managed && matchesInstance {
			orphans = append(orphans, core.ResourceEntry{
				Backend:      "cloudrun",
				ResourceType: "job",
				ResourceID:   job.Name,
				InstanceID:   instanceID,
				CreatedAt:    time.Now(),
			})
		}
	}

	return orphans, nil
}

// CleanupResource deletes a Cloud Run Job.
func (s *Server) CleanupResource(ctx context.Context, entry core.ResourceEntry) error {
	op, err := s.gcp.Jobs.DeleteJob(ctx, &runpb.DeleteJobRequest{
		Name: entry.ResourceID,
	})
	if err != nil {
		return err
	}
	_, err = op.Wait(ctx)
	return err
}
