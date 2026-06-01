# Sockerless - What We Built

Roadmap [PLAN.md](PLAN.md) - status [STATUS.md](STATUS.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md).

This file is intentionally compact. Detailed phase history lives in PR descriptions, `git log`, and issue threads. Keep this file focused on facts that a fresh session needs after context compaction.

## 2026-06-02 - CI Log and Cloud Backend Architecture Cleanup

The cleanup after PR #358 fixed two classes of issues without skipping tests, suppressing warnings, adding fallbacks, or weakening HTTPS/gateway behavior.

Cloud backend `ContainerInspect` and `ContainerList` paths stopped delegating into core `BaseServer` local-state handlers. The cloud backends now resolve/list through cloud state explicitly, and `BaseServer.ContainerList` returns provider list errors instead of silently falling back to incomplete local/pending state. `scripts/check-cloud-backend-isolation.sh` now scans all cloud backend Go files and fails future direct `BaseServer.ContainerInspect` / `BaseServer.ContainerList` use.

The same phase removed misleading pass-green CI output. The GCP host-dispatch allowlist test still enforces the same invariant, but no longer logs a source path in a way GitHub annotates as `##[error]`. Vite package builds run under Bun's runtime to avoid Node 26's Tailwind `module.register()` deprecation warning, `ui-core` emits declaration artifacts for Turbo output tracking, and UI app tests start from `/ui/` so React Router does not print unmatched-route stderr.

The same PR fixed newly opened AWS simulator issues #359 and #360. EC2 EBS snapshots now carry an internal completion due time and settle on public `DescribeSnapshots` and `CreateVolume(SnapshotId)` paths, so a standard CreateVolume -> CreateSnapshot -> DescribeSnapshots -> CreateVolume(SnapshotId) flow observes `completed` and restores successfully. DynamoDB `DeleteItem` now honors `ReturnValues=ALL_OLD` by returning the deleted item's pre-delete attributes when the item existed and no attributes when it did not. Both fixes shipped with real AWS SDK regression coverage.

The pre-push dependency freshness gate also found and fixed stale live-test AWS credential action pins. The live ECS and Lambda workflows now use `aws-actions/configure-aws-credentials@v6.2.0`.

## 2026-06-01 - Real Network Fabric For Issue #336

Issue #336 was fixed. The shared `simulators/realexec` substrate models each cloud network/VPC implementation object as its own Linux network namespace instead of placing the bridge in the host namespace.

`CreateNetwork` creates the network namespace, creates the bridge and gateway address inside it, and brings loopback and the bridge up there. Networks can own multiple subnet bridges. `AttachNamespaceNIC` creates a veth pair, moves the host-side peer into the network namespace, enslaves it to the subnet bridge there, then moves the guest-side peer into the workload namespace with its leased private IP, unique MAC, and default route. The substrate also creates routed egress veth links and programs nftables SNAT for NAT egress.

AWS EC2 VPC/subnet creation, `RunInstances`/Auto Scaling ENIs, Elastic IP allocation, NAT gateways, and NAT routes use the substrate. GCP Compute networks/subnetworks, instance NICs, regional addresses, and Cloud NAT use it. Azure virtual networks/subnets, NIC private IP/MAC allocation, public IP allocation, and NAT gateway subnet programming use it. These public API paths now fail loudly on hosts without the required Linux network namespace, bridge/veth, route, and nftables capabilities.

The mandatory `make realexec-network-test` path proves bridge placement, guest-to-gateway and namespace-to-namespace packet reachability, routed egress reachability, SNAT programming, and cleanup of guest/network namespaces and nftables state.

Simulator Docker test targets expose the required real networking privileges to the Linux test image. That keeps Darwin/local Docker harnesses on the real namespace/nftables path instead of hiding the requirement or falling back to metadata behavior.

