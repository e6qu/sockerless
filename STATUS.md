# Sockerless — Status

**Date: 2026-05-04 v18 (Phase 122i — dispatcher rate-limit + pool quota fixes)**

## Cell scoreboard

| Cell | Path | Last result | Blocker |
|------|------|------------|---------|
| 1 GH × ECS (AWS) | n/a | ✅ GREEN (2026-04-30, run 25075259911) | — |
| 2 GH × Lambda (AWS) | n/a | ✅ GREEN (2026-04-30, run 25113565115) | — |
| 3 GL × ECS (AWS) | n/a | ✅ GREEN (2026-04-30, pipeline 2489246177) | — |
| 4 GL × Lambda (AWS) | n/a | ✅ GREEN (2026-04-30, pipeline 2490478943) | — |
| 5 GH × cloudrun | sockerless-cloudrun | ❌ stuck "Initialize containers" 43min then quota | BUG-929 (startSingleContainerService missing post-deploy invoke) — likely the hang. Independent of CPU quota. |
| 6 GH × gcf | sockerless-gcf | 🟡 SOLO got past CPU+OOM, deployed services, then exit 126 in `probe-cloud-urls` step | BUG-944 (GCS-Fuse mount latency: docker exec runs before file is visible inside container). |
| 7 GL × cloudrun | sockerless-cloudrun | ✅ Was GREEN once at 12-step BUG-925 fix (pipeline 2496721473, 1020s, all 5 arithmetic markers verified). Last attempts failed because `.gitlab-ci.yml` swap reverted to standard lint. | Re-trigger via swap. |
| 8 GL × gcf | sockerless-gcf | ❌ CPU quota → docker daemon timeout 120s | BUG-942 (pool burst) + BUG-944 (probably) |

## What shipped this session (Phase 122i)

7 commits on `phase-118-faas-pods` (currently at `df75d4d`):

| Commit | Subject |
|--------|---------|
| `0f94a53` | runner-task Resources: 4Gi/2cpu (was 512Mi/1cpu OOM) + rate-limit-aware scope.Verify backoff with +10%+1s buffer |
| `c6e7dee` | poller: proactive rate-limit back-off + per-run seen-set + 60s cadence |
| `06561dd` | dispatcher: honor rate-limit window across the cleanup ticker (skip Step() while inside) |
| `71288bf` | revert gcf CPU=0.5 default — Cloud Run gen2 rejects <1 vCPU |
| `9e39130` | (badge bump) |
| `df75d4d` | gcf pool: claim retry with exponential back-off (~5s total) — peers release before we deploy a new function |
| `a3a23cf` / `a9e43e4` | temp `.gitlab-ci.yml` swap + revert (cell 8 trigger pattern) |

## Live infra in `sockerless-live-46x3zg4imo` (us-central1)

- Dispatcher Cloud Run Service `github-runner-dispatcher-gcp` rev `00021-fb2` (poll 60s, runSeen TTL 5m, proactive rate-limit back-off)
- `gitlab-runner-cloudrun`, `gitlab-runner-gcf` (rev `00027-jkg`, BUG-923 async deploy + reverted CPU=1)
- VPC `sockerless-vpc` + `sockerless-connector-subnet` (10.8.0.0/28) + connector `sockerless-connector`
- Cloud NAT `sockerless-nat` pinned to static IP `34.31.88.230` (replaced auto-IP after GitHub abuse-flag)
- AR repos: `sockerless-live`, `docker-hub` (proxy), `gitlab-registry` (proxy), `sockerless-overlay/{gcf,cloudrun}`
- Secret Manager: `github-pat` (v2 active), `gitlab-pat`, `gitlab-runner-token-{cloudrun,gcf}`
- GCS: `sockerless-live-46x3zg4imo-{build,runner-workspace}`
- Cloud Run quota preference: withdrawn (preferredValue=grantedValue=20000 vCPU/min on `CpuAllocPerProjectRegion`)

## Branch state

- `main` synced at PR #121 merge.
- `phase-118-faas-pods` (PR #123) — 25+ commits this session; standard CI green at `df75d4d`; ready for merge once cells GREEN.
- `cell-workflows-on-main` (PR #124, throwaway) — close after cells 5+6 GREEN.

## What did NOT work this session (per "no fakes" rule)

1. **gcf CPU=0.5 default** — Cloud Run gen2 rejects fractional CPU. Reverted.
2. **Quota increase request** (`gcloud beta quotas preferences create CpuAllocPerProjectRegion 20000→200000`) — user rejected ("wrong path"). Withdrew.
3. **4 cells parallel** — exceeds `cpu_allocation` per-minute rate. Pool back-off (`df75d4d`) is the in-progress architectural fix.

See [BUGS.md](BUGS.md) § "Session 2026-05-04" for fix shape per BUG. See [DO_NEXT.md](DO_NEXT.md) for resume runbook.
