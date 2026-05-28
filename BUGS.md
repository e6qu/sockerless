# Known Bugs

**1216 filed · 1207 fixed · 11 open · 2 false positives.**

Standing rule: every CI / live-cloud failure lands here with a one-liner *before* any fix attempt. Workarounds, fakes, placeholders, silent fallbacks, skips, and incomplete implementations are all bugs and get the same treatment. Per-bug fix detail beyond the one-liner: `git log <commit>` or the linked PR.

Live status (cells, branch, milestone) lives in [STATUS.md](STATUS.md). Vibe-pattern numbers reference `docs/VIBE_CODING.md`.

## Open

| ID | Sev | Area | Pattern | One-liner |
|----|-----|------|---------|-----------|
| 1075 | P2 | live-cloud cells red for cloudrun Services + ACA Apps + AZF + Lambda service-mesh + ACA/AZF Azure AD | 6 (untested in real cloud) | Lambda is the only backend with a green live-cloud cell. Cloud Run Services + ACA Apps + AZF cloud-DNS + Lambda service-mesh + ACA/AZF Azure AD remain unvalidated against real clouds. The preflight/runbook work is documented, but the cells must not be marked green without a real run against authenticated cloud projects/subscriptions. |
| 1104 | P0 | Audit-cadence meta tracker — perpetual | meta | Every major phase runs the specialist-skill pass against the diff. This BUG stays Open as the audit-cadence reminder; it closes when there's no meaningful new sim work for ≥ 6 phases (i.e., simulator surface is genuinely complete + matches every active SDK contract). |
| 1200 | P1 | foundational stream/event ingestion services | 9 (missing cloud slice) | AWS Kinesis and Azure Event Hubs are absent. GCP Pub/Sub covers the basic message/event bus in the GCP sim, but the AWS/Azure stream-ingestion equivalents remain missing and should be added when implementing event-system parity. |
| 1201 | P0 | GCP simulator managed analytics data plane | 9 (missing cloud slice) | BigQuery is absent from the GCP simulator. Foundational dataset/table/job/query APIs used by official clients and Terraform must be implemented for the basic managed analytics data-store equivalent. |
| 1202 | P0 | cross-cloud managed NoSQL data-store parity | 9 (missing cloud slice) | AWS DynamoDB exists, but the GCP and Azure simulator equivalents are missing: Firestore/Datastore for GCP document/key-value flows and Cosmos DB for Azure NoSQL/table-style flows. Implement the public API slices rather than documenting the limitation. |
| 1203 | P0 | cross-cloud managed load balancers | 9 (missing cloud slice) | Managed load-balancer services are missing as first-class simulator slices: AWS ELBv2/ELB, GCP Cloud Load Balancing resources, and Azure Load Balancer/Application Gateway/Front Door/Traffic Manager. API Gateway/APIM/CloudFront coverage is not a substitute for L4/L7 managed load-balancer APIs. |
| 1204 | P1 | VPC egress and NAT parity | 7 (partial implementation) | VPC/network primitives exist, and NAT is partially modeled (AWS EC2 NAT Gateway, GCP Router NAT, Azure NAT Gateway), but parity is uneven: GCP address/manual-NAT resources, Azure Public IP/Public IP Prefix resources, subnet-NAT attachment/list semantics, and SDK/CLI/Terraform surface tables/tests need a full pass. |
| 1205 | P1 | Azure DNS parity | 9 (missing cloud slice) | Azure Private DNS is implemented for internal service discovery, but Azure public DNS zones/record sets are not a registered slice. Foundational DNS parity needs the Microsoft.Network/dnsZones public API alongside privateDnsZones. |
| 1206 | P1 | simulator surface-table audit debt | 12 (stale docs) | Several foundational surface tables still carry generic "deferred under BUG-1159 / BUG-1147" test-gap markers even though later phases added tests and those BUGs are closed. The tables must be refreshed so implementation/test status is accurate before claiming full simulator coverage. |
| 1207 | P0 | cross-cloud VM compute APIs | 9 (missing cloud slice) | The sims do not implement EC2 `RunInstances`/instance lifecycle, GCP Compute Engine instances, or Azure Virtual Machines. These should be public-cloud-compatible VM API slices; Firecracker or another real local microVM runtime can be the implementation substrate, but it must not leak into public simulator APIs. |
| 1211 | P0 | Azure simulator host-addressed data-plane local DNS | 7 (partial implementation) | Issue #247: Azure simulator host-addressed data-plane endpoints such as `{account}.blob.localhost`, `{namespace}.servicebus.localhost`, and `{topic}.eventgrid.localhost` preserve cloud-style host contracts but do not resolve on at least macOS. The local addressing strategy must be fixed systematically for real SDK/CLI/Terraform clients without test-side URL rewriting or simulator-specific public APIs. |

