# ACA Backend: Delegate Method Implementation Plan

## Overview

The ACA (Azure Container Apps) backend implements `api.Backend` (65 methods). Currently **14 methods** have cloud-native implementations in `backend_impl.go`:

- `ContainerCreate`, `ContainerStart`, `ContainerStop`, `ContainerKill`, `ContainerRemove`
- `ContainerLogs`, `ContainerRestart`, `ContainerPrune`, `ContainerPause`, `ContainerUnpause`
- `ImagePull`, `ImageLoad`
- `VolumeRemove`, `VolumePrune`

The remaining **51 methods** delegate to `s.BaseServer.Method()`.

## Priority Summary

| Priority | Count | Description |
|----------|-------|-------------|
| P0 | 0 | No actively broken methods |
| P1 | 13 | Works but misses cloud-specific features |
| P2 | 38 | BaseServer implementation is adequate |

---

## P1 — Important (13 methods)

### Pod Lifecycle (4 methods) — HIGH IMPACT

#### PodStart
- **BaseServer**: Marks containers running in-memory. Does NOT create ACA Jobs.
- **Implementation**: Override to call `s.ContainerStart()` per container. Deferred-start mechanism triggers `startMultiContainerJobTyped` automatically.
- **Azure APIs**: Via ContainerStart → `Jobs.BeginCreateOrUpdate` + `Jobs.BeginStart`

#### PodStop
- **BaseServer**: In-memory state changes only. Does NOT stop ACA Job execution.
- **Implementation**: Override to stop the ACA Job execution for the pod's main container via `s.stopExecution()`, then update all container states.
- **Azure APIs**: `Jobs.BeginStopExecution()`

#### PodKill
- **BaseServer**: In-memory state changes with signal exit code. Does NOT stop Azure resources.
- **Implementation**: Same as PodStop but with signal-specific exit codes.

#### PodRemove
- **BaseServer**: In-memory removal. Leaves orphaned ACA Jobs.
- **Implementation**: Delete ACA Job, clean up agent state, unregister from resource registry, then delegate to BaseServer for in-memory cleanup.
- **Azure APIs**: `Jobs.BeginDelete()`

### Agent-Aware Operations (4 methods)

#### ContainerAttach
- **BaseServer**: Local pipe via synthetic stream driver.
- **Implementation**: If agent connected, proxy through agent WebSocket. Otherwise return meaningful error.

#### ContainerTop
- **BaseServer**: Synthetic single-process listing.
- **Implementation**: If agent connected, proxy `ps` through agent exec. Otherwise return synthetic response.

#### ExecCreate
- **BaseServer**: Creates exec in-memory.
- **Enhancement**: Add agent-connectivity check. Reject exec if no agent is connected.

#### ExecStart
- **BaseServer**: Uses driver chain. Works with agent, falls back to synthetic.
- **Enhancement**: Explicitly check for agent and return clear error if unavailable.

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

#### Info
- **BaseServer**: Returns static descriptor fields.
- **Implementation**: Enrich with ACA-specific metadata (environment name, subscription, region).

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

### Phase 1: Pod Lifecycle — HIGH IMPACT
**PodStart, PodStop, PodKill, PodRemove**

New file: `backend_impl_pods.go`. Uses existing Azure clients.
**Effort**: Small (20–40 lines each)

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
