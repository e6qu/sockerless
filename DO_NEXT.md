# Do Next

Roadmap [PLAN.md](PLAN.md) · status [STATUS.md](STATUS.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · architecture [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Branch

`state-save-post-pr138` — PR #139 (open). Carries the post-#138 state save + the full Phase 80 admin UI Topology page. CI to verify after push.

## Resume here

**Phase 80 shipping on PR #139.** Admin UI now has `/ui/topology`:
- Project + instance tree with per-instance status polled every 2s (calls `GET /api/v1/topology/projects/{p}/instances/{i}/status`).
- Per-row Start / Stop / Rebuild (POST to lifecycle endpoints).
- Per-kind add/edit instance modal (sim/backend/bleephub) with auto-allocate-port button.
- Add/delete project modal with confirmation.
- Port registry card (configured ranges + claimed ports).

Replaces legacy `ProjectsPage` + `ProjectCreatePage` (deleted). `/ui/projects/:name` (ProjectDetailPage) + `/ui/projects/:name/logs` (ProjectLogsPage) still served until Phase 81 absorbs them.

**Next after #139 lands: Phase 81 — per-instance logs + live troubleshooting console.** Live tail per instance via SSE from admin (reads `.stack-pids/<name>.log`); combined-timeline view (sim + backend interleaved); API console panel (arbitrary HTTP requests against an instance).

## Invariants (re-state on every commit)

- **Components stay decoupled.** No admin-required env vars on sims/backends/bleephub. Admin reads only what they already expose (`/v1/health`, `/v1/info`, env vars). For Phase 87 observability: components emit OTLP only when `OTEL_EXPORTER_OTLP_ENDPOINT` is set in their env. Unset = today's stdout behaviour.
- **No fallbacks.** Unknown config values fail-loud. No silent defaults.
- **CI green per commit.** Each commit must be independently testable.
- **Test target gating.** All backend integration tests require `SOCKERLESS_TEST_TARGET=sim|cloud` (no skip).
- **No docs-only PRs.** Pair docs updates with implementation work on the same branch / PR.

## Roadmap on this branch

Phases 79.2 → 80 → 81 → 82 → 83 → 84 → 85 → 86 → 87. See [PLAN.md](PLAN.md) for sub-steps. Will likely split into multiple PRs once the natural seam appears (e.g. after Phase 86 ships, Phase 87 — observability — is independent and can land as its own PR).

## After this branch

- Phases 91–94 — real per-cloud volume provisioning (lifts `emptyDir` → real `pd-ephemeral` / `efs-ephemeral` / `azure-files-ephemeral`).
- Live-cloud validation track — Lambda live, Cloud Run Services + ACA Apps live, AZF cloud-dns + Lambda service-mesh + ACA/AZF Azure AD live.
