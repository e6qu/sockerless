# Do Next

Roadmap [PLAN.md](PLAN.md) · status [STATUS.md](STATUS.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · architecture [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Branch

`docs/state-save-post-121b` (PR #136). Phase 121b finish — all 5 sub-tasks (A through E) committed; awaiting CI green.

## Active — Phase 121b finish (PR #136)

All sub-tasks complete; PR awaiting CI:

- ✓ **121b-finish-A** Network discovery adapter consolidation into `*-common` (pattern B). cloudMapDiscovery / cloudDNSDiscovery / acaCloudDNSDiscovery moved with their underlying *Server methods.
- ✓ **121b-finish-B** Host-aliases discovery opt-in on every backend. `Config.NetworkDiscovery` typed field; SOCKERLESS_<X>_NETWORK_DISCOVERY env var; per-backend supported set enforced at Validate.
- ✓ **121b-finish-C** AZF DNS adapter → `private-dns-zone`. AZF NetworkState model + per-network zone provisioning at NetworkCreate time.
- ✓ **121b-finish-D** Lambda DNS + network discovery → `cloud-map`. Lambda NetworkState{NamespaceID} + EC2/ServiceDiscovery clients + namespace lifecycle + service-mesh case in the discovery switch.
- ✓ **121b-finish-E** AZF + ACA `id-token` access via Azure AD. New `api.AccessMechanismAzureAD` + `azurecommon.AzureADAccess` (DefaultAzureCredential, per-request bearer token).

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
