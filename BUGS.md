# Known Bugs

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - resume [DO_NEXT.md](DO_NEXT.md).

**1305 filed - 1305 fixed - 4 open - 3 false positives.**

Every CI failure, live-cloud failure, simulator fidelity gap, or discovered fake/fallback lands here before any fix attempt. Detailed closed-bug history lives in PR descriptions and `git log`.

## Open

| ID | Sev | Area | Pattern | One-liner |
|----|-----|------|---------|-----------|
| 1075 | P2 | live-cloud validation | unvalidated real cloud | Lambda is the only backend with a green live-cloud cell. Cloud Run Services, ACA Apps, AZF cloud-DNS, Lambda service-mesh, and ACA/AZF Azure AD remain unvalidated against authenticated real clouds. Do not mark these green without real cloud runs. |
| 1104 | P0 | simulator audit cadence | meta | Keep re-checking SDK/CLI/Terraform surface claims during simulator phases. This remains open while meaningful simulator work continues; stale "not applicable" rows are treated as real bugs when public clients exist. |
| 1267 | P1 | cross-cloud simulator compute/networking | remaining real-execution data planes | Issues #332, #333, and #338 track the remaining real-execution program after #334/#335 were fixed: guest metadata and umbrella follow-through across the real-execution substrate. |
| 1299 | P1 | simulator VM metadata | missing guest metadata plane | Firecracker-backed VM lifecycle now boots real AWS EC2, GCP Compute Engine, and Azure VM guests, but guest-internal metadata service reachability for 169.254.169.254 / provider metadata hostnames still needs a real network-namespace data-plane implementation before #333 can be fully closed. |

## Recently Closed

This phase closed BUG-1300 through BUG-1305:

- BUG-1300: Moving VM-backed simulator SDK/CLI/Terraform CI jobs to KVM-capable x86_64 hosted runners exposed stale simulator workload execution paths that still forced Docker platform `linux/arm64`. GCP Cloud Functions/Cloud Run Jobs, AWS ECS, and Azure Functions now inspect the resolved local workload image platform before container execution. AWS Lambda translates the public Lambda `Architectures` field (`x86_64` / `arm64`) to Docker's platform strings. Simulator tests still build native Linux workload images for the runner platform, so x64 KVM CI does not require emulation and arm hosts still run arm64 images.
- BUG-1301: Firecracker VM rootfs image creation used `os.Truncate` as if it created `rootfs.ext4`, but Go's truncate call fails when the file does not already exist. Deterministic old workdirs could mask the bug when a previous image file was present; unique launch workdirs exposed it reliably as `truncate .../rootfs.ext4: no such file or directory`. The default workdir then had to be shortened because long cloud resource IDs could exceed Firecracker's Unix socket path budget. The official Firecracker CI S3 listing also includes `vmlinux-*.config` sidecars, and the old sorter could select a config file instead of the ELF kernel. Rootfs image creation now opens the image with create/truncate flags before sizing it, default Firecracker launches allocate a unique hash-prefixed workdir under the common temp root, kernel asset selection rejects config/debug sidecars, and cached/downloaded kernels are validated as ELF images before the launcher records or reuses them. Explicit caller-provided workdirs remain explicit and isolated by the caller.
- BUG-1302: Simulator VM boot used the Firecracker guest MAC on the host-side TAP endpoint and relied on distro network config/kernel `ip=` handling for guest addressing. The direct arithmetic Firecracker smoke passed because it configured the guest NIC after boot, while VM-backed simulator CI timed out waiting for provider-private guest reachability. TAP host endpoints no longer reuse the guest MAC, and Firecracker rootfs setup installs a deterministic boot-time network configurator for the assigned cloud-private IP, gateway, and resolver.
- BUG-1303 / issue #374: Firecracker VM rootfs images were fixed-size 3 GiB sparse files, which risked runner disk exhaustion when a single simulator job booted multiple VMs. Rootfs image sizing now uses `du -sk` on the copied real rootfs, doubles measured payload size, adds bounded headroom, aligns the result, and keeps a 1 GiB floor.
- BUG-1304 / issue #375: Firecracker kernel/rootfs assets were downloaded and unsquashed inside each fresh Actions run while VM-backed jobs used a floating hosted runner label. The workflow now pins VM-backed Firecracker jobs to `ubuntu-24.04` x86_64 and caches `~/.cache/sockerless/firecracker-ci` by Firecracker version plus runner architecture.
- BUG-1305 / issue #376: The official Firecracker CI rootfs preconfigures MAC-derived guest networking through `fcnet-setup.sh`, which conflicted with AWS/GCP/Azure simulator VM paths that assign provider-private `10.x` cloud IPs. Firecracker rootfs preparation now masks/removes the stock `fcnet` systemd/SysV/script/interface paths before building the ext4 image, writes deterministic systemd/netplan/ifupdown/resolver config for the simulator-assigned IP/gateway, and logs real VM boot failures with the Firecracker failure tail before returning provider-shaped API errors.

