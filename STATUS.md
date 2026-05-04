# Sockerless — Status

**Date: 2026-05-04 v19 (Phase 122i — BUG-944 deep-debug, 3 layers peeled)**

## Cell scoreboard

| Cell | Path | Last result | Active blocker |
|------|------|-----------|----------------|
| 1 GH × ECS (AWS) | n/a | ✅ GREEN 2026-04-30 (run 25075259911) | — |
| 2 GH × Lambda (AWS) | n/a | ✅ GREEN 2026-04-30 (run 25113565115) | — |
| 3 GL × ECS (AWS) | n/a | ✅ GREEN 2026-04-30 (pipeline 2489246177) | — |
| 4 GL × Lambda (AWS) | n/a | ✅ GREEN 2026-04-30 (pipeline 2490478943) | — |
| 5 GH × cloudrun | sockerless-cloudrun | ❌ stuck Initialize containers 43min then quota | **BUG-929** (`startSingleContainerService missing post-deploy invoke`) — independent of BUG-944. Cell 5 hangs forever instead of failing fast. |
| 6 GH × gcf | sockerless-gcf | ❌ exit 126 in `probe-cloud-urls` (cell 6 v4 run 25325397798, deploys all succeeded) | **BUG-944 layer 3** (just shipped `a7e3b00`) — verification pending image rebuild. |
| 7 GL × cloudrun | sockerless-cloudrun | ✅ Was GREEN at BUG-925 12-step (pipeline 2496721473, 1020s). `.gitlab-ci.yml` currently standard lint — needs swap-trigger to re-prove. | None known if re-triggered. |
| 8 GL × gcf | sockerless-gcf | ❌ CPU quota → docker daemon timeout 120s (pipeline 2497873066) | Same BUG-944 layer 3 + needs gitlab-runner-gcf image rebuild with the new fixes. |

## What shipped this session (Phase 122i — multi-iteration BUG-944 + observability + dispatcher)

13 commits on `phase-118-faas-pods` (currently at `a7e3b00`):

| Commit | Subject |
|--------|---------|
| `0f94a53` | runner-task 4Gi/2cpu (was 512Mi/1cpu OOM); rate-limit-aware scope.Verify backoff with +10%+1s buffer |
| `06561dd` | dispatcher: honor rate-limit window across cleanup ticker |
| `c6e7dee` | poller: proactive rate-limit back-off + per-run seen-set + 60s cadence |
| `71288bf` | revert gcf CPU=0.5 — Cloud Run gen2 rejects <1 vCPU (gen2 is the constraint, not a bug) |
| `9e39130` | (badge bump) |
| `df75d4d` | gcf pool: claim retry with exponential back-off (~5s total) for concurrent-burst caller waits |
| `a07d63b` | github-runner bootstraps tee backend stderr to Cloud Logging at debug level (was file-only — invisible) |
| `d85b652` | BUG-944 layer 1: GCS-Fuse `MountOptions=[implicit-dirs, ttl-secs=0, negative-ttl-secs=0]` on all 3 GCSVolumeSource constructions |
| `ee63dae` | BUG-944 layer 2: gcf pool-hit branch must call `attachVolumesToFunctionService`; idempotent attach by name |
| `a7e3b00` | BUG-944 layer 3: idempotent attach also compares bucket + MountOptions; replaces stale entries (pool-reused funcs from before MountOptions existed had matching names but no opts → previous skip-update was wrong) |

## Layered investigation summary — BUG-944 anatomy

