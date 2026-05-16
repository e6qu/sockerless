package api

import (
	"context"
)

// NetworkDriver provides pluggable container networking. Used by the
// core network driver registry in backends/core/drivers_network*.go.
type NetworkDriver interface {
	Name() string
	Create(ctx context.Context, name string, opts *NetworkCreateRequest) (*NetworkCreateResponse, error)
	Inspect(ctx context.Context, id string) (*Network, error)
	List(ctx context.Context, filters map[string][]string) ([]*Network, error)
	Remove(ctx context.Context, id string) error
	Connect(ctx context.Context, networkID, containerID string, config *EndpointSettings) error
	Disconnect(ctx context.Context, networkID, containerID string) error
	Prune(ctx context.Context, filters map[string][]string) (*NetworkPruneResponse, error)
}

// VolumeDriver / LogDriver / MountSpec — REMOVED 2026-05-07. Both were
// vestigial scaffolds with no implementations or callers. Storage
// backing now lives in backends/core/storage_backing.go::StorageBackingDriver
// .
