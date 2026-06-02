# Sockerless - Status

Roadmap [PLAN.md](PLAN.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Snapshot

| | |
|---|---|
| Active branch | `main` - no implementation branch active. |
| In-flight | None. |
| Planned next | Continue BUG-1267 follow-through on guest metadata/data-plane verification for #333/#338. Versioned releases/image publishing remain deferred while the project is early. |
| Last merged | Firecracker-backed VM lifecycle for AWS EC2, GCP Compute Engine, and Azure VMs. |
| Open GitHub issues | #332, #333, and #338 at last check. #332/#333 were advanced by real guest lifecycle; #333 remains open until guest metadata reachability is implemented/verified. Number #337 is a merged PR, not an open issue. |
| Bugs | 1299 filed - 1298 fixed - 4 open - 2 false positives. |
| Open BUGs | BUG-1075 live-cloud validation; BUG-1104 audit cadence; BUG-1267 compute/networking real execution; BUG-1299 / issue #371 guest metadata plane. |
| Live infra | None up. |

## Current State

The Firecracker-backed VM lifecycle phase advanced issues #332 and #333. The shared `simulators/realexec` substrate now supports TAP NICs attached to cloud subnet bridges and a Firecracker launcher that downloads the pinned official Firecracker CI kernel/rootfs assets, prepares per-VM Ubuntu ext4 root filesystems with the cloud-private IPv4 address/gateway, starts the real Firecracker process inside the cloud network namespace, configures the Firecracker API over its Unix socket, and waits for real guest packet reachability before public VM state becomes running.

AWS EC2 `RunInstances` now creates the control-plane instance/ENI/volume rows, returns `pending`, and transitions to `running` only after the Firecracker guest boots on the instance private IP. `StopInstances`, `StartInstances`, and `TerminateInstances` stop, restart, or delete the real guest/TAP lifecycle, and `DescribeInstances` reconciles stale persisted `running` state with live guest process state. GCP Compute Engine instance insert/start/stop/delete and Azure VM PUT/start/powerOff/restart/deallocate/delete now follow the same real guest lifecycle while preserving the public provider status names. GCP firewall rules and Azure NSGs apply to Firecracker TAP NICs as well as the existing namespace NIC users. Simulator CI SDK/CLI/Terraform jobs install Firecracker and the rootfs tooling because VM public APIs now require the real substrate.

BUG-1299 / issue #371 remains open because per-instance metadata reachability from inside Firecracker guests still needs a real network-namespace data-plane implementation before #333 can be fully closed.

Issues #366 and #367 were fixed earlier. Simulator image builds now use the shared `simulators/` Docker context everywhere the runtime images are built: the publish-container-images workflow, `simulators/docker-compose.yml`, and the per-cloud `docker-build` Make targets. Each Dockerfile copies the shared context and builds from `/src/aws`, `/src/gcp`, or `/src/azure`, so each module's `../realexec` replace target is present. `simulators/.dockerignore` keeps release-image contexts focused on source by excluding test harnesses, generated test binaries, Terraform caches/state, built UI assets, and local simulator binaries. Local Docker builds for all three simulator images passed from the fixed context, and the context shrank from roughly 1.1 GB of local artifacts to roughly 3.5 MB.

`SIM_RUNTIME=process` is now documented as an explicit API-only startup mode for simulator runs that do not invoke workload execution. It is not a fallback or degraded execution path. Docker/Podman remains required for workload execution, and startup fatal messages now state that requirement while pointing API-only operators to `SIM_RUNTIME=process`. Common and per-cloud README files plus the simulator command comments were updated.

Issue #365 / BUG-1296 was fixed. Local Azure SDK validation on macOS now routes through the same real Linux substrate required by the network-heavy SDK tests: Darwin `make sdk-test` delegates to `docker-sdk-test`, which runs the existing privileged Linux simulator test image and invokes `make sdk-test-local` inside it. Linux direct validation remains `make sdk-test-local`. The Event Grid SDK publish path also became resolver-independent without changing the public request shape: the SDK harness maps only `eventgrid.localhost` / `*.eventgrid.localhost` for the simulator port to loopback at dial time, preserving the advertised URL and Host header for the host-dispatched data plane. `NO_PROXY` / `no_proxy` include localhost wildcard names. No tests were skipped, mocked, or weakened; full macOS `make sdk-test` passed through Docker.

Issue #362 was fixed. The Azure Entra simulator now serves the `GET /{tenant}/oauth2/v2.0/authorize` endpoint advertised by discovery. Discovery returns simulator-local absolute URLs for the served auth/token/JWKS endpoints plus supported response types/modes, grant types, PKCE methods, and signing algorithm metadata. Authorization-code requests validate the documented public OAuth/OIDC parameters, issue short-lived one-time codes, preserve `state`, support `query`, `fragment`, and `form_post` response modes, and redeem codes at `/oauth2/v2.0/token` with matching tenant/client/redirect URI plus PKCE validation. The token endpoint now rejects unsupported grant types, returns signed RS256 access tokens through the existing JWKS path, returns an ID token when `openid` is in scope, and issues/redeems refresh tokens when `offline_access` is in scope. Regression coverage runs through the real Azure simulator SDK-test harness and verifies discovery, redirect, code redemption, ID-token claims, PKCE failure, single-use code behavior, refresh-token redemption, missing-scope rejection, hybrid-flow rejection, and unsupported-grant rejection.

Issue #363, the request to cut versioned releases and publish GHCR images, was intentionally deferred by user direction because the project is still early. Do not cut release tags or add artifact/image publishing work until that deferral is lifted.

The prior GCP/Azure security and load-balancer phase fixed issues #334 and #335. The realexec nftables ingress filter supports explicit accept/drop rules and clearing filters. GCP firewall ingress rules now compile by priority, target tags, source ranges, and source tags onto real instance NIC veth paths, and instance/tag/firewall mutations reapply the filters. Azure subnet/NIC NSGs now compile inbound rules by priority onto real NIC veth paths, preserve default same-VNet allowance, support core source prefixes/service tags, and clear filters when no NSG is attached.

GCP managed HTTP load balancing now has the unmanaged instance-group API slice needed by backend services, implements `backendServices.getHealth`, actively probes configured HTTP/TCP health checks, routes data-plane requests by forwarding-rule frontend IP through target HTTP proxy and URL map state, and proxies to healthy instance-group members. Azure Load Balancer now persists NIC backend-pool membership, routes frontend public-IP data-plane requests through load-balancing rules, resolves backend pools/probes from ARM state, actively probes backend NICs or backend-pool addresses, and proxies to healthy targets.

The CI log/architecture cleanup after PR #358 was completed. Cloud backend `ContainerInspect` and `ContainerList` paths no longer delegate into the core `BaseServer` local-state implementations. They resolve/list through cloud state explicitly, and cloud list failures now return provider errors instead of silently falling back to partial local/pending data. The cloud-backend isolation lint now scans all backend Go files, ignores comment-only matches, and fails future direct `BaseServer.ContainerInspect` / `BaseServer.ContainerList` use in cloud backends.

The same cleanup removed pass-green CI noise without suppressing warnings. The GCP host-dispatch allowlist test no longer emits a GitHub error annotation for its intentional source reference. Vite package builds run under Bun's runtime so Node 26's Tailwind `module.register()` deprecation does not appear, `ui-core` emits declaration artifacts for Turbo output tracking, and UI route tests start from `/ui/` so React Router no longer prints unmatched-route stderr.

Recently opened AWS simulator issues #359 and #360 were fixed in the same PR. EC2 EBS snapshots now settle pending rows deterministically on `DescribeSnapshots` and `CreateVolume(SnapshotId)`, so standard snapshot -> restore flows see `completed` through both filtered and unfiltered public reads. DynamoDB `DeleteItem` now honors `ReturnValues=ALL_OLD` by returning the pre-delete attributes when the item existed and no attributes when it did not. Both fixes shipped with real AWS SDK regression coverage.

The pre-push dependency freshness gate also found stale live-test AWS credential action pins. The live ECS and Lambda workflows now use `aws-actions/configure-aws-credentials@v6.2.0`, matching the latest published semantic tag at the time of the PR.

Issue #336 was fixed. The shared [simulators/realexec](simulators/realexec) network object creates a dedicated Linux network namespace per simulated cloud network/VPC implementation object, supports multiple subnet bridges inside that namespace, attaches veth NICs with lease-based private IPAM and unique MACs, creates routed egress links, and programs nftables SNAT for NAT egress. Public address allocation uses the shared real IPAM pool rather than store-length counters. The mandatory realexec host-network smoke test verifies bridge placement, guest-to-gateway and guest-to-guest reachability, routed egress reachability, SNAT programming, nftables cleanup, and namespace cleanup.

AWS EC2 VPC/subnet creation, ENIs created by `RunInstances` and Auto Scaling, Elastic IP allocation, NAT gateways, and NAT routes now use the substrate. GCP Compute networks/subnetworks, instance NIC allocation, regional addresses, and Cloud NAT now use the substrate. Azure virtual networks/subnets, NIC private IP/MAC allocation, public IP allocation, and NAT gateway subnet programming now use the substrate. These public API paths fail loudly when Linux network namespace, bridge/veth, route, or nftables capabilities are missing. BUG-1267 remains open for Firecracker-backed VM execution, nftables security-group/firewall/NSG enforcement, and managed load-balancer data planes.

Simulator Docker test targets run the test image with real networking privileges so Darwin/local Docker harnesses exercise the same Linux namespace/nftables paths instead of failing from container sandbox restrictions.

The CI fix for PR #358 also corrected the real public-IP and Linux-name paths exposed by the GCP Terraform HTTPS job. The shared public IPv4 allocator now has provider-shaped pools for AWS, GCP, and Azure instead of a documentation-only address block; GCP regional addresses and global forwarding rules draw from the real GCP pool. GCP real network/subnet/NIC names are derived from hashed cloud resource IDs within Linux's 15-character limit, avoiding collisions such as `sdk-nat-network` vs `cli-nat-network`. GCP real network, subnet, and NIC creation now keep the simulator registry and Linux substrate mutation in one critical section so Terraform parallelism cannot interleave duplicate fabric creation. The simulator Docker test targets keep root capabilities for real-network runs, so `CAP_NET_ADMIN` and `CAP_SYS_ADMIN` are present when the public API path requires them. Docker build contexts also exclude local caches, Terraform state, generated simulator binaries, and local agent metadata.

The same PR also advanced issues #334 and #335 for AWS. The substrate has a reusable nftables ingress-filter primitive on real NIC veth peers, and AWS EC2 security-group ingress rules now recompile onto attached real ENIs. AWS ELBv2 target health now performs real TCP/HTTP probes, and ELBv2 data-plane requests route by load-balancer DNS host to healthy targets without binding the listener port from the Query Protocol control plane.

The same control-plane/data-plane separation audit fixed Azure Event Grid topic endpoints. ARM create/get/list no longer allocate per-topic `127.0.0.1:<random>` listener URLs when ARM is reached through localhost. Topics advertise the shared host-dispatched Event Grid data-plane endpoint, or the configured Caddy HTTPS gateway template, and publish requests route through the Azure simulator by Event Grid Host header. Regression tests cover create/get/list endpoint shape and webhook publish through the advertised endpoint. GCP/Azure security policy and load-balancer proxy/probe migrations remain open, as does #333 Firecracker-backed public VM lifecycle.

The AWS/GCP Terraform HTTPS gateway examples were added, and CI kept Terraform provider validation on the Caddy HTTPS path where gateway fidelity matters.

The Azure Tables ARM PR fixed issue #356. The Azure simulator now implements Cosmos DB Tables ARM at `Microsoft.DocumentDB/databaseAccounts/{account}/tables/{table}` with CRUD/list and table throughput at `.../throughputSettings/default`, matching the official Azure REST spec and terraform-provider-azurerm's `azurerm_cosmosdb_table` path. It also implements Storage Tables ARM at `Microsoft.Storage/storageAccounts/{account}/tableServices/default/tables/{table}` with CRUD/list/update, matching `armstorage.TableClient`. ARM-created Cosmos and Storage tables project into the same Azure Tables data-plane store used by `/Tables` and entity operations; deletes remove table entities. The Tables data-plane create handler honors `Prefer: return-no-content`, and table ACL get/set is implemented for the Giovanni client path used by `azurerm_storage_table`. Coverage includes official Azure SDK tests (`armcosmos.TableResourcesClient`, `armstorage.TableClient`), Azure CLI `az rest` tests, and Terraform apply/destroy coverage for `azurerm_cosmosdb_table` and `azurerm_storage_table`.

The AWS simulator fidelity PR fixed issues #305-#308 and #317-#320. S3 `ListObjectsV2` now sorts and paginates keys, Lambda `FunctionConfiguration` responses no longer expose request-only `Code` or `Tags`, SNS confirmation-required subscriptions return `pending confirmation`, SQS rejects invalid receive batch sizes, EC2 honors run counts and filters with a pending-to-running state transition, ECR image digests are content-addressed, and KMS data keys use crypto-random key material.

The AWS Amplify fidelity PR fixed issues #330 and #331. Amplify `StopJob` and `DeleteJob` now used their distinct public REST paths and semantics, `DeleteJob` removed the job and its artifacts, and `ListArtifacts`, `GetArtifactUrl`, and `GenerateAccessLogs` were registered on the real AWS SDK paths with SDK and CLI coverage. No Terraform coverage was added for those operations because the official Terraform AWS provider exposes Amplify app/branch/webhook/domain/backend-environment resources, not job artifact or access-log operations.

The Azure ARM/DNS fidelity PR fixed issues #313, #314, and #340. ARM control-plane requests now required `api-version` and returned Azure's `InvalidApiVersionParameter` error when it was missing. Empty store-backed ARM list responses serialized as `{"value":[]}` instead of `{"value":null}`. Azure Private DNS implemented the public `GET .../privateDnsZones` list-by-resource-group route used by `armprivatedns.PrivateZonesClient.NewListByResourceGroupPager`, and virtual network links implemented `GET .../privateDnsZones/{zoneName}/virtualNetworkLinks`. The fixes shipped with real Azure SDK and Azure CLI coverage.

The GCP fidelity PR fixed issue #304 and issues #309-#311 and #321-#325. API Gateway, Cloud Build, IAM, and Pub/Sub stale client-surface rows now have real gcloud coverage, and API Gateway has `google-beta` Terraform coverage. Cloud Run and Cloud Functions list/LRO/timestamp wire shapes were corrected; Cloud Logging severity filters use Google severity ranks; GCS metadata and IAM policy responses match public client expectations; Cloud SQL backup operations return SQL Admin operation shapes; and Cloud DNS precondition failures return canonical `FAILED_PRECONDITION` details.

The Azure API-shape PR fixed issues #312, #315, and #326-#329. Storage Blob/File/Queue data-plane errors now return XML error envelopes with `x-ms-error-code`, Blob list XML responses include the public XML declaration, `EnumerationResults` attributes, and `NextMarker`, and Queue service properties support the Terraform provider availability probe. Service Bus admin missing entity reads return 404 Atom/XML errors. Event Grid publish validates JSON arrays and required Event Grid envelope fields before delivery. Redis, PostgreSQL Flexible Server, and Event Hubs namespace creates use Azure LRO headers plus in-progress states before converging to final states. Key Vault secret/key/certificate attributes include default recovery metadata.

The GCP timestamp reopen and AWS core-services PR fixed issue #310 and issues #343, #346, and #347. Eventarc, Firestore, and Pub/Sub now use the shared canonical protobuf timestamp formatter. AWS CloudTrail now supports trail CRUD, logging status, event selectors/tags, `LookupEvents`, real API-call recording, and gzipped S3 log delivery. AWS Auto Scaling now supports launch configurations and Auto Scaling Groups, and ASGs materialize/despawn EC2 instances on desired-capacity changes. AWS EBS now supports volume lifecycle, attach/detach/delete/modify, snapshot lifecycle with pending-to-completed state, snapshot restore, and real byte-level snapshot round trips through ECS managed EBS `volumeConfigurations` mounted into real task containers. S3 added `ListObjectVersions` so Terraform `force_destroy` can clean up CloudTrail-delivered objects.

The same PR fixed the CI flake exposed by the first PR run. GitHub-hosted linux/arm64 CPU jobs were being terminated while cold-downloading and compiling dependencies because `actions/setup-go` looked for a root `go.mod` and restored no cache in this multi-module workspace. Every Go setup step now uses explicit `go.work` and `**/go.sum` cache dependency paths. Non-Terraform explicit CI step and Go test timeouts are capped at five minutes; Terraform provider CI is capped at ten minutes for real provider apply/destroy work. Status-only aggregate jobs were removed, `needs` is no longer used in CI, and the workflow has 29 jobs. The initial over-serialization and nineteen-job lint fanout were corrected before merge: backend, FaaS smoke, e2e, simulator, Terraform, and smoke jobs fan out without phase-ordering gates, lint/backend/FaaS smoke work is grouped into fewer real-coverage shards, and AWS CLI simulator tests are sharded by service family with every existing CLI test selected exactly once. No tests were skipped, no mocks were added, and no fallback path was added.

The final CI pass also fixed GCP Cloud Logging's concurrent append race. Cloud Functions and Cloud Run container stdout/stderr are collected by separate real Docker log scanner goroutines; the GCP log store append path now serializes the read-modify-write cycle so stderr lines such as `Parsing expression:` cannot be overwritten by concurrent stdout appends. AWS CloudWatch already used atomic store updates, and Azure Monitor already serialized append operations.

All GitHub Actions workflows now auto-stop stale runs on new pushes to the same PR branch or ref. Each workflow has top-level concurrency keyed by workflow name plus PR source branch/ref with `cancel-in-progress: true`; the previous live AWS workflows' `cancel-in-progress: false` settings were removed.

AWS Terraform CI no longer fails the real provider suite after successful RDS restore work solely because the five-minute step budget was too small for Terraform provider apply/destroy coverage. Terraform provider CI uses a ten-minute cap; non-Terraform simulator and backend steps remain capped at five minutes.

The BUG-1267 real-execution stages landed the architecture/substrate contract, host capability checks, cleanup primitives, Firecracker smoke coverage, and the first public network/NAT migrations for issues #332-#336. `specs/SIMULATOR_EXECUTION.md` documents the current Docker/Podman-backed container/FaaS model, `specs/SIMULATOR_REAL_EXECUTION.md` defines the Firecracker/Linux-networking substrate contract, and `feedback_sim_host_model.md` records the allowed host execution paths. The AWS/GCP/Azure host-dispatch tests reference the explicit real-execution exception while continuing to reject broad host-process workload execution. The shared [simulators/realexec](simulators/realexec) module detects Linux, `/dev/kvm`, required host tools, and kernel capabilities; provides LIFO cleanup; allocates subnet and public IPv4 leases through real IPAM; and creates Linux network namespaces, subnet bridges, veth NICs, addresses, routes, routed egress, and nftables SNAT. CI has a mandatory `firecracker (microVM arithmetic)` job that installs a pinned official Firecracker release, requires `/dev/kvm`, boots a real guest, runs `go test`, `go build`, and multiple `eval-arithmetic` executions inside that microVM, then runs `make realexec-network-test` to create real host networking, verify packet reachability and SNAT, exercise nftables, and verify cleanup. Issue #336 was fixed; AWS security-group ingress and ELBv2 host-dispatched health/proxy paths became the first #335/#334 packet-path migrations; GCP/Azure #334/#335 packet paths were completed later; issues #332, #333, and #338 remain for VM execution and umbrella follow-through.

The same phase fixed aggregate Makefile regressions found during validation. `make test` now runs UI-bearing Go app unit tests with `-tags noui`, backend integration tests are gated behind an explicit `integration` build tag, integration CI/Make targets opt into `noui integration`, UI packages without package-level tests report that cleanly, and Go libraries support the top-level `build-noui` fanout through a compile-check alias.

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
- Caddy's local CA trust-store installation was disabled with `skip_install_trust`; tests and clients trust the exported CA file explicitly through knobs like `SSL_CERT_FILE`, so gateway startup stayed non-interactive while TLS verification remained enabled.
- `STACK_HTTPS=1` stack integration for local dev stacks.
- Admin UI topology card for gateway status, endpoints, CA path, and recovery commands.
- Azure Terraform tests started the simulator on HTTP loopback, started Caddy with per-test state and CA, used `metadata_host`/ARM endpoint through `https://azure.sockerless.localhost:<port>`, and passed the Caddy root CA through `SSL_CERT_FILE`.
- The shared simulator Docker test image included Caddy, installed from the official Caddy package repository.
- Azure Terraform CI installed Caddy on the runner for the direct `make terraform-test` path, the Azure Terraform harness failed loudly when Caddy or HTTPS was missing, and GCP arithmetic SDK coverage asserted the actual `"Result: 30"` Cloud Logging payload.
- SDK/CLI gateway guidance documented real client knobs for AWS CLI/SDKs, gcloud/Google clients, Azure CLI, and Azure SDKs without disabling TLS verification.
- BUG-1104 audit corrected stale `gcp-gcs` CLI coverage: `gcloud storage` now has real bucket/object lifecycle coverage, the simulator accepts current gcloud multipart upload boundaries, GCS `buckets.getStorageLayout` returns the public response shape, and GCS timestamps use Cloud Storage-style millisecond precision.
- AWS/GCP Terraform now had optional `make terraform-https-test` targets that started the simulator on HTTP loopback, put Caddy in front of it, set `SSL_CERT_FILE` to Caddy's local CA, and ran the real Terraform provider apply/destroy harness through the gateway's `https://localhost:<ephemeral-port>` single-simulator route. AWS covered the root production-shape Terraform stack plus the RDS and ElastiCache subpackages through the same HTTPS path. On macOS those targets delegated to the shared Linux simulator test image, matching Azure's CA-trust pattern.
- AWS Terraform Make targets built the real simulator binary once and passed it into every Terraform package. The package list still ran concurrently, but `go test -json` emitted package-qualified events so CI no longer sat silent while root, RDS, and ElastiCache provider flows were active.
- Terraform CI installed Caddy for the Terraform matrix and ran AWS/GCP via those HTTPS targets while Azure continued its mandatory gateway-backed harness.
- BUG-1253 corrected stale `gcp-vpcaccess` Terraform coverage: the GCP Terraform stack now used `vpc_access_custom_endpoint`, provisioned `google_vpc_access_connector`, asserted the canonical connector ID, and marked the matrix row direct.
- BUG-1254 / issue #304 was fixed: API Gateway, Cloud Build, IAM, and Pub/Sub public client surfaces now have real coverage where the CLI/provider exposes them.
- BUG-1263 was fixed: GCP API-shape issues #309-#311 and #321-#325 were corrected with SDK, CLI, and Terraform coverage where applicable.
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
- BUG-1267: issues #332, #333, and #338 track the real-execution compute/networking program. Issue #336's VPC/network/subnet/NIC/public-IP/NAT routing fabric landed on the real substrate, AWS/GCP/Azure firewall and load-balancer packet paths were migrated, and AWS/GCP/Azure VM lifecycle now boots and powers real Firecracker guests. BUG-1299 / issue #371 tracks the remaining guest metadata plane required before #333 can be fully closed. Number #337 is a merged PR, not an open issue.

## Recent Merged Work

- Azure Terraform HTTPS gateway stage: the Azure Terraform harness used the local Caddy gateway end to end, and BUG-1246 fixed Azure Storage data-plane host dispatch so `azure.sockerless.localhost` metadata requests were no longer swallowed by the storage wrapper.
- SDK/CLI HTTPS gateway audit: documented real CA/endpoint knobs for SDK and CLI clients, and fixed GCP GCS CLI coverage discovered by BUG-1104.
- Terraform HTTPS gateway audit: AWS/GCP got optional HTTPS provider harnesses and GCP VPC Access Terraform coverage; issue #304 was opened for larger GCP client-surface gaps.
- AWS simulator fidelity sweep: issues #305-#308 and #317-#320 were fixed with real AWS SDK coverage and targeted AWS CLI regression coverage.
- AWS Amplify fidelity sweep: issues #330 and #331 were fixed with real AWS SDK and AWS CLI coverage.
- Azure ARM/DNS fidelity sweep: issues #313, #314, and #340 were fixed with real Azure SDK and Azure CLI coverage.
- GCP fidelity sweep: issue #304 and issues #309-#311 and #321-#325 were fixed with real Google SDK, gcloud, and Terraform provider coverage where those public client surfaces exist.
- Azure API-shape and LRO sweep: issues #312, #315, and #326-#329 were fixed with real Azure SDK coverage and focused HTTP assertions where the SDK deliberately normalizes 404 data-plane reads.
- GCP/AWS core-services sweep: issue #310 and issues #341, #343, #346, and #347 were fixed. CloudTrail, Auto Scaling Groups, EBS lifecycle/snapshots, ECS managed EBS byte round trips, S3 version listing, and the reopened GCP timestamp call sites shipped with real SDK/CLI/Terraform coverage where public clients expose the surface.
- Real-execution substrate stages: the simulator execution docs, host-dispatch guardrails, mandatory Firecracker microVM arithmetic CI job, and issue #336 real network/NAT substrate migrations were completed.
- PR #299 / issue #298: Azure Redis CLI/Terraform coverage; GCP Memorystore Redis gcloud/Terraform coverage; GCP Cloud SQL `/v1` and `/sql/v1beta4` coverage; GCP Cloud DNS Changes and record-set patch routes.
- Local HTTPS gateway Stage 1: optional Caddy gateway, `.stack-pids` lifecycle integration, docs, and admin UI visibility.
- PR #296/#295/#291/#289 series: AWS Route 53 list fidelity, Lambda Terraform coverage, RDS/ElastiCache/API Gateway client-surface coverage, and Terraform minimum-wait documentation.
- Prior foundational simulator phases: object storage, queues, event systems, streams, managed data SaaS, DNS, VM/instance control planes, managed load balancers, NAT/public-IP, and VPC/networking parity across AWS/GCP/Azure.

Detailed history belongs in PR descriptions and `git log`; this file keeps only resume-critical state.
