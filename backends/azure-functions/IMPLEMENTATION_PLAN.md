# Azure Functions Backend: Delegate Method Implementation Plan

## Overview

The Azure Functions backend implements `api.Backend` (65 methods). Currently **12 methods** have cloud-native implementations in `backend_impl.go`:

- `ContainerCreate`, `ContainerStart`, `ContainerStop`, `ContainerKill`, `ContainerRemove`
- `ContainerLogs`, `ContainerRestart`, `ContainerPrune`, `ContainerPause`, `ContainerUnpause`
- `ImagePull`, `ImageLoad`

The remaining **53 methods** delegate to `s.BaseServer.Method()`.

Azure Functions is a FaaS platform. Many container/image operations have no direct equivalent.

## Priority Summary

| Priority | Count | Description |
|----------|-------|-------------|
| P0 | 4 | BaseServer implementation is actively wrong for pod scenarios |
| P1 | 4 | Works but misses cloud-specific features |
| P2 | 45 | Adequate or N/A for FaaS |

---

## P0 — Critical (4 methods)

These must be overridden because BaseServer defaults bypass AZF-specific lifecycle logic (Function App invocation, agent disconnect, cloud resource cleanup).

### PodStart
- **BaseServer behavior**: Manually sets container state to "running" without calling `ContainerStart`. No Function App invocation occurs, no reverse agent callback, no `exitCh` creation.
- **Implementation**: Override to iterate pod containers and call `s.ContainerStart(cid)` for each, which triggers the AZF HTTP invocation.

### PodStop
- **BaseServer behavior**: Does not call `s.AgentRegistry.Remove(cid)` when stopping pod containers. Reverse agent must be disconnected to unblock the Function App invocation goroutine.
- **Implementation**: For each running container: `s.StopHealthCheck(cid)`, `s.AgentRegistry.Remove(cid)`, `s.Store.ForceStopContainer(cid, 0)`. Emit die + stop events.

### PodKill
- **BaseServer behavior**: Same agent disconnect issue as PodStop.
- **Implementation**: Same as PodStop but with signal-to-exit-code mapping.

### PodRemove
- **BaseServer behavior**: `removePodContainers` does NOT clean up Azure Function App resources (no `s.azure.WebApps.Delete()`, no `s.Registry.MarkCleanedUp()`). Leaves orphaned Function Apps.
- **Implementation**: For each container: disconnect agent, delete Function App via `s.azure.WebApps.Delete()`, mark cleaned up, clean up all state (networks, store, AZF state, log buffers, staging dirs). Then delete pod.

---

## P1 — Important (4 methods)

### ContainerInspect
- **BaseServer**: Returns from in-memory store.
- **Enhancement**: Optionally verify Function App still exists via `s.azure.WebApps.Get()`. If externally deleted, update state to exited.

### Info
- **BaseServer**: Returns generic info with hardcoded values.
- **Implementation**: Query App Service Plan SKU and worker count via `armappservice.AppServicePlansClient.Get`. Map to NCPU/MemTotal. Add subscription ID, resource group, location.

### ContainerStats
- **BaseServer**: Synthetic zero-value stats.
- **Implementation**: Query Azure Monitor Metrics API for `FunctionExecutionCount`, `FunctionExecutionUnits`, `MemoryWorkingSet`, `CpuTime`. Map to Docker stats format.
- **Azure APIs**: `azquery.MetricsClient`
- **Dependencies**: Add `MetricsClient` to `AzureClients`.

### ImagePush (future)
- **Enhancement**: Push to ACR when `s.config.Registry` is set. Low priority.

---

## P2 — Acceptable / N/A for FaaS (45 methods)

### Container Operations (10)
ContainerList, ContainerTop, ContainerWait, ContainerAttach, ContainerRename, ContainerResize, ContainerUpdate, ContainerChanges, ContainerCommit, ContainerExport — all in-memory or agent-backed.

### Container Filesystem (3)
GetArchive, PutArchive, StatPath — filesystem driver with staging dirs.

### Exec (4)
ExecCreate, ExecStart, ExecInspect, ExecResize — reverse agent dispatches correctly.

### Images (9)
ImageBuild, ImageHistory, ImageInspect, ImageList, ImagePrune, ImageRemove, ImageSave, ImageSearch, ImageTag — all in-memory metadata.

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

### Phase 1: P0 Pod Lifecycle (4 methods)
**PodStart, PodStop, PodKill, PodRemove**

Add to `backend_impl.go`. Remove 4 delegate entries from `backend_delegates_gen.go`.
~200 lines of new code.

### Phase 2: P1 Enhancements (2 methods)
**Info** — Add `AppServicePlansClient`. Single API call. Low effort.
**ContainerInspect** — Best-effort Function App existence check.

### Phase 3: Metrics (1 method)
**ContainerStats** — Add `MetricsClient`, query `AppMetrics`.

### Phase 4: ACR Integration (future)
**ImagePush** — ACR push when registry configured.

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
