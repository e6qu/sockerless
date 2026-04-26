# Sockerless — Technical Decisions

Architectural and implementation decisions, with the *why*. Referenced from [PLAN.md](PLAN.md). Per-bug specifics live in [BUGS.md](BUGS.md); roadmap in [PLAN.md](PLAN.md); state in [STATUS.md](STATUS.md).

---

## Architecture

**Docker API as sole interface.** No Kubernetes, no Podman (except libpod pod extensions), no custom APIs. CI runners talk to Sockerless as if it were Docker.

**Driver interfaces.** Cross-backend driver framework with typed dimensions (Exec, Attach, FSRead, FSWrite, FSDiff, FSExport, Commit, Build, Stats, ProcList, Logs, Signal, Registry). `DriverContext` envelope; `Driver.Describe()` populates `NotImplementedError` automatically. Backends construct a `core.TypedDriverSet` at startup; operators override per-cloud-per-dimension via `SOCKERLESS_<BACKEND>_<DIMENSION>=<impl>`. Sim parity required for the default driver in every dimension. Type tightening underway — `core.ImageRef` is the canonical parsed image reference at the typed `RegistryDriver.Push/Pull` boundary. See [specs/DRIVERS.md](specs/DRIVERS.md) for the full per-backend driver matrix.

**Backend model.** 7 backends sharing common `BaseServer` + `Store` from `backend-core`. Cloud backends use self-dispatch (`self api.Backend` field) for typed method overrides. 3 cloud simulators validated against SDKs, CLIs, and Terraform.

**Unified image management.** `core.ImageManager` + `core.AuthProvider` interface. Per-cloud shared modules (`aws-common`, `gcp-common`, `azure-common`) implement auth; all 6 cloud backends delegate 12 image methods to `s.images.*`.

**File size limit.** Source files under 400 lines. Split by responsibility.

---

## Pods & Multi-Container

**Podman libpod API for pods.** Docker has no pod concept; Podman does. Adopted `/libpod/pods/*` as Sockerless extension. `docker run` unchanged (single-container). `podman pod create` + containers = multi-container cloud task.

**PodContext + PodRegistry.** Thread-safe O(1) lookups by pod ID, pod name, container ID, and network name. Default shared namespaces: `["ipc", "net", "uts"]`. `AddContainer` is idempotent.

**Implicit pod grouping.** Two automatic mechanisms: containers on the same user-defined network auto-pod (skip bridge/host/none/default); `NetworkMode: "container:<id>"` creates or joins a pod. Network connect also joins pods.

**Deferred start.** Docker API clients create and start containers one-at-a-time, but cloud backends need all containers upfront. `PodDeferredStart()` on `BaseServer` accumulates containers in a pod; each `handleContainerStart` defers until the last container starts, then returns all containers for combined cloud resource creation. `StartedIDs` on `PodContext` tracks progress.

**No PodMaterializer interface.** Each backend's start handler has vastly different logic (ECS: RunTask + poll; Cloud Run: CreateJob + RunJob + poll; ACA: BeginCreateOrUpdate + BeginStart + poll). A common interface would be too leaky. Shared core helper + per-backend materialisation instead.

**Agent injection on main container only.** First container in `pod.ContainerIDs` = "main" (agent entrypoint + port 9111, essential=true). Sidecars use original entrypoint (essential=false).

**Cloud state on all pod containers.** Task ARN / job name / execution stored on every container in the pod, so stop/remove works from any container.

**FaaS rejection.** Lambda/GCF/AZF return 400 for multi-container pods — single-function invocation by definition.

---

## Cloud backends

**ECS.** Task definition registered at create time (single container) or start time (multi-container pod). `ContainerDefinitions` slice with unique names per container. Forward agent (poll for RUNNING + health) or reverse agent (callback).

**Cloud Run.** Job spec built + created + run all at start time. `Containers` slice in `TaskTemplate`. VPC connector for agent connectivity. Optional `UseService=true` flag dispatches to long-running Cloud Run Services with internal-ingress for peer reachability.

**ACA.** Job spec built + created + started all at start time. `Containers` slice in `JobTemplate`. Manual trigger type. Optional `UseApp=true` flag dispatches to long-running Container Apps with internal-ingress for peer reachability.

**FaaS backends.** Lambda/GCF/AZF invoke single functions. Reverse agent mode only. Helper / cache containers auto-stop after 500 ms.

