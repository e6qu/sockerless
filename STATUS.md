# Sockerless - Status

Roadmap [PLAN.md](PLAN.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Snapshot

| | |
|---|---|
| Active branch | `fix/aws-sim-ecs-service-ddb-gsi` (PR #418 open + audit follow-up in progress) |
| In-flight | PR #418 — DynamoDB GSIs (#416) + ECS Service family & capacity providers (#417), plus ECS/DynamoDB audit follow-ups (BUG-1457–1460: DDB UpdateTable / batch+transact / richer conditions; ECS tags / pagination / list ops), plus azf attach-stdin invoke-race fix (BUG-1461 — flaky `test (azure backends)` CI hang) and CloudFront Function tagging fix (BUG-1462 — flaky `tf (aws)` apply on `aws_cloudfront_function`) |
| Last merged | PR #415 — KMS tagging (#413), EC2 control-plane modeling in API-only mode (#414), Podman container image fix |
| Also merged recently | PR #412 (Azure KV version ordering, #407); PR #410 (bleephub quality tooling + AWS sim KMS rotation / app-autoscaling / scheduler + terraform-test orphan-leak fix) |
| Open GitHub issues | #394 — azuread TF provider upstream blocker (waiting on hashicorp). #416/#417 closing via PR #418 |
| Bugs | 1462 filed · 1419 fixed · 5 open · 3 false positives |
| Open BUGs | BUG-1075 live-cloud validation; BUG-1104 audit cadence; BUG-1345 azuread upstream |
| Planned next | Finish ECS/DynamoDB audit follow-ups; then await new sim-fidelity issues |
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