The same PR fixed the GCP Terraform HTTPS CI failure without weakening tests. Provider public IPv4 leases now come from explicit AWS, GCP, and Azure public pools rather than a documentation block, and Terraform assertions verify the exact provider-shaped CIDRs. GCP load-balancer forwarding rules allocate and release real public IPv4 leases. GCP Linux namespace/bridge/veth names are hash-derived from the full cloud resource ID so different public resource names do not collide after Linux's 15-character truncation. GCP real network, subnet, and NIC creation is serialized with the in-memory fabric registry, so parallel Terraform operations cannot interleave duplicate substrate creation. The simulator Docker test targets preserve the real-network capabilities supplied by `--privileged`; they no longer drop to the host UID before running tests that require `CAP_NET_ADMIN` and `CAP_SYS_ADMIN`. The Docker build context excludes local caches, Terraform state, generated simulator binaries, and local agent metadata.

The same PR started the remaining BUG-1267 packet-path work without closing #333-#335. The shared substrate gained nftables ingress filtering on real NIC veth peers. AWS security-group ingress rules for EC2 ENIs now compile into that filter, and rule updates reprogram attached ENIs. AWS ELBv2 target health now performs real TCP/HTTP probes instead of returning hardcoded `healthy`, and AWS ELBv2 data-plane requests route by load-balancer DNS host to healthy targets without the Query Protocol control plane binding listener ports. Azure Event Grid topic ARM create/get/list stopped allocating per-topic `127.0.0.1:<random>` publish listeners and now advertises the shared host-dispatched topic endpoint or the configured Caddy HTTPS gateway template. GCP/Azure security and load-balancer data-plane migrations remained for the next BUG-1267 phase, and #333 Firecracker-backed public VM lifecycle remained open.

## 2026-06-01 - Azure Tables ARM Fidelity

Issue #356 was fixed. The Azure simulator now implements the public ARM surfaces for Azure Cosmos DB Tables and Azure Storage Tables.

Cosmos DB Tables now supports `PUT`, `GET`, `DELETE`, and list at `Microsoft.DocumentDB/databaseAccounts/{account}/tables/{table}`, plus table throughput get/update at `.../throughputSettings/default`. The implementation follows the official Azure REST spec and the path used by terraform-provider-azurerm's `azurerm_cosmosdb_table`.

Storage Tables now supports ARM create/update/get/delete/list at `Microsoft.Storage/storageAccounts/{account}/tableServices/default/tables/{table}`. ARM-created Storage and Cosmos tables are projected into the same Azure Tables data-plane store used by `/Tables` and entity routes, and table delete removes associated entities. The Tables data plane now also honors the real `Prefer: return-no-content` create contract and implements table ACL get/set for the Giovanni client path used by `azurerm_storage_table`.

The fix shipped with official Azure SDK tests (`armcosmos.TableResourcesClient`, `armstorage.TableClient`), Azure CLI `az rest` tests, and Terraform apply/destroy coverage for `azurerm_cosmosdb_table` and `azurerm_storage_table`.

## 2026-06-01 - Azure Event Grid Host-Dispatched Publish

Azure Event Grid topic endpoints stopped leaking simulator-local data-plane plumbing. Topic ARM create/get/list now advertise the shared host-dispatched Event Grid publish endpoint, or the configured Caddy HTTPS gateway template, rather than allocating per-topic `127.0.0.1:<random>` listener URLs.

Publish requests route through the Azure simulator by Event Grid Host header and fan out to webhook subscriptions. Regression tests cover endpoint shape on create/get/list and real webhook delivery through the advertised endpoint.

## 2026-06-01 - AWS Core Services and CI Flake Hardening

Issue #310's reopened GCP timestamp gap was fixed. Eventarc trigger timestamps, Firestore document timestamps, and Pub/Sub publish times now use the shared canonical protobuf timestamp formatter.

