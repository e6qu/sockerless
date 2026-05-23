# Do Next

Status [STATUS.md](STATUS.md) · roadmap [PLAN.md](PLAN.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · vibe catalogue [docs/VIBE_CODING.md](docs/VIBE_CODING.md) · architecture [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Where we are

PR #172 (pod-model simulator fidelity follow-up — BUG-1096/1097) merged at `1c1fd92`. Next work is **Phase 173 — Simulator wire-fidelity sweep**, a single umbrella branch + PR (`sim-fidelity-issues-173-178`) covering GitHub issues #173–178 plus the meta blind-spot **BUG-1104**. Sub-phases 173.0–173.12 land as granular commits on the same branch; tests pass at each commit boundary; user controls when to wrap up and merge.

**Live-cloud (BUG-1075) is deprioritized** per 2026-05-23 user directive — drive simulator-fidelity bugs and missing-service coverage to ground first, then return to Track A.

GitHub issues triaged 2026-05-23 (all reply-comments posted):
- #173 (S3 `/s3/` URL prefix) → BUG-1098 (P0) → 173.1.
- #174 (S3 stores `aws-chunked` envelope verbatim) → BUG-1099 (P0) → 173.2.
- #175 (Secrets Manager missing `ListSecretVersionIds`) → BUG-1100 (P1) → 173.3.
- #176 (AWS sim missing SQS / SNS / APIGW v1+v2 / RDS / ElastiCache) → BUG-1101 (P2 umbrella) → 173.4 (SQS+SNS) / 173.5 (APIGW v2+v1) / 173.6 (RDS+ElastiCache).
- #177 (GCP sim missing Pub/Sub / Cloud SQL / Memorystore / API Gateway; Secret Manager **already implemented**, correction noted on issue) → BUG-1102 (P2 umbrella) → 173.7 (Pub/Sub) / 173.8 (Memorystore+APIGW) / 173.9 (Cloud SQL Admin).
- #178 (Azure sim missing Blob data plane / Service Bus / Postgres / Redis / APIM; KV secrets data-plane **already implemented**, correction noted on issue) → BUG-1103 (P2 umbrella) → 173.10 (Blob + KV keys/certs) / 173.11 (Redis + Postgres) / 173.12 (Service Bus + APIM).

Plus the meta-bug:
- **BUG-1104 (P0 meta)** — Simulator test infrastructure verifies the sim from the inside, not from the outside. Sub-phase 173.0 codifies the four blind-spot fixes (canonical-config invariant in `sdk-tests/`, stock-binary CLI smoke, emitted-URL round-trip lint, sentinel-header logging) plus README scope tables and 6 new project-local skills under `.claude/skills/` (`sim-canonical-config-test`, `sim-emitted-url-roundtrip`, `sim-streaming-body-handler`, `silent-error-swallow-scan`, `dead-code-silencer-scan`, `backpedal-pattern-audit`).

Per the "no stubs" directive every new sim handler persists real state, returns real-cloud response shapes, and is covered by SDK + CLI + Terraform fidelity tests where the external client surface exists.

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
