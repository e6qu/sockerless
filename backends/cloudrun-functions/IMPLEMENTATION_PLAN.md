# Cloud Run Functions (GCF) Backend: Delegate Method Implementation Plan

## Overview

The Cloud Run Functions backend implements `api.Backend` (65 methods). Currently **12 methods** have cloud-native implementations in `backend_impl.go`:

- `ContainerCreate`, `ContainerStart`, `ContainerStop`, `ContainerKill`, `ContainerRemove`
- `ContainerLogs`, `ContainerRestart`, `ContainerPrune`, `ContainerPause`, `ContainerUnpause`
- `ImagePull`, `ImageLoad`

The remaining **53 methods** delegate to `s.BaseServer.Method()`.

Cloud Run Functions is a FaaS platform. Many container/image operations have no direct equivalent.

## Priority Summary

| Priority | Count | Description |
|----------|-------|-------------|
| P0 | 1 | BaseServer implementation is actively wrong |
| P1 | 4 | Works but misses cloud-specific features |
| P2 | 48 | Adequate or N/A for FaaS |

---

## P0 — Critical (1 method)

### PodStart
- **BaseServer behavior**: Sets container state to "running" and emits events but does NOT invoke the Cloud Run Function. The function's HTTP trigger is never called.
- **Why wrong**: Containers appear "running" but no actual function execution occurs.
- **Implementation**: Override to iterate pod containers and call `s.ContainerStart(cid)` for each, which triggers the GCF HTTP invocation.
- **No new GCP APIs needed** — uses existing ContainerStart.

```go
func (s *Server) PodStart(name string) (*api.PodActionResponse, error) {
    pod, ok := s.Store.Pods.GetPod(name)
    if !ok {
        return nil, &api.NotFoundError{Resource: "pod", ID: name}
    }
    var errs []string
    for _, cid := range pod.ContainerIDs {
        c, ok := s.Store.Containers.Get(cid)
        if !ok || c.State.Running {
            continue
        }
        if err := s.ContainerStart(cid); err != nil {
            errs = append(errs, err.Error())
        }
    }
    s.Store.Pods.SetStatus(pod.ID, "running")
    return &api.PodActionResponse{ID: pod.ID, Errs: errs}, nil
}
```

---

## P1 — Important (4 methods)

### ContainerStats
- **BaseServer**: Returns synthetic stats with zero values.
- **Implementation**: Query Cloud Monitoring for Cloud Run service metrics (`cpu/utilizations`, `memory/utilizations`, `request_count`).
- **GCP APIs**: `monitoring.googleapis.com/v3` — `projects.timeSeries.list`
- **Trade-off**: Cloud Monitoring provides ~60s granularity vs Docker's real-time. Defer unless stats accuracy is a user requirement.

### Info
- **BaseServer**: Returns generic info from `Desc` fields.
- **Implementation**: Enrich with GCP project ID, region, function count via `functions.ListFunctions`.
- **GCP APIs**: `functions.ListFunctions` (already available)

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

### Phase 1: P0 Fix
1. **PodStart** — Override to call `s.ContainerStart()` per container. ~30 lines, 1 hour.

### Phase 2: Low-Hanging P1
2. **Info** — Enrich with function count and project info. ~40 lines, 2 hours.

### Phase 3: Optional Enhancements (defer)
3. **ContainerStats** — Cloud Monitoring integration. ~120 lines + new dependency.
4. **ImageBuild/ImagePush** — Cloud Build + Artifact Registry. Significant complexity. Defer indefinitely.

### New GCP Clients Needed (Phase 3 only)

| Client | Package |
|--------|---------|
| Cloud Monitoring | `cloud.google.com/go/monitoring/apiv3` |

### Recommended Order
1 → 2 → 3 (if needed)