AWS CloudTrail, Auto Scaling, and EBS support were added with public SDK, CLI, and Terraform coverage where the client surfaces exist. CloudTrail records simulator API calls and writes gzipped S3 log objects. Auto Scaling Groups materialize and terminate EC2 instances through the EC2 slice. EBS volumes and snapshots now support lifecycle, pending-to-completed snapshots, restore, and ECS managed EBS task byte round trips through real task containers. S3 `ListObjectVersions` was added so Terraform cleanup can delete CloudTrail-delivered objects.

CI flakiness from hosted linux/arm64 CPU runner shutdowns was fixed without skipping tests. GitHub Actions now caches Go modules using the workspace's real dependency files (`go.work` and `**/go.sum`). Non-Terraform CI step and Go test timeouts stay capped at five minutes so slow or hung tests fail loudly; Terraform provider CI is capped at ten minutes for real apply/destroy coverage. Status-only aggregate jobs were removed, CI has no `needs` edges, and the workflow runs 29 jobs. The initial overcorrection that serialized arm64 matrices and expanded lint into nineteen one-module jobs was corrected before merge: backend, FaaS smoke, e2e, simulator, Terraform, and smoke jobs fan out without phase-ordering gates, lint/backend/FaaS smoke coverage is grouped into fewer real-coverage shards, and AWS CLI simulator tests are sharded by service family with every existing CLI test selected exactly once.

The final CI pass fixed a real GCP Cloud Logging append race exposed by parallel simulator jobs. Cloud Functions and Cloud Run collect stdout and stderr from separate real Docker log scanner goroutines; GCP Cloud Logging now serializes appends to a log name so concurrent stdout/stderr lines are preserved instead of overwriting one another. AWS CloudWatch already used atomic store updates, and Azure Monitor already serialized appends.

All GitHub Actions workflows now auto-cancel stale runs. Each workflow's concurrency group is keyed by workflow name plus PR source branch/ref, with `cancel-in-progress: true`, so a new push to a PR branch stops older queued or running pipelines for that same branch/ref.

## 2026-05-31 - Local HTTPS Gateway Stage 1

The local HTTPS gateway was implemented as optional transport infrastructure for simulator APIs. It did not change simulator public cloud API shapes.

The stage added `make stack-https-up`, `make stack-https-status`, `make stack-https-ca`, and `make stack-https-down`. The gateway runs Caddy under the repo's existing `.stack-pids` and `.sockerless-state` conventions, fronting the normal simulator HTTP ports with HTTPS names:

- `https://aws.sockerless.localhost:8443`
- `https://gcp.sockerless.localhost:8443`
- `https://azure.sockerless.localhost:8443`
- Azure host-addressed data-plane wildcards such as `{account}.blob.azure.sockerless.localhost`, `{vault}.vault.azure.sockerless.localhost`, and `{namespace}.servicebus.azure.sockerless.localhost`.

`STACK_HTTPS=1 make stack-azure-aca` now starts the gateway with the local stack and configures Azure ARM-advertised data-plane endpoint projection under the gateway hostnames. Direct HTTP and direct simulator TLS through `SIM_TLS_CERT` / `SIM_TLS_KEY` remain supported.

Caddy's local CA trust-store installation was disabled with `skip_install_trust`. The gateway still issues internal certificates, and provider tests still validate TLS by trusting the exported Caddy root through `SSL_CERT_FILE` or equivalent client CA knobs. This avoided non-interactive CI hangs and did not introduce insecure TLS or a fallback path.

The admin UI topology page now shows gateway status, endpoints, CA path, and the equivalent recovery `make` commands.

## 2026-05-31 - Azure Terraform Through Local HTTPS Gateway

The Azure Terraform harness was moved from generated simulator TLS certificates to the optional local Caddy gateway. The simulator now starts on HTTP loopback for the test, Caddy terminates HTTPS on a random high port, Terraform uses `https://azure.sockerless.localhost:<port>` for AzureRM metadata and ARM endpoints, and the Linux Docker test container trusts Caddy's local CA through `SSL_CERT_FILE`.

