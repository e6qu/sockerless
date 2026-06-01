# Do Next

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Current State

- Branch: `main`, synced with `origin/main` after the issue #336 real network fabric PR merged.
- Active implementation branch: none.
- Open GitHub issues at last check: #332-#335 and #338. Issues #336 and #356 were fixed and closed. Number #337 is a merged PR, not an open issue.
- Open BUG trackers: BUG-1075, BUG-1104, and BUG-1267.
- Last completed work: issue #336 moved AWS/GCP/Azure VPC/network/subnet/NIC/public-IP/NAT routing paths onto the shared real-execution substrate; AWS security-group ingress and ELBv2 health/proxy paths became the first real #335/#334 packet-path migrations.

## Next Task

Continue the real-execution compute/networking track next: BUG-1267 / issues #332-#335 and #338, unless a higher-priority issue appears.

Next implementation should focus on Firecracker-backed VM execution, GCP/Azure nftables firewall/NSG enforcement, and GCP/Azure managed load-balancer proxy/listener data planes. The implementation must use the cloud's public API shape, create the real Linux/Firecracker host objects required by that API, and fail loudly when host capabilities are missing. It must not add fakes, metadata-only data planes, simulator-only public API knobs, or fallback execution paths.

## Provider Facts To Preserve

- AzureRM is the hard requirement. `metadata_host` is host-only and provider source constructs `https://<host>` for custom Azure metadata discovery.
- Azure Stack is also HTTPS-shaped for ARM/metadata use.
- AzAPI exposes full endpoint URLs and defaults to HTTPS Azure endpoints; it is configurable but should work through the same gateway.
- Google Terraform provider custom endpoints are full URLs; current HTTP simulator endpoint overrides are valid and should keep working.
- AWS Terraform provider custom endpoints are full URLs; official docs explicitly support `http://localhost` service endpoints. HTTPS is optional for realism and CA-bundle coverage.
- Existing simulator direct TLS support via `SIM_TLS_CERT` / `SIM_TLS_KEY` stays. The gateway is an operator/developer front door, not a replacement for direct simulator TLS.

## Completed Gateway Stage

- Caddy config plus `make stack-https-{up,status,ca,down}` targets.
- Caddy local-CA trust-store installation was disabled with `skip_install_trust`; provider tests trusted the exported CA file explicitly and kept TLS verification enabled.
- HTTPS routes to current simulator ports:
   - `aws.sockerless.localhost` -> `127.0.0.1:4566`
   - `gcp.sockerless.localhost` -> `127.0.0.1:4567`
   - `azure.sockerless.localhost` -> `127.0.0.1:4568`
   - Azure data-plane wildcards -> Azure simulator, preserving host-addressed routing.
- `STACK_HTTPS=1` local stack integration, including Azure ARM-advertised data-plane URL projection.
- Admin UI visibility for gateway status, endpoints, CA path, and equivalent recovery `make` commands.
- Azure Terraform tests through the gateway, including Caddy state isolation, CA trust, ARM metadata verification, Azure data-plane endpoint projection, and a 300-second test timeout.
- Shared simulator Docker test image with Caddy installed from the official package repository.
- BUG-1246 fixed Azure Storage data-plane middleware overmatching non-storage `*.localhost` hosts.
- SDK/CLI guidance documents real endpoint and CA knobs for AWS CLI/SDKs, gcloud/Google clients, Azure CLI, and Azure SDKs.
- BUG-1250/BUG-1251 fixed stale `gcp-gcs` CLI coverage: `gcloud storage` now has real bucket/object lifecycle coverage, current gcloud multipart uploads work, GCS `buckets.getStorageLayout` returns the public response shape, and GCS timestamps use Cloud Storage-style millisecond precision.
- AWS/GCP now had `make terraform-https-test` targets. They start the simulator on HTTP loopback, put Caddy in front of it, trust Caddy's CA through `SSL_CERT_FILE`, and run the real Terraform provider apply/destroy harness against the gateway's `https://localhost:<ephemeral-port>` single-simulator route. On macOS those targets run inside the shared Linux simulator test image so provider CA trust matches CI.
- Terraform CI installed Caddy for the Terraform matrix and ran AWS/GCP via the HTTPS gateway targets; Azure continued using its mandatory Caddy-backed Terraform harness.
- BUG-1253 fixed stale `gcp-vpcaccess` Terraform coverage by adding `google_vpc_access_connector` to the GCP Terraform stack and marking the matrix row direct.
- AWS simulator fidelity issues #305-#308 and #317-#320 were fixed:
   - S3 `ListObjectsV2` sorted keys, honored `start-after` / `continuation-token`, emitted `NextContinuationToken`, and returned delimiter `CommonPrefixes`.
   - Lambda `FunctionConfiguration` responses no longer leaked request `Code`, uploaded `ZipFile`, or `Tags`; `GetFunction` kept `Code` and `Tags` only as top-level response members.
   - SNS returned `pending confirmation` for confirmation-required protocols unless `ReturnSubscriptionArn=true`, and topic attributes counted confirmed vs pending subscriptions.
   - SQS rejected invalid `MaxNumberOfMessages` values with `InvalidParameterValue` instead of silently clamping them.
   - EC2 `RunInstances` honored `MinCount`/`MaxCount`, returned `pending` instances, transitioned them to `running`, and `DescribeInstances` applied supported filters while rejecting unsupported filter names.
   - ECR `PutImage` generated deterministic content-addressed `sha256:<64-hex>` digests from image manifests.
   - KMS `GenerateDataKey` returned fresh crypto-random plaintext key material and ciphertext that decrypted back to it.
