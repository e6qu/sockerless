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

Idle on `main` after PR #396 (continuity doc update). All surface tables and the coverage
matrix are current. No in-flight PRs.

## Upcoming Phases

### Phase C — Pagination shape verification

Many simulator list endpoints return all results in a single page rather than the paged
shape the real cloud uses. This is invisible to simple tests but breaks production clients
that page through results.

Scope (one PR per cloud, or combined if small):
- Audit all `GET .../list` handlers across `simulators/aws`, `simulators/gcp`,
  `simulators/azure` for missing `nextPageToken` / `NextToken` / `nextLink` responses.
- Add token-based pagination to any handler that real cloud paginating (use real default
  page sizes from the cloud spec).
- Add SDK/CLI tests that explicitly exhaust two+ pages (create N > page-size resources,
  assert all are returned via paging).
- Update `paged-shape verified` cells in `specs/SIM_SURFACE_TABLES/` from `n/a` to `✓`
  as each handler is fixed.

### Phase D — Error shape fidelity

Simulators return meaningful HTTP status codes, but error envelopes (JSON body shape,
field names, error codes) sometimes diverge from what the real cloud sends. SDKs that
parse error codes (e.g., `ResourceNotFoundException`, `ResourceNotFound`,
`RESOURCE_NOT_FOUND`) will fail to classify errors correctly.

Scope:
- Audit error responses across all three simulators against real cloud wire shapes.
- Fix body shapes so error classification in official SDKs works correctly.
- Add negative-path SDK tests that assert on parsed error types, not raw HTTP status.

### Phase E — azure-kv-data-plane CLI coverage

The coverage matrix marks `azure-kv-data-plane` CLI as `not applicable`, but the `az`
CLI does expose Key Vault data-plane operations:
- `az keyvault secret set/show/list/delete`
- `az keyvault key create/show/list/delete`
- `az keyvault certificate create/show/list/delete`

All three are exercised by `az keyvault` commands that point at the sim's custom endpoint.
Add CLI tests to `simulators/azure/cli-tests/` and update the coverage matrix cell.

### Phase F — Bleephub surface tables

`specs/SIM_SURFACE_TABLES/` has no bleephub entries. bleephub implements a significant
GitHub Enterprise Server REST API surface (orgs, repos, teams, members, PRs, issues,
actions, apps, OAuth/OIDC, Projects v2, checks, deployments, releases, reactions,
webhooks, runners, packages). This surface is exercised by tests but undocumented in the
coverage authorities.

Scope:
- Run a HandleFunc audit across all `bleephub/gh_*.go` files.
- Create surface table files per logical group (e.g., `bleephub-orgs.md`,
  `bleephub-repos.md`, `bleephub-actions.md`, etc.).
- Add corresponding rows to `specs/SIM_TEST_COVERAGE_MATRIX.md`.
- Update `scripts/check-simulator-coverage-matrix.sh` if it needs to include bleephub
  tables (currently it only checks `specs/SIM_SURFACE_TABLES/`).

### Phase G — New cloud service slices

Extend the simulators to cover high-value services not yet implemented. Exact services TBD
based on user priorities and consumer need. Candidates (not exhaustive):

**AWS:**
- AWS Step Functions (state machine CRUD + start/stop execution)
- AWS Batch (job definitions, job queues, compute environments, job submission)
- AWS CodeBuild (build project CRUD + start build) — complements the CI/CD story
- AWS Glue (database/table/job CRUD for ETL pipelines)

**GCP:**
- Cloud Spanner (instance + database CRUD, session management)
- Cloud Dataflow (job submission + status)
- BigTable (instance + table CRUD)

**Azure:**
- Azure Logic Apps (workflow CRUD + run trigger)
- Azure Container Instances (ACI) — if not already covered

Each new slice ships with SDK + CLI + Terraform coverage per the standard contract.

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
