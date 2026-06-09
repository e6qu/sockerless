# Sockerless - Status

Roadmap [PLAN.md](PLAN.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Snapshot

| | |
|---|---|
| Active branch | `phase-g-azure-gcp-aws-coverage` (PR pending — Azure/GCP service slices + AWS coverage audit cleanup) |
| In-flight | BUG-1581, BUG-1582, and BUG-1583 are fixed on this branch: Azure Logic Apps + ACI, GCP Spanner/Dataflow/Bigtable, and the AWS registered-operation residual coverage backfill. |
| Last merged | PR for BUG-1580/#530 + BUG-1540 CloudTrail REST sweep is merged. Earlier ECS workspace blockers #526/#527 and Entra duplicate UPN #525 are merged through PR #529. |
| Open GitHub issues | #394 remains upstream-blocked. Check GitHub before starting the next consumer batch. |
| Bugs | 1585 filed · 1539 fixed · 7 open · 5 false positives |
| Open BUGs | BUG-1075 live-cloud validation; BUG-1104 audit cadence; BUG-1345 azuread upstream; BUG-1584 AzureStack provider deprecation warning despite `metadata_host`; BUG-1585 Bigtable Terraform provider endpoint gap. |
| Planned next | After this PR: next actionable consumer issue or another simulator-surface audit batch. |
| Test-host gating | GCP/Azure Compute+Network SDK/CLI/Terraform apply tests skip off-Linux via `realexec.DetectNetworkCapabilities().Require()` (run for real on the sudo+iproute2/nftables CI runner). EventGrid CLI publish uses loopback + `Host` header (no `*.localhost` DNS dependency). |
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
- **BUG-1345**: azuread Terraform provider has no `microsoft_graph_endpoint` override. Tracked as issue #394. Unblock when upstream resolves https://github.com/hashicorp/terraform-provider-azuread/issues/1837.
- **Issue #363**: versioned releases and GHCR image publishing. Intentionally deferred while the project is early.
- **Issue #394**: azuread upstream blocker (same as BUG-1345).
