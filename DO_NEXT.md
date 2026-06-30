# Do Next

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) - coverage [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).

## Current branch

`feat/aws-sim-revoke-filter-validation` — fix AWS simulator EC2 security-group revoke-not-found and CloudWatch Logs filter-pattern validation (BUG-2262/2263).

---
### Current task: AWS sim revoke/filter validation — DONE

The branch fixes two simulator fidelity gaps filed from open issues #722 and #723:

- **BUG-2262 — EC2 `RevokeSecurityGroupIngress`/`Egress` succeed for non-existent rules.** `simulators/aws/ec2.go` now checks whether the requested permission exists before mutating the security group; a second revoke of the same rule returns `InvalidPermission.NotFound`, matching the AWS SDK for Go v2 documentation for non-default VPCs. A new `ec2PermissionExists` helper compares protocol, ports, and CIDR ranges.

- **BUG-2263 — CloudWatch Logs `PutMetricFilter`/`PutSubscriptionFilter` stored invalid `FilterPattern` values.** `simulators/aws/cloudwatch_logs_ops.go` now calls `cwCompileLogPattern` in both handlers and returns `InvalidParameterException` for malformed patterns, matching the CloudWatch Logs Smithy model. `simulators/aws/cloudwatch_filter_pattern.go` was also corrected so that `{` (an unbalanced brace) is rejected as a malformed structured pattern instead of treated as an unstructured term.

SDK and CLI tests were added for both fixes:
- `simulators/aws/sdk-tests/ec2_networking_coverage_test.go` — `TestEC2_RevokeSecurityGroupRules`
- `simulators/aws/cli-tests/ec2_networking_coverage_test.go` — `TestEC2CLI_RevokeSecurityGroupRules`
- `simulators/aws/sdk-tests/cloudwatch_logs_failloud_test.go` — `TestCloudWatchLogs_PutMetricFilterRejectsInvalidPattern`, `TestCloudWatchLogs_PutSubscriptionFilterRejectsInvalidPattern`
- `simulators/aws/cli-tests/cloudwatch_logs_ops_test.go` — `TestLogs_PutMetricFilterCLIRejectsInvalidPattern`, `TestLogs_PutSubscriptionFilterCLIRejectsInvalidPattern`

**CI-caught follow-up: BUG-2264 — VPC security groups created without the default ALLOW ALL egress rule.** After the revoke-not-found fix landed, the AWS Terraform production-shape test (`TestStackProductionShape`) failed because `terraform-provider-aws` revokes the default egress rule that real AWS creates with every VPC security group. Fixed in `simulators/aws/ec2.go` by initializing `IpPermissionsEgress` with the default rule in `handleCreateSecurityGroup` when `VpcId` is present. Existing SDK tests that assumed empty egress were updated to revoke the default first; `TestStackProductionShape` now passes.

All targeted SDK/CLI tests, the full AWS SDK test suite, AWS sim unit tests, `TestStackProductionShape`, and `make lint` pass.

**Next:** PR #725 CI is green. Awaiting user merge. After merge, rotate continuity files to the post-#725 state.

---
### Prior branch (merged #724): bleephub + UI audit (BUG-2261)
`feat/bleephub-comprehensive-audit-2026-06-29` fixed the GraphQL panic on `query($A:){A}` via `graphqlValidateNoPanic` in `bleephub/gh_graphql.go`, fixed the `DataTable` column-merging rendering bug in `ui/packages/core/src/components/DataTable.tsx`, simplified the workflow run detail jobs table in `ui/packages/bleephub/src/pages/WorkflowDetailPage.tsx`, added Playwright console/page-error failure hooks, and spot-checked GitHub API fidelity with curl. bleephub Go tests/race/lint, UI typecheck/test/build, and Playwright e2e (21/21) pass.
`feat/bleephub-ui-audit-2026-06-29` fixed org-aware PR owner rendering: GraphQL `PullRequest.headRepositoryOwner` now resolves the organization from `repo.FullName` for org-owned repos, and REST PR `head.user`/`base.user` now use the snake_case `simple-user` shape via the new `repoOwnerREST` helper. Added `TestPRGraphQL_OrgOwnedHeadRepositoryOwner` and extended `TestCreatePullRequestREST`. Playwright e2e passed with 31 screenshots; extended fuzz targets passed; UI tests/typecheck/build and Go tests/race/lint all pass.

