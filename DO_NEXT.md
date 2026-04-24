# Do Next

Snapshot pointer for the next session. Updated after every task.

## Branch state

`main` — PR #115 merged 2026-04-24 (Phases 96 / 98 / 98b / 99 / 100 / 101 / 102 + 13-bug audit sweep). PR #116 in flight with the state-doc update. 0 open bugs.

## Up next

1. **Live-cloud burn-in.** Every code path landed but the cloud-side validation runbooks are still on paper. Priority order: (a) Phase 86 Lambda live track (scripts exist), (b) Phase 87 live-GCP, (c) Phase 88 live-Azure, (d) Phase 91 real-EFS burn-in, (e) Phases 92/93/94 real-GCS / real-Azure-Files burn-in, (f) Phase 98b commit round-trip through ECR/AR/ACR. Each runbook should produce a scripted equivalent of `scripts/phase86/*.sh`.
2. **BUG-721 proper fix.** The SSM acknowledge-format workaround (backend dedupes retransmitted `output_stream_data` frames) needs a real wire-level match — Flags byte + PayloadDigest semantics — and that requires a live AWS agent to diff against.
3. **Phase 68 — Multi-Tenant Backend Pools.** P68-001 done; 9 sub-tasks remain (see PLAN.md).
4. **Phase 78 — UI Polish.** Dark mode, design tokens, container detail modal, accessibility, E2E smoke.

## Manual testing

[PLAN_ECS_MANUAL_TESTING.md](PLAN_ECS_MANUAL_TESTING.md) has the ECS + Lambda manual runbook. Post-PR-#115, the following tests need refresh:

- A46 (`docker pause`) — now works on every FaaS backend when the bootstrap writes `/tmp/.sockerless-mainpid` (Phase 99). Pause + unpause should be exercised end-to-end.
- A47 onwards — add agent-driven `docker cp`, `docker top`, `docker diff`, `docker container stat`, `docker export`, `docker commit` (with `SOCKERLESS_ENABLE_COMMIT=1`). All now implemented on Lambda/CR/ACA/GCF/AZF.
- C8/C9 (`diff`/`export`) — previously marked as Error/NotImplemented; now functional when agent is running.
- Track I (stateless restart verification) — also applies after a Phase 98b commit round-trip (image must be resolvable from registry post-restart).

## Operational state

- Local `main` synced with `origin/main` through commit `8494f79` (PR #115 merge).
- `origin-gitlab/main` is behind; push when convenient.
- Branch `state-post-pr115` open as PR #116 — pure doc update (PLAN/STATUS/WHAT_WE_DID compression + README badges).
