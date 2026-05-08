# Do Next

**Resume pointer for the next session.** Roadmap [PLAN.md](PLAN.md) · status [STATUS.md](STATUS.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · architecture [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Branch

`docs-streamline` — off `origin/main` at 9169d4b. Single work-branch rule applies; everything stacks here, no side branches. PR #127 + #128 already merged.

## Pick one

The 8/8 milestone is closed and the `phase-130` work is merged. Three live tracks ready to start. **Confirm choice with user before code lands.**

### Track A — Live-cloud cost gate (must precede next live session)

Goal: bring up a fresh GCP project safely. Without these, the regional-CPU-quota debt cycle from 2026-05-07 repeats. ~$90 was burned on an unmanaged 6-day live project.

1. **Phase 128** — runner job timeout (`SOCKERLESS_JOB_TIMEOUT_SECONDS`, default 1 h; SIGTERM → 30 s grace → SIGKILL; bootstrap reports exit 124). Per-cloud max: Cloud Run 24 h, Lambda 15 min, ECS ~unlimited.
2. **Remaining Phase 129** — BigQuery billing export at project create; per-session resource labels (`sockerless_session=<run-id>`); Cloud Billing Budget alert ($5 alert / $20 hard cap, label-scoped); session-end teardown (`make teardown-live-gcp` → `gcloud projects delete`).
3. Then bring up the next ephemeral GCP project and verify Phase 129 #4 (orphan-svc owner-link GC) live.

### Track B — Driver generalization (Phases 124–127)

Sim prereqs already shipped (PR #127): `generateIdToken` for Phase 126, Compute Disks for Phase 127.

- **124 — Network driver** (`host-aliases` / `cloud-dns` / `service-mesh` / `nat-gateway-only`).
- **125 — DNS driver** (`cloud-map` / `cloud-dns-zone` / `service-discovery` / `private-dns-zone`).
- **126 — Access driver** (`iam-role` / `id-token` / `mTLS` / `none-internal`).
- **127 — Storage driver expansion** (`pd-ephemeral` GCP, `efs-ephemeral` AWS already covered, `azure-files-ephemeral` Azure).

Each phase: design pass in `specs/CLOUD_RESOURCE_MAPPING.md` first, then the 7-step template (api enum → core registry → per-cloud-common impl → per-backend translator → operator config → no-fallbacks at resolve → migrate inline calls). Storage backing (Phase 123) is the worked pilot.

### Track C — Phase 121b Azure sim hardening

Mirror of Phase 121 (cloud-faithful sim work) for ACA + AZF. Open question per PLAN.md: how much of the GCP-style work (proto-JSON enum decoding, real OAuth2 token endpoints, label-filter syntax) transfers to Azure idioms. Lower-stakes but less defined; better as a fill-in track.

## Standing rules

- **Never merge PRs** — user handles all merges. Push only.
- **Never push `main`.** Create branch off `origin/main`, PR it.
- **Single work-branch rule** — everything stacks here; no side branches.
- **State save after every task** — STATUS / PLAN / WHAT_WE_DID / DO_NEXT / BUGS.
- **Bugs file before fix** — every CI / live failure lands in BUGS.md as a one-liner before any analysis or fix attempt. Header counts updated in the same edit.
- **No fakes / no fallbacks** — every gap is a real bug; cross-cloud sweep on every find.
- **Sim parity per commit** — any new SDK call adds a sim handler + matrix row in the same commit.
- **Backend ↔ host primitive must match** — ECS in ECS, Lambda in Lambda, Cloud Run in Cloud Run, etc.
- **Driver phase entry** — start with a `specs/CLOUD_RESOURCE_MAPPING.md` design pass before code.

## Open bugs

`BUG-972` (H, cloudrun+gcf — sim AR-proxy gate) and `BUG-949` (M, sim/gcp — eval-arithmetic GOOS). Detail in [BUGS.md](BUGS.md). Neither blocks the tracks above.
