# Backend Specification

Sockerless has 7 backends. Each embeds `core.BaseServer` and overrides a subset of the 65 `api.Backend` methods via the self-dispatch pattern.

## Backend Types

| Backend | Cloud | Type | Driver | Module |
|---------|-------|------|--------|--------|
| **ECS** | AWS | Container | `ecs-fargate` | `backends/ecs` |
| **Lambda** | AWS | FaaS | `lambda` | `backends/lambda` |
| **Cloud Run** | GCP | Container | `cloudrun-jobs` | `backends/cloudrun` |
| **Cloud Run Functions** | GCP | FaaS | `cloud-run-functions` | `backends/cloudrun-functions` |
| **ACA** | Azure | Container | `aca-jobs` | `backends/aca` |
| **Azure Functions** | Azure | FaaS | `azure-functions` | `backends/azure-functions` |
| **Docker** | Local | Passthrough | `docker` | `backends/docker` |

Container backends run long-lived containers on cloud infrastructure. FaaS backends map containers to function invocations (short-lived, event-driven). Docker is a passthrough to the local Docker daemon.

## Self-Dispatch Pattern

Source: `backends/core/server.go`

```go
type BaseServer struct {
    self api.Backend  // virtual dispatch target
    // ...
}

func (s *BaseServer) SetSelf(b api.Backend) { s.self = b }
```

All HTTP handlers call `s.self.Method()` instead of `s.Method()`:

```go
func (s *BaseServer) handleContainerCreate(w http.ResponseWriter, r *http.Request) {
    resp, err := s.self.ContainerCreate(&req)  // dispatches to ECS/Lambda/etc
}
```

Each backend calls `s.SetSelf(s)` at init, enabling method overrides without interface boilerplate.

## Server Structs

### BaseServer

```go
type BaseServer struct {
    Store          *Store              // in-memory container/image/volume/network state
    Logger         zerolog.Logger
    Desc           BackendDescriptor   // static metadata (driver name, OS, arch)
    Mux            *http.ServeMux      // HTTP routes
    AgentRegistry  *AgentRegistry      // connected agent processes
    Drivers        DriverSet           // exec, filesystem, stream, network drivers
    Registry       *ResourceRegistry   // persistent cloud resource tracking
    EventBus       *EventBus           // docker events streaming
    StatsProvider  StatsProvider       // real container metrics (nil = zeros)
    self           api.Backend         // virtual dispatch
}
```

### ECS Server

```go
type Server struct {
    *core.BaseServer
    config       Config
    aws          *AWSClients           // EC2, ECS, ECR, CloudWatch, CloudMap, SSM
    images       *core.ImageManager    // ECR-integrated
    ECS          *core.StateStore[ECSState]
    NetworkState *core.StateStore[NetworkState]
    VolumeState  *core.StateStore[VolumeState]
    ipCounter    atomic.Int32
}
```

Additional features: VPC security group creation per Docker network, Cloud Map service discovery, EFS volume mounting, Fargate resource tier mapping, ENI IP extraction, CloudWatch Container Insights stats.

### Lambda Server

```go
type Server struct {
    *core.BaseServer
    config    Config
    aws       *AWSClients
    images    *core.ImageManager
    Lambda    *core.StateStore[LambdaState]
    ipCounter atomic.Int32
}
```

Maps `docker run` to Lambda function creation + invocation. Response payload stored as container logs.

### Cloud Run Server

```go
type Server struct {
    *core.BaseServer
    config       Config
    gcp          *GCPClients
    images       *core.ImageManager
    CloudRun     *core.StateStore[CloudRunState]
    NetworkState *core.StateStore[NetworkState]
    VolumeState  *core.StateStore[VolumeState]
    ipCounter    atomic.Int32
}
```

Most capable cloud backend (34 method overrides). Supports pods, archive ops, container top.

### Cloud Run Functions, ACA, Azure Functions

Same pattern with cloud-specific clients and state stores. FaaS backends override ~21 methods; container backends ~19-34.

### Docker Server

```go
type Server struct {
    *core.BaseServer
    docker *dockerclient.Client
}
```

Overrides all 65 methods. Pure passthrough to local Docker daemon via Docker SDK.

## Method Override Matrix

Every backend implements all 65 `api.Backend` methods. Methods not explicitly overridden are generated as thin delegates to `BaseServer` in `backend_delegates_gen.go`. The table below shows **direct overrides** (non-generated):

| Backend | Direct overrides | Generated delegates |
|---------|-----------------|-------------------|
| **Docker** | 65 | 0 |
| **Cloud Run** | 39 | 26 |
| **ACA** | 36 | 29 |
| **ECS** | 35 | 30 |
| **Azure Functions** | 25 | 40 |
| **Lambda** | 21 | 44 |
| **Cloud Run Functions** | 21 | 44 |

## Cloud-Specific Features

### ECS
- VPC Security Groups per Docker network (`cloudNetworkCreate`/`cloudNetworkDelete`)
- Cloud Map service discovery (`cloudServiceRegister`/`cloudServiceDeregister`)
- Fargate resource tier mapping (`fargateResources`)
- EFS-backed bind mounts when `AgentEFSID` set
- CloudWatch Container Insights stats (`ecsStatsProvider`)
- SSM ExecuteCommand for exec (via `cloudExecStart`)

### Cloud Run
- Cloud DNS managed zones per Docker network
- Cloud DNS A records for service discovery
- Container archive ops via agent
- Full pod lifecycle (multi-container Cloud Run jobs)

### ACA
- NSG tracking per Docker network
- In-process DNS registry for service discovery
- Azure Container Apps exec API via WebSocket

### Lambda / Cloud Run Functions / Azure Functions
- FaaS invocation model (one container = one function invocation)
- Response payload as container logs
- No exec, archive, or pod support
