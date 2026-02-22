package api

import (
	"context"
	"io"
	"time"
)

// VolumeDriver provides pluggable volume storage.
type VolumeDriver interface {
	Name() string
	Create(ctx context.Context, name string, labels map[string]string, opts map[string]string) (*Volume, error)
	Inspect(ctx context.Context, name string) (*Volume, error)
	List(ctx context.Context, filters map[string][]string) ([]*Volume, error)
	Remove(ctx context.Context, name string, force bool) error
	// MountSpec returns cloud-specific mount config for attaching this volume to a container.
	MountSpec(ctx context.Context, name string, target string) (*MountSpec, error)
}

// NetworkDriver provides pluggable container networking.
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

// LogDriver provides pluggable log storage and retrieval.
type LogDriver interface {
	Name() string
	Write(ctx context.Context, containerID string, stream byte, data []byte, ts time.Time) error
	Read(ctx context.Context, containerID string, opts ContainerLogsOptions) (io.ReadCloser, error)
}

// MountSpec describes how to mount a volume for a specific cloud platform.
type MountSpec struct {
	Type    string            `json:"type"`    // "efs", "gcsfuse", "azurefile", etc.
	Source  string            `json:"source"`  // Cloud resource identifier
	Target  string            `json:"target"`  // Container mount path
	Options map[string]string `json:"options"` // Platform-specific mount options
}
