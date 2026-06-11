# Specs

Specification documents for sockerless. [`SOCKERLESS_SPEC.md`](SOCKERLESS_SPEC.md) is the core spec; the rest are companion specs, contracts, and maintained matrices. Two are authoritative references cited by implementation work:

- [`CLOUD_RESOURCE_MAPPING.md`](CLOUD_RESOURCE_MAPPING.md) — the authoritative "how does sockerless model X on cloud Y" mapping and the source of truth for the stateless-backend invariant.
- [`SIM_TEST_COVERAGE_MATRIX.md`](SIM_TEST_COVERAGE_MATRIX.md) + [`SIM_SURFACE_TABLES/`](SIM_SURFACE_TABLES/README.md) — the maintained simulator coverage index, enforced in CI.

## Core spec + configuration

| Doc | Summary |
|---|---|
| [`SOCKERLESS_SPEC.md`](SOCKERLESS_SPEC.md) | Core specification: a Docker-compatible REST API daemon that executes containers on cloud serverless backends instead of a local Docker Engine. |
| [`CONFIG.md`](CONFIG.md) | Configuration: the unified `~/.sockerless/config.yaml` file and per-backend environment variables (the file takes precedence when present). |
| [`API_SURFACE.md`](API_SURFACE.md) | The `api.Backend` interface — all 65 methods every backend implements, generated from `api/gen/main.go` + `api/openapi.yaml`. |
| [`DOCKER_REST_API.md`](DOCKER_REST_API.md) | Docker Engine REST API (v1.45 baseline) endpoint-by-endpoint implementation comparison; REST compatibility only. |

## Backend / driver contracts

| Doc | Summary |
|---|---|
| [`BACKENDS.md`](BACKENDS.md) | The 7 backends — each embeds `core.BaseServer` and overrides a subset of `api.Backend` via self-dispatch; per-backend type/driver/module table. |
| [`BACKEND_STATE.md`](BACKEND_STATE.md) | Stateless backend state model: cloud-native tags/labels are the single source of truth; every Docker API call queries the cloud. |
| [`DRIVERS.md`](DRIVERS.md) | The 13 typed driver interfaces (`TypedDriverSet`) dispatching every "perform docker action X against the cloud" decision, plus per-dimension `SOCKERLESS_<BACKEND>_<DIMENSION>` operator overrides. |
| [`FAAS_PODS.md`](FAAS_PODS.md) | Contract for FaaS-shaped backends when a Docker/Podman pod needs multiple containers sharing `localhost` — provide a real shared network namespace or fail clearly. |
| [`CLOUD_RESOURCE_MAPPING.md`](CLOUD_RESOURCE_MAPPING.md) | **Authoritative** Docker/Podman concept ↔ cloud resource mapping per backend, state-derivation rules, the simulator host model, and the stateless recovery contract. |

## Image pipeline

| Doc | Summary |
|---|---|
| [`IMAGE_BUILD.md`](IMAGE_BUILD.md) | `docker build` / `buildx` / `podman build` mapped to cloud-native build services (the `POST /build` surface and its query parameters). |
| [`IMAGE_MANAGEMENT.md`](IMAGE_MANAGEMENT.md) | `core.ImageManager` — wraps the image operations with cloud-specific registry authentication and synchronization. |
| [`IMAGE_REGISTRY.md`](IMAGE_REGISTRY.md) | Cloud-native container registries and the OCI Distribution v2 protocol; the per-cloud `AuthProvider` implementations. |
| [`IMAGE_SCANNING.md`](IMAGE_SCANNING.md) | Vulnerability scanning, image signing, and supply-chain security per cloud (ECR/Inspector, Artifact Analysis, Defender). |

## Simulator architecture + coverage

| Doc | Summary |
|---|---|
| [`SIMULATOR_EXECUTION.md`](SIMULATOR_EXECUTION.md) | Execution-model guardrail: container and FaaS workloads in the sims run through real Docker/Podman containers, never as simulator host processes. |
| [`SIMULATOR_REAL_EXECUTION.md`](SIMULATOR_REAL_EXECUTION.md) | Implementation contract + completion audit for the real-execution substrate behind the EC2 / GCE / Azure VM and VPC/network surfaces. |
| [`SIMULATOR_PERSISTENCE.md`](SIMULATOR_PERSISTENCE.md) | SQLite persistence: the generic `Store[T]` with `MemoryStore` (default) and `SQLiteStore` implementations. |
| [`SIMULATOR_RECOVERY.md`](SIMULATOR_RECOVERY.md) | Restart recovery and re-sync: SQLite state restore plus the `ProcessTracker` PID scan for live workloads. |
| [`SIM_FOUNDATIONAL_AUDIT.md`](SIM_FOUNDATIONAL_AUDIT.md) | Audit of foundational service slices per sim (object storage, data stores, DNS, queues, eventing, VPC, NAT, load balancers). |
| [`SIM_PARITY_MATRIX.md`](SIM_PARITY_MATRIX.md) | Cross-simulator parity matrix: every cloud-API call the backends make, with per-sim implemented / reduced-fidelity / missing status. |
| [`SIM_TEST_COVERAGE_MATRIX.md`](SIM_TEST_COVERAGE_MATRIX.md) | Maintained client-surface index for the simulator testing contract — SDK / CLI / Terraform evidence per surface; `scripts/check-simulator-coverage-matrix.sh` fails CI on drift. |
| [`SIM_SURFACE_TABLES/`](SIM_SURFACE_TABLES/README.md) | Per-service canonical-operation enumerations (✓ implemented / ✗ missing rows) for every sim surface, seeded by `scripts/seed-surface-tables.sh`. |

## Comparisons / parity

| Doc | Summary |
|---|---|
| [`COMPARISONS.md`](COMPARISONS.md) | Per-operation comparison of what Docker does natively vs the API calls, CLI commands, and cloud services each backend uses for the same result. |
| [`BLEEPHUB_GITHUB_API_PARITY.md`](BLEEPHUB_GITHUB_API_PARITY.md) | bleephub ↔ GitHub API signature parity: every bleephub endpoint matches real GitHub's path + request + response shapes modulo base domain. |
