# CloudRun Backend: Delegate Method Implementation Plan

## Overview

The CloudRun backend implements `api.Backend` (65 methods). Currently **32 methods** have cloud-native implementations in `backend_impl.go`:

- `ContainerCreate`, `ContainerStart`, `ContainerStop`, `ContainerKill`, `ContainerRemove`
- `ContainerLogs`, `ContainerRestart`, `ContainerPrune`, `ContainerPause`, `ContainerUnpause`
- `ContainerAttach`, `ContainerTop`, `ContainerUpdate`
- `ContainerGetArchive`, `ContainerPutArchive`, `ContainerStatPath`
- `ContainerExport`, `ContainerCommit`
- `ImagePull`, `ImageLoad`, `ImagePush`
- `VolumeRemove`, `VolumePrune`
- `ExecStart`
- `PodStart`, `PodStop`, `PodKill`, `PodRemove`
- `Info`, `AuthLogin`
- `startMultiContainerJobTyped` (private helper)

The remaining **35 methods** delegate to `s.BaseServer.Method()`.

## Priority Summary

| Priority | Count | Done | Description |
|----------|-------|------|-------------|
| P0 | 1 | 1 | BaseServer implementation is actively wrong |
| P1 | 18 | 18 | Works but misses cloud-specific features (images now unified via ImageManager) |
| P2 | 32 | 0 | BaseServer implementation is adequate |

---

## P0 — Critical (1 method)

### ExecStart ✅ DONE
- **BaseServer behavior**: Uses `s.Drivers.Exec.Exec()` which runs commands locally via synthetic/process driver, not inside the Cloud Run container.
- **Why wrong**: For Cloud Run, exec must be proxied to the agent running inside the Cloud Run Job. Without an agent, exec cannot work.
- **Implementation**: Check `c.AgentAddress`. If set, delegate to `s.BaseServer.ExecStart()` which proxies through the agent's exec driver. If no agent, return `NotImplementedError`.
- **Dependencies**: Agent proxy client code (already exists in core).

---

## P1 — Important (18 methods)

### Container Lifecycle

#### ContainerAttach ✅ DONE
- **BaseServer**: Creates pipe via synthetic stream driver.
- **Why incomplete**: Cloud Run Jobs do not expose interactive I/O.
- **Implementation**: Resolves container, checks `AgentAddress`. If agent connected, delegates to BaseServer (proxies through agent). Otherwise returns `NotImplementedError`.

#### ContainerTop ✅ DONE
- **BaseServer**: Synthetic single-process listing.
- **Implementation**: Resolves container, checks `AgentAddress`. If agent connected, delegates to BaseServer (proxies through agent). Otherwise delegates to BaseServer for synthetic response.

#### ContainerStats
- **BaseServer**: Synthetic stats with fake values.
- **Implementation**: Query Cloud Monitoring (`run.googleapis.com/job/task/cpu_utilization`, `memory_utilization`). Poll at 1s for streaming.
- **GCP APIs**: Cloud Monitoring v3 (`monitoring.NewMetricClient`)
- **Dependencies**: Add `cloud.google.com/go/monitoring` to go.mod, add client to `GCPClients`.

#### ContainerUpdate ✅ DONE
- **BaseServer**: Updates resource limits in-memory only.
- **Implementation**: Logs warning that changes are stored in-memory and take effect on restart. Delegates to BaseServer for in-memory update.

#### ContainerExport / ContainerCommit ✅ DONE
- **Implementation**: Return `NotImplementedError` — no container filesystem access. Validates container reference exists before returning error.

#### ContainerGetArchive / ContainerPutArchive / ContainerStatPath ✅ DONE
- **Implementation**: Resolves container, checks `AgentAddress`. If agent connected, delegates to BaseServer (proxies through agent). Otherwise returns `NotImplementedError`.

### Images — Unified Image Management ✅ DONE

All 12 image methods now delegate to `core.ImageManager` with `ARAuthProvider` (in `image_auth.go`). The old `registry.go` (containing `fetchImageConfig`, `parseImageRef`, `getARToken`, `getDockerHubToken`) has been deleted.

- `ImagePull`, `ImagePush`, `ImageTag`, `ImageRemove` sync to Artifact Registry via `ARAuthProvider`
- `ImageBuild`, `ImageInspect`, `ImageList`, `ImageHistory`, `ImagePrune`, `ImageLoad`, `ImageSave`, `ImageSearch` delegate through `ImageManager`

