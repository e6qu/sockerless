# Do Next

**Resume pointer for the next session.** Roadmap [PLAN.md](PLAN.md) · status [STATUS.md](STATUS.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · architecture [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Branch

`docs-streamline` — off `origin/main` at 9169d4b. Single work-branch rule applies; everything stacks here, no side branches. PR #127 + #128 already merged.

## Active — Phase 135 — Sim host model (ships first)

Architectural fix: services-that-execute provision **hosts** that run workloads through Docker, honouring the workload's `Architecture` field (default `linux/arm64`). Today's `simulators/<cloud>/shared/process.go::StartProcess` `os/exec`s workload binaries directly — the BUG-949 anti-pattern.

References: `feedback_sim_host_model.md`, `feedback_sim_workload_arch.md`, full sub-tasks in [PLAN.md](PLAN.md) § Phase 135.

Sub-task order:

1. **135a** — `HostRunner` interface + `DockerHost` impl in shared sim lib (cross-cloud).
2. **135b** — Migrate each cloud-product (Lambda, ECS, Cloud Run, GCF, Cloud Run Jobs, ACA, App Service/AZF) from `StartProcess` to `HostRunner` with per-product spec translator.
3. **135c** — Host-metadata services per execution-service: AWS IMDSv2 + ECS task v4; GCP `metadata.google.internal`; Azure IMDS expansion (`/metadata/instance` + identity). Lambda Runtime API stays as-is.
4. **135d** — Tests: static "no-os/exec-of-workload" check; per-product arch round-trip (`linux/arm64` + `linux/amd64`); per-product metadata-service round-trip; BUG-949 reproduction case.
5. **135e** — Docs: `specs/SIM_HOST_MODEL.md` (or `CLOUD_RESOURCE_MAPPING.md` section); update `simulators/README.md` § "container-oriented services" (it currently says "execute real OS processes").

Closes BUG-949 (real fix). New BUGS-tagged work files inline as the migration surfaces gaps.

## Queued (after Phase 135)

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

`BUG-972` (H, cloudrun+gcf — sim AR-proxy gate) and `BUG-949` (M, sim/gcp — gcf execs workload as host process; should dispatch via Docker honouring workload arch — closed by Phase 135). Detail in [BUGS.md](BUGS.md). Neither blocks Phase 135.
