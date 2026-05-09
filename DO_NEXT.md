# Do Next

**Resume pointer for the next session.** Roadmap [PLAN.md](PLAN.md) · status [STATUS.md](STATUS.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · architecture [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Branch

`phase-125-dns-driver` — off `origin/main` at 4905be4 (PR #131 merged 2026-05-09). Single work-branch rule: everything stacks here, no side branches.

## Ordered roadmap (do them in this order)

1. **Phase 126 — Access driver** (`iam-role` / `id-token` / `mTLS` / `none-internal`). Sim prereq `generateIdToken` ✅.
2. **Phase 127 — Storage driver expansion** (`pd-ephemeral` / `efs-ephemeral` / `azure-files-ephemeral`). Sim prereq Compute Disks ✅.
3. **Phase 121b — Azure sim hardening** (cloud-faithful for ACA + AZF; mirror of Phase 121).
4. **Phase 78 — UI polish** (dark mode, design tokens, error UX, container detail modal, accessibility).

Driver phases (126–127) follow the 7-step template from [PLAN.md § Driver phase template](PLAN.md#driver-phase-template-124127). Each phase starts with a `specs/CLOUD_RESOURCE_MAPPING.md` design pass before code.

## Active — Phase 125 (DNS driver, ready for PR + CI)

All sub-tasks shipped:

- **125a** — `api/dns_driver.go`: `DNSMechanism` enum + `IsValid()` + `AllDNSMechanisms`. 5 mechanisms: cloud-map, cloud-dns-zone, service-discovery, private-dns-zone, none.
- **125b** — `backends/core/dns_driver.go`: `DNSDriver` interface (`SearchDomain` + `Mechanism`), registry, no-op default (`none`), `ParseDNSMechanismEnv()` (no-fallback semantics), `DNSSearchDomainEnvIfSet()` helper.
- **125c** — Per-backend adapters + wiring: cloudrun cloud-dns-zone, ECS cloud-map, ACA private-dns-zone. `BaseServer.DNS` field defaults to no-op; backend startup overrides. FaaS backends (cloudrun-functions, lambda, azure-functions) keep no-op until per-cloud DNS adapters land.
- **125d** — `SOCKERLESS_DNS_SEARCH_DOMAIN` env wired through every `ContainerCreate` callsite (cloudrun, ECS, ACA, GCF, Lambda, AZF). Cloudrun + GCF bootstraps read the env var and append `search <suffix>` to `/etc/resolv.conf`.

Spec: [specs/CLOUD_RESOURCE_MAPPING.md § DNS driver](specs/CLOUD_RESOURCE_MAPPING.md#dns-driver).

Next: open PR, watch CI, merge, then start Phase 126 (Access driver).

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