- AWS Amplify issues #330 and #331 were fixed:
   - `StopJob` used the real `DELETE /apps/{appId}/branches/{branchName}/jobs/{jobId}/stop` route and cancelled the job.
   - `DeleteJob` used the real `DELETE /apps/{appId}/branches/{branchName}/jobs/{jobId}` route, removed the job, removed its artifacts, and made later `GetJob` calls return `NotFoundException`.
   - `ListArtifacts`, `GetArtifactUrl`, and `GenerateAccessLogs` were registered with their AWS SDK REST paths and covered through the real AWS SDK and AWS CLI.
- Azure ARM/DNS issues #313, #314, and #340 were fixed:
   - ARM control-plane requests required `api-version` and returned `InvalidApiVersionParameter` when omitted.
   - Empty store-backed ARM lists serialized `{"value":[]}` rather than `{"value":null}`.
   - Private DNS zones implemented list-by-resource-group, and Private DNS virtual network links implemented list-by-zone.
   - The routes were covered through real Azure SDK and Azure CLI tests.
- GCP issue #304 and issues #309-#311 and #321-#325 were fixed:
   - API Gateway, Cloud Build, IAM, and Pub/Sub stale public-client rows now have real gcloud coverage; API Gateway also has `google-beta` Terraform coverage.
   - Cloud Run/Functions list pagination, empty-list shape, LRO metadata types, and timestamps match public client expectations.
   - Logging severity ordering, GCS metadata/IAM, Cloud SQL backup operations, and DNS precondition errors were corrected.
- Azure issues #312, #315, and #326-#329 were fixed:
   - Blob/File/Queue storage data-plane errors return XML error envelopes with `x-ms-error-code`.
   - Blob list XML responses include the XML declaration, `EnumerationResults` attributes, and `NextMarker`; Queue service properties support the Terraform provider availability check.
   - Service Bus admin missing entity reads return 404 Atom/XML errors.
   - Event Grid publish validates the Event Grid event envelope before delivery.
   - Redis, PostgreSQL Flexible Server, and Event Hubs namespace creates use Azure LRO headers and converge from in-progress to final states.
   - Key Vault secret/key/certificate attributes include default recovery metadata.
- GCP issue #310 was fixed after its reopen:
   - Eventarc, Firestore, and Pub/Sub timestamp call sites now use the shared canonical protobuf timestamp formatter.
- AWS core issues #341, #343, #346, and #347 were fixed:
   - CloudTrail supports trail CRUD, logging status, event selectors/tags, `LookupEvents`, simulator API-call recording, and gzipped S3 log delivery.
   - Auto Scaling supports launch configurations and ASGs; desired-capacity changes create/terminate EC2 instances.
   - EBS supports volume lifecycle, attach/detach/delete/modify, snapshot lifecycle, pending-to-completed snapshots, restore from snapshot, and ECS managed EBS byte round trips through real task containers.
   - S3 `ListObjectVersions` supports Terraform cleanup of CloudTrail-delivered objects.
- CI linux/arm64 CPU runner flakiness was fixed:
   - Every `actions/setup-go` step now uses explicit `go.work` and `**/go.sum` cache dependency paths, because this repo has no root `go.mod`.
   - Backend, FaaS smoke, e2e, simulator, Terraform, and smoke jobs fan out without phase-ordering gates; the earlier broad arm64 serialization was removed after the cache fix and job-shard reduction addressed the actual pressure point.
   - Non-Terraform explicit CI step and Go test timeouts are capped at five minutes; Terraform provider CI is capped at ten minutes for real provider apply/destroy work.
   - Status-only aggregate jobs were removed; CI has no `needs` edges and runs 29 jobs.
   - Lint, backend integration, and FaaS smoke coverage is grouped into fewer real-coverage shards without disabling checks.
   - AWS CLI simulator coverage is sharded by service family, with every existing CLI test selected exactly once.
   - The fix did not skip tests, add mocks, add fallbacks, or weaken checks.
