# Sockerless - Status

Roadmap [PLAN.md](PLAN.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Snapshot

| | |
|---|---|
| Active branch | `feat/aws-sim-batch-457` (PR pending — seven AWS terraform-idempotency read-back fidelity gaps, #457–#464) |
| In-flight | **Seven consumer fixes (BUG-1497–1503):** SG-rule `-1` port omission (#457) + bare `referenced_security_group_id` (#458); NAT `connectivity_type` (#459); ECS containerDefinitions `healthCheck`+`secrets` (#460); ELBv2 `DescribeCapacityReservation` omits unset `MinimumLoadBalancerCapacity` (#461); create-time tags for CW Logs / DynamoDB / ECR (#462); HTTPS listener `Certificates` round-trip (#464). SDK + CLI each, plus a `terraform plan -detailed-exitcode==0` idempotency stack (`terraform-tests/idempotency-fidelity/`) covering six end-to-end. |
| Also pending (not merged) | PR #463 — `feat/aws-sim-batch-453` (#453/#454/#455 + repo-wide PM-artifact sweep) |
| Last merged | PR #456 — shared OCI /v2/ data plane (BUG-1491–1493, #450–#452) |
| Also merged recently | PR #449 (six consumer issues #441–#447); PR #448 (three flagged follow-ups); PR #440 (five AWS sim gaps #434–#438) |
| Open GitHub issues | Only #394 (azuread TF provider upstream blocker). #453–#455 fixed by PR #463; #457–#464 fixed by this branch. |
| Bugs | 1503 filed · 1459 fixed · 5 open · 4 false positives |
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
