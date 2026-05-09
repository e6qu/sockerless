# Do Next

Roadmap [PLAN.md](PLAN.md) · status [STATUS.md](STATUS.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · architecture [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Branch

`docs/state-save-post-121b` (PR #136). Started as a docs state-save after PR #135 merged; user pulled the deferred 121b items back in so Phase 121b lands in this PR.

## Active — Phase 121b finish (PR #136)

Order picked so each item unblocks the next: A is mechanical and unblocks B's per-backend wiring; C and D need their respective NetworkState models lifted before the driver attaches.

- [ ] **121b-finish-A** Network discovery adapter consolidation. Move `cloudMapDiscovery` (ecs), `cloudDNSDiscovery` (cloudrun), `acaCloudDNSDiscovery` (aca) into their `*-common` packages. Pattern B — callback-based driver in `*-common`, backend-specific state passed via callbacks. Each adapter today is a pass-through to `*Server` methods (`resolveNetworkState`, `connectToService`, etc.); consolidating requires moving the underlying methods into the per-cloud common module too.
- [ ] **121b-finish-B** Register `host-aliases` discovery as opt-in on every backend. Env-var-driven selection: `SOCKERLESS_<X>_NETWORK_DISCOVERY = cloud-map|cloud-dns|host-aliases|none`. Each backend's `server.go` reads its env var and selects via switch. Empty = backend's traditional default; unknown = fail-loud. Covers all 6 backends (ecs, lambda, cloudrun, gcf, aca, azf).
- [ ] **121b-finish-C** AZF DNS adapter → `private-dns-zone`. Requires AZF NetworkState model (today AZF has no per-network state object) + zone creation flow. Wire `azurecommon.PrivateDNSZoneDNS` with the lookup callback.
- [ ] **121b-finish-D** Lambda DNS + network discovery → `cloud-map`. Requires Lambda VPC-mode wiring (config field for VPC subnet IDs + security group + Cloud Map service). Wire `awscommon.CloudMapDNS` + `cloudMapDiscovery` (post-A).
- [ ] **121b-finish-E** AZF + ACA `id-token` access via Azure AD. New `azure-common.AzureADAccess` type. AAD differs from Cloud Run id-tokens — Easy Auth is per-app config not per-call signing. Design pass first.

## After 121b finish

Phase 78 (UI polish): dark mode, design tokens, error handling UX, container detail modal, auto-refresh, performance audit, accessibility, E2E smoke, documentation.

## Background — Phase 121b initial scope (PR #135, merged) recap

Cross-cutting work delivered in #135:
- Azure sim cloud-faithful: Files data plane on disk, HS256-signed Azure AD JWT.
- All 6 backends' integration `TestMain` requires `SOCKERLESS_TEST_TARGET=sim|cloud`.
- In-memory storage backing driver across all 6 backends.
- Driver consolidation pattern B (live in `*-common`).
- GCP sim Cloud Run service URI routes through sim's own `/v2-services-invoke/` handler.
- `gcpcommon.ParseExecResult` extracted; gcf decodes bootstrap envelope before storing logs.
- `TagSet.Labels` propagated through pod_service; `dockerLabelsFromCloudRunService` reverses encoding.
- `scripts/check-latest-deps.sh` (pre-push + CI gate); `make upgrade-deps` fanout; Azure SDK majors + TF providers bumped.
- Publish workflow: dropped QEMU; per-arch native runners; tag format `<sha>-<arch>` + manifest-list.
