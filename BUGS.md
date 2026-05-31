# Known Bugs

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - resume [DO_NEXT.md](DO_NEXT.md).

**1269 filed - 1268 fixed - 4 open - 2 false positives.**

Every CI failure, live-cloud failure, simulator fidelity gap, or discovered fake/fallback lands here before any fix attempt. Detailed closed-bug history lives in PR descriptions and `git log`.

## Open

| ID | Sev | Area | Pattern | One-liner |
|----|-----|------|---------|-----------|
| 1075 | P2 | live-cloud validation | unvalidated real cloud | Lambda is the only backend with a green live-cloud cell. Cloud Run Services, ACA Apps, AZF cloud-DNS, Lambda service-mesh, and ACA/AZF Azure AD remain unvalidated against authenticated real clouds. Do not mark these green without real cloud runs. |
| 1104 | P0 | simulator audit cadence | meta | Keep re-checking SDK/CLI/Terraform surface claims during simulator phases. This remains open while meaningful simulator work continues; stale "not applicable" rows are treated as real bugs when public clients exist. |
| 1264 | P1 | azure simulator fidelity | public API shape / LRO | Issues #312, #315, and #326-#329 track Azure Storage XML errors/listing shape, Service Bus missing entity 404s, Event Grid validation, Redis/Postgres/EventHub LRO create semantics, and Key Vault recovery attributes. ARM api-version validation and empty ARM list shapes were fixed. |
| 1267 | P1 | cross-cloud simulator compute/networking | metadata-only data plane | Issues #332-#336 track the real-execution program for VM instances, VPC/network/subnet/route/NAT/IPAM fabric, security-group/firewall/NSG enforcement, and managed load balancers across AWS/GCP/Azure. |

## Recently Closed

Last phase closed BUG-1254 and BUG-1263 / issues #304, #309-#311, and #321-#325:

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
- BUG-1252: AWS/GCP Terraform had only direct HTTP simulator harnesses even though the optional Caddy gateway existed. AWS/GCP now have `make terraform-https-test` targets that run the real providers through Caddy HTTPS with CA trust while preserving direct HTTP `make terraform-test`.
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
