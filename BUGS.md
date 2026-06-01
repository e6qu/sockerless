# Known Bugs

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - resume [DO_NEXT.md](DO_NEXT.md).

**1286 filed - 1286 fixed - 3 open - 2 false positives.**

Every CI failure, live-cloud failure, simulator fidelity gap, or discovered fake/fallback lands here before any fix attempt. Detailed closed-bug history lives in PR descriptions and `git log`.

## Open

| ID | Sev | Area | Pattern | One-liner |
|----|-----|------|---------|-----------|
| 1075 | P2 | live-cloud validation | unvalidated real cloud | Lambda is the only backend with a green live-cloud cell. Cloud Run Services, ACA Apps, AZF cloud-DNS, Lambda service-mesh, and ACA/AZF Azure AD remain unvalidated against authenticated real clouds. Do not mark these green without real cloud runs. |
| 1104 | P0 | simulator audit cadence | meta | Keep re-checking SDK/CLI/Terraform surface claims during simulator phases. This remains open while meaningful simulator work continues; stale "not applicable" rows are treated as real bugs when public clients exist. |
| 1267 | P1 | cross-cloud simulator compute/networking | remaining real-execution data planes | Issues #332-#335 and #338 track the remaining real-execution program for Firecracker-backed VM execution, GCP/Azure firewall/NSG enforcement, and managed load balancers across AWS/GCP/Azure. Issue #336's VPC/network/subnet/route/NAT/IPAM fabric moved onto the substrate; AWS security-group ingress and ELBv2 health/proxy paths were the first #335/#334 migrations. |

## Recently Closed

This phase closed BUG-1285:

- BUG-1285 / issue #336: VPC/network/subnet/NIC/public-IP/NAT routing paths were metadata-only. The shared realexec substrate now supports per-network Linux namespaces, multiple subnet bridges, veth NIC attachment with real IPAM and unique MACs, routed egress links, public IPv4 IPAM, and nftables SNAT. AWS EC2 VPC/subnet, `RunInstances`/Auto Scaling ENIs, Elastic IPs, NAT gateways, and NAT routes use the substrate. GCP Compute networks/subnetworks, instance NICs, regional addresses, and Cloud NAT use it. Azure virtual networks/subnets, NIC private IP/MAC allocation, public IPs, and NAT gateway subnet programming use it. These paths fail loudly when Linux namespace/bridge/veth/route/nftables capabilities are missing.
- BUG-1286 / issues #334 and #335 partial: AWS ELBv2 and AWS security groups still had fabricated packet-path behavior. AWS ELBv2 target health now probes targets over real TCP/HTTP, ELBv2 listeners start a real local TCP proxy to healthy targets, and EC2 security-group ingress rules compile to nftables on attached real ENI veth peers. The remaining GCP/Azure security and cross-cloud load-balancer data-plane work stays under BUG-1267.

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