The shared simulator Docker test image now includes Caddy from the official package repository, so the macOS delegation path and Linux container path run the same real gateway flow. The harness verifies the direct simulator metadata route before starting Caddy, waits for Caddy's local CA file, verifies `/health`, and validates Azure metadata JSON through the gateway before Terraform starts.

The gateway also preserved high-port Host headers and routed `*.documents.azure.sockerless.localhost` for Cosmos DB document endpoints. BUG-1246 fixed an Azure Storage data-plane middleware bug where the storage wrapper overmatched non-storage `*.localhost` hosts and swallowed `azure.sockerless.localhost` metadata requests.

BUG-1247 fixed the direct GitHub Actions Azure Terraform job by installing Caddy before `make terraform-test`; the Docker test image already had Caddy for containerized runs. BUG-1248 fixed GCP arithmetic SDK coverage to assert the actual Cloud Logging output line, `"Result: 30"`. BUG-1249 made the Azure Terraform harness fail loudly when Caddy is missing or a future edit tries to run that provider path without HTTPS.

The full Azure Terraform apply/destroy test passed through the gateway under the 300-second cap.

## 2026-05-31 - SDK/CLI HTTPS Gateway and GCS CLI Audit

The gateway docs were expanded with SDK/CLI-specific endpoint and CA trust knobs for AWS CLI/SDKs, gcloud/Google clients, Azure CLI, and Azure SDKs. The guidance kept TLS verification enabled: local clients trust the Caddy CA instead of disabling certificate checks.

BUG-1104 audit found stale GCP GCS CLI coverage. Current gcloud supports Cloud Storage endpoint overrides, so `gcp-gcs` was no longer a CLI "not applicable" surface. BUG-1250 added real `gcloud storage` bucket/object lifecycle coverage, fixed the simulator to accept current gcloud multipart upload boundaries, and implemented the public GCS `buckets.getStorageLayout` response used by gcloud's upload path. BUG-1251 corrected GCS timestamp precision to Cloud Storage-style milliseconds so current Linux gcloud did not inject timestamp truncation warnings into command output.

## 2026-05-31 - Terraform HTTPS Gateway and Coverage Audit

AWS and GCP gained optional Terraform HTTPS gateway harnesses without removing the direct HTTP Terraform path. `make terraform-https-test` starts each simulator on HTTP loopback, starts Caddy with isolated state, trusts Caddy's local CA with `SSL_CERT_FILE`, and runs the real provider apply/destroy suite through the gateway's `https://localhost:<ephemeral-port>` single-simulator route. AWS covers the root production-shape Terraform stack plus the RDS and ElastiCache subpackages through that same HTTPS path. On macOS those targets run inside the shared Linux simulator test image so provider CA trust matches CI. The public named gateway hosts remained available for normal stack use; the harness route avoided wildcard `.localhost` resolver differences.

The AWS Terraform Make targets now build the real simulator once and pass that binary into every Terraform package. The package list still runs concurrently, and `go test -json` emits package-qualified events so CI no longer hides the root, RDS, and ElastiCache provider flows behind one opaque silent period.

Terraform CI kept Caddy HTTPS for provider validation. Azure remained mandatory through the gateway because AzureRM metadata discovery requires trusted HTTPS; AWS/GCP used their new HTTPS gateway targets in CI while `make terraform-test` stayed available for direct HTTP.

BUG-1104 audit found stale GCP VPC Access Terraform coverage. BUG-1253 added `vpc_access_custom_endpoint`, provisioned `google_vpc_access_connector` in the GCP Terraform stack, asserted the canonical connector ID, and marked `gcp-vpcaccess` Terraform coverage direct.

The same audit found larger stale GCP client-surface rows for API Gateway, Cloud Build, IAM, and Pub/Sub. BUG-1254 / issue #304 was opened so those public gcloud/Terraform coverage gaps were explicit and tracked.

