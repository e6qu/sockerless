# Driver Specification

Drivers provide backend-agnostic interfaces for execution, filesystem access, streaming, and networking. They are composed into a `DriverSet` on `BaseServer`.

Source: `backends/core/drivers.go`

```go
type DriverSet struct {
    Exec       ExecDriver
    Filesystem FilesystemDriver
    Stream     StreamDriver
    Network    api.NetworkDriver
}
```

## ExecDriver

Runs commands inside containers.

```go
type ExecDriver interface {
    Exec(ctx context.Context, containerID, execID string, cmd []string,
         env []string, workDir string, tty bool, conn net.Conn) (exitCode int)
}
```

- Streams I/O over `net.Conn`
- Non-TTY mode: wraps output with Docker multiplexed stream headers (8-byte prefix: stream type + length)
- Returns process exit code

**Implementations:**
- `AgentExecDriver` — default for all backends. Forwards exec to a connected agent process inside the container via the agent HTTP API.

## FilesystemDriver

Archive operations on container filesystems.

```go
type FilesystemDriver interface {
    PutArchive(containerID, path string, tarStream io.Reader) error
    GetArchive(containerID, path string, w io.Writer) (*FileInfo, error)
    StatPath(containerID, path string) (*FileInfo, error)
    RootPath(containerID string) string
}
```

- `PutArchive` — extracts tar into container at path (`docker cp` into)
- `GetArchive` — writes tar of container path to writer (`docker cp` out)
- `StatPath` — stat a path inside the container
- `RootPath` — host filesystem root for the container (empty string if no local filesystem)

**Implementations:**
- `AgentFilesystemDriver` — default. Forwards to agent HTTP API (`/archive`, `/stat`).

## StreamDriver

Container attach and log streaming.

```go
type StreamDriver interface {
    Attach(ctx context.Context, containerID string, tty bool, conn net.Conn)
    LogBytes(containerID string) []byte
    LogSubscribe(containerID, subID string) chan []byte
    LogUnsubscribe(containerID, subID string)
}
```

- `Attach` — bidirectional stream to container (hijacked HTTP connection)
- `LogBytes` — returns buffered log output
- `LogSubscribe` — returns channel for live log chunks (follow mode); nil if unsupported
- `LogUnsubscribe` — removes subscription

**Implementations:**
- `AgentStreamDriver` — default. Connects to agent for attach; uses agent log buffer.

## NetworkDriver

Container networking (create/inspect/connect/disconnect/remove networks).

```go
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
```

**Implementations:**

### SyntheticNetworkDriver

Source: `backends/core/drivers_network.go`

In-memory network management with IP allocation from configurable subnets. Used on all platforms as the base.

- `Create` — generates ID, allocates subnet from IPAM, stores in `Store.Networks`
- `Connect` — allocates IP, adds to network's Containers map and container's NetworkSettings
- `Disconnect` — releases IP, removes from both maps
- `Remove` — rejects pre-defined networks (bridge, host, none), deletes from store
- `Prune` — removes networks with no connected containers

### LinuxNetworkDriver

Source: `backends/core/drivers_network_linux.go`

Wraps `SyntheticNetworkDriver` with real Linux network namespace operations:
- Creates veth pairs
- Moves interfaces into container netns
- Assigns IPs inside namespace
- Gracefully degrades to synthetic if netns unavailable

Only active on Linux. Other platforms use `SyntheticNetworkDriver` directly.

## Driver Initialization

Source: `backends/core/server.go:InitDrivers()`

```go
func (s *BaseServer) InitDrivers() {
    s.Drivers.Exec = &AgentExecDriver{...}
    s.Drivers.Filesystem = &AgentFilesystemDriver{...}
    s.Drivers.Stream = &AgentStreamDriver{...}

    syntheticNet := &SyntheticNetworkDriver{Store: s.Store, ...}
    if platformDriver := NewPlatformNetworkDriver(syntheticNet, logger); platformDriver != nil {
        s.Drivers.Network = platformDriver  // Linux: real netns
    } else {
        s.Drivers.Network = syntheticNet    // macOS/Windows: in-memory
    }
}
```

## Cloud-Specific Extensions

Cloud backends extend beyond the base drivers with cloud-native operations wired into the `api.Backend` method overrides (not the driver layer):

| Backend | Networking | Service Discovery | Exec | Logging |
|---------|-----------|-------------------|------|---------|
| ECS | VPC Security Groups | AWS Cloud Map | SSM ExecuteCommand | CloudWatch |
| Cloud Run | Cloud DNS zones | Cloud DNS A records | Agent sidecar | Cloud Logging |
| ACA | NSG tracking | In-process DNS | Container Apps exec API | Azure Monitor KQL |
| Lambda | — | — | — | CloudWatch |
| GCF | — | — | — | Cloud Logging |
| AZF | — | — | — | Azure Monitor |

These are implemented directly on the backend Server structs, not as pluggable drivers, because they depend on cloud-specific SDK clients.
