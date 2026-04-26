# Driver Specification

Sockerless dispatches every "perform docker action X against the cloud" decision through one of 13 typed driver interfaces in `backends/core/drivers_typed.go`. Each backend constructs a `TypedDriverSet` at startup; HTTP handlers in `backends/core/handle_*.go` call through these interfaces instead of branching on backend identity. Operators can override per-cloud-per-dimension via `SOCKERLESS_<BACKEND>_<DIMENSION>=<impl>` env vars resolved by `backends/core/driver_override.go`.

The narrow `DriverSet` (Exec/Filesystem/Stream/Network) in `backends/core/drivers.go` predates the typed framework and is being absorbed into it dimension-by-dimension. Interfaces still defined there are kept for the network driver chain (which has platform-specific Linux netns logic that doesn't fit the typed shape) and as bridge points for the typed framework's default adapters.

## Typed dimensions

```go
type TypedDriverSet struct {
    Exec     ExecDriver
    Attach   AttachDriver
    FSRead   FSReadDriver
    FSWrite  FSWriteDriver
    FSDiff   FSDiffDriver
    FSExport FSExportDriver
    Commit   CommitDriver
    Build    BuildDriver
    Stats    StatsDriver
    ProcList ProcListDriver
    Logs     LogsDriver
    Signal   SignalDriver
    Registry RegistryDriver
}
```

Every driver implements `Driver.Describe() string` so `NotImplementedError` messages name the backend + the missing prerequisite without leaking metadata into operator-visible errors.

The envelope passed to every typed driver call:

```go
type DriverContext struct {
    Ctx       context.Context
    Container api.Container        // pre-resolved by the handler via ResolveContainerAuto
    Backend   string               // "docker" | "ecs" | "lambda" | "cloudrun" | "gcf" | "aca" | "azf"
    Region    string
    Logger    zerolog.Logger
}
```

## Per-backend default-driver matrix

Each cell shows the typed driver wired into the slot at backend startup. **bold** = cloud-native typed driver bypassing the api.Backend interface; *italic* = legacy adapter wrapping `s.self.<api.Backend method>`.

| Dimension | docker | ecs | lambda | cloudrun | gcf | aca | azf |
|---|---|---|---|---|---|---|---|
| `ExecDriver` | *WrapLegacyExecStart* (docker SDK) | *WrapLegacyExecStart* (SSM) | **WrapLegacyExec** (ReverseAgent) | **WrapLegacyExec** (ReverseAgent) | **WrapLegacyExec** (ReverseAgent) | **WrapLegacyExec** (ReverseAgent) | **WrapLegacyExec** (ReverseAgent) |
| `AttachDriver` | *WrapLegacyContainerAttach* | **NewCloudLogsAttachDriver** (CloudWatch) | **NewCloudLogsAttachDriver** (CloudWatch) | **NewCloudLogsAttachDriver** (Cloud Logging) | **NewCloudLogsAttachDriver** (Cloud Logging) | **NewCloudLogsAttachDriver** (Azure Monitor) | **NewCloudLogsAttachDriver** (Azure Monitor) |
| `LogsDriver` | *WrapLegacyLogs* (docker SDK) | **NewCloudLogsLogsDriver** (CloudWatch) | **NewCloudLogsLogsDriver** (CloudWatch) | **NewCloudLogsLogsDriver** (Cloud Logging) | **NewCloudLogsLogsDriver** (Cloud Logging) | **NewCloudLogsLogsDriver** (Azure Monitor) | **NewCloudLogsLogsDriver** (Azure Monitor) |
| `SignalDriver` | *WrapLegacyKill* | **ssmSignalDriver** (SSM kill) | *WrapLegacyKill* | *WrapLegacyKill* | *WrapLegacyKill* | *WrapLegacyKill* | *WrapLegacyKill* |
| `ProcListDriver` | *WrapLegacyTop* | **ssmProcListDriver** (SSM ps) | **ReverseAgentProcList** | **ReverseAgentProcList** | **ReverseAgentProcList** | **ReverseAgentProcList** | **ReverseAgentProcList** |
| `FSDiffDriver` | *WrapLegacyChanges* | **ssmFSDiffDriver** (SSM find) | **ReverseAgentFSDiff** | **ReverseAgentFSDiff** | **ReverseAgentFSDiff** | **ReverseAgentFSDiff** | **ReverseAgentFSDiff** |
| `FSReadDriver` | *WrapLegacyFSRead* | **ssmFSReadDriver** (SSM stat+tar) | **ReverseAgentFSRead** | **ReverseAgentFSRead** | **ReverseAgentFSRead** | **ReverseAgentFSRead** | **ReverseAgentFSRead** |
| `FSWriteDriver` | *WrapLegacyFSWrite* | **ssmFSWriteDriver** (SSM tar -x) | **ReverseAgentFSWrite** | **ReverseAgentFSWrite** | **ReverseAgentFSWrite** | **ReverseAgentFSWrite** | **ReverseAgentFSWrite** |
| `FSExportDriver` | *WrapLegacyFSExport* | **ssmFSExportDriver** (SSM tar root) | **ReverseAgentFSExport** | **ReverseAgentFSExport** | **ReverseAgentFSExport** | **ReverseAgentFSExport** | **ReverseAgentFSExport** |
| `CommitDriver` | *WrapLegacyCommit* (docker SDK) | *WrapLegacyCommit* (NotImpl — accepted gap; no Fargate host fs) | **ReverseAgentCommit** | **ReverseAgentCommit** | **ReverseAgentCommit** | **ReverseAgentCommit** | **ReverseAgentCommit** |
| `BuildDriver` | *WrapLegacyBuild* (docker SDK) | *WrapLegacyBuild* (CodeBuild via api.Backend) | *WrapLegacyBuild* (CodeBuild via api.Backend) | *WrapLegacyBuild* (CloudBuild via api.Backend) | *WrapLegacyBuild* (CloudBuild via api.Backend) | *WrapLegacyBuild* (ACR Tasks via api.Backend) | *WrapLegacyBuild* (ACR Tasks via api.Backend) |
| `StatsDriver` | *WrapLegacyStats* | *WrapLegacyStats* | *WrapLegacyStats* | *WrapLegacyStats* | *WrapLegacyStats* | *WrapLegacyStats* | *WrapLegacyStats* |
| `RegistryDriver` | *WrapLegacyRegistry* (docker SDK) | *WrapLegacyRegistry* (ECR via api.Backend) | *WrapLegacyRegistry* (ECR via api.Backend) | *WrapLegacyRegistry* (Artifact Registry) | *WrapLegacyRegistry* (Artifact Registry) | *WrapLegacyRegistry* (ACR) | *WrapLegacyRegistry* (ACR) |

**Legend:**
- ✅ Cloud-native typed driver wired (44 of 91 cells excluding docker, where "legacy adapter wrapping the docker SDK" is the cloud-native path).
- The legacy adapters call through `s.self.<api.Backend method>` — they're scaffolding for the wrapper-removal pass tracked in [PLAN.md § Phase 104](../PLAN.md).

## Composition rule

A driver slot returning `NotImplementedError` from its `Describe()` surfaces a precise message naming the backend + dimension + missing prerequisite. Example: an ECS exec call with no SSM session returns:

```
ecs SSMExec via SSM ExecuteCommand: requires task IAM role with ssmmessages:* and EnableExecuteCommand=true
```

`backends/core/driver_override.go` provides a registry where alternate drivers (overlay-rootfs FSDiff, Kaniko Build, BuildKitRemote) can be installed by name. Operators flip them on via `SOCKERLESS_<BACKEND>_<DIMENSION>=<impl>` — e.g. `SOCKERLESS_LAMBDA_FSDIFF=overlay-upper` to use overlay-rootfs upper-dir diff instead of `find / -newer`.

## Adapters in `backends/core/driver_adapt_*.go`

| File | Wraps | Used for |
|---|---|---|
| `driver_adapt_exec.go` | narrow `LegacyExecDriver` | typed Exec on FaaS+CR+ACA (calls `ReverseAgentExecDriver` directly with the hijacked conn) |
| `driver_adapt_execstart.go` | `BaseServer.ExecStart` | typed Exec default (bridges legacy ExecStart's rwc to the typed conn) |
| `driver_adapt_attach.go` | narrow `StreamDriver` + cloud-logs factory | typed Attach default + cloud-native FaaS attach |
| `driver_adapt_logs.go` | `BaseServer.ContainerLogs` + cloud-logs factory | typed Logs default + cloud-native cloud backends |
| `driver_adapt_signal.go` | `BaseServer.ContainerKill` | typed Signal default |
| `driver_adapt_proclist.go` | `BaseServer.ContainerTop` | typed ProcList default |
| `driver_adapt_fsdiff.go` | `BaseServer.ContainerChanges` | typed FSDiff default |
| `driver_adapt_fs.go` | `BaseServer.Container{Stat,Get,Put}Archive` + `ContainerExport` | typed FSRead/FSWrite/FSExport defaults |
| `driver_adapt_commit.go` | `BaseServer.ContainerCommit` | typed Commit default |
| `driver_adapt_build.go` | `BaseServer.ImageBuild` | typed Build default |
| `driver_adapt_stats.go` | `BaseServer.ContainerStats` | typed Stats default |
| `driver_adapt_registry.go` | `BaseServer.ImagePull` + `ImagePush` | typed Registry default |

The `driver_reverseagent_typed.go` file holds the cloud-native typed drivers shared by every reverse-agent backend (Lambda / GCF / AZF / Cloud Run / ACA): ProcList, FSDiff, FSRead, FSWrite, FSExport, Commit. The `backends/ecs/typed_drivers.go` file holds the parallel SSM-based set for ECS (ProcList, FSDiff, FS*, Signal).

## Network driver

Network operations (create / connect / disconnect / inspect / remove networks) use a separate driver chain in `core.DriverSet.Network`, not a typed `TypedDriverSet` slot. The split exists because network drivers have platform-specific real-Linux behaviour (veth, netns) that doesn't fit the per-container `DriverContext` envelope.

```go
type NetworkDriver interface {
    Name() string
    Create(ctx, name, opts) (*NetworkCreateResponse, error)
    Inspect(ctx, id) (*Network, error)
    List(ctx, filters) ([]*Network, error)
    Remove(ctx, id) error
    Connect(ctx, networkID, containerID, config) error
    Disconnect(ctx, networkID, containerID) error
    Prune(ctx, filters) (*NetworkPruneResponse, error)
}
```

**Implementations:**

- **`SyntheticNetworkDriver`** (`backends/core/drivers_network.go`) — in-memory network management with IP allocation from configurable subnets. Used as the base on every platform.
- **`LinuxNetworkDriver`** (`backends/core/drivers_network_linux.go`) — wraps `SyntheticNetworkDriver` with real Linux network namespace operations: creates veth pairs, moves interfaces into container netns, assigns IPs inside the namespace. Active only on Linux; other platforms use `SyntheticNetworkDriver` directly.

Cloud backends layer on top: ECS uses VPC Security Groups + Cloud Map; Cloud Run uses Cloud DNS managed zones; ACA uses NSG + in-process DNS. Those layers are wired through `api.Backend.NetworkCreate / Connect / etc.` rather than the typed driver framework — see [CLOUD_RESOURCE_MAPPING.md](CLOUD_RESOURCE_MAPPING.md) § Networking per cloud.