Issue #373 was clarified as a false-positive wording issue, not a missing runtime check: `DetectFirecrackerCapabilities()` already includes `firecracker`, and the shared capability detector opens `/dev/kvm` read-write for required Firecracker/jailer command sets. Regression coverage now locks that KVM-required command contract.

Previous phase closed BUG-1297 and BUG-1298:

- BUG-1297 / issue #366: Simulator release-image builds and `simulators/docker-compose.yml` used per-cloud build contexts even though each per-cloud module replaces `github.com/sockerless/simulator-realexec => ../realexec`. The simulator Dockerfiles now build from the shared `simulators/` context and switch into `/src/<cloud>` before `go build`; publish-container-images, compose, and per-cloud `docker-build` Make targets all use that same context. A simulator-scoped `.dockerignore` excludes test harnesses, generated test binaries, Terraform provider caches/state, built UI assets, and locally built simulator binaries from release-image contexts. Real Docker image builds for AWS, GCP, and Azure passed from the fixed context.
- BUG-1298 / issue #367: The explicit `SIM_RUNTIME=process` API-only startup mode existed but was undocumented, and Docker/Podman startup failures did not point operators to it. Common and per-cloud simulator docs now document `SIM_RUNTIME=process` as an explicit API-only mode for runs that do not invoke workload execution, not as a fallback. Startup comments and fatal messages now say Docker/Podman is required for workload execution and mention `SIM_RUNTIME=process` only for explicit non-execution API-only runs.

Previous phase closed BUG-1296:

- BUG-1296 / issue #365: Azure SDK local validation became portable on macOS without skipping tests or weakening the data plane. On Darwin, `make sdk-test` now delegates to the existing privileged Linux simulator test image and runs `make sdk-test-local` inside that container, so real-network SDK tests execute with Linux netns/veth/nftables capabilities instead of failing on macOS host limitations. Linux direct validation remains available through `make sdk-test-local`. The Azure SDK test harness also installs an explicit Event Grid data-plane resolver for the advertised `*.eventgrid.localhost:<simPort>` endpoint; it preserves the request URL and Host header for host-dispatched public data-plane routing while dialing loopback, and sets localhost wildcard names in `NO_PROXY` / `no_proxy`. Full macOS `make sdk-test` now passes through Docker.

Previous phase closed BUG-1295:

- BUG-1295 / issue #362: Azure Entra discovery advertised an `authorization_endpoint`, but the simulator did not serve `GET /{tenant}/oauth2/v2.0/authorize`, so public OIDC authorization-code clients dead-ended before login could complete. The Azure auth middleware now implements the authorization-code front half against the documented Microsoft Entra auth-code slice: absolute discovery endpoints for the served auth/token/JWKS routes, capability metadata for `code`, `query` / `fragment` / `form_post`, `authorization_code` / `client_credentials` / `refresh_token`, and `plain` / `S256`; required authorize parameters; short-lived authorization code issuance; state propagation; one-time code redemption at `/oauth2/v2.0/token`; PKCE validation; OAuth error bodies; unsupported grant rejection; access-token issuance; ID-token issuance when `openid` is in scope; and refresh-token issuance/redemption when `offline_access` is in scope. Regression coverage exercises the real simulator HTTP path through the Azure SDK test harness and verifies discovery, redirect, token exchange, ID-token claims, PKCE failure, single-use code behavior, refresh-token redemption, missing-scope rejection, hybrid-flow rejection, and unsupported-grant rejection.

