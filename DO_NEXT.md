# Do Next

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Current State

- Branch: `feat/aws-sim-consumer-batch-441` (PR pending — six consumer issues, BUG-1485–1490).
- Last merged: PR #448 (three flagged follow-ups, BUG-1482–1484).
- Consumer batch (#441–#447; #444 already fixed by #448, #394 upstream-blocked):
  - **#441 (BUG-1485)** IAM `ListPolicyVersions` — returns the policy's single `v1` (the aws_iam_policy destroy path).
  - **#442 (BUG-1486)** EC2 `DescribeVpcs` — multi-id + `ec2Filters` (vpc-id/cidr/tag) + render `cidrBlockAssociationSet`. New helpers `ec2VpcMatchesFilters`, `ec2TagFilterMatch`.
  - **#443 (BUG-1487)** EC2 `DescribeSecurityGroups` — route through `ec2Filters` (vpc-id/group-name/group-id/tag) + `ec2SecurityGroupMatchesFilters`.
  - **#445 (BUG-1488)** Logs `CreateLogGroup` kmsKeyId stored + echoed; `AssociateKmsKey`/`DisassociateKmsKey` added.
  - **#446 (BUG-1489)** ECS `DescribeClusters` — store Settings/Configuration on the cluster, surface them only when `include` has SETTINGS/CONFIGURATIONS.
  - **#447 (BUG-1490)** IAM `ListRoles` (path-prefix + paging) + tag storage (parse `Tags.member.N` at CreateRole/CreatePolicy, render in role/policy XML) + `ListRoleTags`/`ListPolicyTags`. (`ListPolicies` already existed.) New file `iam_lists.go`.
- Coverage: SDK + CLI for all six; TF stack augmented with aws_iam_policy (destroy → ListPolicyVersions), data.aws_vpc by-filter, ecs cluster settings/config, log-group kms_key_id.
- Flagged follow-ups closed (user asked to tie off the deferreds I noted in recent PRs):
  - **#433 follow-up (BUG-1482)** — EC2 launch-template in-place update: `CreateLaunchTemplateVersion` (appends a version, becomes latest not default) + `ModifyLaunchTemplate` (moves the default; numeric/`$Latest`/`$Default`). Gotcha: the wire param for the default selector is `SetDefaultVersion`, NOT `DefaultVersion`.
  - **#435 follow-up (BUG-1483)** — ECR `aws_ecr_repository` read-back: `CreateRepository`/`DescribeRepositories` now echo imageTagMutability (default MUTABLE), encryptionConfiguration (default AES256), imageScanningConfiguration (default scanOnPush=false); `aws_ecr_repository` added to the TF stack (the resource deferred from #435).
  - **#432 follow-up (BUG-1484)** — CloudWatch CLI query metrics: `cloudwatch_metrics_query.go` registers PutMetricData/GetMetricStatistics/ListMetrics on the query router, sharing the `cwMetrics` store + `cwApplyStat`. The Go SDK still uses the rpc-v2-cbor path; both protocols round-trip the same store.
- Coverage: SDK (LT versions, ECR config) + CLI (CloudWatch query) + TF (`aws_ecr_repository` in the prod-shape stack).
- Five-issue batch (user asked to bundle all open non-upstream-blocked issues in one PR):
  - **#434 KMS** (`kms_grants.go`): grant store + CreateGrant/ListGrants/RevokeGrant; GenerateDataKeyWithoutPlaintext (encrypted-only) + ReEncrypt (decrypt-src-envelope → rewrap-dest) over the existing kms-sim envelope.
  - **#435 ECR** (`ecr_layers.go`): repo-policy store (Set/Get/Delete) + real layer pipeline (Initiate→Upload→Complete with `sha256(buffer)==digest` verify→GetDownloadUrl); `BatchCheckLayerAvailability` now reports real availability. (No `/v2/` OCI registry — these are the awsJson SDK/CLI ops, not docker-push.)
  - **#436 ECS** (`ecs_service.go`): DescribeCapacityProviders (built-in FARGATE/FARGATE_SPOT ACTIVE + cluster-referenced customs, name filter, MISSING failure) + ListTaskDefinitionFamilies (dedup over `ecsTaskDefinitions`, prefix/status filters, pagination).
  - **#437 EC2** (`ec2.go`): DescribeInstanceTypeOfferings — instance-type × location offerings, honours `instance-type`/`location` filters + LocationType.
  - **#438 ELBv2** (`elbv2_rules.go`): rule store + Create/Describe/Modify/DeleteRule (conditions parsed from BOTH the CLI legacy `Values` and TF typed `*Config`, rendered as both; actions forward/fixed-response/redirect with typed round-trip) + ModifyListener. Extended `ELBv2Action` + shared `elbv2ActionsXML`.
- Coverage: SDK + CLI tests for all five; Terraform `aws_lb_listener_rule` + `aws_kms_grant` added to `terraform-tests/main.tf` (TestStackProductionShape apply/destroy clean). ECR repo-policy intentionally SDK+CLI-only (a TF `aws_ecr_repository` would risk unrelated read-back drift in the shared stack — noted as a follow-up).
- AWS EC2 Launch Templates (BUG-1476, issue #433): all four ops returned `InvalidAction` — `CreateLaunchTemplate`, `DescribeLaunchTemplates`, `DescribeLaunchTemplateVersions`, `DeleteLaunchTemplate` — blocking the fck-nat NAT-instance Terraform path (`nat_mode="instance"` uses `aws_launch_template` as the ASG launch config). Added `simulators/aws/ec2_launch_template.go`: a versioned launch-template store keyed by `lt-…` (default = `$Default`); `CreateLaunchTemplate` parses + persists the full `RequestLaunchTemplateData` (ImageId/InstanceType/KeyName/UserData/EbsOptimized/IamInstanceProfile/NetworkInterfaces+groupSet/SecurityGroupIds/BlockDeviceMappings+Ebs/TagSpecifications/MetadataOptions/Monitoring/Placement) and template tags; `DescribeLaunchTemplates` honours `LaunchTemplateId`/`LaunchTemplateName` filters; `DescribeLaunchTemplateVersions` returns stored versions (with `$Latest`/`$Default`/numeric/Min/Max selectors) rendering `launchTemplateData` back at exact SDK locationNames (verified against `ec2@v1.305.2` deserializers); `DeleteLaunchTemplate` removes + echoes. SDK + CLI + Terraform coverage (TF apply/destroy clean → no drift).
- **Possible follow-up (launch-template update-in-place):** the read/create/delete lifecycle (the four ops) covers `aws_launch_template` apply + destroy. An in-place *change* to a launch template makes the AWS provider call `CreateLaunchTemplateVersion` + `ModifyLaunchTemplate` (set default version) — not yet implemented. Add if a consumer mutates a template in place.
- **Possible follow-up (the metrics CLI gap):** the aws CLI (botocore) uses the legacy **query protocol** for CloudWatch (not rpc-v2-cbor) so `aws cloudwatch put-metric-data`/`get-metric-statistics` return `InvalidAction`. Implementing the query-protocol metric ops (backed by the same `cwMetrics` store) would make the CLI work. Separate, sizable.
- After this: no actionable consumer issues (only #394, upstream-blocked). Other audit items (IMDS accountId, GCS preconditions, ACR checkNameAvailability) or await the consumer's next batch.
- Open GitHub issues: #394 (upstream-blocked) — the consumer issue queue is otherwise drained by this PR.
- Open BUG trackers: BUG-1075, BUG-1104, BUG-1345.
- BUG counters: 1490 filed · 1446 fixed · 5 open · 4 false positives.

## Recently Completed

| PR | Description |
|----|-------------|
| #401 | bleephub auth conformance: session/CSRF OAuth flow + site-admin org endpoint |
| #402 | Phase C (AWS): pagination on 12 list endpoints |
| #403 | Phase C (GCP/Azure): pagination on GCP/Azure list endpoints |
| #404 | Phase D: error envelope fidelity + negative-path SDK error classification tests |
| #405 | Phase E+F: Azure KV data-plane CLI tests; 12 bleephub surface table files; webhook schema fixes (BUG-1396–1398) |

## Deferred / Blocked

| Item | Blocker |
|------|---------|
| `azuread_group` / `azuread_user` Terraform tests (BUG-1345) | Upstream: no `microsoft_graph_endpoint` override in `hashicorp/terraform-provider-azuread` (issue #1837 upstream, issue #394 here) |
| Live-cloud validation (BUG-1075) | Requires authenticated real-cloud runs; no timeline |

## What to Work On Next

**Phase G — New cloud service slices** (see PLAN.md for candidates). Each new slice ships with SDK + CLI + Terraform coverage per standard contract. No scope finalised yet — discuss with user before starting.

## Start Checklist (every session)

1. `git fetch origin && git checkout main && git reset --hard origin/main`
2. `gh issue list --state open --limit 30`
3. Check current open BUGs and the counter in `BUGS.md`.
4. Create a fresh branch from `origin/main`.
5. File BUG entries in `BUGS.md` **before** writing any code.
6. Run `go test ./...` in affected modules after every meaningful edit.

## Rules

- No stubs, fakes, mocks, synthetic responses, or silent fallbacks.
- Every new simulator public API path: SDK + CLI + Terraform coverage where those surfaces exist.
- One PR per cloud area; do not split into sub-phases.
- User merges PRs — never run `gh pr merge`.
- Rebase PR branch on `origin/main` before push.
- File bugs before fixes, not after.
