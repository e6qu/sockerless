# Known Bugs

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - resume [DO_NEXT.md](DO_NEXT.md).

**1254 filed - 1253 fixed - 3 open - 2 false positives.**

Every CI failure, live-cloud failure, simulator fidelity gap, or discovered fake/fallback lands here before any fix attempt. Detailed closed-bug history lives in PR descriptions and `git log`.

## Open

| ID | Sev | Area | Pattern | One-liner |
|----|-----|------|---------|-----------|
| 1075 | P2 | live-cloud validation | unvalidated real cloud | Lambda is the only backend with a green live-cloud cell. Cloud Run Services, ACA Apps, AZF cloud-DNS, Lambda service-mesh, and ACA/AZF Azure AD remain unvalidated against authenticated real clouds. Do not mark these green without real cloud runs. |
| 1104 | P0 | simulator audit cadence | meta | Keep re-checking SDK/CLI/Terraform surface claims during simulator phases. This remains open while meaningful simulator work continues; stale "not applicable" rows are treated as real bugs when public clients exist. |
| 1254 | P1 | gcp simulator coverage | stale not-applicable rows | Issue #304 tracks larger BUG-1104 findings: public gcloud/Terraform client surfaces exist for GCP API Gateway, Cloud Build, IAM, and Pub/Sub rows still marked partly not applicable. |

## Recently Closed

Last phase closed BUG-1242, BUG-1243, BUG-1244, BUG-1245 / issue #298, BUG-1246, BUG-1247, BUG-1248, BUG-1249, BUG-1250, BUG-1251, BUG-1252, and BUG-1253:

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