Previous phase closed BUG-1293 and BUG-1294:

- BUG-1293 / issue #335: GCP firewall rules and Azure NSGs were still metadata-only on the real veth packet path. The shared realexec ingress filter now supports explicit accept/drop verdicts and clearing filters. GCP Compute firewall ingress rules compile by priority, target tags, source ranges, and source tags onto attached instance NICs with the implied deny-ingress rule enforced by nftables. Azure subnet/NIC NSGs compile inbound rules by priority, support scalar/list address and port fields plus core service tags, preserve default same-VNet inbound allowance, and remove nftables filters when no NSG is attached. Rule, tag, subnet association, NSG, and NIC mutations reapply filters to affected real NICs and fail loudly on substrate errors.
- BUG-1294 / issue #334: GCP and Azure managed load balancers were metadata-only after the AWS ELBv2 migration. GCP now implements the unmanaged instance-group API slice needed by backend services, `backendServices.getHealth`, health probing through configured HTTP/TCP health checks, host-dispatched forwarding-rule data-plane routing by frontend IP, URL-map/backend-service resolution, and proxying to healthy instance-group members. Azure Load Balancer now persists NIC backend-pool membership, dispatches data-plane requests by frontend public IP, resolves load-balancing rules/backend pools/probes from ARM state, probes backend NICs or backend-pool addresses, and proxies to healthy backends. Regression coverage uses real local HTTP listeners and official GCP SDK compile coverage for the new instance-group surface.

Previous phase closed BUG-1288 through BUG-1292:

- BUG-1288: Cloud backends still had direct `BaseServer.ContainerInspect` / `BaseServer.ContainerList` delegates, and the shared list path ignored `CloudState.ListContainers` errors. Cloud backend inspect/list paths now resolve through cloud state explicitly, provider list errors fail loudly, and `scripts/check-cloud-backend-isolation.sh` fails future direct inspect/list delegates across all cloud backend Go files.
- BUG-1289: CI logs still had warning/error-looking noise after PR #358 even when tests passed. The GCP host-dispatch allowlist test no longer emits a GitHub `##[error]` annotation for its intentional source reference, Vite package builds run under Bun's runtime instead of Node 26's deprecated `module.register()` path, `ui-core` emits declaration artifacts for Turbo output tracking, and UI router tests start from `/ui/` so they no longer print unmatched-route stderr.
- BUG-1290 / issue #359: AWS EC2 EBS snapshots could remain observably `pending` for standard snapshot restore flows. EC2 snapshots now persist an internal completion due time and settle pending snapshots on `DescribeSnapshots` and `CreateVolume(SnapshotId)`, so both filtered and unfiltered public reads expose `completed` before restore succeeds. AWS SDK coverage exercises CreateVolume -> CreateSnapshot -> filtered and unfiltered DescribeSnapshots -> CreateVolume(SnapshotId) without requiring VPC setup.
- BUG-1291 / issue #360: DynamoDB `DeleteItem` ignored `ReturnValues=ALL_OLD`. The handler now captures the pre-delete item and returns it as `Attributes` only when the item existed and `ReturnValues` is `ALL_OLD`; missing items still return no attributes. AWS SDK coverage verifies both existing and missing-item paths.
- BUG-1292: Pre-push dependency freshness failed because the live ECS and Lambda workflows pinned `aws-actions/configure-aws-credentials@v6.1.3` while the latest published semantic tag was `v6.2.0`. Both workflows now use `v6.2.0`.

Previous phase closed BUG-1285 through BUG-1287:

