# Do Next

Snapshot pointer for the next session. Updated after every task.

## Branch state

`continue-plan-post-113` — successor to PR #113. Phase 91/92/93 (real per-cloud volume provisioning on ECS/CR/ACA) landed on this branch. BUG-735/736/737 fixed. Queue below is the revised sequence after the 2026-04-21 "no workarounds, no fakes, real implementations" directive — every previously-framed "platform limit" is now a scoped phase.

## Up next on this branch

1. **Phase 94 prereq — shared-helper lift.** ✅ Closed 2026-04-21. Per-cloud volume managers now live in `backends/{aws,gcp,azure}-common/volumes.go`; CR/ACA/ECS embed them unchanged. A small correctness fix fell out of the ECS lift (`fileSystemId` option is now populated even with `SOCKERLESS_ECS_AGENT_EFS_ID` set).
2. **Phase 94 — GCF + AZF real volumes.** ✅ Closed 2026-04-21. GCF attaches GCS buckets via `Services.GetService`/`UpdateService` on `fn.ServiceConfig.Service`; AZF attaches Azure Files shares via `WebApps.UpdateAzureStorageAccounts`.
3. **Phase 94b — Lambda EFS via `Function.FileSystemConfigs[]`.** ✅ Closed 2026-04-21. Reuses `awscommon.EFSManager` (shared with ECS); requires `SOCKERLESS_LAMBDA_SUBNETS` to be set.
4. **Phase 95 — FaaS invocation-lifecycle tracker.** ✅ Closed 2026-04-21. `core.InvocationResult` + `Store.{Put,Get,Delete}InvocationResult`; per-backend wiring on Lambda + GCF + AZF; 7 BUG-744 tests re-enabled.
5. **Phase 96 — Reverse-agent exec for CR Jobs + ACA Jobs.** Ports `sockerless-lambda-bootstrap` to two new overlay images + `/v1/cloudrun/reverse` + `/v1/aca/reverse` WebSocket endpoints. Unblocks Phase 98/98b/99 on those backends.
6. **Phase 97 — Docker labels charset-safe on GCP.** ✅ Closed 2026-04-21. `AsGCPLabels` filters charset-invalid values to `AsGCPAnnotations`; GCF carries the JSON blob via a `SOCKERLESS_LABELS` env var (Function v2 has no Annotations field).
7. **Phase 98 — Agent-driven filesystem + introspection ops.** Reverse-agent RPCs (`agent.Archive*`, `agent.Stat`, `agent.ProcList`, `agent.Changes`) for `docker cp` / `export` / `stat` / `top` / `diff`; ECS uses SSM `ExecuteCommand` for the archive path. Fixes BUG-751/752/753 on every backend that has the reverse-agent after Phase 96.
8. **Phase 98b — Agent-driven `docker commit`.** Opt-in via `SOCKERLESS_ENABLE_COMMIT`. Fixes BUG-750 on CR/ACA/Lambda; ECS Fargate gets the agent path too once SSM archive + ECR push land.
9. **Phase 99 — Agent-driven `pause` / `unpause`.** Reverse-agent SIGSTOP/SIGCONT broadcast to every process in the task; ECS Fargate uses SSM `signal`. Fixes BUG-749.
10. **Phase 100 — Docker backend pod synthesis.** Shared `sockerless-pod=<id>` label convention across all backends so Docker reproduces the pod grouping every cloud backend already emits. Fixes BUG-754.

Each phase ships as granular commits under a single mega-PR (per user direction). Bug tracking remains in `BUGS.md`; each re-enabled test is referenced against its phase.

## Live-cloud validation runbooks (need creds)

- **Phase 87 live-GCP** — Cloud Run Services validation against real GCP.
- **Phase 88 live-Azure** — ACA Apps validation against real Azure.
- **Phase 86 Lambda live track** — scripted already, deferred for session-budget reasons.
- **Phase 91 live-AWS EFS** — exercise the access-point provisioning path against real EFS once Phase 91 has real-AWS burn-in.
- **Phase 92 live-GCP GCS volumes**, **Phase 93 live-Azure Files volumes** — real-cloud burn-in of the new volume paths.

## Other queued

- **Phase 68** — Multi-Tenant Backend Pools (P68-002 → 010).
- **Phase 78** — UI Polish.

## Operational state

- Branch pushed to GitHub: **yes** (`continue-plan-post-113`, latest `cfeea5f`).
- Local `main` synced with `origin/main` through commit `0109667` (PR #113 merge).
- `origin-gitlab/main` is 5 commits behind GitHub; push when convenient.