## 2026-05-31 - AWS Simulator Fidelity Sweep

Issues #305-#308 and #317-#320 were fixed in one AWS simulator PR.

S3 `ListObjectsV2` returned keys in ascending bytewise order, honored `start-after` and `continuation-token`, emitted usable `NextContinuationToken`, and returned delimiter `CommonPrefixes`. Lambda `FunctionConfiguration` responses stopped leaking request-only `Code`, uploaded `ZipFile`, and `Tags`; `GetFunction` kept `Code` and `Tags` only as top-level response members.

SNS returned the literal `pending confirmation` for confirmation-required protocols unless `ReturnSubscriptionArn=true`, and topic attributes counted pending vs confirmed subscriptions. SQS `ReceiveMessage` rejected out-of-range `MaxNumberOfMessages` with `InvalidParameterValue` instead of silently clamping.

EC2 `RunInstances` honored `MinCount`/`MaxCount`, returned instances in `pending`, and transitioned them to `running`; `DescribeInstances` applied supported filters and rejected unsupported filter names. ECR `PutImage` produced deterministic content-addressed `sha256:<64-hex>` manifest digests. KMS `GenerateDataKey` used fresh crypto-random key material and returned ciphertext that decrypted to the generated plaintext.

The fixes shipped with real AWS SDK coverage for each issue and targeted AWS CLI regression coverage for the affected AWS surfaces.

## 2026-05-31 - AWS Amplify Fidelity Sweep

Issues #330 and #331 were fixed in one AWS simulator PR.

Amplify `StopJob` and `DeleteJob` now used separate public AWS REST paths and semantics. `StopJob` handled `DELETE /apps/{appId}/branches/{branchName}/jobs/{jobId}/stop` and cancelled the job. `DeleteJob` handled `DELETE /apps/{appId}/branches/{branchName}/jobs/{jobId}`, removed the job, removed its artifacts, updated branch job counters, and made later `GetJob` calls return `NotFoundException`.

The missing Amplify artifact and access-log operations were added on the AWS SDK paths: `ListArtifacts`, `GetArtifactUrl`, and `GenerateAccessLogs`. Started jobs and deployments now had stored artifact records so `ListArtifacts` and `GetArtifactUrl` exercised real simulator state instead of returning arbitrary IDs. `GenerateAccessLogs` validated that the requested app owned the requested default or associated domain before returning the public response shape.

The fixes shipped with real AWS SDK and AWS CLI coverage. Terraform coverage was not added because the official Terraform AWS provider does not expose these Amplify job artifact or access-log operations.

## 2026-05-31 - GCP Fidelity and Client-Surface Sweep

Issue #304 and issues #309-#311 and #321-#325 were fixed in one GCP simulator PR.

The stale GCP client-surface rows were corrected with real public clients. API Gateway, Cloud Build, IAM, and Pub/Sub now have gcloud coverage where the public CLI exposes the surface. API Gateway also has Terraform provider coverage through `google-beta` resources for APIs, API configs, and gateways.

Cloud Run services/jobs/executions and Cloud Functions list paths now return empty arrays instead of JSON nulls, apply stable ordering, support `pageSize`/`pageToken`, and emit canonical millisecond timestamps. Long-running operations now use the public metadata `@type` values expected by official Google SDK operation decoders.

Cloud Logging severity comparisons now use Google severity ranks. GCS object metadata now includes generation, metageneration, CRC32C, MD5 where applicable, and compose component counts. Bucket IAM policy responses now include public `kind`, `resourceId`, and base64 etags. Cloud SQL backup insert/delete paths now return SQL Admin operation shapes, and Cloud DNS precondition mismatches now return canonical `FAILED_PRECONDITION` error details.

The fixes passed the GCP simulator package tests plus real Google SDK, gcloud, and Terraform provider suites.

## 2026-05-31 - Azure ARM and Private DNS Fidelity Sweep

