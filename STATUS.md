# Sockerless — Status

Roadmap [PLAN.md](PLAN.md) · resume [DO_NEXT.md](DO_NEXT.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Snapshot

| | |
|---|---|
| Active branch | `docs/state-save-post-121b` (PR #136) — scope expanded to *finish* Phase 121b (formerly-deferred items folded in) |
| Last merged | PR #135 — Phase 121b initial scope: Azure sim hardening + harness restructure + drivers + sim invoke routing (2026-05-09) |
| Cells | 8/8 runner-integration cells GREEN since 2026-05-07. |
| Bugs | 0 open. |
| Live infra | None up. |

## In flight — Phase 121b finish (PR #136)

PR #136 started as a docs state-save after PR #135 merged; user pulled the deferred 121b items back into it so Phase 121b lands in one PR (initial #135 + finish #136). Sub-task list:

- **121b-finish-A** Network discovery adapter consolidation — move `cloudMapDiscovery` / `cloudDNSDiscovery` / `acaCloudDNSDiscovery` into `*-common` (callback-based, pattern B). They're pass-throughs to `*Server` methods today; consolidating requires moving the underlying methods too.
- **121b-finish-B** Register `host-aliases` discovery as opt-in on every backend with env-var-driven selection (`SOCKERLESS_<X>_NETWORK_DISCOVERY = cloud-map|cloud-dns|host-aliases|none`) across all 6 backends.
- **121b-finish-C** AZF DNS adapter → `private-dns-zone` — needs AZF NetworkState model + zone creation flow first.
- **121b-finish-D** Lambda DNS + network discovery → `cloud-map` — needs Lambda VPC-mode wiring first (config field for VPC subnets + security group).
- **121b-finish-E** AZF + ACA `id-token` access via Azure AD — needs `azure-common.AzureADAccess` type + Easy Auth integration design.

Order: A → B (depends on A's consolidation), then C, D, E in parallel where the per-backend NetworkState lifts allow.

After 121b finish: Phase 78 (UI polish).

Initial scope shipped in PR #135:

- ✓ **121b-A** Azure Files data plane on disk (`simulators/azure/files.go` `handleAzureFilesPath`).
- ✓ **121b-B** HS256-signed Azure AD JWT (`simulators/azure/auth.go` `mintAzureSimJWT`).
- ✓ **121b-C** All 6 backends' integration `TestMain` requires `SOCKERLESS_TEST_TARGET=sim|cloud`.
- ✓ **121b-D** Azure terraform-test darwin fail-loud.
- ✓ **121b-E** `make/go-app.mk` + `make/go-lib.mk` integration targets; CI sets `SOCKERLESS_TEST_TARGET=sim`.
- ✓ **121b-F** In-memory storage backing driver across all 6 backends.
- ✓ **121b-G** Cloudrun TestMain disables overlay path in sim mode; `TestCloudRunJobTimeout` removed (timer unit-tested).
- ✓ **121b-H** Driver consolidation (pattern B — live in `*-common`): IDTokenAccess, IAMRoleAccess, CloudDNSZoneDNS, CloudMapDNS, PrivateDNSZoneDNS.
- ✓ **121b-I** GCP sim Cloud Run service URI routes through sim's own `/v2-services-invoke/` handler.
- ✓ **121b-J** GCF `invokeFunction` parses bootstrap envelope; `gcpcommon.ParseExecResult` extracted.
- ✓ **121b-K** GCF pod-Service propagates Docker labels via `TagSet.Labels`; `dockerLabelsFromCloudRunService` reverses encoding.
- ✓ Tooling: `scripts/check-latest-deps.sh` (pre-push + CI gate); `make upgrade-deps` fanout; Azure SDK majors bumped.
- ✓ Publish workflow: dropped QEMU; per-arch native runners; tag format `<sha>-<arch>` + manifest-list.

Phase 121b finish (PR #136 — in flight) covers the formerly-deferred items: see "In flight" section above.

After 121b finish: Phase 78 (UI polish).

## Recently shipped

| Date | PR | Headline |
|---|---|---|
| 2026-05-09 | #135 | Phase 121b Azure sim hardening + cross-cutting test harness restructure + driver consolidation + GCP sim Cloud Run invoke routing + envelope parsing + label round-trip. |
| 2026-05-09 | #134 | Phase 127 storage driver expansion (pd-ephemeral / efs-ephemeral / azure-files-ephemeral). |
| 2026-05-09 | #133 | Phase 126 Access driver (iam-role / id-token / mTLS / none-internal). |
| 2026-05-09 | #132 | Phase 125 DNS driver. |
| 2026-05-09 | #131 | Phase 124 network discovery driver. |
| 2026-05-09 | #130 | Phase 128 runner job timeout. |
| 2026-05-09 | #129 | Phase 135 sim host model + native arm64 CI. |

Older PRs in [WHAT_WE_DID.md](WHAT_WE_DID.md).
