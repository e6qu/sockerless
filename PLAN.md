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
5a. **bleephub storage split.** bleephub keeps its own metadata/index state in SQLite and keeps durable byte content in object storage: git objects, Actions artifacts/caches, release assets, and job logs belong in S3-compatible storage (or the configured durable filesystem for local development). The code should minimize persisted state and derive from git/object storage whenever possible.
6. The user merges PRs. Agents create branches, commits, and PRs only.
7. Continuity docs are updated in every PR and written to stay correct after the PR merges (see [AGENTS.md](AGENTS.md)).

## Next — continuation plan

Active focus: **Bleephub real-service hardening + live-cloud validation (BUG-1075)**. Bleephub keeps replacing fake/shallow test boundaries with real clients and object/git-backed behavior; the S3 git-storage filesystem tests now use a real AWS simulator S3 endpoint and caught a CopyObject list-visibility bug, Actions artifact/cache/log byte storage now has an S3-compatible object-store backend with real-simulator coverage, GraphQL issue sub-issue fields now project the same ordered links as REST, issue rows now carry validated organization issue-type assignments projected through GraphQL and the issue sidebar, GraphQL `Issue.issueFieldValues` exposes REST-backed organization issue-field values through GitHub's typed issue-field value unions, Projects v2 GraphQL field values now round-trip every store-backed text, number, date, single-select, and iteration kind, and GraphQL issue comments now expose persisted REST-backed pin state through `Issue.comments.nodes.isPinned`. The issue sidebar uses repository owner metadata to decide whether organization issue types exist, rather than probing org-only endpoints on user-owned repositories. Actions event and schedule triggers now require their git refs to resolve to real commits instead of substituting `HEAD` or all-zero SHAs for missing refs. Organization audit-log reads now page persisted events with Link headers and search actual action/actor/org/detail fields. Bleephub's own metadata persistence is SQLite-only now: the unsupported PostgreSQL path was removed, the obsolete `BLEEPHUB_DATABASE_URL` knob fails loudly, and persistence tests run unconditionally against SQLite. The sim-proven runner cells still need real cloud infra validation: GitHub `actions/runner` (container job + `services:` + dispatcher) AND GitLab docker-executor (build + artifact + `services:`) are both green on every container-capable backend (ECS, Cloud Run, GCF, ACA, AZF), but no live cell is marked green without authenticated real-cloud runs.

### A. GitHub + GitLab runner topology across all backends — DONE
The bleephub GitHub `actions/runner` cell (TEST 12 container job + TEST 13 `services:` container + TEST 14 dispatcher-spawned runner) is green on ECS, Cloud Run, GCF, **ACA and AZF**; the bleeplab GitLab docker-executor cell is green on the same set (§ D). Each composes the runner's container/network/DNS/services/volumes from that cloud's primitives — ACA Apps + per-job Private DNS; **AZF App Service sites + cloud-dns (VNet + serverFarms-delegated subnet + Private DNS)**; ECS Cloud Map; Cloud Run/GCF network-pod revisions. The azf GitHub cell's hurdles were BUG-1834 (ACR-Tasks build `--load` portability) and BUG-1835 (cloud-dns service-vs-exec discriminator for the GitHub container-job topology).

### B. Faithful build → push → pull overlays — DONE
The Azure and GCP overlay flows use real registry push/pull paths through the simulator's `/v2/` object boundary rather than local-daemon shortcuts. `SOCKERLESS_AZURE_ACR_ENDPOINT` and `SOCKERLESS_GCP_AR_ENDPOINT` are coordinates for the registry endpoint; services remain agnostic and communicate only through the OCI registry contract. The full round-trip is covered by the simulator SDK tests and integration harnesses.

### C. Cloud Run + GCF runner topology — DONE
Cloud Run and Cloud Run Functions cells use the same reverse-agent overlay model and are green for container jobs, service containers, and the dispatcher loop. This completed the GCP half of the runner topology sweep.

### D. Arc 3 — GitLab docker-executor parity — DONE
The bleeplab sim-backed harness proves the full gitlab-runner helper + build + cross-stage artifact + `services:` flow on **every** cloud backend: ECS (#578/#581), Cloud Run (#585), GCF (#586), ACA + AZF (#587). Each cell composes the runner's network/DNS/services/volumes from that cloud's primitives (ECS Cloud Map; Cloud Run/GCF network-pod revisions; ACA ingress + per-build network; **azf App Service VNet integration + Private DNS** for cloud-dns service discovery). Hardening (`feat/azf-clouddns-hardening`) completed it: azf `NetworkConnect` registers `--network-alias` on connect-after-create (PendingCreate stamp + live VNet-integrate/CNAME), and the swift-integration endpoint has the full SDK+CLI+Terraform testing contract (the TF stack added a `Microsoft.Web/serverFarms`-delegated subnet + `azurerm_app_service_virtual_network_swift_connection`; surfaced + fixed sim BUG-1831/1832).

### E. FaaS multi-container pod assembly (BUG-1781) — DONE
All three FaaS backends now deliver full pod semantics with shared-loopback networking. **lambda** and **gcf** already did: lambda runs pod members as chroot subprocesses of one supervisor in a single execution env (one shared netns → `localhost`); gcf co-deploys members into one multi-container Cloud Run revision + `/etc/hosts` alias→127.0.0.1. **azf** (the remaining gap, which had rejected multi-container pods) now assembles the pod as ONE App Service site with **sitecontainers** (the native Azure multi-container primitive — `isMain` + N sidecars sharing a netns): sim models the `sitecontainers` sub-resource + starts members sharing one netns on invoke; backend's network-pod materializer (`pod_site.go`/`network_pod.go`) creates the site, runs the overlay as `isMain` and sidecars as raw images, reconstructs members from a site-tag manifest (stateless), and the azf bootstrap writes `SOCKERLESS_HOST_ALIASES` for by-name resolution. A shared-workspace volume across azf pod members (a named volume mounted into every sitecontainer via the site-level Azure Files share) and per-sidecar exec routing (each overlaid sidecar registers its own reverse-agent keyed by container ID, so `docker exec <sidecar>` routes to that member) are both implemented. Proven by `TestAZFMultiContainerPodSharesLocalhost` (asserts the main reads a marker the sidecar wrote to the shared volume, and `docker exec` into the sidecar succeeds) + sim SDK/CLI sitecontainers coverage.

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
- **bleephub:** a GitHub Enterprise Server REST + GraphQL + Actions simulator — orgs/repos/teams/PRs/issues/apps/OAuth-OIDC/Projects-v2/checks/releases/webhooks/runners/packages, durable SQLite metadata storage plus filesystem/S3 git content, the GitHub-style UI, and complete GitHub Actions support (workflow engine, secrets/variables, checks integration, the official `actions/runner` protocol) proven by the in-repo runner harness.
- **Runner integration:** GitHub `actions/runner` and GitLab Runner (docker executor) both run against sockerless as their Docker daemon; the runner-as-cloud-task topology (container jobs + services + dispatcher loop) is sim-proven for the GitHub cells.
- **Infra:** local HTTPS Caddy gateway, React/Vite admin UI at `/ui/`, `cmd/sockerless/` CLI with `~/.sockerless/` contexts.
