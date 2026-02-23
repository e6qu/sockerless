package api

// Container represents a container's full state (inspect response).
type Container struct {
	ID              string           `json:"Id"`
	Name            string           `json:"Name"`
	Created         string           `json:"Created"`
	Path            string           `json:"Path"`
	Args            []string         `json:"Args"`
	State           ContainerState   `json:"State"`
	Image           string           `json:"Image"`
	Config          ContainerConfig  `json:"Config"`
	HostConfig      HostConfig       `json:"HostConfig"`
	NetworkSettings NetworkSettings  `json:"NetworkSettings"`
	Mounts          []MountPoint     `json:"Mounts"`
	Platform        string           `json:"Platform"`
	Driver          string           `json:"Driver"`
	RestartCount    int              `json:"RestartCount"`
	LogPath         string           `json:"LogPath"`
	ResolvConfPath  string           `json:"ResolvConfPath"`
	HostnamePath    string           `json:"HostnamePath"`
	HostsPath       string           `json:"HostsPath"`
	ExecIDs         []string         `json:"ExecIDs"`
	AgentAddress    string           `json:"AgentAddress,omitempty"`
	AgentToken      string           `json:"AgentToken,omitempty"`
}

// ContainerState holds the current state of a container.
type ContainerState struct {
	Status     string       `json:"Status"`
	Running    bool         `json:"Running"`
	Paused     bool         `json:"Paused"`
	Restarting bool         `json:"Restarting"`
	OOMKilled  bool         `json:"OOMKilled"`
	Dead       bool         `json:"Dead"`
	Pid        int          `json:"Pid"`
	ExitCode   int          `json:"ExitCode"`
	Error      string       `json:"Error"`
	StartedAt  string       `json:"StartedAt"`
	FinishedAt string       `json:"FinishedAt"`
	Health     *HealthState `json:"Health,omitempty"`
}

// ContainerConfig holds container configuration.
type ContainerConfig struct {
	Hostname     string              `json:"Hostname"`
	Domainname   string              `json:"Domainname"`
	User         string              `json:"User"`
	AttachStdin  bool                `json:"AttachStdin"`
	AttachStdout bool                `json:"AttachStdout"`
	AttachStderr bool                `json:"AttachStderr"`
	ExposedPorts map[string]struct{} `json:"ExposedPorts,omitempty"`
	Tty          bool                `json:"Tty"`
	OpenStdin    bool                `json:"OpenStdin"`
	StdinOnce    bool                `json:"StdinOnce"`
	Env          []string            `json:"Env"`
	Cmd          []string            `json:"Cmd"`
	Image        string              `json:"Image"`
	Volumes      map[string]struct{} `json:"Volumes,omitempty"`
	WorkingDir   string              `json:"WorkingDir"`
	Entrypoint   []string            `json:"Entrypoint"`
	Labels       map[string]string   `json:"Labels"`
	StopSignal   string              `json:"StopSignal,omitempty"`
	StopTimeout  *int                `json:"StopTimeout,omitempty"`
	Healthcheck  *HealthcheckConfig  `json:"Healthcheck,omitempty"`
}

// HostConfig holds host-specific container configuration.
type HostConfig struct {
	NetworkMode   string                      `json:"NetworkMode"`
	Binds         []string                    `json:"Binds,omitempty"`
	AutoRemove    bool                        `json:"AutoRemove"`
	PortBindings  map[string][]PortBinding    `json:"PortBindings,omitempty"`
	RestartPolicy RestartPolicy               `json:"RestartPolicy"`
	// Fields below are accepted, stored, and returned on inspect — but not enforced.
	Privileged  bool              `json:"Privileged,omitempty"`
	CapAdd      []string          `json:"CapAdd,omitempty"`
	CapDrop     []string          `json:"CapDrop,omitempty"`
	Init        *bool             `json:"Init,omitempty"`
	UsernsMode  string            `json:"UsernsMode,omitempty"`
	ShmSize     int64             `json:"ShmSize,omitempty"`
	Tmpfs       map[string]string `json:"Tmpfs,omitempty"`
	SecurityOpt []string          `json:"SecurityOpt,omitempty"`
	LogConfig   *LogConfig        `json:"LogConfig,omitempty"`
	ExtraHosts  []string          `json:"ExtraHosts,omitempty"`
	Mounts      []Mount           `json:"Mounts,omitempty"`
	Isolation   string            `json:"Isolation,omitempty"`
}

// PortBinding represents a port binding between the host and a container.
type PortBinding struct {
	HostIP   string `json:"HostIp"`
	HostPort string `json:"HostPort"`
}

