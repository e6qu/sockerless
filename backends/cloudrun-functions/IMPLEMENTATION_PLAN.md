# Cloud Run Functions (GCF) Backend: Delegate Method Implementation Plan

## Overview

The Cloud Run Functions backend implements `api.Backend` (65 methods). Currently **14 methods** have cloud-native implementations in `backend_impl.go`:

- `ContainerCreate`, `ContainerStart`, `ContainerStop`, `ContainerKill`, `ContainerRemove`
- `ContainerLogs`, `ContainerRestart`, `ContainerPrune`, `ContainerPause`, `ContainerUnpause`
- `ImagePull`, `ImageLoad`
- `PodStart` (P0 — DONE), `Info` (P1 — DONE)

The remaining **51 methods** delegate to `s.BaseServer.Method()`.

Cloud Run Functions is a FaaS platform. Many container/image operations have no direct equivalent.

## Priority Summary

| Priority | Count | Description |
|----------|-------|-------------|
| P0 | 1 | BaseServer implementation is actively wrong |
| P1 | 4 | Works but misses cloud-specific features |
| P2 | 48 | Adequate or N/A for FaaS |

---

## P0 — Critical (1 method) — DONE

### PodStart — DONE
- **Status**: Implemented in `backend_impl.go` (lines 889-909).
- **What it does**: Iterates pod containers, skips already-running or missing ones, calls `s.ContainerStart(cid)` for each (triggering GCF HTTP invocation), collects errors, sets pod status to "running".
- **Edge cases handled**: pod not found (NotFoundError), already-running containers (skipped), container not in store (skipped), error collection (returned in response), nil slice normalized to `[]string{}`.

---

## P1 — Important (4 methods, 2 DONE)

### ContainerStats
- **BaseServer**: Returns synthetic stats with zero values.
- **Implementation**: Query Cloud Monitoring for Cloud Run service metrics (`cpu/utilizations`, `memory/utilizations`, `request_count`).
- **GCP APIs**: `monitoring.googleapis.com/v3` — `projects.timeSeries.list`
- **Trade-off**: Cloud Monitoring provides ~60s granularity vs Docker's real-time. Defer unless stats accuracy is a user requirement.

### Info — DONE
- **Status**: Implemented in `backend_impl.go` (lines 912-920).
- **What it does**: Calls `s.BaseServer.Info()`, then enriches `Name` with GCP project and region metadata.
- **Note**: Does not query `ListFunctions` for function count (simpler approach, avoids extra API call). Can be enhanced later if needed.

### ImageBuild
- **BaseServer**: Parses Dockerfile, creates synthetic in-memory image.
- **Phase 1**: Keep BaseServer behavior (functional for CI workflows — image config is preserved).
- **Phase 2**: Submit to Cloud Build, push to Artifact Registry.
- **GCP APIs**: Cloud Build v1 (`cloudbuild.NewClient`)
- **Recommendation**: Defer — current approach works for all CI/CD use cases.

### ImagePush
- **BaseServer**: Synthetic "pushed" progress.
- **Implementation**: Defer — very high complexity for marginal benefit. GCF already pulls from public/private registries.

---

## P2 — Acceptable / N/A for FaaS (48 methods)

### All Exec Methods (4) — P2
All exec methods work correctly via the reverse agent pattern. `ContainerStart` sets up the agent connection, and the BaseServer's driver chain dispatches to the agent. No GCF-specific override needed.

### All Network Methods (7) — P2
Cloud Run Functions networking is managed by Google (VPC connectors, ingress). No mapping from Docker bridge/overlay networks.

### All Volume Methods (5) — P2
No persistent storage concept. Docker volume semantics do not map meaningfully.

### Most Container Methods — P2
Inspect, List, Wait, Attach, Top, Rename, Resize, Update, Changes, Export, PutArchive, StatPath, GetArchive, Commit — all work via in-memory store or agent-backed drivers.

### All Image Metadata Methods — P2
Inspect, List, Remove, History, Prune, Save, Search, Tag — all in-memory operations.

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

### Phase 3: Optional Enhancements (defer)
3. **ContainerStats** — Cloud Monitoring integration. ~120 lines + new dependency.
4. **ImageBuild/ImagePush** — Cloud Build + Artifact Registry. Significant complexity. Defer indefinitely.

### New GCP Clients Needed (Phase 3 only)

| Client | Package |
|--------|---------|
| Cloud Monitoring | `cloud.google.com/go/monitoring/apiv3` |

### Recommended Order
3 (if needed)
