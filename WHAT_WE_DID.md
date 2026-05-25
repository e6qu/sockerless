# Sockerless — What We Built

Docker-compatible REST API that runs containers on cloud backends (ECS, Lambda, Cloud Run, GCF, ACA, AZF) or local Docker. 7 backends, 3 cloud simulators, validated against SDKs / CLIs / Terraform. Designed to power CI runners on cloud serverless capacity — see [docs/RUNNERS.md](docs/RUNNERS.md).

State [STATUS.md](STATUS.md) · roadmap [PLAN.md](PLAN.md) · resume [DO_NEXT.md](DO_NEXT.md) · bugs [BUGS.md](BUGS.md) · architecture [specs/](specs/).

This file keeps narrative — *why* each phase, what was surprising, what blocked. Per-bug detail in [BUGS.md](BUGS.md); code-level detail in `git log`.

## 2026-05-25 — Phase 178: 9 community-filed issues closed (1 reopen + 8 new) + five class-of-bug remediations

PR #202 (Phase 177) merged. The user immediately filed 8 new issues (#203..#210) and reopened #196 (S3 multipart `ListParts` missed in Phase 176/177's S3 sweep). The reopen + the new issues decomposed into **five distinct classes of bug** that the existing skill catalogue caught only partially: (1) partial-table coverage — `surface-table-completeness` was reactive, requiring a table to exist before it could enforce completeness, and the repo had 30+ service surfaces without tables; (2) mux pattern collision — the collapsed-port sim had no scanner to flag pattern shadowing, so the wrong handler responded with a plausible-looking error; (3) wire-shape drift on List / paged endpoints — single-record envelope from a List op silently passed `.Value[0]`-style SDK tests; (4) state-machine fakery — sim stored data flat with no lifecycle states even though SDKs read them; (5) terraform-provider call sequence drift — `simulators/{cloud}/terraform-tests/` existed locally with 77 stock-provider resources across the three clouds but the CI `sim` job invoked only `sdk-test` + `cli-test`, so every recent reopen (BUG-1098 / 1099 / 1142 / 1147) surfaced against a tf-provider sequence the SDK tests had already marked green. Phase 178 closes the 9 community-filed items AND the 5 class-of-bug remediations on a single PR (18 commits, 6 stages + a CI-wiring follow-up).