#### ImageBuild
- **Phase 1** (current): Delegates to `ImageManager` which uses BaseServer logic (synthetic image from Dockerfile). Functional for CI workflows.
- **Phase 2** (future): Submit to Cloud Build, push to Artifact Registry. Requires `cloud.google.com/go/cloudbuild/apiv1`.

### Volumes

#### VolumeCreate
- **BaseServer**: Local temp directory.
- **Implementation**: Create GCS bucket or prefix (`gs://sockerless-volumes-{project}/{name}/`). Store in `VolumeState.BucketPath`. Mount via GCS FUSE in job spec.
- **GCP APIs**: `storage.Client.Bucket().Create()` (already in GCPClients)

### Pods

#### PodStart ✅ DONE
- **BaseServer**: Marks containers running without creating Cloud Run Job.
- **Implementation**: Calls `s.ContainerStart(cid)` per container (skips already-running). Deferred-start mechanism triggers `startMultiContainerJobTyped` automatically.

#### PodStop / PodKill ✅ DONE
- **BaseServer**: In-memory state changes only.
- **Implementation**: Calls `s.ContainerStop()`/`s.ContainerKill()` per container (cancels Cloud Run executions). PodKill defaults signal to SIGKILL.

#### PodRemove ✅ DONE
- **BaseServer**: In-memory removal only.
- **Implementation**: Checks for running containers when force=false. Calls `s.ContainerRemove()` per container (deletes Cloud Run Jobs), then deletes pod.

### System

#### Info ✅ DONE
- **BaseServer**: Static descriptor fields.
- **Implementation**: Enriches BaseServer.Info() with GCP project/region in OperatingSystem, Driver, and KernelVersion fields.

#### AuthLogin ✅ DONE
- **BaseServer**: Always returns success.
- **Implementation**: Detects GCR/Artifact Registry addresses (`.gcr.io`, `-docker.pkg.dev`, `.pkg.dev`), logs warning about `gcloud auth configure-docker` for production. Delegates to BaseServer for credential storage.

---

## P2 — Acceptable (32 methods)

- **Container**: Inspect, List, Wait, Rename, Resize, Changes
- **Exec**: Create, Inspect, Resize
- **Images**: All 12 methods now use unified `ImageManager` (see P1 Images section above)
- **Networks**: Create, List, Inspect, Connect, Disconnect, Remove, Prune
- **Volumes**: List, Inspect
- **Pods**: Create, List, Inspect, Exists
- **System**: Df, Events

---

## Implementation Phases

### Phase A: Critical Fix (P0) ✅ DONE
1. **ExecStart** — Agent check, delegates or returns NotImplementedError.

### Phase B: Pod Lifecycle (P1) ✅ DONE
2. **PodStart** — Calls `s.ContainerStart()` per container.
3. **PodStop/PodKill** — Calls `s.ContainerStop()`/`s.ContainerKill()` per container.
4. **PodRemove** — Calls `s.ContainerRemove()` per container + delete pod.

### Phase C: Agent-Proxied Operations (P1) ✅ DONE
5. **ContainerAttach, GetArchive, PutArchive, StatPath, Top** ✅ — agent check, proxy through BaseServer or NotImplementedError.
6. **ContainerExport, ContainerCommit** ✅ — return NotImplementedError.
7. **Info** ✅ — GCP project/region enrichment.
8. **ContainerUpdate** ✅ — warning log + BaseServer delegate.
9. **ImagePush** ✅ — returns NotImplementedError.
10. **AuthLogin** ✅ — GCR/AR detection + warning + BaseServer delegate.

### Phase D: Cloud-Native Enhancements (P1)
11. **ContainerStats** — Cloud Monitoring metrics. Requires new GCP client.
12. **VolumeCreate** — GCS-backed volumes.
13. **ImageBuild** — Cloud Build integration (stretch goal).

### New GCP Clients Needed

| Client | Phase | Package |
|--------|-------|---------|
| Cloud Monitoring | D | `cloud.google.com/go/monitoring/apiv3` |
| Cloud Build | D | `cloud.google.com/go/cloudbuild/apiv1` |

### Recommended Order
A → B → C → D
