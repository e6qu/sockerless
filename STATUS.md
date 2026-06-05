# Sockerless - Status

Roadmap [PLAN.md](PLAN.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Snapshot

| | |
|---|---|
| Active branch | `feat/aws-sim-consumer-batch-441` (PR pending — six consumer issues, BUG-1485–1490, #441–#443/#445–#447) |
| In-flight | Six consumer issues bundled: **#441** IAM ListPolicyVersions (aws_iam_policy destroy); **#442** DescribeVpcs filtering + cidrBlockAssociationSet; **#443** DescribeSecurityGroups vpc-id filter; **#445** CloudWatch Logs CreateLogGroup kmsKeyId (+Associate/DisassociateKmsKey); **#446** ECS DescribeClusters settings/configuration (include-gated); **#447** IAM ListRoles + ListRoleTags/ListPolicyTags (+ role/policy tag storage). #444 (ECR scan/encryption config) was already fixed by #448. SDK + CLI for all six; TF stack augmented (aws_iam_policy, data.aws_vpc filter, ecs cluster settings, log-group kms). |
| Last merged | PR #448 — three flagged follow-ups (BUG-1482–1484) |
| Also merged recently | PR #440 (five AWS sim gaps #434–#438); PR #439 (EC2 Launch Template ops #433); PR #432 (real CloudWatch metrics #1475) |
| Open GitHub issues | None actionable — only #394 (azuread TF provider upstream blocker) |
| Bugs | 1490 filed · 1446 fixed · 5 open · 4 false positives |
| Open BUGs | BUG-1075 live-cloud validation; BUG-1104 audit cadence; BUG-1345 azuread upstream |
| Planned next | Consumer issue queue drained (only #394 upstream-blocked). Options: fresh fidelity audit; Phase G new slices (GCP Spanner/Dataflow/Bigtable, Azure); or await new consumer issues |
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
