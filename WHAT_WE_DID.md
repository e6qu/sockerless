# Sockerless - What We Built

Roadmap [PLAN.md](PLAN.md) - status [STATUS.md](STATUS.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md).

Detailed per-phase history lives in PR descriptions and `git log`. This file keeps only the last few phases and a compressed summary of completed foundations.

## 2026-06-05 — GCP Cloud KMS service (issue #419, PR pending)

New `simulators/gcp/cloudkms.go` brings GCP to key-management parity with AWS KMS and Azure Key Vault (BUG-1463). Implements the Cloud KMS v1 REST surface: keyRings (create with `keyRingId`, get, list; 404 on missing, 409 on duplicate), cryptoKeys (create with `cryptoKeyId`+`purpose=ENCRYPT_DECRYPT` auto-creating primary version 1, get, list, patch rotation via `updateMask`; 404 when the ring is absent), cryptoKeyVersions (list, get, destroy → `DESTROY_SCHEDULED` with a real `destroyTime`), and `cryptoKeys:encrypt`/`:decrypt`. Crypto is real, not faked — each version gets a random non-exportable AES-256 key and encrypt/decrypt use AES-256-GCM (the ciphertext is framed `version||nonce||sealed` so decrypt selects the version). Responses carry the CRC32C integrity fields `gcloud kms` verifies (`ciphertextCrc32c`, `verifiedPlaintextCrc32c`, `plaintextCrc32c`); incoming bytes accept both standard and URL-safe base64 (gcloud emits URL-safe — the first CLI run failed `illegal base64 data` until that was handled, exactly the issue's note). Coverage: SDK (`cloudkms_test.go` — keyring lifecycle, encrypt/decrypt round-trip with AAD enforcement + malformed-ciphertext rejection, version destroy), CLI (`gcloud kms` encrypt/decrypt file round-trip), Terraform (`fixtures/kms-lifecycle` — `google_kms_key_ring` + `google_kms_crypto_key` apply/destroy). Surface table `specs/SIM_SURFACE_TABLES/gcp-cloudkms.md` + coverage-matrix row added; routes use literal paths so `seed-surface-tables.sh` parses them.

## 2026-06-05 — AWS DynamoDB/ECS fidelity + azf attach-stdin race (PR #418, in-flight)

Closes #416 (DynamoDB GSIs dropped) and #417 (ECS Service family missing), plus ECS/DynamoDB audit follow-ups (BUG-1457–1460). While the AWS-sim work was committed, `test (azure backends)` flaked in CI: `TestAZFGitLabRunnerAttachStdin` hung to a `panic: test timed out after 5m0s` after `Post .../api/function: EOF`. Investigation (not a reorder): the azf `ContainerAttach` overlay path uses a **buffered** exec model (`attach_stream.go` — `Read` blocks until the invoke publishes), faithful to the cloud primitive (ACA/Azure-Functions expose request/response exec, not live bidirectional attach), so the spec-correct client order is write-stdin-before-dispatch. The real defect was in `ContainerStart`'s invoke goroutine: it POSTed the `/bin/sh` exec envelope to the Function App HTTP trigger before the in-container bootstrap bound its port, and the 600s invoke-client timeout stranded the attached reader past go-test's 5-min limit → opaque panic (BUG-1461). Fix: `waitAZFFunctionListening` — a 90s-bounded TCP-readiness probe before the POST, with a clear `function app not ready` fail-fast. Deliberately does **not** use the reverse-agent: `ws://host.docker.internal/...` is unreachable from inside the container under local Podman (confirmed: every reverse-agent path, e.g. `TestAZFContainerExec`, fails locally with the same dial-back error), so the buffered HTTP-invoke path is the one azf path that stays locally validatable — now passes in ~21s. The test was left unchanged; a reorder would have masked the backend bug.

A second flake then surfaced in `tf (aws)` (`TestStackProductionShape`): `Error: listing tags for CloudFront Function (...function/tf-fn/LIVE): ListTagsForResource ... 404 NoSuchResource`. The sim's CloudFront `tagging` handlers (`handleCFListTags`/`TagResource`/`UntagResource`) resolved only **distribution** ARNs (`cfDistributionIDFromARN` → `cfDistributions`); a function ARN missed → 404. The AWS provider tolerates that 404 on some read paths but not others, so it failed intermittently (identical sim code passed runs 26984053264/26985224721, failed 26987472514). Fix (BUG-1462): added `cfStoredFunction.Tags`, a `cfFunctionNameFromARN` resolver, and function-ARN branches to all three handlers, with shared order-preserving `cfMergeTags`/`cfDropTags`. SDK regression added to `TestCloudFrontFunctionLifecycle` (ListTags on a function ARN now 200; tag/untag round-trip).

## 2026-06-04 — Phase E+F: KV data-plane CLI tests, bleephub surface tables, hooks schema fix (PR #405)

Added Azure Key Vault data-plane CLI tests (`simulators/azure/cli-tests/keyvault_dataplane_test.go`) — secrets, keys, and certificates CRUD via `az rest` with explicit `Host` header routing to bypass DNS/TLS requirements. Updated coverage matrix `azure-kv-data-plane` CLI cell from `not applicable` to `direct`.

Created 12 `specs/SIM_SURFACE_TABLES/bleephub-*.md` files (actions, apps, checks, deployments, hooks, issues, orgs, pulls, releases, repos, teams, users) with full `HandleFunc` audit + coverage status per operation. Added 12 corresponding rows to `SIM_TEST_COVERAGE_MATRIX.md`.

Fixed three webhook schema gaps against GitHub's published REST API (BUG-1396–1398): `hookToJSON` now includes `url`, `test_url`, `ping_url`, `deliveries_url`, `last_response`; `deliveryToJSON` now includes `status` (human-readable string), `url` (target URL), `installation_id`, `repository_id`; `WebhookDelivery` carries `TargetURL` field. Added `deliveryStatus()` helper. Added `bleephub/gh_hooks_test.go` with 5 tests (`TestHooks_CRUD`, `TestHooks_Ping`, `TestHooks_Deliveries_Redeliver`, `TestHooks_NotFound`, `TestHooks_ValidationErrors`) covering full webhook lifecycle against the GitHub schema shape.

## 2026-06-03 — Phase D: error shape fidelity (PR #404)

Added negative-path SDK tests that assert on parsed error types rather than raw HTTP status across all three simulators. Fixed GCP Cloud Run service 404 handler URL (V1→V2), fixed Azure KV secret client missing `DisableChallengeResourceVerification: true`. BUG-1386–1395 filed and closed.

## 2026-06-02 — Phase C: pagination shape verification (PRs #402, #403)

Added token-based pagination to 12 AWS list endpoints and pagination support across GCP/Azure simulator list endpoints. SDK/CLI tests exhaust 2+ pages. BUG-1371–1385 filed and closed.

## 2026-06-03 — bleephub GET /api/v3/user/teams (PR #385, fixes issue #384)

`GET /api/v3/user/teams` was unregistered — requests returned 404. OIDC relying parties call this endpoint at sign-in to map team membership → roles. Added `ListTeamsByUser(userID int) []*Team` and `GetOrgByID(id int) *Org` to `store_orgs.go`, wired `handleListAuthUserTeams` in `gh_teams_rest.go`, and registered the route. `TestListUserTeams` covers the full org-create → team-create → add-member → GET /user/teams flow with `organization.login` assertion. BUG-1315 filed and closed.

## 2026-06-02 — AWS SDK/CLI Coverage Gaps (PR #383)

Added SDK and CLI tests for four AWS simulator operations that were implemented but had no public-client tests (BUG-1311–1314):

- **KMS `GetKeyPolicy`**: default policy round-trip + PutKeyPolicy→GetKeyPolicy verbatim.
- **Secrets Manager `GetResourcePolicy`**: ARN/Name shape; `ResourcePolicy` is nil (not empty string) when unset.
- **SSM `ListTagsForResource`**: empty TagList is `[]` not absent; AddTags→ListTags round-trip.
- **DynamoDB `DescribeTable`**: `BillingModeSummary`, `WarmThroughput.Status=ACTIVE`, `TableClassSummary`, `ProvisionedThroughput` all non-nil.

## 2026-06-02 — AWS EBS/VPC Control-Plane/Data-Plane Separation (PR #382, fixes issue #381)

Fixed two control-plane/data-plane coupling bugs in the AWS simulator:

- **ECS managed EBS volumes**: switched from bind-mounting paths from the sim's own filesystem to Docker named volumes (`sockerless-ebs-<volumeID>`). Sibling task containers always reach their volume data regardless of sim topology. Snapshot create/restore copies between Docker volumes via a short-lived Alpine container. Snapshot restore from EC2/Firecracker host-path snapshots into ECS Docker volumes falls back to host-path copy (only works in on-host topology).
- **CreateVpc / CreateSubnet**: store state unconditionally; attempt real Linux network-namespace fabric only when `DetectNetworkCapabilities().Require()` returns nil. NAT gateway and NAT route operations retain their gates (genuinely need real networking).

## Completed Foundations (compressed)

All of the following are fully implemented with SDK + CLI + Terraform coverage. See PR history and `git log` for detail.

**Simulator real-execution substrate** (`simulators/realexec`): Linux network namespaces per VPC, bridges/veth/TAP NICs, lease-based IPAM, nftables SNAT/DNAT/filtering, provider-shaped `169.254.169.254` metadata DNAT, Firecracker VM lifecycle (kernel download, rootfs, API socket, guest boot, reachability gate), active health probes, load-balancer proxying. Mandatory CI job: `firecracker (microVM arithmetic)` on `ubuntu-24.04` with `/dev/kvm`.

**AWS simulator**: EC2 (Firecracker VMs, EBS block-level with snapshot byte round-trips, VPC/networking, security groups, NAT, ELBv2 with real probes), ECS (managed EBS, Docker named volumes), Lambda, S3, RDS, ElastiCache, DynamoDB, KMS, SSM, Secrets Manager, SNS, SQS, CloudWatch, CloudTrail, Auto Scaling (real EC2 lifecycle on scale), ELBv2, Route 53, IAM (SLR + OIDC), ECR, EFS, API Gateway (v1+v2), Kinesis, EventBridge, Cloud Map, WAFv2, ACM, Amplify, CloudFront, STS.

**GCP simulator**: Compute Engine (Firecracker VMs, disks, networking, firewall rules compiling to nftables, NAT, managed HTTP load balancing with real probes), Cloud Run (services/jobs/executions), Cloud Functions Gen2, GCS, Pub/Sub, Cloud SQL, Memorystore Redis, BigQuery, Firestore, IAM (service accounts + project policy), Artifact Registry, Secret Manager, Cloud DNS, API Gateway, Cloud Build, Cloud Logging (severity ordering, concurrent log append), Eventarc, VPC Access.

**Azure simulator**: VMs (Firecracker-backed), Container Apps/Jobs (real Docker execution), Azure Functions, ACR (with cache rules), Key Vault (ARM + data-plane challenge auth), Storage (Blob/File/Queue/Table, ARM + data-plane, XML error envelopes, LRO), Service Bus (ARM + admin + data-plane), Event Hubs, Event Grid (host-dispatched data plane), Cosmos DB (SQL + Tables ARM + data-plane), Redis, PostgreSQL Flexible, Application Insights / Log Analytics (ARM; **SDK/CLI tests pending — Phase A**), Private DNS (zone + vnet links; **A-record SDK tests pending — Phase A**), DNS, Managed Identity (UAI), Entra OIDC (authorization-code + PKCE + refresh token + ID token), Networking (VNet/Subnet/NSG compiling to nftables/NIC/LB with real probes/NAT). Azure Terraform tests run behind local Caddy (required by AzureRM `metadata_host` HTTPS discovery).

**bleephub**: Full GitHub REST + GraphQL API simulator — orgs, repos, teams (`GET /user/teams` added PR #385), members, PRs, issues, actions/workflows/jobs/artifacts, GitHub Apps (JWT, OAuth, installations, user tokens), checks, deployments, releases, labels, reactions, Projects v2 GraphQL, Service Bus-style webhooks, OIDC, pagination, RBAC, OpenAPI conformance.

**Admin UI + CLI**: React 19/Vite/Tailwind 4 SPA at `/ui/`; `cmd/sockerless/` CLI with context config at `~/.sockerless/contexts/{name}/config.json`.