// RestartPolicy holds the restart policy for a container.
type RestartPolicy struct {
	Name              string `json:"Name"`
	MaximumRetryCount int    `json:"MaximumRetryCount"`
}

// LogConfig holds the logging configuration for a container.
type LogConfig struct {
	Type   string            `json:"Type"`
	Config map[string]string `json:"Config,omitempty"`
}

// Mount represents a mount configuration for container create.
type Mount struct {
	Type        string       `json:"Type"`
	Source      string       `json:"Source"`
	Target      string       `json:"Target"`
	ReadOnly    bool         `json:"ReadOnly,omitempty"`
	Consistency string       `json:"Consistency,omitempty"`
	BindOptions *BindOptions `json:"BindOptions,omitempty"`
}

// BindOptions holds options for bind mounts.
type BindOptions struct {
	Propagation string `json:"Propagation,omitempty"`
}

// NetworkSettings holds the network settings for a container.
type NetworkSettings struct {
	Networks map[string]*EndpointSettings `json:"Networks"`
	Ports    map[string][]PortBinding     `json:"Ports,omitempty"`
}

// EndpointSettings holds the endpoint settings for a container on a network.
type EndpointSettings struct {
	NetworkID  string `json:"NetworkID"`
	EndpointID string `json:"EndpointID"`
	Gateway    string `json:"Gateway"`
	IPAddress  string `json:"IPAddress"`
	IPPrefixLen int   `json:"IPPrefixLen"`
	MacAddress string   `json:"MacAddress"`
	Aliases    []string `json:"Aliases,omitempty"`
}

// MountPoint represents a mount point in a container.
type MountPoint struct {
	Type        string `json:"Type"`
	Name        string `json:"Name,omitempty"`
	Source      string `json:"Source"`
	Destination string `json:"Destination"`
	Driver      string `json:"Driver,omitempty"`
	Mode        string `json:"Mode"`
	RW          bool   `json:"RW"`
	Propagation string `json:"Propagation,omitempty"`
}

// ContainerSummary is the short form returned by container list.
type ContainerSummary struct {
	ID              string            `json:"Id"`
	Names           []string          `json:"Names"`
	Image           string            `json:"Image"`
	ImageID         string            `json:"ImageID"`
	Command         string            `json:"Command"`
	Created         int64             `json:"Created"`
	State           string            `json:"State"`
	Status          string            `json:"Status"`
	Ports           []Port            `json:"Ports"`
	Labels          map[string]string `json:"Labels"`
	SizeRw          int64             `json:"SizeRw,omitempty"`
	NetworkSettings *SummaryNetworkSettings `json:"NetworkSettings,omitempty"`
	Mounts          []MountPoint      `json:"Mounts"`
}

// Port represents a port exposed by a container.
type Port struct {
	IP          string `json:"IP,omitempty"`
	PrivatePort uint16 `json:"PrivatePort"`
	PublicPort  uint16 `json:"PublicPort,omitempty"`
	Type        string `json:"Type"`
}

// SummaryNetworkSettings holds the network settings in a container list response.
type SummaryNetworkSettings struct {
	Networks map[string]*EndpointSettings `json:"Networks"`
}

// ContainerCreateRequest is the request body for creating a container.
type ContainerCreateRequest struct {
	*ContainerConfig
	HostConfig       *HostConfig       `json:"HostConfig,omitempty"`
	NetworkingConfig *NetworkingConfig  `json:"NetworkingConfig,omitempty"`
}

// NetworkingConfig holds the networking configuration for container creation.
type NetworkingConfig struct {
	EndpointsConfig map[string]*EndpointSettings `json:"EndpointsConfig,omitempty"`
}

// ContainerCreateResponse is the response from creating a container.
type ContainerCreateResponse struct {
	ID       string   `json:"Id"`
	Warnings []string `json:"Warnings"`
}

// ContainerListOptions are the options for listing containers.
type ContainerListOptions struct {
	All     bool                `json:"All"`
	Limit   int                 `json:"Limit,omitempty"`
	Filters map[string][]string `json:"Filters,omitempty"`
}

// ContainerLogsOptions are the options for container logs.
type ContainerLogsOptions struct {
	ShowStdout bool   `json:"ShowStdout"`
	ShowStderr bool   `json:"ShowStderr"`
	Follow     bool   `json:"Follow"`
	Timestamps bool   `json:"Timestamps"`
	Tail       string `json:"Tail"`
	Since      string `json:"Since"`
	Until      string `json:"Until"`
}

