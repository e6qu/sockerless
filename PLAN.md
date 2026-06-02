# Sockerless - Roadmap

State [STATUS.md](STATUS.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Goal

Replace Docker Engine with Sockerless for Docker API clients such as `docker`, Docker Compose, Testcontainers, and CI runners, backed by real cloud infrastructure or high-fidelity local cloud simulators.

## Current Phase

Idle on `main`. No implementation branch is active.

Last completed phase: EC2 EBS Firecracker attach for issue #378. AWS EC2 EBS data volumes now have real sparse raw block images under their volume host paths, and Firecracker-backed EC2 guests use those block images through substrate-managed drive slots. Running `AttachVolume` patches a Firecracker drive slot to the attached volume's image, `DetachVolume` patches the slot back to an empty backing file, and `ModifyVolume` refreshes the drive after resizing. `CreateSnapshot` and `CreateVolume(SnapshotId)` copy the volume host path including the raw block image, so guest-written bytes persist through snapshot/restore. The mandatory Firecracker smoke proves that path with a real guest block-device write, host-side snapshot copy, restored image, and guest readback.

Next planned phase: check live open issues first. If none exist, continue BUG-1104 with a focused simulator SDK/CLI/Terraform coverage audit. Versioned release/image publishing is intentionally deferred while the project is early.

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
- AWS/GCP Terraform HTTPS examples and harnesses: `make terraform-https-test` starts the simulator on HTTP loopback, starts Caddy with isolated state, trusts Caddy's local CA through `SSL_CERT_FILE`, and runs the real provider apply/destroy path against the gateway's resolver-independent `https://localhost:<ephemeral-port>` single-simulator route. AWS covers the root production-shape stack plus the RDS and ElastiCache Terraform subpackages through that same HTTPS path.
- AWS Terraform Make targets build the simulator once, pass that real binary into each Terraform package, keep package-level concurrency, and emit package-qualified JSON test events so CI shows which provider flow is running.
- Terraform CI kept Caddy HTTPS for provider validation: Azure remained mandatory through the gateway, and AWS/GCP used their optional HTTPS gateway targets in CI while direct HTTP `make terraform-test` stayed available locally.
- BUG-1253 fixed stale GCP VPC Access Terraform coverage by adding `google_vpc_access_connector` to the GCP Terraform stack and updating the coverage matrix.
- BUG-1254 / issue #304 fixed stale GCP client-surface coverage rows. API Gateway, Cloud Build, IAM, and Pub/Sub now have real gcloud coverage where the public CLI exposes the surface, and API Gateway has Terraform provider coverage through `google-beta`.
- BUG-1263 / issues #309-#311 and #321-#325 fixed the GCP API-shape backlog: Cloud Run list pagination and empty-list wire shape, Cloud Run/Functions/API Gateway/Eventarc LRO metadata types, canonical millisecond timestamps, Cloud Logging severity ordering, GCS object metadata and bucket IAM policy shape, Cloud SQL backup operations, and Cloud DNS precondition error shape.
- BUG-1264 / issues #312, #315, and #326-#329 fixed the Azure API-shape backlog: Storage Blob/File/Queue data-plane errors now return XML error envelopes, storage list responses include the public XML declaration/attributes/markers, and Queue service properties support the provider availability probe; Service Bus admin missing queue/topic/subscription/rule reads return real 404 Atom/XML errors; Event Grid publish rejects malformed and schema-invalid events before delivery; Redis, PostgreSQL Flexible Server, and Event Hubs namespace creates return Azure LRO headers and in-progress resource states before converging; Key Vault secret/key/certificate attributes include default recovery metadata.
- BUG-1272 / issue #310 fixed the reopened GCP protobuf timestamp gap: Eventarc trigger `createTime`/`updateTime`, Firestore document timestamps, and Pub/Sub `publishTime` now use the shared canonical timestamp formatter.
- BUG-1273..BUG-1276 / issues #341, #343, #346, and #347 fixed AWS core-service gaps: CloudTrail trail CRUD/logging/lookup/S3 delivery, Auto Scaling launch configurations and ASGs that materialize EC2 instances, EBS volume/snapshot lifecycle with pending-to-completed snapshots, ECS managed EBS byte-level task/snapshot/restore round trips, and S3 `ListObjectVersions` for provider cleanup.
- BUG-1277..BUG-1283 fixed CI flakiness without weakening coverage: all Go setup steps use explicit `go.work`/`**/go.sum` cache dependency paths for the multi-module workspace, hosted linux/arm64 CPU jobs fan out without broad ordered queues or phase-ordering `needs` gates, status-only aggregate jobs were removed, CI now has 29 jobs, non-Terraform explicit CI step and Go test timeouts are capped at five minutes, Terraform provider CI is capped at ten minutes, lint/backend/FaaS smoke work is grouped into fewer real-coverage shards, GCP Cloud Logging appends now preserve concurrent stdout/stderr container lines, every workflow auto-cancels older runs for the same PR branch/ref, and AWS simulator CLI coverage is sharded by service family with every existing CLI test selected exactly once. VM-backed Firecracker simulator jobs are pinned to `ubuntu-24.04` x86_64 rather than the floating hosted label, and cache `~/.cache/sockerless/firecracker-ci` by Firecracker version plus runner architecture.
- BUG-1284 / issue #356 fixed Azure Tables ARM fidelity: Cosmos DB Tables ARM CRUD/list/throughput, Storage Tables ARM CRUD/list/update, shared projection into the Tables data plane, data-plane `Prefer: return-no-content`, and table ACL get/set used by `azurerm_storage_table`. Coverage includes official Azure SDK, Azure CLI, and Terraform provider apply/destroy paths.
- BUG-1287 fixed Azure Event Grid topic endpoint leakage. Topic ARM create/get/list now advertise the shared host-dispatched Event Grid data-plane endpoint, or the configured Caddy HTTPS gateway template, instead of allocating simulator-local `127.0.0.1:<random>` publish listeners. Regression tests verify create/get/list endpoint shape and publish delivery through the advertised host.
- BUG-1293 / issue #335 fixed the remaining GCP/Azure security enforcement packet paths. GCP firewall ingress and Azure NSG inbound rules compile to nftables on real NIC veth peers, with rule/tag/subnet/NIC mutations reapplying filters and failing loudly on substrate errors.
- BUG-1294 / issue #334 fixed the remaining GCP/Azure managed load-balancer packet paths. GCP backend services use unmanaged instance groups, `getHealth`, real probes, frontend-IP dispatch, URL-map resolution, and proxying to healthy members; Azure Load Balancer uses frontend public IP dispatch, ARM rule/backend/probe resolution, real probes, and proxying to healthy backend NICs or backend-pool addresses.
- BUG-1295 / issue #362 fixed Azure Entra OIDC authorization-code flow. Discovery's advertised `authorization_endpoint` is now served with absolute simulator-local metadata for the implemented auth-code slice, authorization requests redirect with code/state, token redemption consumes codes exactly once, PKCE is validated, unsupported grants are rejected, OAuth errors use public error fields, `openid` scopes receive a signed ID token alongside the access token, and `offline_access` receives a redeemable refresh token.
- BUG-1296 / issue #365 fixed Azure SDK local portability. Darwin `make sdk-test` uses the existing Docker real-network harness instead of the macOS host for Linux-only netns/veth/nftables tests, Linux direct runs remain available as `make sdk-test-local`, and Event Grid SDK publish tests resolve advertised `*.eventgrid.localhost` data-plane hosts explicitly while preserving Host-dispatched public routing.

Remaining staged work:

1. **BUG-1104.** Continue focused simulator coverage audits and file/fix concrete SDK/CLI/Terraform gaps when found.

## Real-Execution Substrate

The BUG-1267 stages established and completed the architecture, host-model contract, capability checks, cleanup primitives, and real host-network paths for issues #332-#336:

- [specs/SIMULATOR_EXECUTION.md](specs/SIMULATOR_EXECUTION.md) was rewritten to reflect the current Docker/Podman-backed container/FaaS execution model.
- [specs/SIMULATOR_REAL_EXECUTION.md](specs/SIMULATOR_REAL_EXECUTION.md) defined the VM/network real-execution substrate: Firecracker guests, Linux netns/bridges/tap/veth, netlink routing, nftables policy/NAT, L4/L7 proxying, active health checks, per-instance metadata, capability checks, and no metadata fallback.
- [feedback_sim_host_model.md](feedback_sim_host_model.md) documented the allowed host execution paths.
- The AWS/GCP/Azure host-dispatch tests now pointed at the explicit real-execution exception while continuing to reject broad `os/exec` workload execution.
- CI gained `make firecracker-test`, which installed the pinned official Firecracker release, required `/dev/kvm`, booted a real Firecracker Linux guest, and ran `go test`, `go build`, and multiple `eval-arithmetic` executions inside that microVM.
- [simulators/realexec](simulators/realexec) added the shared real-execution substrate module with deterministic host capability detection, LIFO cleanup, an auditable host command runner, lease-based IPv4 IPAM, and Linux bridge/netns/veth NIC creation.
- `make realexec-network-test` now creates a real Linux bridge, network namespaces, veth NICs, routes, leases, and an nftables table in the mandatory Firecracker CI job, verifies gateway and namespace-to-namespace reachability with real packets, and verifies cleanup removes host artifacts.
- The network object now owns a dedicated Linux network namespace. Its bridge and gateway address live inside that namespace, and attached NIC veth peers are moved into that namespace before being enslaved to the bridge. The host-network smoke test verifies the bridge is inside the network namespace and that guest namespaces still reach the gateway and each other with real packets.
- Issue #336 was fixed. AWS `CreateVpc`/`CreateSubnet`, EC2 ENIs from `RunInstances` and Auto Scaling, Elastic IP allocation, NAT gateways, and NAT routes now use the substrate. GCP Compute networks/subnetworks, instance NIC allocation, regional addresses, and Cloud NAT now use the substrate. Azure virtual networks/subnets, NIC private IP/MAC allocation, public IP allocation, and NAT gateway subnet programming now use the substrate. These paths require Linux network namespace, bridge/veth, routing, and nftables capabilities and fail loudly when the host cannot provide them.
- The PR #358 CI fix made those real-network test paths runnable in the simulator Docker image without fallbacks: real-network Docker targets keep root capabilities, provider public IPv4 pools are explicit for AWS/GCP/Azure, GCP real fabric names are hash-derived to avoid Linux-name collisions, GCP fabric mutations are serialized with the simulator registry, and Docker test builds exclude local caches and generated artifacts from the context.
- AWS security groups and ELBv2 were the first #335/#334 public paths to move beyond metadata: EC2 ENI ingress rules compile to nftables on the real veth path, ELBv2 target health uses real TCP/HTTP probes, and ELBv2 data-plane requests route by load-balancer DNS host to healthy targets without the Query Protocol control plane binding listener ports.
- Azure Event Grid topic publish also follows the control-plane/data-plane separation rule: ARM responses advertise a shared host-dispatched topic endpoint instead of leaking per-topic localhost listener plumbing, and publish requests route by Event Grid Host header through the Azure simulator.

Firecracker-backed public VM lifecycle landed for AWS EC2, GCP Compute Engine, and Azure VMs. The shared substrate added TAP NIC attachment to cloud subnet bridges and a Firecracker launcher that prepares provider-private-IP guest root filesystems, boots the guest inside the cloud network namespace, gates public running state on real packet reachability, creates and sizes `rootfs.ext4` explicitly, uses unique per-launch default workdirs, verifies Firecracker kernels as ELF images before booting, and copies the extracted official rootfs contents without nesting the source directory under the per-VM rootfs. AWS EC2 `RunInstances` returns `pending` and transitions only after boot; stop/start/terminate operate on the guest. GCP and Azure VM lifecycle handlers use the same substrate, and GCP firewall/Azure NSG filters apply to TAP NICs. VM-backed simulator CI uses x64 KVM runners, while container/FaaS workload slices derive architecture from the local image or public Lambda `Architectures` field instead of forcing arm64. The guest metadata plane routes provider-shaped `169.254.169.254:80` through nftables DNAT in each cloud network namespace to the simulator metadata handlers, preserving guest private source IP for instance-specific metadata. GCP guest rootfs setup also maps `metadata.google.internal` / `metadata` to the link-local metadata IP. Issue #332 and the stale #338 meta tracker were closed by the umbrella audit.

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

The AWS core-services sweep fixed issues #341, #343, #346, and #347:

- CloudTrail implements trail CRUD, logging status, event selectors, tag operations, `LookupEvents`, recording of simulator API calls, and gzipped CloudTrail object delivery into S3. SDK, CLI, and Terraform `aws_cloudtrail` coverage exercise the public client surfaces.
- Auto Scaling implements launch configurations, Auto Scaling Groups, desired-capacity updates, scaling activities, tags, and deletion. ASGs create and terminate EC2 instances through the EC2 simulator slice, and SDK/CLI/Terraform coverage verifies desired-capacity materialization.
- EBS implements create/attach/detach/delete/modify volumes, snapshot create/describe/delete, pending-to-completed snapshot state, and volume restore from snapshot. ECS managed EBS `volumeConfigurations` create real EBS-backed host directories, mount them into task containers, snapshot the written bytes, restore from the snapshot, and prove the bytes are present in a later task through SDK and CLI tests. EC2-attached EBS data volumes are wired into Firecracker guests as file-backed block devices, and guest-written block data persists through snapshot/restore. Terraform coverage exercises EC2 EBS volume, attachment, snapshot, and restore resources.
- S3 `ListObjectVersions` was added so Terraform `force_destroy` can enumerate and remove CloudTrail-delivered objects.

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
- BUG-1267: closed by the real-execution umbrella audit. Future real-execution regressions should be filed as concrete issues/BUGs unless the whole architecture regresses.

## Current Capability Summary

- Docker-compatible REST API with cloud backends for AWS, GCP, and Azure plus Docker passthrough.
- AWS/GCP/Azure simulators with SDK, CLI, and Terraform validation for the public API slices sockerless depends on.
- Bleephub GitHub API simulator compatible with real `gh` CLI paths.
- Admin UI and local stack orchestration.
- Foundational simulator parity exists for object storage, queues, events, streams, managed data SaaS, DNS, VPC/networking, NAT/public-IP, managed load balancers, and VM/instance control planes.

## History

Detailed phase history has been intentionally removed from continuity docs to keep fresh sessions actionable. Use PR descriptions, issue threads, and `git log` for older per-phase details.
