# Sockerless — Status

**Date: 2026-05-03**. PR #123 (`phase-118-faas-pods`) all standard CI green; runner cells 5-8 LIVE work in flight.

## Cells 1-4 (AWS) — GREEN (Phase 110, closed 2026-04-30)
| Cell | URL |
|---|---|
| 1 GH × ECS | https://github.com/e6qu/sockerless/actions/runs/25075259911 |
| 2 GH × Lambda | https://github.com/e6qu/sockerless/actions/runs/25113565115 |
| 3 GL × ECS | https://gitlab.com/e6qu/sockerless/-/pipelines/2489246177 |
| 4 GL × Lambda | https://gitlab.com/e6qu/sockerless/-/pipelines/2490478943 |

## Cells 5-8 (GCP) — IN FLIGHT (Phase 122c→f) — session-end summary

**Phase 122e + 122f bugs closed**: BUG-907..921 + BUG-924 (16 live-only bugs). **Open architectural**: BUG-922, BUG-923, BUG-925.

**End-of-session cell status**:
- Cell 5 (GH × cloudrun): queued — picked up by self-hosted runner; will fail same shape as cell 7.
- Cell 6 (GH × gcf): queued — same.
- Cell 7 (GL × cloudrun): https://gitlab.com/e6qu/sockerless/-/pipelines/2496141650 failed at "Starting service postgres:16-alpine" — BUG-925: postgres routes correctly to Cloud Run Service (per isRunnerPattern), but per-container Service deploy >120s + Cloud Run Service URLs are HTTPS not docker-network port reachability.
- Cell 8 (GL × gcf): https://gitlab.com/e6qu/sockerless/-/pipelines/2496146014 failed at permission container — BUG-923 still: gcf Cloud Function deploy (Cloud Build ~30s + CreateFunction ~60-120s) exceeds 120s gitlab-runner timeout for one-shot containers. min_instance_count=1 fix only helps runner-pattern (long-lived).

**Remaining architectural work** (next session, multi-step per Phase 122f spec):

1. **Pre-deploy Service per runner shape** (Lesson 1): terraform-managed `sockerless-runner-helper`, `sockerless-runner-postgres`, etc. Cloud Run Services + Functions ready before any cell runs. Backend's ContainerCreate updates env + volume mounts on the existing Service rather than CreateFunction-per-call.

2. **Private DNS for service:port reachability**: Cloud DNS private zone via VPC connector — `postgres.sockerless.internal` resolves to the Service's internal IP. gitlab-runner / github-runner can connect on `postgres:5432` directly.

3. **Reverse-agent for `docker exec` on cloudrun + gcf** (Lesson 3, port from ACA).

4. **gitlab-runner timeout tuning** at the gitlab-runner side (config.toml has `[runners.docker] wait_for_services_timeout`; default 30s) — though this won't bypass the hardcoded 120s docker-daemon-connection timeout in `linux_set.go`.



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
