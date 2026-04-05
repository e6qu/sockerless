package core

import (
	"context"

	"github.com/sockerless/api"
)

// CloudStateProvider abstracts cloud-side container queries.
// Backends that implement this use the cloud as the source of truth
// for container state, with no local Store.Containers dependency.
//
// When BaseServer.CloudState is nil, the local Store is used (backward compatible).
type CloudStateProvider interface {
	// GetContainer returns a container by sockerless ID, Docker name, or short ID prefix.
	// The returned container has accurate state from the cloud provider.
	GetContainer(ctx context.Context, ref string) (api.Container, bool, error)

	// ListContainers returns all containers matching the given options.
	// Applies Docker-style filters (name, label, status, id, before, since).
	ListContainers(ctx context.Context, all bool, filters map[string][]string) ([]api.Container, error)

	// CheckNameAvailable returns true if no cloud resource uses the given Docker name.
	CheckNameAvailable(ctx context.Context, name string) (bool, error)

	// WaitForExit blocks until the container reaches a stopped state.
	// Returns the exit code. Respects context cancellation.
	WaitForExit(ctx context.Context, containerID string) (int, error)
}
