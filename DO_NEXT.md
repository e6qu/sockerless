# Do Next

Roadmap [PLAN.md](PLAN.md) · status [STATUS.md](STATUS.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · architecture [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Branch

`docs/state-save-post-121b-finish` (PR #137). Started as a docs state-save after Phase 121b finish (#136); user re-scoped the same PR to carry Phase 78 (UI polish) so the branch carries both.

## Active — Phase 78 UI polish (PR #137)

Across the 12 UI packages (core + 6 cloud backends + docker backend + docker frontend + admin + bleephub):

- ✓ **Dark mode + design tokens.** `useTheme` + `ThemeToggle` (sidebar footer).
- ✓ **Error UX.** `ToastProvider` (top-right stack) + `useToast` + `useReportError`; `InlineError` (in-page banner) wired into every list page with Retry.
- ✓ **Container detail modal.** Native `<dialog>`-backed `Modal` + row-click on the Containers page.
- ✓ **Auto-refresh.** TanStack Query polling already set; visibility-pause was already correct (default).
- ✓ **Accessibility.** DataTable sort headers as buttons + `aria-sort`, clickable rows keyboard-activatable, AppShell skip-link + landmark labels, Spinner role.
- ✓ **Performance.** DataTable hover via CSS selectors, not inline handlers; bundle sizes verified.
- ✓ **Documentation.** Workspace + core READMEs.

CI green pending; PR ready for merge once it passes.

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
- Network-discovery adapter consolidation into `*-common`.
- Host-aliases discovery opt-in on every backend.
- AZF NetworkState model + per-network Private DNS zone.
- Lambda NetworkState + EC2/ServiceDiscovery clients + Cloud Map namespace lifecycle.
- New `api.AccessMechanismAzureAD` + `azurecommon.AzureADAccess`.
- DNS driver + cloud-side resource provisioning gated on matching NetworkDiscovery.
