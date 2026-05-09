# Sockerless — Status

Roadmap [PLAN.md](PLAN.md) · resume [DO_NEXT.md](DO_NEXT.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Snapshot

| | |
|---|---|
| Active branch | none — `main` clean |
| Last merged | PR #136 — Phase 121b finish: driver consolidation, host-aliases everywhere, AZF/Lambda DNS, Azure AD access (2026-05-10) |
| Cells | 8/8 runner-integration cells GREEN since 2026-05-07. |
| Bugs | 0 open. |
| Live infra | None up. |

## Next — Phase 78 (UI polish)

Phase 121b complete (#135 + #136 merged). Next queued: Phase 78 (UI polish across the 12 UI packages). After that: Phase 68 (multi-tenant backend pools), Phases 91–94 (real per-cloud volume provisioning), live-cloud validation track. See [PLAN.md](PLAN.md) for the ordered list.

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
| 2026-05-10 | #136 | Phase 121b finish — driver consolidation, host-aliases everywhere, AZF cloud-dns + Lambda service-mesh, Azure AD access driver, DNS↔NetworkDiscovery gating. |
| 2026-05-09 | #135 | Phase 121b Azure sim hardening + cross-cutting test harness restructure + driver consolidation + GCP sim Cloud Run invoke routing + envelope parsing + label round-trip. |
| 2026-05-09 | #134 | Phase 127 storage driver expansion (pd-ephemeral / efs-ephemeral / azure-files-ephemeral). |
| 2026-05-09 | #133 | Phase 126 Access driver (iam-role / id-token / mTLS / none-internal). |
| 2026-05-09 | #132 | Phase 125 DNS driver. |
| 2026-05-09 | #131 | Phase 124 network discovery driver. |
| 2026-05-09 | #130 | Phase 128 runner job timeout. |
| 2026-05-09 | #129 | Phase 135 sim host model + native arm64 CI. |

Older PRs in [WHAT_WE_DID.md](WHAT_WE_DID.md).
