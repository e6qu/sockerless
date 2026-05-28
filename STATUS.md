# Sockerless — Status

Roadmap [PLAN.md](PLAN.md) · resume [DO_NEXT.md](DO_NEXT.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · vibe catalogue [docs/VIBE_CODING.md](docs/VIBE_CODING.md).

## Snapshot

| | |
|---|---|
| Active branch | `main` - idle after stream/event ingestion parity. |
| In-flight | None; next implementation pass starts from the remaining managed-data and infrastructure simulator audit gaps. |
| Last merged | AWS Kinesis and Azure Event Hubs stream/event ingestion parity (2026-05-28). |
| Standing merge auth | **None.** User merges every PR. |
| Cells | 8/8 runner-integration cells GREEN since 2026-05-07. |
| Bugs | 1216 filed · 1209 fixed · 9 open · 2 false positives. Open: BUG-1075, BUG-1104, BUG-1201..1207. |
| Live infra | None up. |

## Invariants (carry across compactions / fresh sessions)

### Process
- **Never auto-merge PRs.** Push, wait for `gh pr checks` green, ping user.
- **Single-branch rule.** All in-flight work for one phase lands on one branch; many granular commits, one PR.
- **File BUGs *before* fixing.** Survey first, write `BUGS.md § Open` entries, only then start fix commits.
- **State save every task.** STATUS.md + DO_NEXT.md + WHAT_WE_DID.md + MEMORY.md.
- **Test all the time.** `go test ./...` in every touched module; harness-touch re-runs the harness; terraform-touch runs `terragrunt validate`.
- **Branch hygiene.** Rebase phase branch on `origin/main` before pushing; sync local `main` after merge.
- **Pre-push hooks own the truth.** If `check-latest-deps` flags dep drift, bump deps in the same branch — never skip.
- **Read `.claude/skills/avoid-vibe-slop/SKILL.md` before every non-trivial change.**

### Architecture
- **Components stay decoupled from admin / UI.** Sims, backends, bleephub run independently via env vars; admin reads only `/v1/health`, `/v1/info`, env.
- **Backend ↔ host primitive must match.** ECS in ECS, Lambda in Lambda, Cloud Run in Cloud Run, GCF in CRF, ACA in ACA, AZF in AZF.
- **No fakes / no fallbacks.** Unknown values fail loud. Operator-requested persistence + auth never silently degrade.
- **Persistence is opt-in + fail-loud.** `BLEEPHUB_PERSIST=true` / `SIM_PERSIST=true` → SQLite. Open-failure *and* write-failure must surface.
- **HTTP handlers in `backends/core/handle_*.go` must dispatch through `s.self.<Method>`** — never read `s.Store.*` directly.
- **Test target gating.** Backend integration tests require `SOCKERLESS_TEST_TARGET=sim|cloud`; never implicit skip.
- **No phase / bug IDs in code comments or test docstrings.** Metadata lives in commits / PRs / BUGS.md; comments document the *why*.
- **SDK / CLI / Terraform-provider call sequences differ materially.** Simulator endpoint-fidelity fixes need the real external clients, not internal shortcuts.
- **`specs/CLOUD_RESOURCE_MAPPING.md` is authoritative** for "how does sockerless model X on cloud Y."
- **Closed enumeration → full-table audit before "fixed".** Surface tables live in `specs/SIM_SURFACE_TABLES/` (46+ tables seeded in Phase 178). No silent ✗ rows.
- **Every reopen carries a postmortem trail** (what test passed but should have failed; what SDK code path was missed; what new canonical-client test catches the regression). P0 by default.
- **Mux pattern overlap is a real-bug class.** Collapsed-port sim has no DNS-level service isolation; `scripts/scan-mux-overlap.sh` runs in pre-commit (warn mode).
- **List* ops use paged-iterator tests.** Single-record envelopes silently pass `.Value[0]`-style tests.
- **Stateful resources model their state machine.** `sim-state-machine-completeness` skill audits every row of every surface table whose handler is stateful.

### bleephub-specific
- **`gh` CLI is the reference adaptor.** No URL hackery.
- **`gh` is HTTPS-only against non-`github.com` hosts.** Quick-start covers self-signed-cert + system-trust path.
- **GitHub Apps and OAuth Apps are separate concepts** with distinct token prefixes (`ghp_` / `gho_` / `ghu_` / `ghs_` / `ghr_`).
- **No `alg:none` JWTs in OAuth issuance.**

## Recently closed phases

| PR | Phase | Headline |
|---|---|---|
| issues #260/#261 | Azure Storage keys + Entra JWKS | Azure Storage account `listKeys` now emits deterministic per-account 512-bit base64 SharedKeys, and Azure Entra simulator tokens are RS256-signed with the public RSA key published through JWKS. Shims and other downstream data-plane verifiers can validate SharedKey and bearer-authenticated requests without simulator-only public API fields or shared-secret side channels. Service Bus AMQP receiver links with outstanding credit are also drained when a later send enqueues matching messages. |
| issue #257 | Azure ARM shim endpoint composition | Azure ARM responses now advertise operator-configured, shim-routable data-plane endpoints via `SIM_AZURE_ARM_EXTERNAL_DATA_PLANE_URLS_JSON` while keeping public Azure request/response shapes. Storage, Key Vault, Service Bus/Event Hubs, Event Grid, and metadata suffixes share the configured host projection. The phase also added resource group list/resource enumeration, Storage Account PATCH, Key Vault PATCH/access-policy routes, deleted-vault Get/List/Purge state, fuller provider metadata, and SDK/CLI/Terraform coverage for the azurerm composition path. |
| stream ingestion | AWS Kinesis + Azure Event Hubs | Closed BUG-1200. AWS Kinesis now supports stream lifecycle, shard listing, record put/read flows, tags, retention, enhanced monitoring, encryption state, shard-count updates, and limits with SDK/CLI/Terraform coverage. Azure Event Hubs now supports ARM namespace/event hub/consumer group/auth-rule lifecycle and Event Hubs AMQP send/receive over the raw AMQP/TLS listener, with SDK/CLI/Terraform coverage. The AWS workload callback host was also corrected so Lambda Runtime API containers reach the host sidecar through `host.docker.internal` under Podman. |
| #256 | Azure data-plane DNS portability | Closed BUG-1211/#247. The Azure simulator now has an opt-in DNS listener that serves configured local zones over UDP and TCP for host-addressed simulator endpoints such as Blob Storage, Service Bus, Event Grid, and Key Vault. Real local clients can keep Azure-shaped host URLs and configure their resolver to the simulator DNS listener instead of rewriting URLs or injecting private Host headers. |
| #255 | advanced event-service parity | Closed BUG-1213/#249, BUG-1214/#250, and BUG-1215/#251. AWS EventBridge now supports event-bus lifecycle, bus policy permissions, archives, and replays with SDK/CLI/Terraform coverage. Azure Event Grid now supports domains, domain topics, system topics, partner topics, and event subscriptions on those scopes, with SDK coverage for current module-exposed clients, CLI coverage for partner-topic routes, and Terraform coverage for provider-exposed resources. GCP Eventarc now supports channels, provider discovery, and channel connections; Terraform coverage uses `google_eventarc_channel`, and provider-schema inspection confirmed no `google_eventarc_channel_connection` resource in current `hashicorp/google` or `hashicorp/google-beta`. |
| #254 | simulator Docker test harness | Closed BUG-1212/#248 and BUG-1216/#253 by adding one shared Linux Docker test image with Go, Terraform, AWS CLI, gcloud, Azure CLI, Docker CLI, and Make; wiring top-level and per-cloud `make docker-test` targets to run the existing SDK/CLI/Terraform suites from the repository root; and delegating Azure Terraform `go test` on macOS into that Linux image so the real Terraform providers can trust the generated simulator CA via `SSL_CERT_FILE`. |
| #252 | foundational event routing | Closed BUG-1197..1199 by adding AWS EventBridge rule/target/event delivery, GCP Eventarc trigger lifecycle, and Azure Event Grid topic/subscription/publish flows. Coverage uses official SDKs, vendor CLIs, and Terraform provider resources. Follow-up BUG-1211/#247 was later closed by the Azure data-plane DNS portability phase; BUG-1213/#249, BUG-1214/#250, and BUG-1215/#251 were later closed by the advanced event-service parity phase. |
| #246 | foundational simulator audit | Recorded foundational simulator service coverage in `specs/SIM_FOUNDATIONAL_AUDIT.md`; filed BUG-1197..1207 for missing EventBridge/Eventarc/Event Grid, Kinesis/Event Hubs, BigQuery, Firestore/Datastore, Cosmos DB, VM lifecycle APIs, managed load balancers, Azure public DNS, NAT/public-IP parity, and stale surface-table status rows; closed BUG-1208/BUG-1209 by making the bleephub invalid-JWT test deterministic and reusing local Docker image tags across the GCF FaaS smoke subprocess; closed BUG-1210 by refreshing the GCP modules to the latest `google.golang.org/api`. |
| #245 | issues #243/#244 | Azure ARM handlers for Service Bus, Redis, APIM, PostgreSQL Flexible Server, and Container Apps now derive Azure-shaped endpoint fields from the simulator ARM request host; Service Bus listKeys connection strings use the same derived namespace endpoint; Container Apps Jobs/Apps derive Docker platform from each resolved local image manifest instead of hardcoding `linux/arm64`. |
| #242 | issues #239/#240/#241 | GCS metadata writes now validate `customTime` and `contentLanguage`, invalid metadata returns `400 INVALID_ARGUMENT` across upload/resumable/compose/copy/rewrite, redundant metadata cloning is removed, and direct GCS object-store writes are guarded by a source-level test. |
| #238 | issues #236/#237 | GCS copy/rewrite now honors destination object resource metadata, inherits absent fields from the source object, returns metadata in JSON reads and download headers, and shares upload/resumable upload/compose/copy persistence through one helper. |
| #235 | issues #232/#233/#234 | Azure Blob Copy Blob now handles `x-ms-copy-source` with real stored-byte copies, Azure copy headers/status, host/path-style source URL parsing, escaped blob names, and metadata precedence. GCS now implements JSON API `rewriteTo` / `copyTo` object copy and returns lexicographically sorted object and prefix listings. |
| #231 | issue #230 | Azure Service Bus now exposes raw AMQP/TLS transport for the official `azservicebus` default path, with namespace routing preferring AMQP Open hostname over TLS SNI fallback, entity routing through AMQP link addresses, and SDK queue + topic/subscription Send/Receive tests using `CustomEndpoint` without `NewWebSocketConn`. |
| #229 | issues #227/#228 | Azure Blob block staging routes now support official `azblob/blockblob` StageBlock, CommitBlockList, and GetBlockList; Azure Service Bus message data plane now supports official `azservicebus` AMQP-over-WebSocket queue and topic/subscription Send/Receive with CBS negotiation and receive-and-delete coverage. |
| #226 | 226 | Azure Storage blob/file/queue/table data-plane host/path dispatch now has official Azure SDK lifecycle + List pager coverage; SDK audit fixed missing File ListShares and Tables `/{table}()` entity-list shape; added `azure-storage-data-plane` surface table and refreshed stale Key Vault data-plane table rows. |
| #225 | issue #223 | Azure Service Bus namespace-level ATOM XML admin routes for queues, topics, subscriptions, and rules; official `azservicebus/admin` SDK lifecycle coverage; surface table for the host-scoped admin protocol. |
| #222 | admin stack UI | Fixed stack Makefile parsing/background-process/env defaults; admin UI can restart/stop individual topology instances and managed processes, schedule full `make stack-down`, link component UIs, create default local contexts, and show recovery `make` commands on failure. |
| #221 | issue #220 | Azure Blob `GET /?comp=list` now emits per-container `<Properties>` with `Last-Modified` and quoted `Etag`; container ETags persist from create through get/list. Raw wire + real Azure CLI regressions added. |
| #219 | issue #218 | GCP Secret Manager lifecycle endpoints: ListSecretVersions, UpdateSecret labels, DeleteSecret, replication metadata, payload CRC32C; covered by SDK, gcloud CLI, and Terraform tests. |
| #217 | 179 follow-up | README badge refresh from Phase 179 plus hook portability fixes (`lint-changed.sh`, `check-cloud-backend-isolation.sh`). |
| #216 | 179 | 2 reopens (#209/#210) + 3 new issues (#213/#214/#215). BUG-1174..1180 closed: GCP Redis upgrade/failover keying, Pub/Sub IAM verbs, Azure real-shape listKeys, Resources Tags API, Service Bus authorizationRules, AWS IAM policy/instance-profile lifecycle, API Gateway method/integration responses. |
| #211 | 178 | 9 community-filed issues (#196 reopen + #203..#210) + 5 class-of-bug remediations: proactive surface-table seed (46 tables) + mux-overlap scanner + paged-iterator rule + state-machine skill + CI-gated tf-tests (`tf (aws\|gcp\|azure)` matrix). 25 BUGs closed (1148-1173). Surfaced + fixed 12 real handler / shape gaps via the new tf-azure gate (KV subscription-list / deletedVaults / purge, App Service publishingcredentials/authsettings/slotconfignames/logs/backup/basicPublishingCredentialsPolicies, Microsoft.Web checkNameAvailability, azurestorageaccounts properties-shape, ACA listSecrets, App Service config case-canonicalization, arm64 image manifest, ECR-Public rate-limit retry). Merged at `7a7c9f0`. |
| #202 | 177 | KV WWW-Authenticate URL reopen (#193) + S3 bucket-subresources (#201) + 4 meta-skill improvements (sim-canonical-config-test extension, surface-table-completeness, reopen-postmortem, tf-tests parity). 6 BUGs closed (1142-1147). Merged at `aa847b1`. |
| #200 | 176 | 8 community-filed issues (#190 reopened + #193..#199) + 12 in-PR audit findings + path-style storage dispatcher contamination + `make hooks`. 8 BUGs closed (1134-1141). Merged at `2d8e604`. |
| #192 | 175 | Second skill-sweep audit + 3 community-filed issues (#189/#190/#191) + signal-driven FaaS smoke tests + CI test-job split + new `timeless-comments` skill. 14 BUGs closed (1120-1133). Merged at `ca11405`. |
| #180 | 174 | Skill-sweep audit + community-issue triage — 3 rounds; 15 BUGs closed (1105–1119). Merged at `7a5d588`. |
| #179 | 173 | Simulator wire-fidelity sweep — 20 commits, ~180 sim ops added; closes #173–#178 + BUG-1098..1104. Merged at `64a13a8`. |

Older phases (#112–#172): one-line headlines in [PLAN.md § Closed phases](PLAN.md); per-phase narrative in [WHAT_WE_DID.md](WHAT_WE_DID.md).
