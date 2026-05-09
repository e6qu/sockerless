# Do Next

Roadmap [PLAN.md](PLAN.md) · status [STATUS.md](STATUS.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · architecture [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Branch

`phase-121b-azure-sim-hardening` (PR #135) — off `origin/main` at 3e39e3a.

## Active — Phase 121b (single-PR scope)

Mirror of Phase 121 GCP sim hardening + cross-cutting test harness restructure + new drivers + driver consolidation.

**Done:**
- 121b-A Azure Files data plane on disk
- 121b-B HS256-signed Azure AD JWT
- 121b-C All 6 backends' integration `TestMain` requires `SOCKERLESS_TEST_TARGET=sim|cloud`. No skips/fallbacks/build tags/legacy env. Per-test `skipIfNoIntegration` deleted.
- 121b-D Azure terraform-test darwin fail-loud
- 121b-E `make/go-app.mk` + `make/go-lib.mk`: `test-integration` (sim) / `test-integration-cloud` (cloud). CI sets `SOCKERLESS_TEST_TARGET=sim`.
- 121b-F In-memory storage backing driver (`core.MemoryDriver`, `BackingMemory`).

**Done (this PR, additional):**
- 121b-G Cloudrun TestMain builds `sockerless-cloudrun-bootstrap` + sets `SOCKERLESS_CLOUDRUN_BOOTSTRAP` so `TestCloudRunJobTimeout` exercises the bootstrap timer end-to-end (test was previously hidden by build tag; 121b-C exposed the gap).
- 121b-H Driver consolidation pattern B (live in `*-common`, shared by both backends in that cloud, value-at-construction config):
  - `gcp-common.IDTokenAccess` (cloudrun + cloudrun-functions per-backend adapters deleted)
  - `aws-common.IAMRoleAccess` (ecs + lambda per-backend adapters deleted)
  - `core.NoneInternalAccess` used directly by ACA + AZF (per-backend adapters deleted)
  - `gcp-common.CloudDNSZoneDNS` (callback-based, cloudrun adapter deleted)
  - `aws-common.CloudMapDNS` (callback-based, ecs adapter deleted)
  - `azure-common.PrivateDNSZoneDNS` (callback-based, aca adapter deleted)

## Stacked follow-up PR (queued)

Each is its own mini-phase (requires per-backend NetworkState model that doesn't exist yet on target backends, or operator infra not modeled today):
- 121b-I Register `host-aliases` discovery as opt-in on every backend (env-var selection across 6 backends).
- 121b-J AZF DNS adapter → `private-dns-zone` — needs AZF NetworkState model + zone creation flow.
- 121b-K Lambda DNS + network discovery → `cloud-map` — needs Lambda VPC-mode wiring.
- 121b-L AZF + ACA `id-token` access via Azure AD — needs `azure-common.AzureADAccess` type + Easy Auth integration design.
- Network discovery adapter consolidation — pass-through methods on `*Server`; consolidating requires moving the underlying methods.

After Phase 121b stack: Phase 78 (UI polish).

## Standing rules

- **Never merge PRs** — user handles merges.
- **Never push `main`.** Branch off `origin/main`, PR it.
- **Single work-branch rule.**
- **State save after every task** — STATUS / PLAN / WHAT_WE_DID / DO_NEXT / BUGS.
- **Bugs file before fix** — every CI/live failure lands in BUGS.md before analysis.
- **No fakes / no fallbacks / no skips** — explicit config, fail loud on missing.
- **Sim parity per commit** — any new SDK call adds a sim handler + matrix row in the same commit.
- **Backend ↔ host primitive must match.**
- **Driver phase entry** — `specs/CLOUD_RESOURCE_MAPPING.md` design pass before code.
- **Cross-cloud is permanently off the table** — cloud-specific drivers extend the generic shape; cross-cloud duplication is fine, in-cloud duplication should consolidate into `*-common`.

## Open bugs

None.
