# Sockerless - Status

Roadmap [PLAN.md](PLAN.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Snapshot

| | |
|---|---|
| Active branch | `fix/ecs-overrides-cloudtrail` (PR pending — consumer #530 + CloudTrail REST sweep) |
| In-flight | BUG-1580 and BUG-1540 are fixed on this branch: ECS `RunTask.overrides.containerOverrides` now affects the real task container runtime and task response shape; CloudTrail records REST/RPC management calls for Lambda, S3, API Gateway v1/v2, Batch, EFS, CloudFront, Amplify, Route53, and CloudWatch metrics, including failed-call error fields. |
| Last merged | PR #529 fixed ECS workspace blockers #526/#527 and Entra duplicate UPN #525. Earlier ECS VPC/netns/metadata/route-table/ExecuteCommand chain is merged through PR #524. |
| Open GitHub issues | #394 remains upstream-blocked; #530 is addressed by this branch. Check GitHub before starting the next consumer batch. |
| Bugs | 1580 filed · 1536 fixed · 5 open · 5 false positives |
| Open BUGs | BUG-1075 live-cloud validation; BUG-1104 audit cadence; BUG-1345 azuread upstream. |
| Planned next | After this PR: Phase G new-service-slice PR or the next actionable consumer issue. |
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