// ContainerWaitResponse is the response from waiting on a container.
type ContainerWaitResponse struct {
	StatusCode int        `json:"StatusCode"`
	Error      *WaitError `json:"Error,omitempty"`
}

// WaitError holds an error message from container wait.
type WaitError struct {
	Message string `json:"Message"`
}

// ContainerAttachOptions are the options for attaching to a container.
type ContainerAttachOptions struct {
	Stream     bool   `json:"Stream"`
	Stdin      bool   `json:"Stdin"`
	Stdout     bool   `json:"Stdout"`
	Stderr     bool   `json:"Stderr"`
	DetachKeys string `json:"DetachKeys,omitempty"`
	Logs       bool   `json:"Logs"`
}

// ExecInstance represents an exec instance.
type ExecInstance struct {
	ID            string            `json:"ID"`
	ContainerID   string            `json:"ContainerID"`
	Running       bool              `json:"Running"`
	ExitCode      int               `json:"ExitCode"`
	Pid           int               `json:"Pid"`
	OpenStdin     bool              `json:"OpenStdin"`
	OpenStdout    bool              `json:"OpenStdout"`
	OpenStderr    bool              `json:"OpenStderr"`
	ProcessConfig ExecProcessConfig `json:"ProcessConfig"`
	CanRemove     bool              `json:"CanRemove"`
}

// ExecProcessConfig holds the process configuration for an exec instance.
type ExecProcessConfig struct {
	Tty        bool     `json:"tty"`
	Entrypoint string   `json:"entrypoint"`
	Arguments  []string `json:"arguments"`
	Privileged bool     `json:"privileged"`
	User       string   `json:"user"`
	Env        []string `json:"env,omitempty"`
	WorkingDir string   `json:"workingDir,omitempty"`
}

// ExecCreateRequest is the request to create an exec instance.
type ExecCreateRequest struct {
	AttachStdin  bool     `json:"AttachStdin"`
	AttachStdout bool     `json:"AttachStdout"`
	AttachStderr bool     `json:"AttachStderr"`
	Tty          bool     `json:"Tty"`
	Cmd          []string `json:"Cmd"`
	Env          []string `json:"Env,omitempty"`
	WorkingDir   string   `json:"WorkingDir,omitempty"`
	User         string   `json:"User,omitempty"`
	Privileged   bool     `json:"Privileged"`
	DetachKeys   string   `json:"DetachKeys,omitempty"`
}

// ExecCreateResponse is the response from creating an exec instance.
type ExecCreateResponse struct {
	ID string `json:"Id"`
}

// ExecStartRequest is the request to start an exec instance.
type ExecStartRequest struct {
	Detach bool `json:"Detach"`
	Tty    bool `json:"Tty"`
}

// Image represents an image.
type Image struct {
	ID            string           `json:"Id"`
	RepoTags      []string         `json:"RepoTags"`
	RepoDigests   []string         `json:"RepoDigests"`
	Created       string           `json:"Created"`
	Size          int64            `json:"Size"`
	VirtualSize   int64            `json:"VirtualSize"`
	Config        ContainerConfig  `json:"Config"`
	Architecture  string           `json:"Architecture"`
	Os            string           `json:"Os"`
	Author        string           `json:"Author,omitempty"`
	Parent        string           `json:"Parent,omitempty"`
	Comment       string           `json:"Comment,omitempty"`
	DockerVersion string           `json:"DockerVersion,omitempty"`
	RootFS        RootFS           `json:"RootFS"`
	Metadata      ImageMetadata    `json:"Metadata"`
}

// RootFS describes the root filesystem of an image.
type RootFS struct {
	Type   string   `json:"Type"`
	Layers []string `json:"Layers,omitempty"`
}

// ImageMetadata holds metadata about an image.
type ImageMetadata struct {
	LastTagTime string `json:"LastTagTime,omitempty"`
}

// ImagePullRequest is the request to pull an image.
type ImagePullRequest struct {
	Reference string `json:"Reference"`
	Auth      string `json:"Auth,omitempty"`
}

// Network represents a network resource.
type Network struct {
	Name       string                       `json:"Name"`
	ID         string                       `json:"Id"`
	Created    string                       `json:"Created"`
	Scope      string                       `json:"Scope"`
	Driver     string                       `json:"Driver"`
	EnableIPv6 bool                         `json:"EnableIPv6"`
	IPAM       IPAM                         `json:"IPAM"`
	Internal   bool                         `json:"Internal"`
	Attachable bool                         `json:"Attachable"`
	Ingress    bool                         `json:"Ingress"`
	Containers map[string]EndpointResource  `json:"Containers"`
	Options    map[string]string            `json:"Options"`
	Labels     map[string]string            `json:"Labels"`
}