**Symptom**: cell 6 SOLO got past CPU quota, deployed all services successfully, then exit 126 on the very first `run:` step (`docker exec` couldn't invoke the script).

| Layer | Hypothesis | Evidence | Outcome |
|-------|-----------|----------|---------|
| 0 | CPU quota | "Quota exceeded for total allowable CPU per project per region" in earlier runs | ❌ false — single cell solo cleared quota |
| 1 | GCS-Fuse cross-execution metadata cache (positive-TTL=60s, negative-TTL=5s) hides the fresh script file from the spawned container | architecturally plausible; AWS-side EFS = NFS strong consistency vs GCP GCS-Fuse object-store + cache | shipped `MountOptions = [implicit-dirs, ttl-secs=0, negative-ttl-secs=0]` (`d85b652`); cell 6 v3 still failed |
| 2 | Pool-hit branch returns early before calling `attachVolumesToFunctionService` → reused functions inherit zero volumes from prior allocation | verified `spec.template.spec.volumes = null` on the deployed function | shipped pool-hit branch attach + idempotent merge by name (`ee63dae`); cell 6 v4 still failed |
| 3 | Stale pool-volumes already had matching NAMES but no MountOptions — idempotent merge skipped the update so OLD format stayed, default cache-TTLs intact, exit 126 | verified the pool-reused function had volumes with names matching but no MountOptions | shipped MountOptions-aware comparison (`a7e3b00`); pool functions purged so fresh deploys can prove the chain works end-to-end. **Verification pending.** |

## Current stop-and-think

Per project rule: **no fallbacks, no workarounds, no fakes — and "transient errors are bugs"**. Two outstanding items I had previously dismissed as transient or out-of-scope:

1. **Runner image build flakiness**: `tar | installdependencies.sh` step in `tests/runners/github/dockerfile-gcf/Dockerfile` failed on first attempt (5fb50, 2c156a, 2 retries needed). Treated as transient — actually a real bug, currently being investigated (debug build in flight task `bakyy540j`).
2. **Multiple BUG-944 iterations**: I shipped a fix three times. Should have inspected the deployed function's `spec.template.spec.volumes` BEFORE assuming the fix worked. Established new diagnostic discipline: after every BUG-944-class fix, dump deployed Cloud Run service spec + verify field-by-field against the intended state.

## Live infra in `sockerless-live-46x3zg4imo` (us-central1)

- Dispatcher Cloud Run Service `github-runner-dispatcher-gcp` rev `00021-fb2` (60s poll + runSeen TTL 5m + proactive rate-limit back-off + cleanup-ticker rate-limit-window guard)
- `gitlab-runner-cloudrun`, `gitlab-runner-gcf` rev `00027-jkg` (revertible CPU=1; **needs rebuild with `a7e3b00`** before cell 8 retest)
- VPC `sockerless-vpc` + connector `sockerless-connector` (10.8.0.0/28)
- Cloud NAT `sockerless-nat` pinned to static IP `34.31.88.230` (don't auto-rotate)
- AR repos: `sockerless-live`, `docker-hub` (proxy), `gitlab-registry` (proxy), `sockerless-overlay/{gcf,cloudrun}`
- Secret Manager: `github-pat` v2 active, `gitlab-pat`, `gitlab-runner-token-{cloudrun,gcf}`
- GCS: `sockerless-live-46x3zg4imo-{build,runner-workspace}`
- Cloud Run quota preference: withdrawn (preferredValue=grantedValue=20000 vCPU/min). Don't recreate.
- Stale pool: 0 functions remaining (purged 2026-05-04 14:55 to force fresh MountOptions on next deploy)

## What did NOT work this session — preserve as anti-recipe

1. **gcf default CPU=0.5** — Cloud Run gen2 rejects fractional. Per user directive: gen2 is the constraint we keep (latest stable, no deprecated APIs). Reverted in `71288bf`.
2. **Quota-increase request** (`gcloud beta quotas preferences create CpuAllocPerProjectRegion 20000→200000`) — user explicitly rejected ("wrong path"). Withdrew.
3. **4 cells parallel** — exceeds Cloud Run regional `cpu_allocation` per-minute window. Solo runs avoid; pool back-off (`df75d4d`) amortizes some bursts.
4. **Idempotent volume merge by name only** — pool-reused volumes have matching names but stale MountOptions. Need full-shape compare (`a7e3b00`).
5. **Treating runner image build error as transient** — actually a real bug; user reminded "transients and flakiness must be treated as bugs". Active investigation.

See [BUGS.md](BUGS.md) § "Session 2026-05-04" for the per-bug fix shape. See [DO_NEXT.md](DO_NEXT.md) for the resume runbook.
