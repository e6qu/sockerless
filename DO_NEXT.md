# Do Next

Status [STATUS.md](STATUS.md) · roadmap [PLAN.md](PLAN.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · vibe catalogue [docs/VIBE_CODING.md](docs/VIBE_CODING.md).

## Where we are

The simulator Docker test harness phase closed the local verification blockers from PR #252. `make docker-test` now exists at the top level and per simulator, builds a real shared Linux test image, and runs the existing SDK/CLI/Terraform test categories from the repository root. Azure Terraform tests on macOS now delegate direct `go test` execution into that Linux image so the real Terraform providers can validate the generated simulator CA via `SSL_CERT_FILE`.

PR #252 implemented the first follow-up from PR #246's foundational simulator service audit. The audit lives in `specs/SIM_FOUNDATIONAL_AUDIT.md` and covers object storage, managed data stores, DNS, queues, event routing, stream/event ingestion, VM/EC2-like compute, VPC/networking, NAT/egress, gateways, and managed load balancers across AWS/GCP/Azure.

Event routing is now present across the three sims for foundational flows: AWS EventBridge rule/target/event delivery, GCP Eventarc trigger lifecycle, and Azure Event Grid topic/subscription/publish flows. Coverage uses official SDKs, vendor CLIs, and Terraform provider resources. BUG-1197..1199 are closed.

The advanced/sibling event-service parity phase closed BUG-1213/#249, BUG-1214/#250, and BUG-1215/#251. AWS EventBridge now has event-bus lifecycle, bus policy permissions, archives, and replays. Azure Event Grid now has domains, domain topics, system topics, partner topics, and event subscriptions on those scopes. GCP Eventarc now has channels, provider discovery, and channel connections. Coverage uses the official SDKs where the current modules expose clients, vendor CLIs, and Terraform provider resources where the providers expose them; the GCP provider schema was checked and does not expose an Eventarc channel-connection resource.

The Azure host-addressed local DNS portability issue is now closed. The Azure simulator has an opt-in DNS listener for local simulator zones, so SDK/CLI/Terraform clients can keep Azure-shaped host-addressed data-plane endpoints and resolve them through normal DNS instead of URL rewrites or Host-header injection.

The stream/event ingestion gap is now closed. AWS Kinesis is implemented as a real JSON-protocol slice with stream lifecycle, shard listing, records, shard iterators, tags, retention, monitoring, encryption state, shard-count update, and limits. Azure Event Hubs is implemented with ARM namespace/event hub/consumer group/auth-rule lifecycle plus AMQP send/receive over the raw AMQP/TLS listener.

The managed data SaaS gap is now closed: GCP BigQuery, GCP Firestore, Azure Cosmos DB, and the AWS DynamoDB query/filter audit landed with SDK/CLI/Terraform coverage. The follow-up Azure azurerm storage endpoint report is also closed: the exact `azurerm_storage_container` + `storage_account_name` path now runs in the Azure Terraform harness and verifies the `{account}.blob.{suffix}` endpoint shape plus matching ARM resource ID.

The simulator test-contract matrix backfill is now closed. `specs/SIM_TEST_COVERAGE_MATRIX.md` has one row per canonical simulator surface table and CI/pre-commit enforcement through `scripts/check-simulator-coverage-matrix.sh`. The reported concrete holes were fixed with real external clients: AWS DynamoDB/SQS/SNS have direct AWS CLI lifecycle coverage, and the GCP Terraform harness covers Cloud Functions v2, Cloud Build triggers, Pub/Sub topics/subscriptions, and Cloud Logging sinks/metrics against simulator routes implemented for the provider call sequence.

The Azure Entra token-resource gap is now closed. The simulator token endpoint derives RS256 JWT `aud` from OAuth v2 `scope` and OAuth v1 `resource`, so ARM keeps the management audience while Key Vault, Service Bus, and Storage data-plane clients can receive the resource-specific audiences their SDKs request.

The VM/instance compute gap is now closed. AWS EC2, GCP Compute Engine, and Azure Virtual Machines expose their public control-plane lifecycle APIs through the simulators with official SDK, vendor CLI, and Terraform coverage.

The managed load-balancer gap is now closed. AWS ELBv2, GCP global external HTTP load balancing, and Azure Load Balancer expose their public control-plane APIs through the simulators with official SDK, vendor CLI, and Terraform coverage.

The NAT/public-IP parity gap is now closed. AWS EC2 Elastic IP + NAT Gateway + route-table flows, GCP regional addresses + manual Cloud NAT, and Azure Public IP Prefix + NAT Gateway + subnet NAT association flows are implemented through public cloud API surfaces and covered by official SDKs, vendor CLIs, and Terraform providers. BUG-1206 stays open as the remaining broad surface-table audit-debt tracker.

The Azure Service Bus ARM Terraform parity issue is now closed. `Microsoft.ServiceBus/namespaces/{name}/networkRuleSets/default` supports get/update, network rule sets can be listed, disaster recovery and migration configuration lists return empty Azure-shaped list results when no config exists, and absent aliases/configurations return Azure-shaped 404s. The Azure Terraform harness now creates `azurerm_servicebus_namespace` plus `azurerm_servicebus_queue`, with matching official SDK and Azure CLI coverage.

## Stage plan

Current phase: idle after NAT/public-IP simulator parity for issue #279. The next implementation pass should move to BUG-1206's broad surface-table audit cleanup unless a new community issue arrives first.

Issue #279 finding: the audit gap was real. AWS had EC2 NAT primitives but lacked explicit SDK/CLI/Terraform coverage for the EIP/NAT route path. GCP lacked the regional address and address-label public APIs that manual Cloud NAT and the Terraform provider use. Azure lacked Public IP Prefix resources and NAT Gateway/subnet association list/back-reference behavior. The fix added those public API routes and pinned them with official SDK, vendor CLI, and Terraform coverage across all three simulators.

Issue #263 finding: the audit gap was real. API Gateway, APIM, CloudFront, and existing network primitives did not cover managed L4/L7 load-balancer APIs. The fix added AWS ELBv2 load balancer/target group/listener lifecycle and target-health operations; GCP Compute global health checks, backend services, URL maps, target HTTP proxies, and global forwarding rules; and Azure `Microsoft.Network/loadBalancers` with public IP frontends, backend pools, probes, and load-balancing rules. Each cloud has official SDK, vendor CLI, and Terraform coverage.

Issue #276 finding: the report was real. The azurerm Service Bus namespace resource reads `networkRuleSets/default` immediately after namespace creation, and the simulator had not implemented that public ARM child resource. The fix added persisted network rule set get/list/update, empty disaster recovery and migration configuration list reads, and Azure-shaped 404s for absent configs, then pinned the path with official `armservicebus` SDK, Azure CLI `az rest`, and azurerm Terraform namespace+queue coverage.

Issue #266 finding: the audit gap was real. AWS EC2 exposed VPC/subnet/security-group/NAT helpers but not EC2 instance lifecycle; GCP Compute exposed networks/subnets/firewalls/routers/NAT/disks/zones but not instances; Azure exposed Network resources but not `Microsoft.Compute/virtualMachines` and the NIC/public-IP wiring VM resources require. The fix added public-cloud-compatible control-plane VM slices for all three clouds with SDK/CLI/Terraform coverage. No Firecracker or local execution substrate is exposed through public simulator APIs.

Issue #272 finding: the report was real. The Azure simulator minted RS256/JWKS-verifiable tokens, but every token had `aud=https://management.azure.com/`, even when real Azure clients requested data-plane audiences through OAuth v2 `scope` such as `https://vault.azure.net/.default` or OAuth v1 `resource` such as `https://servicebus.azure.net`. The fix derives the JWT audience from those public token-request fields, keeps the ARM default for omitted audience fields, and covers the behavior with unit tests plus simulator SDK-harness HTTP tests.

Issue #264 finding: the report was real. The repository had SDK/CLI/Terraform jobs, but no maintained matrix tying those jobs to the canonical simulator surface tables. The concrete examples were also real: AWS DynamoDB/SQS/SNS needed direct AWS CLI lifecycle coverage, and the GCP Terraform harness still missed Cloud Functions v2, Cloud Build triggers, Pub/Sub topic/subscription resources, and Cloud Logging sink/metric resources. The fix added those real-client tests, the missing GCP Cloud Logging and Cloud Build provider routes, and a CI/pre-commit matrix check.

Issue #269 finding: the reported endpoint requirement made sense, and the simulator implementation on `main` already supported templated storage endpoints through `SIM_AZURE_ARM_EXTERNAL_DATA_PLANE_URLS_JSON` plus matching `/metadata/endpoints` storage suffix derivation. The missing piece was exact Terraform provider coverage for the realistic data-plane resource path. The fix added `azurerm_storage_container` with `storage_account_name`, not `storage_account_id`, so the provider must parse `primary_blob_endpoint`, accept the `{account}.blob.{suffix}` shape, and issue Blob data-plane calls.

Issue #267 finding: the foundational data SaaS gap was real. The fix added GCP BigQuery dataset/table/job/query/tabledata flows, GCP Firestore document CRUD/commit/batch/query flows, and Azure Cosmos DB ARM plus SQL data-plane flows. The AWS DynamoDB audit found Query/filtered Scan returned unfiltered table contents; that now honors equality expressions using `ExpressionAttributeNames` and `ExpressionAttributeValues`.

Issue #243 finding: Azure ARM resource responses were inconsistent with the simulator's cloud-facing contract. Storage and Key Vault derived endpoint hosts from the incoming ARM request, but Service Bus, Redis, APIM, PostgreSQL Flexible Server, and Container Apps still returned production cloud suffixes. The fix derives those endpoint fields from the simulator request host while preserving Azure-shaped field names and host patterns; Service Bus listKeys connection strings were updated with the same host derivation.

Issue #244 finding: Container Apps Jobs and Apps passed `Architecture: "linux/arm64"` to Docker for every started container, including sidecars. That made the real container start fail on amd64 hosts when the local image manifest was amd64. The fix resolves each image and inspects its local manifest platform before calling `StartContainerSync`, matching the Cloud Run Services pattern.

Issue #239 finding: PR #238 made GCS object metadata durable and observable but did not validate accepted metadata fields. The fix implements validation where the public docs are explicit: `customTime` must parse as RFC 3339 and `contentLanguage` must be at most 100 characters. Invalid metadata now returns `400 INVALID_ARGUMENT` across multipart upload, resumable upload init/finalization, compose, `copyTo`, and `rewriteTo`.

Issue #240 finding: `gcsObjectResource.applyTo` and `persistGCSObject` both cloned custom metadata in the normal write flow. The fix marks metadata that was already cloned from the request resource and leaves `persistGCSObject` as the store-boundary clone for any uncloned map, removing the redundant clone without weakening isolation.

Issue #241 finding: PR #238 centralized GCS object writes through `persistGCSObject`, but future direct `objects.Put` calls could bypass the disk-backed byte write and metadata normalization. The fix adds a source-level guard test that fails if GCS object store writes occur outside `persistGCSObject`.

Issue #236 finding: the GCS `rewriteTo` / `copyTo` endpoints were real public JSON API surfaces, and callers can supply a destination object resource body with metadata beyond `contentType`. The fix persists custom metadata plus HTTP metadata fields, applies destination-over-source precedence for supplied fields, inherits absent fields from the source object, and returns the stored fields from metadata reads and download headers.

Issue #237 finding: upload, resumable upload, compose, and copy/rewrite all performed the same object-byte write, checksum, timestamp, and store-update work independently. The fix routes those paths through one persistence helper so future object metadata changes update one real write path.

Issue #232 finding: Azure Blob Copy Blob is a public data-plane `PUT` selected by `x-ms-copy-source`, not a multipart-copy detail. The fix branches before Put Blob, resolves host-style and path-style source URLs with escaped names, copies the real stored source bytes, returns Azure copy ID/status headers, and preserves source metadata unless destination metadata is supplied.

Issue #233 finding: GCS object copy is a public JSON API surface. The fix implements canonical `rewriteTo` and legacy `copyTo` routes in the existing object POST handler, backed by real object bytes. `rewriteTo` completes synchronously with `done: true` for same-simulator copies and returns SDK-compatible string byte counts.

Issue #234 finding: GCS object listing is documented as lexicographic by object name. The fix sorts `items[]` after filtering and also sorts delimiter-produced `prefixes[]` for stable directory-style listings.

## Standing invariants (full list in STATUS.md)

- Never auto-merge; user merges every PR.
- Single-branch rule per phase; never more than 1 PR open.
- File BUGs in BUGS.md *before* any fix attempt.
- No fakes / no fallbacks / no silent shims.
- Every reopen carries a postmortem trail (`.claude/skills/reopen-postmortem/SKILL.md`).
- Every closed-enumeration surface has a `specs/SIM_SURFACE_TABLES/<name>.md` with no silent ✗ rows.
- Every List* op has a paged-iterator test (`sim-canonical-config-test` rule).
- Every stateful resource type has a state-machine assertion (`sim-state-machine-completeness`).
- `make hooks` on every fresh clone (wires `mux-overlap-scan` + gofmt + golangci-lint + …).

## Session-resume checklist

1. `git fetch origin && gh pr list --state open && git status`.
2. If a phase PR is open: `gh pr checks <N>`; report state.
3. If a PR merged: sync `main`, delete merged branch, prune remotes, refresh continuity docs to idle.
4. If fresh issues filed: `gh issue list --state open --limit 30`; file each as a BUG in BUGS.md before any fix attempt.
5. Read `.claude/skills/avoid-vibe-slop/SKILL.md` before any code change.

## Reference for next reopen / new issue

If a community-filed issue surfaces against a closed enumeration (subresources, ops on a single service, paged List, state-bearing resource), the routine is:

1. Identify the surface table at `specs/SIM_SURFACE_TABLES/<surface>.md`. If none exists, create one before any fix.
2. File a BUG in BUGS.md citing which row(s) the issue covers + which siblings should be checked.
3. Fix the named row + every reasonable sibling (`surface-table-completeness` rule).
4. SDK test uses the canonical client (no raw `net/http` where an SDK exists; for List* use a Pager).
5. For reopens: BUG entry MUST include the three postmortem fields (what test passed but should have failed / what SDK code path was missed / what new canonical-client test catches the regression).