// IPAM holds IP Address Management configuration.
type IPAM struct {
	Driver  string       `json:"Driver"`
	Config  []IPAMConfig `json:"Config"`
	Options map[string]string `json:"Options,omitempty"`
}

// IPAMConfig holds an IPAM pool configuration.
type IPAMConfig struct {
	Subnet  string `json:"Subnet"`
	IPRange string `json:"IPRange,omitempty"`
	Gateway string `json:"Gateway"`
}

// EndpointResource holds endpoint info as shown in network inspect.
type EndpointResource struct {
	Name        string `json:"Name"`
	EndpointID  string `json:"EndpointID"`
	MacAddress  string `json:"MacAddress"`
	IPv4Address string `json:"IPv4Address"`
	IPv6Address string `json:"IPv6Address"`
}

// NetworkCreateRequest is the request to create a network.
type NetworkCreateRequest struct {
	Name       string            `json:"Name"`
	Driver     string            `json:"Driver"`
	Internal   bool              `json:"Internal"`
	Attachable bool              `json:"Attachable"`
	Ingress    bool              `json:"Ingress"`
	EnableIPv6 bool              `json:"EnableIPv6"`
	IPAM       *IPAM             `json:"IPAM,omitempty"`
	Options    map[string]string `json:"Options,omitempty"`
	Labels     map[string]string `json:"Labels,omitempty"`
}

// NetworkCreateResponse is the response from creating a network.
type NetworkCreateResponse struct {
	ID      string `json:"Id"`
	Warning string `json:"Warning"`
}

// NetworkDisconnectRequest is the request to disconnect a container from a network.
type NetworkDisconnectRequest struct {
	Container string `json:"Container"`
	Force     bool   `json:"Force"`
}

// NetworkPruneResponse is the response from pruning networks.
type NetworkPruneResponse struct {
	NetworksDeleted []string `json:"NetworksDeleted"`
}

// Volume represents a volume.
type Volume struct {
	Name       string            `json:"Name"`
	Driver     string            `json:"Driver"`
	Mountpoint string            `json:"Mountpoint"`
	CreatedAt  string            `json:"CreatedAt,omitempty"`
	Status     map[string]any    `json:"Status,omitempty"`
	Labels     map[string]string `json:"Labels"`
	Scope      string            `json:"Scope"`
	Options    map[string]string `json:"Options,omitempty"`
}

// VolumeCreateRequest is the request to create a volume.
type VolumeCreateRequest struct {
	Name       string            `json:"Name,omitempty"`
	Driver     string            `json:"Driver,omitempty"`
	DriverOpts map[string]string `json:"DriverOpts,omitempty"`
	Labels     map[string]string `json:"Labels,omitempty"`
}

// VolumeListResponse wraps the volume list with metadata.
type VolumeListResponse struct {
	Volumes  []*Volume `json:"Volumes"`
	Warnings []string  `json:"Warnings"`
}

// AuthRequest is the request for registry authentication.
type AuthRequest struct {
	Username      string `json:"username"`
	Password      string `json:"password"`
	Email         string `json:"email,omitempty"`
	ServerAddress string `json:"serveraddress"`
}

// AuthResponse is the response from authentication.
type AuthResponse struct {
	Status        string `json:"Status"`
	IdentityToken string `json:"IdentityToken,omitempty"`
}

// BackendInfo returns system information about the backend.
type BackendInfo struct {
	ID                string `json:"ID"`
	Name              string `json:"Name"`
	ServerVersion     string `json:"ServerVersion"`
	Containers        int    `json:"Containers"`
	ContainersRunning int    `json:"ContainersRunning"`
	ContainersPaused  int    `json:"ContainersPaused"`
	ContainersStopped int    `json:"ContainersStopped"`
	Images            int    `json:"Images"`
	Driver            string `json:"Driver"`
	OperatingSystem   string `json:"OperatingSystem"`
	OSType            string `json:"OSType"`
	Architecture      string `json:"Architecture"`
	NCPU              int    `json:"NCPU"`
	MemTotal          int64  `json:"MemTotal"`
	KernelVersion     string `json:"KernelVersion"`
}

// HealthcheckConfig holds the health check configuration for a container.
type HealthcheckConfig struct {
	Test        []string `json:"Test"`
	Interval    int64    `json:"Interval,omitempty"`
	Timeout     int64    `json:"Timeout,omitempty"`
	StartPeriod int64    `json:"StartPeriod,omitempty"`
	Retries     int      `json:"Retries,omitempty"`
}