**Unified tagging.** 5 standard tags: `sockerless-managed`, `sockerless-container-id`, `sockerless-backend`, `sockerless-instance`, `sockerless-created-at`. 3 output formats: `AsMap` (AWS), `AsGCPLabels` (underscores, max 63 chars; charset-invalid values move to annotations), `AsAzurePtrMap`.

**Stateless invariant.** Cloud backends maintain zero local container state. The cloud is the source of truth; `docker ps` queries the cloud API. Backend restart = `docker ps` still returns running containers without recovery glue.

---

## Crash recovery

**Crash-only software.** No graceful shutdown. Safe to crash at any point; startup = recovery.

**RecoverOnStartup.** Called in every cloud backend's startup. Scans for orphaned cloud resources, reconstructs container state from cloud-resource tags. No on-disk state needed.

**Per-cloud teardown sweep.** Each terraform module ships a `null_resource sockerless_runtime_sweep` that, on destroy, lists every sockerless-managed resource (functions, jobs, services, etc.), clears VPC config (where applicable to release hyperplane ENIs), then deletes them — so `terragrunt destroy` succeeds without manual cleanup.

---

## bleephub (GitHub API Simulator)

**bleephub.** Minimal open-source implementation of GitHub's server-side runner infrastructure. 5 service groups: auth/tokens, agent registration, broker (sessions + long-poll), run service, timeline + logs.

**GraphQL engine.** `graphql-go/graphql` v0.8.1. Globally unique type names (prefixed per connection type). Enum types for `gh` CLI compatibility.

**Git hosting.** `go-git/go-git/v5` with in-memory storage. Smart HTTP protocol for clone/push. HEAD updated to match pushed branch.

**`gh` CLI compatibility.** GHES-style auth. Full URLs via `gh api` to bypass hostname restrictions. TLS via `BPH_TLS_CERT`/`BPH_TLS_KEY`. OpenAPI schema validation. RFC 5988 Link headers.

**Runner enhancements.** Workflow YAML parsing, `uses:` action tarball proxy with cache, multi-job `needs:` dependency engine, matrix expansion, artifact/cache stubs.

---

## Simulators

**Self-contained.** Each simulator (AWS, GCP, Azure) implements enough cloud API for its backends — at cloud-API fidelity. Validated against official SDKs, CLIs, and Terraform providers. TLS via `SIM_TLS_CERT`/`SIM_TLS_KEY`.

**Sim parity per commit.** Any new SDK call added to a backend must update [specs/SIM_PARITY_MATRIX.md](specs/SIM_PARITY_MATRIX.md) and add the sim handler in the same commit. Pre-commit hook enforces SDK + CLI + Terraform test coverage for every new endpoint.

**Azure TLS.** azurestack provider hardcodes `https://`. Terraform tests generate self-signed TLS certs. Docker-only on Linux (macOS Go uses Security.framework, ignores `SSL_CERT_FILE`).

---

## Unified configuration file

**config.yaml as optional unified config.** A single `~/.sockerless/config.yaml` replaces per-context JSON files with a structured YAML format. Environments map to named backend configs; simulators are first-class entries referenced by environments. The CLI reads config.yaml and exports values as env vars before starting backends, so backend binaries remain env-var-only consumers. Legacy JSON contexts (`contexts/*/config.json`) still work — config.yaml takes precedence when present.

**Rationale.** JSON context files require `--set KEY=VALUE` for every env var, making complex configs verbose and error-prone. YAML provides structure (nested cloud-specific sections), cross-referencing (simulator names), and a single-file overview of all environments. The `config migrate` command provides a zero-effort upgrade path.

**Alternatives considered.** TOML — less common in the Go ecosystem, no advantage over YAML for nested config. Extending JSON contexts — would require migrating all existing contexts and doesn't solve the single-file overview problem. HCL — too Terraform-specific, adds a heavy dependency.

---

## Known limitations

- **FaaS transient failures** — occasional reverse-agent cleanup timing flakes on FaaS backends.
- **Upstream act individual mode** — azf backend requires `--individual` flag.
- **Azure terraform tests** — Docker-only (Linux). macOS Go ignores `SSL_CERT_FILE`.
- **Lambda 15-minute hard cap** — no long-running Lambda containers; runner integration uses Lambda only for fast one-shots.
- **CommitDriver on ECS** — accepted gap. No bootstrap insertion point on Fargate.
