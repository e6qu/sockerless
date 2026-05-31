# Sockerless - Status

Roadmap [PLAN.md](PLAN.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Snapshot

| | |
|---|---|
| Active branch | `main` - no implementation branch active. |
| In-flight | None. |
| Planned next | Optional local HTTPS gateway for simulator APIs, starting with Azure Terraform/provider compatibility. |
| Last merged | PR #299 / issue #298: Azure Redis, GCP Memorystore Redis, GCP Cloud SQL, and GCP Cloud DNS Changes client-surface coverage (2026-05-31). |
| Open GitHub issues | None at last check. |
| Bugs | 1245 filed - 1245 fixed - 2 open - 2 false positives. |
| Open BUGs | BUG-1075 live-cloud validation; BUG-1104 audit-cadence tracker. |
| Live infra | None up. |

## Current Next Task

Build an optional Caddy/local-HTTPS gateway for simulator APIs.

The gateway is local transport infrastructure. It must not add simulator-only public API endpoints, request fields, headers, or response shapes.

Provider facts:

- AzureRM requires trusted HTTPS for custom metadata discovery because `metadata_host` is host-only and the provider builds `https://<host>`.
- Azure Stack is HTTPS-shaped for ARM/metadata usage.
- AzAPI exposes full endpoint URLs and defaults to HTTPS Azure endpoints.
- AWS and GCP Terraform providers accept full custom endpoint URLs; current HTTP localhost simulator endpoints remain valid and must keep working.
- Existing direct simulator TLS via `SIM_TLS_CERT` / `SIM_TLS_KEY` remains supported.

## Invariants

### Process

- Never auto-merge PRs. The user handles merges.
- Use one branch per phase and one PR per phase.
- Before a PR is ready: `git fetch origin main`, rebase on `origin/main`, push, then sync local `main` after merge.
- No interactive commands.
- File concrete BUG entries before fixing discovered gaps.
- Continuity docs must be updated in each PR and written so they are correct after the PR merges.

### Implementation

- No stubs, fakes, mocks, synthetic responses, silent fallbacks, or degraded modes.
- Simulator public APIs must match real cloud public APIs. Local admin/gateway infrastructure may exist, but must not leak into cloud API surfaces.
- One simulator binary per cloud.
- Every new simulator public API slice needs official SDK, vendor CLI, and Terraform-provider coverage where those public client surfaces exist.
- SDK, CLI, and Terraform call sequences differ; do not infer coverage from one client surface to another.
- `specs/SIM_TEST_COVERAGE_MATRIX.md` and `specs/SIM_SURFACE_TABLES/` are the coverage authorities.
- Mux overlap, paged List operations, and resource state machines are recurring bug classes; audit them when touching simulator routes.

### Deferred Trackers

- BUG-1075: live-cloud validation remains deferred. Do not mark cells green without real authenticated cloud runs.
- BUG-1104: audit cadence remains open. Continue re-checking stale SDK/CLI/Terraform not-applicable claims during simulator phases.

## Recent Merged Work

- PR #299 / issue #298: Azure Redis CLI/Terraform coverage; GCP Memorystore Redis gcloud/Terraform coverage; GCP Cloud SQL `/v1` and `/sql/v1beta4` coverage; GCP Cloud DNS Changes and record-set patch routes.
- PR #296/#295/#291/#289 series: AWS Route 53 list fidelity, Lambda Terraform coverage, RDS/ElastiCache/API Gateway client-surface coverage, and Terraform minimum-wait documentation.
- Prior foundational simulator phases: object storage, queues, event systems, streams, managed data SaaS, DNS, VM/instance control planes, managed load balancers, NAT/public-IP, and VPC/networking parity across AWS/GCP/Azure.

Detailed history belongs in PR descriptions and `git log`; this file keeps only resume-critical state.
