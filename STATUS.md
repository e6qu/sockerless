# Sockerless — Status

**Where we are right now.** Roadmap [PLAN.md](PLAN.md) · resume [DO_NEXT.md](DO_NEXT.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Snapshot

| | |
|---|---|
| Active branch | `docs-streamline` (off `origin/main` at 9169d4b) |
| Last merged | PR #128 — Makefile standardization + sim test stability (2026-05-08) |
| Milestone | **8/8 runner-integration cells GREEN** (since 2026-05-07) |
| Bugs | 974 filed · 972 fixed · 2 open ([BUGS.md](BUGS.md)) |
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

## Next up

**Phase 135 — Sim host model.** Architectural fix on the active branch: services-that-execute provision **hosts**, hosts run workloads through Docker honouring the workload's `Architecture` field (default `linux/arm64`). Today's `os/exec` workload pattern in `simulators/<cloud>/shared/process.go` is the BUG-949 anti-pattern. Bundles host-runner interface + per-product host adaptation + per-product host-metadata services + tests + docs. Detail in [PLAN.md](PLAN.md). Resume in [DO_NEXT.md](DO_NEXT.md).

## Recently shipped (chronological)

| Date | PR | Headline |
|------|----|----|
| 2026-05-08 | #128 | Makefile standardization + per-app leaf Makefiles + stack orchestration; 17 doc updates; BUG-973/974 sim test stability (`Eventually` polling). |
| 2026-05-08 | #127 | Phase 129 #4 orphan pod-Service GC (owner-link via `CLOUD_RUN_JOB`); sim parity prep (GCP `generateIdToken` + Compute Disks); Phases 130/131/132 (bleephub workflow runs / workflows / apps + oauth REST + UI dispatch + AppsPage + OAuthPage). |
| 2026-05-07 | #123 | Phase 123 storage-backing driver (`emptyDir` / `gcs-sync` / `gcs-fuse`); 4 GCP runner cells GREEN (5/6/7/8); milestone closure → 8/8 GREEN. |
| 2026-04-30 | #122 | Phase 110 — 4 AWS runner cells GREEN (1/2/3/4); 32 bug fixes (845–876). |

Older PRs in [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Open bugs (2)

| ID | Sev | Area | Hook |
|---|---|---|---|
| 972 | H | cloudrun + gcf | `ImagePull` rewrites Docker Hub refs to AR proxy unconditionally; sim has no AR proxy → 403. Gate on `s.config.EndpointURL == ""`. |
| 949 | M | sim/gcp | `gcf::simCommand` execs workload binary as a host process, ignoring workload arch. Fix: dispatch through Docker honouring workload `Architecture` (default `linux/arm64`); sim binary stays host-native. |
