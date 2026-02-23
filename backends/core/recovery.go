package core

import (
	"context"
	"time"

	"github.com/sockerless/api"
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

// ReconstructContainerState rebuilds in-memory Store container entries
// from active registry entries so the backend can track and clean them up.
// Returns the count of reconstructed containers.
func ReconstructContainerState(store *Store, registry *ResourceRegistry) int {
	active := registry.ListActive()
	recovered := 0
	for _, entry := range active {
		if _, exists := store.Containers.Get(entry.ContainerID); exists {
			continue
		}
		name := entry.Metadata["name"]
		if name == "" {
			id := entry.ContainerID
			if len(id) > 12 {
				id = id[:12]
			}
			name = "/" + id
		}
		image := entry.Metadata["image"]
		container := api.Container{
			ID:      entry.ContainerID,
			Name:    name,
			Created: entry.CreatedAt.UTC().Format(time.RFC3339Nano),
			Image:   image,
			State: api.ContainerState{
				Status:    "running",
				Running:   true,
				Pid:       1,
				StartedAt: entry.CreatedAt.UTC().Format(time.RFC3339Nano),
			},
			Config:          api.ContainerConfig{Image: image},
			NetworkSettings: api.NetworkSettings{Networks: make(map[string]*api.EndpointSettings)},
			Mounts:          make([]api.MountPoint, 0),
		}
		store.Containers.Put(entry.ContainerID, container)
		store.ContainerNames.Put(name, entry.ContainerID)
		recovered++
	}
	return recovered
}