## Recently closed (last phase only — older history lives in PR descriptions + `git log`)

This phase closed BUG-1213, BUG-1214, and BUG-1215. AWS EventBridge now includes event-bus lifecycle, bus policy permissions, archive lifecycle, and replay lifecycle/delivery with official SDK, AWS CLI, and Terraform coverage where provider resources exist. Azure Event Grid now includes domains, domain topics, system topics, partner topics, and event subscriptions on each supported Event Grid scope, covered by the Azure management SDK where the current module exposes clients, Azure CLI/`az rest`, and azurerm Terraform resources where the provider exposes them. GCP Eventarc now includes channels, provider discovery, and channel connections with official SDK and gcloud coverage; Terraform coverage uses `google_eventarc_channel`, and schema inspection confirmed the current `hashicorp/google` and `hashicorp/google-beta` providers do not expose a channel-connection resource.

This phase closed BUG-1212 and BUG-1216. The simulator Docker test harness now has a real shared Linux test image with Go, Terraform, AWS CLI, gcloud, Azure CLI, Docker CLI, and Make installed. The top-level `make docker-test` target and each per-cloud `make docker-test` target run the existing SDK/CLI/Terraform test categories inside that image while mounting the repository root and host Docker socket. The Azure Terraform test harness now delegates direct macOS `go test` execution to the same Linux container, so the real `azurestack` and `azurerm` providers validate the simulator CA through Linux `SSL_CERT_FILE` instead of failing on Darwin's SystemCertPool behavior.

This phase closed BUG-1197, BUG-1198, and BUG-1199 for the foundational event-routing flows. The AWS simulator now has an EventBridge JSON-protocol slice for rules, targets, tags, and `PutEvents`, including SQS/SNS target delivery and SDK/CLI/Terraform coverage. The GCP simulator now has Eventarc v1 trigger lifecycle routes with AIP-style long-running operations and SDK/gcloud/Terraform coverage. The Azure simulator now has Microsoft.EventGrid topic and event-subscription ARM routes plus a real topic publish endpoint that fans out to webhook subscriptions, covered by the Azure management SDK, Azure CLI, and Terraform topic coverage. The advanced/sibling parity follow-up was later closed as BUG-1213, BUG-1214, and BUG-1215.

PR #246 closed BUG-1210 after the pre-push dependency-freshness hook found `google.golang.org/api` drift in the GCP simulator/backend modules. The affected modules now pin the current release required by the hook.

PR #246 closed BUG-1208 and BUG-1209 while landing the foundational simulator audit. The bleephub JWT invalid-signature regression now mutates decoded signature bytes before re-encoding, so the test cannot accidentally produce a still-valid signature by changing only trailing base64url characters. The GCF FaaS smoke harness now reuses already-built local Docker tags across its subprocess TestMain run instead of rebuilding Alpine tags from Public ECR and risking a registry 429.

