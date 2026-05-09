# Do Next

**Resume pointer for the next session.** Roadmap [PLAN.md](PLAN.md) · status [STATUS.md](STATUS.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · architecture [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Branch

`phase-124-network-driver` — off `origin/main` at d2d9e55 (PR #130 merged 2026-05-09). Single work-branch rule: everything stacks here, no side branches.

## Ordered roadmap (do them in this order)

1. **Phase 128 — Runner job timeout** (live-cloud cost gate). Hard cap on Cloud Run / Lambda / ECS task duration; default 1 h; SIGTERM → 30 s → SIGKILL; bootstrap reports exit 124. Operator override via dispatcher TOML `runner_job_timeout` + bootstrap env `SOCKERLESS_JOB_TIMEOUT_SECONDS`. **Must precede the next live-cloud session** — without it, hung subprocesses pin quota and burn money.
2. **Phase 124 — Network driver** (`host-aliases` / `cloud-dns` / `service-mesh` / `nat-gateway-only`).
3. **Phase 125 — DNS driver** (`cloud-map` / `cloud-dns-zone` / `service-discovery` / `private-dns-zone`). Depends on 124.
4. **Phase 126 — Access driver** (`iam-role` / `id-token` / `mTLS` / `none-internal`). Sim prereq `generateIdToken` ✅.
5. **Phase 127 — Storage driver expansion** (`pd-ephemeral` / `efs-ephemeral` / `azure-files-ephemeral`). Sim prereq Compute Disks ✅.
6. **Phase 121b — Azure sim hardening** (cloud-faithful for ACA + AZF; mirror of Phase 121).
7. **Phase 78 — UI polish** (dark mode, design tokens, error UX, container detail modal, accessibility).

Driver phases (124–127) follow the 7-step template from [PLAN.md § Driver phase template](PLAN.md#driver-phase-template-124127). Each phase starts with a `specs/CLOUD_RESOURCE_MAPPING.md` design pass before code.

## Active — Phase 124 (network discovery driver, ready for PR + CI)

4/4 sub-tasks shipped:

- **124a** — `api/network_discovery.go`: `NetworkDiscoveryKind` enum + `IsValid()` + `AllNetworkDiscoveryKinds`. 4 categories: host-aliases, cloud-dns, service-mesh, nat-gateway-only.
- **124b** — `backends/core/network_discovery_driver.go`: interface, registry, no-op default (nat-gateway-only), `ParseNetworkDiscoveryEnv()` (no-fallback semantics).
- **124c** — `backends/core/network_discovery_hostaliases.go`: in-process host-aliases impl.
- **124d** — Per-backend adapters + wiring: cloudrun cloud-DNS, ECS service-mesh, ACA cloud-DNS, GCF host-aliases. `BaseServer.NetworkDiscovery` field defaults to no-op; backend startup overrides. Cloudrun `NetworkConnect` register-A-record callsite migrated to driver-mediated call.

Spec: [specs/CLOUD_RESOURCE_MAPPING.md § Network discovery driver](specs/CLOUD_RESOURCE_MAPPING.md#network-discovery-driver-phase-124).

Next: open PR, watch CI, merge, then start Phase 125 (DNS driver).

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
