# Azure Functions Backend: Delegate Method Implementation Plan

## Overview

The Azure Functions backend implements `api.Backend` (65 methods). Currently **23 methods** have cloud-native implementations:

- `ContainerCreate`, `ContainerStart`, `ContainerStop`, `ContainerKill`, `ContainerRemove`
- `ContainerLogs`, `ContainerRestart`, `ContainerPrune`, `ContainerPause`, `ContainerUnpause`
- `ContainerExport`, `ContainerCommit`, `ContainerAttach`
- `ImagePull`, `ImageLoad`, `ImageBuild`, `ImagePush`
- `PodStart`, `PodStop`, `PodKill`, `PodRemove` (in `backend_impl_pods.go`)
- `Info` (in `backend_impl.go`)

The remaining **42 methods** delegate to `s.BaseServer.Method()`.

Azure Functions is a FaaS platform. Many container/image operations have no direct equivalent.

## Priority Summary

| Priority | Count | Description |
|----------|-------|-------------|
| P0 | 4 | ALL DONE — Pod lifecycle with AZF-specific cleanup |
| P1 | 4 | 2 DONE (Info, ImagePush), 2 remaining (ContainerInspect, ContainerStats) |
| P2 | 39 | Adequate or N/A for FaaS |

---

## P0 — Critical (4 methods) — ALL DONE

These must be overridden because BaseServer defaults bypass AZF-specific lifecycle logic (Function App invocation, agent disconnect, cloud resource cleanup).

### PodStart — DONE
- **BaseServer behavior**: Manually sets container state to "running" without calling `ContainerStart`. No Function App invocation occurs, no reverse agent callback, no `exitCh` creation.
- **Implementation**: Override to iterate pod containers and call `s.ContainerStart(cid)` for each, which triggers the AZF HTTP invocation.
- **Implemented in**: `backend_impl_pods.go`

### PodStop — DONE
- **BaseServer behavior**: Does not call `s.AgentRegistry.Remove(cid)` when stopping pod containers. Reverse agent must be disconnected to unblock the Function App invocation goroutine.
- **Implementation**: For each running container: `s.StopHealthCheck(cid)`, `s.AgentRegistry.Remove(cid)`, `s.Store.ForceStopContainer(cid, 0)`. Emit die + stop events.
- **Implemented in**: `backend_impl_pods.go`

### PodKill — DONE
- **BaseServer behavior**: Same agent disconnect issue as PodStop.
- **Implementation**: Same as PodStop but with signal-to-exit-code mapping.
- **Implemented in**: `backend_impl_pods.go`

### PodRemove — DONE
- **BaseServer behavior**: `removePodContainers` does NOT clean up Azure Function App resources (no `s.azure.WebApps.Delete()`, no `s.Registry.MarkCleanedUp()`). Leaves orphaned Function Apps.
- **Implementation**: For each container: disconnect agent, delete Function App via `s.azure.WebApps.Delete()`, mark cleaned up, clean up all state (networks, store, AZF state, log buffers, staging dirs). Then delete pod.
- **Implemented in**: `backend_impl_pods.go`

---

## P1 — Important (4 methods)

### ContainerInspect
- **BaseServer**: Returns from in-memory store.
- **Enhancement**: Optionally verify Function App still exists via `s.azure.WebApps.Get()`. If externally deleted, update state to exited.

### Info — DONE
- **BaseServer**: Returns generic info with hardcoded values.
- **Implementation**: Enriches BaseServer info with Azure location in OperatingSystem and subscription/resource group in Name.
- **Implemented in**: `backend_impl.go` (end of file)

### ContainerStats
- **BaseServer**: Synthetic zero-value stats.
- **Implementation**: Query Azure Monitor Metrics API for `FunctionExecutionCount`, `FunctionExecutionUnits`, `MemoryWorkingSet`, `CpuTime`. Map to Docker stats format.
- **Azure APIs**: `azquery.MetricsClient`
- **Dependencies**: Add `MetricsClient` to `AzureClients`.

### ImagePush — DONE (NotImplementedError)
- **Current**: Returns `NotImplementedError` directing users to push to ACR directly.
- **Future enhancement**: Full ACR push when `s.config.Registry` is set. Low priority.

### ImageLoad — DONE (BaseServer delegation)
- **Current**: Delegates to BaseServer to allow `docker save | docker load` round-trips.
- **Previously**: Returned `NotImplementedError`.

