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

### Terraform drift fixes (to close BUG-1532)

Root causes are the `# forces replacement` / `-> null` lines in the second plan;
the `(known after apply)` diffs are downstream cascades that clear once their
parent resource stops being replaced.

**GCP** (`simulators/gcp/`):
1. **logging sink** (`logging.go`) — `name` returns the full `projects/{p}/sinks/{n}` path; real `LogSink.name` is the **short** name. Return short `name` in create/get/list/update responses while keeping the full-path store key.
2. **logging metric** (`logging.go`) — same full-path-vs-short `name` issue; same fix.
3. **memorystore redis** (`memorystore_redis.go`) — read-back omits the `connectMode` (default `DIRECT_PEERING`) and `transitEncryptionMode` (default `DISABLED`) defaults → both force replacement. Populate them on get.
4. **gcs bucket** (`gcs.go`) — `location` returned lowercase `us-central1`; GCS bucket locations are **uppercase** (`US-CENTRAL1`) → forces replacement. Upper-case the location on read.
5. **api gateway api-config** (`apigateway.go`) — the config read omits the OpenAPI document (`openapi_documents[].document.contents`/`path`) → forces replacement of `google_api_gateway_api_config`. Return the stored document.
6. **dns managed zone / compute** — a `type = "PUBLIC"` default is added on re-plan (forces replacement); pin down the exact resource (dns zone `visibility`/managed-zone type or router-nat) and return the default.

**AZURE** (`simulators/azure/`):
7. **`tags = {} -> null` (pervasive)** — `Tags map[string]string json:"tags,omitempty"` drops an empty map → terraform refreshes `tags` to null vs the `{}` in state. Affects app-insights, cosmosdb account, key-vault, vnet, public-ip, etc. Fix: emit `"tags": {}` (drop `omitempty` + initialise to `{}` on read) for tagged resources.
8. **`zones = [] -> null`** (public-ip / public-ip-prefix) — emit `[]` not null.
9. **`ip_tags = {} -> null`** (public-ip) — emit `{}` not null.
10. **public-ip / public-ip-prefix `sku_tier = "Regional"`** — default not returned → forces replacement. Return it.
11. **storage account `account_tier = "Standard"`** — not returned on read → forces replacement. Return it.
12. **app-insights `application_type = "web"`** — default not returned → forces replacement. Return it. (Also `daily_data_cap_notifications_disabled`/`enabled` boolean defaults `-> null`.)
13. **apim api `revision = "1"`** — default not returned → forces replacement (from the #480 APIM addition). Return it.

Approach: fix one cloud fully (so its `tf` job goes green), push, read the next CI plan, repeat. File each as its own BUG-#### as fixed.

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
