# Sockerless - Status

Roadmap [PLAN.md](PLAN.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Snapshot

| | |
|---|---|
| Active branch | `feat/ecr-oci-manifest-head-465` (PR #468 — OCI `/v2/` header fidelity #465 + ECS task-def tags include path #467) |
| In-flight | **#465 (BUG-1504):** ECR manifest-HEAD 400 did NOT reproduce (HEAD→404, g-c-r push works); fixed the real gap — emit `Docker-Distribution-Api-Version` on every `/v2/` response (shared `serve()`, 3 cloud copies). **#467 (BUG-1506):** `DescribeTaskDefinition --include TAGS` returned no tags (sim leaked them inside `taskDefinition`, which the SDK drops; provider reads response top-level `tags`). Fixed: `Tags`→`json:"-"`, emit top-level tags from Register (always) + Describe (include=TAGS). SDK+CLI + the `idempotency-fidelity` TF stack (ECS task-def added). |
| Last merged | PR #466 (#457–#464, BUG-1497–1503); PR #463 (#453–#455 + PM-artifact sweep); PR #456 (OCI /v2/ data plane, #450–#452) |
| Open GitHub issues | #465 + #467 fixed by PR #468 (this branch). Only #394 (azuread TF upstream) remains. |
| Bugs | 1506 filed · 1461 fixed · 6 open · 4 false positives (open incl. BUG-1505 flaky azf-backends 5min timeout, CI-only) |
| Open BUGs | BUG-1075 live-cloud validation; BUG-1104 audit cadence; BUG-1345 azuread upstream |
| Planned next | After these two PRs merge: fresh fidelity audit; Phase G new slices (GCP Spanner/Dataflow/Bigtable, Azure); or await new consumer issues |
| Test-host gating | GCP/Azure Compute+Network real-exec tests skip off-Linux via `realexec.DetectNetworkCapabilities().Require()` (run for real on the sudo+iproute2/nftables CI runner). EventGrid CLI publish uses loopback + `Host` header (no `*.localhost` DNS dependency). |
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
