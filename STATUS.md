# Sockerless - Status

Roadmap [PLAN.md](PLAN.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Snapshot

| | |
|---|---|
| Active branch | `fix/aws-sim-acm-dns-validation` (PR pending — ACM DNS validation, issues #420 + #421) |
| In-flight | AWS ACM DNS-validation fixes (BUG-1464/1465): DNS-validated cert reaches ISSUED once its `_acm-challenge` records exist in the Route53 sim store; wildcard SAN validation record name strips `*.`. SDK + CLI + Terraform coverage all green locally. |
| Last merged | PR #422 — GCP Cloud KMS service (#419) |
| Also merged recently | PR #418 (DynamoDB GSIs #416, ECS Service #417, azf attach-stdin #1461, CloudFront tagging #1462); PR #415 (KMS tagging #413, EC2 API-only #414) |
| Open GitHub issues | #420 + #421 — ACM DNS validation (closing via the pending PR). #423 — Azure KV version-less key crypto (queued). #394 — azuread TF provider upstream blocker |
| Bugs | 1465 filed · 1422 fixed · 5 open · 3 false positives |
| Open BUGs | BUG-1075 live-cloud validation; BUG-1104 audit cadence; BUG-1345 azuread upstream |
| Planned next | After ACM PR: Azure KV #423, then planned Azure/GCP test-gap PRs |
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