- BUG-1285 / issue #336: VPC/network/subnet/NIC/public-IP/NAT routing paths were metadata-only. The shared realexec substrate now supports per-network Linux namespaces, multiple subnet bridges, veth NIC attachment with real IPAM and unique MACs, routed egress links, public IPv4 IPAM, and nftables SNAT. AWS EC2 VPC/subnet, `RunInstances`/Auto Scaling ENIs, Elastic IPs, NAT gateways, and NAT routes use the substrate. GCP Compute networks/subnetworks, instance NICs, regional addresses, and Cloud NAT use it. Azure virtual networks/subnets, NIC private IP/MAC allocation, public IPs, and NAT gateway subnet programming use it. These paths fail loudly when Linux namespace/bridge/veth/route/nftables capabilities are missing.
- BUG-1286 / issues #334 and #335 partial: AWS ELBv2 and AWS security groups still had fabricated packet-path behavior. AWS ELBv2 target health now probes targets over real TCP/HTTP, ELBv2 data-plane requests route by load-balancer DNS host to healthy targets without control-plane listener-port binding, and EC2 security-group ingress rules compile to nftables on attached real ENI veth peers. Later BUG-1293/BUG-1294 completed the GCP/Azure #334/#335 packet paths.
- BUG-1287: Azure Event Grid topic ARM create/get/list leaked simulator-local data-plane plumbing by allocating per-topic `127.0.0.1:<random>` HTTP listeners when ARM was reached through localhost. Topics now always advertise the shared host-dispatched Event Grid data-plane endpoint, or the configured HTTPS gateway template, and publish requests route through the Azure simulator handler by Event Grid Host header. Regression tests verify create/get/list endpoint shape and publish fanout through the advertised host without per-topic local listener endpoints.

Previous phase closed BUG-1284:

- BUG-1284 / issue #356: Azure Cosmos DB Tables ARM and Azure Storage Tables ARM were missing. The Azure simulator now implements `Microsoft.DocumentDB/databaseAccounts/{account}/tables/{table}` CRUD/list plus table throughput at `.../throughputSettings/default`, and `Microsoft.Storage/storageAccounts/{account}/tableServices/default/tables/{table}` CRUD/list/update. ARM-created tables project into the real Tables data-plane store, deletes remove table entities, and the Storage Tables data plane honors the real `Prefer: return-no-content` create behavior and table ACL get/set calls used by terraform-provider-azurerm. The fix shipped with official Azure SDK coverage (`armcosmos.TableResourcesClient`, `armstorage.TableClient`), Azure CLI `az rest` coverage, and Terraform `azurerm_cosmosdb_table` / `azurerm_storage_table` apply/destroy coverage.

Previous phase closed BUG-1272 through BUG-1283:

- BUG-1272 / issue #310: Eventarc, Firestore, and Pub/Sub still emitted some protobuf `Timestamp` fields with `time.RFC3339Nano`, producing non-canonical fractional-second widths. Those call sites now use the shared canonical timestamp formatter and have SDK regression coverage.
- BUG-1273 / issue #343: AWS CloudTrail was missing. The AWS simulator now implements trail CRUD, logging status, event selectors/tags, `LookupEvents`, records simulator API calls, and delivers gzipped CloudTrail logs into S3 with SDK, CLI, and Terraform coverage.
- BUG-1274 / issue #346: AWS Auto Scaling Groups were missing. The AWS simulator now implements launch configurations, ASG lifecycle, desired-capacity updates, scaling activities, tags, and ASG-driven EC2 instance materialization with SDK, CLI, and Terraform coverage.
- BUG-1275 / issue #347: EBS was read-only metadata and ECS managed EBS could not prove snapshot data round trips. The AWS simulator now implements EBS volume/snapshot lifecycle, pending-to-completed snapshot state, restore from snapshot, and ECS managed EBS task mounts that write bytes, snapshot them, restore them, and read them back in a later task through real SDK and CLI flows.
- BUG-1276: Terraform cleanup could not enumerate CloudTrail-delivered objects through S3 version listing. S3 now implements `ListObjectVersions` with real object entries so provider `force_destroy` can clean up delivered logs.
- BUG-1277: GitHub Actions linux/arm64 CPU jobs were flaky because the repo has no root `go.mod`, so `actions/setup-go` restored no module cache and many hosted arm64 jobs cold-downloaded and compiled dependencies concurrently until runners were torn down mid-test. CI now gives every Go setup step an explicit `go.work`/`**/go.sum` cache key, without skipping tests or increasing timeouts.
- BUG-1278: CI still carried stale ten- and fifteen-minute command timeouts that could hide slow tests after the flake fix. Explicit CI step and Go test timeouts are now capped at five minutes.
- BUG-1279: Aggregate `lint` and `build-check` CI jobs exceeded the five-minute job budget even though the underlying work passed. CI now runs lint, build gates, amd64 builds, and arm64 builds as independent capped jobs; status-only aggregate jobs were removed so `needs` is not used for contexts that do not consume upstream outputs.
- BUG-1280: The first CI flake fix overcorrected by serializing hosted linux/arm64 CPU matrices and expanded lint into nineteen one-module jobs, producing a slow forty-plus-job PR run. CI now keeps the explicit Go cache and five-minute caps, removes broad `max-parallel: 1` serialization and phase-ordering `needs` gates from backend, FaaS smoke, e2e, simulator, Terraform, and smoke jobs, and groups lint/backend/FaaS smoke work into fewer real-coverage shards. No tests or jobs were disabled.
- BUG-1281: GCP Cloud Logging lost container log lines when stdout and stderr scanner goroutines appended to the same log name concurrently through a read-modify-write store sequence. Cloud Logging appends are now serialized so real container stdout/stderr lines are preserved; AWS already used atomic store updates and Azure already serialized monitor appends.
- BUG-1282: Several workflows either lacked concurrency cancellation or explicitly kept old runs alive, leaving stale pipelines queued/running after new PR branch pushes. Every workflow now has top-level concurrency keyed by workflow plus PR source branch/ref and `cancel-in-progress: true`, so new pushes auto-stop older runs for the same branch/ref.
- BUG-1283: PR CI still had two oversized real-test steps: AWS simulator CLI coverage bundled all 76 AWS CLI tests into one five-minute step and AWS Terraform provider coverage could exceed five minutes after real RDS restore work passed. AWS simulator coverage now keeps SDK coverage intact and shards the AWS CLI tests by service family, with a mechanical coverage check confirming every existing CLI test is selected exactly once. Terraform provider CI has a ten-minute cap, matching the allowed budget for real provider apply/destroy work. CI has 29 jobs and no `needs` edges.

Previous phase closed BUG-1270 and BUG-1271:

- BUG-1270: Top-level `make test` failed on UI-bearing Go apps and backend integration packages. The shared Go app `test` target now uses `-tags noui` when a UI package is configured, backend integration tests are build-tagged with `integration`, and integration CI/Make targets opt into `noui integration` explicitly.
- BUG-1271: Top-level fanout targets failed on leaf packages without the expected target shape. UI packages without a `test` script now report that explicitly instead of invoking `/bin/test`, and Go library Makefiles provide a `build-noui` compile-check alias for the aggregate no-UI build.

Last phase closed BUG-1264 / issues #312, #315, and #326-#329:

- BUG-1264 / issues #312 and #315: Azure Storage Blob/File/Queue data-plane errors now return XML `<Error>` envelopes with `Content-Type: application/xml` and `x-ms-error-code`; Blob list XML responses include the XML declaration, public `EnumerationResults` attributes, and `NextMarker`; Queue service properties return the public XML shape used by Terraform provider availability checks.
- Issue #326: Service Bus admin data-plane missing queue/topic/subscription/rule reads now return 404 Atom/XML errors instead of 200 empty feeds.
- Issue #327: Event Grid publish now rejects malformed JSON, empty batches, missing required Event Grid envelope fields, and invalid event times before webhook delivery.
- Issue #328: Redis, PostgreSQL Flexible Server, and Event Hubs namespace creates now use Azure LRO headers and in-progress states before converging to final public states.
- Issue #329: Key Vault secret/key/certificate attributes now include `recoveryLevel: Recoverable+Purgeable` and `recoverableDays: 90`.

Earlier recent phase closed BUG-1254 and BUG-1263 / issues #304, #309-#311, and #321-#325:

