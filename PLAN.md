# Sockerless - Roadmap

State [STATUS.md](STATUS.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Goal

Replace Docker Engine with Sockerless for Docker API clients such as `docker`, Docker Compose, Testcontainers, and CI runners, backed by real cloud infrastructure or high-fidelity local cloud simulators.

## Guiding Principles

1. Match public APIs exactly: Docker API, GitHub API (bleephub), and public cloud APIs (simulators).
2. No stubs, fakes, mocks, synthetic behavior, silent fallbacks, or degraded modes.
3. Simulators are real local cloud slices — one binary per cloud, not per product.
4. Every new simulator public API slice ships with official SDK + vendor CLI + Terraform-provider coverage.
5. Components remain decoupled. Admin UI and local gateway infrastructure must not become required dependencies of simulator public APIs.
6. User merges PRs. Agents create branches, commits, and PRs only.
7. Continuity docs are updated in every PR and written so they remain correct after the PR merges.

## Current Work

The current branch is `bleephub-parity-storage`. It is the single working branch
for a multi-session Bleephub parity and durability PR. Keep one PR open for this
branch, make granular commits, and update the continuity files before and after
each subtask so the next session can resume without relying on chat history.

### Bleephub parity and durability target

Bleephub should behave like a real GitHub Enterprise-style server for the
surfaces it exposes. The current audit found several gaps that must be treated as
implementation defects, not accepted compatibility shortcuts:

- Actions cache routes currently reserve nothing, discard uploads, and always
  miss. They must implement real cache records, immutable saved entries, restore
  key lookup, chunk upload storage, and successful restore/download behavior.
- Unhandled API paths currently return `200 OK`; they must return GitHub-shaped
  errors or Git smart-HTTP statuses.
- Persistence is SQLite-only and covers only a subset of state. Bleephub must
  support SQLite and PostgreSQL, with explicit configuration and no silent
  in-memory fallback when persistence is requested.
- Git smart HTTP exists and uses real go-git plumbing, but repo creation ignores
  git storage errors and git content only supports memory/filesystem storage.
  Bleephub must fail loudly on git-storage errors and support an S3/MinIO-shaped
  object-store backend for git content.
- Git clone/fetch/push must enforce repo visibility and token permissions.
- The UI hard-codes an admin token while the server requires
  `BLEEPHUB_ADMIN_TOKEN`; the UI needs a real operator/auth configuration path.
- Advertised long-tail GitHub surfaces such as Pages builds, audit log content,
  run artifact listings, environment approvals, and GraphQL status rollups still
  include shape-only or empty responses.
- Bleephub docs are stale around admin tokens, git storage, persistence, TLS, UI
  auth, and operator setup.

### Subtask queue for the single PR

Aim for one commit per item below. Do not split implementation and tests into
separate commits unless a CI-only fix is needed after review. Each commit should
leave the branch buildable or have the continuity docs explicitly call out the
temporary failing check and the next command to run.

1. **Baseline audit tests and error handling** — add focused tests for the known
   fake paths, change unmatched routes to GitHub/git-shaped errors, and refresh
   the parity notes with concrete current gaps.
2. **Actions cache and artifact indexing** — replace cache no-ops with real
   cache records, restore-key lookup, upload/finalize/download behavior, and
   run/repo artifact list indexing.
3. **Persistence abstraction** — keep SQLite support, add PostgreSQL support,
   define explicit env/config names, and fail loudly if the requested database
   cannot open or migrate.
4. **Persist the Bleephub state that users expect to survive restarts** — extend
   write-through/load coverage beyond users/tokens/apps/repos to issues, PRs,
   workflows, runners, hooks, checks, deployments, releases, and other exposed
   API state where the public API promises durability.
5. **Git storage hardening** — stop ignoring git init/storage errors, make repo
   deletion clean up durable git data where applicable, and enforce clone/fetch
   and push permissions.
6. **S3/MinIO git object storage** — add a real S3-compatible git content
   backend using an actual object-store client, with MinIO-based integration
   coverage and clear key layout docs.
7. **UI auth and operator status** — remove hard-coded admin credentials, add a
   real configured auth/session path, and expose storage/database/git backend
   status in the UI.
8. **UI/API parity gaps** — add or deepen UI/API coverage for caches, artifacts,
   webhooks/deliveries, orgs/teams, branch protection, audit events, and repo git
   refs where Bleephub already exposes the backing API.
9. **Long-tail fake removal and docs** — replace or explicitly delist shape-only
   endpoints for Pages builds, audit log events, run approvals, and GraphQL
   status rollups; update README, gh CLI docs, parity specs, build/run docs,
   Caddy/TLS notes, SQLite/PostgreSQL notes, and S3/MinIO git storage docs.
10. **Final verification and PR hygiene** — run Bleephub Go/UI tests, targeted
    real-client smoke tests, lint/guard checks, rebase on `origin/main`, push one
    PR, then sync local `main` after the user merges.

### Continuity protocol for this branch

Before starting any subtask:

1. Read `STATUS.md` and `DO_NEXT.md`.
2. Confirm the active branch and current subtask.
3. Check `git status --short --branch`.
4. Update `DO_NEXT.md` if the next command list is stale.

After finishing each subtask:

1. Run the narrowest meaningful tests for the touched area.
2. Update `STATUS.md` with completed work, current risks, and test state.
3. Update `DO_NEXT.md` with the next subtask, exact files likely involved, and
   the first verification command to run.
4. Add a short `WHAT_WE_DID.md` entry only when a meaningful chunk completed.
5. Commit the code, tests, and continuity docs together with a natural commit
   message that describes the user-facing change.

## Previous Completed Work

**Terraform idempotency drift sweep** (started in PR #491). The new second-plan
`terraform plan -detailed-exitcode` drift assertions on the gcp + azure apply
stacks (BUG-1532) surfaced ~13 pre-existing read-back fidelity bugs that cause a
non-empty second plan. PR #491 landed the green pieces (scheduler `cron(...)`
evaluation BUG-1531, the drift assertions themselves, and the
Docker-Hub→ECR-gallery harness fix BUG-1533), followed by the drift fixes below
to make `tf (gcp)` / `tf (azure)` green.

### Terraform drift fixes (BUG-1534, closing BUG-1532)

Root causes are the `# forces replacement` / `-> null` lines in the second plan;
the `(known after apply)` diffs are downstream cascades that clear once their
parent resource stops being replaced.

**GCP** (`simulators/gcp/`) — DONE (gcp drift reproduced + fixed locally; DNS
runs on Mac, compute verified blind via `tf (gcp)`):
1. **logging sink/metric** (`logging.go`) — `name` is the **short** identifier (verified vs logging/v2 `LogSink.name` doc); added the separate output-only `resourceName` full-path field for sinks. The SDK test asserted the full path against a false "Real GCP returns the full resource name" comment → corrected.
2. **memorystore redis** (`memorystore_redis.go`) — `connectMode`/`transitEncryptionMode` defaults.
3. **gcs bucket** (`gcs.go`) — `location` upper-cased.
4. **api gateway api-config** (`apigateway.go`) — `openapiDocuments` round-trip.
5. **compute router NAT** (`compute.go`) — the `type=PUBLIC` forces-replacement was **Cloud NAT**, not the network; default `type=PUBLIC`. Also network `networkFirewallPolicyEnforcementOrder=AFTER_CLASSIC_FIREWALL`.
6. **cloudfunctions2** (`cloudfunctions.go`) — `allTrafficOnLatestRevision=true` + `ingressSettings=ALLOW_ALL`.
7. **dns managed zone** (`dns.go`) — terraform sends `privateVisibilityConfig{networks:[]}` even for public zones; the provider flatten materialises a phantom block. Drop an empty `privateVisibilityConfig` from the read-back.

**AZURE** (`simulators/azure/`) — DONE (11 resources; verified blind via
`tf (azure)`, Docker-only so can't run on Mac):
8. **public IP / prefix / LB** (`compute.go`, `network.go`) — `sku_tier=Regional` (SkuName.Tier), `tags={}`, `zones=[]`, public-IP `ip_tags=[]` + `idle_timeout=4`.
9. **storage account** (`files.go`) — `sku.tier` from sku name (`account_tier`), `supportsHttpsTrafficOnly=true`, `minimumTlsVersion=TLS1_2`, `primaryLocation`.
10. **application_insights** (`insights.go`) — `application_type=web`, retention=90, sampling=100, public-network ingestion/query=Enabled, `tags={}`.
11. **cosmosdb_account** (`cosmos.go`) — round-trip `properties.locations` (`isZoneRedundant` was dropped) + write/readLocations.
12. **key_vault / key_vault_key** (`keyvault.go`) — round-trip `softDeleteRetentionInDays`; echo `key_ops` in request order.
13. **container_app** (`containerapps_apps.go`) — scale `cooldownPeriod=300`/`pollingInterval=30`.
14. **linux_function_app** (`functions.go`) — `clientCertMode=Optional` + siteConfig `loadBalancing`/`managedPipelineMode`/`ipSecurityRestrictionsDefaultAction` defaults.
15. **apim api** (`apim.go`) — `apiRevision=1` + `isCurrent`.
16. **container_registry** (`acr.go`) — `networkRuleBypassOptions=AzureServices`.
17. **virtual_network** (`network.go`) — `privateEndpointVNetPolicies=Disabled`.
18. **eventgrid_system_topic** (`eventgrid.go`) — `tags={}`.

**Second-pass residuals (after the cosmos provider-panic fix unblocked the azure
apply), all fixed:**
- cosmosdb_account — the geo_location fix had added read/writeLocations without
  `provisioningState`; the provider's create poll dereferences it → nil panic
  that aborted the whole apply. Rebuilt the shape with `failoverPolicies` (what
  geo_location actually reads) + a shared id + `provisioningState=Succeeded`.
- application_insights — round-trip `properties.WorkspaceResourceId` (workspace_id).
- container_app_environment — `log_analytics_workspace_id` is resolved by the
  provider via a subscription-scope workspace LIST matched by customerId; added
  that LIST handler (monitor.go) so the read recovers it.
- linux_function_app — mirror site-PUT siteConfig.appSettings into the
  /config/appsettings store (functions_extension_version / storage_account_name
  / builtin_logging_enabled); siteConfig minTlsVersion/scmMinTlsVersion/
  scmIpSecurityRestrictionsDefaultAction defaults; backup config (POST
  /config/backup/list) now 404s when unconfigured so the provider's
  FlattenBackupConfig doesn't materialise a phantom backup{enabled=false} block.

**DONE — `tf (gcp)` and `tf (azure)` both green; full PR #491 CI passes.**
Approach used: fix one cloud fully, push, read the next CI plan, repeat. Azure
verified blind via CI + per-resource curl wire-shape checks (azure terraform is
Docker-only, can't run on Mac).

## Completed Phases

- **Phase C** (PRs #402, #403): Token-based pagination on AWS/GCP/Azure list endpoints.
- **Phase D** (PR #404): Error envelope fidelity + negative-path SDK error classification tests.
- **Phase E** (PR #405): Azure KV data-plane CLI tests via `az rest` + Host header routing.
- **Phase F** (PR #405): 12 bleephub surface table files + coverage matrix rows.

### Completed Cloud Service Slice Expansion

The merged service-slice branch combined Azure and GCP service slices plus AWS
coverage-audit cleanup in one PR. Each new slice shipped with SDK + CLI +
Terraform coverage where the provider exposed the surface. Surface tables and
coverage matrix rows shipped in the same PR.

#### AWS

- **Step Functions**: state machine CRUD (`CreateStateMachine`, `DescribeStateMachine`,
  `ListStateMachines`, `DeleteStateMachine`) + execution lifecycle (`StartExecution`,
  `DescribeExecution`, `ListExecutions`, `StopExecution`). The simulator now
  executes supported ASL states instead of reporting unconditional success.
- **Batch**: job definitions (`RegisterJobDefinition`, `DescribeJobDefinitions`,
  `DeregisterJobDefinition`), job queues (`CreateJobQueue`, `DescribeJobQueues`,
  `DeleteJobQueue`), compute environments (`CreateComputeEnvironment`,
  `DescribeComputeEnvironments`, `DeleteComputeEnvironment`), job submission
  (`SubmitJob`, `DescribeJobs`, `CancelJob`). Submitted jobs now run real
  workload containers and report status from exit codes.
- **CodeBuild**: build project CRUD (`CreateProject`, `BatchGetProjects`, `ListProjects`,
  `DeleteProject`) + start build (`StartBuild`, `BatchGetBuilds`). Builds now
  run buildspec commands through a real process path.
- **Glue**: database CRUD (`CreateDatabase`, `GetDatabase`, `GetDatabases`,
  `DeleteDatabase`), table CRUD (`CreateTable`, `GetTable`, `GetTables`, `DeleteTable`),
  job CRUD (`CreateJob`, `GetJob`, `GetJobs`, `DeleteJob`, `StartJobRun`,
  `GetJobRun`). Python shell job runs now execute S3-backed scripts.

#### GCP

- **Cloud Spanner**: instance CRUD (`projects.instances` Create/Get/List/Delete) +
  database CRUD (`projects.instances.databases` Create/Get/List/Delete) + session
  management (Create/Delete/List).
- **Cloud Dataflow**: job submission (`projects.locations.jobs.create`) + status
  (`projects.locations.jobs.get`, `projects.locations.jobs.list`).
- **Bigtable**: instance CRUD (`projects.instances` Create/Get/List/Delete) +
  cluster CRUD + table CRUD (`projects.instances.tables` Create/Get/List/Delete).
  Terraform coverage was later added with `google_bigtable_instance` and
  `google_bigtable_table`, using the provider's official Bigtable gRPC emulator
  path for Admin calls.

#### Azure

- **Logic Apps**: workflow CRUD (`PUT/GET/DELETE/LIST workflows`) + enable/disable/validate + trigger run history.
- **Azure Container Instances (ACI)**: container group CRUD
  (`PUT/GET/DELETE/LIST containerGroups`) + container exec + logs.

### Phase H — azuread Terraform provider (blocked upstream, BUG-1345)

`hashicorp/terraform-provider-azuread` has no endpoint override for Microsoft Graph API
calls. Tracked upstream: https://github.com/hashicorp/terraform-provider-azuread/issues/1837.
Sockerless issue: #394.

When upstream ships `microsoft_graph_endpoint` support, add `azuread_group`,
`azuread_user`, `azuread_group_member` to `simulators/azure/terraform-tests/main.tf`
and update the `azure-entra` coverage matrix Terraform cell from `not applicable` to
`direct`.

## Deferred

- **BUG-1075**: live-cloud validation. Do not mark any live cell green without real
  authenticated runs. No timeline set.
- **BUG-1104**: audit cadence. Remains open while simulator work continues; re-audit
  after each simulator phase.
- **Issue #363**: versioned releases and GHCR image publishing. Deferred while the
  project is early.
- **Issue #394 / BUG-1345**: azuread Terraform provider upstream blocker (see Phase H).

## Completed Work (summary)

Detailed history lives in PR descriptions and `git log`. Highlights by area:

### bleephub

Full GitHub Enterprise Server REST + GraphQL API simulator: orgs (including
`POST /admin/organizations`, PR #393), repos, teams (PR #385 added `GET /user/teams`),
members, PRs, issues, actions, apps, OAuth/OIDC (PR #393 added Graph provisioning +
ROPC), Projects v2, checks, deployments, releases, reactions, webhooks, runners,
packages.

### AWS simulator

EC2 (VMs, EBS block-level, VPC/networking, security groups, NAT), ECS (managed EBS
named volumes), Lambda, S3 (multipart, lifecycle, presigned), RDS, ElastiCache,
DynamoDB, KMS, SSM, Secrets Manager, SNS, SQS, CloudWatch, CloudTrail, Auto Scaling,
ELBv2, Route 53, IAM, ECR, EFS, API Gateway v1+v2, Kinesis, EventBridge, Cloud Map,
WAFv2, ACM, Amplify, STS. All surfaces with SDK + CLI + Terraform coverage.

### GCP simulator

Compute Engine (VMs, disks, networking, firewalls, NAT, load balancing, instance
templates PR #392), Cloud Run, Cloud Functions Gen2, GCS, Pub/Sub, Cloud SQL,
Memorystore Redis, BigQuery, Firestore, IAM (service accounts + SA keys PR #392,
project IAM), Artifact Registry, Secret Manager, Cloud DNS, API Gateway, Cloud Build
(triggers PR #392), Cloud Logging (sinks + metrics PR #392), Eventarc, VPC Access.

### Azure simulator

VMs (Firecracker-backed), Container Apps/Jobs, Azure Functions, ACR (ARM + data-plane
+ image ops PR #388), Key Vault (ARM + data-plane), Storage (Blob/File/Queue/Table,
ARM + data-plane), Service Bus (ARM + admin + data-plane), Event Hubs, Event Grid,
Cosmos DB (SQL + Tables), Redis, PostgreSQL Flexible, App Insights/Monitor (PR #388),
Private DNS (zones + A-records PR #388 + generic record types + VNet links), DNS,
Managed Identity, Entra OIDC (Graph provisioning + ROPC PR #393), Networking
(VNet/Subnet/NSG/NIC/LB/NAT).

### Infrastructure

- **Real-execution substrate**: `simulators/realexec` — Firecracker VM lifecycle,
  Linux network namespaces, bridges/veth/TAP NICs, IPAM, nftables, active health
  probes, load-balancer proxying.
- **Local HTTPS gateway**: Caddy front door (`make stack-https-{up,status,ca,down}`).
- **Admin UI**: React 19 + Vite + Tailwind 4 SPA at `/ui/`.
- **CLI**: `cmd/sockerless/` with context config at `~/.sockerless/`.

### Coverage authorities

`specs/SIM_TEST_COVERAGE_MATRIX.md` and `specs/SIM_SURFACE_TABLES/` are the coverage
authorities. All rows currently `direct | direct | direct` or `not applicable` with
documented justification. PR #395 backfilled surface tables for all gaps from PRs
#388/392/393 (new: `azure-entra.md`, `azure-private-dns.md`; updated: `gcp-compute.md`,
`gcp-iam.md`, `azure-acr.md`, `azure-monitor.md`).