// HealthState holds the current health state of a container.
type HealthState struct {
	Status        string      `json:"Status"`
	FailingStreak int         `json:"FailingStreak"`
	Log           []HealthLog `json:"Log"`
}

// HealthLog records a single health check execution.
type HealthLog struct {
	Start    string `json:"Start"`
	End      string `json:"End"`
	ExitCode int    `json:"ExitCode"`
	Output   string `json:"Output"`
}

// ContainerTopResponse holds the response from container top.
type ContainerTopResponse struct {
	Titles    []string   `json:"Titles"`
	Processes [][]string `json:"Processes"`
}

// ContainerPruneResponse holds the response from container prune.
type ContainerPruneResponse struct {
	ContainersDeleted []string `json:"ContainersDeleted"`
	SpaceReclaimed    uint64   `json:"SpaceReclaimed"`
}

// NetworkConnectRequest is the request to connect a container to a network.
type NetworkConnectRequest struct {
	Container      string            `json:"Container"`
	EndpointConfig *EndpointSettings `json:"EndpointConfig,omitempty"`
}

// VolumePruneResponse holds the response from volume prune.
type VolumePruneResponse struct {
	VolumesDeleted []string `json:"VolumesDeleted"`
	SpaceReclaimed uint64   `json:"SpaceReclaimed"`
}

// ImageListOptions holds options for listing images.
type ImageListOptions struct {
	All     bool                `json:"All"`
	Filters map[string][]string `json:"Filters,omitempty"`
}

// ImageSummary holds summary info about an image.
type ImageSummary struct {
	ID          string            `json:"Id"`
	ParentID    string            `json:"ParentId"`
	RepoTags    []string          `json:"RepoTags"`
	RepoDigests []string          `json:"RepoDigests"`
	Created     int64             `json:"Created"`
	Size        int64             `json:"Size"`
	SharedSize  int64             `json:"SharedSize"`
	VirtualSize int64             `json:"VirtualSize"`
	Labels      map[string]string `json:"Labels"`
	Containers  int64             `json:"Containers"`
}

// ImageDeleteResponse holds info about a deleted image.
type ImageDeleteResponse struct {
	Untagged string `json:"Untagged,omitempty"`
	Deleted  string `json:"Deleted,omitempty"`
}

// ImageHistoryEntry holds a single layer in image history.
type ImageHistoryEntry struct {
	ID        string   `json:"Id"`
	Created   int64    `json:"Created"`
	CreatedBy string   `json:"CreatedBy"`
	Tags      []string `json:"Tags"`
	Size      int64    `json:"Size"`
	Comment   string   `json:"Comment"`
}

// ImagePruneResponse holds the response from image prune.
type ImagePruneResponse struct {
	ImagesDeleted  []*ImageDeleteResponse `json:"ImagesDeleted"`
	SpaceReclaimed uint64                 `json:"SpaceReclaimed"`
}

// EventsOptions holds options for the events stream.
type EventsOptions struct {
	Since   string              `json:"Since,omitempty"`
	Until   string              `json:"Until,omitempty"`
	Filters map[string][]string `json:"Filters,omitempty"`
}

// Event represents a Docker system event.
type Event struct {
	Type   string     `json:"Type"`
	Action string     `json:"Action"`
	Actor  EventActor `json:"Actor"`
	Time   int64      `json:"time"`
	TimeNano int64    `json:"timeNano"`
}

// EventActor identifies the object that generated an event.
type EventActor struct {
	ID         string            `json:"ID"`
	Attributes map[string]string `json:"Attributes"`
}

// ContainerUpdateRequest holds fields that can be updated on a running container.
type ContainerUpdateRequest struct {
	RestartPolicy RestartPolicy `json:"RestartPolicy"`
}

// ContainerUpdateResponse is the response from updating a container.
type ContainerUpdateResponse struct {
	Warnings []string `json:"Warnings"`
}

// ContainerChangeItem represents a single filesystem change in a container.
type ContainerChangeItem struct {
	Path string `json:"Path"`
	Kind int    `json:"Kind"` // 0=Modified, 1=Added, 2=Deleted
}

// ContainerCommitResponse is the response from committing a container.
type ContainerCommitResponse struct {
	ID string `json:"Id"`
}

// DiskUsageResponse holds the response from system df.
type DiskUsageResponse struct {
	LayersSize int64                    `json:"LayersSize"`
	Images     []*ImageSummary          `json:"Images"`
	Containers []*ContainerSummary      `json:"Containers"`
	Volumes    []*Volume                `json:"Volumes"`
}
