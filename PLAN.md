# Sockerless - Roadmap

State [STATUS.md](STATUS.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Goal

Replace Docker Engine with Sockerless for Docker API clients (`docker`, Docker Compose, Testcontainers, CI runners), backed by real cloud infrastructure or high-fidelity local cloud simulators.

## Guiding principles

1. Match public APIs exactly: Docker API, GitHub API (bleephub), and public cloud APIs (simulators).
2. No stubs, fakes, mocks, synthetic behavior, silent fallbacks, or degraded modes.
3. Simulators are real local cloud slices — one binary per cloud, not per product. Every new public-API slice ships with official SDK + vendor CLI + Terraform coverage where the surface exists.
4. Backend ↔ host primitive must match: each cloud's native model serves sub-task dispatch on that cloud (ECS-in-ECS, Lambda-in-Lambda, …), never one cloud's backend baked into another's host.
4a. **Experiential parity, assembled from cloud primitives.** Every Docker/Podman abstraction — containers, multi-container pods, networks, volumes — is *composed* out of cloud primitives on every backend (FaaS included) so the user's experience inside any backend is the same as local Docker/Podman. No "no cloud analog"; a FaaS platform that can't run multiple containers per function is *our job to assemble* (sidecars where offered, else per-member functions + cloud DNS + shared volume), never to reject. See [AGENTS.md](AGENTS.md) § "Assemble Docker abstractions from cloud primitives" and [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md) rule 8.
5. Components stay decoupled. The admin UI and local gateway must not become required dependencies of the simulator public APIs.
6. The user merges PRs. Agents create branches, commits, and PRs only.
7. Continuity docs are updated in every PR and written to stay correct after the PR merges (see [AGENTS.md](AGENTS.md)).

## Next — continuation plan

Active focus: the **pod model + GitHub/GitLab runner integration across all backends**. ACA is the lead container cell — its GitHub container-mode job (bleephub harness TEST 12) is **green** end-to-end (overlay built via the sim's ACR Tasks → ACA App → reverse-agent exec; the overlay round-trips through the registry faithfully, BUG-1785 azure half). The plan below finishes that cell, then sweeps the rest. No committed timeline; confirm before live spend.

### A. Finish the ACA GitHub topology cell
1. **TEST 13 — service containers (BUG-1784).** A `services:` sibling App's alias doesn't resolve from inside the job App. Wire ACA per-job-network service discovery (Private DNS A-records for sibling service names on the per-job `github_network_*`), the container-backend analog of the FaaS work in E.
2. **TEST 14 — dispatcher-spawned runner.** The spawned runner can't reach bleephub (`Connection refused host.docker.internal:80`) — a published-port / external-URL wiring detail in the ACA harness.

### B. BUG-1785 gcp half — faithful Cloud Build push→pull
Carry the azure pattern (real `docker push`/`pull`, sim services agnostic, connected only by `/v2/`) through `simulators/gcp/cloudbuild.go` (confirmed-local push → real `docker push` + `rmi`) and the cloudrun/gcf overlay flows: add a GCP AR registry-endpoint override (parallel to `SOCKERLESS_AZURE_ACR_ENDPOINT`), and update `simulators/gcp/sdk-tests/build_test.go` (CI-validatable with a `registry:2` stand-in) **and** the gated `cloudrun`/`cloudrun-functions` integration tests (which rely on the local-daemon shortcut). **Validate via `make test-integration`** — the integration suite is where the full round-trip is exercised; don't land it on the sdk-test alone. (Reusable finding: Docker auto-trusts loopback registries, Podman does not — the host engine needs a scoped insecure entry for the published sim `/v2/`.)

### C. Extend the topology sweep — Cloud Run + GCF cells
Same overlay model (the GCP Cloud Build slice + the `cloudrun-bootstrap` reverse-agent). Prove container jobs + service containers + the dispatcher loop on each, as ACA did. Depends on B (faithful Cloud Build) for the overlay round-trip.

### D. Arc 3 — GitLab docker-executor parity
A sim-backed harness proving the full helper + build + service-container flow across backends (today only per-backend stdin-attach unit tests exist).

### E. FaaS multi-container pod assembly (BUG-1781)
Lambda, GCF, and AZF currently reject (or can't run) multiple containers sharing one pod, so GitHub `services:` / sidecar `container:` jobs and GitLab service containers don't run on the FaaS backends. Assemble the container-pod execution model out of cloud primitives per backend — native sidecar containers where the platform offers them (Azure Functions sidecars), else **a pod assembled from multiple functions (one per container)** wired by cloud DNS/service-mesh (Cloud Map / Cloud DNS / Private DNS) + a shared workspace volume. Deliver the **full** pod abstraction including **localhost / shared-loopback networking between members** (sibling on `localhost:<port>`): intrinsic where the primitive shares a netns, else assembled by the **agent proxying `localhost:<port>` to the sibling member** over the cloud network. Stage: (a) name the per-FaaS primitive + loopback assembly in CLOUD_RESOURCE_MAPPING, (b) implement the multi-function pod + agent loopback mesh per backend, (c) extend the harness to prove `services:`/`container:` jobs on each. Replaces the interim fail-fast rejections.

### F. Standing candidates
- **Runner-as-cloud-task live pass (BUG-1075).** Cells 1+2 (GitHub → ECS/Lambda) sim-proven; the *live* run against real cloud infra remains. No live cell green without real authenticated runs.
- **Versioned releases + GHCR images (issue #363).**
- **Further sim fidelity audits** (the repeatable method below keeps finding real bugs).

## Sim fidelity audit method (repeatable)

Op-presence is already CI-enforced (coverage matrix + surface tables, with SDK + CLI + Terraform coverage per surface). Deeper audits target the recurring bug class — dropped writable fields, wrong list envelopes, missing idempotency/error codes — by **narrowing then probing**:

1. Sweep registered ops vs test coverage per sim. The broad "X% untested" map is noisy: most untested ops are out-of-slice (no backend/terraform calls them) or already covered by dedicated fidelity tests an op-name grep misses.
2. Narrow to ops that are **load-bearing** (a backend or terraform actually calls them, by grepping `backends/` + `terraform/`) **and** complex enough to harbor a bug (round-trip fields, filters, pagination, idempotency/error codes).
3. **Probe with the real client**, mirroring the exact backend call pattern. "Untested ≠ working." Confirm the bug before filing (avoid false positives).
4. File in [BUGS.md](BUGS.md) before fixing; fix for real; ship a permanent regression test driving the real client.

## Deferred / blocked

- **BUG-1075** — live-cloud validation. No timeline; no live cell marked green without real runs.
- **BUG-1345 / issue #394** — `terraform-provider-azuread` has no Microsoft Graph endpoint override ([upstream issue 1837](https://github.com/hashicorp/terraform-provider-azuread/issues/1837)). When it ships `microsoft_graph_endpoint`, add `azuread_group`/`azuread_user`/`azuread_group_member` to the azure terraform tests and flip the `azure-entra` matrix Terraform cell to `direct`.
- **Issue #363** — versioned releases + GHCR (see Next #2).

## Built so far (summary)

Detailed history is in `git log`, PR descriptions, and [WHAT_WE_DID.md](WHAT_WE_DID.md). By area:

- **Backends (7):** docker passthrough + 6 stateless cloud backends (ecs, lambda, cloudrun, cloudrun-functions, aca, azure-functions), each implementing the full `api.Backend` Docker-API surface with the cloud as the source of truth. Pod model (multi-container), reverse/forward agent exec, named-volume → cloud-storage translation, bind-mount → shared-volume translation for the runner workspace. Per-cloud `github-runner-dispatcher-{aws,gcp,azure}` (the ARC-without-k8s analog).
- **Simulators (3):** real local cloud-API slices — AWS, GCP, Azure — covering every service sockerless uses, validated against the official SDK/CLI/Terraform clients and the vendored machine-readable cloud-API specs (`specs/cloud-api/`) via static surface-conformance + runtime wire-shape gates. `simulators/realexec` provides the Firecracker/netns/nftables real-execution substrate.
- **bleephub:** a GitHub Enterprise Server REST + GraphQL + Actions simulator — orgs/repos/teams/PRs/issues/apps/OAuth-OIDC/Projects-v2/checks/releases/webhooks/runners/packages, durable storage (SQLite/PostgreSQL + filesystem/S3 git content), the GitHub-style UI, and complete GitHub Actions support (workflow engine, secrets/variables, checks integration, the official `actions/runner` protocol) proven by the in-repo runner harness.
- **Runner integration:** GitHub `actions/runner` and GitLab Runner (docker executor) both run against sockerless as their Docker daemon; the runner-as-cloud-task topology (container jobs + services + dispatcher loop) is sim-proven for the GitHub cells.
- **Infra:** local HTTPS Caddy gateway, React/Vite admin UI at `/ui/`, `cmd/sockerless/` CLI with `~/.sockerless/` contexts.
