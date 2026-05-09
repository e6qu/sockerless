# Sockerless — Status

Roadmap [PLAN.md](PLAN.md) · resume [DO_NEXT.md](DO_NEXT.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Snapshot

| | |
|---|---|
| Active branch | none — `main` clean |
| Last merged | PR #135 — Phase 121b Azure sim hardening + harness restructure + drivers + sim invoke routing (2026-05-09) |
| Cells | 8/8 runner-integration cells GREEN since 2026-05-07. |
| Bugs | 0 open. |
| Live infra | None up. |

## Next — Phase 78 (UI polish)

Phase 121b shipped. The work delivered in PR #135:

- ✓ **121b-A** Azure Files data plane on disk (`simulators/azure/files.go` `handleAzureFilesPath`).
- ✓ **121b-B** HS256-signed Azure AD JWT (`simulators/azure/auth.go` `mintAzureSimJWT`).
- ✓ **121b-C** All 6 backends' integration `TestMain` requires `SOCKERLESS_TEST_TARGET=sim|cloud`. No skips, no fallbacks, no `//go:build integration` tag, no `SOCKERLESS_INTEGRATION` env.
- ✓ **121b-D** Azure terraform-test darwin fail-loud.
- ✓ **121b-E** `make/go-app.mk` + `make/go-lib.mk`: `test-integration` (sim) / `test-integration-cloud` (cloud). CI sets `SOCKERLESS_TEST_TARGET=sim`.
- ✓ **121b-F** In-memory storage backing driver (`core.MemoryDriver`, `BackingMemory`, registered across all 6 backends).
- ✓ **121b-G** Cloudrun TestMain disables overlay path in sim mode (overlay would activate bootstrap-as-PID1 in long-lived HTTP-server mode → containers never exit). `TestCloudRunJobTimeout` removed; timer is fully unit-tested in `agent/cmd/sockerless-cloudrun-bootstrap/main_test.go`.
- ✓ **121b-H** Driver consolidation (pattern B — live in `*-common`).
- ✓ **121b-I** GCP sim Cloud Run service URI now routes through sim's own `/v2-services-invoke/{project}/{location}/{service}` handler instead of bogus `*.run.app` (which 401'd against public Google's wildcard cert). Sim runs the overlay container on demand, forwards the envelope POST body to the bootstrap.
- ✓ **121b-J** GCF `invokeFunction` parses bootstrap envelope before storing logs (extracted `gcpcommon.ParseExecResult`). Subprocess exit code now propagates through `inv.ExitCode`.
- ✓ **121b-K** GCF pod-Service propagates Docker labels via `TagSet.Labels`; `serviceToPodMemberContainer` reverses the encoding via `dockerLabelsFromCloudRunService`.
- ✓ Tooling: `scripts/check-latest-deps.sh` (pre-push + CI gate, no warn tier), `make upgrade-deps` per module + root fanout, all Go modules + TF providers + Azure SDK majors bumped.
- ✓ Publish workflow: dropped QEMU; per-arch native runners (`ubuntu-latest` + `ubuntu-24.04-arm`); tag format `<sha>-<arch>` + manifest-list assembly.
- **Deferred to stacked follow-up PRs** (each its own mini-phase, requires per-backend NetworkState model that doesn't exist yet on the target backends):
  - 121b-deferred-I Register `host-aliases` discovery as opt-in on every backend (env-var-driven selection across 6 backends).
  - 121b-deferred-J AZF DNS adapter → `private-dns-zone` — needs AZF NetworkState model + zone creation flow first.
  - 121b-deferred-K Lambda DNS + network discovery → `cloud-map` — needs Lambda VPC-mode wiring first.
  - 121b-deferred-L AZF + ACA `id-token` access via Azure AD — needs `azure-common.AzureADAccess` type + Easy Auth integration design.
  - Network discovery adapter consolidation (`cloudMapDiscovery`, `cloudDNSDiscovery`, `acaCloudDNSDiscovery`) — they're pass-throughs to `*Server` methods; consolidating requires moving the underlying methods too.

After 121b: Phase 78 (UI polish).

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
