# Sockerless — Next Steps

## Phase 83 — Type-Safe API Schema Infrastructure (COMPLETE)

### Completed
- **Phase A**: Aligned 7 `api.*` field names with Docker SDK (CPUShares, CPUQuota, CPUPeriod, NanoCPUs, DNS, DNSSearch, DNSOptions). JSON tags unchanged. All modules compile, core tests pass.
- **Phase B1**: Added goverter v1.9.4 to `backends/docker/go.mod`, created `generate.go`.
- **Phase B2**: Created goverter converter interface (17 methods, 30+ extend functions). Generated 275 lines of type-safe mapping code. Files: `converter.go`, `converter_gen.go`, `converter_init.go`, `converter_manual.go`.
- **Phase B3**: Replaced all 51+ manual mapping sites with converter calls. 604 lines deleted across 5 files. go vet clean.
- **Phase C1**: Implemented `api.Backend` on Docker backend Server (`backends/docker/backend_impl.go`, ~580 lines). All 44 interface methods delegate to `s.docker.*` + goverter converters. Helper types: `hijackedRWC`, `nopRWC`. Renamed `Info()` → `getInfo()` in client.go to avoid conflict. Added `Name` field to `api.ContainerCreateRequest`.
- **Phase C2**: Implemented `api.Backend` on core BaseServer (`backends/core/backend_impl.go`, ~2080 lines). All 44 methods extracted from HTTP handlers into typed methods. Helper types: `pipeRWC`, `pipeConn` (net.Conn adapter for driver Attach/Exec). Added `matchImageFilters()` for typed image filtering. 302 core tests pass.
- **Phase C3**: Changed frontend to use `api.Backend`. `Server.backend` is now `api.Backend` (was `*BackendClient`). Added `Server.httpProxy *BackendClient` for operations not in interface (pods, build, archive, resize, push, save, search, commit, export, changes, update). Rewrote 8 handler files: ~30 handlers use typed `s.backend.Method()` calls, ~20 use `s.httpProxy` HTTP proxy. Created `BackendHTTPAdapter` implementing `api.Backend` via HTTP for backward compat. Created `backend_adapter.go` (~700 lines). 7 frontend tests pass.
- **Phase C4**: Startup composition updated. `NewServer(logger, backend api.Backend, backendAddr string)` accepts any in-process backend. `cmd/main.go` uses `BackendHTTPAdapter` for HTTP-based backends. In-process wiring ready for Phase 68 (Multi-Tenant Backend Pools).

### Files Created (untracked)
- `backends/docker/converter.go` — goverter interface + extend functions
- `backends/docker/converter_gen.go` — generated code (275 lines, DO NOT EDIT)
- `backends/docker/converter_init.go` — `!goverter` build tag init
- `backends/docker/converter_manual.go` — composite converters for complex types
- `backends/docker/generate.go` — `//go:generate` directive
- `backends/docker/backend_impl.go` — Docker backend `api.Backend` implementation (~580 lines)
- `backends/core/backend_impl.go` — Core BaseServer `api.Backend` implementation (~2080 lines)
- `frontends/docker/backend_adapter.go` — `BackendHTTPAdapter` implementing `api.Backend` via HTTP (~700 lines)

### Files Modified
- `api/types.go` — 7 field renames + added `Name` to `ContainerCreateRequest`
- `backends/docker/containers.go` — replaced mapContainerFromDocker + helpers (-310 lines)
- `backends/docker/extended.go` — replaced SystemDf, event, image, change mappings (-168 lines)
- `backends/docker/images.go` — replaced image inspect mapping (-67 lines)
- `backends/docker/networks.go` — replaced network list/inspect/IPAM mappings (-61 lines)
- `backends/docker/volumes.go` — replaced volume create/list/inspect mappings (-34 lines)
- `backends/docker/client.go` — renamed `Info()` → `getInfo()` to avoid interface conflict
- `backends/core/handle_extended.go` — field renames for ContainerUpdate
- `backends/docker/go.mod` + `go.sum` — goverter dep
- `frontends/docker/server.go` — `backend api.Backend` + `httpProxy *BackendClient`, new `NewServer` signature
- `frontends/docker/containers.go` — typed `api.Backend` calls
- `frontends/docker/containers_stream.go` — typed `api.Backend` calls + httpProxy for archive/resize
- `frontends/docker/exec.go` — typed `api.Backend` calls + httpProxy for resize
- `frontends/docker/images.go` — typed `api.Backend` calls + httpProxy for build/push/save/search/commit
- `frontends/docker/networks.go` — typed `api.Backend` calls (fully converted)
- `frontends/docker/volumes.go` — typed `api.Backend` calls (fully converted)
- `frontends/docker/system.go` — typed `api.Backend` calls (fully converted)
- `frontends/docker/pods.go` — httpProxy only (Libpod extensions not in interface)
- `frontends/docker/server_test.go` — updated `NewServer` calls
- `frontends/docker/cmd/main.go` — uses `BackendHTTPAdapter` wrapper

### Phase D — Docker OpenAPI Spec Subset — DONE

**D1: Docker v1.44 spec subset** — Created `api/docker-v1.44-subset.yaml` with 73 type definitions mapping all `api.*` Go types to their Docker spec equivalents. Each type lists field names, types, required status, Docker spec name, and notes. Supports `embeds` for types with embedded structs and `extensions` for Sockerless-only fields.

**D2: Field coverage validation test** — Created `api/coverage_test.go` using `gopkg.in/yaml.v3`. Parses the YAML spec and compares against Go struct fields via reflection. Tests both directions: spec fields missing from Go, and Go fields missing from spec. Handles embedded types and Sockerless extensions. 73 subtests (one per type), all PASS.

---

Phase 78 (UI Polish) — 10 tasks pending.
Phase 68 (Multi-Tenant Backend Pools) — 9 tasks paused.
