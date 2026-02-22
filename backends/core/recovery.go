package core

import (
	"context"
)

// CloudScanner lists and cleans up Sockerless-managed resources.
type CloudScanner interface {
	// ScanOrphanedResources discovers Sockerless-managed cloud resources
	// for the given instance that are not tracked by the registry.
	ScanOrphanedResources(ctx context.Context, instanceID string) ([]ResourceEntry, error)

	// CleanupResource attempts to delete a cloud resource.
	CleanupResource(ctx context.Context, entry ResourceEntry) error
}

// RecoverOnStartup loads the registry from disk, scans cloud for tagged
// resources not in the registry, and registers them as orphans.
func RecoverOnStartup(ctx context.Context, registry *ResourceRegistry, scanner CloudScanner, instanceID string) error {
	// Load existing registry state
	if err := registry.Load(); err != nil {
		return err
	}

	// Scan cloud for orphaned resources
	orphans, err := scanner.ScanOrphanedResources(ctx, instanceID)
	if err != nil {
		return err
	}

	// Register any discovered orphans that aren't already tracked
	known := make(map[string]bool)
	for _, e := range registry.ListAll() {
		known[e.ResourceID] = true
	}

	for _, o := range orphans {
		if !known[o.ResourceID] {
			registry.Register(o)
		}
	}

	return registry.Save()
}
