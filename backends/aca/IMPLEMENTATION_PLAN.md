# ACA Backend: Delegate Method Implementation Plan

## Overview

The ACA (Azure Container Apps) backend implements `api.Backend` (65 methods). Currently **25 methods** have cloud-native implementations:

In `backend_impl.go` (14 methods):
- `ContainerCreate`, `ContainerStart`, `ContainerStop`, `ContainerKill`, `ContainerRemove`
- `ContainerLogs`, `ContainerRestart`, `ContainerPrune`, `ContainerPause`, `ContainerUnpause`
- `ImagePull`, `ImageLoad`
- `VolumeRemove`, `VolumePrune`

In `backend_impl_pods.go` (11 methods):
- `PodStart`, `PodStop`, `PodKill`, `PodRemove`
- `ExecCreate`, `ExecStart`
- `ContainerAttach`, `ContainerExport`, `ContainerCommit`
- `AuthLogin`
- `Info`

The remaining **40 methods** delegate to `s.BaseServer.Method()`.

## Priority Summary

| Priority | Count | Description |
|----------|-------|-------------|
| P0 | 0 | No actively broken methods |
| P1 | 13 (11 DONE, 2 remaining) | Works but misses cloud-specific features |
| P2 | 36 | BaseServer implementation is adequate |

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

#### ContainerAttach — DONE
- **BaseServer**: Local pipe via synthetic stream driver.
- **Implementation**: If agent connected, delegates to BaseServer (driver chain). Otherwise returns `NotImplementedError`.
- **File**: `backend_impl_pods.go`

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

#### AuthLogin — DONE
- **BaseServer**: Always returns success.
- **Implementation**: For ACR registries (`*.azurecr.io`), logs warning about local-only credential storage, delegates to BaseServer. Non-ACR registries delegate directly.
- **File**: `backend_impl_pods.go`

### Not Implemented (explicit errors)

#### ContainerExport — DONE
- **BaseServer**: Delegates to driver chain.
- **Implementation**: Returns `NotImplementedError` — ACA Jobs do not provide filesystem access for container export. Validates container exists first.
- **File**: `backend_impl_pods.go`

#### ContainerCommit — DONE
- **BaseServer**: Delegates to driver chain.
- **Implementation**: Returns `NotImplementedError` — ACA containers cannot be snapshotted into images. Validates container exists first.
- **File**: `backend_impl_pods.go`

### Image Operations

#### ImageBuild (deferred)
- **BaseServer**: Synthetic Dockerfile-parsed image.
- **Implementation**: Optional — use ACR Build Tasks for real cloud builds. Currently delegates to BaseServer.
- **Azure APIs**: `armcontainerregistry.RegistryClient.BeginScheduleRun()`
- **Dependencies**: Requires ACR instance. Add `SOCKERLESS_ACA_ACR_NAME` config.

#### ImagePush — DONE
- **Implementation**: For ACR targets (`*.azurecr.io`), performs real OCI Distribution push (blob upload + manifest PUT). For other registries, delegates to BaseServer (synthetic progress). ACR push failures are non-fatal (logs warning, returns synthetic progress).
- **File**: `backend_impl_images.go`
- **Helpers**: `pushToACR`, `uploadBlob`, `setAuthHeader`

#### ImageTag — DONE
- **Implementation**: Delegates to BaseServer for in-memory tagging. For ACR targets, async goroutine syncs tag via manifest GET + PUT. Non-fatal on ACR errors.
- **File**: `backend_impl_images.go`

#### ImageRemove — DONE
- **Implementation**: Delegates to BaseServer for in-memory removal. For images with ACR repo tags, async goroutine deletes manifest via HEAD (get digest) + DELETE. Non-fatal on ACR errors.
- **File**: `backend_impl_images.go`

#### ImageLoad — DONE
- **Implementation**: Delegates to BaseServer to allow `docker save | docker load` round-trips (was previously NotImplementedError).
- **File**: `backend_impl.go`

---

## P2 — Acceptable (36 methods)

- **Container**: Inspect, List, Wait, Rename, Resize, Changes, GetArchive, PutArchive, StatPath
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

### Phase 2: Agent-Aware Exec/Attach — MOSTLY DONE
**ExecCreate, ExecStart, ContainerAttach** — DONE (in Phase 1b and Round 2)
**ContainerTop** — remaining

Only ContainerTop remains: proxy `ps` through agent exec if connected, otherwise return synthetic response.
**Effort**: Small

### Phase 3: Azure Files Volumes
**VolumeCreate**

New Azure SDK client, integration with job spec builder.
**Effort**: Medium

### Phase 4: Metrics and Monitoring
**ContainerStats, Info**

New Azure Monitor metrics client.
**Effort**: Medium

### Phase 5: Registry Operations — DONE
**AuthLogin** — DONE (Round 2, ACR warning + BaseServer delegation)
**ImagePush** — DONE (OCI push for ACR targets, BaseServer fallback)
**ImageTag** — DONE (BaseServer + async ACR sync)
**ImageRemove** — DONE (BaseServer + async ACR delete)
**ImageLoad** — DONE (BaseServer delegation)
**ImageBuild** — deferred (delegates to BaseServer)

ACR integration for push/tag/remove implemented in `backend_impl_images.go`.
**Effort**: Complete

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
