# Do Next

Roadmap [PLAN.md](PLAN.md) · status [STATUS.md](STATUS.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · architecture [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Branch

`main` clean. PR #135 merged 2026-05-09.

## Next — Phase 78 (UI polish)

Dark mode, design tokens, design pass across the 12 UI packages (core + 6 cloud backends + docker backend + docker frontend + admin + bleephub).

## Stacked follow-up PRs (queued; each its own mini-phase)

Each requires per-backend NetworkState model or operator infra not modeled today:
- **121b-deferred-I** Register `host-aliases` discovery as opt-in on every backend (env-var selection across 6 backends).
- **121b-deferred-J** AZF DNS adapter → `private-dns-zone` — needs AZF NetworkState model + zone creation flow.
- **121b-deferred-K** Lambda DNS + network discovery → `cloud-map` — needs Lambda VPC-mode wiring.
- **121b-deferred-L** AZF + ACA `id-token` access via Azure AD — needs `azure-common.AzureADAccess` type + Easy Auth integration design.
- Network discovery adapter consolidation — pass-through methods on `*Server`; consolidating requires moving the underlying methods.

## Background — Phase 121b (PR #135) recap

Cross-cutting work delivered in the merged PR:
- Azure sim cloud-faithful: Files data plane on disk, HS256-signed Azure AD JWT.
- All 6 backends' integration `TestMain` requires `SOCKERLESS_TEST_TARGET=sim|cloud` (no skips/fallbacks/build tags/legacy env).
- In-memory storage backing driver across all 6 backends.
- Driver consolidation pattern B (live in `*-common`, shared cross-backend within cloud).
- GCP sim Cloud Run service URI now routes through sim's own `/v2-services-invoke/` handler — fixed `*.run.app` cert mismatch by hosting URIs locally.
- `gcpcommon.ParseExecResult` extracted; gcf `invokeFunction` decodes bootstrap envelope before storing logs.
- `TagSet.Labels` propagated through pod_service; `dockerLabelsFromCloudRunService` reverses encoding on the read path.
- `scripts/check-latest-deps.sh` (pre-push + CI gate, no warn tier); `make upgrade-deps` per module + root fanout; all Go modules + TF providers + Azure SDK majors bumped.
- Publish workflow: dropped QEMU; per-arch native runners; tag format `<sha>-<arch>` + manifest-list assembly.
