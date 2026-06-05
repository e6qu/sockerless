# Sockerless - Status

Roadmap [PLAN.md](PLAN.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Snapshot

| | |
|---|---|
| Active branch | `feat/aws-sim-fck-nat-onion-batch` (PR pending — five AWS sim gaps, BUG-1477–1481, issues #434–#438) |
| In-flight | Five AWS simulator gaps bundled in one PR (the next fck-nat onion layers + consumer read-completeness): **#434** KMS grants (CreateGrant/ListGrants/RevokeGrant) + GenerateDataKeyWithoutPlaintext/ReEncrypt; **#435** ECR repository policy + image-layer push/pull pipeline (Initiate/Upload/Complete/GetDownloadUrl, real content-addressed blobs, BatchCheckLayerAvailability now real); **#436** ECS DescribeCapacityProviders + ListTaskDefinitionFamilies; **#437** EC2 DescribeInstanceTypeOfferings; **#438** ELBv2 listener rules (CreateRule/DescribeRules/ModifyRule/DeleteRule) + ModifyListener. Query-protocol ops (EC2/ELBv2) rendered at exact `ec2@v1.305.2` / `elasticloadbalancingv2@v1.55.3` locationNames. SDK + CLI coverage for all five; Terraform: `aws_lb_listener_rule` + `aws_kms_grant` added to the production-shape stack (apply/destroy clean). |
| Last merged | PR #439 — EC2 Launch Template ops (BUG-1476, #433) |
| Also merged recently | PR #432 (real CloudWatch metrics #1475); PR #431 (IAM policy simulation #427); PR #430 (EC2 ENI ops #428) |
| Open GitHub issues | None actionable — only #394 (azuread TF provider upstream blocker) |
| Bugs | 1481 filed · 1437 fixed · 5 open · 4 false positives |
| Open BUGs | BUG-1075 live-cloud validation; BUG-1104 audit cadence; BUG-1345 azuread upstream |
| Planned next | Possible follow-ups: launch-template update-in-place (`CreateLaunchTemplateVersion`/`ModifyLaunchTemplate`); ECR `aws_ecr_repository` Terraform read-back completeness (image_tag_mutability/encryption/scanning) if a consumer adds the resource; query-protocol CloudWatch metrics; or await new issues |
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
