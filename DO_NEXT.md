# Do Next

**Resume pointer for the next session.** Roadmap [PLAN.md](PLAN.md) · status [STATUS.md](STATUS.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · architecture [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Branch

`phase-127-storage-driver-expansion` — off `origin/main` at f1818b6 (PR #133 merged 2026-05-09). Single work-branch rule: everything stacks here, no side branches.

## Ordered roadmap (do them in this order)

1. **Phase 121b — Azure sim hardening** (cloud-faithful for ACA + AZF; mirror of Phase 121).
2. **Phase 78 — UI polish** (dark mode, design tokens, error UX, container detail modal, accessibility).

## Active — Phase 127 (Storage driver expansion, ready for PR + CI)

All sub-tasks shipped:

- **127a** — `core.storage_backing.go` extended with 3 new constants (`pd-ephemeral`, `efs-ephemeral`, `azure-files-ephemeral`) + 3 new `BackingSpec` payload structs (`PDEphemeralSpec`, `EFSEphemeralSpec`, `AzureFilesEphemeralSpec`). `SharedVolumeRef` carries the per-backing fields (PD size/zone, EFS FS+AP, Azure account+share, ReadOnly).
- **127b** — Per-cloud driver impls: `gcp-common.PDEphemeralDriver`, `aws-common.EFSEphemeralDriver`, `azure-common.AzureFilesEphemeralDriver`. Each is a `core.StorageBackingDriver` with a `CloudSpec` translator and no-op `PreExec`/`PostExec` (live filesystem; no sockerless-side data sync).
- **127c** — Per-backend registry wiring: cloudrun + cloudrun-functions register `pd-ephemeral`; ECS + Lambda gain a `storageBackings` registry pre-populated with `efs-ephemeral` (sharing the existing `EFSManager`); ACA + AZF gain a `storageBackings` registry pre-populated with `azure-files-ephemeral` (defaulting to the configured storage account).
- **127d** — 15 unit tests (5 per driver) covering Backing(), CloudSpec defaults + overrides + required-field rejection, PreExec/PostExec no-ops.

Spec: [specs/CLOUD_RESOURCE_MAPPING.md § Storage backing — ephemeral managed FS expansion](specs/CLOUD_RESOURCE_MAPPING.md#storage-backing--ephemeral-managed-fs-expansion).

Next: open PR, watch CI, merge, then start Phase 121b (Azure sim hardening).

## Standing rules

- **Never merge PRs** — user handles all merges. Push only.
- **Never push `main`.** Branch off `origin/main`, PR it.
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