**Stage A — infrastructure.** `scripts/seed-surface-tables.sh` proactively populates `specs/SIM_SURFACE_TABLES/` from every registered `mux.HandleFunc(...)` / `AWSRouter.Register(...)` call across `simulators/{aws,azure,gcp}/*.go` — 46 tables covering ~700 ops, so the reactive-skill gap (BUG-1145) becomes a proactive one. New `mux-overlap-scan` skill + scanner flags root-greedy wildcard patterns per cloud (286 baseline shadow pairs reported, almost all AWS S3's `{bucket}/{key...}` over literal-prefix services). Pre-commit hook runs the scanner in warn mode. Extended `sim-canonical-config-test` with a paged-iterator rule + `paged-shape verified` column in surface tables. New `sim-state-machine-completeness` skill formalises the data-model invariant for stateful resources with a worked example for Azure KV soft-delete.

**Stage B — AWS routing collisions.** BUG-1154 (#208): `AWSQueryRouter` extended with `RegisterVersioned(version, action, handler)`; dispatch is `(Version, Action) → handler` with the legacy `(Action,)` bucket as fallback. RDS / ElastiCache / SNS migrated to versioned registration; EC2 / IAM / STS keep legacy `Register` since their Action names are globally unique. BUG-1150 (#204): register the missing `POST /v2/apis/{apiId}/deployments` family + add a known-bucket gate to S3's POST/PUT/GET/DELETE `/{bucket}/{key...}` dispatchers so future missing-service paths fall through to canonical 404 instead of being swallowed.

**Stage C — AWS missing ops.** BUG-1148 (#196 reopened): `handleS3ListParts` returns canonical XML in monotonic PartNumber order; `aws-s3-multipart.md` surface table covers every multipart op. Reopen-postmortem in BUGS.md cites `manager.Uploader`'s mid-upload retry path as the SDK code path PR #200's test missed. BUG-1153 (#207): RDS snapshot family with `creating → available → deleted` state machine; SNS `SetTopicAttributes` persists `(AttributeName, AttributeValue)` into per-topic Attributes map; SQS `PurgeQueue` empties Messages while preserving Attributes/Tags.

**Stage D — Azure KV state machine.** BUG-1149 + 1151 (#203 + #205): `kvSecretStored` refactored from flat `name → SecretBundle` to a versioned chain with soft-delete state (`Versions []kvSecretVersion + DeletedAt + ScheduledPurgeAt + RecoveryID`). New handlers cover GetSecretVersion, ListSecretVersions (paged `SecretListResult`), PatchSecret (attribute-only updates per version), GetDeletedSecret, ListDeletedSecrets, RecoverDeletedSecret, PurgeDeletedSecret. State machine: active → soft-deleted (recoverable for 90 days) → recovered / purged. SDK tests `TestKeyVault_State_FullVersionChain` (uses `NewListSecretPropertiesVersionsPager` — the paged-iterator rule) + `TestKeyVault_State_SoftDeleteRoundTrip` (full transition cycle through canonical SDK methods).

**Stage E — Azure App Service config + extras.** BUG-1152 (#206): `/sites/{name}/config/{section}` routes for `appsettings` (PUT + POST `/list`), `connectionstrings` (PUT + POST `/list`), `web` (GET + PUT). POST `/list` follows real Azure's secret-bearing endpoint pattern. BUG-1156 (#210): PG FlexibleServer `configurations` LIST / GET / PUT; APIM `operations` + `backends` + `namedValues` CRUD; Cache Redis `firewallRules` CRUD + `POST .../listKeys`.

**Stage F — GCP missing ops.** BUG-1155 (#209): Cloud SQL `backupRuns.{insert,list,get,delete}` with state machine + `instances/{name}/clone` LRO; selfLink hard-coded `https://`. Memorystore Redis `:upgrade` + `:failover` with state-preserved transitions. Pub/Sub Snapshot CRUD with 7d ExpireTime. API Gateway AIP-130 IAM v1 (getIamPolicy / setIamPolicy / testIamPermissions).

**Stage G (CI follow-up) — gate the tf-tests.** BUG-1161: new `tf (aws|gcp|azure)` matrix job in `.github/workflows/ci.yml` runs `make tf-test` per cloud (stock `hashicorp/aws` / `hashicorp/google` / `hashicorp/azurerm` providers against the sim binary via `terraform init` + `apply -auto-approve`). The Makefile target + the test sources already existed; this just wires the gate so the tf-provider call shape is exercised every push.

BUGS.md after this branch lands: **1161 filed · 1159 fixed · 2 open.** The proactive surface-table seed + the mux-overlap scanner + the paged-iterator rule + the state-machine skill + the CI-gated tf-tests are the load-bearing scaffolding that should keep similar reopen patterns from recurring — the next time the user finds a missing op, the table audit catches it before "fixed" is claimed, and the tf-provider gate catches divergence between SDK and provider call sequences.

## 2026-05-24 — Phase 177: KV WWW-Authenticate URL reopen + S3 bucket-subresources + four meta-skill improvements

PR #200 merged. User pushed back twice: **issue #193 reopened** (Azure KV `WWW-Authenticate` `authorization` URL had only 3 path-split segments; every Azure SDK indexes `parts[3]` to extract the tenant and panicked), and **issue #201 filed** (S3 bucket-level PUT subresources routed to CreateBucket → 409). Both surfaced the moment a real consumer ran an SDK or terraform against the merged build.

The shape: my test for a fix matches my implementation's path, not the real-client contract. Phase 177 closes both community-filed items plus four meta improvements that turn the fix-and-reopen loop into a single-pass workflow:

- **BUG-1142** (#201) — S3 bucket-subresource dispatcher with 20-entry `bucketSubresourceHandlers` map; bodies persist in `s3BucketConfigs` so PUT → GET round-trips byte-for-byte.
- **BUG-1143** (#193 reopened, postmortem of BUG-1135) — Authorization URL changes to `http://<host>/00000000-0000-0000-0000-000000000000` (4 path segments). `keyvault_sdk_test.go` uses real `azsecrets` / `azkeys` / `azcertificates` SDK clients exercising the full challenge-then-retry handshake.
- **BUG-1144** — `sim-canonical-config-test` extended: "use the SDK, not raw HTTP" + "permitted SDK-config diffs" + "terraform-provider tests".
- **BUG-1145** — `specs/SIM_SURFACE_TABLES/` + `surface-table-completeness` skill. Initial population covered AWS S3 bucket-subresources + Azure KV data plane.
- **BUG-1146** — `reopen-postmortem` skill: every reopen carries (a) what test passed but should have failed, (b) what SDK code path was missed, (c) what new canonical-client test catches the regression. Backfilled BUG-1134 + BUG-1143.
- **BUG-1147** — terraform-tests parity: AWS gains 6 bucket-subresource resources; Azure gains `azurerm_key_vault_secret` exercising the challenge handshake at the terraform layer.

In-PR audit pass closed 5 additional findings (2 pre-existing fakes in s3.go, 2 timeless-comments violations, surface-table gaps). The new `surface-table-completeness` + `reopen-postmortem` skills exercised themselves against the same PR and surfaced their own findings — the new skills work.

## 2026-05-24 — Phase 176: 8 community-filed issues + path-style storage dispatcher

PR #192 merged. User filed 7 new issues + reopened #190 (path-style storage). Phase 176 closed all 8 on a single PR (BUG-1134..1141):

- **#193** Azure KV WWW-Authenticate (BUG-1135) — initial fix; reopened in Phase 177 because the URL format broke the SDK parser. Same commit replaced `"AQAB"` JWK placeholder with real EC / oct key generation.
- **#195** Azure Service Bus REST data plane (BUG-1137) — replaced stub with real `servicebus_dataplane.go`; SendMessage→201, ReceiveAndDelete→200/204, PeekLock→201+Location, CompleteLock→204.
- **#190 reopened** (BUG-1134) — `hasAzureStorageSignal(r)` discriminator (`x-ms-version` / `x-ms-date` / `x-ms-blob-type` / `restype` / `comp` / `SharedKey`) partitions storage path-style requests from IMDS / MSI / Monitor co-tenants on the shared sim port.
- Plus #194 RDS+ElastiCache EngineVersion (BUG-1136), #196 S3 multipart family (BUG-1138; ListParts missed → reopened in Phase 178), #197 GCP `/v1/operations` (BUG-1139), #198 GCS compose + resumable name + https (BUG-1140), #199 Lambda subresources (BUG-1141).

Twelve in-PR audit findings landed in the same branch: real KV crypto (no `AQAB`), SB PeekLock Location with `/messages/` segment, BrokerProperties marshal fail-loud, dead code purges, silent `io.ReadAll(_)` swallows fixed, KV WWW-Authenticate URL annotated `// external:`, streaming-body comments per the positive-confirmation rule, 4 phase-ref test comments rewritten.

## 2026-05-24 — Phase 175: second skill-sweep audit + 3 community-filed issues + signal-driven FaaS smoke tests

The audit cadence introduced in Phase 174 ran for the second time. Six parallel skill agents surfaced 36 candidate sites across 6 BUG umbrellas — every one a real bug, including two recurrences of Phase 174 introductions. Most consequential: two persistence-strip bugs (BUG-1098 shape) — GCP Secret Manager's `payload []byte` unexported, Azure KV Secret/Key/Cert carrying `json:"-"` on Vault+Name. Both fixed via persistence-wrapper records.

Three fresh GitHub issues against the post-Phase-174 sim code (#189 / #190 / #191) — all closed in the same PR.

Side-tracks: de-flaked the FaaS smoke tests (`ContainerStop` instead of file-based polling); split the CI `test` job into `test-core` + `test-backend` matrix×6 + `test-faas-smoke` matrix×5 (11 → 22 jobs); new `timeless-comments` skill codified the rule that code comments must explain current invariants, never narrate evolution.

14 BUGs closed (1120-1133).

## 2026-05-24 — Phase 174: first skill-sweep audit + community-issue triage (PR #180)

Two rounds on one branch. Round 1 ran the 5 specialist skills committed in Phase 173.0 across the repo and found exactly the patterns each was written to detect, including in code just landed in Phase 173 (the meta-shape — write the rule, then violate it elsewhere, then have the rule catch it). 4 quick fixes + 3 follow-up BUGs filed (1109 Azure storage data planes, 1110 streaming-body sweep, 1111 emitted-URL annotations).

Round 2 closed the 3 follow-ups plus all 8 new GitHub issues #181..#188 — case-insensitive ARM routes, Pub/Sub Subscription field drops, relative selfLink, KV duplicated host, placeholder RSA modulus, SQS Attributes drop, SM `latest` alias literal echo. 15 BUGs closed (1105–1119).

## 2026-05-23 — Phase 173: simulator wire-fidelity sweep (PR #179)

Six GitHub issues (#173–#178) surfaced wire-protocol drift across all three sims. The smoking gun was #173 (AWS S3 routes under `/s3/` prefix instead of canonical root) — latent since 165+ phases because three test-side workarounds independently reconfigured around it. The meta blind-spot codified as BUG-1104: simulator tests verifying the sim against itself rather than the real cloud.

Phase 173 sub-phases 0–12: six new project-local Claude skills (canonical-config, emitted-url-roundtrip, streaming-body, silent-error-swallow, dead-code-silencer, backpedal-pattern-audit); sentinel-header logging; ~180 new simulator operations across AWS (S3 routing + aws-chunked + Secrets Manager versions + SQS + SNS + API Gateway v1/v2 + RDS + ElastiCache), GCP (Pub/Sub + Memorystore + API Gateway + Cloud SQL), Azure (Blob data plane + KV keys/certs + Cache Redis + PG FlexibleServer + Service Bus + APIM). All six issues closed before PR merge.

## Compressed older-phase headlines (Phase 78–172)

Per-bug detail lives in [BUGS.md](BUGS.md); per-commit detail in `git log <PR-number>`.

| PR | Phase | Headline |
|---|---|---|
| #172 | pod-model follow-up | Sim pod materialization fidelity — real multi-container execution for ECS / Cloud Run Services + Jobs / ACA Apps; AZF docs corrected to single-container. |
| #170 | 168 follow-up | FaaS runner smokes for Lambda / Cloud Run / GCF / ACA Apps / AZF; AZF bootstrap hardening; GCP AR endpoint-fidelity; live-validation runbook. |
| #166 | 166 | Real fixes for 3 Phase-165 follow-ups: AWS 5 sim handler gaps (KMS / SM / SSM / DDB / S3 path-style), Azure azurerm-against-sim wiring, GCP `iam_beta_custom_endpoint`. |
| #165 | 165 | Third vibe-slop sweep + sim test-pyramid expansion + codex CLI review + continuity-doc compression. 9 BUGs closed. |
| #164 | 164 | Second vibe-slop sweep — 19 BUGs closed (1014–1032): strict-decode sweep, dead-code purges, phase-ref sweep, tf-tests expansion. |
| #163 | 163 | Makefile legacy alias rip-out + docs sweep. |
| #162 | 162 | `docs/VIBE_CODING.md` 23 → 35 patterns; `avoid-vibe-slop` skill expanded 17 → 26. |
| #161 | 161 | First comprehensive vibe-slop sweep — 18 BUGs closed (994–1008); bleephub GraphQL completion. |
| #160 | 160 | Project-local Claude skills (sim-handler-checklist + cross-resource-stack-test); component-README adaptor-led sweep. |
| #159 | 159 | AWS sim CloudFront + ACM + Route 53 + WAFv2 + Amplify + IAM SLR/OIDC (11 sub-tasks). |
| #158 | 158 | Handler→`s.self` delegation; `docs/VIBE_CODING.md` 23-pattern catalogue; first 3 project-local skills. |
| #155–157 | 155–157 | Component ⇄ reference-adaptor docs sweep; `backends/docker/README.md` rewrite. |
| #153–154 | 153–154 | bleephub ↔ GitHub API parity + SQLite persistence + real `gh` CLI compat. |
| #150–152 | 87c/d, 92 | zerolog → OTel logs bridge across 12 components; trace propagation; cloudrun + gcf `Backing: gcs-fuse` deregistered. |
| #147–149 | 91, 91b, 91c | `BackingMemory` translator across 5 backends; Lambda volume-translator framework. |
| #145–146 | 87, 87b | Observability stack (otel-collector + VictoriaLogs + Jaeger). |
| #143–144 | 85–86 | Config edit + hot reload; health + supervision (exit-code capture, `/diagnostics`). |
| #137–142 | 78–84 | UI polish + admin orchestration (`sockerless.yaml` topology, Topology page, per-instance logs + console). |
| #135–136 | 121b | Azure sim hardening; driver consolidation pattern B. |
| #128–134 | 124–134 | Driver framework + Makefile std + sim host model + arm64 CI runners. |
| #125 | CI reorg | Workflows reorganized: zero auto-fire on main; live-tests-{cloud}. |
| #112–123 | 86–123 | Sim parity; stateless backends; FaaS pod overlays; **8/8 runner cells GREEN**. |
