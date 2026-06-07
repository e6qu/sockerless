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

## Current Phase

**Terraform idempotency drift sweep** (started in PR #491). The new
second-plan `terraform plan -detailed-exitcode` drift assertions on the gcp +
azure apply stacks (BUG-1532) surfaced ~13 pre-existing read-back fidelity bugs
that cause a non-empty second plan. PR #491 already lands the green pieces
(scheduler `cron(...)` evaluation BUG-1531, the drift assertions themselves, and
the Docker-Hub→ECR-gallery harness fix BUG-1533); the drift fixes below are the
remaining work to make `tf (gcp)` / `tf (azure)` green. These stacks cannot run
on a Mac host (gcp needs Linux real-exec; azure-Docker times out locally), so
each fix is verified blind via the CI `tf (...)` job.

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

**Watch (CI to confirm):** container_app_environment `log_analytics_workspace_id`
— the ARM API does not return the workspace **resource id** (only the
`customerId`), so this is provider-side; not guessed at — confirm via `tf (azure)`.

Approach: fix one cloud fully, push, read the next CI plan, repeat until `tf (gcp)`
and `tf (azure)` are green.

## Completed Phases

- **Phase C** (PRs #402, #403): Token-based pagination on AWS/GCP/Azure list endpoints.
- **Phase D** (PR #404): Error envelope fidelity + negative-path SDK error classification tests.
- **Phase E** (PR #405): Azure KV data-plane CLI tests via `az rest` + Host header routing.
- **Phase F** (PR #405): 12 bleephub surface table files + coverage matrix rows.

### Phase G — New cloud service slices (one PR per cloud)

Three sequential PRs. Each new slice ships with SDK + CLI + Terraform coverage per the
standard contract. Surface table file(s) and coverage matrix row(s) ship in the same PR.

#### Phase G-AWS

- **Step Functions**: state machine CRUD (`CreateStateMachine`, `DescribeStateMachine`,
  `ListStateMachines`, `DeleteStateMachine`) + execution lifecycle (`StartExecution`,
  `DescribeExecution`, `ListExecutions`, `StopExecution`).
- **Batch**: job definitions (`RegisterJobDefinition`, `DescribeJobDefinitions`,
  `DeregisterJobDefinition`), job queues (`CreateJobQueue`, `DescribeJobQueues`,
  `DeleteJobQueue`), compute environments (`CreateComputeEnvironment`,
  `DescribeComputeEnvironments`, `DeleteComputeEnvironment`), job submission
  (`SubmitJob`, `DescribeJobs`, `CancelJob`).
- **CodeBuild**: build project CRUD (`CreateProject`, `BatchGetProjects`, `ListProjects`,
  `DeleteProject`) + start build (`StartBuild`, `BatchGetBuilds`).
- **Glue**: database CRUD (`CreateDatabase`, `GetDatabase`, `GetDatabases`,
  `DeleteDatabase`), table CRUD (`CreateTable`, `GetTable`, `GetTables`, `DeleteTable`),
  job CRUD (`CreateJob`, `GetJob`, `GetJobs`, `DeleteJob`, `StartJobRun`,
  `GetJobRun`).

#### Phase G-GCP

- **Cloud Spanner**: instance CRUD (`projects.instances` Create/Get/List/Delete) +
  database CRUD (`projects.instances.databases` Create/Get/List/Delete) + session
  management (Create/Delete/List).
- **Cloud Dataflow**: job submission (`projects.locations.jobs.create`) + status
  (`projects.locations.jobs.get`, `projects.locations.jobs.list`).
- **Bigtable**: instance CRUD (`projects.instances` Create/Get/List/Delete) + cluster
  CRUD + table CRUD (`projects.instances.tables` Create/Get/List/Delete).

#### Phase G-Azure

- **Logic Apps**: workflow CRUD (`PUT/GET/DELETE/LIST workflows`) + run trigger
  (`POST workflows/{name}/triggers/{trigger}/run`) + run history
  (`GET workflows/{name}/runs`).
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
