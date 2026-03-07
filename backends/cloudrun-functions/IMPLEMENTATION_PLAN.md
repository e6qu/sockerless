# Cloud Run Functions (GCF) Backend: Delegate Method Implementation Plan

## Overview

The Cloud Run Functions backend implements `api.Backend` (65 methods). Currently **19 methods** have cloud-native implementations in `backend_impl.go`:

- `ContainerCreate`, `ContainerStart`, `ContainerStop`, `ContainerKill`, `ContainerRemove`
- `ContainerLogs`, `ContainerRestart`, `ContainerPrune`, `ContainerPause`, `ContainerUnpause`
- `ContainerExport`, `ContainerCommit`, `ContainerAttach`
- `ImagePull`, `ImageLoad`, `ImageBuild`, `ImagePush`
- `PodStart` (P0 — DONE), `Info` (P1 — DONE)

The remaining **46 methods** delegate to `s.BaseServer.Method()`.

Cloud Run Functions is a FaaS platform. Many container/image operations have no direct equivalent.

## Priority Summary

| Priority | Count | Description |
|----------|-------|-------------|
| P0 | 1 | BaseServer implementation is actively wrong |
| P1 | 4 | ALL DONE |
| P2 | 43 | Adequate or N/A for FaaS |

---

## P0 — Critical (1 method) — DONE

### PodStart — DONE
- **Status**: Implemented in `backend_impl.go` (lines 889-909).
- **What it does**: Iterates pod containers, skips already-running or missing ones, calls `s.ContainerStart(cid)` for each (triggering GCF HTTP invocation), collects errors, sets pod status to "running".
- **Edge cases handled**: pod not found (NotFoundError), already-running containers (skipped), container not in store (skipped), error collection (returned in response), nil slice normalized to `[]string{}`.

---

## P1 — Important (4 methods, 4 DONE)

### ContainerStats
- **BaseServer**: Returns synthetic stats with zero values.
- **Implementation**: Query Cloud Monitoring for Cloud Run service metrics (`cpu/utilizations`, `memory/utilizations`, `request_count`).
- **GCP APIs**: `monitoring.googleapis.com/v3` — `projects.timeSeries.list`
- **Trade-off**: Cloud Monitoring provides ~60s granularity vs Docker's real-time. Defer unless stats accuracy is a user requirement.

### Info — DONE
- **Status**: Implemented in `backend_impl.go` (lines 912-920).
- **What it does**: Calls `s.BaseServer.Info()`, then enriches `Name` with GCP project and region metadata.
- **Note**: Does not query `ListFunctions` for function count (simpler approach, avoids extra API call). Can be enhanced later if needed.

### ImageBuild — DONE
- **Implementation**: Returns `NotImplementedError` directing users to push pre-built images to Artifact Registry.

### ImagePush — DONE
- **Implementation**: Returns `NotImplementedError` directing users to push images directly to Artifact Registry.

### Unified Image Management — DONE
All 12 image methods now delegate to `core.ImageManager` with `ARAuthProvider` (in `image_auth.go`). The old `registry.go` (containing `parseImageRef`, `getARToken`) has been deleted. `ImageTag` and `ImageRemove` now sync to Artifact Registry (consistent with CloudRun behavior). `ImageBuild` remains `NotImplementedError` (FaaS override).

---

## P2 — Acceptable / N/A for FaaS (43 methods)

### All Exec Methods (4) — P2
All exec methods work correctly via the reverse agent pattern. `ContainerStart` sets up the agent connection, and the BaseServer's driver chain dispatches to the agent. No GCF-specific override needed.

### All Network Methods (7) — P2
Cloud Run Functions networking is managed by Google (VPC connectors, ingress). No mapping from Docker bridge/overlay networks.

### All Volume Methods (5) — P2
No persistent storage concept. Docker volume semantics do not map meaningfully.

### Most Container Methods — P2
Inspect, List, Wait, Top, Rename, Resize, Update, Changes, PutArchive, StatPath, GetArchive — all work via in-memory store or agent-backed drivers.

**Moved to backend_impl.go (DONE)**:
- `ContainerExport` — Returns `NotImplementedError` (no local filesystem). Validates container exists.
- `ContainerCommit` — Returns `NotImplementedError` (no local filesystem). Validates container param and existence.
- `ContainerAttach` — Delegates to BaseServer when agent connected, returns `NotImplementedError` otherwise.

### All Image Metadata Methods — DONE (unified)
Inspect, List, Remove, History, Prune, Save, Search, Tag — all now delegate through `core.ImageManager`. `ImageTag` and `ImageRemove` sync to Artifact Registry via `ARAuthProvider`.

### Pod Operations (except PodStart) — P2
PodCreate, PodList, PodInspect, PodExists, PodStop, PodKill, PodRemove — BaseServer handles via self-dispatch. GCF rejects multi-container pods at ContainerStart.

### System — P2
Df, Events, AuthLogin — all adequate with in-memory implementations.

---

## Implementation Phases

### Phase 1: P0 Fix — DONE
1. **PodStart** — Override to call `s.ContainerStart()` per container. ~20 lines. DONE.

### Phase 2: Low-Hanging P1 — DONE
2. **Info** — Enrich with project/region metadata. ~9 lines. DONE.

### Phase 3: FaaS NotImplemented Methods — DONE
3. **ContainerExport** — ✅ DONE. Validates container exists, returns `NotImplementedError`.
4. **ContainerCommit** — ✅ DONE. Validates container param and existence, returns `NotImplementedError`.
5. **ContainerAttach** — ✅ DONE. Delegates to BaseServer when agent connected, returns `NotImplementedError` otherwise.
6. **ImageBuild** — ✅ DONE. Returns `NotImplementedError` directing users to Artifact Registry.
7. **ImagePush** — ✅ DONE. Returns `NotImplementedError` directing users to Artifact Registry.

### Phase 4: Optional Enhancements (defer)
8. **ContainerStats** — Cloud Monitoring integration. ~120 lines + new dependency.

### New GCP Clients Needed (Phase 3 only)

| Client | Package |
|--------|---------|
| Cloud Monitoring | `cloud.google.com/go/monitoring/apiv3` |

### Recommended Order
3 (if needed)