PR #245 closed issues #243/#244 and BUG-1195/BUG-1196. Azure ARM handlers for Service Bus, Redis, APIM, PostgreSQL Flexible Server, and Container Apps now derive Azure-shaped endpoint fields from the incoming simulator ARM host instead of production cloud suffixes; Service Bus listKeys connection strings use the same derived namespace endpoint. The storage path-style dispatcher also recognizes the newly advertised non-storage Azure subdomains so collapsed-port storage routing does not steal those requests. Container Apps jobs and apps now inspect the resolved local image manifest before starting real containers, including sidecars, instead of hardcoding `linux/arm64`.

PR #242 closed BUG-1192 / issue #239, BUG-1193 / issue #240, and BUG-1194 / issue #241. GCS object metadata writes now validate the accepted fields with explicit published contracts (`customTime` as RFC 3339 and `contentLanguage` length <= 100 characters), invalid metadata returns `400 INVALID_ARGUMENT` across upload/resumable/compose/copy/rewrite paths, redundant metadata cloning is removed without weakening the store-boundary copy, and a source-level guard prevents future direct GCS object-store writes outside `persistGCSObject`.

PR #238 closed BUG-1190 / issue #236 and BUG-1191 / issue #237. GCS object copy/rewrite now persists destination object resource metadata (`metadata`, cache-control, content-disposition, content-encoding, content-language, storage-class, custom-time), inherits absent fields from the source object, returns the persisted fields from JSON metadata reads and download headers, and routes upload/resumable upload/compose/copy writes through the shared object-persistence helper.

PR #235 closed BUG-1187 / issue #232, BUG-1188 / issue #233, and BUG-1189 / issue #234. Azure Blob now implements the public Copy Blob data-plane operation via `x-ms-copy-source`, copies real blob bytes, preserves Azure copy headers/status on the destination, handles host-style and path-style source URLs, and applies destination metadata precedence over copied source metadata. GCS now implements the public JSON API `rewriteTo` and `copyTo` object-copy endpoints backed by real stored object bytes, and `objects.list` returns deterministic lexicographic object and prefix ordering.

PR #231 closes BUG-1186 / issue #230. Azure Service Bus now exposes a raw AMQP/TLS listener for the official `azservicebus` default transport, so callers can use the SDK's `CustomEndpoint` and `TLSConfig` knobs without wiring simulator-specific WebSocket adapters. The raw listener reuses the same AMQP parser/session implementation as the WebSocket path, resolves namespace from TLS SNI or the AMQP Open hostname, and is covered by official SDK queue and topic/subscription Send/Receive tests without `NewWebSocketConn`.

PR #229 closed BUG-1184 / issue #227 and BUG-1185 / issue #228. Azure Blob now implements real block blob staging for official `azblob/blockblob` StageBlock, CommitBlockList, and GetBlockList flows, with committed/uncommitted block state persisted in the simulator store. Azure Service Bus now exposes an AMQP 1.0 WebSocket data-plane slice for official `azservicebus` queue and topic/subscription Send/Receive, including SASL anonymous negotiation, CBS claim RPCs, entity sender/receiver links, link credit, accepted delivery dispositions, topic fan-out, subscription receiver paths, and receive-and-delete message transfer.

Phase 226 closes BUG-1183: Azure Storage blob/file/queue/table data-plane host/path dispatch now has official Azure SDK lifecycle + List pager coverage. The SDK tests exposed two real protocol gaps fixed in the simulator: File `GET /?comp=list` service-level ListShares, and Tables SDK entity-list path `/{table}()`. The phase also adds `azure-storage-data-plane.md` and refreshes stale Key Vault data-plane table rows.

PR #225 closes BUG-1182 / issue #223: Azure Service Bus namespace-level ATOM XML admin protocol was missing from the `{namespace}.servicebus.<host>` data-plane dispatcher, so the official `azservicebus/admin` SDK could not create, read, list, or delete queues, topics, subscriptions, or rules. The fix adds the namespace admin surface, SDK lifecycle + paged-list coverage, and an `azure-servicebus-admin` surface table. In-PR audit also fixed the systematic list bug where namespace filtering happened after `$top` / `$skip`.

