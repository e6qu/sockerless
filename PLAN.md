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

### D. Arc 3 — GitLab docker-executor parity — DONE
The bleeplab sim-backed harness proves the full gitlab-runner helper + build + cross-stage artifact + `services:` flow on **every** cloud backend: ECS (#578/#581), Cloud Run (#585), GCF (#586), ACA + AZF (#587). Each cell composes the runner's network/DNS/services/volumes from that cloud's primitives (ECS Cloud Map; Cloud Run/GCF network-pod revisions; ACA ingress + per-build network; **azf App Service VNet integration + Private DNS** for cloud-dns service discovery). Hardening (`feat/azf-clouddns-hardening`) completed it: azf `NetworkConnect` registers `--network-alias` on connect-after-create (PendingCreate stamp + live VNet-integrate/CNAME), and the swift-integration endpoint has the full SDK+CLI+Terraform testing contract (the TF stack added a `Microsoft.Web/serverFarms`-delegated subnet + `azurerm_app_service_virtual_network_swift_connection`; surfaced + fixed sim BUG-1831/1832).

### E. FaaS multi-container pod assembly (BUG-1781) — DONE
All three FaaS backends now deliver full pod semantics with shared-loopback networking. **lambda** and **gcf** already did: lambda runs pod members as chroot subprocesses of one supervisor in a single execution env (one shared netns → `localhost`); gcf co-deploys members into one multi-container Cloud Run revision + `/etc/hosts` alias→127.0.0.1. **azf** (the remaining gap, which had rejected multi-container pods) now assembles the pod as ONE App Service site with **sitecontainers** (the native Azure multi-container primitive — `isMain` + N sidecars sharing a netns): sim models the `sitecontainers` sub-resource + starts members sharing one netns on invoke; backend's network-pod materializer (`pod_site.go`/`network_pod.go`) creates the site, runs the overlay as `isMain` and sidecars as raw images, reconstructs members from a site-tag manifest (stateless), and the azf bootstrap writes `SOCKERLESS_HOST_ALIASES` for by-name resolution. Proven by `TestAZFMultiContainerPodSharesLocalhost` + sim SDK/CLI sitecontainers coverage. Follow-on polish (not blockers): a shared-workspace volume across azf pod members + per-sidecar exec routing.

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
