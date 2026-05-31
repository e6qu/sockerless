# Sockerless - Roadmap

State [STATUS.md](STATUS.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Goal

Replace Docker Engine with Sockerless for Docker API clients such as `docker`, Docker Compose, Testcontainers, and CI runners, backed by real cloud infrastructure or high-fidelity local cloud simulators.

## Current Phase

Idle on `main`. No implementation branch is active.

Next planned phase: stage the compute/networking real-execution program from BUG-1267 / issues #332-#336, unless a higher-priority issue appears.

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
- Caddy local-CA trust-store installation was disabled with `skip_install_trust`; tests trusted the generated CA explicitly through `SSL_CERT_FILE` or client-specific CA knobs instead of mutating host trust stores.
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
- BUG-1254 / issue #304 fixed stale GCP client-surface coverage rows. API Gateway, Cloud Build, IAM, and Pub/Sub now have real gcloud coverage where the public CLI exposes the surface, and API Gateway has Terraform provider coverage through `google-beta`.
- BUG-1263 / issues #309-#311 and #321-#325 fixed the GCP API-shape backlog: Cloud Run list pagination and empty-list wire shape, Cloud Run/Functions/API Gateway/Eventarc LRO metadata types, canonical millisecond timestamps, Cloud Logging severity ordering, GCS object metadata and bucket IAM policy shape, Cloud SQL backup operations, and Cloud DNS precondition error shape.
- BUG-1264 / issues #312, #315, and #326-#329 fixed the Azure API-shape backlog: Storage Blob/File/Queue data-plane errors now return XML error envelopes, storage list responses include the public XML declaration/attributes/markers, and Queue service properties support the provider availability probe; Service Bus admin missing queue/topic/subscription/rule reads return real 404 Atom/XML errors; Event Grid publish rejects malformed and schema-invalid events before delivery; Redis, PostgreSQL Flexible Server, and Event Hubs namespace creates return Azure LRO headers and in-progress resource states before converging; Key Vault secret/key/certificate attributes include default recovery metadata.

Remaining staged work:

1. **BUG-1267 / issues #332-#336.** Stage the compute/networking real-execution program. Start with architecture and Linux capability plumbing before implementing Firecracker-backed VMs, real VPC/IPAM/routing/NAT, nftables enforcement, or real load-balancer data planes.

## AWS Fidelity Sweep

The AWS simulator fidelity sweep fixed issues #305-#308 and #317-#320:

- S3 `ListObjectsV2` returned lexicographically sorted keys, honored cursors, emitted continuation tokens, and supported delimiter common prefixes.
- Lambda `FunctionConfiguration` responses omitted request-only `Code`/`Tags`, while `GetFunction` kept `Code` and `Tags` only as top-level members.
- SNS returned `pending confirmation` for confirmation-required protocols unless `ReturnSubscriptionArn=true`.
- SQS rejected invalid `MaxNumberOfMessages` values instead of silently clamping them.
- EC2 `RunInstances` honored `MinCount`/`MaxCount`, returned `pending`, transitioned to `running`, and `DescribeInstances` applied supported filters.
- ECR `PutImage` used content-addressed SHA-256 manifest digests.
- KMS `GenerateDataKey` used fresh crypto-random key material.

The AWS Amplify fidelity sweep fixed issues #330 and #331:

- `StopJob` used `DELETE /apps/{appId}/branches/{branchName}/jobs/{jobId}/stop` and cancelled the job.
- `DeleteJob` used `DELETE /apps/{appId}/branches/{branchName}/jobs/{jobId}`, removed the job and its artifacts, and made later `GetJob` / `GetArtifactUrl` calls return `NotFoundException`.
- `ListArtifacts`, `GetArtifactUrl`, and `GenerateAccessLogs` were registered with their public AWS SDK REST paths and covered through the real AWS SDK and AWS CLI.

The Azure ARM/DNS fidelity sweep fixed issues #313, #314, and #340:

- ARM control-plane paths now required `api-version` and returned `InvalidApiVersionParameter` with the public Azure error shape when callers omitted it.
- Store-backed ARM list responses returned empty arrays rather than JSON nulls.
- Private DNS zones supported list-by-resource-group through the real `GET .../privateDnsZones` route, and Private DNS virtual network links supported list-by-zone.
- The fixes were covered through the real Azure SDK and Azure CLI.

The Azure API-shape and LRO fidelity sweep fixed issues #312, #315, and #326-#329:

- Storage Blob/File/Queue data-plane errors returned XML `<Error>` envelopes with `Content-Type: application/xml` and `x-ms-error-code`, and Queue service properties returned the public service-properties XML used by Terraform provider availability checks.
- Blob `ListContainers` and `ListBlobs` XML responses emitted XML declarations, public `EnumerationResults` attributes, and `NextMarker`.
- Service Bus admin data-plane missing entity reads returned 404 Atom/XML errors rather than successful empty feeds.
- Event Grid topic publish rejected malformed JSON, empty batches, missing required Event Grid envelope fields, and invalid `eventTime` values before webhook fanout.
- Redis, PostgreSQL Flexible Server, and Event Hubs namespace creates returned Azure LRO headers with in-progress resource state and converged to final public states through operation polling.
- Key Vault secret/key/certificate attributes included `recoveryLevel: Recoverable+Purgeable` and `recoverableDays: 90`.

## GCP Fidelity Sweep

The GCP simulator fidelity sweep fixed issue #304 and issues #309-#311 and #321-#325:

- API Gateway, Cloud Build, IAM, and Pub/Sub stale client-surface rows were corrected with real gcloud coverage. API Gateway also gained Terraform provider coverage through `google-beta`.
- Cloud Run services/jobs/executions and Cloud Functions list operations returned empty arrays, stable ordering, page tokens, and canonical millisecond timestamps where public clients observe them.
- Long-running operation metadata used the public resource-specific `@type` values expected by official Google SDK operation decoders.
- Cloud Logging severity comparisons used Google severity ranks instead of lexicographic string ordering.
- GCS object metadata included generation, metageneration, CRC32C, MD5 where applicable, and compose component counts; bucket IAM policy responses included public `kind`, `resourceId`, and base64 etags.
- Cloud SQL backup insert/delete returned real SQL Admin operation shapes, and Cloud DNS precondition failures returned canonical `FAILED_PRECONDITION` error details.

## Deferred Work

- BUG-1075: live-cloud validation. Deferred by user direction. Do not mark live cells green without authenticated real-cloud runs.
- BUG-1104: audit cadence. Keep open while simulator work continues; every simulator phase should re-check stale SDK/CLI/Terraform coverage claims.
- BUG-1267: compute/networking real-execution backlog. Issues #332-#336 remain open and require staged architecture work before implementation PRs.

## Current Capability Summary

- Docker-compatible REST API with cloud backends for AWS, GCP, and Azure plus Docker passthrough.
- AWS/GCP/Azure simulators with SDK, CLI, and Terraform validation for the public API slices sockerless depends on.
- Bleephub GitHub API simulator compatible with real `gh` CLI paths.
- Admin UI and local stack orchestration.
- Foundational simulator parity exists for object storage, queues, events, streams, managed data SaaS, DNS, VPC/networking, NAT/public-IP, managed load balancers, and VM/instance control planes.

## History

Detailed phase history has been intentionally removed from continuity docs to keep fresh sessions actionable. Use PR descriptions, issue threads, and `git log` for older per-phase details.
