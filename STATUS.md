# Sockerless — Status

**2026-05-06 — Cells 1+2+3+4+7+8 GREEN (6/8 cells). Cells 5+6 (GitHub × GCP) still need a workflow trigger via PR push.** All required code is on `phase-118-faas-pods` (PR #123).

## Cell scoreboard (8 cells × cloud × runner × backend)

| Cell | Path | State | Job / Pipeline URL | Evidence |
|------|------|-------|--------------------|----------|
| **1** GH × ECS | sockerless-ecs | ✅ GREEN (2026-04-30) | https://github.com/e6qu/sockerless/actions/runs/25075259911 | Phase 110 closed |
| **2** GH × Lambda | sockerless-lambda | ✅ GREEN (2026-04-30) | https://github.com/e6qu/sockerless/actions/runs/25113565115 | Phase 110 closed |
| **3** GL × ECS | sockerless-ecs | ✅ GREEN (2026-04-30) | https://gitlab.com/e6qu/sockerless/-/pipelines/2489246177 | Phase 110 closed |
| **4** GL × Lambda | sockerless-lambda | ✅ GREEN (2026-04-30) | https://gitlab.com/e6qu/sockerless/-/pipelines/2490478943 | Phase 117 closed (BUG-875/876) |
| **5** GH × cloudrun | sockerless-cloudrun | ❌ FAILING | https://github.com/e6qu/sockerless/actions/runs/25431102860 | After 4 architectural fixes today (BUG-956/957/958/959/960), pod-Service materializes correctly but `execStartViaInvoke` POST never reaches bootstrap → 10 min HTTP timeout → exit 255. **BUG-961**. |
| **6** GH × gcf | sockerless-gcf | ❌ FAILING | https://github.com/e6qu/sockerless/actions/runs/25431102827 | Same fix stack as cell 5; got past materialize, exec ran, but response not docker-stream-framed → "Unrecognized input header: 115" → exit 1. **BUG-962**. |
| **7** GL × cloudrun | sockerless-cloudrun | ✅ **GREEN v54** 2026-05-06 09:43 UTC | https://gitlab.com/e6qu/sockerless/-/jobs/14237010667 | `Job succeeded duration_s=178.245`. `all arithmetic checks pass` at `2026-05-06T09:43:11.835` on Service `sockerless-svc-33dbd39babad`. |
| **8** GL × gcf | sockerless-gcf | ✅ **GREEN v28** 2026-05-06 08:05 UTC | https://gitlab.com/e6qu/sockerless/-/jobs/14234857458 | `Job succeeded duration_s=147.77`. `all arithmetic checks pass` at `2026-05-06T08:05:04.053` on Service `sockerless-svc-c547886ab439`. |

### Cloud Logging evidence (cells 7 + 8)

Runner traces in `sockerless-live-46x3zg4imo` (us-central1):
- Cell 7 (v54): https://console.cloud.google.com/logs/query;query=resource.type%3D%22cloud_run_revision%22%20resource.labels.service_name%3D%22gitlab-runner-cloudrun%22?project=sockerless-live-46x3zg4imo
- Cell 8 (v28): https://console.cloud.google.com/logs/query;query=resource.type%3D%22cloud_run_revision%22%20resource.labels.service_name%3D%22gitlab-runner-gcf%22?project=sockerless-live-46x3zg4imo

Per-stage pod-Service logs (recoverable via service_name filter — services were cleaned up post-run per discipline):
- Cell 7 v54 step_script: `sockerless-svc-33dbd39babad`
- Cell 8 v28 step_script: `sockerless-svc-c547886ab439`

Persist module evidence (proves /builds carried across pod-Services per BUG-957):
- Cell 8 v28: `persist save … 10959360 bytes -> gs://sockerless-volume-sockerless-live-46x3zg4imo-ce82db2431657f69/sockerless-volume.tar` (get_sources stage), then `persist restore … 10959872 bytes -> /builds` (step_script stage on the new Service).
- Cell 7 v54: same pattern at bucket `sockerless-volume-…-362203bfaa845a82`.

### Each "all arithmetic checks pass" exit gate

The cell yml asserts five arithmetic results:
- `3 + 4 * 2 = 11`
- `(10 - 3) * 2 = 14`
- `100 / 5 + 1 = 21`
- `2 * (3 + 4) - 1 = 13`
- `1.5 + 2.5 * 2 = 6.5`

Five `expected … got …` assertions plus the final `all arithmetic checks pass` line. Both cells 7 + 8 emitted that line — captured above with timestamps.

## Today's architectural stack (15 fixes total, 3 new this session)

| # | Fix | Bug |
|---|---|---|
| 1–12 | See WHAT_WE_DID.md § "Phase 122k third session" | 953/954/955 |
| 13 | `pendingMembersOfNetwork` filters already-materialized OpenStdin=true mains; sidecars (postgres) stay so each stage's pod-Service revision gets its own postgres copy. | **956** ✅ |
| 14 | gcf bootstrap got `persist.go` (ported from cloudrun); `handleInvoke` wraps subprocess + `saveAll`. New `OverlayImageSpec.BootstrapBinaryHash` + `HashBootstrapBinary` so updating the binary at the same path invalidates AR overlay caches. | **957** ✅ |
| 15 | cloudrun `ContainerStart` mirrors gcf BUG-955: already-running + OpenStdin=true + fresh stdinPipe → kick `invokeRunningRunnerStage`. cloudrun `ContainerStop` keeps the Service alive when OpenStdin=true. New `invokeRunningRunnerStage` helper. | **958** ✅ |

Detail in BUGS.md and WHAT_WE_DID.md. All 3 new fixes shipped on commits `b223ecb` (BUG-956 + BUG-957) and `e97399c` (BUG-958).

## Live infra in `sockerless-live-46x3zg4imo` (us-central1)

| Service | Rev | Digest |
|---|---|---|
| `gitlab-runner-cloudrun` | `00005-qnv` | `sha256:a221956c` ← v54 GREEN |
| `gitlab-runner-gcf` | `00060-72h` | `sha256:d792e563` ← v28 GREEN |
| `github-runner-dispatcher-gcp` | `00021-fb2` | unchanged |
| `runner:cloudrun-amd64` | latest | `sha256:2b4efebf` (rebuilt today with BUG-957/958) |
| `runner:gcf-amd64` | latest | `sha256:b3b9a9de` (rebuilt today with BUG-957) |
| VPC connector | `sockerless-connector` | Cloud NAT static IP `34.31.88.230` |

Cleanup discipline applied: every test run ends with `gcloud run services delete sockerless-svc-* / skls-*` + `gcloud run revisions delete <old>`.

## Project state

- **Branch**: `phase-118-faas-pods` at `e97399c`. Pushed.
- **PR #123**: open. Description needs update once cells 5+6 GREEN.
- **PR #124**: throwaway — exists to fire `pull_request`-triggered cells 5+6 against the current branch's content.
- **Live project lifetime**: keep `sockerless-live-46x3zg4imo` alive until cells 5+6 also GREEN.
