# Sockerless — Status

**Where we are right now.** Roadmap [PLAN.md](PLAN.md) · resume [DO_NEXT.md](DO_NEXT.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Snapshot

| | |
|---|---|
| Active branch | `phase-128-job-timeout` (off `origin/main` at 7b35d90) |
| Last merged | PR #129 — Phase 135 sim host model + 3-tier coverage + ARM-native CI (2026-05-09) |
| Milestone | **8/8 runner-integration cells GREEN** (since 2026-05-07); **sim host model shipped, CI fully green on native arm64** (Phase 135, 2026-05-09). |
| Bugs | 984 filed · 984 fixed · **0 open** ([BUGS.md](BUGS.md)). |
| Sim parity | 77/77 ✓ across current backends (AWS 33, GCP 16, Azure 28) — [specs/SIM_PARITY_MATRIX.md](specs/SIM_PARITY_MATRIX.md) |
| Live infra | None up. All projects torn down end of 2026-05-07. |

## Cell scoreboard

| Cell | Stack | State | URL |
|------|-------|-------|-----|
| 1 | GH × ECS | GREEN | [run](https://github.com/e6qu/sockerless/actions/runs/25075259911) |
| 2 | GH × Lambda | GREEN | [run](https://github.com/e6qu/sockerless/actions/runs/25113565115) |
| 3 | GL × ECS | GREEN | [pipeline](https://gitlab.com/e6qu/sockerless/-/pipelines/2489246177) |
| 4 | GL × Lambda | GREEN | [pipeline](https://gitlab.com/e6qu/sockerless/-/pipelines/2490478943) |
| 5 | GH × Cloud Run | GREEN v17 | [run](https://github.com/e6qu/sockerless/actions/runs/25506792865) |
| 6 | GH × GCF | GREEN v17 | [run](https://github.com/e6qu/sockerless/actions/runs/25506792937) |
| 7 | GL × Cloud Run | GREEN v54 | [job](https://gitlab.com/e6qu/sockerless/-/jobs/14237010667) |
| 8 | GL × GCF | GREEN v28 | [job](https://gitlab.com/e6qu/sockerless/-/jobs/14234857458) |

Each green run: probe-capabilities → probe-localhost-peer (postgres sidecar `localhost:5432`) → clone-and-compile (`git clone` + `go build` of `simulators/testdata/eval-arithmetic`) → 5 arithmetic invocations.

## Up next on `phase-128-job-timeout`

In flight: **Phase 128 — Runner job timeout**. 4/4 sub-tasks shipped:

- **128a** — Bootstrap timer (`runWithTimeout`, `jobTimeoutFromEnv`) in cloudrun + gcf bootstraps. SIGTERM → 30s grace → SIGKILL → exit 124. 5 unit tests.
- **128b** — `backends/core/job_timeout.go` shared helpers (`JobTimeoutEnvName`, `DefaultJobTimeoutSeconds=3600`, `JobTimeoutDefault()`, `JobTimeoutEnvIfUnset()`). Wired into cloudrun + gcf backend env injection (per-job override wins).
- **128c** — Cloud-native cap: cloudrun TaskTemplate.Timeout + ACA ReplicaTimeout derived from `core.JobTimeoutDefault()`, clamped to per-cloud max. Lambda already at 900s cap.
- **128d** — Integration test (alpine + sleep 9999 + SOCKERLESS_JOB_TIMEOUT_SECONDS=2; expects exit 124 + log line).

Spec: [specs/CLOUD_RESOURCE_MAPPING.md § Job lifecycle](specs/CLOUD_RESOURCE_MAPPING.md#job-lifecycle-timeouts-and-termination-phase-128).

Then Phase 124 → 125 → 126 → 127 → 121b → 78 per [PLAN.md § Roadmap (ordered)](PLAN.md#roadmap-ordered).

## Recently shipped (chronological)

| Date | PR | Headline |
|------|----|----|
| 2026-05-09 | #129 | Phase 135 sim host model + 3-tier coverage + native arm64 CI runners. 12 bugs closed (BUG-949/972/975-984). |
| 2026-05-08 | #128 | Makefile standardization + per-app leaf Makefiles + stack orchestration; 17 doc updates; BUG-973/974 sim test stability (`Eventually` polling). |
| 2026-05-08 | #127 | Phase 129 #4 orphan pod-Service GC (owner-link via `CLOUD_RUN_JOB`); sim parity prep (GCP `generateIdToken` + Compute Disks); Phases 130/131/132 (bleephub workflow runs / workflows / apps + oauth REST + UI dispatch + AppsPage + OAuthPage). |
| 2026-05-07 | #123 | Phase 123 storage-backing driver (`emptyDir` / `gcs-sync` / `gcs-fuse`); 4 GCP runner cells GREEN (5/6/7/8); milestone closure → 8/8 GREEN. |
| 2026-04-30 | #122 | Phase 110 — 4 AWS runner cells GREEN (1/2/3/4); 32 bug fixes (845–876). |

Older PRs in [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Open bugs (0)

All filed bugs closed. New CI / live-cloud failures land in [BUGS.md](BUGS.md) per the standing rule.
