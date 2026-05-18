# Do Next

Status [STATUS.md](STATUS.md) · roadmap [PLAN.md](PLAN.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · vibe catalogue [docs/VIBE_CODING.md](docs/VIBE_CODING.md) · architecture [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Where we are

Phase 168 follow-up merged 2026-05-18 (PR #170, `a5639811` on `origin/main`). Current branch is `docs/post-pr170-doc-sweep`, a documentation-only sweep to align continuity, runner, simulator, and backend docs with the implementation that landed in PR #170.

Merged PR #170 scope: FaaS runner smoke tests for Lambda/Cloud Run/GCF/ACA/AZF, Make/CI wiring for those smokes, AZF bootstrap test-pyramid coverage, simulator endpoint-fidelity fixes, and live-validation runbook/docs. The GCP Artifact Registry simulator-fidelity fix is covered by official SDK, gcloud CLI, Terraform provider, and OCI Distribution tests. BUG-1075 needs real live-cloud credentials/setup; do not fake or mark it done without a live run.

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

## Session-resume checklist

1. `git fetch origin && git checkout main && git pull origin main`.
2. `git log --oneline -10`.
3. Create or check out the active work branch, then read STATUS.md + this file + PLAN.md + BUGS.md § Open.
4. Read [`.claude/skills/avoid-vibe-slop/SKILL.md`](.claude/skills/avoid-vibe-slop/SKILL.md) before any code change.
5. Pick the next ◻ sub-task; mark it `in_progress` in tasks; commit when verified.