---
### Prior branch (merged #718): continuity rotation after #717
No code change; `STATUS.md`/`DO_NEXT.md` reconciled to the post-#717 state.

---
### Prior branch (merged #717): bleephub fidelity audit (BUG-2256/2257)
`feat/bleephub-fidelity-audit-2026-06-29` implemented runner `AgentRefreshMessage` broker delivery (`sendAgentRefreshMessage` in `broker.go` + site-admin `POST /internal/agents/{agent_id}/refresh-message` in `handle_mgmt.go`), fixed GraphQL `repositoryOwner(login:)` to return real organization data via `orgToGraphQL` instead of a synthetic partial User-shaped payload, and corrected the stale "Artifact + cache stubs" comment in `server.go`. Added `broker_refresh_test.go` and `TestRepoGraphQL_RepositoryOwnerOrg`. All bleephub Go tests, fuzz targets, race tests, UI tests/typecheck/build, and both Docker integration test suites pass; `make lint` clean.

---
### Prior branch (merged #716): continuity rotation after #715
No code change; `STATUS.md`/`DO_NEXT.md` reconciled to the post-#715 state.

---
### Prior branch (merged #715): AWS Budgets Terraform parity (#714, BUG-2255)
`fix/aws-budgets-terraform-parity-714` closed the Terraform lifecycle gaps in the AWS Budgets service slice. `CreateBudget`/`DescribeBudget`/`DeleteBudget`/`UpdateBudget`/`DescribeBudgets` now derive `AccountId` from `awsAccountID()` when the request omits it, matching real AWS behavior when the caller's signing credentials supply the account (the path used by `terraform-provider-aws` with `skip_requesting_account_id = true`). `ListTagsForResource`, `TagResource`, and `UntagResource` are implemented so `aws_budgets_budget` can complete its Create+Read+Delete cycle. SDK tests cover tag round-trips and the implicit-account raw-HTTP path; the terraform-tests production-shape stack gained an `aws_budgets_budget` resource plus endpoint alias and assertions. Boyscout: corrected the SQS missing-queue error `__type` to `AWS.SimpleQueueService.NonExistentQueue`.

---
### Prior branch (merged #713): AWS simulator stored-but-not-enforced sweep + Budgets service slice (#703-#712, BUG-2242 through BUG-2251, plus CI-caught BUG-2253/2254)
Closed all ten open AWS-focused GitHub issues. Each fix ships real side effects and SDK tests: SQS DLQ redrive, ACM real PEM minting, AWS Budgets service slice, Route 53 DNS server, CloudWatch Logs metric-filter→metric publishing, CloudWatch alarm→SNS dispatch, Application Auto Scaling target tracking for ECS, ELBv2 HTTPS/TLS termination, ECS service scheduler, and EC2 security-group host-firewall enforcement. Added `allowedNonSpecTargets` to `spec_conformance_test.go` for the Budgets service, which is real but not in the vendored Smithy corpus. Added deterministic unit tests for the ECS scheduler because the SDK integration test requires a healthy container runtime. Identified and filed BUG-2252: the conformance/coverage gates do not catch behavioral side-effect gaps (background evaluators, protocol listeners, cross-service dispatch); documented in WHAT_WE_DID.md.

The PR #713 CI run surfaced two regressions in the new code and they were fixed in the same branch:
- **BUG-2253:** `TestECS_ServiceScheduler_ReconcilesDesiredCount` flaked because `DescribeServices.RunningCount` lagged behind `DescribeTasks.LastStatus`. Fixed by computing the counts from the live task set in `handleECSDescribeServices`.
- **BUG-2254:** #712's host-firewall enforcement installed a deny-all filter when an awsvpc task/ENI had no explicit security groups, breaking VPC reachability tests. Fixed by clearing the ingress filter instead of applying an empty ruleset when the SG list is empty.

