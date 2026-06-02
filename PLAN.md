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

Idle on `main` after PR #385 (bleephub `GET /api/v3/user/teams`).

Next: two focused simulator coverage-gap PRs (details in DO_NEXT.md).

## Upcoming Phases

### Phase A — Azure simulator coverage gaps (one PR)

Three missing SDK/CLI test areas; all implementations already exist.

- **Application Insights**: `azurerm_application_insights` is in the Terraform stack and `insights.go` implements the ARM paths, but there are zero SDK or CLI tests. Add `armappinsights.ComponentsClient` SDK tests + `az monitor app-insights component` CLI tests.
- **Private DNS A-records**: Zone CRUD is tested, but A/AAAA/CNAME record operations are only exercised via raw `az rest`. Add `armprivatedns.RecordSetsClient` SDK tests for A-record CRUD.
- **ACR image operations**: Only cache-rule tests exist. Add image manifest push/list/delete via the `azcontainerregistry` data-plane client.

### Phase B — GCP simulator coverage gaps (one PR)

Two missing route implementations plus four areas with routes but no SDK tests.

**Implement:**
- Service account keys (`/v1/projects/{p}/serviceAccounts/{email}/keys` POST/GET/LIST/DELETE in `iam.go`)
- Compute instance templates (`/compute/v1/projects/{p}/global/instanceTemplates` in `compute.go`)

**Add SDK + CLI tests (routes already exist):**
- Cloud Functions Gen2 CRUD (`functions2.NewCloudFunctionsClient`)
- Cloud Build trigger CRUD (`cloudbuild.NewCloudBuildClient` trigger methods)
- Cloud Logging sink + metric CRUD (`logging.NewConfigClient`)
- Project IAM `getIamPolicy` / `setIamPolicy` + add `google_project_iam_member` to Terraform stack

### Deferred

- **BUG-1075**: live-cloud validation. Not started; do not mark any live cell green without real authenticated runs.
- **BUG-1104**: audit cadence. Remains open while simulator work continues; re-audit after each simulator phase.
- **Issue #363**: versioned releases and GHCR image publishing. Deferred while project is early.

## Completed Work (summary)

Detailed history lives in PR descriptions and `git log`. Highlights:

- **bleephub**: full GitHub REST+GraphQL API simulator (orgs, repos, teams, members, PRs, issues, actions, apps, OAuth/OIDC, webhooks, Projects v2, checks, deployments, releases). PR #385 added `GET /api/v3/user/teams` for OIDC team→role mapping.
- **AWS simulator**: EC2 (VMs, EBS block-level, VPC/networking, security groups, NAT), ECS (managed EBS named volumes), Lambda, S3, RDS, ElastiCache, DynamoDB, KMS, SSM, Secrets Manager, SNS, SQS, CloudWatch, CloudTrail, Auto Scaling, ELBv2, Route 53, IAM, ECR, EFS, API Gateway, Kinesis, EventBridge, Cloud Map, WAFv2, ACM, Amplify, CloudFront, ACM, STS. All surfaces with SDK + CLI + Terraform coverage.
- **GCP simulator**: Compute Engine (VMs, disks, networking, firewalls, NAT, load balancing), Cloud Run, Cloud Functions Gen2, GCS, Pub/Sub, Cloud SQL, Memorystore Redis, BigQuery, Firestore, IAM (service accounts), Artifact Registry, Secret Manager, Cloud DNS, API Gateway, Cloud Build, Cloud Logging, Eventarc, VPC Access.
- **Azure simulator**: VMs (Firecracker-backed), Container Apps/Jobs, Azure Functions, ACI, ACR, Key Vault (ARM + data-plane), Storage (Blob/File/Queue/Table, ARM + data-plane), Service Bus (ARM + admin + data-plane), Event Hubs, Event Grid, Cosmos DB (SQL + Tables), Redis, PostgreSQL Flexible, App Insights/Monitor, Private DNS, DNS, Managed Identity, Entra OIDC, Networking (VNet/Subnet/NSG/NIC/LB/NAT).
- **Real-execution substrate**: shared `simulators/realexec` module with Firecracker VM lifecycle, Linux network namespaces, bridges/veth/TAP NICs, IPAM, nftables SNAT/DNAT/filtering, nftables metadata DNAT for `169.254.169.254`, active health probes, and load-balancer proxying across all three clouds.
- **Local HTTPS gateway**: optional Caddy front door (`make stack-https-{up,status,ca,down}`) required by AzureRM provider for `metadata_host` HTTPS discovery. AWS/GCP also have optional HTTPS harnesses.
- **Admin UI**: React 19 + Vite + Tailwind 4 SPA at `/ui/` for topology management.
- **CLI**: `cmd/sockerless/` with context config at `~/.sockerless/contexts/{name}/config.json`.

### Coverage matrix

`specs/SIM_TEST_COVERAGE_MATRIX.md` and `specs/SIM_SURFACE_TABLES/` are the coverage authorities. All rows are currently `direct | direct | direct` or `not applicable`. The Phase A and B gaps will be added after those PRs close.
