# ACA Backend: Delegate Method Implementation Plan

## Overview

The ACA (Azure Container Apps) backend implements `api.Backend` (65 methods). Currently **21 methods** have cloud-native implementations:

In `backend_impl.go` (14 methods):
- `ContainerCreate`, `ContainerStart`, `ContainerStop`, `ContainerKill`, `ContainerRemove`
- `ContainerLogs`, `ContainerRestart`, `ContainerPrune`, `ContainerPause`, `ContainerUnpause`
- `ImagePull`, `ImageLoad`
- `VolumeRemove`, `VolumePrune`

In `backend_impl_pods.go` (7 methods):
- `PodStart`, `PodStop`, `PodKill`, `PodRemove`
- `ExecCreate`, `ExecStart`
- `Info`

The remaining **44 methods** delegate to `s.BaseServer.Method()`.

## Priority Summary

| Priority | Count | Description |
|----------|-------|-------------|
| P0 | 0 | No actively broken methods |
| P1 | 13 (7 DONE, 6 remaining) | Works but misses cloud-specific features |
| P2 | 38 | BaseServer implementation is adequate |

---

## P1 — Important (13 methods)

### Pod Lifecycle (4 methods) — HIGH IMPACT

#### PodStart — DONE
- **BaseServer**: Marks containers running in-memory. Does NOT create ACA Jobs.
- **Implementation**: Override to call `s.ContainerStart()` per container. Deferred-start mechanism triggers `startMultiContainerJobTyped` automatically.
- **Azure APIs**: Via ContainerStart → `Jobs.BeginCreateOrUpdate` + `Jobs.BeginStart`
- **File**: `backend_impl_pods.go`

#### PodStop — DONE
- **BaseServer**: In-memory state changes only. Does NOT stop ACA Job execution.
- **Implementation**: Override to call `s.ContainerStop()` per container, which stops ACA Job executions.
- **Azure APIs**: Via ContainerStop → `Jobs.BeginStopExecution()`
- **File**: `backend_impl_pods.go`

#### PodKill — DONE
- **BaseServer**: In-memory state changes with signal exit code. Does NOT stop Azure resources.
- **Implementation**: Calls `s.ContainerKill()` per container with signal forwarding. Defaults to SIGKILL.
- **File**: `backend_impl_pods.go`

#### PodRemove — DONE
- **BaseServer**: In-memory removal. Leaves orphaned ACA Jobs.
- **Implementation**: Calls `s.ContainerRemove()` per container (which deletes ACA Jobs), then deletes pod. Checks running containers when force=false. Copies ContainerIDs before iteration to avoid mutation during loop.
- **Azure APIs**: Via ContainerRemove → `Jobs.BeginDelete()`
- **File**: `backend_impl_pods.go`

### Agent-Aware Operations (4 methods)

#### ContainerAttach
- **BaseServer**: Local pipe via synthetic stream driver.
- **Implementation**: If agent connected, proxy through agent WebSocket. Otherwise return meaningful error.

#### ContainerTop
- **BaseServer**: Synthetic single-process listing.
- **Implementation**: If agent connected, proxy `ps` through agent exec. Otherwise return synthetic response.

#### ExecCreate — DONE
- **BaseServer**: Creates exec in-memory.
- **Enhancement**: Resolves container, validates running state, checks `AgentAddress`. Returns `NotImplementedError` if no agent. Delegates to BaseServer for actual exec creation.
- **File**: `backend_impl_pods.go`

#### ExecStart — DONE
- **BaseServer**: Uses driver chain. Works with agent, falls back to synthetic.
- **Enhancement**: Resolves exec instance, validates container still exists, checks `AgentAddress`. Returns `NotImplementedError` if no agent. Delegates to BaseServer for driver-chain exec.
- **File**: `backend_impl_pods.go`

### Cloud Enhancements

#### ContainerStats
- **BaseServer**: Synthetic stats with zero values.
- **Implementation**: Query Azure Monitor for `CpuUsage` and `MemoryWorkingSetBytes` metrics.
- **Azure APIs**: `azquery.MetricsClient.QueryResource()`
- **Dependencies**: Add `azquery.NewMetricsClient()` to `AzureClients`.

