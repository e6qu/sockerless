# Do Next

Status [STATUS.md](STATUS.md) · roadmap [PLAN.md](PLAN.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · vibe catalogue [docs/VIBE_CODING.md](docs/VIBE_CODING.md) · architecture [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Where we are

**Phase 174 — skill-sweep audit + community-issue triage. Branch `phase-174-skill-sweep`. PR #180 (draft).**

Round 1 (skill audit, committed in `24362f7` + `a54a156`):
- BUG-1105 — 23 silent `sim.ReadJSON` sites across Phase 173 handlers (silent-error-swallow-scan caught my own diff after the skill was committed; BUG-1104 meta-shape in action).
- BUG-1106 — silent decode in `handleKVCreateCertificate`.
- BUG-1107 — dead `var _ = aws.Config{}` import silencer in metadata_sdk_test.go.
- BUG-1108 — dead `singleSelectInputValueType` "reserved for future" silencer in bleephub.

Round 2 (user's "fix all outstanding" + GitHub issues #181–#188, in flight on the same branch):
- BUG-1109 (umbrella) → Azure File/Queue/Table data planes (new `storage_dataplane.go` with subdomain dispatch + Files share+file CRUD + Queues with messages + Tables with OData entity addressing).
- BUG-1110 (umbrella) → `openStreamingBody(r)` helper per cloud (`simulators/azure/streaming.go`, `simulators/gcp/streaming.go`) wired into 9 upload handler sites (GCS, Cloud Run invoke, AR blob upload × 2, ACR blob upload × 4, Azure Blob PutBlob). Transparent gzip decode; 415 on unknown encoding.
- BUG-1111 → External URL field annotations on Azure Functions ScriptHref/ConfigHref/Href/InvokeURLTemplate + AWS Amplify WebhookUrl + AWS ECR RepositoryUri.
- BUG-1112 (#181) → Azure ARM case-insensitivity via path-normalization middleware.
- BUG-1113 (#182) → GCP Pub/Sub subscription response now echoes all 7 canonical fields.
- BUG-1114 (#183) → GCP Secret Manager ListSecrets registered explicitly (was falling through to GCS catch-all).
- BUG-1115 (#184) → Azure KV id/kid URLs no longer duplicate the vault host + emit https not http.
- BUG-1116 (#185) → Azure KV RSA key create generates a real `crypto/rsa.GenerateKey` modulus + base64url encoding.
- BUG-1117 (#186) → AWS SQS persists every CreateQueue / SetQueueAttributes attribute in a map; GetQueueAttributes echoes them.
- BUG-1118 (#187) → GCP Cloud SQL selfLink fully-qualified via `gcpSelfLink(r, path)` helper.
- BUG-1119 (#188) → GCP Secret Manager `:access` + GetSecretVersion resolve `latest` alias to the concrete version number in response `name`.

All AWS / GCP / Azure SDK regression suites green. `go vet` + gofmt clean.

BUGS.md after this branch lands: **1119 filed · 1117 fixed · 2 open · 2 false positives.** Open: BUG-1075 (live-cloud cells, deprioritized 2026-05-23) + BUG-1104 (meta tracking; Phase 174 IS the periodic audit the entry called for).

Branch `sim-fidelity-issues-173-178`: 20 commits (15 implementation + 5 CI-driven wrap) closing GitHub issues #173–#178 (all commented + closed) plus the meta blind-spot BUG-1104. ~180 new simulator operations across AWS / GCP / Azure; ~25 new SDK + HTTP integration tests; 6 new project-local Claude skills (`sim-canonical-config-test`, `sim-emitted-url-roundtrip`, `sim-streaming-body-handler`, `silent-error-swallow-scan`, `dead-code-silencer-scan`, `backpedal-pattern-audit`); sentinel-header logging across all 3 simulators' shared middleware.

CI surfaced 4 real issues, all fixed on the branch:
1. 17 stale Go-module + Terraform-provider pins — `make upgrade-deps` + 2 per-module bumps on `backends/{aws,gcp}-common`.
2. Makefile fanout missed `TEST_DIRS` — extended so sdk-tests/cli-tests/terraform-tests don't drift independently.
3. `gofmt -l` flagged 16 Phase 173 files I had not formatted — `gofmt -w`.
4. `staticcheck`/`unused` flagged 2 real Go issues — empty branch in `sns.go::handleSNSDeleteTopic` (restructured) and unused `(*SMSecret).currentVersion` method (deleted; dead-code-silencer-scan skill would have flagged it). Plus `go vet`'s printf-checker caught 3 non-constant format string calls to `sim.GCPErrorf` in `sqladmin.go` — passed `err.Error()` directly as the format; literal `%` in an error would have been misinterpreted. Fixed by passing `"%s"` as the constant format.

All 11 CI jobs pass: lint, check-deps, ui, terraform, sim (aws/gcp/azure), smoke, build-check, test, test (e2e).

**Open BUGs after merge**: BUG-1075 (live-cloud cells; deprioritized 2026-05-23) and BUG-1104 (meta tracking entry until a quarterly `backpedal-pattern-audit` confirms no new instances). BUGS.md: **1104 filed · 1101 fixed · 2 open · 2 false positives.**

## Phase 174 audit results

| Skill | Sites found | Fix shape | Status |
|---|---|---|---|
| `silent-error-swallow-scan` | 23 `_ = sim.ReadJSON` + 1 `_ = json.NewDecoder().Decode` | Add err check + 400 envelope | ✅ Fixed (BUG-1105, 1106) |
| `dead-code-silencer-scan` | 1 import silencer + 1 "reserved for future" var | Delete both + their dead refs | ✅ Fixed (BUG-1107, 1108) |
| `sim-canonical-config-test` | 0 quirk patterns | — | ✅ Regression-guard test still green |
| `sim-emitted-url-roundtrip` | Azure {File,Queue,Table} + AZF function URLs + Amplify webhook + ECR repo URI | New `storage_dataplane.go` for the 3 data planes; field-annotations for the rest | ✅ Fixed (BUG-1109 + 1111) |
| `sim-streaming-body-handler` | GCS object upload + Cloud Run invoke + AR blob × 2 + ACR blob × 4 + Azure Blob PutBlob = 9 sites | `openStreamingBody(r)` helper per cloud (gzip transparent decode + 415 unknown) | ✅ Fixed (BUG-1110) |
| `backpedal-pattern-audit` | 1105/1106 ≡ BUG-1016/1017 recurrence; 1107/1108 ≡ BUG-1020/1022 recurrence | Skills validated as load-bearing — the audit cadence IS the load-bearing piece | ✅ No new pattern categories |

## GitHub issues triage (round 2)

8 issues filed by the user against Phase 173 code, all closed in-branch:

| Issue | BUG | Headline |
|---|---|---|
| #181 | 1112 | Azure Cache for Redis ARM case-sensitive — fixed via path-normalization middleware (extended to 6 provider segments) |
| #182 | 1113 | GCP Pub/Sub Subscription response dropped 5 of 7 fields — added all 5 + ExpirationPolicy nested struct |
| #183 | 1114 | GCP Secret Manager ListSecrets fell through to GCS catch-all — registered explicit handler |
| #184 | 1115 | Azure KV id/kid URLs duplicated host + http scheme — buildKVURL now uses r.Host directly + https |
| #185 | 1116 | Azure KV RSA key create returned placeholder modulus — real crypto/rsa.GenerateKey + base64url N/E |
| #186 | 1117 | AWS SQS CreateQueue attributes silently dropped — added Attributes map; persist + echo all |
| #187 | 1118 | GCP Cloud SQL selfLink was relative — added gcpSelfLink helper, used for instance + database |
| #188 | 1119 | GCP Secret Manager `:latest` alias not resolved in response — accessSecretPayloadResolved + resolveLatestVersionID helpers |

## Phase 173 sub-phase status

| Sub | Bug | Issue | Commit | Headline |
|---|---|---|---|---|
| 173.0 | 1104 (meta) | — | `466c45e` `243dbdd` `ec076a8` | Planning + 6 new skills + sentinel-header logging |
| 173.1 | 1098 | #173 | `20aff53` | AWS S3 routes re-mounted at canonical root |
| 173.2 | 1099 | #174 | `b604c81` | AWS S3 aws-chunked envelope decoder |
| 173.3 | 1100 | #175 | `4b724e3` | AWS SM version history + ListSecretVersionIds + GetRandomPassword |
| 173.4 | 1101 (1/3) | #176 | `2176c7d` | AWS SQS (JSON) + SNS (Query) — SQS protocol-migration gotcha caught |
| 173.5 | 1101 (2/3) | #176 | `d549cd9` | AWS APIGW v2 + v1 — singular-`item` v1 quirk caught |
| 173.6 | 1101 (3/3) | #176 | `fce921c` | AWS RDS + ElastiCache with engine→port mapping |
| 173.7 | 1102 (1/3) | #177 | `37d7ec1` | GCP Pub/Sub (12 ops, REST) |
| 173.8 | 1102 (2/3) | #177 | `74cee70` | GCP Memorystore Redis + API Gateway (LRO) |
| 173.9 | 1102 (3/3) | #177 | `4ebcc61` | GCP Cloud SQL Admin (10 ops) |
| 173.10 | 1103 (1/3) | #178 | `e2e0c1f` | Azure Blob data plane via subdomain dispatch + KV keys/certs |
| 173.11 | 1103 (2/3) | #178 | `c5606d9` | Azure Cache for Redis (ARM) + Postgres FlexibleServer (ARM) |
| 173.12 | 1103 (3/3) | #178 | `70c6639` | Azure Service Bus + APIM (ARM, cascade-delete) |
| wrap | — | — | `9820931` | Dep freshness — 17 stale pins bumped + continuity docs |
| wrap | — | — | `4963570` | Makefile fanout extended to TEST_DIRS |
| wrap | — | — | `22026e0` | `gofmt -w` across 16 Phase 173 files (lint fix) |
| wrap | — | — | `ed215de` | staticcheck/unused — SA9003 empty branch in sns.go + unused SMSecret.currentVersion |
| wrap | — | — | `a448971` | go vet — 3 non-constant format string calls to GCPErrorf in sqladmin.go |

## Wire-protocol surprises caught (lessons for future-me)

1. **SQS migrated to awsJson1_0 in late 2023** — issue #176's "SQS: awsQuery" matched older docs. Always grep the SDK's serializer source first.
2. **APIGW v1 uses singular `"item"` as the list-response array field name** vs v2's plural `"items"`. Real-AWS inconsistency.
3. **JSON tag casing differs across services** — APIGW v1 uses lowercase (`id`, `item`), v2 uses camelCase (`apiId`, `items`).
4. **`json:"-"` strips fields during sim.Store JSON-serialized persistence** — use non-canonical tag names (`restApiIdRef`) when you need to keep parent refs in stored records.
5. **SNS → SQS fanout requires real `md5.Sum` on the JSON envelope** — aws-sdk-go-v2 SQS validates client-side.
6. **Azure Blob data plane needs `<account>.blob.<host>` Host-based subdomain dispatch** — same `WrapHandler` pattern KV already uses.

All codified in skills (`sim-handler-checklist`, `sim-canonical-config-test`, `sim-streaming-body-handler`, `sim-emitted-url-roundtrip`).

## Session-resume checklist

1. `git fetch origin && git checkout main && git pull origin main`.
2. `git log --oneline -10`.
3. After PR #179 merges: the next phase number is **174**. No specific phase queued; revisit Track A (live-cloud cells, BUG-1075) only after operator decides.
4. The `backpedal-pattern-audit` skill should be run periodically to surface any new repeat-pattern shapes in BUGS.md.
5. Read [`.claude/skills/avoid-vibe-slop/SKILL.md`](.claude/skills/avoid-vibe-slop/SKILL.md) before any code change.

## Phase 168 sub-task status

| Sub | Status | Headline |
|---|---|---|
| **P168.0** | ✅ | Filed 9 BUGs (1046–1054); 2 more (1055, 1056) surfaced + filed during P168.3 survey. |
| **P168.1** | ✅ | Lambda Path B ripped; `CallbackURL` required at NewServer; reverse-agent-only ExecStart (BUG-1046). Commit `5f745039`. |
| **P168.2** | ✅ | GCF + cloudrun Path B ripped (BUG-1047 + 1054). Commit `5f745039`. |
| **P168.3** | ✅ | `core.ReverseAgentRegistry.WaitForAgent` + per-backend `BootstrapTimeoutFromEnv` (default 90s, `SOCKERLESS_<BACKEND>_BOOTSTRAP_TIMEOUT_SEC`). Wired into ContainerStart for lambda / gcf / cloudrun / aca / azf. ACA `cloudExecStart` management-API fallback ripped (BUG-1056). GCF `SOCKERLESS_CALLBACK_URL` env injection added — was missing entirely (BUG-1055). |
| **P168.4** | ✅ | `exec_invoke.go` (lambda + GCF + cloudrun) + `core/exec_driver.go` CloudExecDriver interface DELETED. Commit `5f745039`. |
| **P168.5** | ✅ | tmpfs default for cloudrun + gcf + ACA. `core.StorageBackingRegistry.SetDefault(BackingMemory)`; `core.TmpfsSizeFromEnv` (default 2048 MiB, `SOCKERLESS_<BACKEND>_TMPFS_SIZE_MIB`); `core.ParseMemoryMiB` + `core.ValidateTmpfsFitsMemory`; GCF NewServer fatal when `tmpfs + 256 > GCF_MEMORY`; per-backend memory defaults bumped (GCF 4Gi, cloudrun 4Gi/container, ACA 4Gi/2 vCPU). Lambda + AZF unchanged (volume translators reject `BackingMemory`). |
| **P168.6** | ✅ | Bootstraps detect ENOSPC writes → exec envelope `exit_code=28` + operator-guidance message. Shared helper in `agent/enospc.go` (`DetectENOSPC` + `AnnotateENOSPC` + `ENOSPCExitCode=28`); wired into lambda + GCF + cloudrun bootstrap exec-result construction. AZF bootstrap was added later and PR #170 adds focused AZF bootstrap tests for exec-envelope stdin/env/workdir, default invoke/workdir, timeout parsing, and argv decode errors. |
| **P168.7** | ✅ | Strict cleanup-path errors: `ContainerRemove` on all 5 FaaS-style backends now accumulates errors via `errors.Join` and returns them. Split `deleteJob` / `deleteService` / `deleteApp` into two flavours: lenient (rollback paths, error logged) + `*Strict` (ContainerRemove, error propagates). Already-not-found is idempotent (returns nil). `docker rm` only succeeds when the cloud is actually clean. |
| **P168.8** | ✅ | Protocol type `agent.TypeLifetimeExpired` + `agent.SendLifetimeExpired(ws, mu)` helper + `ReverseAgentConn.OnSystemMessage` hook. Sockerless side: `ReverseAgentRegistry.MarkLifetimeExpired` / `IsLifetimeExpired`; wired into `HandleReverseAgentWS` so inbound `lifetime_expired` marks the container, and into ExecStart on lambda/gcf/cloudrun/aca/azf to return a `FaaSPodLifetimeExceeded` operator-guidance error. Lambda bootstrap wires the timer goroutine in `handleOneInvocation` (fires at deadline-5s of each invocation via `Lambda-Runtime-Deadline-Ms`). Cloud Run + GCF bootstraps catch SIGTERM, send `lifetime_expired`, and exit cleanly; ACA Apps and AZF now use reverse-agent bootstrap overlay paths. |
| **P168.9** | ✅ | E2E/readiness track merged across PR #168 (`3565e413`) and PR #169 (`0bd75902`). PR #170 adds `Test*FaaSE2ESmoke` for Lambda, Cloud Run, GCF, ACA, and AZF, plus `make faas-smoke-test-*`/`make faas-smoke-test-all` and CI wiring. The smoke guard surfaced BUG-1094 in Cloud Run/ACA service/app wait/remove semantics; fixed in the same branch. A simulator endpoint-fidelity sweep then surfaced and fixed BUG-1095 in GCP Artifact Registry remote-repo behavior, with SDK/CLI/Terraform/OCI regression coverage. Remaining follow-up: BUG-1075 live-cloud validation only. |

## Invariants snapshot (full list in STATUS.md)

- Never auto-merge; user merges every PR.
- Single-branch rule.
- File BUGs *before* fixing.
- Verify each significant chunk; don't batch fixes.
- **No fallbacks anywhere**: no silent substitution, no "best-effort with logging," no transparent re-invoke. If a primary path fails, surface it loudly to the operator.
- Driver pluggability preserved: each backend registers ONE driver per dimension; operator can swap; no primary-with-backup pairs.
- `gh` CLI is the reference adaptor for bleephub.
- SDK, CLI, and Terraform provider call sequences differ — endpoint-fidelity fixes need all three external-client layers when the service exposes them.
- `specs/CLOUD_RESOURCE_MAPPING.md` is authoritative.

## Resumable tracks (longer-horizon)

- **Track A** — Live-cloud validation (one branch per cell).
- **Track B** — UI / TypeScript vibe-slop sweep (carried from Phase 161).
- **Track C** — Phase 91d (bookmarked; needs cloud capability change).
- **Track D** — Phase 166 follow-up gaps: GCP Cloud Functions Gen2 + Pub/Sub + Compute instance/template terraform coverage; Azure Key Vault data-plane terraform coverage. Filed informally; can become a Phase 169 if leverage materialises.
- **Track E** — Run the new real-runner simulator arithmetic checks once a simulator-backed backend is intentionally started on `SOCKERLESS_DOCKER_HOST`; collect GitHub run URL / GitLab pipeline URL as evidence.

## Session-resume checklist

1. `git fetch origin && git checkout main && git pull origin main`.
2. `git log --oneline -10`.
3. Create or check out the active work branch, then read STATUS.md + this file + PLAN.md + BUGS.md § Open.
4. Read [`.claude/skills/avoid-vibe-slop/SKILL.md`](.claude/skills/avoid-vibe-slop/SKILL.md) before any code change.
5. Pick the next ◻ sub-task; mark it `in_progress` in tasks; commit when verified.
