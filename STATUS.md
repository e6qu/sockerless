# Sockerless — Status

Roadmap [PLAN.md](PLAN.md) · resume [DO_NEXT.md](DO_NEXT.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Snapshot

| | |
|---|---|
| Active branch | `phase-121b-azure-sim-hardening` (PR #135) |
| Last merged | PR #134 — Phase 127 storage driver expansion (2026-05-09) |
| Cells | 8/8 runner-integration cells GREEN since 2026-05-07. |
| Bugs | 0 open. |
| Live infra | None up. |

## In flight — Phase 121b (PR #135)

Mirror of Phase 121 GCP sim hardening + cross-cutting test harness restructure + new drivers. **Decided scope (single PR):**

- ✓ **121b-A** Azure Files data plane on disk (`simulators/azure/files.go` `handleAzureFilesPath`).
- ✓ **121b-B** HS256-signed Azure AD JWT (`simulators/azure/auth.go` `mintAzureSimJWT`).
- ✓ **121b-C** All 6 backends' integration `TestMain` requires `SOCKERLESS_TEST_TARGET=sim|cloud`. No skips, no fallbacks, no `//go:build integration` tag, no `SOCKERLESS_INTEGRATION` env.
- ✓ **121b-D** Azure terraform-test darwin fail-loud.
- ✓ **121b-E** `make/go-app.mk` + `make/go-lib.mk`: `test-integration` (sim) / `test-integration-cloud` (cloud). CI sets `SOCKERLESS_TEST_TARGET=sim`.
- ✓ **121b-F** In-memory storage backing driver (`core.MemoryDriver`, `BackingMemory`, registered across all 6 backends).
- ⏳ **121b-G** Cloudrun TestMain builds `sockerless-cloudrun-bootstrap` + sets `SOCKERLESS_CLOUDRUN_BOOTSTRAP` (fixes `TestCloudRunJobTimeout` failure exposed by 121b-C).
- ⏳ **121b-H** Driver consolidation (pattern B — live in `*-common`, shared by both backends in that cloud):
  - `gcp-common.IDTokenAccess` (cloudrun + cloudrun-functions adapters become thin wrappers)
  - `aws-common.IAMRoleAccess` (ecs + lambda)
  - `core.NoneInternalAccess` (already cloud-agnostic)
  - DNS adapters: `cloudMapDNS`, `cloudDNSZoneDNS`, `privateDNSZoneDNS` move to `*-common`
  - Network discovery adapters: `cloudMapDiscovery`, `cloudDNSDiscovery`, `acaCloudDNSDiscovery` move to `*-common`
- ⏳ **121b-I** Register `host-aliases` discovery as opt-in on every backend.
- ⏳ **121b-J** AZF DNS adapter → `private-dns-zone` (mirror ACA).
- ⏳ **121b-K** Lambda DNS + network discovery → `cloud-map` (mirror ECS).
- ⏳ **121b-L** AZF + ACA `id-token` access via Azure AD (`azidentity.DefaultAzureCredential`; audience required via `SOCKERLESS_<BACKEND>_AAD_AUDIENCE`; operator owns Easy Auth setup).

After 121b: Phase 78 (UI polish).

## Recently shipped

| Date | PR | Headline |
|---|---|---|
| 2026-05-09 | #134 | Phase 127 storage driver expansion (pd-ephemeral / efs-ephemeral / azure-files-ephemeral). |
| 2026-05-09 | #133 | Phase 126 Access driver (iam-role / id-token / mTLS / none-internal). |
| 2026-05-09 | #132 | Phase 125 DNS driver. |
| 2026-05-09 | #131 | Phase 124 network discovery driver. |
| 2026-05-09 | #130 | Phase 128 runner job timeout. |
| 2026-05-09 | #129 | Phase 135 sim host model + native arm64 CI. |

Older PRs in [WHAT_WE_DID.md](WHAT_WE_DID.md).
