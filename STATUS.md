# Sockerless - Status

Roadmap [PLAN.md](PLAN.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Snapshot

| | |
|---|---|
| Active branch | `feat/aws-sim-ec2-launch-templates` (PR pending — EC2 Launch Template ops, BUG-1476, issue #433) |
| In-flight | AWS EC2 Launch Templates (BUG-1476, issue #433): `CreateLaunchTemplate`/`DescribeLaunchTemplates`/`DescribeLaunchTemplateVersions`/`DeleteLaunchTemplate` returned `InvalidAction`, blocking the fck-nat NAT-instance path (`nat_mode="instance"` uses `aws_launch_template`). Added a versioned launch-template store; `CreateLaunchTemplate` parses + persists the full `RequestLaunchTemplateData` and template tags; `DescribeLaunchTemplateVersions` renders `launchTemplateData` back at exact SDK locationNames (verified against `ec2@v1.305.2` deserializers) so it round-trips with no Terraform drift. SDK + CLI + Terraform coverage green (TF apply/destroy clean). |
| Last merged | PR #432 — real CloudWatch metrics (BUG-1475) |
| Also merged recently | PR #431 (IAM policy simulation #427); PR #430 (EC2 ENI ops #428); PR #429 (five fidelity-audit fixes) |
| Open GitHub issues | None actionable — only #394 (azuread TF provider upstream blocker) |
| Bugs | 1476 filed · 1432 fixed · 5 open · 4 false positives |
| Open BUGs | BUG-1075 live-cloud validation; BUG-1104 audit cadence; BUG-1345 azuread upstream |
| Planned next | Possible follow-ups: launch-template update-in-place ops (`CreateLaunchTemplateVersion`/`ModifyLaunchTemplate`) if a consumer needs them; query-protocol (aws CLI/botocore) CloudWatch metrics surface; other audit items; or await new issues |
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