Issues #313, #314, and #340 were fixed in one Azure simulator PR.

Azure ARM control-plane paths now enforce the public `api-version` query parameter contract and return `InvalidApiVersionParameter` when it is absent. The unused `AzureRouter` validator was removed so the live middleware is the only ARM validation path.

Store-backed Azure ARM list responses now serialize empty lists as `{"value":[]}` instead of `{"value":null}`. This fixes the common ARM `*ListResult` shape at the state-store boundary instead of patching one handler at a time.

Azure Private DNS now implements `GET .../privateDnsZones` for list-by-resource-group, matching `armprivatedns.PrivateZonesClient.NewListByResourceGroupPager`. Private DNS virtual network links now implement `GET .../privateDnsZones/{zoneName}/virtualNetworkLinks`, matching the public list-by-zone route. Both route families are covered by real Azure SDK and Azure CLI tests.

## 2026-05-31 - Azure API Shape and LRO Fidelity Sweep

Issues #312, #315, and #326-#329 were fixed in one Azure simulator PR.

Azure Storage Blob/File/Queue data-plane errors now use XML `<Error>` envelopes, `Content-Type: application/xml`, and `x-ms-error-code`. Blob `ListContainers` and `ListBlobs` responses now emit the XML declaration, public `EnumerationResults` attributes, and `NextMarker`. Queue service properties now return the public service-properties XML used by the Terraform provider's data-plane availability check.

Service Bus admin data-plane reads for missing queues, topics, subscriptions, and rules now return 404 Atom/XML errors instead of successful empty feeds. Event Grid topic publish now rejects malformed JSON, empty batches, missing required Event Grid envelope fields, and invalid event times before delivering to subscriptions.

Azure Cache for Redis, PostgreSQL Flexible Server, and Event Hubs namespace create operations now return 201 with `Azure-AsyncOperation`, `Location`, and `Retry-After` headers. The created resources start in their public in-progress states and converge through the operation endpoint to `Succeeded`, `Ready`, or `Active` as appropriate.

Key Vault secret, key, and certificate attributes now include default soft-delete recovery metadata: `recoveryLevel` is `Recoverable+Purgeable`, and `recoverableDays` is 90.

The fixes were covered through real Azure SDK integration tests plus raw HTTP assertions for wire details that official SDKs normalize.

## 2026-06-01 - Real-Execution Substrate Contract

The first BUG-1267 stage established the host-model contract for issues #332-#336 without pretending the VM/VPC/LB data plane was already implemented.

`specs/SIMULATOR_EXECUTION.md` now describes the current Docker/Podman-backed container/FaaS execution model and the narrow VM-level exception for the future real-execution substrate. `specs/SIMULATOR_REAL_EXECUTION.md` defines the substrate objects and rules for Firecracker guests, Linux netns/bridge/tap/veth routing, nftables security/NAT, real load-balancer listeners, active health checks, per-instance metadata, and loud capability failures. `feedback_sim_host_model.md` records which host process paths are allowed.

The AWS/GCP/Azure host-dispatch tests were updated to keep rejecting broad host-process workload execution while pointing at the explicit Firecracker/Linux-networking substrate exception.

CI now includes a mandatory `firecracker (microVM arithmetic)` job. It installs pinned Firecracker v1.15.1, requires `/dev/kvm`, boots a real Firecracker Linux guest, copies the repo's `simulators/testdata/eval-arithmetic` source and configured Go toolchain into the guest rootfs, then runs `go test`, `go build`, and multiple arithmetic executions inside the microVM. This is a real substrate smoke test, not a mock or metadata-only probe.

At that point, issues #332-#336 remained open because this stage did not yet change EC2/GCE/Azure VM, VPC, firewall, NAT, or load-balancer public behavior. Later BUG-1267 stages moved #336 and the first AWS #334/#335 packet paths onto the substrate.

## 2026-06-01 - Real-Execution Host Network Substrate