- BUG-1254 / issue #304: GCP API Gateway, Cloud Build, IAM, and Pub/Sub stale public-client rows were corrected. The simulator now has real gcloud coverage for those public CLI surfaces, and API Gateway has Terraform provider coverage through `google-beta`.
- BUG-1263 / issues #309-#311 and #321-#325: GCP Cloud Run and Cloud Functions list/LRO/timestamp wire shapes, Cloud Logging severity ordering, GCS metadata and IAM policy shape, Cloud SQL backup operation shapes, and Cloud DNS precondition error status were fixed with real SDK, CLI, and Terraform coverage where those public client surfaces exist.

Earlier recent phase closed BUG-1268 and BUG-1269 / issues #313, #314, and #340:

- BUG-1268 / issue #340: Azure Private DNS now implements `PrivateZonesClient.NewListByResourceGroupPager` at `GET .../privateDnsZones`, and virtual network links now implement the public list-by-zone endpoint. Both paths are covered by the real Azure SDK and Azure CLI.
- Issues #313 and #314: Azure ARM control-plane requests now reject missing `api-version` with `InvalidApiVersionParameter`, the dead unused AzureRouter validator was removed, and simulator list serialization returns `{"value":[]}` instead of `{"value":null}` for empty stores.
- BUG-1269: AWS Terraform HTTPS CI could hang waiting for Caddy because `tls internal` attempted to install the generated local CA into CI runner trust stores even though the harness already trusted the CA through `SSL_CERT_FILE`. The shared gateway Caddyfile now uses `skip_install_trust`; tests still verify TLS normally with the exported CA file, and Caddy startup no longer performs host trust-store mutation.

Earlier recent phase closed BUG-1265..BUG-1266 / issues #330-#331:

- BUG-1265 / issue #330: Amplify `StopJob` and `DeleteJob` now used distinct public REST paths. `StopJob` cancelled the job through `.../jobs/{jobId}/stop`; `DeleteJob` removed the job and its artifacts through `.../jobs/{jobId}`.
- BUG-1266 / issue #331: Amplify `ListArtifacts`, `GetArtifactUrl`, and `GenerateAccessLogs` were implemented on the AWS SDK REST paths and covered by real AWS SDK and AWS CLI tests. Terraform was not changed because the official AWS provider does not expose these job-artifact or access-log operations.

Earlier recent phase closed BUG-1255..BUG-1262 / issues #305-#308 and #317-#320:

- BUG-1255 / issue #305: S3 `ListObjectsV2` now returns lexicographically sorted keys, honors `start-after` and `continuation-token`, emits `NextContinuationToken`, and supports delimiter `CommonPrefixes`.
- BUG-1256 / issue #306: Lambda `FunctionConfiguration` responses no longer leak request-only `Code`, uploaded `ZipFile`, or `Tags`; `GetFunction` keeps `Code` and `Tags` as top-level members.
- BUG-1257 / issue #307: SNS returns `pending confirmation` for email/http/https-style protocols unless `ReturnSubscriptionArn=true`, and topic attributes distinguish confirmed vs pending subscriptions.
- BUG-1258 / issue #308: SQS `ReceiveMessage` rejects out-of-range `MaxNumberOfMessages` with `InvalidParameterValue` instead of silently clamping.
- BUG-1259 / issue #317: EC2 `DescribeInstances` applies supported filters and rejects unsupported filter names.
- BUG-1260 / issue #318: EC2 `RunInstances` honors `MinCount`/`MaxCount`, returns `pending`, and transitions instances to `running`.
- BUG-1261 / issue #319: ECR `PutImage` stores deterministic content-addressed SHA-256 manifest digests.
- BUG-1262 / issue #320: KMS `GenerateDataKey` returns fresh crypto-random plaintext key material and ciphertext that decrypts back to it.

Earlier recent phases closed BUG-1242, BUG-1243, BUG-1244, BUG-1245 / issue #298, BUG-1246, BUG-1247, BUG-1248, BUG-1249, BUG-1250, BUG-1251, BUG-1252, and BUG-1253:

