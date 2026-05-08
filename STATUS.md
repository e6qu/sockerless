# Sockerless — Status

**Where we are right now.** Roadmap [PLAN.md](PLAN.md) · resume [DO_NEXT.md](DO_NEXT.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Snapshot

| | |
|---|---|
| Active branch | `docs-streamline` (off `origin/main` at 9169d4b) — open PR #129 |
| Last merged | PR #128 — Makefile standardization + sim test stability (2026-05-08) |
| Milestone | **8/8 runner-integration cells GREEN** (since 2026-05-07); **CI fully green on linux/arm64 native runners** (Phase 135f, 2026-05-09). |
| Bugs | 984 filed · 984 fixed · **0 open** ([BUGS.md](BUGS.md)). Phase 135 (host model + 3-tier coverage) closed BUG-949/972/975/976/977/978/979/980/981/982/983/984. |
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

## In flight on `docs-streamline` (PR #129)

**Phase 135 — Sim host model.** 6 sub-tasks shipped:

- **135a** — `ContainerConfig.Architecture` field across all 3 sims; plumbed to Docker `ImagePull` + `ContainerCreate` Platform.
- **135b** — Workloads dispatch through Docker (no `os/exec` of workloads). `parsePlatform("")` errors at the shared-lib boundary. GCP Cloud Functions migrated from `StartProcess` to `StartContainerSync`; AWS ECS ExecuteCommand fallback dropped.
- **135c** — Host-metadata services per execution-service. AWS IMDSv2 + ECS task metadata v4 + instance-identity-document; GCP `metadata.google.internal/computeMetadata/v1/*`; Azure IMDS `/metadata/instance` + identity. Workload-host wiring via env (`GCE_METADATA_HOST` / `AWS_EC2_METADATA_SERVICE_ENDPOINT` / `IDENTITY_ENDPOINT`).
- **135d** — Static no-`os/exec`-of-workload check across all 3 sims.
- **135e** — Docs in [specs/CLOUD_RESOURCE_MAPPING.md § Simulator host model](specs/CLOUD_RESOURCE_MAPPING.md#simulator-host-model-phase-135) + simulators/README.md.
- **135f — three-tier coverage** — SDK (cloud.google.com/go/compute/metadata × 6 + aws-sdk-go-v2/feature/ec2/imds × 4 + azidentity ManagedIdentityCredential × 1), CLI (gcloud Compute Disks), Terraform (google_compute_disk). Plus `ubuntu-24.04-arm` native runners for sim/test/test-e2e/smoke (no QEMU needed; sim host == workload arch).

## Recently shipped (chronological)

| Date | PR | Headline |
|------|----|----|
| 2026-05-08 | #128 | Makefile standardization + per-app leaf Makefiles + stack orchestration; 17 doc updates; BUG-973/974 sim test stability (`Eventually` polling). |
| 2026-05-08 | #127 | Phase 129 #4 orphan pod-Service GC (owner-link via `CLOUD_RUN_JOB`); sim parity prep (GCP `generateIdToken` + Compute Disks); Phases 130/131/132 (bleephub workflow runs / workflows / apps + oauth REST + UI dispatch + AppsPage + OAuthPage). |
| 2026-05-07 | #123 | Phase 123 storage-backing driver (`emptyDir` / `gcs-sync` / `gcs-fuse`); 4 GCP runner cells GREEN (5/6/7/8); milestone closure → 8/8 GREEN. |
| 2026-04-30 | #122 | Phase 110 — 4 AWS runner cells GREEN (1/2/3/4); 32 bug fixes (845–876). |

Older PRs in [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Open bugs (0)

All filed bugs closed. New CI / live-cloud failures land in [BUGS.md](BUGS.md) per the standing rule.
