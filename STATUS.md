# Sockerless - Status

Roadmap [PLAN.md](PLAN.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Snapshot

| | |
|---|---|
| Active branch | `feat/azure-coverage-gaps-pr-a` — Azure simulator coverage gaps PR |
| In-flight | PR A: Azure App Insights SDK/CLI, Private DNS RecordSets SDK/CLI, ACR image ops SDK/CLI |
| Last merged | PR #386 — continuity doc compression |
| Also merged this session | PR #385 bleephub `GET /api/v3/user/teams`; PR #383 AWS SDK/CLI gaps; PR #382 AWS EBS/VPC |
| Open GitHub issues | None |
| Bugs | 1323 filed · 1323 fixed · 2 open · 3 false positives |
| Open BUGs | BUG-1075 live-cloud validation; BUG-1104 audit cadence |
| Planned next | PR B: GCP gaps (SA keys + instance templates implementation; CF Gen2 / Cloud Build trigger / Logging sink+metric / project IAM SDK tests) |
| Live infra | None up |

## Invariants

### Process
- Never auto-merge PRs. User handles all merges.
- One branch per phase, one PR per phase.
- Rebase on `origin/main` before pushing.
- File concrete BUG entries before fixing.
- Update continuity docs in every PR.

### Implementation
- No stubs, fakes, mocks, synthetic responses, or silent fallbacks.
- Simulator public APIs must match real cloud public APIs exactly.
- One simulator binary per cloud.
- Every new public API slice ships with official SDK + vendor CLI + Terraform-provider coverage where those surfaces exist.
- `specs/SIM_TEST_COVERAGE_MATRIX.md` and `specs/SIM_SURFACE_TABLES/` are the coverage authorities.

### Infrastructure
- AzureRM provider requires HTTPS for custom metadata discovery (`metadata_host` is host-only); simulator runs behind local Caddy for Azure Terraform tests.
- AWS/GCP Terraform providers accept `http://localhost` custom endpoints.
- Azure simulator port: 4568; AWS: 4566; GCP: 4567.
- Caddy gateway: `make stack-https-{up,status,ca,down}`.

## Deferred Trackers

- **BUG-1075**: live-cloud validation. Do not mark cloud cells green without authenticated real-cloud runs. No timeline set.
- **BUG-1104**: audit cadence. Keep open while simulator work continues. Every simulator phase re-checks stale SDK/CLI/Terraform claims.
- **Issue #363**: versioned releases and GHCR image publishing. Intentionally deferred while the project is early.