PR #221 closed BUG-1181 / issue #220: Azure Blob `List Containers` now returns per-container `<Properties>` with `Last-Modified` and quoted `Etag`, backed by the same stored container ETag returned by `Get Container Properties`. Raw wire + real `az storage container list` regressions cover the gap.

PR #219 closed issue #218: GCP Secret Manager lifecycle endpoints for ListSecretVersions, UpdateSecret labels, DeleteSecret, replication metadata, and payload CRC32C across SDK, gcloud CLI, and Terraform flows.

Phase 179 (PR #216 + #217 follow-up) closed BUG-1174..1180 and issues #209/#210 reopens + #213/#214/#215; README badge refresh and hook portability followed in #217. BUGS.md after Phase 179 had only BUG-1075 + BUG-1104 open.

Phase 178 closed BUG-1148..1173 across 25 commits — see PR #211 description + `git log 7a7c9f0` for per-bug detail. Headlines: 9 community-filed issues (#196 reopen + #203..#210) + 5 class-of-bug remediations (proactive surface-table seed, mux-overlap scanner, paged-iterator rule, state-machine skill, CI-gated tf-tests) + 12 real handler / wire-shape gaps the new tf-azure gate surfaced.

Older closed BUGs: 1098..1147 across Phases 173–177. See [WHAT_WE_DID.md](WHAT_WE_DID.md) per-phase narrative + PR descriptions for fix detail.

## False positives

| Area | Finding | Why it's not a bug |
|------|---------|--------------------|
| `backends/aca/azure.go::fakeCredential` | Returns literal `"fake-token"` against simulator endpoints. | Sims don't verify bearer tokens — would require real Azure AD endpoint not emulated. Credential wired only via `newAzureClientsWithEndpoint` (sim path); production uses `azidentity.NewDefaultAzureCredential`. |
| `cmd/sockerless-admin/api_observability.go::envOrDefault` | Returns canonical OTel resource-attribute name when unset. | Documented default-value helper, not an error-hiding fallback. No silent failure mode. |

## Class-of-bug rules

- **Backend ↔ host primitive must match (P0).** ECS in ECS, Lambda in Lambda, Cloud Run in Cloud Run, GCF in CRF, ACA in ACA, AZF in AZF.
- **No fakes / no fallbacks / no skips.** Synthetic exit codes, silent shims, fake-data fallbacks, conditional `t.Skip` for missing config — all file as bugs.
- **Cross-cloud sweep on every find.** When a pattern surfaces in one backend, the same code paths in the other 5 backends / 3 sims get checked in the same commit.
- **HTTP 500 reserved for unexpected panics.** Never return 5xx as a designed failure path.
- **Doc-only fixes are unsafe when the cloud rejects the config.** Verify cloud-acceptable, not just sockerless-controllable.
- **External test fixtures must use the real client.** The `gh` CLI test harness uses real `gh repo create` against bleephub, not URL hackery.
- **Closed enumeration → full-table audit.** Every community-filed issue against a closed operation table (subresources, KV ops, Pub/Sub verbs, etc.) gets the full table reviewed before "fixed" is claimed (`surface-table-completeness` skill).
- **Reopens carry a postmortem trail.** Every reopened community-filed issue is P0 by default; the BUG entry must include (a) what test passed but should have failed, (b) what SDK code path was missed, (c) what new canonical-client test catches the regression (`reopen-postmortem` skill).
- **List* ops use paged-iterator tests.** Single-record envelopes from List endpoints silently pass `.Value[0]`-style tests (`sim-canonical-config-test` paged-iterator rule).
- **Stateful resources model their state machine.** Real cloud resources transition through documented lifecycle states; sim implementations carry the state field + transitions (`sim-state-machine-completeness` skill).
- **Mux pattern overlap is a real-bug class.** The collapsed-port sim has no DNS-level service isolation; root-greedy wildcards can shadow other services. Run `scripts/scan-mux-overlap.sh` (also wired into pre-commit) when adding routes.
