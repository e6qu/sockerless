# Do Next

**Resume pointer for the next session.** Roadmap [PLAN.md](PLAN.md) · status [STATUS.md](STATUS.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · architecture [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Branch

`phase-126-access-driver` — off `origin/main` at 6875aa1 (PR #132 merged 2026-05-09). Single work-branch rule: everything stacks here, no side branches.

## Ordered roadmap (do them in this order)

1. **Phase 127 — Storage driver expansion** (`pd-ephemeral` / `efs-ephemeral` / `azure-files-ephemeral`). Sim prereq Compute Disks ✅.
2. **Phase 121b — Azure sim hardening** (cloud-faithful for ACA + AZF; mirror of Phase 121).
3. **Phase 78 — UI polish** (dark mode, design tokens, error UX, container detail modal, accessibility).

Driver phases (127) follow the 7-step template from [PLAN.md § Driver phase template](PLAN.md#driver-phase-template-124127). Each phase starts with a `specs/CLOUD_RESOURCE_MAPPING.md` design pass before code.

## Active — Phase 126 (Access driver, ready for PR + CI)

All sub-tasks shipped:

- **126a** — `api/access_driver.go`: `AccessMechanism` enum + `IsValid()` + `AllAccessMechanisms`. 4 mechanisms: iam-role, id-token, mTLS, none-internal.
- **126b** — `backends/core/access_driver.go`: `AccessDriver` interface (`Mechanism` + `WorkloadPrincipal` + `AuthenticatedClient`), registry, `NoneInternalAccess` default, `ParseAccessMechanismEnv()` (no-fallback semantics).
- **126c** — Per-backend adapters + wiring: cloudrun + cloudrun-functions `idTokenAccess` (wraps `idtoken.NewClient`), ECS + Lambda `iamRoleAccess` (returns `http.DefaultClient`; SigV4 happens at SDK layer), ACA + AZF `noneInternalAccess`. `BaseServer.Access` field defaults to `NoneInternalAccess{}`; backend startup overrides.
- **126d** — Every `idtoken.NewClient(ctx, url)` callsite migrated through `s.Access.AuthenticatedClient(ctx, url)`. `idtoken` import removed from cloudrun + cloudrun-functions backends. `cloudrun.Config.ServiceAccount` (sourced from `SOCKERLESS_CLOUDRUN_SERVICE_ACCOUNT`) added so the workload principal is configurable.

Spec: [specs/CLOUD_RESOURCE_MAPPING.md § Access driver](specs/CLOUD_RESOURCE_MAPPING.md#access-driver).

Next: open PR, watch CI, merge, then start Phase 127 (Storage driver expansion).

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