The next BUG-1267 stage added the first shared implementation code for the real-execution substrate without changing cloud public API behavior.

[simulators/realexec](simulators/realexec) now provides deterministic host capability checks for Linux, required substrate commands, `/dev/kvm`, and kernel capabilities; an idempotent LIFO cleanup stack; an auditable host command runner for substrate operations; lease-based IPv4 IPAM; and Linux bridge/netns/veth NIC creation. IP allocation reserves unusable, gateway, network, broadcast, and duplicate addresses instead of deriving addresses from counters or store length.

`make realexec-network-test` runs only on Linux hosts with the required real tools and privileges. It creates a real bridge, network namespaces, veth NICs, IP leases, routes, and an nftables table; verifies packet reachability to the gateway and between namespaces; and verifies cleanup removes host artifacts. The mandatory Firecracker CI job runs this host-network target after the microVM arithmetic target, so CI guards both real guest execution and the first real network/NIC path.

At that point, issues #332-#336 remained open because this stage still did not migrate EC2/GCE/Azure VM, VPC, firewall, NAT, or load-balancer public API paths onto the substrate. Later BUG-1267 stages moved #336 and the first AWS #334/#335 packet paths onto the substrate.

Validation also fixed BUG-1270 and BUG-1271 in the shared Makefile layer. `make test` now uses `-tags noui` for UI-bearing Go apps, backend integration tests are behind an explicit `integration` build tag, integration CI/Make targets opt into `noui integration`, UI packages without a `test` script report that cleanly, and Go libraries have a `build-noui` compile-check alias for the top-level fanout.

## 2026-06-01 - GCP Timestamp Reopen and AWS Core Services

Issue #310 was fixed after it was reopened. Eventarc trigger create/update timestamps, Firestore document timestamps, and Pub/Sub publish times now use the shared canonical protobuf timestamp formatter instead of `time.RFC3339Nano`.

AWS core-service issues #341, #343, #346, and #347 were fixed in one simulator PR.

CloudTrail now implements trail CRUD, logging start/stop/status, event selectors, tag operations, and `LookupEvents`. The simulator records real API calls made against the AWS simulator and, for logging trails, writes gzipped CloudTrail JSON objects into S3 under the public `AWSLogs/<account>/CloudTrail/<region>/...` key shape. SDK, CLI, and Terraform `aws_cloudtrail` coverage exercise the public client surfaces.

Auto Scaling now implements launch configurations, Auto Scaling Groups, desired-capacity changes, scaling activities, tags, and deletion. ASG reconciliation creates and terminates EC2 instances through the EC2 simulator slice so `DescribeAutoScalingGroups` and `DescribeInstances` observe the same fleet.

EBS now supports create/attach/detach/delete/modify volumes, create/describe/delete snapshots, pending-to-completed snapshot state, and restore from snapshot. ECS managed EBS `volumeConfigurations` create real EBS-backed host directories, mount them into ECS task containers, snapshot bytes written by one task, restore from that snapshot, and prove the bytes are readable in a later task. This avoids local storage fakes while keeping the public AWS API shape.

S3 added `ListObjectVersions` so Terraform provider cleanup can enumerate and remove CloudTrail-delivered objects when `force_destroy` is enabled.

At that point, issues #332-#336 remained open. That PR did not migrate EC2/GCE/Azure VM, VPC, security, NAT, or load-balancer public APIs onto Firecracker and the real network substrate. Later BUG-1267 stages moved #336 and the first AWS #334/#335 packet paths onto the substrate.

## 2026-05-31 - Terraform Provider HTTPS Behavior Audit

We checked whether a generic local HTTPS gateway for simulator APIs made sense, especially for Terraform providers that require HTTPS even when pointed at a local simulator.

The answer was cloud-specific:

- AzureRM requires trusted HTTPS for custom metadata discovery. Its `metadata_host` setting is a hostname, and the provider constructs `https://<host>` internally.
- Azure Stack is also HTTPS-shaped for ARM/metadata endpoint use.
- AzAPI exposes full endpoint URLs and defaults to HTTPS Azure endpoints. It is more configurable than AzureRM, but should work through the same gateway.
- AWS and GCP do not require HTTPS for the current simulator Terraform paths. Their providers accept full custom endpoint URLs, and current sockerless Terraform harnesses use HTTP successfully.

The planned implementation is therefore an optional Caddy/local-HTTPS front door, not a simulator public API change. It should front all three simulators for developer ergonomics, while preserving existing direct HTTP and `SIM_TLS_CERT` / `SIM_TLS_KEY` modes. Azure Terraform is the first target because provider behavior actually requires trusted HTTPS there.

## 2026-05-31 - Latest Merged Simulator Work

Issue #298 and BUG-1242..BUG-1245 were closed.

What changed:

- Azure Cache for Redis has real Azure CLI and azurerm Terraform coverage for cache lifecycle, `listKeys`, firewall rules, SKU round-tripping, and PATCH.
- GCP Memorystore Redis has real `gcloud redis instances` and terraform-provider-google `google_redis_instance` coverage.
- GCP Cloud SQL exposes both `/v1` and `/sql/v1beta4` SQL Admin paths needed by SDK, gcloud, and terraform-provider-google flows.
- GCP Cloud DNS implements public Changes.Create/Get/List and ResourceRecordSets.Get/Patch, including exact delete/add validation and unknown-change NOT_FOUND behavior.

## Current Capabilities Snapshot

Sockerless provides a Docker-compatible REST API backed by cloud backends and local simulators. The project has:

- Backends for Docker passthrough, AWS ECS/Lambda, GCP Cloud Run/GCF, and Azure ACA/AZF.
- One simulator binary per cloud: `simulators/aws`, `simulators/gcp`, and `simulators/azure`.
- Simulator coverage through official SDKs, vendor CLIs, and official Terraform providers.
- Admin UI and stack orchestration for local components.
- Bleephub GitHub API simulator compatible with real `gh` CLI paths.

Recent simulator parity work added or hardened foundational cloud slices across object storage, queues, event systems, streams, DNS, data SaaS, VPC/networking, public IP/NAT, managed load balancers, and VM/instance control planes.

## Deferred Work

- BUG-1075: live-cloud validation remains intentionally deferred. Do not mark live cloud cells green without authenticated real-cloud runs.
- BUG-1104: audit cadence remains open. Every simulator phase should re-check SDK/CLI/Terraform surface claims and file concrete BUG entries before fixing; the GCP GCS CLI and VPC Access Terraform audits closed stale "not applicable" rows.
- BUG-1254 / issue #304 and BUG-1263 / issues #309-#311 and #321-#325 were fixed in the GCP fidelity sweep.
- BUG-1264 / issues #312, #315, and #326-#329 were fixed in the Azure API-shape and LRO sweep.
- BUG-1267: issues #332-#335 and #338 track the remaining cross-cloud compute/networking real-execution program. Issue #336's VPC/network/subnet/NIC/public-IP/NAT routing fabric landed on the real substrate, AWS security-group ingress plus ELBv2 host-dispatched health/proxying were the first #335/#334 packet-path migrations, and Azure Event Grid stopped leaking simulator-local publish listener plumbing from ARM. Firecracker-backed public VM APIs, GCP/Azure security enforcement, and remaining load-balancer data-plane work remain.

## Continuity Rules

- No mocks, fakes, synthetic behavior, silent fallbacks, or degraded modes.
- Public simulator endpoints must match real cloud APIs. Admin/local infrastructure can exist, but it must not leak into public cloud API surfaces.
- Any new simulator public API slice requires SDK, CLI, and Terraform coverage unless the public service has no such client surface.
- User merges PRs. Agents create branches and PRs only.
