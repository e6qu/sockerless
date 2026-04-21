package core

import (
	"context"

	"github.com/sockerless/api"
)

// CloudStateProvider abstracts cloud-side container queries.
// Backends that implement this use the cloud as the source of truth
// for container state, with no local Store.Containers dependency.
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

// CloudImageLister is an optional CloudStateProvider extension for
// backends that can derive `docker images` from their configured cloud
// container registry (ECR, Artifact Registry, ACR, etc.). When a
// provider implements this, BaseServer.ImageList augments the
// in-memory cache with live cloud-registry data so `docker images`
// after a restart reflects what's actually in the registry rather
// than what was pulled this process lifetime/.
type CloudImageLister interface {
	// ListImages returns all images available in the configured cloud
	// registry. The returned summaries should carry the fully-qualified
	// registry URL in RepoTags so that clients can docker-pull them
	// back without further resolution.
	ListImages(ctx context.Context) ([]*api.ImageSummary, error)
}

// CloudPodLister is an optional CloudStateProvider extension for
// backends that can derive pods from cloud actuals (multi-container
// task / app grouped by the sockerless-pod tag). When a provider
// implements this, BaseServer.PodList queries the cloud rather than
// the in-memory Store.Pods registry/.
type CloudPodLister interface {
	// ListPods returns all pods (multi-container groups) the backend
	// knows about in its configured environment, projected into the
	// same shape BaseServer.PodList emits.
	ListPods(ctx context.Context) ([]*api.PodListEntry, error)
}
