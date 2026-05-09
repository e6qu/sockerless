# Do Next

Roadmap [PLAN.md](PLAN.md) · status [STATUS.md](STATUS.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · architecture [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Branch

`main` clean. PR #136 merged 2026-05-10 (Phase 121b finish).

## Next — Phase 78 (UI polish)

Across the 12 UI packages (core + 6 cloud backends + docker backend + docker frontend + admin + bleephub):

- Dark mode + design tokens (currently light-only, no token system).
- Error handling UX (toast/inline banners; today errors hit the console).
- Container detail modal (today inline expansion; modal lets users open multiple).
- Auto-refresh (manual refresh today; needs interval + visibility-aware cadence).
- Performance audit (TanStack Query hit-rate; bundle size per package).
- Accessibility (focus order, ARIA roles, keyboard nav, screen-reader labels).
- E2E smoke (Playwright on the Vite dev server against a sim-mode backend).
- Documentation (per-package README; component story for each shared piece).

## Queued (after 78)

- **Phases 91–94 — Real per-cloud volume provisioning.** Lifts the runner-task `emptyDir` fallback to real-workload provisioning of pd-ephemeral / efs-ephemeral / azure-files-ephemeral. Designs in `specs/CLOUD_RESOURCE_MAPPING.md` § Volume provisioning per backend.
- **Live-cloud validation track.** Per-backend live-cloud sweeps:
  - Lambda live (deferred from Phase 86).
  - Cloud Run Services / ACA Apps live (closed in code 2026-04-21 behind UseService/UseApp; live-cloud pending).
  - AZF + cloud-dns on Azure live (new in #136).
  - Lambda + service-mesh on AWS live (new in #136).
  - ACA / AZF + Azure AD access on Azure live (new in #136).

## Background — Phase 121b recap (#135 + #136, merged)

Initial scope (#135):
- Azure sim cloud-faithful (Files data plane on disk, HS256-signed Azure AD JWT).
- All-6-backends test harness restructured to `SOCKERLESS_TEST_TARGET=sim|cloud`.
- In-memory storage backing driver across all 6 backends.
- Driver consolidation pattern B (live in `*-common`).
- GCP sim Cloud Run service URI routes through sim's own `/v2-services-invoke/` handler.
- `gcpcommon.ParseExecResult` extracted; gcf decodes bootstrap envelope.
- `TagSet.Labels` propagated through pod_service.
- `scripts/check-latest-deps.sh` (pre-push + CI gate); `make upgrade-deps` fanout.
- Publish workflow: dropped QEMU; per-arch native runners; `<sha>-<arch>` + manifest-list.

Finish (#136):
- Network-discovery adapter consolidation into `*-common` (cloudMapDiscovery, cloudDNSDiscovery, acaCloudDNSDiscovery + their underlying *Server methods).
- Host-aliases discovery opt-in on every backend (`Config.NetworkDiscovery` typed field; `SOCKERLESS_<X>_NETWORK_DISCOVERY` env).
- AZF NetworkState model + per-network Private DNS zone provisioning + cloud-dns case.
- Lambda NetworkState + EC2/ServiceDiscovery clients + Cloud Map namespace lifecycle + service-mesh case.
- New `api.AccessMechanismAzureAD` + `azurecommon.AzureADAccess` (DefaultAzureCredential, per-request bearer token).
- DNS driver + cloud-side resource provisioning gated on matching NetworkDiscovery selection (no dead provisioning).
