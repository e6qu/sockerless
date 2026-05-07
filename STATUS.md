# Sockerless — Status

**2026-05-07 — 8/8 cells GREEN.** The runner-integration milestone is closed: every cell-pair (GitHub × {ECS, Lambda, cloudrun, gcf} and GitLab × the same four) runs the full probe + git-clone + go-build + arithmetic suite end-to-end against real cloud infrastructure. Phase 123 (storage backing driver abstraction with `gcs-sync`) was the architectural pillar that closed cells 5+6. Narrative + what-failed-along-the-way: [WHAT_WE_DID.md](WHAT_WE_DID.md). Per-bug detail: [BUGS.md](BUGS.md). Roadmap (next architectural direction): [PLAN.md](PLAN.md). Resume pointer: [DO_NEXT.md](DO_NEXT.md).

## Cell scoreboard

| Cell | Path | State | Job / Pipeline URL |
|------|------|-------|--------------------|
| **1** GH × ECS | sockerless-ecs | GREEN | https://github.com/e6qu/sockerless/actions/runs/25075259911 |
| **2** GH × Lambda | sockerless-lambda | GREEN | https://github.com/e6qu/sockerless/actions/runs/25113565115 |
| **3** GL × ECS | sockerless-ecs | GREEN | https://gitlab.com/e6qu/sockerless/-/pipelines/2489246177 |
| **4** GL × Lambda | sockerless-lambda | GREEN | https://gitlab.com/e6qu/sockerless/-/pipelines/2490478943 |
| **5** GH × cloudrun | sockerless-cloudrun | GREEN v17 | https://github.com/e6qu/sockerless/actions/runs/25506792865 |
| **6** GH × gcf | sockerless-gcf | GREEN v17 | https://github.com/e6qu/sockerless/actions/runs/25506792937 |
| **7** GL × cloudrun | sockerless-cloudrun | GREEN v54 | https://gitlab.com/e6qu/sockerless/-/jobs/14237010667 |
| **8** GL × gcf | sockerless-gcf | GREEN v28 | https://gitlab.com/e6qu/sockerless/-/jobs/14234857458 |

Each green run executed: probe-capabilities → probe-kernel → probe-env → probe-parameters → probe-localhost-peer (postgres sidecar reachable on `localhost:5432`) → clone-and-compile (`git clone` of this repo + `go build` of `simulators/testdata/eval-arithmetic`) → 5 arithmetic invocations with expected results.

## Live infra (`sockerless-live-46x3zg4imo`, us-central1)

| Resource | Digest |
|----------|--------|
| `gitlab-runner-cloudrun` | `sha256:a221956c` |
| `gitlab-runner-gcf` | `sha256:d792e563` |
| `github-runner-dispatcher-gcp` | latest revision (rev 00023+) with `runner_workspace_backing=gcs-sync` on cloudrun + gcf labels |
| `runner:cloudrun-amd64` / `runner:gcf-amd64` | latest |
| VPC connector + Cloud NAT | `34.31.88.230` |
| GCS bucket | `sockerless-live-46x3zg4imo-runner-workspace` (gcs-sync per-exec tar objects) |

## Project state

- **Branch**: `phase-118-faas-pods` — pushed.
- **PRs**: #123 (open, today's code).
- **Live project**: `sockerless-live-46x3zg4imo` retained for follow-up work (deploy-hygiene + driver-generalization phases).

## Next session

See [DO_NEXT.md](DO_NEXT.md). The two architectural threads queued:

1. **Deploy hygiene** — orphan `sockerless-svc-*` GC after failed pipelines (BUG-970's structural fix only solves min=0; orphans from cancelled / failed runs still pin regional Cloud Run CPU quota).
2. **Driver-generalization roadmap** — extend the proven storage-backing-driver pattern to Network, DNS, and Access (Phases 124-127, see PLAN.md).
