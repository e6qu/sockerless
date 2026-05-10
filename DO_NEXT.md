# Do Next

Roadmap [PLAN.md](PLAN.md) · status [STATUS.md](STATUS.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · architecture [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Branch

`state-save-post-pr139` — PR #140 (Phase 81 + Phase 82 + state save post-#139). Awaiting CI + review/merge.

## Resume here

After PR #140 merges, start a new branch for **Phase 83 — sim UI parity.**

Lift the three sim UIs (sim-aws, sim-gcp, sim-azure) onto the same shell shape backend UIs use:

- Containers / Resources / Metrics pages with the shared `BackendApp` shell.
- ToastProvider + ErrorBoundary at the app root.
- ThemeToggle in the AppShell nav.
- Log tailer (reuse the SSE endpoint shipped in Phase 81 with the topology surface — sim-side instance lookup uses `/api/v1/topology/instances`).
- API console panel (reuse the `useLogStream` hook + the proxy endpoint).

The sims currently have minimal pages (391/372/366 lines compared to backends' multi-page shells). The cleanup should cut net code via shared components.

## Invariants (re-state on every commit)

- **Components stay decoupled.** No admin-required env vars on sims/backends/bleephub. Admin reads only what they already expose (`/v1/health`, `/v1/info`, env vars). For Phase 87 observability: components emit OTLP only when `OTEL_EXPORTER_OTLP_ENDPOINT` is set in their env. Unset = today's stdout behaviour.
- **No fallbacks.** Unknown config values fail-loud. No silent defaults.
- **CI green per commit.** Each commit must be independently testable.
- **Test target gating.** All backend integration tests require `SOCKERLESS_TEST_TARGET=sim|cloud` (no skip).
- **No docs-only PRs.** Pair docs updates with implementation work on the same branch / PR.

## Roadmap on this branch

Phases 79.2 → 80 → 81 → 82 ✓ all in #140. Next branch: 83 → 84 → 85 → 86 → 87. See [PLAN.md](PLAN.md) for sub-steps. Will likely split into multiple PRs once the natural seam appears (e.g. after Phase 86 ships, Phase 87 — observability — is independent and can land as its own PR).

## After this branch

- Phases 91–94 — real per-cloud volume provisioning (lifts `emptyDir` → real `pd-ephemeral` / `efs-ephemeral` / `azure-files-ephemeral`).
- Live-cloud validation track — Lambda live, Cloud Run Services + ACA Apps live, AZF cloud-dns + Lambda service-mesh + ACA/AZF Azure AD live.
