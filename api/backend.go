package api

import "io"

// Backend defines the interface that all backends must implement.
type Backend interface {
	// System
	Info() (*BackendInfo, error)

	// Containers
	ContainerCreate(req *ContainerCreateRequest) (*ContainerCreateResponse, error)
	ContainerInspect(id string) (*Container, error)
	ContainerList(opts ContainerListOptions) ([]*ContainerSummary, error)
	ContainerStart(id string) error
	ContainerStop(id string, timeout *int) error
	ContainerKill(id string, signal string) error
	ContainerRemove(id string, force bool) error
	ContainerLogs(id string, opts ContainerLogsOptions) (io.ReadCloser, error)
	ContainerWait(id string, condition string) (*ContainerWaitResponse, error)
	ContainerAttach(id string, opts ContainerAttachOptions) (io.ReadWriteCloser, error)
	ContainerRestart(id string, timeout *int) error
	ContainerTop(id string, psArgs string) (*ContainerTopResponse, error)
	ContainerPrune(filters map[string][]string) (*ContainerPruneResponse, error)
	ContainerStats(id string, stream bool) (io.ReadCloser, error)
	ContainerRename(id string, newName string) error
	ContainerPause(id string) error
	ContainerUnpause(id string) error

	// Exec
	ExecCreate(containerID string, req *ExecCreateRequest) (*ExecCreateResponse, error)
	ExecStart(id string, opts ExecStartRequest) (io.ReadWriteCloser, error)
	ExecInspect(id string) (*ExecInstance, error)

	// Images
	ImagePull(ref string, auth string) (io.ReadCloser, error)
	ImageInspect(name string) (*Image, error)
	ImageLoad(r io.Reader) (io.ReadCloser, error)
	ImageTag(source string, repo string, tag string) error
	ImageList(opts ImageListOptions) ([]*ImageSummary, error)
	ImageRemove(name string, force bool, prune bool) ([]*ImageDeleteResponse, error)
	ImageHistory(name string) ([]*ImageHistoryEntry, error)
	ImagePrune(filters map[string][]string) (*ImagePruneResponse, error)

	// Auth
	AuthLogin(req *AuthRequest) (*AuthResponse, error)

	// Networks
	NetworkCreate(req *NetworkCreateRequest) (*NetworkCreateResponse, error)
	NetworkList(filters map[string][]string) ([]*Network, error)
	NetworkInspect(id string) (*Network, error)
	NetworkConnect(id string, req *NetworkConnectRequest) error
	NetworkDisconnect(id string, req *NetworkDisconnectRequest) error
	NetworkRemove(id string) error
	NetworkPrune(filters map[string][]string) (*NetworkPruneResponse, error)

	// Volumes
	VolumeCreate(req *VolumeCreateRequest) (*Volume, error)
	VolumeList(filters map[string][]string) (*VolumeListResponse, error)
	VolumeInspect(name string) (*Volume, error)
	VolumeRemove(name string, force bool) error
	VolumePrune(filters map[string][]string) (*VolumePruneResponse, error)

	// System
	SystemEvents(opts EventsOptions) (io.ReadCloser, error)
	SystemDf() (*DiskUsageResponse, error)
}
