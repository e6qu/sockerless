# Sockerless — Status

**Date: 2026-05-03**. PR #123 (`phase-118-faas-pods`) all standard CI green; runner cells 5-8 LIVE work in flight.

## Cells 1-4 (AWS) — GREEN (Phase 110, closed 2026-04-30)
| Cell | URL |
|---|---|
| 1 GH × ECS | https://github.com/e6qu/sockerless/actions/runs/25075259911 |
| 2 GH × Lambda | https://github.com/e6qu/sockerless/actions/runs/25113565115 |
| 3 GL × ECS | https://gitlab.com/e6qu/sockerless/-/pipelines/2489246177 |
| 4 GL × Lambda | https://gitlab.com/e6qu/sockerless/-/pipelines/2490478943 |

## Cells 5-8 (GCP) — IN FLIGHT (Phase 122c→f)

All 4 cells have failed across 5+ iterations this session. **15+ live-only bugs** surfaced + closed. The remaining failures are architectural — Phase 122f is the proper-fix path documented in `specs/CLOUD_RESOURCE_MAPPING.md`:

- Switch runner-pattern containers from Cloud Run **Jobs** (one-shot) to Cloud Run **Service** (long-lived) — sockerless config flag `SOCKERLESS_GCR_USE_SERVICE=1` exists, needs VPC connector + reverse-agent for `docker exec`.
- Pre-deploy one Service per runner-image shape (ECS task-def pattern, Lesson 1).
- Reverse-agent for exec (Lesson 3 — already implemented in ACA, partial in cloudrun).

Current blockers: BUG-921 (cloudrun /start blocked on RunJob.Wait — closed code-side, needs full validation), BUG-922 (cloudrun auto-removes container after first exec — Cloud Run Job lifecycle vs runner expectation), BUG-923 (gcf ContainerCreate blocks 150-200s on Cloud Build + CreateFunction.Wait — same Wait pattern as BUG-921 in CREATE).

Live infra in `sockerless-live-46x3zg4imo`:
- Dispatcher Cloud Run Service `github-runner-dispatcher-gcp` rev `00006-j4v` (post BUG-907..912 fixes)
- gitlab-runner-cloudrun rev `00015-mmb` + gitlab-runner-gcf rev `00016-xc2` (post BUG-913..921 fixes)
- AR repos: `sockerless-live`, `docker-hub` (Docker Hub proxy), `gitlab-registry` (registry.gitlab.com proxy)
- Secret Manager: `github-pat`, `gitlab-pat`, `gitlab-runner-token-{cloudrun,gcf}`
- GCS buckets: `sockerless-live-46x3zg4imo-build`, `sockerless-live-46x3zg4imo-runner-workspace`

## Architectural state (specs/CLOUD_RESOURCE_MAPPING.md)
4 new sections this session (1063 lines total):
1. Runner job lifecycle (docker executor) — required cloud primitives
2. Per-backend container concerns matrix (21 concerns × 7 backends)
3. Lessons from ECS+lambda → cloudrun+gcf adjustments (7 lessons + Phase 122f synthesis)
4. Dispatcher scope rule (only spawns runners; sockerless config is runner-image-internal)

See [PLAN.md](PLAN.md) for roadmap, [BUGS.md](BUGS.md) for bug log, [WHAT_WE_DID.md](WHAT_WE_DID.md) for narrative, [DO_NEXT.md](DO_NEXT.md) for resume pointer.

## Branch state
- `main` synced with `origin/main` at PR #121.
- `phase-118-faas-pods` (PR #123) — 17+ commits this session; all CI green; ready for review/merge once cells GREEN.
- `cell-workflows-on-main` (PR #124, throwaway) — must NOT be merged; closes when cells 5+6 GREEN.
- `gitlab-cell-7-test` + `gitlab-cell-8-test` on `origin-gitlab` — fire pipelines for cells 7+8.