- GCP Cloud Logging container-output durability was fixed:
   - Cloud Functions and Cloud Run real container stdout/stderr scanners can append to the same Cloud Logging log name concurrently.
   - GCP Cloud Logging now serializes the append read-modify-write cycle, so stderr lines such as `Parsing expression:` are not overwritten by concurrent stdout appends.
   - AWS CloudWatch already used atomic store updates, and Azure Monitor already serialized append operations.
- GitHub Actions stale-run cancellation was fixed:
   - Every workflow now has top-level concurrency keyed by workflow name plus PR source branch/ref.
   - `cancel-in-progress: true` is set everywhere, including live-test and release/publish workflows, so new pushes stop older runs for the same branch/ref.
- The first BUG-1267 real-execution substrate stage landed:
   - `specs/SIMULATOR_EXECUTION.md` describes the current Docker/Podman-backed container/FaaS execution model plus the narrow VM-level real-execution exception.
   - `specs/SIMULATOR_REAL_EXECUTION.md` defines Firecracker guests, Linux network namespaces, bridges/tap/veth, netlink routing, nftables policy/NAT, load-balancer proxying, active health checks, per-instance metadata, capability checks, and loud failure semantics.
   - `feedback_sim_host_model.md` records the allowed host execution paths.
   - AWS/GCP/Azure host-dispatch tests point at the explicit substrate exception while still rejecting broad `os/exec` workload execution.
   - CI runs `make firecracker-test` in a mandatory `firecracker (microVM arithmetic)` job. That job installs pinned Firecracker v1.15.1, requires `/dev/kvm`, boots a real guest, and runs Go test/build plus `eval-arithmetic` executions inside the microVM.
- The second BUG-1267 substrate stage landed:
   - [simulators/realexec](simulators/realexec) provides deterministic capability detection, LIFO cleanup, an auditable host runner, lease-based IPv4 IPAM, and Linux bridge/netns/veth NIC creation.
   - `make realexec-network-test` creates real Linux networking artifacts, verifies gateway and namespace-to-namespace packet reachability, creates/removes an nftables table, and verifies cleanup removes the bridge and network namespaces.
- The third BUG-1267 substrate stage landed:
   - Each realexec network now owns a dedicated Linux network namespace.
   - The bridge and gateway address are created inside that namespace.
   - Attached NIC host-side veth peers move into the network namespace before joining the bridge; guest-side peers move into the workload namespace with their leased IP and default route.
   - The mandatory host-network smoke test verifies the bridge namespace placement plus real packet reachability and cleanup.
- The mandatory Firecracker CI job runs both the microVM arithmetic target and the real host-network target.
- Azure Tables ARM issue #356 was fixed:
   - Cosmos DB Tables support public ARM CRUD/list at `Microsoft.DocumentDB/databaseAccounts/{account}/tables/{table}` plus table throughput at `.../throughputSettings/default`.
   - Storage Tables support public ARM CRUD/list/update at `Microsoft.Storage/storageAccounts/{account}/tableServices/default/tables/{table}`.
   - ARM-created tables project into the real Azure Tables data-plane store, deletes remove entities, and data-plane table create honors `Prefer: return-no-content`.
   - Table ACL get/set is implemented for terraform-provider-azurerm's Giovanni client path.
   - Coverage includes official Azure SDK, Azure CLI `az rest`, and Terraform `azurerm_cosmosdb_table` / `azurerm_storage_table` apply/destroy tests.

## Remaining Stages

1. Attach one public VM family to Firecracker using the real network/IPAM/NIC substrate.
2. Add GCP/Azure nftables security enforcement on their packet paths.
3. Add the remaining real load-balancer proxying and health checks across AWS/GCP/Azure.
4. Repeat across AWS, GCP, and Azure without creating product-specific emulators.

## Deferred Trackers

- BUG-1075: live-cloud validation remains deferred by user direction. Do not mark cloud cells green without authenticated real-cloud runs.
- BUG-1104: audit-cadence meta tracker remains open. Every simulator phase should audit SDK/CLI/Terraform surface claims and file concrete BUGs before fixing.
- BUG-1267: issues #332-#335 and #338 track the remaining compute/networking real-execution program: Firecracker-backed VM instances, remaining nftables security enforcement, and real L4/L7 load balancing with health checks. Issue #336's VPC/network/subnet/NIC/public-IP/NAT routing fabric landed on the real substrate, and AWS security-group ingress plus ELBv2 health/proxying were the first #335/#334 packet-path migrations.

## Start Checklist

1. `git fetch origin`
2. `git checkout main`
3. `git pull origin main`
4. `gh issue list --state open --limit 30`
5. Create a fresh branch from `origin/main`.
6. File any concrete gaps in `BUGS.md` before code changes.

## Rules That Matter For This Task

- No simulator-specific public API changes.
- No mocks, fakes, or fallback modes.
- No interactive commands.
- Rebase the PR branch on `origin/main` before pushing.
- User merges PRs; never run `gh pr merge`.
