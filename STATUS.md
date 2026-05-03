# Sockerless — Status

**Date: 2026-05-03**. PR #123 (`phase-118-faas-pods`) all standard CI green; runner cells 5-8 LIVE work in flight; Phase 122g architectural rebuild planned.

## Cells 1-4 (AWS) — GREEN (Phase 110, closed 2026-04-30)
| Cell | URL |
|---|---|
| 1 GH × ECS | https://github.com/e6qu/sockerless/actions/runs/25075259911 |
| 2 GH × Lambda | https://github.com/e6qu/sockerless/actions/runs/25113565115 |
| 3 GL × ECS | https://gitlab.com/e6qu/sockerless/-/pipelines/2489246177 |
| 4 GL × Lambda | https://gitlab.com/e6qu/sockerless/-/pipelines/2490478943 |

## Cells 5-8 (GCP) — Phase 122e closed 16 live-only bugs; BUG-927 architectural blocker identified; Phase 122g is the unblock

**End-of-session evidence**:
- Cell 7 https://gitlab.com/e6qu/sockerless/-/pipelines/2496190828 reports SUCCESS — but ZERO workload markers in Cloud Logging (no `apk add`, no `git clone`, no `eval-arithmetic`). FAKE success.
- Backend trace shows the gitlab-runner pattern: `attach 200 (216s blocking) → exec 409 'Container not running' → wait 200 → stop 304 × N`. Cloud Run Job (one-shot) cannot host the long-lived-build-container + per-stage-exec model.
- Cell 6 (gh × gcf) + Cell 8 (gl × gcf): blocked by BUG-923 (CreateFunction.Wait > 120s gitlab-runner timeout).
- Cell 5 (gh × cloudrun): same shape as cell 7 — would fake-succeed.

**Open architectural bugs**: BUG-923 (gcf CreateFunction.Wait), BUG-925 (postgres Service deploy + DNS), BUG-927 (cloudrun fake-success on stock images). All three dissolve under Phase 122g.

## Phase 122g unblock plan (next session — see DO_NEXT.md)
1. Lift `backends/lambda/image_inject.go` → `backends/gcp-common/image_inject.go` (shared overlay renderer).
2. New `agent/cmd/sockerless-cloudrun-bootstrap` binary mirroring `sockerless-lambda-bootstrap` (HTTP server bound to `$PORT`; recognises `execEnvelope{argv,tty,workdir,env,stdin}` shape; returns `{exitCode,stdout,stderr}` base64).
3. cloudrun: drop `isRunnerPattern` gating; ALL containers route to Cloud Run Service via overlay. `ContainerExec` = Path B HTTP POST (Lambda `execStartViaInvoke` analogue) to Service URL.
4. gcf: extend existing bootstrap to recognise the same `execEnvelope` shape; `ContainerExec` = Path B HTTP POST to `Function.ServiceConfig.Uri`.
5. Pre-deploy Service per shape via terraform (Lesson 1: stable shape catalog).
6. Pool semantics (Lesson 2): ContainerStop releases label `sockerless_allocation`; ContainerRemove deletes above pool cap.

This dissolves BUG-921/922/923/925/927.

## Architectural state (specs/CLOUD_RESOURCE_MAPPING.md, 1219 lines)
Authoritative reference. Today's session updated:
- Lesson 6 REVISED for BUG-927 (overlay IS needed on cloudrun for stock images)
- Lesson 8 ADDED — Lambda's `execStartViaInvoke` Path B as the gcf+cloudrun adaptation pattern
- Synthesis section rewritten for Phase 122g scope

## Branch state
- `main` synced with `origin/main` at PR #121 merge.
- `phase-118-faas-pods` (PR #123) — 18+ commits this session; all CI green; ready for merge once cells GREEN.
- `cell-workflows-on-main` (PR #124, throwaway) — close after cells GREEN; do NOT merge.
- `gitlab-cell-7-test`, `gitlab-cell-8-test` on `origin-gitlab` — pipelines for cells 7+8.

## Live infra in `sockerless-live-46x3zg4imo` (us-central1)
- Dispatcher Cloud Run Service `github-runner-dispatcher-gcp`
- gitlab-runner-cloudrun (rev `00021-rzl` post BUG-922 fix), gitlab-runner-gcf
- VPC `sockerless-vpc` + subnet `sockerless-connector-subnet` (10.8.0.0/28) + connector `sockerless-connector`
- AR repos: `sockerless-live`, `docker-hub` (proxy), `gitlab-registry` (proxy), `sockerless-overlay/gcf`
- Secret Manager: `github-pat`, `gitlab-pat`, `gitlab-runner-token-{cloudrun,gcf}`
- GCS: `sockerless-live-46x3zg4imo-build`, `sockerless-live-46x3zg4imo-runner-workspace`

See [PLAN.md](PLAN.md) for roadmap, [BUGS.md](BUGS.md) for bug log, [WHAT_WE_DID.md](WHAT_WE_DID.md) for narrative, [DO_NEXT.md](DO_NEXT.md) for resume runbook.
