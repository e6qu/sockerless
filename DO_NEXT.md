# Do Next

**Resume pointer for the next session.** Roadmap [PLAN.md](PLAN.md) · status [STATUS.md](STATUS.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · architecture [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Branch

`docs-streamline` — off `origin/main` at 9169d4b. Single work-branch rule applies; everything stacks here, no side branches. PR #127 + #128 already merged.

## Phase 135 — Sim host model + 3-tier coverage (PR #129, closing)

6 sub-tasks shipped (135a–f). Detail in [STATUS.md](STATUS.md#in-flight-on-docs-streamline-pr-129) and [specs/CLOUD_RESOURCE_MAPPING.md § Simulator host model](specs/CLOUD_RESOURCE_MAPPING.md#simulator-host-model-phase-135). 12 bugs closed (BUG-949 / 972 / 975 / 976 / 977 / 978 / 979 / 980 / 981 / 982 / 983 / 984). Sim CI runs on native `ubuntu-24.04-arm` runners (no QEMU). Awaiting your merge.

## Pick next

### Track A — Live-cloud cost gate (must precede next live session)

Phase 128 (job timeout) + Phase 129 remainder (BigQuery export, per-session labels, budget alert, session-end teardown). Without this, the regional-CPU-quota debt cycle from 2026-05-07 repeats. Detail in [PLAN.md](PLAN.md).

### Track B — Driver generalization (Phases 124–127)

Sim prereqs already shipped (PR #127): `generateIdToken` + Compute Disks. 124 Network · 125 DNS · 126 Access · 127 Storage expansion. Each phase: design pass in `specs/CLOUD_RESOURCE_MAPPING.md` first, then 7-step template (api enum → core registry → per-cloud-common impl → per-backend translator → operator config → no-fallbacks at resolve → migrate inline).

### Track C — Phase 121b Azure sim hardening

Mirror of Phase 121 cloud-faithful work for ACA + AZF.

## Standing rules

- **Never merge PRs** — user handles all merges. Push only.
- **Never push `main`.** Create branch off `origin/main`, PR it.
- **Single work-branch rule** — everything stacks here; no side branches.
- **State save after every task** — STATUS / PLAN / WHAT_WE_DID / DO_NEXT / BUGS.
- **Bugs file before fix** — every CI / live failure lands in BUGS.md as a one-liner before any analysis or fix attempt. Header counts updated in the same edit.
- **No fakes / no fallbacks** — every gap is a real bug; cross-cloud sweep on every find.
- **Sim parity per commit** — any new SDK call adds a sim handler + matrix row in the same commit.
- **Backend ↔ host primitive must match** — ECS in ECS, Lambda in Lambda, Cloud Run in Cloud Run, etc.
- **Sim binary arch ≠ workload arch** — sim runs host-native; workloads carry arch config (default `linux/arm64`); never `os/exec` workloads from a sim handler. (`feedback_sim_workload_arch.md` + `feedback_sim_host_model.md`)
- **Driver phase entry** — start with a `specs/CLOUD_RESOURCE_MAPPING.md` design pass before code.

## Open bugs

None. Detail in [BUGS.md](BUGS.md).
