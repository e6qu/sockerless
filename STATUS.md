# Sockerless - Status

Roadmap [PLAN.md](PLAN.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Snapshot

| | |
|---|---|
| Active branch | `main` - no implementation branch active. |
| In-flight | None. |
| Planned next | GCP issue group: #304, #309-#311, and #321-#325, unless a higher-priority issue appears. |
| Last merged | AWS Amplify fidelity fixes for issues #330 and #331. |
| Open GitHub issues | #304, #309-#315, #321-#329, and #332-#336. |
| Bugs | 1267 filed - 1264 fixed - 6 open - 2 false positives. |
| Open BUGs | BUG-1075 live-cloud validation; BUG-1104 audit cadence; BUG-1254 GCP client-surface gaps; BUG-1263 GCP API-shape backlog; BUG-1264 Azure API-shape backlog; BUG-1267 compute/networking real execution. |
| Live infra | None up. |

## Current State

The AWS/GCP Terraform HTTPS gateway examples were added, and CI kept Terraform provider validation on the Caddy HTTPS path where gateway fidelity matters.

The AWS simulator fidelity PR fixed issues #305-#308 and #317-#320. S3 `ListObjectsV2` now sorts and paginates keys, Lambda `FunctionConfiguration` responses no longer expose request-only `Code` or `Tags`, SNS confirmation-required subscriptions return `pending confirmation`, SQS rejects invalid receive batch sizes, EC2 honors run counts and filters with a pending-to-running state transition, ECR image digests are content-addressed, and KMS data keys use crypto-random key material.

The AWS Amplify fidelity PR fixed issues #330 and #331. Amplify `StopJob` and `DeleteJob` now used their distinct public REST paths and semantics, `DeleteJob` removed the job and its artifacts, and `ListArtifacts`, `GetArtifactUrl`, and `GenerateAccessLogs` were registered on the real AWS SDK paths with SDK and CLI coverage. No Terraform coverage was added for those operations because the official Terraform AWS provider exposes Amplify app/branch/webhook/domain/backend-environment resources, not job artifact or access-log operations.

Azure Terraform already ran through the local Caddy HTTPS gateway. The gateway remains local transport infrastructure. It does not add simulator-only public API endpoints, request fields, headers, or response shapes.

Provider facts:

- AzureRM requires trusted HTTPS for custom metadata discovery because `metadata_host` is host-only and the provider builds `https://<host>`.
- Azure Stack is HTTPS-shaped for ARM/metadata usage.
- AzAPI exposes full endpoint URLs and defaults to HTTPS Azure endpoints.
- AWS and GCP Terraform providers accept full custom endpoint URLs; current HTTP localhost simulator endpoints remain valid and must keep working.
- Existing direct simulator TLS via `SIM_TLS_CERT` / `SIM_TLS_KEY` remains supported.

Implemented gateway surface:

- `make stack-https-up`, `make stack-https-status`, `make stack-https-ca`, `make stack-https-down`.
- Caddy routes for AWS, GCP, Azure ARM/metadata, Azure host-addressed data-plane wildcards including Cosmos DB documents, and an explicit `https://localhost:<port>` single-simulator route used by AWS/GCP Terraform HTTPS harnesses.
- `STACK_HTTPS=1` stack integration for local dev stacks.
- Admin UI topology card for gateway status, endpoints, CA path, and recovery commands.
- Azure Terraform tests started the simulator on HTTP loopback, started Caddy with per-test state and CA, used `metadata_host`/ARM endpoint through `https://azure.sockerless.localhost:<port>`, and passed the Caddy root CA through `SSL_CERT_FILE`.
- The shared simulator Docker test image included Caddy, installed from the official Caddy package repository.
- Azure Terraform CI installed Caddy on the runner for the direct `make terraform-test` path, the Azure Terraform harness failed loudly when Caddy or HTTPS was missing, and GCP arithmetic SDK coverage asserted the actual `"Result: 30"` Cloud Logging payload.
- SDK/CLI gateway guidance documented real client knobs for AWS CLI/SDKs, gcloud/Google clients, Azure CLI, and Azure SDKs without disabling TLS verification.
- BUG-1104 audit corrected stale `gcp-gcs` CLI coverage: `gcloud storage` now has real bucket/object lifecycle coverage, the simulator accepts current gcloud multipart upload boundaries, GCS `buckets.getStorageLayout` returns the public response shape, and GCS timestamps use Cloud Storage-style millisecond precision.
- AWS/GCP Terraform now had optional `make terraform-https-test` targets that started the simulator on HTTP loopback, put Caddy in front of it, set `SSL_CERT_FILE` to Caddy's local CA, and ran the real Terraform provider apply/destroy harness through the gateway's `https://localhost:<ephemeral-port>` single-simulator route. On macOS those targets delegated to the shared Linux simulator test image, matching Azure's CA-trust pattern.
- Terraform CI installed Caddy for the Terraform matrix and ran AWS/GCP via those HTTPS targets while Azure continued its mandatory gateway-backed harness.
- BUG-1253 corrected stale `gcp-vpcaccess` Terraform coverage: the GCP Terraform stack now used `vpc_access_custom_endpoint`, provisioned `google_vpc_access_connector`, asserted the canonical connector ID, and marked the matrix row direct.
- BUG-1254 / issue #304 was opened for larger GCP audit gaps: API Gateway, Cloud Build, IAM, and Pub/Sub have public client surfaces where some matrix rows still need real gcloud/Terraform coverage.
- BUG-1255..BUG-1262 / issues #305-#308 and #317-#320 were fixed in the AWS simulator with SDK coverage and targeted CLI regression coverage.

## Invariants

### Process

- Never auto-merge PRs. The user handles merges.
- Use one branch per phase and one PR per phase.
- Before a PR is ready: `git fetch origin main`, rebase on `origin/main`, push, then sync local `main` after merge.
- No interactive commands.
- File concrete BUG entries before fixing discovered gaps.
- Continuity docs must be updated in each PR and written so they are correct after the PR merges.

### Implementation

- No stubs, fakes, mocks, synthetic responses, silent fallbacks, or degraded modes.
- Simulator public APIs must match real cloud public APIs. Local admin/gateway infrastructure may exist, but must not leak into cloud API surfaces.
- One simulator binary per cloud.
- Every new simulator public API slice needs official SDK, vendor CLI, and Terraform-provider coverage where those public client surfaces exist.
- SDK, CLI, and Terraform call sequences differ; do not infer coverage from one client surface to another.
- `specs/SIM_TEST_COVERAGE_MATRIX.md` and `specs/SIM_SURFACE_TABLES/` are the coverage authorities.
- Mux overlap, paged List operations, and resource state machines are recurring bug classes; audit them when touching simulator routes.

### Deferred Trackers

- BUG-1075: live-cloud validation remains deferred. Do not mark cells green without real authenticated cloud runs.
- BUG-1104: audit cadence remains open. Continue re-checking stale SDK/CLI/Terraform not-applicable claims during simulator phases.
- BUG-1254: issue #304 tracks larger GCP client-surface coverage gaps found during the latest BUG-1104 pass.
- BUG-1263: GCP API-shape issues #309-#311 and #321-#325 remain open.
- BUG-1264: Azure API-shape issues #312-#315 and #326-#329 remain open.
- BUG-1267: issues #332-#336 track the real-execution compute/networking program across AWS/GCP/Azure.

## Recent Merged Work

- Azure Terraform HTTPS gateway stage: the Azure Terraform harness used the local Caddy gateway end to end, and BUG-1246 fixed Azure Storage data-plane host dispatch so `azure.sockerless.localhost` metadata requests were no longer swallowed by the storage wrapper.
- SDK/CLI HTTPS gateway audit: documented real CA/endpoint knobs for SDK and CLI clients, and fixed GCP GCS CLI coverage discovered by BUG-1104.
- Terraform HTTPS gateway audit: AWS/GCP got optional HTTPS provider harnesses and GCP VPC Access Terraform coverage; issue #304 was opened for larger GCP client-surface gaps.
- AWS simulator fidelity sweep: issues #305-#308 and #317-#320 were fixed with real AWS SDK coverage and targeted AWS CLI regression coverage.
- AWS Amplify fidelity sweep: issues #330 and #331 were fixed with real AWS SDK and AWS CLI coverage.
- PR #299 / issue #298: Azure Redis CLI/Terraform coverage; GCP Memorystore Redis gcloud/Terraform coverage; GCP Cloud SQL `/v1` and `/sql/v1beta4` coverage; GCP Cloud DNS Changes and record-set patch routes.
- Local HTTPS gateway Stage 1: optional Caddy gateway, `.stack-pids` lifecycle integration, docs, and admin UI visibility.
- PR #296/#295/#291/#289 series: AWS Route 53 list fidelity, Lambda Terraform coverage, RDS/ElastiCache/API Gateway client-surface coverage, and Terraform minimum-wait documentation.
- Prior foundational simulator phases: object storage, queues, event systems, streams, managed data SaaS, DNS, VM/instance control planes, managed load balancers, NAT/public-IP, and VPC/networking parity across AWS/GCP/Azure.

Detailed history belongs in PR descriptions and `git log`; this file keeps only resume-critical state.