- Azure Cache for Redis has Azure CLI and azurerm Terraform coverage.
- GCP Memorystore Redis has gcloud and terraform-provider-google coverage.
- GCP Cloud SQL exposes the `/v1` and `/sql/v1beta4` SQL Admin paths needed by SDK, gcloud, and Terraform.
- GCP Cloud DNS implements public Changes.Create/Get/List and ResourceRecordSets.Get/Patch with SDK, gcloud, and Terraform coverage.
- BUG-1246: Azure Storage data-plane middleware overmatched non-storage `*.localhost` hosts and swallowed `azure.sockerless.localhost` metadata requests. It now dispatches only real Azure Storage service labels: `blob`, `file`, `queue`, `table`, `web`, and `dfs`.
- BUG-1247: Azure Terraform CI ran the gateway-backed harness directly on the runner without installing Caddy. The Azure Terraform CI job now installs the real Caddy binary before `make terraform-test`.
- BUG-1248: GCP Cloud Run arithmetic SDK coverage asserted an exact `"30"` log entry, but the workload logs the real output line as `"Result: 30"`. The assertion now checks the joined Cloud Logging payloads for the actual result line.
- BUG-1249: Azure Terraform HTTPS coverage could fail late if Caddy was missing or if a future edit accidentally changed the provider endpoint away from HTTPS. The harness now preflights the Caddy executable and fails loudly unless the Terraform endpoint is HTTPS.
- BUG-1250: BUG-1104 audit found stale `gcp-gcs` CLI coverage marked not applicable even though current gcloud supports Cloud Storage endpoint overrides. The simulator now has real `gcloud storage` bucket/object lifecycle coverage, accepts the current CLI multipart boundary form, and implements the public `buckets.getStorageLayout` probe.
- BUG-1251: GCS object and bucket timestamps used RFC3339 nanosecond precision, which caused current Linux gcloud Storage to emit timestamp truncation warnings into command output. GCS timestamps now use Cloud Storage-style millisecond precision.
- BUG-1252: AWS/GCP Terraform had direct HTTP simulator harnesses even though the optional Caddy gateway existed. AWS/GCP now have `make terraform-https-test` targets that run the real providers through Caddy HTTPS with CA trust while preserving direct HTTP `make terraform-test`; AWS covers both the root stack and the RDS/ElastiCache Terraform subpackages through the HTTPS path.
- BUG-1253: The `gcp-vpcaccess` Terraform matrix row was marked not applicable even though terraform-provider-google exposes `google_vpc_access_connector`. The GCP Terraform stack now provisions a real connector through `vpc_access_custom_endpoint`, asserts its canonical ID, and marks the matrix row direct.

Older closed bugs are intentionally not repeated here. Use PR descriptions and `git log` for exact fix details.

## False Positives

| Area | Finding | Why it is not a bug |
|------|---------|---------------------|
| `backends/aca/azure.go::fakeCredential` | Returns literal `"fake-token"` against simulator endpoints. | Simulator auth does not validate bearer tokens. Production clients use `azidentity.NewDefaultAzureCredential`; this credential is only for simulator endpoint clients. |
| `cmd/sockerless-admin/api_observability.go::envOrDefault` | Returns canonical OTel resource-attribute name when unset. | This is a documented default-value helper, not an error-hiding fallback. |

## Class-of-Bug Rules

- No stubs, fakes, mocks, synthetic data, silent fallbacks, or degraded modes.
- Public simulator APIs must match real cloud APIs; local gateway/admin plumbing must not leak into those APIs.
- Backend host primitive must match the cloud service: ECS in ECS, Lambda in Lambda, Cloud Run in Cloud Run, GCF in Cloud Run Functions, ACA in ACA, AZF in Azure Functions.
- External test fixtures use real clients: official SDKs, vendor CLIs, Terraform providers, and `gh` for bleephub.
- Closed enumeration means full-table audit before claiming fixed.
- Reopens require a postmortem: what test passed but should have failed, what client path was missed, and what new canonical-client test catches it.
- List operations need paged-iterator tests.
- Stateful resources need state-machine assertions.
- Mux pattern overlap is a recurring simulator bug class; run the overlap scanner when adding routes.
