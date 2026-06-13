# Sockerless - Roadmap

State [STATUS.md](STATUS.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Goal

Replace Docker Engine with Sockerless for Docker API clients (`docker`, Docker Compose, Testcontainers, CI runners), backed by real cloud infrastructure or high-fidelity local cloud simulators.

## Guiding principles

1. Match public APIs exactly: Docker API, GitHub API (bleephub), and public cloud APIs (simulators).
2. No stubs, fakes, mocks, synthetic behavior, silent fallbacks, or degraded modes.
3. Simulators are real local cloud slices — one binary per cloud, not per product. Every new public-API slice ships with official SDK + vendor CLI + Terraform coverage where the surface exists.
4. Backend ↔ host primitive must match: each cloud's native model serves sub-task dispatch on that cloud (ECS-in-ECS, Lambda-in-Lambda, …), never one cloud's backend baked into another's host.
5. Components stay decoupled. The admin UI and local gateway must not become required dependencies of the simulator public APIs.
6. The user merges PRs. Agents create branches, commits, and PRs only.
7. Continuity docs are updated in every PR and written to stay correct after the PR merges (see [AGENTS.md](AGENTS.md)).

## Next

No committed timeline; pick per session and confirm with the user before live spend:

1. **Runner-as-cloud-task live pass (BUG-1075).** Cells 1+2 (GitHub runner → ECS/Lambda) are sim-proven end-to-end by the bleephub official-runner harness; cells 3+4 (GitLab) run sim-backed. The remaining work is the *live* run against real cloud infra. Do not mark any live cell green without real authenticated runs.
2. **Versioned releases + GHCR images (issue #363).** Tagging, a release workflow, and image publishing — the launch-gating step for external users. Deferred while the project was early; now a reasonable consolidation milestone.
3. **Further sim fidelity audits.** The repeatable method (below) keeps finding real bugs.

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
