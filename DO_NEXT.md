# Do Next

Status [STATUS.md](STATUS.md) · roadmap [PLAN.md](PLAN.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · vibe catalogue [docs/VIBE_CODING.md](docs/VIBE_CODING.md) · architecture [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Where we are

Phase 166 merged 2026-05-17 (PR #167, `49050c2d` on `origin/main`).

**Phases 167 + 168 in flight on `phase-167-pod-model-analysis`** — PR #168 is open; keep using the same branch and PR. User directive: begin work with stated defaults; surface new findings; update continuity docs and check CI after each significant chunk.

## Phase 168 sub-task status

| Sub | Status | Headline |
|---|---|---|
| **P168.0** | ✅ | Filed 9 BUGs (1046–1054); 2 more (1055, 1056) surfaced + filed during P168.3 survey. |
| **P168.1** | ✅ | Lambda Path B ripped; `CallbackURL` required at NewServer; reverse-agent-only ExecStart (BUG-1046). Commit `5f745039`. |
| **P168.2** | ✅ | GCF + cloudrun Path B ripped (BUG-1047 + 1054). Commit `5f745039`. |
| **P168.3** | ✅ | `core.ReverseAgentRegistry.WaitForAgent` + per-backend `BootstrapTimeoutFromEnv` (default 90s, `SOCKERLESS_<BACKEND>_BOOTSTRAP_TIMEOUT_SEC`). Wired into ContainerStart for lambda / gcf / cloudrun / aca / azf. ACA `cloudExecStart` management-API fallback ripped (BUG-1056). GCF `SOCKERLESS_CALLBACK_URL` env injection added — was missing entirely (BUG-1055). |
| **P168.4** | ✅ | `exec_invoke.go` (lambda + GCF + cloudrun) + `core/exec_driver.go` CloudExecDriver interface DELETED. Commit `5f745039`. |
| **P168.5** | ✅ | tmpfs default for cloudrun + gcf + ACA. `core.StorageBackingRegistry.SetDefault(BackingMemory)`; `core.TmpfsSizeFromEnv` (default 2048 MiB, `SOCKERLESS_<BACKEND>_TMPFS_SIZE_MIB`); `core.ParseMemoryMiB` + `core.ValidateTmpfsFitsMemory`; GCF NewServer fatal when `tmpfs + 256 > GCF_MEMORY`; per-backend memory defaults bumped (GCF 4Gi, cloudrun 4Gi/container, ACA 4Gi/2 vCPU). Lambda + AZF unchanged (volume translators reject `BackingMemory`). |
| **P168.6** | ✅ | Bootstraps detect ENOSPC writes → exec envelope `exit_code=28` + operator-guidance message. Shared helper in `agent/enospc.go` (`DetectENOSPC` + `AnnotateENOSPC` + `ENOSPCExitCode=28`); wired into lambda + GCF + cloudrun bootstrap exec-result construction. AZF bootstrap doesn't exist yet (AZF uses the universal sockerless-agent path); skipped. |
| **P168.7** | ✅ | Strict cleanup-path errors: `ContainerRemove` on all 5 FaaS-style backends now accumulates errors via `errors.Join` and returns them. Split `deleteJob` / `deleteService` / `deleteApp` into two flavours: lenient (rollback paths, error logged) + `*Strict` (ContainerRemove, error propagates). Already-not-found is idempotent (returns nil). `docker rm` only succeeds when the cloud is actually clean. |
| **P168.8** | ✅ | Protocol type `agent.TypeLifetimeExpired` + `agent.SendLifetimeExpired(ws, mu)` helper + `ReverseAgentConn.OnSystemMessage` hook. Sockerless side: `ReverseAgentRegistry.MarkLifetimeExpired` / `IsLifetimeExpired`; wired into `HandleReverseAgentWS` so inbound `lifetime_expired` marks the container, and into ExecStart on lambda/gcf/cloudrun/aca/azf to return a `FaaSPodLifetimeExceeded` operator-guidance error. Lambda bootstrap wires the timer goroutine in `handleOneInvocation` (fires at deadline-5s of each invocation via `Lambda-Runtime-Deadline-Ms`). Cloud Run + GCF bootstraps now catch SIGTERM, send `lifetime_expired`, and exit cleanly; ACA + AZF remain tied to the missing real bootstrap paths tracked in BUG-1067 / BUG-1069. |
| **P168.9** | ◐ | E2E/readiness track. Cloud Run + GCF overlay-baked bootstrap exec now pass real simulator backend tests (`ContainerStart` reverse-agent registration + `docker exec` over WebSocket), and the full Cloud Run/GCF backend package simulator suites pass after scoping the exec e2e harness and fixing implicit-network cleanup (BUG-1081 / 1082). CI runs `26002912832` / `26003621756` / `26003955164` exposed harness/simulator regressions, now fixed locally: Cloud Run/ACA smoke declares a reachable callback URL, listens on all container interfaces, opens stdin for non-exec smoke containers, and preloads `alpine` into the simulator host Docker daemon; the GCP simulator overlay readiness probe now TCP-dials the bootstrap listener instead of invoking `/`; Cloud Run v2 Service EnvVar decoding accepts both real proto-JSON oneof shapes and SDK coverage asserts reverse-agent env round-trips; GCP overlay build platform is explicit in env and unified config, and simulator tests use the host platform; Cloud Run Service simulation keeps a real per-Service container instance alive until delete. CI run `26004958009` then exposed BUG-1087 in GCF one-shot lifecycle state, and full local GCF package validation exposed BUG-1088 in simulator cold-start invoke handling; both are fixed and pushed. CI run `26011828766` then exposed BUG-1089: ACA Jobs were waiting for a reverse-agent on stock one-shot job images. That fix is pushed, and BUG-1068 is pushed too: ACA Apps simulator PUT now starts real App-replica containers and logs under `ContainerAppName_s`. CI run `26012702755` then exposed BUG-1090: Azure SDK monitor tests still read `Log_s` by fixed column ordinal after the real App-name column landed. That test bug is fixed locally by resolving result columns by name, and the focused monitor tests pass locally. Remaining: commit/push this chunk, re-check CI on PR #168, and keep ACA/AZF e2e readiness blocked on missing bootstraps BUG-1067 / 1069 plus lifetime follow-up 1071. External `codex review` is blocked unless the user explicitly approves exporting repo diff/context. |

## Invariants snapshot (full list in STATUS.md)

- Never auto-merge; user merges every PR.
- Single-branch rule.
- File BUGs *before* fixing.
- Verify each significant chunk; don't batch fixes.
- **No fallbacks anywhere**: no silent substitution, no "best-effort with logging," no transparent re-invoke. If a primary path fails, surface it loudly to the operator.
- Driver pluggability preserved: each backend registers ONE driver per dimension; operator can swap; no primary-with-backup pairs.
- `gh` CLI is the reference adaptor for bleephub.
- Terraform provider call sequences differ from raw SDK — both test layers required.
- `specs/CLOUD_RESOURCE_MAPPING.md` is authoritative.

## Resumable tracks (longer-horizon)

- **Track A** — Live-cloud validation (one branch per cell).
- **Track B** — UI / TypeScript vibe-slop sweep (carried from Phase 161).
- **Track C** — Phase 91d (bookmarked; needs cloud capability change).
- **Track D** — Phase 166 follow-up gaps: GCP Cloud Functions Gen2 + Pub/Sub + Compute instance/template terraform coverage; Azure Key Vault data-plane terraform coverage. Filed informally; can become a Phase 169 if leverage materialises.

## Session-resume checklist

1. `git fetch origin && git checkout phase-167-pod-model-analysis && git pull`.
2. `git log --oneline -10`.
3. Read STATUS.md + this file + PLAN.md § Active phase + BUGS.md § Open.
4. Read [`.claude/skills/avoid-vibe-slop/SKILL.md`](.claude/skills/avoid-vibe-slop/SKILL.md) before any code change.
5. Pick the next ◻ sub-task; mark it `in_progress` in tasks; commit when verified.
