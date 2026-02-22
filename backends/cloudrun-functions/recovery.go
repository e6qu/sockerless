package gcf

import (
	"context"
	"fmt"
	"time"

	functionspb "cloud.google.com/go/functions/apiv2/functionspb"
	core "github.com/sockerless/backend-core"
	"google.golang.org/api/iterator"
)

// ScanOrphanedResources discovers Sockerless-managed Cloud Run Functions.
func (s *Server) ScanOrphanedResources(ctx context.Context, instanceID string) ([]core.ResourceEntry, error) {
	var orphans []core.ResourceEntry

	parent := fmt.Sprintf("projects/%s/locations/%s", s.config.Project, s.config.Region)
	it := s.gcp.Functions.ListFunctions(ctx, &functionspb.ListFunctionsRequest{
		Parent: parent,
	})

	for {
		fn, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}

		managed := fn.Labels["sockerless_managed"] == "true"
		matchesInstance := fn.Labels["sockerless_instance"] == instanceID

		if managed && matchesInstance {
			orphans = append(orphans, core.ResourceEntry{
				Backend:      "gcf",
				ResourceType: "function",
				ResourceID:   fn.Name,
				InstanceID:   instanceID,
				CreatedAt:    time.Now(),
			})
		}
	}

	return orphans, nil
}

// CleanupResource deletes a Cloud Run Function.
func (s *Server) CleanupResource(ctx context.Context, entry core.ResourceEntry) error {
	op, err := s.gcp.Functions.DeleteFunction(ctx, &functionspb.DeleteFunctionRequest{
		Name: entry.ResourceID,
	})
	if err != nil {
		return err
	}
	return op.Wait(ctx)
}
