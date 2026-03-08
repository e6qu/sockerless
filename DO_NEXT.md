# Sockerless — Next Steps

## Recently Completed

- **Phase 90**: Removed memory backend, spec-driven state machine tests, cloud operation mappings
- **Unified Image Management** (PR #100): Per-cloud shared image modules (`aws-common`, `gcp-common`, `azure-common`), `core.ImageManager` + `AuthProvider` interface, ~2000 lines of duplication eliminated

## Pending Work

### Phase 68 — Multi-Tenant Backend Pools (In Progress)
9 tasks remaining (P68-002 through P68-010). Pool registry, request router, concurrency limiter, lifecycle, metrics, scheduling, resource limits, tests.

### Phase 78 — UI Polish
10 tasks pending. Dark mode, design tokens, error handling UX, container detail modal, auto-refresh, performance audit, accessibility, E2E smoke, documentation.

## TODOs in Codebase

### Image Management — Incomplete Cloud Integration
The `AuthProvider` implementations in `backends/{aws,gcp,azure}-common/` are wired but exercise cloud SDK calls that only work against real cloud APIs or simulators with sufficient coverage:

- **ECR `OnPush`/`OnTag`/`OnRemove`**: Uses ECR SDK (`PutImage`, `BatchDeleteImage`) — works against AWS simulator
- **AR `OnPush`/`OnTag`**: Uses `core.OCIPush` against Artifact Registry — requires OCI Distribution API on simulator
- **AR `OnRemove`**: OCI manifest DELETE — handles 405 gracefully (no simulator support needed)
- **ACR `OnPush`/`OnTag`**: Uses `core.OCIPush` against ACR — requires OCI Distribution API on simulator
- **ACR `OnRemove`**: OCI manifest DELETE — handles 405 gracefully

### Cloud Backend Gaps
Several cloud backends return `NotImplementedError` for operations that have no cloud equivalent:
- **Pause/Unpause**: All 6 cloud backends — no cloud equivalent exists
- **Container Export/Commit**: ECS, CloudRun, ACA — no filesystem access on managed containers
- **Container Changes**: CloudRun — no diff capability on managed containers