### AuthLogin — DONE (ACR detection)
- **Current**: For ACR registries (`*.azurecr.io`), logs warning about using managed identity for production, then delegates to BaseServer. For other registries, delegates directly to BaseServer.
- **File**: `backend_impl.go`

---

## P2 — Acceptable / N/A for FaaS (39 methods)

### Container Operations (7)
ContainerList, ContainerTop, ContainerWait, ContainerRename, ContainerResize, ContainerUpdate, ContainerChanges — all in-memory or agent-backed.

**Moved to backend_impl.go (DONE)**:
- `ContainerExport` — Returns `NotImplementedError` (no local filesystem). Validates container exists.
- `ContainerCommit` — Returns `NotImplementedError` (no local filesystem). Validates container param and existence.
- `ContainerAttach` — Delegates to BaseServer when agent connected, returns `NotImplementedError` otherwise.

### Container Filesystem (3)
GetArchive, PutArchive, StatPath — filesystem driver with staging dirs.

### Exec (4)
ExecCreate, ExecStart, ExecInspect, ExecResize — reverse agent dispatches correctly.

### Images (6)
ImageHistory, ImageInspect, ImageList, ImagePrune, ImageRemove, ImageSave, ImageSearch, ImageTag — all in-memory metadata.

**Moved to backend_impl.go (DONE)**:
- `ImageBuild` — Returns `NotImplementedError` directing users to Azure Container Registry.
- `ImagePush` — Returns `NotImplementedError` directing users to Azure Container Registry.
- `ImageLoad` — Delegates to BaseServer (allows `docker save | docker load` round-trips).

### Networks (7)
All in-memory. Azure Functions networking is managed at Function App/VNet level.

### Volumes (5)
All in-memory. Azure Functions has no Docker volume equivalent.

### Pods (4)
PodCreate, PodExists, PodInspect, PodList — in-memory metadata.

### System (3)
AuthLogin, SystemDf, SystemEvents — in-memory.

---

## Implementation Phases

### Phase 1: P0 Pod Lifecycle (4 methods) — DONE
**PodStart, PodStop, PodKill, PodRemove**

Added to `backend_impl_pods.go`. Removed 4 delegate entries from `backend_delegates_gen.go`.
207 lines of new code.

### Phase 2: P1 Enhancements (2 methods) — Info DONE
**Info** — DONE. Enriches BaseServer info with Azure location/subscription/resource group. In `backend_impl.go`.
**ContainerInspect** — Best-effort Function App existence check. Remaining.

### Phase 3: FaaS NotImplemented Methods (6 methods) — DONE
8. **ContainerExport** — ✅ DONE. Validates container exists, returns `NotImplementedError`.
9. **ContainerCommit** — ✅ DONE. Validates container param and existence, returns `NotImplementedError`.
10. **ContainerAttach** — ✅ DONE. Delegates to BaseServer when agent connected, returns `NotImplementedError` otherwise.
11. **ImageBuild** — ✅ DONE. Returns `NotImplementedError` directing users to Azure Container Registry.
12. **ImagePush** — ✅ DONE. Returns `NotImplementedError` directing users to Azure Container Registry.
13. **ImageLoad** — ✅ DONE. Delegates to BaseServer (was NotImplementedError).

### Phase 4: Metrics (1 method)
**ContainerStats** — Add `MetricsClient`, query `AppMetrics`.

### Phase 5: ACR Integration (future)
**ImagePush** — Full ACR push when registry configured (currently returns NotImplementedError).
**AuthLogin** — ✅ DONE. ACR detection with managed identity warning + BaseServer delegation. Removed from delegates.

### New Azure SDK Clients Needed

| Client | Phase | Package |
|--------|-------|---------|
| `armappservice.AppServicePlansClient` | 2 | Already imported |
| `azquery.MetricsClient` | 3 | `github.com/Azure/azure-sdk-for-go/sdk/monitor/azquery` |
| `azcontainerregistry.Client` | 4 | `github.com/Azure/azure-sdk-for-go/sdk/containers/azcontainerregistry` |

### Risks
- **PodStart**: Must handle multi-container pod rejection (existing in `ContainerStart`). Collect errors per container.
- **Agent disconnect timing**: In PodStop/PodKill, remove agent before `ForceStopContainer` to avoid races.

### Recommended Order
1 → 2 → 3 → 4
