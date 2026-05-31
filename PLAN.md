# Sockerless - Roadmap

State [STATUS.md](STATUS.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Goal

Replace Docker Engine with Sockerless for Docker API clients such as `docker`, Docker Compose, Testcontainers, and CI runners, backed by real cloud infrastructure or high-fidelity local cloud simulators.

## Current Phase

Idle on `main`. No implementation branch is active.

Next planned phase: the GCP issue group (#304, #309-#311, and #321-#325), unless a higher-priority issue appears.

## Guiding Principles

1. Match public APIs exactly: Docker API, GitHub API for bleephub, and public cloud APIs for simulators.
2. No stubs, fakes, mocks, synthetic behavior, silent fallbacks, or degraded modes.
3. Simulators are real local cloud slices, not product-specific emulators.
4. One simulator binary per cloud.
5. Every new simulator public API slice ships with official SDK, vendor CLI, and Terraform-provider coverage where those surfaces exist.
6. Components remain decoupled. Admin UI and local gateway infrastructure must not become required by the simulators' public APIs.
7. User merges PRs. Agents create branches, commits, and PRs only.
8. Continuity docs are updated in every PR and written so they are correct after the PR merges.

## Local HTTPS Gateway

The optional Caddy/local-HTTPS front door for simulator APIs was added.

This is local transport infrastructure, not a simulator public API change. Public simulator routes, headers, request bodies, and response shapes must remain cloud-compatible.

### Provider Facts

- AzureRM is the hard requirement. Its `metadata_host` field is host-only, and provider source builds `https://<host>` for metadata discovery.
- Azure Stack is also HTTPS-shaped for ARM/metadata usage.
- AzAPI exposes full endpoint URLs and defaults to HTTPS Azure endpoints.
- Google provider custom endpoints are full URLs and current HTTP simulator overrides remain valid.
- AWS provider custom endpoints are full URLs and official docs explicitly support `http://localhost` service endpoints.
- Existing direct simulator TLS via `SIM_TLS_CERT` / `SIM_TLS_KEY` remains supported.

Implemented:

- Caddy config and `make stack-https-{up,status,ca,down}` targets.
- HTTPS routing for `aws.sockerless.localhost`, `gcp.sockerless.localhost`, `azure.sockerless.localhost`, and Azure host-addressed data-plane wildcards, including Cosmos DB documents.
- `STACK_HTTPS=1 make stack-azure-aca` style stack integration, including Azure ARM-advertised data-plane URL projection.
- Admin API/UI visibility for gateway status, endpoints, CA path, and recovery make commands.
- Docs for local CA trust and provider-specific HTTPS behavior.
- Azure Terraform harness over the gateway: per-test Caddy state and CA, `metadata_host`/ARM endpoint through `https://azure.sockerless.localhost:<port>`, ARM-advertised Azure data-plane URLs under the gateway, and `SSL_CERT_FILE` CA trust in the Linux Docker test container.
- Shared simulator Docker test image with Caddy installed from the official package repository.
- SDK/CLI gateway guidance for AWS CLI/SDKs, gcloud/Google clients, Azure CLI, and Azure SDKs.
- BUG-1104 GCS CLI audit: current `gcloud storage` endpoint overrides were documented and covered by real CLI tests; the simulator accepted current gcloud multipart upload boundaries and implemented `buckets.getStorageLayout`.
- AWS/GCP Terraform HTTPS examples and harnesses: `make terraform-https-test` starts the simulator on HTTP loopback, starts Caddy with isolated state, trusts Caddy's local CA through `SSL_CERT_FILE`, and runs the real provider apply/destroy path against the gateway's resolver-independent `https://localhost:<ephemeral-port>` single-simulator route.
- Terraform CI kept Caddy HTTPS for provider validation: Azure remained mandatory through the gateway, and AWS/GCP used their optional HTTPS gateway targets in CI while direct HTTP `make terraform-test` stayed available locally.
- BUG-1253 fixed stale GCP VPC Access Terraform coverage by adding `google_vpc_access_connector` to the GCP Terraform stack and updating the coverage matrix.

Remaining staged work:

1. **BUG-1254 / issue #304 plus BUG-1263.** Add real public-client coverage for larger stale GCP not-applicable rows and fix the GCP API-shape bugs in issues #309-#311 and #321-#325.
2. **BUG-1264.** Fix Azure API-shape and LRO bugs in issues #312-#315 and #326-#329.

## AWS Fidelity Sweep

The AWS simulator fidelity sweep fixed issues #305-#308 and #317-#320:

- S3 `ListObjectsV2` returned lexicographically sorted keys, honored cursors, emitted continuation tokens, and supported delimiter common prefixes.
- Lambda `FunctionConfiguration` responses omitted request-only `Code`/`Tags`, while `GetFunction` kept `Code` and `Tags` only as top-level members.
- SNS returned `pending confirmation` for confirmation-required protocols unless `ReturnSubscriptionArn=true`.
- SQS rejected invalid `MaxNumberOfMessages` values instead of silently clamping them.
- EC2 `RunInstances` honored `MinCount`/`MaxCount`, returned `pending`, transitioned to `running`, and `DescribeInstances` applied supported filters.
- ECR `PutImage` used content-addressed SHA-256 manifest digests.
- KMS `GenerateDataKey` used fresh crypto-random key material.

## Deferred Work

- BUG-1075: live-cloud validation. Deferred by user direction. Do not mark live cells green without authenticated real-cloud runs.
- BUG-1104: audit cadence. Keep open while simulator work continues; every simulator phase should re-check stale SDK/CLI/Terraform coverage claims.
- BUG-1254: GCP client-surface coverage gaps from the latest audit pass. Issue #304 tracks the remaining work.
- BUG-1263: GCP API-shape backlog. Issues #309-#311 and #321-#325 remain open.
- BUG-1264: Azure API-shape backlog. Issues #312-#315 and #326-#329 remain open.

## Current Capability Summary

- Docker-compatible REST API with cloud backends for AWS, GCP, and Azure plus Docker passthrough.
- AWS/GCP/Azure simulators with SDK, CLI, and Terraform validation for the public API slices sockerless depends on.
- Bleephub GitHub API simulator compatible with real `gh` CLI paths.
- Admin UI and local stack orchestration.
- Foundational simulator parity exists for object storage, queues, events, streams, managed data SaaS, DNS, VPC/networking, NAT/public-IP, managed load balancers, and VM/instance control planes.

## History

Detailed phase history has been intentionally removed from continuity docs to keep fresh sessions actionable. Use PR descriptions, issue threads, and `git log` for older per-phase details.
