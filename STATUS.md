# Sockerless — Status

**Date: 2026-05-03**. PR #123 (`phase-118-faas-pods`) all standard CI green; runner cells 5-8 LIVE work in flight; Phase 122g architectural rebuild planned.

## Cells 1-4 (AWS) — GREEN (Phase 110, closed 2026-04-30)
| Cell | URL |
|---|---|
| 1 GH × ECS | https://github.com/e6qu/sockerless/actions/runs/25075259911 |
| 2 GH × Lambda | https://github.com/e6qu/sockerless/actions/runs/25113565115 |
| 3 GL × ECS | https://gitlab.com/e6qu/sockerless/-/pipelines/2489246177 |
| 4 GL × Lambda | https://gitlab.com/e6qu/sockerless/-/pipelines/2490478943 |

## Cells 5-8 (GCP) — Phase 122g IN FLIGHT (overlay + Path B exec shipped)

**Phase 122g code-complete + deployed live (2026-05-03 v13)** — 7 commits:
1. `step 1`: lift `OverlayImageSpec` + renderer + tarball logic to `backends/gcp-common/image_inject.go`. gcf imports + sets prefix=`gcf-`; cloudrun uses same renderer with prefix=`cloudrun-`.
2. `step 2`: new `agent/cmd/sockerless-cloudrun-bootstrap` HTTP server (mirror of gcf bootstrap structure). Recognises envelope shape OR env-baked CMD. 7 unit tests pass. Runner image build pipeline (4 Dockerfiles + Makefiles) updated to bake the binary.
3. `step 2.5`: shared `ExecEnvelopeRequest/Response` + `PostExecEnvelope` helper in `gcp-common`. 5 unit tests pass.
4. `step 3a`: cloudrun ContainerCreate overlay-injects bootstrap when `SOCKERLESS_CLOUDRUN_BOOTSTRAP` env points at the binary. config.Image becomes the AR overlay URI; user.Entrypoint+user.Cmd ride as `SOCKERLESS_USER_*` env vars.
5. `step 3b/4`: cloudrun ExecStart Path B HTTP POST via `idtoken.NewClient` to Service URL. Reverse-agent WS reserved for interactive (TTY+stdin).
6. `step 5`: ContainerStart routes overlay-imaged containers to Cloud Run Service path (vs Job). New `useServicePath` helper triggers when image URI is in `sockerless-overlay/` AR repo.
7. `step 6`: gcf bootstrap extended with envelope shape; gcf ExecStart wired to Path B HTTP POST to Function URL.

**Live infra updates (2026-05-03 v13)**:
- AR images pushed: `runner:cloudrun-amd64` (rev `df504cdc…`), `gitlab-runner:cloudrun-amd64` (rev `ad45341c…`), `runner:gcf-amd64` (rev `18601a8e…`), `gitlab-runner:gcf-amd64` (rev `8431c6e7…`).
- Service redeployed: `gitlab-runner-cloudrun` rev `00023-dgh` + `gitlab-runner-gcf` rev `00020-5g8`.

**BUG-928 (Phase 122g new, fix shipped)**: cell 7 first 122g run progressed past auto-remove + start-cycle issues — Cloud Run Service WAS deployed for the gitlab-runner-helper permission container — but Cloud Run rejected with `terminated: Application failed to start; STARTUP TCP probe failed for port 8080`. Root cause: `VpcAccess_ALL_TRAFFIC` routes ALL container egress through the VPC connector subnet (10.8.0.0/28), which has no Cloud NAT — GCSFuse can't reach `storage.googleapis.com`, volume mount stalls, bootstrap never gets to bind PORT. Fix: switch to `VpcAccess_PRIVATE_RANGES_ONLY` (only RFC1918 traffic via connector; public Google APIs via platform egress). Shipped in `backends/cloudrun/{servicespec,jobspec}.go`.

**Cell 7 evidence chain**:
- v11 (pre-Phase-122g, post-BUG-922): pipeline 2496190828 SUCCESS but FAKE — zero workload markers in Cloud Logging.
- v12 (Phase 122g step 5 deployed): pipeline 2496241337 FAILED — Cloud Run Service deployed, but startup probe failed on port 8080 due to BUG-928 GCSFuse timeout.
- v13 (BUG-928 PRIVATE_RANGES_ONLY shipped): in flight at https://gitlab.com/e6qu/sockerless/-/pipelines/{TBD-after-monitor-completes}.

**Open architectural bugs**: BUG-923 (gcf CreateFunction.Wait — addressed by overlay pool reuse, awaiting verification), BUG-925 (postgres Service deploy + DNS — partly addressed by 122g, will revisit if cell 7 surfaces new evidence).

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