---
### Prior branch (merged #702): second GCP gRPC round (Cloud KMS + Secret Manager) + Compute v1 control-plane tranche #2 (BUG-2240) + AWS ECS ExecuteCommand flake fix (BUG-2241)
`feat/gcp-ratchet-5-grpc` — second GCP gRPC round (Cloud KMS + Secret Manager) + Compute v1 control-plane tranche #2 (BUG-2240), plus boyscout fix for AWS ECS ExecuteCommand flake (BUG-2241). gcp build/lint(0)/vet clean; new SDK tests green; conformance gates green; AWS ECS exec tests updated for the systematic RUNNING-after-start fix. Merged via PR #702.

---
### Prior branch (merged #700): native gRPC data planes for Firestore/Pub/Sub/Spanner + Compute v1 control-plane tranche (BUG-2239)
The high-level Go SDK clients `cloud.google.com/go/{firestore,pubsub,spanner}.NewClient` are gRPC-only and previously could not target the sim at all; this PR extended the Bigtable gRPC transport pattern (#656) to all three, each sharing the existing REST slice's store. Firestore gRPC (~980 lines, full document CRUD + server-streaming BatchGetDocuments/RunQuery reusing the REST evaluator + transactions + BatchWrite; Listen/Write Unimplemented with justification). Pub/Sub gRPC (~1300 lines, Publisher+Subscriber+SchemaService with real at-least-once delivery via a background ack-deadline sweeper; review caught + fixed 4 `projects/projects/...` double-prefix bugs). Spanner gRPC (~520 lines, the defining ExecuteSql/Read need real table storage → per-database in-memory SQLite engine reconciled from `spannerDDLs`, runs REAL parameterized queries — no stubs). Plus a Compute v1 metadata tranche (`compute_more2.go`, 440→864/1994) + IAM triplets on 10 resources; review caught + fixed a nodeTypes catalog fake (hardcoded cpu/memory → real per-name specs). Boyscout: pre-push spanner v1.91.0→v1.92.0 dep bump. gcp build/lint(0)/vet clean; all _GRPC SDK suites green (×2); 0 new spec violations.

---
### Prior branch (merged #698): Bigtable gRPC data-plane coverage close-out (BUG-2237) + CloudBuild test-hang fix (BUG-2238) + no-skip-if-absent rule
PR #656 had already landed the Bigtable Data API v2 gRPC slice; #698 closed its coverage gaps: 18 SDK filter subtests (one per implemented RowFilter, real survival semantics), DeleteCellsInColumn/Family/TimestampRange mutations, AppendValue, ApplyBulk (incl. partial-failure per-entry status), SampleRowKeys, and an unconditional `cbt` CLI test (built in TestMain via `go install cloud.google.com/go/bigtable/cmd/cbt@v1.13.0` — the reference implementation of the new no-skip-if-absent rule). Boyscout: bounded every `docker` call in `TestCloudBuild_FaithfulBuildPush`/setup via `dockerCLIWithTimeout` + a 120s build POST (BUG-2238 — a wedged container runtime now fails in ~3min instead of hanging the sdk-test suite 8–10m); added the "No skip-if-absent tests" section to AGENTS.md; pre-push dep bumps (smithy-go v1.27.2→v1.27.3, configure-aws-credentials v6.2.0→v6.2.1).

---
### Prior branch (merged #697): GCP ratchet round 3 (BUG-2235) + realexec netns robustness (BUG-2236)
Compute Engine v1 174→440/1994 (control plane: images/snapshots, load balancers, instance-group managers, instance actions, catalog reads — metadata-only/host-agnostic); Cloud Run v2 62→89/116 (worker pools, instances, UpdateJob/DeleteExecution/Tasks + a `:getIamPolicy` colon-split fix); GCP 3180→3473/5244 (61%→66%). Plus a CI-caught realexec fix: `CreateNetworkNamespace` reclaims an orphan netns before retrying once (GCP ratchet's Compute SDK test materializes a host-global netns; a killed-process leak made the next sim process fail `ip netns add …: File exists`).

---
### Prior branch (merged #696): fourth Azure service ratchet (BUG-2234)
Logic Apps 100%, App Service/Web Apps 37→161/692, Cosmos DB both versions 100% + PEC + Log Analytics query, API Management + PostgreSQL to 100%, Resources 36/40; Azure 1409→1758/2597 (54%→68%). Plus BUG-2233 (Web FunctionEnvelope name leak).

---
### Prior branch (merged #695): third Azure service ratchet (BUG-2229) + CI-caught fixes (BUG-2230/2231/2232)
Cosmos DB (Mongo/Cassandra/Gremlin families) + Event Grid (both docs 100%, partner family) + API Management (apis 52/91, five docs 100%) + PostgreSQL/Resources/subscriptions/App Insights; Azure 1000→1409/2597. Plus CI-caught fixes: async-op Retry-After (30s→1s polls), CLI timeout budget, Event Grid keyGeneration leak, and a GCP dep-cascade build fix.

---
### Prior branch (merged #694): second Azure service ratchet (BUG-2226) + CI-caught fixes (BUG-2227)
Storage ARM (blob/file/queue/table 100%), DNS/Private DNS/LB/NIC/Public IP/VNet all 100%, Redis/Key Vault/Managed Identity all 100%, Container Instances 100% + RBAC up; Azure 857→1000/2597. Plus two CI-caught test fixes (org-account-ordering flake, stale KeyPermission assertion).

---
### Prior branch (merged #693): first Azure service ratchet (BUG-2224) + EC2 ClientToken idempotency (BUG-2225)
Container Apps / Container Registry / Service Bus + Event Hubs all to 100%, Networking up; Azure 630→857/2597. Plus a CI-caught boyscout fix: EC2 `RunInstances` honors `ClientToken` idempotency.

---
### Prior branch (merged #692): ELBv2 NLB stable DNSName (#691, BUG-2223)
Reverted #683's host:port DNSName hijack — DescribeLoadBalancers returns the stable AWS-shaped hostname again; reachability via listener-port bind + ExtraHosts hostname resolution (per-NLB loopback IP on Linux). Plus the appdata CLI shard split (flakiness).

---
### Prior branch (merged #690): ELBv2 TCP target group HealthCheckPath (#688, BUG-2222)
Same HTTP-only class as #685's Matcher — `HealthCheckPath` was defaulted/emitted for every protocol; now omitted for non-HTTP health checks (`elbv2MatcherApplies` → `elbv2HTTPHealthCheck` + `elbv2DefaultedHealthCheckPath`). SDK/CLI + a TCP `health_check` block in the idempotency TF stack.

---
### Prior branch (merged #689): GCP coverage ratchet round 2 + Azure operation-coverage gate (BUG-2220/2221)
Built `azureMethodFloor` in `simulators/azure/azure_coverage_test.go` (the Swagger-spec analogue of `serviceCoverageFloor`/`gcpMethodFloor`, ratchet over 90 swagger files — all three sims now gated; Azure 630/2597 = 24%); GCP ratcheted 2413→3180/5244 (46%→61%) with ~22 services at 100% (Spanner, Cloud SQL v1, VPC Access, ServiceUsage, IAM Credentials, Dataflow to 100%; CRM v3 11→105, Logging 170→480, Bigtable 65→136, Cloud Run/Functions up); plus a smoke-build proxy-retry resilience fix (BUG-2221).

---
### Prior branch (merged #687): CI flake hardening + ELBv2 #685/#683 + CloudTrail (BUG-2216/2217/2218/2219)
- Flaky-pattern hardening across AWS/GCP/Azure test suites (~20 racy waits → poll-until / widened deadlines; no assertion weakened).
- ELBv2 #685: omit HealthCheck `Matcher` for non-HTTP/HTTPS health checks (terraform idempotency). ELBv2 #683: real NLB raw-TCP data plane, made discoverable via DescribeLoadBalancers (a client `net.Dial`s the reported endpoint). CloudTrail: added the missing ElastiCache `2015-02-02` eventSource mapping (events were being dropped).

---
### Prior branch (merged #686): GCP operation-coverage gate + ratchet (BUG-2214/2215)
Brought the GCP simulator's conformance gate up to AWS parity, all spec-validated against the Discovery schemas (0 new divergences) + real Google Cloud Go SDK.

- **GCP had route-validity + doc-consumption gates but no operation-coverage ratchet.** Built `gcpMethodFloor` in `gcp_coverage_test.go` — per vendored Google Discovery document, it counts how many REST methods the sim implements (a method is covered when a registered route matches its HTTP-method + normalized path under the same `matchSegs` rules the route-validity gate uses) and locks the count with an exact-equality ratchet. `TestServiceConformance_GCPCoverage` logs the per-doc fraction; `TestServiceConformance_GCPCoverageFloor` is the ratchet.
- **12 mid-size services ratcheted** (2 rounds, one service file per agent): **Cloud Build 104→130/130, Memorystore Redis 64→90/90, Firestore admin 89→112/112, Cloud Storage JSON 32→84/84 (all 100%)**; Cloud KMS 122→157/166 (real Go-stdlib crypto for mac/raw/asymmetric/generateRandomBytes; honest metadata-only for EKM/HSM/post-quantum decapsulate), IAM admin 204→264/266 (workload/workforce identity pools, OAuth clients, custom roles, SA keys), Artifact Registry 97→144/147 (packages/versions/tags/files/rules/attachments), Eventarc 97→124/132, BigQuery 38→86/95, Cloud DNS 24→74/80 (real DNSSEC DS digests), Pub/Sub 77→86/92 (schemas), Secret Manager 50→60/64 (regional).
- **GCP coverage 1986→2413/5244 (38%→46%); 6 GCP services now at 100%.**
- The consistently-uncovered remainder per service is the `{+name}`/`{+resource}` reserved-expansion *template alternates* the Discovery docs list alongside each flatPath — the flatPath form every real client uses is covered; matching the template form would need an over-broad catch-all the route-validity gate forbids.
- Integration: the whole module built once all agents finished; floor bumps reconciled from a single measured-coverage pass; 2 staticcheck (dns ECDSA embedded-field) + 1 unused func (iam) cleared.
- Tests: gcp sim build/lint(0) green; route-validity + doc-consumption + coverage-floor gates pass; per-service spec-validator 0 new violations.

**Next candidates:** keep ratcheting GCP — the larger mid-size services (**Spanner 186/198, SQL Admin 136/148**, the small-gap batch **API Gateway 54/60 / ServiceUsage 19/20 / VPC Access 15/16 / IAM Credentials 11/14**), then the big surfaces (**Cloud Run 61/152, Logging 154/508, Bigtable Admin 62/162, Cloud Resource Manager 11/124, Cloud Functions 15/42, Dataflow 8/84**; Compute 174/1994 is enormous — ratchet selectively). Then the **Azure simulator** (no coverage gate yet — build the equivalent), and the **live-cloud track (BUG-1075)**. Open GitHub issues: #394 (azuread upstream-blocked).

## Working agreement

The full before/after-task continuity-file workflow, the no-fakes rules, and branch/PR hygiene live in [AGENTS.md](AGENTS.md). In short: read `STATUS.md`/`DO_NEXT.md` first; run the narrowest meaningful tests for the touched area; file bugs before fixing; update the continuity files in the same commit as the code; rebase on `origin/main` before pushing; never merge the PR.

Narrowest-test recipes for the common surfaces:

```bash
# Simulator SDK probe
cd simulators/<cloud>/sdk-tests && GOWORK=off CGO_ENABLED=0 go test -tags noui -run '<pat>' -timeout 15m .
# Simulator module unit tests + lint
cd simulators/<cloud> && make unit-test
# A backend's unit tests
cd backends/<name> && GOWORK=off go test ./...
# bleephub runner topology harness (self-contained)
make bleephub-runner-docker-test
```
