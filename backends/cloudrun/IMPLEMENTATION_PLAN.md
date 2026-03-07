# CloudRun Backend: Delegate Method Implementation Plan

## Overview

The CloudRun backend implements `api.Backend` (65 methods). Currently **24 methods** have cloud-native implementations in `backend_impl.go`:

- `ContainerCreate`, `ContainerStart`, `ContainerStop`, `ContainerKill`, `ContainerRemove`
- `ContainerLogs`, `ContainerRestart`, `ContainerPrune`, `ContainerPause`, `ContainerUnpause`
- `ContainerExport`, `ContainerCommit`
- `ImagePull`, `ImageLoad`
- `VolumeRemove`, `VolumePrune`
- `ExecStart`
- `PodStart`, `PodStop`, `PodKill`, `PodRemove`
- `Info`
- `startMultiContainerJobTyped` (private helper)

The remaining **43 methods** delegate to `s.BaseServer.Method()`.

## Priority Summary

| Priority | Count | Done | Description |
|----------|-------|------|-------------|
| P0 | 1 | 1 | BaseServer implementation is actively wrong |
| P1 | 18 | 7 | Works but misses cloud-specific features |
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

#### ContainerAttach
- **BaseServer**: Creates pipe via synthetic stream driver.
- **Why incomplete**: Cloud Run Jobs do not expose interactive I/O.
- **Implementation**: Return `NotImplementedError` for containers without agents. If agent connected, proxy through agent WebSocket.

#### ContainerTop
- **BaseServer**: Synthetic single-process listing.
- **Implementation**: If agent connected, proxy `ps` through agent. Otherwise return synthetic response.

#### ContainerStats
- **BaseServer**: Synthetic stats with fake values.
- **Implementation**: Query Cloud Monitoring (`run.googleapis.com/job/task/cpu_utilization`, `memory_utilization`). Poll at 1s for streaming.
- **GCP APIs**: Cloud Monitoring v3 (`monitoring.NewMetricClient`)
- **Dependencies**: Add `cloud.google.com/go/monitoring` to go.mod, add client to `GCPClients`.

#### ContainerUpdate
- **BaseServer**: Updates resource limits in-memory only.
- **Implementation**: Update in-memory state. Log warning that changes take effect on restart. Modify `buildJobSpec` to read from updated HostConfig.

#### ContainerExport / ContainerCommit ✅ DONE
- **Implementation**: Return `NotImplementedError` — no container filesystem access. Validates container reference exists before returning error.

#### ContainerGetArchive / ContainerPutArchive / ContainerStatPath
- **Implementation**: If agent connected, proxy through agent. Otherwise return `NotImplementedError`.

### Images

#### ImageBuild
- **BaseServer**: Parses Dockerfile, creates synthetic image.
- **Phase 1**: Keep BaseServer behavior (functional for CI workflows).
- **Phase 2**: Submit to Cloud Build, push to Artifact Registry.
- **GCP APIs**: Cloud Build v1 (`cloudbuild.NewClient`)

#### ImagePush
- **BaseServer**: Synthetic "pushed" progress.
- **Implementation**: Keep synthetic for now. Real AR push if Cloud Build is implemented.

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

#### AuthLogin
- **BaseServer**: Always returns success.
- **Implementation**: For GCR/AR registries, validate credentials against registry. Store valid credentials for ImagePull.

---

## P2 — Acceptable (32 methods)

- **Container**: Inspect, List, Wait, Rename, Resize, Changes
- **Exec**: Create, Inspect, Resize
- **Images**: Inspect, List, Remove, History, Prune, Save, Search, Tag
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

### Phase C: Agent-Proxied Operations (P1) — Partially Done
5. ContainerAttach, GetArchive, PutArchive, StatPath, Top — proxy through agent or NotImplementedError.
6. **ContainerExport, ContainerCommit** ✅ — return NotImplementedError.
7. **Info** ✅ — GCP project/region enrichment.

### Phase D: Cloud-Native Enhancements (P1)
8. **ContainerStats** — Cloud Monitoring metrics. Requires new GCP client.
9. **ContainerUpdate** — Store limits, apply on restart.
10. **VolumeCreate** — GCS-backed volumes.
11. **AuthLogin** — Validate GCR/AR credentials.
12. **ImageBuild/ImagePush** — Cloud Build integration (stretch goal).

### New GCP Clients Needed

| Client | Phase | Package |
|--------|-------|---------|
| Cloud Monitoring | D | `cloud.google.com/go/monitoring/apiv3` |
| Cloud Build | D | `cloud.google.com/go/cloudbuild/apiv1` |

### Recommended Order
A → B → C → D
