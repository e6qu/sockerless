# Sockerless — Status

**🎯 2026-05-07 — 8/8 cells GREEN. Cell 5 v17 + Cell 6 v17 both `all arithmetic checks pass`. Phase 123 (storage backing driver abstraction with gcs-sync) shipped end-to-end. Today's sweep: Phase 123 architecture + 8 supporting bug fixes (BUG-964/966/967/968/969/970 + multi-container memory OOM + workflow-yaml `file` package).**

## Cell scoreboard

| Cell | Path | State | Job / Pipeline URL | Notes |
|------|------|-------|--------------------|------|
| **1** GH × ECS | sockerless-ecs | ✅ GREEN | https://github.com/e6qu/sockerless/actions/runs/25075259911 | Phase 110 closed |
| **2** GH × Lambda | sockerless-lambda | ✅ GREEN | https://github.com/e6qu/sockerless/actions/runs/25113565115 | Phase 110 closed |
| **3** GL × ECS | sockerless-ecs | ✅ GREEN | https://gitlab.com/e6qu/sockerless/-/pipelines/2489246177 | Phase 110 closed |
| **4** GL × Lambda | sockerless-lambda | ✅ GREEN | https://gitlab.com/e6qu/sockerless/-/pipelines/2490478943 | Phase 117 closed |
| **5** GH × cloudrun | sockerless-cloudrun | ✅ **GREEN v17** | https://github.com/e6qu/sockerless/actions/runs/25506792865 | 12m20s, `all arithmetic checks pass` at `2026-05-07T16:08:23` (Phase 123 gcs-sync data plane closed BUG-965; main 2Gi closed mid-build OOM) |
| **6** GH × gcf | sockerless-gcf | ✅ **GREEN v17** | https://github.com/e6qu/sockerless/actions/runs/25506792937 | 14m30s, `all arithmetic checks pass`. Same fix family as cell 5; gcf-side BUG-964 closed by `invokePodServiceMain` skipIfNoStdin. |
| **7** GL × cloudrun | sockerless-cloudrun | ✅ **GREEN v54** | https://gitlab.com/e6qu/sockerless/-/jobs/14237010667 | 178s, `all arithmetic checks pass` at `2026-05-06T09:43:11.835` |
| **8** GL × gcf | sockerless-gcf | ✅ **GREEN v28** | https://gitlab.com/e6qu/sockerless/-/jobs/14234857458 | 147s, `all arithmetic checks pass` at `2026-05-06T08:05:04.053` |

### Cells 5+6 progression today (architectural layers peeled per iteration)

| Iteration | Failure | Bug filed |
|-----------|---------|-----------|
| v1–v3 | early stage failures, container-name RFC 1123 | — |
| v4 | cell 5 hung 10 min on exec; cell 6 stream framing | BUG-959 (mat) BUG-960 (Typed.Exec) |
| v5 | cell 5 hung; cell 6 framing | BUG-961 cloudrun skip-default-invoke + BUG-962 stdcopy framing |
| **v5b** | both reached `probe-cloud-urls` failed `cd: line N: can't open /__w/_temp/X.sh` | BUG-963 dispatcher GCS workspace mount |
| **v6** (current) | cell 5 deep into `clone-and-compile` → GCSFuse stale-handle; cell 6 → 10 min default-invoke hang on gcf | BUG-964 (gcf mirror of BUG-961) + BUG-965 (GCSFuse) |

## Today's commits (8 architectural fixes shipped)

| Commit | Fixes |
|--------|-------|
| `b223ecb` | BUG-956 + BUG-957 (cell 8 architectural close-out) |
| `e97399c` | BUG-958 (cloudrun multi-stage runner-pattern) |
| `2ba02f5` | BUG-959 (GH actions/runner materialize on second-arrival) |
| `e8a85e6` | BUG-960 (Typed.Exec routes through s.ExecStart) |
| `33e205a` | BUG-961 (cloudrun skip-default-invoke) + BUG-962 (stdcopy framing) |
| `c01067b` | BUG-963 (dispatcher GCS workspace mount) |

## Live infra (`sockerless-live-46x3zg4imo`, us-central1)

| Resource | Digest |
|----------|--------|
| `gitlab-runner-cloudrun` | `sha256:a221956c` (cell 7 v54 GREEN) |
| `gitlab-runner-gcf` | `sha256:d792e563` (cell 8 v28 GREEN) |
| `github-runner-dispatcher-gcp` | `sha256:1a3997bb` (BUG-963 wired) |
| `runner:cloudrun-amd64` | `sha256:4bd9dfa3` |
| `runner:gcf-amd64` | `sha256:1940ec7d` |
| VPC connector + Cloud NAT | `34.31.88.230` |
| GCS bucket | `sockerless-live-46x3zg4imo-runner-workspace` (workspace + JOB pod-Service share via this) |

## Next session

Phase 123 implementation per [DO_NEXT.md](DO_NEXT.md). Driver matrix:

| Driver | Use | Status |
|---|---|---|
| `emptyDir` | ephemeral, single-container | implementing |
| `gcs-sync` (NEW) | shared workspace, per-exec GCS sync, no FUSE | implementing — replaces GCSFuse for cells 5+6 |
| `gcs-fuse` (legacy) | cells 7+8 tar-pack persist | implementing (retained) |
| `pd-ephemeral`, `efs-ephemeral` | future | bookmarked — sockerless-managed lifecycle, scale-to-zero |

Rejected (always-on cost): NFS / Filestore / JuiceFS+Redis / persistent-mode PDs.

Co-shipped: BUG-964 (gcf `invokePodServiceMain` skip-default-invoke).

## Project state

- **Branch**: `phase-118-faas-pods @ fbd3d2b` — pushed.
- **PRs**: #123 (open, today's code) + #124 (throwaway trigger PR for cells 5+6).
- **Live project lifetime**: keep `sockerless-live-46x3zg4imo` until cells 5+6 also GREEN.
