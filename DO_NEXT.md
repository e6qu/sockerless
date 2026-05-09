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

**In progress (this PR):**
- 121b-G Cloudrun TestMain: build + reference `sockerless-cloudrun-bootstrap` so `TestCloudRunJobTimeout` exercises the bootstrap timer end-to-end (the test was previously hidden by the build tag; 121b-C exposed the gap).
- 121b-H Driver consolidation, pattern B (live in `*-common`, shared by both backends in that cloud, value-at-construction config):
  - `gcp-common.IDTokenAccess` ← cloudrun + cloudrun-functions thin wrappers
  - `aws-common.IAMRoleAccess` ← ecs + lambda
  - `core.NoneInternalAccess` (already cloud-agnostic; keep here)
  - DNS adapters (`cloudMapDNS`, `cloudDNSZoneDNS`, `privateDNSZoneDNS`) → `*-common`
  - Network discovery adapters (`cloudMapDiscovery`, `cloudDNSDiscovery`, `acaCloudDNSDiscovery`) → `*-common`
- 121b-I Register `host-aliases` discovery as opt-in on every backend.
- 121b-J AZF DNS adapter → `private-dns-zone` (mirror ACA).
- 121b-K Lambda DNS + network discovery → `cloud-map` (mirror ECS).
- 121b-L AZF + ACA `id-token` access via Azure AD (`azidentity.DefaultAzureCredential`; audience required via `SOCKERLESS_<BACKEND>_AAD_AUDIENCE`; operator owns Easy Auth setup).

After Phase 121b ships: Phase 78 (UI polish — dark mode, design tokens, error UX, container detail modal, accessibility).

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