#### ContainerUpdate
- **BaseServer**: Updates resource limits in-memory only.
- **Implementation**: Update ACA Job template with new resource limits. Map Docker fields to ACA CPU/memory tiers (0.25–4.0 CPU, 0.5Gi–8Gi memory).
- **Azure APIs**: `Jobs.BeginCreateOrUpdate()` with updated template
- **Caveat**: ACA has fixed tiers. Docker values must be rounded.

#### VolumeCreate
- **BaseServer**: Creates local temp directory.
- **Implementation**: Create Azure Files share. Store share name in `VolumeState`. Mount in job template.
- **Azure APIs**: `armStorage.FileSharesClient.Create()`
- **Dependencies**: Uses existing `Config.StorageAccount`.

#### Info — DONE
- **BaseServer**: Returns static descriptor fields.
- **Implementation**: Enriches Name field with `(aca:{resourceGroup}/{environment})` suffix.
- **File**: `backend_impl_pods.go`

#### AuthLogin
- **BaseServer**: Always returns success.
- **Implementation**: For ACR registries (`*.azurecr.io`), validate via token exchange. Use existing `getACRToken()` pattern.

### Image Operations (deferred)

#### ImageBuild
- **BaseServer**: Synthetic Dockerfile-parsed image.
- **Implementation**: Optional — use ACR Build Tasks for real cloud builds.
- **Azure APIs**: `armcontainerregistry.RegistryClient.BeginScheduleRun()`
- **Dependencies**: Requires ACR instance. Add `SOCKERLESS_ACA_ACR_NAME` config.

#### ImagePush
- **BaseServer**: Synthetic progress.
- **Implementation**: Keep synthetic. If ACR build is implemented, built images are already in ACR.

---

## P2 — Acceptable (38 methods)

- **Container**: Inspect, List, Wait, Rename, Resize, Changes, Export, Commit, GetArchive, PutArchive, StatPath
- **Exec**: Inspect, Resize
- **Images**: Inspect, List, Remove, History, Prune, Tag, Save, Search
- **Networks**: Create, List, Inspect, Connect, Disconnect, Remove, Prune
- **Volumes**: List, Inspect
- **Pods**: Create, List, Inspect, Exists
- **System**: Df, Events

---

## Implementation Phases

### Phase 1: Pod Lifecycle — DONE
**PodStart, PodStop, PodKill, PodRemove**

Implemented in `backend_impl_pods.go`. Uses existing Azure clients via `s.ContainerStart/Stop/Kill/Remove`.

### Phase 1b: Exec + Info — DONE
**ExecCreate, ExecStart, Info**

Implemented in `backend_impl_pods.go`. Agent-connectivity checks + BaseServer delegation for exec, enriched Name for Info.

### Phase 2: Agent-Aware Exec/Attach
**ExecCreate, ExecStart, ContainerAttach, ContainerTop**

Add agent-connectivity validation. Existing driver chain handles agent case.
**Effort**: Small

### Phase 3: Azure Files Volumes
**VolumeCreate**

New Azure SDK client, integration with job spec builder.
**Effort**: Medium

### Phase 4: Metrics and Monitoring
**ContainerStats, Info**

New Azure Monitor metrics client.
**Effort**: Medium

### Phase 5: Registry Operations
**AuthLogin, ImageBuild, ImagePush**

ACR integration for auth and cloud builds.
**Effort**: Large

### Phase 6: Container Update
**ContainerUpdate**

Propagate resource limits to ACA Jobs.
**Effort**: Small

### New Azure SDK Clients Needed

| Client | Phase | Package |
|--------|-------|---------|
| `azquery.MetricsClient` | 4 | `github.com/Azure/azure-sdk-for-go/sdk/monitor/azquery` |
| `armStorage.FileSharesClient` | 3 | `github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage` |
| `armcontainerregistry.RegistriesClient` | 5 | `github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerregistry/armcontainerregistry` |

### Recommended Order
1 → 2 → 3 → 4 → 6 → 5
