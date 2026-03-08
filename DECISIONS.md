# Sockerless — Technical Decisions

Architectural and implementation decisions made across phases. Referenced from [PLAN.md](PLAN.md).

---

## Architecture

**Docker API as sole interface** — No Kubernetes, no Podman (except libpod pod extensions), no custom APIs. CI runners talk to Sockerless as if it were Docker. (Principle 4)

**Driver interfaces** — 4 driver types: `ExecDriver`, `FilesystemDriver`, `StreamDriver`, `NetworkDriver`. Agent drivers handle connected containers; inline fallback for disconnected. `DriverSet` on `BaseServer` via `InitDrivers()`. (Phase 32, simplified in Phase 90). See [FEATURE_MATRIX.md](FEATURE_MATRIX.md) for the per-backend driver table.

**Backend model** — 7 backends sharing common `BaseServer` + `Store` from `backend-core`. Cloud backends use self-dispatch (`self api.Backend` field) for typed method overrides. 3 cloud simulators validated against SDKs, CLIs, and Terraform.

**Unified image management** — `core.ImageManager` + `core.AuthProvider` interface. Per-cloud shared modules (`aws-common`, `gcp-common`, `azure-common`) implement auth; all 6 cloud backends delegate 12 image methods to `s.images.*`. (Post-Phase 90)

**File size limit** — Source files under 400 lines. Split by responsibility. (Principle 6)

---

## Pods & Multi-Container

**Podman libpod API for pods** — Docker has no pod concept; Podman does. Adopted `/libpod/pods/*` as Sockerless extension. `docker run` unchanged (single-container). `podman pod create` + containers = multi-container cloud task. (Phase 45)

**PodContext + PodRegistry** — Thread-safe O(1) lookups by pod ID, pod name, container ID, and network name. Default shared namespaces: `["ipc", "net", "uts"]`. `AddContainer` is idempotent. (Phase 45)

**Implicit pod grouping** — Two automatic mechanisms: (1) containers on same user-defined network auto-pod (skip bridge/host/none/default), (2) `NetworkMode: "container:<id>"` creates or joins pod. Network connect also joins pods. (Phase 45)

**Deferred start** — Docker API clients create and start containers one-at-a-time, but cloud backends need all containers upfront. Solution: `PodDeferredStart()` on BaseServer — containers accumulate in pod; each `handleContainerStart` defers until the last container starts, then returns all containers for combined cloud resource creation. `StartedIDs` on PodContext tracks progress. (Phase 46)

**No PodMaterializer interface** — Each backend's start handler has vastly different logic (ECS: RunTask+poll, CloudRun: CreateJob+RunJob+poll, ACA: BeginCreateOrUpdate+BeginStart+poll). A common interface would be too leaky. Shared core helper + per-backend materialization instead. (Phase 46)

**Agent injection on main container only** — First container in `pod.ContainerIDs` = "main" (agent entrypoint + port 9111, essential=true). Sidecars use original entrypoint (essential=false). (Phase 46)

**Cloud state on all pod containers** — Task ARN / job name / execution stored on ALL containers in the pod, so stop/remove works from any container. (Phase 46)

**FaaS rejection** — Lambda/GCF/AZF return 400 for multi-container pods (single-function invocation). (Phase 46)

---

## Cloud Backends

**ECS** — Task definition registered at create time (single container) or start time (multi-container pod). `ContainerDefinitions` slice with unique names per container. Forward agent (poll for RUNNING + health) or reverse agent (callback). (Phases 43-46)

**Cloud Run** — Job spec built + created + run all at start time. `Containers` slice in `TaskTemplate`. VPC connector for agent connectivity. (Phases 43-46)

**ACA** — Job spec built + created + started all at start time. `Containers` slice in `JobTemplate`. Manual trigger type. (Phases 43-46)

**FaaS backends** — Lambda/GCF/AZF invoke single functions. Reverse agent mode only. Helper/cache containers auto-stop after 500ms. (Phases 43-44)

**Unified tagging** — 5 standard tags: `sockerless-managed`, `sockerless-container-id`, `sockerless-backend`, `sockerless-instance`, `sockerless-created-at`. 3 output formats: `AsMap` (AWS), `AsGCPLabels` (underscores, max 63 chars), `AsAzurePtrMap`. (Phase 43)

---

## Crash Recovery

**Crash-only software** — No graceful shutdown. Safe to crash at any point, startup = recovery. (Phase 44)

**Resource registry** — Auto-save on every Register/MarkCleanedUp (atomic tmp+rename). Status lifecycle: pending → active → cleanedUp. Metadata per entry. (Phases 43-44)

**RecoverOnStartup** — Called in all 6 cloud backends. Scans for orphaned cloud resources, reconstructs container state from registry entries. (Phase 44)

**Deferred enhancements** — WAL/append-only log, session recovery, idempotency audit, chaos testing, operation deduplication — all scoped out for future phases.

---

## bleephub (GitHub API Simulator)

**bleephub** — Minimal open-source implementation of GitHub's server-side runner infrastructure (since GitHub is not open-source). 5 service groups: auth/tokens, agent registration, broker (sessions + long-poll), run service, timeline + logs. (Phase 35)

**GraphQL engine** — `graphql-go/graphql` v0.8.1. Globally unique type names (prefixed per connection type). Enum types for `gh` CLI compatibility. (Phases 36-41)

**Git hosting** — `go-git/go-git/v5` with in-memory storage. Smart HTTP protocol for clone/push. HEAD updated to match pushed branch. (Phase 37)

**`gh` CLI compatibility** — GHES-style auth. Full URLs via `gh api` to bypass hostname restrictions. TLS via `BPH_TLS_CERT`/`BPH_TLS_KEY`. OpenAPI schema validation. RFC 5988 Link headers. (Phases 36-41)

**Runner enhancements** — Workflow YAML parsing, `uses:` action tarball proxy with cache, multi-job `needs:` dependency engine, matrix expansion, artifact/cache stubs. (Phase 42)

---

## Simulators

**Self-contained** — Each simulator (AWS, GCP, Azure) implements enough cloud API for its backends. Validated against official SDKs, CLIs, and Terraform providers. TLS via `SIM_TLS_CERT`/`SIM_TLS_KEY`. (Phase 43)

**Azure TLS** — azurestack provider hardcodes `https://`. Terraform tests generate self-signed TLS certs. Docker-only on Linux (macOS Go uses Security.framework, ignores `SSL_CERT_FILE`). (Memory note)

---

## Known Limitations

1. **FaaS transient failures** — ~1 per sequential E2E run on FaaS backends due to reverse agent cleanup timing.
2. **Upstream act individual mode** — azf backend requires `--individual` flag.
3. **Azure terraform tests** — Docker-only (Linux). macOS Go ignores `SSL_CERT_FILE`.
