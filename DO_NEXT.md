# Do Next

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Current State

- Branch: `feat/ecr-oci-manifest-head-465` (PR pending — OCI `/v2/` registry header fidelity, #465, BUG-1504).
- **#465 finding:** consumer reported ECR manifest HEAD returns 400 (go-containerregistry); **could not reproduce on current main** — built a throwaway g-c-r client, HEAD→404 across 1/2/3-segment repos and the full push works. Real defect fixed: only `GET /v2/` set `Docker-Distribution-Api-Version`; now set on **every** `/v2/` response in the shared `serve()` (all 3 cloud copies identical). Regression test `TestECR_OCIManifestHeadMissing` (sdk-tests/ecr_oci_test.go) locks missing-tag HEAD→404 + header + push round-trip. Honest: hardening, not a proven 400 repro — g-c-r doesn't key on the header for manifest HEAD. **Next: ask the consumer (via #465) for their exact commit SHA + g-c-r version + proxy/auth to pin the discrepancy.**
- Merged earlier today: PR #466 (#457–#464, BUG-1497–1503), PR #463 (#453–#455 + PM-artifact sweep). All consumer issues #453–#464 closed; #465 open (this branch), #394 upstream-blocked.
- **Seven consumer fixes (#457–#464):** all reproduced against a running sim with the real `aws` CLI first (ground truth), then SDK + CLI tests + a `terraform plan -detailed-exitcode==0` idempotency stack (`terraform-tests/idempotency-fidelity/`, covers 6 end-to-end; #460 SDK+CLI-only).
  - **#457 (BUG-1497)** SG-rule `sgrItemXML` omits `<fromPort>`/`<toPort>` when `IpProtocol=="-1"`.
  - **#458 (BUG-1498)** `referencedGroupInfo` omits `<userId>` for same-account refs (provider was prefixing `userId/sg-id` under `skip_requesting_account_id`).
  - **#459 (BUG-1499)** `EC2NatGateway.ConnectivityType` parsed at create (default `public`), rendered in Create + Describe.
  - **#460 (BUG-1500)** `ECSContainerDefinition` gains `HealthCheck`/`Secrets json.RawMessage` passthrough.
  - **#461 (BUG-1501)** `DescribeCapacityReservation` drops the always-zero `MinimumLoadBalancerCapacity` (no setter exists). Issue mis-titled `DescribeLoadBalancerAttributes`.
  - **#462 (BUG-1502)** create-time tags stored + returned for CW Logs / DynamoDB / ECR (real ARN lookups, not stubs). ECS task-def tags already worked.
  - **#464 (BUG-1503)** `CreateListener` parses nested `Certificates.member.N.CertificateArn` (via existing `parseELBv2Certificates`).
- Last merged: PR #456 (shared OCI /v2/ data plane, BUG-1491–1493, #450–#452).
- **Three consumer fixes (#453/#454/#455):**
  - **#453 DynamoDB SSE (BUG-1494):** `DDBTable` gains `SSEDescription{Status,SSEType,KMSMasterKeyArn}`; CreateTable parses `SSESpecification` (Enabled → ENABLED, SSEType default KMS); DescribeTable echoes it.
  - **#454 ECS deploymentConfiguration (BUG-1495):** `ECSService` gains `DeploymentConfiguration json.RawMessage` (same pattern as networkConfiguration); CreateService stores it, DescribeServices echoes it.
  - **#455 EC2 ModifySecurityGroupRules (BUG-1496):** new op — updates each `SecurityGroupRule.N` (by `SecurityGroupRuleId`) in `ec2SecurityGroupRules`; NotFound for an unknown id. The provider's in-place SG-rule update path.
  - SDK + CLI for each.
- **Repo-wide PM-artifact sweep:** removed every project-management reference (BUG-NNNN, issue/PR #NNN, roadmap Phase NNN, `(#NNN)`) from SOURCE — comments, identifiers, file names — across ~120 files (~240 occurrences). Done in passes: automated parenthetical strips, then automated inline-formula strips, then manual rephrasing of the residue. **Kept** (NOT PM artifacts): the gitlab-e2e `Phase 1-6` step narrative (`tests/gitlab_runner_e2e_test.go`) and bleephub `issue #1` test data (`bleephub/test/run-gh-test.sh`). See [[feedback-no-phase-mentions]] (now broadened to file names + identifiers, not just comments).
- **Cross-cloud OCI Distribution `/v2/` Docker Registry data plane** (real `docker push`/`pull` through the shim), one shared library wired into all three sims:
  - New `simulators/<cloud>/shared/oci.go` (package `simulator`, identical copy per cloud) = `sim.OCIRegistry`: GET /v2/ base; blob upload POST(init/monolithic)/PATCH(chunk)/PUT(finalize) with sha256 digest verify; blob GET/HEAD; manifest PUT/GET/HEAD/DELETE stored by tag+digest (DELETE removes all aliases); tags/list. Mounted per-method on the /v2/ subtree (GET covers HEAD) — avoids the awsJson `POST /` and apigatewayv2 `/v2/apis` mux conflicts. Hooks: `OnManifestPut`, `HydrateManifest`, `SkipPath`.
  - **#450 AWS ECR** (`ecr_oci.go`): new — `OnManifestPut` registers an ECR image row.
  - **#451 GCP AR** (`artifactregistry.go`): replaced the inline /v2/ handler (which lacked chunked PATCH); kept pull-through docker-hub hydration via `HydrateManifest` + DockerImage rows via `OnManifestPut`; `SkipPath` for /v2/projects/.
  - **#452 Azure ACR** (`acr.go`): replaced the per-method /v2/ handlers (POST mis-routed for multi-segment repos); `/acr/v1/_catalog` + `_tags` read `reg.Manifests`.
- **Test-host gating added (fixes pre-existing macOS failures):** GCP/Azure Compute+Network real-exec tests now skip when `realexec.DetectNetworkCapabilities().Require()` fails (off-Linux / no ip+nft+sysctl+CAP_NET_ADMIN) — they run for real on the sudo+iproute2/nftables CI runner. The EventGrid CLI publish now POSTs to the loopback base URL with a `Host: <topic>.eventgrid.localhost` header (the sim routes /api/events by Host), removing the `*.localhost` DNS dependency. `realexec` added (local replace) to gcp/azure {sdk,cli}-tests go.mod.
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
- Open GitHub issues: #394 (upstream-blocked). #453/#454/#455 fixed by this PR.
- Open BUG trackers: BUG-1075, BUG-1104, BUG-1345.
- BUG counters: 1496 filed · 1452 fixed · 5 open · 4 false positives.

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
