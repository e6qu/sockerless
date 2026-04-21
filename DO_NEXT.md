# Do Next

Snapshot pointer for the next session. Updated after every task.

## Branch state

`continue-plan-post-113` — successor to PR #113. Phase 91 (ECS EFS volumes) landed, BUG-735/736/737 fixed. Local commits unpushed. Queue below is the sequence picked up on 2026-04-21.

## Up next on this branch

1. **Phase 94** — GCF + AZF inherit helpers from Phase 92/93 via `backends/gcp-common` + `backends/azure-common`; no new cloud resources, just wiring.
4. **Phase 95** — FaaS invocation-lifecycle tracker (Lambda + GCF + AZF). Per-backend cloud-native completion signal (Lambda `Invoke` response + CloudWatch `END RequestId`; GCF/AZF HTTP response from the invoke URL). Re-enables 7 deleted tests. See `specs/CLOUD_RESOURCE_MAPPING.md` § "Per-invocation container state" for the per-backend mapping.
5. **Phase 96** — Reverse-agent exec for CR Jobs + ACA Jobs. Ports `sockerless-lambda-bootstrap` to two new overlay images + `/v1/cloudrun/reverse` + `/v1/aca/reverse` WebSocket endpoints.
6. **Phase 97** — Docker labels as GCP annotations / Azure tags on FaaS + CR/ACA. Fixes BUG-746's round-trip drop — switches `TagSet.AsGCPLabels` to split individual labels between GCP labels (charset-safe keys) and annotations (the JSON blob fallback).
7. **Phase 98** — Agent-driven `docker cp` / `export` / `stat` / `top` / `diff`. Reverse-agent RPCs (`agent.Archive*`, `agent.Stat`, `agent.ProcList`, `agent.Changes`) + ECS SSM archive helper. Fixes BUG-751/752/753 on every cloud backend that has the reverse-agent after Phase 96.
8. **Phase 98b** — Optional agent-driven `docker commit` behind `SOCKERLESS_ENABLE_COMMIT`. Fixes BUG-750 for CR/ACA/Lambda; ECS/Lambda Fargate stay NotImplemented on control-plane grounds.

**Platform limits** (documented, not fixable): BUG-748 (Lambda named volumes — no cross-invocation persistent storage), BUG-749 (`docker pause/unpause` — no cloud-native pause primitive across any backend), BUG-754 (Docker backend pods — docker daemon has no pod concept).

Each phase ships as granular commits under a single mega-PR (per user direction). Bug tracking remains in `BUGS.md`; each re-enabled test is referenced against its phase.

## Live-cloud validation runbooks (need creds)

- **Phase 87 live-GCP** — Cloud Run Services validation against real GCP.
- **Phase 88 live-Azure** — ACA Apps validation against real Azure.
- **Phase 86 Lambda live track** — scripted already, deferred for session-budget reasons.
- **Phase 91 live-AWS EFS** — exercise the access-point provisioning path against real EFS once Phase 91 has real-AWS burn-in.

## Other queued

- **Phase 68** — Multi-Tenant Backend Pools (P68-002 → 010).
- **Phase 78** — UI Polish.

## Operational state

- Branch pushed to GitHub: **no** (local-only, clean `continue-plan-post-113`).
- Local `main` synced with `origin/main` through commit `0109667` (PR #113 merge).
- `origin-gitlab/main` is 5 commits behind GitHub; push when convenient.
